package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shvgroups/veto/server/internal/app"
	"github.com/shvgroups/veto/server/internal/apptest"
	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/store"
)

// fakeUpstream is a minimal MCP server over Streamable HTTP.
type fakeUpstream struct {
	mu    sync.Mutex
	calls []map[string]any
	auth  []string
}

func (f *fakeUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var msg map[string]any
	_ = json.NewDecoder(r.Body).Decode(&msg)
	f.mu.Lock()
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	f.mu.Unlock()
	id, _ := json.Marshal(msg["id"])
	switch msg["method"] {
	case "initialize":
		w.Header().Set("Mcp-Session-Id", "sess-1")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2026-07-28","capabilities":{"tools":{}}}}`, id)
	case "tools/call":
		f.mu.Lock()
		f.calls = append(f.calls, msg["params"].(map[string]any))
		f.mu.Unlock()
		params := msg["params"].(map[string]any)
		if params["name"] == "stream_report" {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\"}\n\n")
			fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{\"content\":[],\"isError\":true}}\n\n", id)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"content":[{"type":"text","text":"done"}]}}`, id)
	default:
		w.WriteHeader(http.StatusAccepted)
	}
}

func setup(t *testing.T) (*apptest.Harness, *Proxy, *fakeUpstream) {
	t.Helper()
	h := apptest.New(t)
	ctx := context.Background()
	up := &fakeUpstream{}
	srv := httptest.NewServer(up)
	t.Cleanup(srv.Close)
	secret := "upstream-token"
	conn, err := h.App.CreateConnection(ctx, h.Owner, app.ConnectionInput{Name: "crm-mcp", BaseURL: srv.URL, AuthScheme: "bearer", Secret: &secret})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.App.CreateMCPServer(ctx, h.Owner, app.MCPServerInput{Name: "crm", UpstreamURL: srv.URL, ConnectionID: &conn.ID}); err != nil {
		t.Fatal(err)
	}
	for _, in := range []app.ToolInput{
		{Name: "mcp.crm.lookup", RiskClass: domain.RiskRead},
		{Name: "mcp.crm.stream_report", RiskClass: domain.RiskRead},
		{Name: "mcp.crm.update_contact", RiskClass: domain.RiskMedium,
			Safer: json.RawMessage(`[{"op":"set","path":"dry_run","value":true}]`)},
	} {
		if _, err := h.App.SaveTool(ctx, h.Owner, in); err != nil {
			t.Fatal(err)
		}
	}
	p := New(h.App, &http.Client{}, 2*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return h, p, up
}

func rpc(t *testing.T, p *Proxy, actor app.Actor, body string, headers map[string]string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp/crm", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	p.Serve(rec, req, actor, "crm")
	var out map[string]any
	if strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec, out
}

func TestPassthroughAndUpstreamCredential(t *testing.T) {
	h, p, up := setup(t)
	rec, out := rpc(t, p, h.Agent, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, nil)
	if rec.Code != 200 || rec.Header().Get("Mcp-Session-Id") != "sess-1" || out["result"] == nil {
		t.Fatalf("initialize: %d %v %s", rec.Code, rec.Header(), rec.Body)
	}
	if up.auth[0] != "Bearer upstream-token" {
		t.Fatalf("upstream auth %q", up.auth[0])
	}
}

func TestApprovedCallIsForwardedAndRecorded(t *testing.T) {
	h, p, up := setup(t)
	rec, out := rpc(t, p, h.Agent, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"lookup","arguments":{"email":"a@acme.com"}}}`,
		map[string]string{HeaderRequest: "Find%20Acme%27s%20contact"})
	result := out["result"].(map[string]any)
	if rec.Code != 200 || result["isError"] == true {
		t.Fatalf("response %s", rec.Body)
	}
	if len(up.calls) != 1 || up.calls[0]["name"] != "lookup" {
		t.Fatalf("upstream calls %v", up.calls)
	}
	actions, _, _ := h.Store.ListActions(context.Background(), h.Owner.WorkspaceID, store.ActionFilter{})
	if len(actions) != 1 || actions[0].Source != domain.SourceMCP || actions[0].Request != "Find Acme's contact" || actions[0].Outcome == nil || *actions[0].Outcome != "succeeded" {
		t.Fatalf("recorded action %+v", actions)
	}
}

func TestSaferVariantIsForwarded(t *testing.T) {
	h, p, up := setup(t)
	h.Model.Set(map[string]float64{"proceed": 0.02, "proceed_safer": 0.96, "stop": 0.02}, map[string]float64{"matches_request": 0.95}, nil)
	_, out := rpc(t, p, h.Agent, `{"jsonrpc":"2.0","id":"a","method":"tools/call","params":{"name":"update_contact","arguments":{"id":"c1","phone":"555"}}}`,
		map[string]string{HeaderRequest: "Update the phone number"})
	if r, ok := out["result"].(map[string]any); !ok || r["isError"] == true {
		t.Fatalf("response %v", out)
	}
	args := up.calls[0]["arguments"].(map[string]any)
	if args["dry_run"] != true || args["phone"] != "555" {
		t.Fatalf("forwarded args %v", args)
	}
}

func TestStoppedAndHeldCallsReturnToolErrors(t *testing.T) {
	h, p, up := setup(t)
	h.Model.Set(map[string]float64{"proceed": 0.01, "proceed_safer": 0.01, "stop": 0.98}, nil, nil)
	_, out := rpc(t, p, h.Agent, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"update_contact","arguments":{"id":"c1"}}}`, nil)
	result := out["result"].(map[string]any)
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if result["isError"] != true || !strings.Contains(text, "Veto stopped this call") {
		t.Fatalf("stop response %v", out)
	}

	h.Model.Set(map[string]float64{"proceed": 0.4, "proceed_safer": 0.3, "stop": 0.3}, nil, nil)
	start := time.Now()
	_, out = rpc(t, p, h.Agent, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"update_contact","arguments":{"id":"c2"}}}`,
		map[string]string{"Mcp-Session-Id": "sess-9"})
	result = out["result"].(map[string]any)
	text = result["content"].([]any)[0].(map[string]any)["text"].(string)
	if result["isError"] != true || !strings.Contains(text, "holding this call") || time.Since(start) < 2*time.Second {
		t.Fatalf("held response %v after %v", out, time.Since(start))
	}
	if len(up.calls) != 0 {
		t.Fatal("blocked calls reached the upstream")
	}

	// A reviewer approves; the agent retries the same call in the same
	// session and it goes through without a second review.
	pending, _, _ := h.Store.ListActions(context.Background(), h.Owner.WorkspaceID, store.ActionFilter{Statuses: []string{"pending_review"}})
	if _, err := h.App.Review(context.Background(), h.Owner, pending[0].ID, app.ReviewRequest{Option: domain.OptionProceed}, app.ChannelConsole); err != nil {
		t.Fatal(err)
	}
	_, out = rpc(t, p, h.Agent, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"update_contact","arguments":{"id":"c2"}}}`,
		map[string]string{"Mcp-Session-Id": "sess-9"})
	if out["result"].(map[string]any)["isError"] == true || len(up.calls) != 1 {
		t.Fatalf("retry after approval %v calls %d", out, len(up.calls))
	}
}

func TestSSEOutcomeAndBatches(t *testing.T) {
	h, p, _ := setup(t)
	rec, _ := rpc(t, p, h.Agent, `{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"stream_report","arguments":{}}}`, nil)
	if !bytes.Contains(rec.Body.Bytes(), []byte("notifications/progress")) {
		t.Fatalf("stream not passed through: %s", rec.Body)
	}
	actions, _, _ := h.Store.ListActions(context.Background(), h.Owner.WorkspaceID, store.ActionFilter{})
	if *actions[0].Outcome != "failed" {
		t.Fatalf("SSE isError result not recorded as failed: %v", *actions[0].Outcome)
	}
	_, out := rpc(t, p, h.Agent, `[{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"lookup"}}]`, nil)
	if out["error"] == nil {
		t.Fatalf("batch with tools/call accepted: %v", out)
	}
}
