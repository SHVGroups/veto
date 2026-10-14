package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shvgroups/veto/server/internal/app"
	"github.com/shvgroups/veto/server/internal/apptest"
	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/jev"
	"github.com/shvgroups/veto/server/internal/store"
)

type harness struct{ *apptest.Harness }

func newHarness(t *testing.T) harness { return harness{apptest.New(t)} }

func (h harness) submit(t *testing.T, req app.SubmitRequest, wait time.Duration) *app.ActionResult {
	t.Helper()
	res, err := h.App.Submit(context.Background(), h.Agent, domain.SourceAPI, req, wait)
	if err != nil {
		t.Fatalf("submit %s: %v", req.Tool, err)
	}
	return res
}

func email(to string) app.SubmitRequest {
	return app.SubmitRequest{
		Tool:    "gmail.send",
		Args:    json.RawMessage(`{"to":["ap@acme.com","` + to + `"],"subject":"Invoice Q3"}`),
		Request: "Send the Q3 invoice to Acme's finance contact",
	}
}

func TestReadToolIsAllowedByPolicyAndGrantWorksOnce(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	res := h.submit(t, app.SubmitRequest{Tool: "crm.lookup", Args: json.RawMessage(`{"customer":"acme"}`)}, 0)
	if res.Status != domain.StatusApproved || res.Decision.DecidedBy != domain.DecidedByPolicy || res.Grant == nil {
		t.Fatalf("result %+v", res)
	}
	if h.Model.Calls() != 0 {
		t.Fatalf("model called %d times for a policy-allowed call", h.Model.Calls())
	}
	verify := app.VerifyRequest{Token: res.Grant.Token, Tool: "crm.lookup", Args: json.RawMessage(`{ "customer" : "acme" }`), Consume: true}
	if v, err := h.App.VerifyGrant(ctx, h.Agent, verify); err != nil || !v.Consumed {
		t.Fatalf("verify: %+v %v", v, err)
	}
	if _, err := h.App.VerifyGrant(ctx, h.Agent, verify); err == nil {
		t.Fatal("grant honored twice")
	}
	verify.Args, verify.Consume = json.RawMessage(`{"customer":"globex"}`), false
	var ae *app.Error
	if _, err := h.App.VerifyGrant(ctx, h.Agent, verify); !errors.As(err, &ae) || ae.Code != app.CodeForbidden {
		t.Fatalf("grant for different args: %v", err)
	}
	// After the action ran, no new grant is handed out.
	again, err := h.App.GetResult(ctx, h.Agent, res.ID, 0)
	if err != nil || again.Grant != nil {
		t.Fatalf("grant reissued after use: %+v %v", again.Grant, err)
	}
}

func TestConfidentModelRunsSaferVariant(t *testing.T) {
	h := newHarness(t)
	h.Model.Set(map[string]float64{"proceed": 0.03, "proceed_safer": 0.95, "stop": 0.02}, map[string]float64{"matches_request": 0.97}, nil)
	res := h.submit(t, email("cfo@rival.io"), 0)
	if res.Status != domain.StatusApproved || res.Decision.Option != domain.OptionProceedSafer || res.Decision.DecidedBy != domain.DecidedByModel {
		t.Fatalf("result %+v reasons %+v", res, res.HoldReasons)
	}
	if res.Decision.Tool != "gmail.create_draft" || strings.Contains(string(res.Decision.Args), "rival.io") {
		t.Fatalf("decision %s %s", res.Decision.Tool, res.Decision.Args)
	}
	detail, err := h.App.GetActionDetail(context.Background(), h.Owner, res.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Assessment == nil || len(detail.Evaluations) != 2 || len(detail.Grants) != 1 || len(detail.Ledger) < 3 {
		t.Fatalf("detail: assessment=%v evals=%d grants=%d ledger=%d", detail.Assessment != nil, len(detail.Evaluations), len(detail.Grants), len(detail.Ledger))
	}
	kinds := []string{}
	for _, e := range detail.Ledger {
		// Audit sampling is keyed by the random action id, so roughly one
		// run in fifty also records audit.sampled.
		if e.Kind != "audit.sampled" {
			kinds = append(kinds, e.Kind)
		}
	}
	if strings.Join(kinds, ",") != "action.received,action.decided,grant.issued" {
		t.Fatalf("ledger kinds %v", kinds)
	}
}

func TestLowConfidenceIsHeldThenReviewed(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.Model.Set(map[string]float64{"proceed": 0.5, "proceed_safer": 0.3, "stop": 0.2}, map[string]float64{"matches_request": 0.9}, nil)
	res := h.submit(t, email("ops@acme.com"), 0)
	if res.Status != domain.StatusPendingReview || res.ReviewURL == "" || res.Decision != nil {
		t.Fatalf("result %+v", res)
	}
	if res.HoldReasons[0].Code != domain.ReasonLowConfidence {
		t.Fatalf("reasons %+v", res.HoldReasons)
	}
	if _, err := h.App.Review(ctx, h.Agent, res.ID, app.ReviewRequest{Option: domain.OptionProceed}, app.ChannelAPI); err == nil {
		t.Fatal("agent was allowed to review")
	}
	act, err := h.App.Review(ctx, h.Owner, res.ID, app.ReviewRequest{Option: domain.OptionProceedSafer, Note: "draft first"}, app.ChannelConsole)
	if err != nil || act.Status != domain.StatusApproved {
		t.Fatalf("review: %+v %v", act, err)
	}
	if _, err := h.App.Review(ctx, h.Owner, res.ID, app.ReviewRequest{Option: domain.OptionStop}, app.ChannelConsole); err == nil {
		t.Fatal("decided twice")
	}
	got, err := h.App.GetResult(ctx, h.Agent, res.ID, 0)
	if err != nil || got.Decision.DecidedBy != domain.DecidedByHuman || got.Decision.Note != "draft first" || got.Grant == nil {
		t.Fatalf("result after review %+v %v", got, err)
	}
	dec, _ := h.Store.GetDecision(ctx, h.Owner.WorkspaceID, res.ID)
	if dec.ModelChoice == nil || *dec.ModelChoice != "proceed" || dec.ReviewerID == nil {
		t.Fatalf("decision lacks calibration data: %+v", dec)
	}
}

func TestSubmitWaitsForReviewer(t *testing.T) {
	h := newHarness(t)
	h.Model.Set(map[string]float64{"proceed": 0.4, "proceed_safer": 0.35, "stop": 0.25}, nil, nil)
	var wg sync.WaitGroup
	results := make(chan *app.ActionResult, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		results <- h.submit(t, email("x@acme.com"), 8*time.Second)
	}()
	deadline := time.Now().Add(5 * time.Second)
	var pending []store.ActionSummary
	for time.Now().Before(deadline) {
		pending, _, _ = h.Store.ListActions(context.Background(), h.Owner.WorkspaceID, store.ActionFilter{Statuses: []string{"pending_review"}})
		if len(pending) == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(pending) != 1 {
		t.Fatal("action never became pending")
	}
	if _, err := h.App.Review(context.Background(), h.Owner, pending[0].ID, app.ReviewRequest{Option: domain.OptionStop}, app.ChannelConsole); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	res := <-results
	if res.Status != domain.StatusDenied || res.Decision.Option != domain.OptionStop {
		t.Fatalf("waiting submit returned %+v", res)
	}
}

func TestIdempotentSubmissions(t *testing.T) {
	h := newHarness(t)
	h.Model.Set(map[string]float64{"proceed": 0.95, "proceed_safer": 0.03, "stop": 0.02}, map[string]float64{"matches_request": 0.98}, nil)
	req := email("ap2@acme.com")
	req.IdempotencyKey = "retry-1"
	first := h.submit(t, req, 0)
	second := h.submit(t, req, 0)
	if first.ID != second.ID || second.Grant == nil {
		t.Fatalf("ids %s %s", first.ID, second.ID)
	}
	req.Args = json.RawMessage(`{"to":["someone@else.com"]}`)
	var ae *app.Error
	if _, err := h.App.Submit(context.Background(), h.Agent, domain.SourceAPI, req, 0); !errors.As(err, &ae) || ae.Code != app.CodeConflict {
		t.Fatalf("reused key with different args: %v", err)
	}
}

func TestUnknownToolIsDiscoveredAndHeld(t *testing.T) {
	h := newHarness(t)
	res := h.submit(t, app.SubmitRequest{Tool: "salesforce.delete_account", Args: json.RawMessage(`{"id":"001"}`)}, 0)
	if res.Status != domain.StatusPendingReview || res.HoldReasons[0].Code != domain.ReasonPolicyReview {
		t.Fatalf("result %+v", res)
	}
	tool, err := h.Store.GetTool(context.Background(), h.Owner.WorkspaceID, "salesforce.delete_account")
	if err != nil || tool.Registered || tool.RiskClass != domain.RiskHigh {
		t.Fatalf("discovered tool %+v %v", tool, err)
	}
}

func TestModelOutageFailsClosed(t *testing.T) {
	h := newHarness(t)
	h.Model.Set(nil, nil, &jev.APIError{Endpoint: "typesafe", StatusCode: 529})
	res := h.submit(t, email("a@acme.com"), 0)
	if res.Status != domain.StatusPendingReview || res.HoldReasons[0].Code != domain.ReasonModelUnavailable {
		t.Fatalf("result %+v", res)
	}
	detail, _ := h.App.GetActionDetail(context.Background(), h.Owner, res.ID)
	if len(detail.Evaluations) != 2 || detail.Evaluations[1].Error == "" {
		t.Fatalf("model error not recorded: %+v", detail.Evaluations)
	}
}

func TestStepUpAndExpiry(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.Model.Set(map[string]float64{"proceed": 0.4, "proceed_safer": 0.4, "stop": 0.2}, map[string]float64{"matches_request": 0.9, "recipient_matches": 0.9}, nil)
	res := h.submit(t, app.SubmitRequest{Tool: "stripe.refund", Args: json.RawMessage(`{"amount_cents":12000,"charge":"ch_1"}`), Request: "Refund Acme's duplicate charge"}, 0)
	if res.Status != domain.StatusPendingReview {
		t.Fatalf("refund not held: %+v", res)
	}
	var ae *app.Error
	if _, err := h.App.Review(ctx, h.Owner, res.ID, app.ReviewRequest{Option: domain.OptionProceed}, app.ChannelConsole); !errors.As(err, &ae) || ae.Code != app.CodeForbidden {
		t.Fatalf("step-up not enforced: %v", err)
	}
	if _, err := h.App.Review(ctx, h.Owner, res.ID, app.ReviewRequest{Option: domain.OptionProceedSafer, Password: "correct horse battery"}, app.ChannelConsole); err != nil {
		t.Fatalf("step-up with password: %v", err)
	}

	held := h.submit(t, app.SubmitRequest{Tool: "stripe.refund", Args: json.RawMessage(`{"amount_cents":100}`), Request: "Refund"}, 0)
	if _, err := h.Store.Pool().Exec(ctx, `UPDATE actions SET review_deadline = now() - interval '1 second' WHERE id = $1`, held.ID); err != nil {
		t.Fatal(err)
	}
	refs, err := h.Store.ExpiredPending(ctx, 10)
	if err != nil || len(refs) != 1 {
		t.Fatalf("expired refs %v %v", refs, err)
	}
	if err := h.App.Expire(ctx, refs[0]); err != nil {
		t.Fatal(err)
	}
	got, _ := h.App.GetResult(ctx, h.Agent, held.ID, 0)
	if got.Status != domain.StatusExpired || got.Decision.DecidedBy != domain.DecidedByExpiry {
		t.Fatalf("expired result %+v", got)
	}
}

func TestBadOutcomeQueuesAuditAndLedgerVerifies(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.Model.Set(map[string]float64{"proceed": 0.96, "proceed_safer": 0.02, "stop": 0.02}, map[string]float64{"matches_request": 0.99}, nil)
	res := h.submit(t, email("billing@acme.com"), 0)
	if res.Decision == nil || res.Decision.DecidedBy != domain.DecidedByModel {
		t.Fatalf("result %+v %+v", res, res.HoldReasons)
	}
	if _, err := h.App.ReportOutcome(ctx, h.Agent, res.ID, app.OutcomeRequest{Status: domain.OutcomeComplaint, Detail: json.RawMessage(`{"by":"customer"}`)}); err != nil {
		t.Fatal(err)
	}
	audits, err := h.App.ListAudits(ctx, h.Owner, true)
	if err != nil || len(audits) != 1 || audits[0].Reason != "outcome" {
		t.Fatalf("audits %+v %v", audits, err)
	}
	if _, err := h.App.RecordAudit(ctx, h.Owner, audits[0].ID, domain.VerdictShouldHold, "customer complained"); err != nil {
		t.Fatal(err)
	}
	o, err := h.App.Overview(ctx, h.Owner, "24h")
	if err != nil || o.Rates.FalseApproval == nil || o.Rates.FalseApproval.Value != 1 {
		t.Fatalf("overview false approval %+v %v", o, err)
	}
	if err := h.App.Checkpoint(ctx, h.Owner.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	rep, err := h.App.VerifyLedger(ctx, h.Owner)
	if err != nil || rep.Break != nil || rep.Entries < 10 || rep.Checkpoint == nil || !rep.Checkpoint.Valid {
		t.Fatalf("verify %+v %v", rep, err)
	}
	if _, err := h.App.Calibration(ctx, h.Owner, "30d", ""); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteThroughConnection(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()
	secret := "sk_live_secret"
	if _, err := h.App.CreateConnection(ctx, h.Owner, app.ConnectionInput{
		Name: "billing", BaseURL: srv.URL, AuthScheme: "bearer", Secret: &secret, AllowPrivate: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.App.SaveTool(ctx, h.Owner, app.ToolInput{Name: "http.billing", RiskClass: domain.RiskLow}); err != nil {
		t.Fatal(err)
	}
	h.Model.Set(map[string]float64{"proceed": 0.97, "stop": 0.03}, map[string]float64{"matches_request": 0.99}, nil)
	res := h.submit(t, app.SubmitRequest{Tool: "http.billing", Args: json.RawMessage(`{"method":"POST","path":"/invoices","body":{"amount":100}}`), Request: "Create the invoice"}, 0)
	if res.Status != domain.StatusApproved {
		t.Fatalf("not approved: %+v %+v", res, res.HoldReasons)
	}
	resp, err := h.App.Execute(ctx, h.Agent, res.ID)
	if err != nil || resp.Status != 200 || gotAuth != "Bearer sk_live_secret" {
		t.Fatalf("execute %+v %v auth %q", resp, err, gotAuth)
	}
	if _, err := h.App.Execute(ctx, h.Agent, res.ID); err == nil {
		t.Fatal("executed twice")
	}
	detail, _ := h.App.GetActionDetail(ctx, h.Owner, res.ID)
	if len(detail.Outcomes) != 1 || detail.Outcomes[0].Status != domain.OutcomeSucceeded {
		t.Fatalf("outcomes %+v", detail.Outcomes)
	}
}

func TestPermissions(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv, token, err := h.App.Invite(ctx, h.Owner, app.InviteRequest{Email: "viewer@acme.com", Role: domain.RoleViewer})
	if err != nil || inv.ID == "" {
		t.Fatal(err)
	}
	sess, err := h.App.AcceptInvitation(ctx, app.AcceptRequest{Token: token, Name: "Vic Viewer", Password: "another long password"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	viewer, _ := h.App.Authenticate(ctx, sess.Token)
	var ae *app.Error
	if _, err := h.App.SaveTool(ctx, viewer, app.ToolInput{Name: "x.y", RiskClass: domain.RiskLow}); !errors.As(err, &ae) || ae.Code != app.CodeForbidden {
		t.Fatalf("viewer saved a tool: %v", err)
	}
	if _, _, err := h.App.ListActions(ctx, viewer, store.ActionFilter{}); err != nil {
		t.Fatalf("viewer cannot view: %v", err)
	}
	if err := h.App.RemoveMember(ctx, h.Owner, h.Owner.ID); err == nil {
		t.Fatal("removed the last owner")
	}
	if _, err := h.App.AcceptInvitation(ctx, app.AcceptRequest{Token: token, Name: "x", Password: "another long password"}, "", ""); err == nil {
		t.Fatal("invitation used twice")
	}
}
