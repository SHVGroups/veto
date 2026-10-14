// Package mcp proxies the Model Context Protocol's Streamable HTTP transport
// and puts Veto in front of every tool call. Agents point their MCP client
// at https://<veto>/mcp/<server> with a Veto agent key; Veto forwards
// everything to the upstream server unchanged except tools/call requests,
// which it decides first.
//
// An approved call is forwarded, rewritten to the safer variant when that
// was chosen. A stopped or still-held call is answered by Veto with a tool
// result whose isError is true and whose text says why, so the agent's model
// sees the outcome as an ordinary tool failure it can reason about.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shvgroups/veto/server/internal/app"
	"github.com/shvgroups/veto/server/internal/canonical"
	"github.com/shvgroups/veto/server/internal/connector"
	"github.com/shvgroups/veto/server/internal/domain"
)

// Headers an agent may send to give Veto context the MCP request lacks.
// Values may be percent-encoded.
const (
	HeaderPrincipal        = "Veto-Principal"
	HeaderPrincipalDisplay = "Veto-Principal-Display"
	HeaderRequest          = "Veto-Request"
	HeaderIdempotencyKey   = "Veto-Idempotency-Key"
)

// maxBody bounds a JSON-RPC request body.
const maxBody = 4 << 20

// headers forwarded in each direction. Authorization is never forwarded: the
// agent authenticates to Veto, and Veto authenticates to the upstream.
var (
	requestHeaders  = []string{"Content-Type", "Accept", "Mcp-Session-Id", "Mcp-Protocol-Version", "Last-Event-Id"}
	responseHeaders = []string{"Content-Type", "Mcp-Session-Id", "Mcp-Protocol-Version", "Cache-Control"}
)

// Proxy serves /mcp/{server}.
type Proxy struct {
	app         *app.App
	http        *http.Client
	holdTimeout time.Duration
	log         *slog.Logger
}

// New returns a proxy. client must not have a timeout, because SSE streams
// stay open; holdTimeout is how long a tools/call waits for a reviewer.
func New(a *app.App, client *http.Client, holdTimeout time.Duration, log *slog.Logger) *Proxy {
	return &Proxy{app: a, http: client, holdTimeout: holdTimeout, log: log}
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type callParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

// Serve handles one request for the named server on behalf of an agent.
func (p *Proxy) Serve(w http.ResponseWriter, r *http.Request, actor app.Actor, server string) {
	if actor.Kind != app.ActorAgent {
		http.Error(w, "the MCP proxy needs an agent API key", http.StatusForbidden)
		return
	}
	srv, conn, err := p.app.MCPUpstream(r.Context(), actor, server)
	if err != nil {
		writeHTTPError(w, err)
		return
	}
	if r.Method != http.MethodPost {
		p.forward(w, r, srv.UpstreamURL, conn, nil, nil)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var batch []rpcRequest
		if json.Unmarshal(trimmed, &batch) == nil {
			for _, m := range batch {
				if m.Method == "tools/call" {
					writeRPCError(w, nil, -32600, "Veto does not accept tools/call inside a JSON-RPC batch; send it on its own")
					return
				}
			}
		}
		p.forward(w, r, srv.UpstreamURL, conn, body, nil)
		return
	}
	var msg rpcRequest
	if err := json.Unmarshal(trimmed, &msg); err != nil || msg.Method != "tools/call" {
		p.forward(w, r, srv.UpstreamURL, conn, body, nil)
		return
	}
	p.toolCall(w, r, actor, server, srv.UpstreamURL, conn, msg)
}

func (p *Proxy) toolCall(w http.ResponseWriter, r *http.Request, actor app.Actor, server, upstream string, conn *connector.Connection, msg rpcRequest) {
	var params callParams
	if err := json.Unmarshal(msg.Params, &params); err != nil || params.Name == "" {
		writeRPCError(w, msg.ID, -32602, "tools/call needs params.name")
		return
	}
	if len(params.Arguments) == 0 || string(params.Arguments) == "null" {
		params.Arguments = json.RawMessage("{}")
	}
	tool := "mcp." + server + "." + params.Name
	if !app.ValidToolName(tool) {
		writeRPCError(w, msg.ID, -32602, "tool name contains characters Veto does not accept")
		return
	}
	req := app.SubmitRequest{
		Tool:    tool,
		Args:    params.Arguments,
		Request: header(r, HeaderRequest),
	}
	if id := header(r, HeaderPrincipal); id != "" {
		req.Principal = &domain.Principal{ID: id, Display: header(r, HeaderPrincipalDisplay)}
	}
	req.IdempotencyKey = header(r, HeaderIdempotencyKey)
	if req.IdempotencyKey == "" {
		if session := r.Header.Get("Mcp-Session-Id"); session != "" {
			req.IdempotencyKey = idempotencyKey(session, tool, params.Arguments)
		}
	}

	res, err := p.app.Submit(r.Context(), actor, domain.SourceMCP, req, p.holdTimeout)
	if err != nil {
		var ae *app.Error
		if errors.As(err, &ae) {
			writeRPCError(w, msg.ID, -32602, "Veto rejected the call: "+ae.Message)
			return
		}
		p.log.Error("mcp proxy submit failed", "error", err)
		writeRPCError(w, msg.ID, -32603, "Veto could not evaluate the call")
		return
	}

	switch res.Status {
	case domain.StatusApproved:
	case domain.StatusPendingReview:
		text := fmt.Sprintf("Veto is holding this call for human review (action %s). Retry the same call after a reviewer decides.", res.ID)
		if res.ReviewURL != "" {
			text += " Review it at " + res.ReviewURL
		}
		writeToolError(w, msg.ID, res, text)
		return
	default:
		text := fmt.Sprintf("Veto stopped this call (action %s).", res.ID)
		if res.Decision != nil {
			var labels []string
			for _, reason := range res.Decision.Reasons {
				labels = append(labels, app.ReasonLabel(reason))
			}
			if len(labels) > 0 {
				text += " Reason: " + strings.Join(labels, "; ") + "."
			}
			if res.Decision.Note != "" {
				text += " Reviewer note: " + res.Decision.Note
			}
		}
		writeToolError(w, msg.ID, res, text)
		return
	}

	d, err := p.app.ClaimProxiedRun(r.Context(), actor, res.ID)
	if err != nil {
		var ae *app.Error
		if errors.As(err, &ae) {
			writeToolError(w, msg.ID, res, "Veto did not run this call: "+ae.Message)
			return
		}
		writeRPCError(w, msg.ID, -32603, "Veto could not prepare the call")
		return
	}
	finalName, ok := strings.CutPrefix(d.FinalTool, "mcp."+server+".")
	if !ok {
		writeToolError(w, msg.ID, res, "Veto approved a variant that runs on a different server ("+d.FinalTool+"); it was not forwarded")
		return
	}
	params.Name, params.Arguments = finalName, d.FinalArgs
	rewrittenParams, _ := json.Marshal(params)
	msg.Params = rewrittenParams
	body, _ := json.Marshal(msg)

	outcome := &outcomeTracker{id: msg.ID}
	p.forward(w, r, upstream, conn, body, outcome)
	status, detail := outcome.result()
	detail["tool"] = d.FinalTool
	detail["option"] = d.Option
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Second)
	defer cancel()
	if err := p.app.RecordProxiedOutcome(ctx, actor, res.ID, status, detail); err != nil {
		p.log.Warn("could not record MCP outcome", "action", res.ID, "error", err)
	}
}

// forward sends a request upstream and streams the response back. When
// tracker is set, the response is inspected for the JSON-RPC result.
func (p *Proxy) forward(w http.ResponseWriter, r *http.Request, upstream string, conn *connector.Connection, body []byte, tracker *outcomeTracker) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else if r.Method == http.MethodPost {
		reader = r.Body
	}
	ctx := r.Context()
	req, err := http.NewRequestWithContext(ctx, r.Method, upstream, reader)
	if err != nil {
		http.Error(w, "bad upstream URL", http.StatusBadGateway)
		return
	}
	for _, h := range requestHeaders {
		if v := r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	if conn != nil {
		if err := connector.Authorize(req, *conn); err != nil {
			http.Error(w, "upstream credential is misconfigured", http.StatusBadGateway)
			return
		}
	}
	resp, err := p.http.Do(req)
	if err != nil {
		if tracker != nil {
			tracker.fail("upstream unreachable: " + err.Error())
		}
		p.log.Warn("mcp upstream request failed", "upstream", redact(upstream), "error", err)
		http.Error(w, "MCP upstream is unreachable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for _, h := range responseHeaders {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if tracker != nil {
		tracker.httpStatus = resp.StatusCode
		tracker.sse = strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream")
	}
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if tracker != nil {
				tracker.observe(buf[:n])
			}
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

// outcomeTracker reads a forwarded response, plain JSON or SSE, looking for
// the JSON-RPC response to the forwarded call.
type outcomeTracker struct {
	id         json.RawMessage
	httpStatus int
	sse        bool
	buf        bytes.Buffer
	failure    string
}

const trackLimit = 1 << 20

func (t *outcomeTracker) observe(b []byte) {
	if t.buf.Len() < trackLimit {
		t.buf.Write(b[:min(len(b), trackLimit-t.buf.Len())])
	}
}

func (t *outcomeTracker) fail(reason string) { t.failure = reason }

func (t *outcomeTracker) result() (domain.OutcomeStatus, map[string]any) {
	detail := map[string]any{"http_status": t.httpStatus}
	if t.failure != "" {
		detail["error"] = t.failure
		return domain.OutcomeFailed, detail
	}
	var messages [][]byte
	if t.sse {
		sc := bufio.NewScanner(bytes.NewReader(t.buf.Bytes()))
		sc.Buffer(make([]byte, 64<<10), trackLimit)
		for sc.Scan() {
			if data, ok := strings.CutPrefix(sc.Text(), "data:"); ok {
				messages = append(messages, []byte(strings.TrimSpace(data)))
			}
		}
	} else {
		messages = append(messages, t.buf.Bytes())
	}
	for _, m := range messages {
		var resp struct {
			ID     json.RawMessage `json:"id"`
			Result *struct {
				IsError bool `json:"isError"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(m, &resp) != nil || !bytes.Equal(bytes.TrimSpace(resp.ID), bytes.TrimSpace(t.id)) {
			continue
		}
		switch {
		case resp.Error != nil:
			detail["rpc_error"] = resp.Error.Message
			return domain.OutcomeFailed, detail
		case resp.Result != nil && resp.Result.IsError:
			detail["tool_error"] = true
			return domain.OutcomeFailed, detail
		case resp.Result != nil:
			return domain.OutcomeSucceeded, detail
		}
	}
	if t.httpStatus >= 400 {
		return domain.OutcomeFailed, detail
	}
	detail["note"] = "response did not contain a JSON-RPC result for the call"
	return domain.OutcomeSucceeded, detail
}

func writeToolError(w http.ResponseWriter, id json.RawMessage, res *app.ActionResult, text string) {
	writeJSON(w, map[string]any{
		"jsonrpc": "2.0",
		"id":      rawID(id),
		"result": map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
			"isError": true,
			"_meta":   map[string]any{"veto": map[string]any{"action_id": res.ID, "status": res.Status}},
		},
	})
}

func writeRPCError(w http.ResponseWriter, id json.RawMessage, code int, message string) {
	writeJSON(w, map[string]any{"jsonrpc": "2.0", "id": rawID(id), "error": map[string]any{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}

func writeHTTPError(w http.ResponseWriter, err error) {
	var ae *app.Error
	if errors.As(err, &ae) && ae.Code == app.CodeNotFound {
		http.Error(w, ae.Message, http.StatusNotFound)
		return
	}
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func rawID(id json.RawMessage) any {
	if len(id) == 0 {
		return nil
	}
	return id
}

func header(r *http.Request, name string) string {
	v := r.Header.Get(name)
	if d, err := url.PathUnescape(v); err == nil {
		return d
	}
	return v
}

func idempotencyKey(session, tool string, args json.RawMessage) string {
	h, err := canonical.ArgsHashJSON(tool, args)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(append([]byte("mcp\n"+session+"\n"), h...))
	return "mcp:" + hex.EncodeToString(sum[:16])
}

func redact(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "invalid-url"
	}
	u.RawQuery, u.User = "", nil
	return u.String()
}
