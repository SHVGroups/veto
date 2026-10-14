// Package apptest builds a fully wired App on a real database for tests,
// with a scripted decision model whose answers each test controls.
package apptest

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/shvgroups/veto/server/internal/app"
	"github.com/shvgroups/veto/server/internal/connector"
	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/events"
	"github.com/shvgroups/veto/server/internal/jev"
	"github.com/shvgroups/veto/server/internal/store"
	"github.com/shvgroups/veto/server/internal/store/storetest"
	"github.com/shvgroups/veto/server/internal/vault"
)

// Model answers the disposition question with fixed probabilities
// and every signal with a safe value, unless told to fail.
type Model struct {
	mu    sync.Mutex
	probs map[string]float64
	nouls map[string]float64
	err   error
	calls int
}

// Calls returns how many requests the model has answered.
func (m *Model) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// Set scripts the disposition probabilities, noul answers and error.
func (m *Model) Set(probs map[string]float64, nouls map[string]float64, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.probs, m.nouls, m.err = probs, nouls, err
}

// Evaluate implements decide.Model.
func (m *Model) Evaluate(_ context.Context, req jev.Request) (*jev.Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	resp := &jev.Response{Model: "jev-test", Answers: map[string]jev.Answer{}}
	for qid, q := range req.Questions {
		switch q.Type {
		case jev.TypeChoice:
			keys := q.Criteria.(jev.ChoiceCriteria).Keys()
			probs := map[string]float64{}
			best, bestP := keys[0], -1.0
			for _, k := range keys {
				p := 1.0 / float64(len(keys))
				if qid == "disposition" {
					p = m.probs[k]
				} else if k == keys[0] {
					p = 1
				} else {
					p = 0
				}
				probs[k] = p
				if p > bestP {
					best, bestP = k, p
				}
			}
			n := float64(len(keys))
			conf := (bestP - 1/n) / (1 - 1/n)
			resp.Answers[qid] = jev.Answer{Type: jev.TypeChoice, Choice: best, Probabilities: probs, Confidence: &conf}
		case jev.TypeScore:
			zero, one := 0.0, 1.0
			resp.Answers[qid] = jev.Answer{Type: jev.TypeScore, Score: &zero, Confidence: &one}
		case jev.TypeNoul:
			v := m.nouls[qid]
			resp.Answers[qid] = jev.Answer{Type: jev.TypeNoul, Noul: &v}
		}
	}
	body, _ := json.Marshal(req)
	raw, _ := json.Marshal(resp)
	resp.Usage.InputTokens = 321
	return &jev.Result{Response: resp, Endpoint: "scripted", Model: "jev-test", RequestBody: body, RawResponse: raw}, nil
}

// Harness is a running App with a workspace, an owner and an agent.
type Harness struct {
	App        *app.App
	Store      *store.Store
	Model      *Model
	Owner      app.Actor
	Agent      app.Actor
	OwnerToken string
	AgentKey   string
	Password   string
}

// New builds a Harness on a fresh database. Tests using it are skipped
// without a test database.
func New(t testing.TB) *Harness {
	t.Helper()
	s := storetest.New(t)
	// Cleanups run last-registered first: stop the event listener before the
	// store closes, or closing waits forever for its connection.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	key, _ := vault.ParseKey(vault.GenerateKey())
	kr, _ := vault.NewKeyring(key)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := events.NewBus(s.Pool(), log)
	go bus.Run(ctx)
	model := &Model{nouls: map[string]float64{"matches_request": 0.97}}
	a, err := app.New(app.Deps{
		Store: s, Bus: bus, Keyring: kr, Model: model, ModelName: "jev-test", Log: log,
		PublicURL: "https://veto.example.com", DecisionTimeout: 2 * time.Second, MaxWait: 10 * time.Second,
		Connector:        connector.NewExecutor(connector.Client(false, 5*time.Second)),
		PrivateConnector: connector.NewExecutor(connector.Client(true, 5*time.Second)),
	})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := a.Setup(ctx, app.SetupRequest{WorkspaceName: "Acme", Name: "Priya Owner", Email: "priya@acme.com", Password: "correct horse battery"}, "", "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := a.Authenticate(ctx, sess.Token)
	if err != nil {
		t.Fatal(err)
	}
	_, key1, err := a.CreateAgent(ctx, owner, app.AgentInput{Name: "billing-agent", Vendor: "anthropic"})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := a.AuthenticateKey(ctx, key1.Secret)
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []app.ToolInput{
		{Name: "gmail.send", RiskClass: domain.RiskMedium, Pack: "communication",
			Safer: json.RawMessage(`[{"op":"swap_tool","tool":"gmail.create_draft"},{"op":"filter_domains","path":"to","allow":["acme.com"]}]`)},
		{Name: "crm.lookup", RiskClass: domain.RiskRead},
		{Name: "stripe.refund", RiskClass: domain.RiskHigh, Pack: "payment", StepUp: true,
			Safer: json.RawMessage(`[{"op":"cap_number","path":"amount_cents","max":5000}]`)},
	} {
		if _, err := a.SaveTool(ctx, owner, in); err != nil {
			t.Fatal(err)
		}
	}
	return &Harness{App: a, Store: s, Model: model, Owner: owner, Agent: agent, OwnerToken: sess.Token, AgentKey: key1.Secret, Password: "correct horse battery"}
}
