package httpapi_test

import (
	"bufio"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shvgroups/veto/server/internal/apptest"
	"github.com/shvgroups/veto/server/internal/config"
	"github.com/shvgroups/veto/server/internal/httpapi"
	"github.com/shvgroups/veto/server/internal/mcp"
	"github.com/shvgroups/veto/server/internal/metrics"
)

type client struct {
	t    *testing.T
	base string
}

func newServer(t *testing.T) (*apptest.Harness, *client) {
	t.Helper()
	h := apptest.New(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{PublicURL: "https://veto.example.com", RateLimitRPS: 100, RateLimitBurst: 100, MaxWait: 5 * time.Second}
	proxy := mcp.New(h.App, http.DefaultClient, time.Second, log)
	srv := httptest.NewServer(httpapi.New(h.App, cfg, log, proxy, metrics.NewRegistry()).Handler())
	t.Cleanup(srv.Close)
	return h, &client{t: t, base: srv.URL}
}

// do sends a request and decodes a JSON response into out when out is set.
func (c *client) do(method, path, body string, header map[string]string, out any) int {
	c.t.Helper()
	req, err := http.NewRequest(method, c.base+path, strings.NewReader(body))
	if err != nil {
		c.t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			c.t.Fatalf("%s %s: decode %q: %v", method, path, raw, err)
		}
	}
	return resp.StatusCode
}

func session(h *apptest.Harness, extra map[string]string) map[string]string {
	m := map[string]string{"Cookie": httpapi.SessionCookie + "=" + h.OwnerToken}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestHealthAndUnknownRoutes(t *testing.T) {
	_, c := newServer(t)
	var health map[string]string
	if code := c.do("GET", "/healthz", "", nil, &health); code != 200 || health["status"] != "ok" {
		t.Fatalf("healthz %d %v", code, health)
	}
	var e struct {
		Error struct{ Code string } `json:"error"`
	}
	if code := c.do("GET", "/v1/nope", "", nil, &e); code != 404 || e.Error.Code != "not_found" {
		t.Fatalf("unknown route %d %+v", code, e)
	}
}

func TestAuthenticationLevels(t *testing.T) {
	h, c := newServer(t)
	agent := map[string]string{"Authorization": "Bearer " + h.AgentKey}

	if code := c.do("GET", "/v1/overview", "", nil, nil); code != 401 {
		t.Fatalf("anonymous overview: %d", code)
	}
	if code := c.do("GET", "/v1/overview", "", agent, nil); code != 403 {
		t.Fatalf("agent key on console route: %d", code)
	}
	if code := c.do("GET", "/v1/overview?range=24h", "", session(h, nil), nil); code != 200 {
		t.Fatalf("session overview: %d", code)
	}
	if code := c.do("POST", "/v1/actions", `{"tool":"crm.lookup","args":{}}`, session(h, nil), nil); code != 401 {
		t.Fatalf("session on agent route: %d", code)
	}
	if code := c.do("GET", "/v1/auth/me", "", agent, nil); code != 403 {
		t.Fatalf("api key on session-only route: %d", code)
	}
}

func TestCSRFAndOrigin(t *testing.T) {
	h, c := newServer(t)
	body := `{"name":"Priya"}`
	if code := c.do("PATCH", "/v1/auth/me", body, session(h, nil), nil); code != 403 {
		t.Fatalf("missing CSRF header: %d", code)
	}
	csrf := map[string]string{httpapi.CSRFHeader: "1"}
	if code := c.do("PATCH", "/v1/auth/me", body, session(h, map[string]string{httpapi.CSRFHeader: "1", "Origin": "https://evil.example"}), nil); code != 403 {
		t.Fatalf("foreign origin: %d", code)
	}
	if code := c.do("PATCH", "/v1/auth/me", body, session(h, map[string]string{httpapi.CSRFHeader: "1", "Origin": "https://veto.example.com"}), nil); code != 200 {
		t.Fatalf("public url origin: %d", code)
	}
	if code := c.do("PATCH", "/v1/auth/me", body, session(h, csrf), nil); code != 200 {
		t.Fatalf("same-site request without origin: %d", code)
	}
}

func TestSubmitReviewAndDetail(t *testing.T) {
	h, c := newServer(t)
	agent := map[string]string{"Authorization": "Bearer " + h.AgentKey}
	h.Model.Set(map[string]float64{"proceed": 0.5, "proceed_safer": 0.3, "stop": 0.2}, map[string]float64{"matches_request": 0.9}, nil)

	var res struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	code := c.do("POST", "/v1/actions", `{"tool":"gmail.send","args":{"to":["ops@acme.com"]},"request":"send the report"}`, agent, &res)
	if code != 202 || res.Status != "pending_review" {
		t.Fatalf("submit %d %+v", code, res)
	}
	var list struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if code := c.do("GET", "/v1/actions?held=true&status=pending_review", "", session(h, nil), &list); code != 200 || len(list.Items) != 1 || list.Items[0].ID != res.ID {
		t.Fatalf("held list %d %+v", code, list)
	}
	var detail struct {
		Action   struct{ Status string } `json:"action"`
		Decision struct{ Option string } `json:"decision"`
	}
	code = c.do("POST", "/v1/actions/"+res.ID+"/review", `{"option":"proceed_safer","note":"draft it"}`, session(h, map[string]string{httpapi.CSRFHeader: "1"}), &detail)
	if code != 200 || detail.Action.Status != "approved" || detail.Decision.Option != "proceed_safer" {
		t.Fatalf("review %d %+v", code, detail)
	}
	var result struct {
		Status string `json:"status"`
		Grant  *struct {
			Token string `json:"token"`
		} `json:"grant"`
	}
	if code := c.do("GET", "/v1/actions/"+res.ID, "", agent, &result); code != 200 || result.Grant == nil || result.Status != "approved" {
		t.Fatalf("result %d %+v", code, result)
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	h, c := newServer(t)
	agent := map[string]string{"Authorization": "Bearer " + h.AgentKey}
	var e struct {
		Error struct{ Code, Message string } `json:"error"`
	}
	if code := c.do("POST", "/v1/actions", `{"tool":"crm.lookup","args":{},"bogus":1}`, agent, &e); code != 400 || e.Error.Code != "invalid_request" {
		t.Fatalf("unknown field %d %+v", code, e)
	}
}

func TestEventStream(t *testing.T) {
	h, c := newServer(t)
	req, _ := http.NewRequest("GET", c.base+"/v1/events", nil)
	req.Header.Set("Cookie", httpapi.SessionCookie+"="+h.OwnerToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	lines := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	// Wait for the stream to be subscribed before causing an event.
	for l := range lines {
		if l == ": connected" {
			break
		}
	}
	agent := map[string]string{"Authorization": "Bearer " + h.AgentKey}
	if code := c.do("POST", "/v1/actions", `{"tool":"crm.lookup","args":{"id":1}}`, agent, nil); code != 200 {
		t.Fatalf("submit %d", code)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatal("stream closed")
			}
			if l == "event: action.received" {
				return
			}
		case <-deadline:
			t.Fatal("no action.received event within 5s")
		}
	}
}
