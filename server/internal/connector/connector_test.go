package connector

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDoAttachesCredentialAndBuildsURL(t *testing.T) {
	var got *http.Request
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "secret=1")
		io.WriteString(w, `{"id":"re_123"}`)
	}))
	defer srv.Close()

	e := NewExecutor(Client(true, 5*time.Second))
	req, err := ParseRequest(json.RawMessage(`{"method":"post","path":"/v1/refunds","query":{"expand":"charge"},"body":{"amount":500}}`))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := e.Do(context.Background(), Connection{
		Name: "stripe", BaseURL: srv.URL + "/api", AuthScheme: "bearer", Secret: []byte("sk_test"),
		AllowMethods: []string{"GET", "POST"},
	}, req)
	if err != nil {
		t.Fatal(err)
	}
	if got.URL.Path != "/api/v1/refunds" || got.URL.Query().Get("expand") != "charge" {
		t.Errorf("url %s", got.URL)
	}
	if got.Header.Get("Authorization") != "Bearer sk_test" || gotBody != `{"amount":500}` {
		t.Errorf("auth %q body %q", got.Header.Get("Authorization"), gotBody)
	}
	if resp.Status != 200 || string(resp.Body) != `{"id":"re_123"}` {
		t.Errorf("resp %+v", resp)
	}
	if _, leaked := resp.Headers["Set-Cookie"]; leaked {
		t.Error("response headers leak cookies")
	}
}

func TestParseRequestRejectsEscapes(t *testing.T) {
	for _, raw := range []string{
		`{"path":"relative"}`,
		`{"path":"/a/../../admin"}`,
		`{"path":"//evil.example.com/x"}`,
		`{"path":"/x?y=1"}`,
		`{"path":"/x","headers":{"Authorization":"Bearer stolen"}}`,
		`{"path":"/x","extra":1}`,
	} {
		if _, err := ParseRequest(json.RawMessage(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}

func TestDoRejectsMethod(t *testing.T) {
	e := NewExecutor(Client(true, time.Second))
	req, _ := ParseRequest(json.RawMessage(`{"method":"DELETE","path":"/x"}`))
	_, err := e.Do(context.Background(), Connection{BaseURL: "https://api.example.com", AuthScheme: "none", AllowMethods: []string{"GET"}}, req)
	if !errors.Is(err, ErrMethodNotAllowed) {
		t.Fatalf("got %v", err)
	}
}

func TestGuardBlocksPrivateAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	_, err := Client(false, time.Second).Get(srv.URL)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("loopback request was not blocked: %v", err)
	}
	for ip, want := range map[string]bool{
		"10.1.2.3": true, "169.254.169.254": true, "::1": true, "fd00::1": true, "::ffff:127.0.0.1": true,
		"8.8.8.8": false, "2606:4700::1111": false,
	} {
		if got := Blocked(net.ParseIP(ip)); got != want {
			t.Errorf("Blocked(%s) = %v, want %v", ip, got, want)
		}
	}
}
