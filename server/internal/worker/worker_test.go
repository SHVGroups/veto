package worker

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/shvgroups/veto/server/internal/app"
	"github.com/shvgroups/veto/server/internal/connector"
	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/events"
	"github.com/shvgroups/veto/server/internal/notify"
	"github.com/shvgroups/veto/server/internal/store/storetest"
	"github.com/shvgroups/veto/server/internal/vault"
)

func TestWebhookDeliveryAndRetry(t *testing.T) {
	s := storetest.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	key, _ := vault.ParseKey(vault.GenerateKey())
	kr, _ := vault.NewKeyring(key)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := events.NewBus(s.Pool(), log)
	go bus.Run(ctx)
	a, err := app.New(app.Deps{Store: s, Bus: bus, Keyring: kr, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := a.Setup(ctx, app.SetupRequest{WorkspaceName: "Acme", Name: "Owner", Email: "o@acme.com", Password: "a long enough password"}, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	owner, _ := a.Authenticate(ctx, sess.Token)
	_, k, err := a.CreateAgent(ctx, owner, app.AgentInput{Name: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	agent, _ := a.AuthenticateKey(ctx, k.Secret)

	var mu sync.Mutex
	var bodies [][]byte
	var sigs []string
	fail := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		b, _ := io.ReadAll(r.Body)
		if fail {
			fail = false
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		bodies = append(bodies, b)
		sigs = append(sigs, r.Header.Get(notify.SignatureHeader))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	hook, err := a.CreateWebhook(ctx, owner, app.WebhookInput{URL: srv.URL, Events: []string{notify.EventActionPending}})
	if err != nil {
		t.Fatal(err)
	}

	// With no model configured, a medium tool's call is held for review.
	if _, err := a.SaveTool(ctx, owner, app.ToolInput{Name: "gmail.send", RiskClass: domain.RiskMedium}); err != nil {
		t.Fatal(err)
	}
	res, err := a.Submit(ctx, agent, domain.SourceAPI, app.SubmitRequest{Tool: "gmail.send", Args: json.RawMessage(`{"to":"a@b.c"}`)}, 0)
	if err != nil || res.Status != domain.StatusPendingReview {
		t.Fatalf("submit %+v %v", res, err)
	}

	w := New(a, s, notify.NewSender(connector.Client(true, 5*time.Second)), log, DefaultIntervals(), nil)
	if err := w.deliver(ctx); err != nil {
		t.Fatal(err)
	}
	deliveries, _ := s.ListDeliveries(ctx, owner.WorkspaceID, hook.ID, 10)
	if len(deliveries) != 1 || deliveries[0].Status != "pending" || deliveries[0].Attempts != 1 || *deliveries[0].ResponseStatus != 502 {
		t.Fatalf("after failure: %+v", deliveries)
	}
	if !deliveries[0].NextAttemptAt.After(time.Now().Add(20 * time.Second)) {
		t.Fatalf("retry not scheduled with backoff: %v", deliveries[0].NextAttemptAt)
	}
	if _, err := s.Pool().Exec(ctx, `UPDATE webhook_deliveries SET next_attempt_at = now()`); err != nil {
		t.Fatal(err)
	}
	if err := w.deliver(ctx); err != nil {
		t.Fatal(err)
	}
	deliveries, _ = s.ListDeliveries(ctx, owner.WorkspaceID, hook.ID, 10)
	if deliveries[0].Status != "delivered" || deliveries[0].Attempts != 2 {
		t.Fatalf("after retry: %+v", deliveries[0])
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("received %d bodies", len(bodies))
	}
	if err := notify.Verify([]byte(hook.Secret), sigs[0], bodies[0], time.Now(), time.Minute); err != nil {
		t.Fatalf("signature: %v", err)
	}
	var payload map[string]any
	_ = json.Unmarshal(bodies[0], &payload)
	data := payload["data"].(map[string]any)
	if payload["event"] != notify.EventActionPending || data["id"] != res.ID {
		t.Fatalf("payload %v", payload)
	}

	// Expiry runs through the worker too.
	if _, err := s.Pool().Exec(ctx, `UPDATE actions SET review_deadline = now() - interval '1 second' WHERE id = $1`, res.ID); err != nil {
		t.Fatal(err)
	}
	if err := w.expire(ctx); err != nil {
		t.Fatal(err)
	}
	act, _ := s.GetAction(ctx, owner.WorkspaceID, res.ID)
	if act.Status != domain.StatusExpired {
		t.Fatalf("status %s", act.Status)
	}
	if err := w.checkpoint(ctx); err != nil {
		t.Fatal(err)
	}
	if err := w.retention(ctx); err != nil {
		t.Fatal(err)
	}
}
