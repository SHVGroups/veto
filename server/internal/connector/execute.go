package connector

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// ToolPrefix is the tool name prefix for calls made through a connection:
// a call to "http.stripe" goes through the connection named "stripe".
const ToolPrefix = "http."

// MaxResponseBytes caps how much of a response body is returned and stored.
const MaxResponseBytes = 1 << 20

// Request is the argument shape of a connection tool call.
type Request struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Query   map[string]string `json:"query,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    json.RawMessage   `json:"body,omitempty"`
}

// Response is what the connection returned.
type Response struct {
	Status    int               `json:"status"`
	Headers   map[string]string `json:"headers"`
	Body      json.RawMessage   `json:"body,omitempty"`
	BodyText  string            `json:"body_text,omitempty"`
	Truncated bool              `json:"truncated,omitempty"`
	LatencyMs int64             `json:"latency_ms"`
}

// Connection is the subset of a stored connection the executor needs.
type Connection struct {
	Name         string
	BaseURL      string
	AuthScheme   string
	HeaderName   string
	Secret       []byte
	AllowMethods []string
}

// Errors returned for requests the connection does not permit.
var (
	ErrMethodNotAllowed = errors.New("connector: method is not allowed on this connection")
	ErrBadPath          = errors.New("connector: path must be relative to the connection's base URL")
)

// headers agents may not set because they carry credentials or routing.
var reservedHeaders = []string{"authorization", "cookie", "host", "proxy-authorization", "x-forwarded-for", "x-forwarded-host"}

// ParseRequest decodes and validates connection call arguments.
func ParseRequest(raw json.RawMessage) (Request, error) {
	var r Request
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return r, fmt.Errorf("connector: invalid request arguments: %w", err)
	}
	r.Method = strings.ToUpper(strings.TrimSpace(r.Method))
	if r.Method == "" {
		r.Method = http.MethodGet
	}
	if !strings.HasPrefix(r.Path, "/") || strings.HasPrefix(r.Path, "//") || strings.Contains(r.Path, "..") || strings.Contains(r.Path, "://") || strings.ContainsAny(r.Path, "?#") {
		return r, ErrBadPath
	}
	for h := range r.Headers {
		if slices.Contains(reservedHeaders, strings.ToLower(h)) {
			return r, fmt.Errorf("connector: header %q cannot be set by an agent", h)
		}
	}
	return r, nil
}

// Executor performs connection calls.
type Executor struct {
	http *http.Client
}

// NewExecutor returns an executor using the given client.
func NewExecutor(c *http.Client) *Executor { return &Executor{http: c} }

// Do sends a request through a connection, attaching its credential.
func (e *Executor) Do(ctx context.Context, conn Connection, r Request) (*Response, error) {
	if !slices.Contains(conn.AllowMethods, r.Method) {
		return nil, ErrMethodNotAllowed
	}
	target, err := buildURL(conn.BaseURL, r.Path, r.Query)
	if err != nil {
		return nil, err
	}
	var body io.Reader
	if len(r.Body) > 0 && string(r.Body) != "null" {
		body = bytes.NewReader(r.Body)
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, target, body)
	if err != nil {
		return nil, err
	}
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "veto-connector/1")
	if err := authorize(req, conn); err != nil {
		return nil, err
	}

	start := time.Now()
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connector: %s: %w", conn.Name, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("connector: %s: read response: %w", conn.Name, err)
	}
	out := &Response{Status: resp.StatusCode, Headers: map[string]string{}, LatencyMs: time.Since(start).Milliseconds()}
	if len(raw) > MaxResponseBytes {
		raw, out.Truncated = raw[:MaxResponseBytes], true
	}
	for _, h := range []string{"Content-Type", "Location", "Retry-After", "Request-Id", "X-Request-Id"} {
		if v := resp.Header.Get(h); v != "" {
			out.Headers[h] = v
		}
	}
	if json.Valid(raw) && !out.Truncated {
		out.Body = raw
	} else if utf8.Valid(raw) {
		out.BodyText = string(raw)
	} else {
		out.BodyText = base64.StdEncoding.EncodeToString(raw)
		out.Headers["X-Veto-Body-Encoding"] = "base64"
	}
	return out, nil
}

func buildURL(base, path string, query map[string]string) (string, error) {
	b, err := url.Parse(base)
	if err != nil || b.Host == "" {
		return "", fmt.Errorf("connector: invalid base URL")
	}
	u := *b
	u.Path = strings.TrimRight(b.Path, "/") + path
	u.RawPath = ""
	q := u.Query()
	for k, v := range query {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	if u.Host != b.Host || u.Scheme != b.Scheme {
		return "", ErrBadPath
	}
	return u.String(), nil
}

func authorize(req *http.Request, c Connection) error {
	secret := string(c.Secret)
	switch c.AuthScheme {
	case "none":
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+secret)
	case "header":
		if c.HeaderName == "" {
			return errors.New("connector: header auth needs a header name")
		}
		req.Header.Set(c.HeaderName, secret)
	case "basic":
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(secret)))
	default:
		return fmt.Errorf("connector: unknown auth scheme %q", c.AuthScheme)
	}
	return nil
}

// ValidateBaseURL checks a connection's base URL when it is saved.
func ValidateBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("base URL must be an absolute URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("base URL must use http or https")
	}
	if u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return errors.New("base URL must not contain a query, fragment or credentials")
	}
	return nil
}

// Authorize sets a connection's credential on an outgoing request. The MCP
// proxy uses it for upstream servers bound to a connection.
func Authorize(req *http.Request, c Connection) error { return authorize(req, c) }
