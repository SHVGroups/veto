package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/shvgroups/veto/server/internal/canonical"
	"github.com/shvgroups/veto/server/internal/decide"
	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/events"
	"github.com/shvgroups/veto/server/internal/grant"
	"github.com/shvgroups/veto/server/internal/id"
	"github.com/shvgroups/veto/server/internal/ledger"
	"github.com/shvgroups/veto/server/internal/notify"
	"github.com/shvgroups/veto/server/internal/policy"
	"github.com/shvgroups/veto/server/internal/safer"
	"github.com/shvgroups/veto/server/internal/store"
)

// Limits on what an agent may submit. They keep a single action from
// exhausting memory or the model's context.
const (
	maxArgsBytes       = 256 << 10
	maxRequestChars    = 8000
	maxUntrustedItems  = 50
	maxUntrustedChars  = 100000
	maxUntrustedTotal  = 400000
	maxContextBytes    = 16 << 10
	maxIdempotencyChar = 200
	maxPrincipalChars  = 200
)

var toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,199}$`)

// ValidToolName reports whether s is an acceptable tool name.
func ValidToolName(s string) bool { return toolNamePattern.MatchString(s) }

// SubmitRequest is an agent's proposal to call a tool.
type SubmitRequest struct {
	Tool           string             `json:"tool"`
	Args           json.RawMessage    `json:"args"`
	Principal      *domain.Principal  `json:"principal,omitempty"`
	Request        string             `json:"request,omitempty"`
	Untrusted      []domain.Untrusted `json:"untrusted,omitempty"`
	Context        json.RawMessage    `json:"context,omitempty"`
	IdempotencyKey string             `json:"idempotency_key,omitempty"`
}

func (r *SubmitRequest) validate() error {
	if !ValidToolName(r.Tool) {
		return invalidf("tool must be 1 to 200 characters of letters, digits and _ . : / -")
	}
	if len(r.Args) == 0 {
		r.Args = json.RawMessage("{}")
	}
	if len(r.Args) > maxArgsBytes {
		return invalidf("args must be at most %d bytes", maxArgsBytes)
	}
	v, err := canonical.Decode(r.Args)
	if err != nil {
		return invalidf("args must be valid JSON: %v", err)
	}
	if _, ok := v.(map[string]any); !ok {
		return invalidf("args must be a JSON object")
	}
	if utf8.RuneCountInString(r.Request) > maxRequestChars {
		return invalidf("request must be at most %d characters", maxRequestChars)
	}
	if len(r.Untrusted) > maxUntrustedItems {
		return invalidf("untrusted may hold at most %d items", maxUntrustedItems)
	}
	total := 0
	for i, u := range r.Untrusted {
		n := utf8.RuneCountInString(u.Content)
		if n > maxUntrustedChars {
			return invalidf("untrusted[%d].content must be at most %d characters", i, maxUntrustedChars)
		}
		if len(u.Source) > 500 {
			return invalidf("untrusted[%d].source must be at most 500 bytes", i)
		}
		total += n
	}
	if total > maxUntrustedTotal {
		return invalidf("untrusted content must total at most %d characters", maxUntrustedTotal)
	}
	if len(r.Context) > 0 {
		if len(r.Context) > maxContextBytes {
			return invalidf("context must be at most %d bytes", maxContextBytes)
		}
		cv, err := canonical.Decode(r.Context)
		if err != nil {
			return invalidf("context must be valid JSON: %v", err)
		}
		if _, ok := cv.(map[string]any); !ok {
			return invalidf("context must be a JSON object")
		}
	}
	if len(r.IdempotencyKey) > maxIdempotencyChar {
		return invalidf("idempotency_key must be at most %d characters", maxIdempotencyChar)
	}
	if r.Principal != nil && (len(r.Principal.ID) > maxPrincipalChars || len(r.Principal.Display) > maxPrincipalChars) {
		return invalidf("principal id and display must be at most %d characters", maxPrincipalChars)
	}
	return nil
}

// ActionResult is what an agent receives about its action.
type ActionResult struct {
	ID             string              `json:"id"`
	Status         domain.ActionStatus `json:"status"`
	Tool           string              `json:"tool"`
	Decision       *ResultDecision     `json:"decision"`
	Grant          *ResultGrant        `json:"grant"`
	HoldReasons    []domain.Reason     `json:"hold_reasons"`
	Options        []domain.OptionSpec `json:"options"`
	ReviewURL      string              `json:"review_url,omitempty"`
	ReviewDeadline *time.Time          `json:"review_deadline"`
	ReceivedAt     time.Time           `json:"received_at"`
	DecidedAt      *time.Time          `json:"decided_at"`
}

// ResultDecision says what to run, if anything.
type ResultDecision struct {
	Option     domain.Option    `json:"option"`
	Approved   bool             `json:"approved"`
	Tool       string           `json:"tool,omitempty"`
	Args       json.RawMessage  `json:"args,omitempty"`
	DecidedBy  domain.DecidedBy `json:"decided_by"`
	Reasons    []domain.Reason  `json:"reasons"`
	Confidence *float64         `json:"confidence"`
	Note       string           `json:"note,omitempty"`
}

// ResultGrant is the permission token for an approved action.
type ResultGrant struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// evaluation is everything the pipeline learned about one action.
type evaluation struct {
	gate          policy.Result
	policyVersion *int
	gateLatency   time.Duration
	rateChecked   bool
	recentCalls   int
	options       []domain.OptionSpec
	pack          *decide.Pack
	assessment    *decide.Assessment
	calls         []decide.Call
	modelErr      error
	thresholds    domain.Thresholds
	verdict       decide.Verdict
}

// Submit receives a proposed tool call, decides it, and returns the result.
// When the action is held and wait is positive, Submit waits up to wait for
// a reviewer before returning.
func (a *App) Submit(ctx context.Context, actor Actor, src domain.Source, req SubmitRequest, wait time.Duration) (*ActionResult, error) {
	if actor.Kind != ActorAgent {
		return nil, forbiddenf("only agent API keys can submit actions")
	}
	if err := req.validate(); err != nil {
		return nil, err
	}
	start := a.now()
	hash, err := canonical.ArgsHashJSON(req.Tool, req.Args)
	if err != nil {
		return nil, invalidf("args: %v", err)
	}
	ws, err := a.Store.GetWorkspace(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	agent, err := a.Store.GetAgent(ctx, ws.ID, actor.AgentID)
	if err != nil {
		return nil, mapStoreErr(err, "agent")
	}

	if req.IdempotencyKey != "" {
		if prev, err := a.Store.GetActionByIdempotencyKey(ctx, ws.ID, agent.ID, req.IdempotencyKey); err == nil {
			return a.replay(ctx, prev, req.Tool, hash, wait)
		} else if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
	}

	tool, discovered, err := a.Store.EnsureTool(ctx, ws.ID, req.Tool)
	if err != nil {
		return nil, err
	}

	act := &store.Action{
		ID:          id.New(id.Action),
		WorkspaceID: ws.ID,
		AgentID:     agent.ID,
		AgentName:   agent.Name,
		Tool:        req.Tool,
		Args:        req.Args,
		ArgsHash:    hash,
		Request:     req.Request,
		Untrusted:   req.Untrusted,
		Context:     req.Context,
		Source:      src,
		Status:      domain.StatusEvaluating,
	}
	if req.Principal != nil {
		act.Principal = *req.Principal
	}
	if req.IdempotencyKey != "" {
		key := req.IdempotencyKey
		act.IdempotencyKey = &key
	}

	err = a.Store.Tx(ctx, func(q *store.Queries) error {
		if err := q.InsertAction(ctx, act); err != nil {
			return err
		}
		if discovered {
			if err := record(ctx, q, ws.ID, ledger.KindToolUpdated, "", actor, map[string]any{
				"tool": tool.Name, "registered": false, "risk_class": tool.RiskClass, "discovered_by": act.ID,
			}); err != nil {
				return err
			}
			if err := enqueueWebhook(ctx, q, ws.ID, notify.EventToolDiscovered, map[string]any{"tool": tool.Name, "action_id": act.ID}); err != nil {
				return err
			}
			if err := publish(ctx, q, events.Event{WorkspaceID: ws.ID, Kind: events.ToolRegistered, Subject: tool.Name}); err != nil {
				return err
			}
		}
		if err := record(ctx, q, ws.ID, ledger.KindActionReceived, act.ID, actor, map[string]any{
			"tool":            act.Tool,
			"args_hash":       hex.EncodeToString(hash),
			"agent_id":        agent.ID,
			"principal":       act.Principal.ID,
			"source":          src,
			"idempotency_key": req.IdempotencyKey,
			"untrusted_items": len(req.Untrusted),
		}); err != nil {
			return err
		}
		return publish(ctx, q, events.Event{WorkspaceID: ws.ID, Kind: events.ActionReceived, ActionID: act.ID, Status: string(act.Status)})
	})
	if errors.Is(err, store.ErrConflict) && req.IdempotencyKey != "" {
		// Another request with the same key won the race.
		prev, err := a.Store.GetActionByIdempotencyKey(ctx, ws.ID, agent.ID, req.IdempotencyKey)
		if err != nil {
			return nil, err
		}
		return a.replay(ctx, prev, req.Tool, hash, wait)
	}
	if err != nil {
		return nil, err
	}

	ev := a.evaluate(ctx, a.Model, ws, agent, tool, act)
	if err := a.applyEvaluation(ctx, ws, actor, tool, act, ev); err != nil {
		return nil, err
	}

	route := string(ev.verdict.Route)
	resolution := "held"
	if ev.verdict.Route == decide.RouteAuto {
		resolution = string(ev.verdict.Option)
	}
	a.Metrics.Actions.Inc(string(src), route, resolution)
	a.Metrics.DecisionSeconds.Observe(a.now().Sub(start).Seconds(), route)

	if ev.verdict.Route == decide.RouteHuman {
		go a.notifySlackHeld(ws.ID, act.ID)
		if wait > 0 {
			decided, err := a.WaitForDecision(ctx, ws.ID, act.ID, wait)
			if err != nil {
				return nil, err
			}
			return a.Result(ctx, decided)
		}
	}
	fresh, err := a.Store.GetAction(ctx, ws.ID, act.ID)
	if err != nil {
		return nil, err
	}
	return a.Result(ctx, fresh)
}

// replay answers a retried submission with the original action's result.
func (a *App) replay(ctx context.Context, prev store.Action, tool string, hash []byte, wait time.Duration) (*ActionResult, error) {
	if prev.Tool != tool || !bytes.Equal(prev.ArgsHash, hash) {
		return nil, conflictf("idempotency key %q was already used for a different call", *prev.IdempotencyKey)
	}
	if prev.Status == domain.StatusPendingReview && wait > 0 {
		decided, err := a.WaitForDecision(ctx, prev.WorkspaceID, prev.ID, wait)
		if err != nil {
			return nil, err
		}
		prev = decided
	}
	return a.Result(ctx, prev)
}

// evaluate runs the gate, the rate limit, option building and the model. It
// never fails: every problem becomes part of the evaluation, and the
// decision rules decide what it means.
func (a *App) evaluate(ctx context.Context, model decide.Model, ws store.Workspace, agent store.Agent, tool domain.Tool, act *store.Action) evaluation {
	ev := evaluation{thresholds: tool.Thresholds()}
	ev.options = []domain.OptionSpec{
		{Key: domain.OptionProceed, Available: true, Tool: act.Tool, Args: act.Args},
		{Key: domain.OptionProceedSafer},
		{Key: domain.OptionStop, Available: true},
	}

	if agent.DisabledAt != nil {
		ev.gate = policy.Result{Outcome: policy.OutcomeDeny}
		ev.verdict = decide.Verdict{
			Route: decide.RouteAuto, Option: domain.OptionStop, DecidedBy: domain.DecidedByPolicy,
			Reasons: []domain.Reason{{Code: domain.ReasonAgentDisabled}},
			Ranking: []domain.Option{domain.OptionStop},
		}
		return ev
	}

	args, _ := canonical.Decode(act.Args)
	gateStart := a.now()
	set, err := a.policySet(ctx, ws)
	if err != nil {
		a.Log.Error("policy evaluation unavailable; holding action", "workspace", ws.ID, "action", act.ID, "error", err)
		ev.gate = policy.Result{Outcome: policy.OutcomeReview, Errors: []policy.Diagnostic{{Message: err.Error()}}}
	} else {
		if set != nil {
			v := set.Version
			ev.policyVersion = &v
		}
		ev.gate = set.Evaluate(policy.Input{
			Agent:       policy.Agent{ID: agent.ID, Name: agent.Name, Vendor: agent.Vendor},
			Tool:        tool,
			Args:        args,
			PrincipalID: act.Principal.ID,
			Source:      act.Source,
			Now:         a.now(),
		})
	}
	ev.gateLatency = a.now().Sub(gateStart)

	if tool.MaxPerHour != nil {
		ev.rateChecked = true
		n, err := a.Store.CountRecentCalls(ctx, ws.ID, agent.ID, tool.Name, time.Hour)
		if err != nil {
			a.Log.Error("rate limit check failed; holding action", "action", act.ID, "error", err)
			n = math.MaxInt32
		}
		ev.recentCalls = n
	}

	rules, err := safer.ParseRules(tool.Safer)
	if err != nil {
		a.Log.Warn("tool has invalid safer-variant rules", "workspace", ws.ID, "tool", tool.Name, "error", err)
	} else if b, err := safer.Build(act.Tool, act.Args, rules); err != nil {
		a.Log.Warn("could not build safer variant", "action", act.ID, "error", err)
	} else {
		ev.options[1] = b
	}

	if decide.NeedsModel(ev.gate) && model != nil {
		pack, err := a.pack(ctx, ws.ID, tool.Pack)
		if err != nil {
			ev.modelErr = err
		} else {
			ev.pack = pack
			mctx, cancel := context.WithTimeout(ctx, a.DecisionTimeout)
			ev.assessment, ev.calls, ev.modelErr = decide.Assess(mctx, model, pack, decide.Subject{
				ActionID:          act.ID,
				Request:           act.Request,
				Agent:             agent.Name,
				Tool:              tool,
				Args:              act.Args,
				Safer:             ev.options[1],
				Untrusted:         act.Untrusted,
				UntrustedMaxChars: ws.Settings.UntrustedMaxChars,
			}, ev.thresholds.DoubleCheck)
			cancel()
			for _, c := range ev.calls {
				result := "ok"
				if c.Err != nil {
					result = "error"
				}
				a.Metrics.ModelRequests.Inc(c.Stage, result)
				a.Metrics.ModelSeconds.Observe(c.Latency.Seconds(), c.Stage)
			}
			if ev.modelErr != nil {
				a.Log.Warn("decision model unavailable", "action", act.ID, "error", ev.modelErr)
			}
		}
	}

	ev.verdict = decide.Resolve(decide.Inputs{
		Gate:            ev.gate,
		RateLimited:     ev.rateChecked && ev.recentCalls > *tool.MaxPerHour,
		Thresholds:      ev.thresholds,
		FailMode:        tool.FailMode,
		SaferAvailable:  ev.options[1].Available,
		ModelConfigured: model != nil,
		Assessment:      ev.assessment,
		ModelErr:        ev.modelErr,
	})
	return ev
}

// applyEvaluation persists an evaluation and, for automatic verdicts, the
// decision and its grant, all in one transaction.
func (a *App) applyEvaluation(ctx context.Context, ws store.Workspace, actor Actor, tool domain.Tool, act *store.Action, ev evaluation) error {
	v := ev.verdict
	return a.Store.Tx(ctx, func(q *store.Queries) error {
		if err := a.saveEvaluations(ctx, q, act, ev); err != nil {
			return err
		}
		model := modelSummary(ev)
		gate := map[string]any{"outcome": ev.gate.Outcome, "policies": ev.gate.Policies, "version": ev.policyVersion}

		if v.Route == decide.RouteHuman {
			deadline := a.now().Add(ws.Settings.ReviewTimeout())
			if err := q.RecordEvaluation(ctx, ws.ID, act.ID, store.EvaluatedAction{
				Status: domain.StatusPendingReview, Options: ev.options, Ranking: v.Ranking,
				HoldReasons: v.Reasons, PolicyVersion: ev.policyVersion, ReviewDeadline: &deadline,
			}); err != nil {
				return err
			}
			if err := record(ctx, q, ws.ID, ledger.KindActionHeld, act.ID, System(ws.ID), map[string]any{
				"reasons": v.Reasons, "ranking": v.Ranking, "deadline": deadline, "gate": gate, "model": model,
			}); err != nil {
				return err
			}
			if err := enqueueWebhook(ctx, q, ws.ID, notify.EventActionPending, a.webhookAction(act, v.Reasons, &deadline)); err != nil {
				return err
			}
			return publish(ctx, q, events.Event{WorkspaceID: ws.ID, Kind: events.ActionHeld, ActionID: act.ID, Status: string(domain.StatusPendingReview), Subject: act.Tool})
		}

		if err := q.RecordEvaluation(ctx, ws.ID, act.ID, store.EvaluatedAction{
			Status: domain.StatusEvaluating, Options: ev.options, Ranking: v.Ranking, PolicyVersion: ev.policyVersion,
		}); err != nil {
			return err
		}
		d := &store.Decision{
			ActionID:    act.ID,
			WorkspaceID: ws.ID,
			Option:      v.Option,
			DecidedBy:   v.DecidedBy,
			Reasons:     v.Reasons,
			Thresholds:  ev.thresholds,
		}
		if ev.assessment != nil {
			conf, risk, choice := ev.assessment.Confidence, ev.assessment.Risk, string(ev.assessment.Choice)
			d.Confidence, d.Risk, d.ModelChoice, d.ModelConfidence = &conf, &risk, &choice, &conf
		}
		spec := optionSpec(ev.options, v.Option)
		return a.finalize(ctx, q, ws, act, d, spec, System(ws.ID), map[string]any{"gate": gate, "model": model})
	})
}

// finalize records a final decision on an action: status, decision row,
// grant, ledger entry, audit sampling, webhook and event. q must be inside a
// transaction.
func (a *App) finalize(ctx context.Context, q *store.Queries, ws store.Workspace, act *store.Action, d *store.Decision, spec domain.OptionSpec, actor Actor, extra map[string]any) error {
	status := domain.StatusApproved
	d.Outcome = "approved"
	switch {
	case d.DecidedBy == domain.DecidedByExpiry:
		status, d.Outcome = domain.StatusExpired, "expired"
	case d.Option == domain.OptionStop:
		status, d.Outcome = domain.StatusDenied, "denied"
	}
	decidedAt, err := q.FinalizeAction(ctx, ws.ID, act.ID, status)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			return conflictf("action %s was already decided", act.ID)
		}
		return err
	}
	if status == domain.StatusApproved {
		d.FinalTool, d.FinalArgs = spec.Tool, spec.Args
	}
	if err := q.InsertDecision(ctx, d); err != nil {
		return err
	}
	act.Status, act.DecidedAt = status, &decidedAt

	payload := map[string]any{
		"option":     d.Option,
		"outcome":    d.Outcome,
		"decided_by": d.DecidedBy,
		"reasons":    d.Reasons,
		"confidence": d.Confidence,
		"risk":       d.Risk,
		"note":       d.Note,
	}
	if status == domain.StatusApproved {
		h, err := canonical.ArgsHashJSON(spec.Tool, spec.Args)
		if err != nil {
			return err
		}
		payload["final_tool"] = spec.Tool
		payload["final_args_hash"] = hex.EncodeToString(h)
	}
	for k, v := range extra {
		payload[k] = v
	}
	kind := ledger.KindActionDecided
	if status == domain.StatusExpired {
		kind = ledger.KindActionExpired
	}
	if err := record(ctx, q, ws.ID, kind, act.ID, actor, payload); err != nil {
		return err
	}

	if status == domain.StatusApproved {
		if _, _, err := a.issueGrant(ctx, q, ws.ID, act.ID, d.Option, spec.Tool, spec.Args); err != nil {
			return err
		}
		auto := d.DecidedBy == domain.DecidedByModel || d.DecidedBy == domain.DecidedByFailsafe
		if auto && sampled(act.ID, ws.Settings.AuditSampleRate) {
			if err := a.queueAudit(ctx, q, ws.ID, act.ID, "sample"); err != nil {
				return err
			}
		}
	}

	event := notify.EventActionDecided
	if status == domain.StatusExpired {
		event = notify.EventActionExpired
	}
	data := a.webhookAction(act, nil, nil)
	data["decision"] = map[string]any{
		"option": d.Option, "outcome": d.Outcome, "decided_by": d.DecidedBy, "reasons": d.Reasons,
		"final_tool": d.FinalTool, "confidence": d.Confidence, "reviewer_id": d.ReviewerID,
	}
	if err := enqueueWebhook(ctx, q, ws.ID, event, data); err != nil {
		return err
	}
	return publish(ctx, q, events.Event{WorkspaceID: ws.ID, Kind: events.ActionDecided, ActionID: act.ID, Status: string(status), Subject: act.Tool})
}

func (a *App) queueAudit(ctx context.Context, q *store.Queries, ws, actionID, reason string) error {
	added, err := q.InsertAudit(ctx, store.Audit{ID: id.New(id.Audit), ActionID: actionID, WorkspaceID: ws, Reason: reason})
	if err != nil || !added {
		return err
	}
	if err := record(ctx, q, ws, ledger.KindAuditSampled, actionID, System(ws), map[string]any{"reason": reason}); err != nil {
		return err
	}
	if err := enqueueWebhook(ctx, q, ws, notify.EventAuditSampled, map[string]any{"action_id": actionID, "reason": reason}); err != nil {
		return err
	}
	return publish(ctx, q, events.Event{WorkspaceID: ws, Kind: events.AuditSampled, ActionID: actionID})
}

func (a *App) saveEvaluations(ctx context.Context, q *store.Queries, act *store.Action, ev evaluation) error {
	gateResult, _ := json.Marshal(map[string]any{
		"outcome": ev.gate.Outcome, "policies": ev.gate.Policies, "errors": ev.gate.Errors, "policy_version": ev.policyVersion,
	})
	if err := q.InsertEvaluation(ctx, store.Evaluation{
		ID: id.New(id.Evaluation), ActionID: act.ID, WorkspaceID: act.WorkspaceID, Stage: "gate",
		Provider: "cedar", Result: gateResult, LatencyMs: int(ev.gateLatency.Milliseconds()),
	}); err != nil {
		return err
	}
	if ev.rateChecked {
		res, _ := json.Marshal(map[string]any{"calls_last_hour": ev.recentCalls, "limited": ev.verdict.Route == decide.RouteHuman && hasReason(ev.verdict.Reasons, domain.ReasonRateLimited)})
		if err := q.InsertEvaluation(ctx, store.Evaluation{
			ID: id.New(id.Evaluation), ActionID: act.ID, WorkspaceID: act.WorkspaceID, Stage: "rate_limit", Result: res,
		}); err != nil {
			return err
		}
	}
	for i, c := range ev.calls {
		e := store.Evaluation{
			ID: id.New(id.Evaluation), ActionID: act.ID, WorkspaceID: act.WorkspaceID, Stage: c.Stage,
			LatencyMs: int(c.Latency.Milliseconds()),
		}
		if c.Result != nil {
			e.Provider, e.Model = c.Result.Endpoint, c.Result.Model
			e.Request, e.Response = c.Result.RequestBody, c.Result.RawResponse
			e.InputTokens = c.Result.Response.Usage.InputTokens
		} else {
			e.Request, _ = json.Marshal(c.Request)
		}
		if c.Err != nil {
			e.Error = truncate(c.Err.Error(), 1000)
		}
		switch {
		case i == 0 && ev.assessment != nil:
			e.Result, _ = json.Marshal(ev.assessment)
		case c.Stage == decide.StageRecheck && ev.assessment != nil && ev.assessment.Recheck != nil:
			e.Result, _ = json.Marshal(ev.assessment.Recheck)
		}
		if err := q.InsertEvaluation(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// issueGrant signs and stores a grant for an approved action.
func (a *App) issueGrant(ctx context.Context, q *store.Queries, ws, actionID string, option domain.Option, tool string, args json.RawMessage) (string, store.Grant, error) {
	key, err := a.signer(ctx, ws)
	if err != nil {
		return "", store.Grant{}, err
	}
	hash, err := canonical.ArgsHashJSON(tool, args)
	if err != nil {
		return "", store.Grant{}, err
	}
	now := a.now().UTC().Truncate(time.Second)
	g := store.Grant{
		ID: id.New(id.Grant), ActionID: actionID, WorkspaceID: ws, Tool: tool, ArgsHash: hash,
		KeyID: key.id, ExpiresAt: now.Add(grant.DefaultTTL), CreatedAt: now,
	}
	if err := q.InsertGrant(ctx, g); err != nil {
		return "", g, err
	}
	token, err := grant.Sign(grantClaims(g, option), key.private)
	if err != nil {
		return "", g, err
	}
	if err := record(ctx, q, ws, ledger.KindGrantIssued, actionID, System(ws), map[string]any{
		"grant_id": g.ID, "tool": tool, "args_hash": hex.EncodeToString(hash), "expires_at": g.ExpiresAt, "key_id": key.id,
	}); err != nil {
		return "", g, err
	}
	return token, g, nil
}

func grantClaims(g store.Grant, option domain.Option) grant.Claims {
	return grant.Claims{
		ID: g.ID, ActionID: g.ActionID, WorkspaceID: g.WorkspaceID, Tool: g.Tool,
		ArgsHash: hex.EncodeToString(g.ArgsHash), Option: option, KeyID: g.KeyID,
		IssuedAt: g.CreatedAt.Unix(), ExpiresAt: g.ExpiresAt.Unix(),
	}
}

// Result builds the agent-facing view of an action. For approved actions it
// includes a usable grant, re-signing the current one or issuing a new one
// if the last expired unused.
func (a *App) Result(ctx context.Context, act store.Action) (*ActionResult, error) {
	r := &ActionResult{
		ID: act.ID, Status: act.Status, Tool: act.Tool, HoldReasons: act.HoldReasons, Options: act.Options,
		ReviewDeadline: act.ReviewDeadline, ReceivedAt: act.ReceivedAt, DecidedAt: act.DecidedAt,
	}
	if act.Status == domain.StatusPendingReview {
		r.ReviewURL = a.reviewURL(act.ID)
	}
	if !act.Status.Final() {
		return r, nil
	}
	d, err := a.Store.GetDecision(ctx, act.WorkspaceID, act.ID)
	if err != nil {
		return nil, err
	}
	r.Decision = &ResultDecision{
		Option: d.Option, Approved: d.Outcome == "approved", DecidedBy: d.DecidedBy,
		Reasons: d.Reasons, Confidence: d.Confidence, Note: d.Note,
	}
	if !r.Decision.Approved {
		return r, nil
	}
	r.Decision.Tool, r.Decision.Args = d.FinalTool, d.FinalArgs
	if act.RedactedAt != nil {
		return r, nil
	}
	g, err := a.usableGrant(ctx, act, d.Option)
	if err != nil {
		return nil, err
	}
	r.Grant = g
	return r, nil
}

func (a *App) usableGrant(ctx context.Context, act store.Action, option domain.Option) (*ResultGrant, error) {
	grants, err := a.Store.ListGrants(ctx, act.WorkspaceID, act.ID)
	if err != nil {
		return nil, err
	}
	key, err := a.signer(ctx, act.WorkspaceID)
	if err != nil {
		return nil, err
	}
	now := a.now()
	for i := len(grants) - 1; i >= 0; i-- {
		g := grants[i]
		if g.ConsumedAt != nil {
			// The action already ran; no further grant is issued.
			return nil, nil
		}
		if g.ExpiresAt.After(now) && g.KeyID == key.id {
			token, err := grant.Sign(grantClaims(g, option), key.private)
			if err != nil {
				return nil, err
			}
			return &ResultGrant{Token: token, ExpiresAt: g.ExpiresAt}, nil
		}
	}
	d, err := a.Store.GetDecision(ctx, act.WorkspaceID, act.ID)
	if err != nil {
		return nil, err
	}
	var token string
	var g store.Grant
	err = a.Store.Tx(ctx, func(q *store.Queries) error {
		var err error
		token, g, err = a.issueGrant(ctx, q, act.WorkspaceID, act.ID, option, d.FinalTool, d.FinalArgs)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &ResultGrant{Token: token, ExpiresAt: g.ExpiresAt}, nil
}

// WaitForDecision blocks until the action is decided, the timeout passes, or
// the context ends, and returns the action's current state.
func (a *App) WaitForDecision(ctx context.Context, workspaceID, actionID string, timeout time.Duration) (store.Action, error) {
	if timeout > a.MaxWait {
		timeout = a.MaxWait
	}
	ch, cancel := a.Bus.Subscribe(workspaceID, func(e events.Event) bool {
		return e.ActionID == actionID && e.Kind == events.ActionDecided
	})
	defer cancel()
	act, err := a.Store.GetAction(ctx, workspaceID, actionID)
	if err != nil || act.Status.Final() {
		return act, mapStoreErr(err, "action")
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ch:
	case <-timer.C:
	case <-ctx.Done():
		return act, nil
	}
	act, err = a.Store.GetAction(ctx, workspaceID, actionID)
	return act, mapStoreErr(err, "action")
}

// GetResult returns an agent's view of one of its actions, optionally
// waiting for a decision.
func (a *App) GetResult(ctx context.Context, actor Actor, actionID string, wait time.Duration) (*ActionResult, error) {
	act, err := a.Store.GetAction(ctx, actor.WorkspaceID, actionID)
	if err != nil {
		return nil, mapStoreErr(err, "action")
	}
	if actor.Kind == ActorAgent && act.AgentID != actor.AgentID {
		return nil, notFoundf("action not found")
	}
	if actor.Kind != ActorAgent {
		if err := require(actor, domain.PermView); err != nil {
			return nil, err
		}
	}
	if act.Status == domain.StatusPendingReview && wait > 0 {
		if act, err = a.WaitForDecision(ctx, actor.WorkspaceID, actionID, wait); err != nil {
			return nil, err
		}
	}
	return a.Result(ctx, act)
}

func (a *App) webhookAction(act *store.Action, reasons []domain.Reason, deadline *time.Time) map[string]any {
	m := map[string]any{
		"id":          act.ID,
		"tool":        act.Tool,
		"agent_id":    act.AgentID,
		"agent_name":  act.AgentName,
		"principal":   act.Principal,
		"request":     truncate(act.Request, 500),
		"status":      act.Status,
		"source":      act.Source,
		"received_at": act.ReceivedAt,
	}
	if reasons != nil {
		m["hold_reasons"] = reasons
	}
	if deadline != nil {
		m["review_deadline"] = deadline
	}
	if u := a.reviewURL(act.ID); u != "" {
		m["review_url"] = u
	}
	return m
}

func modelSummary(ev evaluation) map[string]any {
	if ev.assessment == nil {
		m := map[string]any{"used": false}
		if ev.modelErr != nil {
			m["error"] = truncate(ev.modelErr.Error(), 200)
		}
		return m
	}
	as := ev.assessment
	m := map[string]any{
		"used": true, "pack": as.Pack, "pack_version": as.PackVersion, "choice": as.Choice,
		"confidence": as.Confidence, "risk": as.Risk, "guards": len(as.Guards),
	}
	if len(ev.calls) > 0 && ev.calls[0].Result != nil {
		m["model"] = ev.calls[0].Result.Model
		m["endpoint"] = ev.calls[0].Result.Endpoint
	}
	if as.Recheck != nil {
		m["recheck_agrees"] = as.Recheck.Agrees
	}
	return m
}

func optionSpec(options []domain.OptionSpec, o domain.Option) domain.OptionSpec {
	for _, s := range options {
		if s.Key == o {
			return s
		}
	}
	return domain.OptionSpec{Key: o}
}

func hasReason(rs []domain.Reason, code string) bool {
	for _, r := range rs {
		if r.Code == code {
			return true
		}
	}
	return false
}

// sampled decides deterministically whether an action is audited, so the
// choice can be reproduced from the action id alone.
func sampled(actionID string, rate float64) bool {
	if rate <= 0 {
		return false
	}
	if rate >= 1 {
		return true
	}
	h := sha256.Sum256([]byte("veto.audit.sample\n" + actionID))
	return float64(binary.BigEndian.Uint64(h[:8]))/float64(math.MaxUint64) < rate
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// modelAssessment returns the stored assessment of an action's main model
// call, if there was one.
func (a *App) modelAssessment(ctx context.Context, q *store.Queries, ws, actionID string) (*decide.Assessment, error) {
	evs, err := q.ListEvaluations(ctx, ws, actionID)
	if err != nil {
		return nil, err
	}
	for _, e := range evs {
		if e.Stage == decide.StageModel && e.Error == "" && len(e.Result) > 2 {
			var as decide.Assessment
			if err := json.Unmarshal(e.Result, &as); err != nil {
				return nil, fmt.Errorf("app: stored assessment: %w", err)
			}
			return &as, nil
		}
	}
	return nil, nil
}
