package notify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWebhookSignatureRoundTrip(t *testing.T) {
	secret := []byte("whsec_test")
	body := []byte(`{"event":"action.decided"}`)
	now := time.Unix(1_800_000_000, 0)
	h := Sign(secret, now, body)
	if err := Verify(secret, h, body, now.Add(time.Minute), 5*time.Minute); err != nil {
		t.Fatal(err)
	}
	if Verify(secret, h, []byte(`{"event":"x"}`), now, 5*time.Minute) == nil {
		t.Error("accepted altered body")
	}
	if Verify(secret, h, body, now.Add(10*time.Minute), 5*time.Minute) == nil {
		t.Error("accepted stale signature")
	}
	if Verify([]byte("other"), h, body, now, 5*time.Minute) == nil {
		t.Error("accepted wrong secret")
	}
}

func TestSenderSendsSignedRequest(t *testing.T) {
	var got http.Header
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	s := NewSender(srv.Client())
	status, err := s.Send(context.Background(), srv.URL, "dlv_1", EventActionDecided, []byte("k"), []byte(`{"a":1}`))
	if err != nil || status != 204 {
		t.Fatalf("status %d err %v", status, err)
	}
	if got.Get("Veto-Event") != EventActionDecided || got.Get("Veto-Delivery") != "dlv_1" {
		t.Errorf("headers %v", got)
	}
	if err := Verify([]byte("k"), got.Get(SignatureHeader), gotBody, time.Now(), time.Minute); err != nil {
		t.Errorf("receiver could not verify: %v", err)
	}
}

func TestRetryDelayGrows(t *testing.T) {
	prev := time.Duration(0)
	for i := 1; i <= MaxAttempts; i++ {
		d := RetryDelay(i)
		if d < prev {
			t.Fatalf("delay shrank at attempt %d", i)
		}
		prev = d
	}
}

func TestSlackPostAndInteraction(t *testing.T) {
	var posted map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer xoxb-1" {
			io.WriteString(w, `{"ok":false,"error":"invalid_auth"}`)
			return
		}
		switch r.URL.Path {
		case "/chat.postMessage":
			_ = json.NewDecoder(r.Body).Decode(&posted)
			io.WriteString(w, `{"ok":true,"ts":"1700000000.000100"}`)
		case "/users.info":
			io.WriteString(w, `{"ok":true,"user":{"profile":{"email":"rev@acme.com"}}}`)
		default:
			io.WriteString(w, `{"ok":false,"error":"unknown_method"}`)
		}
	}))
	defer srv.Close()
	s := NewSlack(srv.Client(), srv.URL)
	ts, err := s.PostHeld(context.Background(), "xoxb-1", "C123", HeldMessage{
		ActionID: "act_1", AgentName: "billing-agent", Tool: "gmail.send", Request: "Send <invoice> to Acme",
		Options: []SlackOption{{Key: "proceed_safer", Label: "Save as draft", Recommended: true, Changes: []string{"Runs gmail.create_draft"}},
			{Key: "proceed", Label: "Send"}, {Key: "stop", Label: "Stop"}},
		ReviewURL: "https://veto.example.com/actions/act_1",
	})
	if err != nil || ts != "1700000000.000100" {
		t.Fatalf("ts %q err %v", ts, err)
	}
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(posted["blocks"])
	raw := buf.String()
	for _, want := range []string{`"action_id":"veto_proceed_safer"`, `"style":"primary"`, `"style":"danger"`, `Send &lt;invoice&gt;`} {
		if !strings.Contains(raw, want) {
			t.Errorf("blocks missing %s: %s", want, raw)
		}
	}
	if _, err := s.PostHeld(context.Background(), "bad", "C1", HeldMessage{}); err == nil || !strings.Contains(err.Error(), "invalid_auth") {
		t.Errorf("bad token: %v", err)
	}
	email, err := s.UserEmail(context.Background(), "xoxb-1", "U1")
	if err != nil || email != "rev@acme.com" {
		t.Errorf("email %q %v", email, err)
	}

	payload := `{"type":"block_actions","user":{"id":"U1"},"team":{"id":"T1"},"channel":{"id":"C123"},
		"container":{"message_ts":"1700000000.000100"},"response_url":"https://hooks.slack.com/x",
		"actions":[{"action_id":"veto_proceed_safer","value":"act_1"}]}`
	in, ok, err := ParseInteraction(url.Values{"payload": {payload}})
	if err != nil || !ok || in.Option != "proceed_safer" || in.ActionID != "act_1" || in.MessageTS == "" {
		t.Fatalf("interaction %+v ok %v err %v", in, ok, err)
	}
	_, ok, _ = ParseInteraction(url.Values{"payload": {strings.Replace(payload, "veto_proceed_safer", "veto_open", 1)}})
	if ok {
		t.Error("open link parsed as a decision")
	}
}

func TestVerifySlackRequest(t *testing.T) {
	secret := "8f742231b10e8888abcd99yyyzzz85a5"
	body := []byte("payload=%7B%7D")
	now := time.Now()
	ts := strconv.FormatInt(now.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + ts + ":"))
	mac.Write(body)
	h := http.Header{}
	h.Set("X-Slack-Request-Timestamp", ts)
	h.Set("X-Slack-Signature", "v0="+hex.EncodeToString(mac.Sum(nil)))
	if err := VerifySlackRequest(secret, h, body, now); err != nil {
		t.Fatal(err)
	}
	if VerifySlackRequest(secret, h, []byte("payload=tampered"), now) == nil {
		t.Error("accepted tampered body")
	}
	if VerifySlackRequest(secret, h, body, now.Add(10*time.Minute)) == nil {
		t.Error("accepted replayed request")
	}
}
