package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/id"
	"github.com/shvgroups/veto/server/internal/ledger"
	"github.com/shvgroups/veto/server/internal/store"
	"github.com/shvgroups/veto/server/internal/store/storetest"
)

func seed(t *testing.T, s *store.Store) (ws, agent string) {
	t.Helper()
	ctx := context.Background()
	ws = id.New(id.Workspace)
	if err := s.CreateWorkspace(ctx, ws, "Acme", domain.DefaultSettings()); err != nil {
		t.Fatal(err)
	}
	agent = id.New(id.Agent)
	if err := s.CreateAgent(ctx, store.Agent{ID: agent, WorkspaceID: ws, Name: "billing-agent"}); err != nil {
		t.Fatal(err)
	}
	return ws, agent
}

func insertAction(t *testing.T, s *store.Store, ws, agent, tool string) *store.Action {
	t.Helper()
	a := &store.Action{
		ID: id.New(id.Action), WorkspaceID: ws, AgentID: agent, Tool: tool,
		Args: json.RawMessage(`{"to":"a@acme.com"}`), ArgsHash: []byte{1, 2, 3},
		Request: "send the invoice", Source: domain.SourceAPI, Status: domain.StatusEvaluating,
	}
	if err := s.InsertAction(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestMigrateIsIdempotent(t *testing.T) {
	s := storetest.New(t)
	ran, err := s.Migrate(context.Background())
	if err != nil || len(ran) != 0 {
		t.Fatalf("second migrate ran %v err %v", ran, err)
	}
	v, err := s.SchemaVersion(context.Background())
	if err != nil || v != "0001_initial" {
		t.Fatalf("version %q %v", v, err)
	}
}

func TestLedgerChainVerifiesFromDatabase(t *testing.T) {
	s := storetest.New(t)
	ctx := context.Background()
	ws, _ := seed(t, s)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := s.Tx(ctx, func(q *store.Queries) error {
				_, err := q.AppendLedger(ctx, ws, ledger.KindActionDecided, "act_x", "model",
					map[string]any{"i": i, "confidence": 0.875, "nested": map[string]any{"b": 1, "a": []int{1, 2}}})
				return err
			})
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()

	seq, head, err := s.LedgerHead(ctx, ws)
	if err != nil || seq != 20 {
		t.Fatalf("head seq %d err %v", seq, err)
	}
	entries, err := s.LedgerRange(ctx, ws, 1, seq)
	if err != nil {
		t.Fatal(err)
	}
	last, brk := ledger.Verify(entries, 1, ledger.Genesis(ws))
	if brk != nil {
		t.Fatalf("chain break %+v", brk)
	}
	if string(last) != string(head) {
		t.Fatal("head hash differs from recomputed chain")
	}

	if _, err := s.Pool().Exec(ctx, `UPDATE ledger_entries SET payload = '{"i": 99}' WHERE workspace_id = $1 AND seq = 7`, ws); err != nil {
		t.Fatal(err)
	}
	entries, _ = s.LedgerRange(ctx, ws, 1, seq)
	if _, brk := ledger.Verify(entries, 1, ledger.Genesis(ws)); brk == nil || brk.Seq != 7 {
		t.Fatalf("tampering not detected at 7: %+v", brk)
	}
}

func TestActionLifecycle(t *testing.T) {
	s := storetest.New(t)
	ctx := context.Background()
	ws, agent := seed(t, s)

	key := "idem-1"
	a := &store.Action{
		ID: id.New(id.Action), WorkspaceID: ws, AgentID: agent, Tool: "gmail.send",
		Args: json.RawMessage(`{"to":"a@acme.com"}`), ArgsHash: []byte{1}, IdempotencyKey: &key,
		Source: domain.SourceAPI, Status: domain.StatusEvaluating,
		Untrusted: []domain.Untrusted{{Source: "email", Content: "hi"}},
	}
	if err := s.InsertAction(ctx, a); err != nil {
		t.Fatal(err)
	}
	dup := *a
	dup.ID = id.New(id.Action)
	if err := s.InsertAction(ctx, &dup); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate idempotency key: %v", err)
	}
	got, err := s.GetActionByIdempotencyKey(ctx, ws, agent, key)
	if err != nil || got.ID != a.ID || got.AgentName != "billing-agent" || len(got.Untrusted) != 1 {
		t.Fatalf("got %+v %v", got, err)
	}

	deadline := time.Now().Add(time.Hour)
	if err := s.RecordEvaluation(ctx, ws, a.ID, store.EvaluatedAction{
		Status:         domain.StatusPendingReview,
		Options:        []domain.OptionSpec{{Key: domain.OptionProceed, Available: true}, {Key: domain.OptionProceedSafer, Available: true, Args: json.RawMessage(`{"x":1}`)}},
		Ranking:        []domain.Option{domain.OptionProceedSafer, domain.OptionProceed, domain.OptionStop},
		HoldReasons:    []domain.Reason{{Code: domain.ReasonLowConfidence}},
		ReviewDeadline: &deadline,
	}); err != nil {
		t.Fatal(err)
	}
	pending, _, err := s.ListActions(ctx, ws, store.ActionFilter{Statuses: []string{"pending_review"}})
	if err != nil || len(pending) != 1 || pending[0].HoldReasons[0].Code != domain.ReasonLowConfidence {
		t.Fatalf("pending %+v %v", pending, err)
	}

	if _, err := s.FinalizeAction(ctx, ws, a.ID, domain.StatusApproved); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinalizeAction(ctx, ws, a.ID, domain.StatusDenied); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("second finalize: %v", err)
	}
	conf := 0.91
	if err := s.InsertDecision(ctx, &store.Decision{
		ActionID: a.ID, WorkspaceID: ws, Option: domain.OptionProceedSafer, Outcome: "approved",
		DecidedBy: domain.DecidedByModel, FinalTool: "gmail.create_draft", Confidence: &conf,
		Reasons: []domain.Reason{{Code: domain.ReasonConfident}}, Thresholds: domain.DefaultThresholds(domain.RiskMedium),
	}); err != nil {
		t.Fatal(err)
	}
	d, err := s.GetDecision(ctx, ws, a.ID)
	if err != nil || *d.Confidence != 0.91 || d.Thresholds.AutoConfidence != 0.75 {
		t.Fatalf("decision %+v %v", d, err)
	}

	n, err := s.RedactBefore(ctx, ws, time.Now().Add(time.Minute), 100)
	if err != nil || n != 1 {
		t.Fatalf("redacted %d %v", n, err)
	}
	red, _ := s.GetAction(ctx, ws, a.ID)
	if string(red.Args) != `{"redacted": true}` || len(red.Untrusted) != 0 || red.Options[1].Args != nil || red.Options[1].Key != domain.OptionProceedSafer {
		t.Fatalf("redacted action %+v", red)
	}
}

func TestListActionsPaginatesAndFilters(t *testing.T) {
	s := storetest.New(t)
	ctx := context.Background()
	ws, agent := seed(t, s)
	for i := 0; i < 7; i++ {
		tool := "crm.lookup"
		if i%2 == 0 {
			tool = "gmail.send"
		}
		insertAction(t, s, ws, agent, tool)
	}
	var all []store.ActionSummary
	cursor := ""
	for {
		page, next, err := s.ListActions(ctx, ws, store.ActionFilter{Limit: 3, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, page...)
		if next == "" {
			break
		}
		cursor = next
	}
	if len(all) != 7 {
		t.Fatalf("paged %d actions, want 7", len(all))
	}
	seen := map[string]bool{}
	for _, a := range all {
		if seen[a.ID] {
			t.Fatal("duplicate across pages")
		}
		seen[a.ID] = true
	}
	gmail, _, err := s.ListActions(ctx, ws, store.ActionFilter{Tool: "gmail.send"})
	if err != nil || len(gmail) != 4 {
		t.Fatalf("tool filter: %d %v", len(gmail), err)
	}
	q, _, err := s.ListActions(ctx, ws, store.ActionFilter{Query: "100%_invoice"})
	if err != nil || len(q) != 0 {
		t.Fatalf("wildcards not escaped: %d %v", len(q), err)
	}
}

func TestGrantConsumedOnce(t *testing.T) {
	s := storetest.New(t)
	ctx := context.Background()
	ws, agent := seed(t, s)
	a := insertAction(t, s, ws, agent, "gmail.send")
	g := store.Grant{ID: id.New(id.Grant), ActionID: a.ID, WorkspaceID: ws, Tool: "gmail.send", ArgsHash: []byte{1}, KeyID: "k", ExpiresAt: time.Now().Add(time.Minute)}
	if err := s.InsertGrant(ctx, g); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeGrant(ctx, ws, g.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeGrant(ctx, ws, g.ID); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("second consume: %v", err)
	}
	if err := s.ConsumeGrant(ctx, ws, "grt_missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing grant: %v", err)
	}
}

func TestOverviewAndCalibrationQueriesRun(t *testing.T) {
	s := storetest.New(t)
	ctx := context.Background()
	ws, agent := seed(t, s)
	for i := 0; i < 3; i++ {
		a := insertAction(t, s, ws, agent, "gmail.send")
		_, _ = s.FinalizeAction(ctx, ws, a.ID, domain.StatusApproved)
		choice := "proceed"
		mc := 0.65 + float64(i)*0.1
		by := domain.DecidedByModel
		if i == 0 {
			by = domain.DecidedByHuman
		}
		if err := s.InsertDecision(ctx, &store.Decision{ActionID: a.ID, WorkspaceID: ws, Option: domain.OptionProceed,
			Outcome: "approved", DecidedBy: by, FinalTool: "gmail.send", ModelChoice: &choice, ModelConfidence: &mc}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	o, err := s.Overview(ctx, ws, now.Add(-24*time.Hour), now.Add(time.Minute), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if o.Counts.Received != 3 || o.Counts.ByHuman != 1 || o.Counts.ByModel != 2 || len(o.Series) < 24 {
		t.Fatalf("overview counts %+v series %d", o.Counts, len(o.Series))
	}
	bins, err := s.CalibrationBins(ctx, ws, now.Add(-time.Hour), now.Add(time.Minute), "")
	if err != nil || len(bins) != 3 {
		t.Fatalf("bins %+v %v", bins, err)
	}
}
