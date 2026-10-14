package app

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shvgroups/veto/server/internal/auth"
	"github.com/shvgroups/veto/server/internal/canonical"
	"github.com/shvgroups/veto/server/internal/connector"
	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/events"
	"github.com/shvgroups/veto/server/internal/grant"
	"github.com/shvgroups/veto/server/internal/id"
	"github.com/shvgroups/veto/server/internal/ledger"
	"github.com/shvgroups/veto/server/internal/notify"
	"github.com/shvgroups/veto/server/internal/store"
)

// Review channels, recorded with each human decision.
const (
	ChannelConsole = "console"
	ChannelSlack   = "slack"
	ChannelAPI     = "api"
)

// ReviewRequest is a reviewer's decision on a held action.
type ReviewRequest struct {
	Option domain.Option `json:"option"`
	Note   string        `json:"note"`
	// Password confirms the reviewer's identity for tools that require it.
	Password string `json:"password,omitempty"`
}

// Review decides a held action.
func (a *App) Review(ctx context.Context, actor Actor, actionID string, req ReviewRequest, channel string) (*store.Action, error) {
	if err := require(actor, domain.PermReview); err != nil {
		return nil, err
	}
	if !req.Option.Valid() {
		return nil, invalidf("option must be proceed, proceed_safer or stop")
	}
	if utf8.RuneCountInString(req.Note) > 2000 {
		return nil, invalidf("note must be at most 2000 characters")
	}
	ws, err := a.Store.GetWorkspace(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	var act store.Action
	err = a.Store.Tx(ctx, func(q *store.Queries) error {
		var err error
		act, err = q.GetActionForUpdate(ctx, ws.ID, actionID)
		if err != nil {
			return mapStoreErr(err, "action")
		}
		if act.Status != domain.StatusPendingReview {
			return conflictf("action is %s, not waiting for review", act.Status)
		}
		spec := domain.OptionSpec{Key: domain.OptionStop, Available: true}
		if req.Option != domain.OptionStop {
			var ok bool
			if spec, ok = act.Option(req.Option); !ok {
				return invalidf("option %s is not available for this action", req.Option)
			}
		}
		tool, err := q.GetTool(ctx, ws.ID, act.Tool)
		if errors.Is(err, store.ErrNotFound) {
			tool = domain.UnregisteredTool(ws.ID, act.Tool)
		} else if err != nil {
			return err
		}
		if tool.StepUp && req.Option != domain.OptionStop {
			if err := a.confirmPassword(ctx, q, actor, req.Password); err != nil {
				return err
			}
		}
		as, err := a.modelAssessment(ctx, q, ws.ID, act.ID)
		if err != nil {
			return err
		}
		secs := a.now().Sub(act.ReceivedAt).Seconds()
		d := &store.Decision{
			ActionID: act.ID, WorkspaceID: ws.ID, Option: req.Option, DecidedBy: domain.DecidedByHuman,
			Reasons:    []domain.Reason{{Code: domain.ReasonReviewerDecision, Detail: channel}},
			Thresholds: tool.Thresholds(), Note: req.Note, ReviewSeconds: &secs,
		}
		if actor.Kind == ActorUser {
			uid := actor.ID
			d.ReviewerID = &uid
		}
		if as != nil {
			conf, risk, choice := as.Confidence, as.Risk, string(as.Choice)
			d.Confidence, d.Risk, d.ModelChoice, d.ModelConfidence = &conf, &risk, &choice, &conf
		}
		return a.finalize(ctx, q, ws, &act, d, spec, actor, map[string]any{"channel": channel, "held_seconds": secs})
	})
	if err != nil {
		return nil, err
	}
	a.Metrics.Reviews.Inc(string(req.Option), channel)
	go a.notifySlackResolved(ws.ID, actionID)
	return &act, nil
}

func (a *App) confirmPassword(ctx context.Context, q *store.Queries, actor Actor, password string) error {
	if actor.Kind != ActorUser {
		return forbiddenf("this tool needs a reviewer to confirm with their password in the console")
	}
	if password == "" {
		return &Error{Code: CodeForbidden, Message: "this tool needs you to confirm with your password", Details: map[string]any{"step_up": true}}
	}
	u, err := q.GetUser(ctx, actor.ID)
	if err != nil {
		return err
	}
	if _, err := auth.CheckPassword(password, u.PasswordHash); err != nil {
		return &Error{Code: CodeForbidden, Message: "password is incorrect", Details: map[string]any{"step_up": true}}
	}
	return nil
}

// Expire stops a held action whose review deadline has passed. It does
// nothing if the action was decided in the meantime.
func (a *App) Expire(ctx context.Context, ref store.PendingRef) error {
	ws, err := a.Store.GetWorkspace(ctx, ref.WorkspaceID)
	if err != nil {
		return err
	}
	expired := false
	err = a.Store.Tx(ctx, func(q *store.Queries) error {
		act, err := q.GetActionForUpdate(ctx, ws.ID, ref.ActionID)
		if err != nil {
			return err
		}
		if act.Status != domain.StatusPendingReview || act.ReviewDeadline == nil || act.ReviewDeadline.After(a.now()) {
			return nil
		}
		as, err := a.modelAssessment(ctx, q, ws.ID, act.ID)
		if err != nil {
			return err
		}
		d := &store.Decision{
			ActionID: act.ID, WorkspaceID: ws.ID, Option: domain.OptionStop, DecidedBy: domain.DecidedByExpiry,
			Reasons: []domain.Reason{{Code: domain.ReasonReviewExpired, Detail: ws.Settings.ReviewTimeout().String()}},
		}
		if as != nil {
			choice, conf := string(as.Choice), as.Confidence
			d.ModelChoice, d.ModelConfidence = &choice, &conf
		}
		expired = true
		return a.finalize(ctx, q, ws, &act, d, domain.OptionSpec{Key: domain.OptionStop}, System(ws.ID), nil)
	})
	if err == nil && expired {
		a.Metrics.Actions.Inc("expiry", "human", "expired")
		go a.notifySlackResolved(ws.ID, ref.ActionID)
	}
	return err
}

// RecordAudit stores a reviewer's verdict on an automatic decision.
func (a *App) RecordAudit(ctx context.Context, actor Actor, auditID string, verdict domain.AuditVerdict, note string) (store.Audit, error) {
	if err := require(actor, domain.PermReview); err != nil {
		return store.Audit{}, err
	}
	if !verdict.Valid() {
		return store.Audit{}, invalidf("verdict must be correct, should_be_safer, should_hold or should_block")
	}
	if utf8.RuneCountInString(note) > 2000 {
		return store.Audit{}, invalidf("note must be at most 2000 characters")
	}
	var out store.Audit
	err := a.Store.Tx(ctx, func(q *store.Queries) error {
		var err error
		out, err = q.RecordAuditVerdict(ctx, actor.WorkspaceID, auditID, verdict, actor.ID, note)
		if err != nil {
			return mapStoreErr(err, "audit")
		}
		if err := record(ctx, q, actor.WorkspaceID, ledger.KindAuditRecorded, out.ActionID, actor, map[string]any{
			"audit_id": auditID, "verdict": verdict, "note": note,
		}); err != nil {
			return err
		}
		return publish(ctx, q, events.Event{WorkspaceID: actor.WorkspaceID, Kind: events.AuditRecorded, ActionID: out.ActionID})
	})
	return out, err
}

// OutcomeRequest reports what happened after an action ran.
type OutcomeRequest struct {
	Status domain.OutcomeStatus `json:"status"`
	Detail json.RawMessage      `json:"detail,omitempty"`
}

// ReportOutcome records an outcome. Bad outcomes of automatic decisions are
// queued for audit so a person looks at them.
func (a *App) ReportOutcome(ctx context.Context, actor Actor, actionID string, req OutcomeRequest) (store.Outcome, error) {
	if !req.Status.Valid() {
		return store.Outcome{}, invalidf("status must be succeeded, failed, reversed or complaint")
	}
	if len(req.Detail) > 16<<10 {
		return store.Outcome{}, invalidf("detail must be at most 16 KiB")
	}
	if len(req.Detail) > 0 {
		v, err := canonical.Decode(req.Detail)
		if err != nil {
			return store.Outcome{}, invalidf("detail must be valid JSON")
		}
		if _, ok := v.(map[string]any); !ok {
			return store.Outcome{}, invalidf("detail must be a JSON object")
		}
	}
	act, err := a.Store.GetAction(ctx, actor.WorkspaceID, actionID)
	if err != nil {
		return store.Outcome{}, mapStoreErr(err, "action")
	}
	switch actor.Kind {
	case ActorAgent:
		if act.AgentID != actor.AgentID {
			return store.Outcome{}, notFoundf("action not found")
		}
	default:
		if err := require(actor, domain.PermReview); err != nil {
			return store.Outcome{}, err
		}
	}
	if act.Status != domain.StatusApproved {
		return store.Outcome{}, conflictf("outcomes can only be reported for approved actions; this one is %s", act.Status)
	}
	o := store.Outcome{ID: id.New(id.Outcome), ActionID: act.ID, WorkspaceID: act.WorkspaceID, Status: req.Status, Detail: req.Detail, ReportedBy: actor.String()}
	err = a.Store.Tx(ctx, func(q *store.Queries) error {
		return a.saveOutcome(ctx, q, &act, &o, actor)
	})
	if err != nil {
		return store.Outcome{}, err
	}
	a.Metrics.Outcomes.Inc(string(req.Status))
	return o, nil
}

func (a *App) saveOutcome(ctx context.Context, q *store.Queries, act *store.Action, o *store.Outcome, actor Actor) error {
	if err := q.InsertOutcome(ctx, o); err != nil {
		return err
	}
	if err := record(ctx, q, act.WorkspaceID, ledger.KindOutcomeReported, act.ID, actor, map[string]any{
		"outcome_id": o.ID, "status": o.Status,
	}); err != nil {
		return err
	}
	if o.Status != domain.OutcomeSucceeded {
		d, err := q.GetDecision(ctx, act.WorkspaceID, act.ID)
		if err != nil {
			return err
		}
		if d.DecidedBy != domain.DecidedByHuman {
			if err := a.queueAudit(ctx, q, act.WorkspaceID, act.ID, "outcome"); err != nil {
				return err
			}
		}
	}
	if err := enqueueWebhook(ctx, q, act.WorkspaceID, notify.EventOutcome, map[string]any{
		"action_id": act.ID, "tool": act.Tool, "status": o.Status, "detail": o.Detail,
	}); err != nil {
		return err
	}
	return publish(ctx, q, events.Event{WorkspaceID: act.WorkspaceID, Kind: events.OutcomeReported, ActionID: act.ID, Status: string(o.Status)})
}

// VerifyRequest asks whether a grant authorizes a specific call.
type VerifyRequest struct {
	Token   string          `json:"token"`
	Tool    string          `json:"tool"`
	Args    json.RawMessage `json:"args"`
	Consume bool            `json:"consume"`
}

// VerifyResult describes a verified grant.
type VerifyResult struct {
	Valid     bool          `json:"valid"`
	GrantID   string        `json:"grant_id"`
	ActionID  string        `json:"action_id"`
	Option    domain.Option `json:"option"`
	ExpiresAt time.Time     `json:"expires_at"`
	Consumed  bool          `json:"consumed"`
}

// VerifyGrant checks that a grant covers exactly this tool and arguments,
// and with Consume marks it used so it cannot authorize a second run.
func (a *App) VerifyGrant(ctx context.Context, actor Actor, req VerifyRequest) (*VerifyResult, error) {
	if actor.Kind == ActorUser {
		return nil, forbiddenf("grants are verified with an API key")
	}
	claims, err := grant.Parse(req.Token)
	if err != nil {
		a.Metrics.Grants.Inc("malformed")
		return nil, invalidf("grant token is malformed")
	}
	if claims.WorkspaceID != actor.WorkspaceID {
		a.Metrics.Grants.Inc("wrong_workspace")
		return nil, invalidf("grant was not issued by this workspace")
	}
	sk, err := a.Store.SigningKeyByKeyID(ctx, actor.WorkspaceID, claims.KeyID)
	if err != nil {
		a.Metrics.Grants.Inc("unknown_key")
		return nil, invalidf("grant was signed by an unknown key")
	}
	if len(req.Args) == 0 {
		req.Args = json.RawMessage("{}")
	}
	hash, err := canonical.ArgsHashJSON(req.Tool, req.Args)
	if err != nil {
		return nil, invalidf("args must be valid JSON")
	}
	if _, err := grant.Verify(req.Token, sk.PublicKey, req.Tool, hash, a.now()); err != nil {
		var reason string
		switch {
		case errors.Is(err, grant.ErrExpired):
			reason = "expired"
		case errors.Is(err, grant.ErrMismatch):
			reason = "mismatch"
		default:
			reason = "bad_signature"
		}
		a.Metrics.Grants.Inc(reason)
		return nil, &Error{Code: CodeForbidden, Message: "grant does not authorize this call: " + strings.TrimPrefix(err.Error(), "grant: "), Details: map[string]any{"reason": reason}}
	}
	res := &VerifyResult{Valid: true, GrantID: claims.ID, ActionID: claims.ActionID, Option: claims.Option, ExpiresAt: claims.Expires()}
	if req.Consume {
		err := a.Store.Tx(ctx, func(q *store.Queries) error {
			if err := q.ConsumeGrant(ctx, actor.WorkspaceID, claims.ID); err != nil {
				if errors.Is(err, store.ErrConflict) {
					return conflictf("grant was already used, or this action already ran")
				}
				return mapStoreErr(err, "grant")
			}
			return record(ctx, q, actor.WorkspaceID, ledger.KindGrantConsumed, claims.ActionID, actor, map[string]any{
				"grant_id": claims.ID, "tool": req.Tool, "args_hash": hex.EncodeToString(hash),
			})
		})
		if err != nil {
			a.Metrics.Grants.Inc("already_used")
			return nil, err
		}
		res.Consumed = true
	}
	a.Metrics.Grants.Inc("valid")
	return res, nil
}

// Execute runs an approved connection call through Veto, attaching the
// stored credential, and records the result as the action's outcome.
func (a *App) Execute(ctx context.Context, actor Actor, actionID string) (*connector.Response, error) {
	if actor.Kind != ActorAgent {
		return nil, forbiddenf("only agent API keys can execute actions")
	}
	act, err := a.Store.GetAction(ctx, actor.WorkspaceID, actionID)
	if err != nil || act.AgentID != actor.AgentID {
		return nil, notFoundf("action not found")
	}
	if act.Status != domain.StatusApproved {
		return nil, conflictf("action is %s, not approved", act.Status)
	}
	d, err := a.Store.GetDecision(ctx, act.WorkspaceID, act.ID)
	if err != nil {
		return nil, err
	}
	name, ok := strings.CutPrefix(d.FinalTool, connector.ToolPrefix)
	if !ok {
		return nil, invalidf("only %s<connection> tools are executed by Veto; run %s yourself with the grant", connector.ToolPrefix, d.FinalTool)
	}
	conn, err := a.Store.GetConnectionByName(ctx, act.WorkspaceID, name)
	if err != nil {
		return nil, mapStoreErr(err, "connection "+name)
	}
	if conn.DisabledAt != nil {
		return nil, conflictf("connection %s is disabled", name)
	}
	exec := a.Connector
	if conn.AllowPrivate {
		if a.PrivateConnector == nil {
			return nil, conflictf("connection %s targets private addresses, which this server does not allow", name)
		}
		exec = a.PrivateConnector
	}
	req, err := connector.ParseRequest(d.FinalArgs)
	if err != nil {
		return nil, invalidf("%v", err)
	}
	var secret []byte
	if len(conn.SecretEnc) > 0 {
		if secret, err = a.Keyring.Open(conn.SecretEnc, "connection:"+conn.ID); err != nil {
			return nil, fmt.Errorf("app: open connection secret: %w", err)
		}
	}

	// Claim the action before calling out, so a retry cannot run it twice.
	if err := a.claimRun(ctx, actor, act, d, "execute"); err != nil {
		return nil, err
	}

	resp, callErr := exec.Do(ctx, connector.Connection{
		Name: conn.Name, BaseURL: conn.BaseURL, AuthScheme: conn.AuthScheme, HeaderName: conn.HeaderName,
		Secret: secret, AllowMethods: conn.AllowMethods,
	}, req)

	status := domain.OutcomeSucceeded
	detail := map[string]any{"connection": conn.Name, "method": req.Method, "path": req.Path}
	if callErr != nil {
		status = domain.OutcomeFailed
		detail["error"] = truncate(callErr.Error(), 500)
		a.Metrics.Executions.Inc("error")
	} else {
		detail["status"] = resp.Status
		detail["latency_ms"] = resp.LatencyMs
		if resp.Status >= 400 {
			status = domain.OutcomeFailed
		}
		a.Metrics.Executions.Inc(fmt.Sprintf("%dxx", resp.Status/100))
	}
	rawDetail, _ := json.Marshal(detail)
	o := store.Outcome{ID: id.New(id.Outcome), ActionID: act.ID, WorkspaceID: act.WorkspaceID, Status: status, Detail: rawDetail, ReportedBy: "veto:connector"}
	err = a.Store.Tx(ctx, func(q *store.Queries) error {
		if err := record(ctx, q, act.WorkspaceID, ledger.KindActionExecuted, act.ID, actor, detail); err != nil {
			return err
		}
		return a.saveOutcome(ctx, q, &act, &o, System(act.WorkspaceID))
	})
	if err != nil {
		a.Log.Error("could not record execution outcome", "action", act.ID, "error", err)
	}
	if callErr != nil {
		return nil, &Error{Code: CodeUpstream, Message: callErr.Error()}
	}
	return resp, nil
}

// SlackInteraction handles a button press from Slack. It returns a message
// to show the reviewer when the press could not be applied.
func (a *App) SlackInteraction(ctx context.Context, workspaceID string, headers http.Header, body []byte) (string, error) {
	integ, err := a.Store.GetIntegration(ctx, workspaceID, "slack")
	if err != nil {
		return "", notFoundf("Slack is not configured for this workspace")
	}
	secrets, err := a.slackSecrets(workspaceID, integ)
	if err != nil {
		return "", err
	}
	if err := notify.VerifySlackRequest(secrets.SigningSecret, headers, body, a.now()); err != nil {
		return "", &Error{Code: CodeUnauthorized, Message: err.Error()}
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return "", invalidf("malformed Slack payload")
	}
	in, ok, err := notify.ParseInteraction(form)
	if err != nil {
		return "", invalidf("%v", err)
	}
	if !ok {
		return "", nil
	}
	email, err := a.Slack.UserEmail(ctx, secrets.BotToken, in.UserID)
	if err != nil {
		return "Veto could not read your Slack email address, so it cannot match you to a Veto account.", nil
	}
	u, err := a.Store.GetUserByEmail(ctx, email)
	if err != nil || u.DisabledAt != nil {
		return fmt.Sprintf("No active Veto account uses %s.", email), nil
	}
	m, err := a.Store.GetMember(ctx, workspaceID, u.ID)
	if err != nil {
		return fmt.Sprintf("%s is not a member of this Veto workspace.", email), nil
	}
	actor := Actor{Kind: ActorUser, ID: u.ID, Name: u.Name, WorkspaceID: workspaceID, Role: m.Role}
	if _, err := a.Review(ctx, actor, in.ActionID, ReviewRequest{Option: domain.Option(in.Option)}, ChannelSlack); err != nil {
		var ae *Error
		if errors.As(err, &ae) {
			return ae.Message, nil
		}
		return "", err
	}
	return "", nil
}

func (a *App) slackSecrets(workspaceID string, integ store.Integration) (notify.SlackSecrets, error) {
	var s notify.SlackSecrets
	if len(integ.SecretEnc) == 0 {
		return s, conflictf("Slack credentials are missing")
	}
	raw, err := a.Keyring.Open(integ.SecretEnc, "integration:"+workspaceID+":slack")
	if err != nil {
		return s, fmt.Errorf("app: open Slack secrets: %w", err)
	}
	return s, json.Unmarshal(raw, &s)
}

func (a *App) notifySlackHeld(workspaceID, actionID string) {
	if a.Slack == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	integ, err := a.Store.GetIntegration(ctx, workspaceID, "slack")
	if err != nil {
		return
	}
	var settings notify.SlackSettings
	if err := json.Unmarshal(integ.Config, &settings); err != nil || settings.Channel == "" {
		return
	}
	secrets, err := a.slackSecrets(workspaceID, integ)
	if err != nil {
		a.Log.Warn("slack secrets unavailable", "workspace", workspaceID, "error", err)
		return
	}
	act, err := a.Store.GetAction(ctx, workspaceID, actionID)
	if err != nil || act.Status != domain.StatusPendingReview {
		return
	}
	msg := a.heldMessage(act)
	msg.Mention = settings.MentionHeld
	ts, err := a.Slack.PostHeld(ctx, secrets.BotToken, settings.Channel, msg)
	if err != nil {
		a.Log.Warn("could not post held action to Slack", "workspace", workspaceID, "action", actionID, "error", err)
		return
	}
	if err := a.Store.SaveSlackMessage(ctx, workspaceID, actionID, settings.Channel, ts); err != nil {
		a.Log.Warn("could not save Slack message", "action", actionID, "error", err)
	}
}

func (a *App) notifySlackResolved(workspaceID, actionID string) {
	if a.Slack == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	channel, ts, err := a.Store.GetSlackMessage(ctx, workspaceID, actionID)
	if err != nil {
		return
	}
	integ, err := a.Store.GetIntegration(ctx, workspaceID, "slack")
	if err != nil {
		return
	}
	secrets, err := a.slackSecrets(workspaceID, integ)
	if err != nil {
		return
	}
	act, err := a.Store.GetAction(ctx, workspaceID, actionID)
	if err != nil {
		return
	}
	d, err := a.Store.GetDecision(ctx, workspaceID, actionID)
	if err != nil {
		return
	}
	var resolution string
	switch d.DecidedBy {
	case domain.DecidedByHuman:
		who := "a reviewer"
		if d.ReviewerName != nil {
			who = *d.ReviewerName
		}
		resolution = fmt.Sprintf("*%s* chose *%s*.", who, OptionLabel(d.Option))
	case domain.DecidedByExpiry:
		resolution = "*Stopped*: nobody decided before the deadline."
	default:
		resolution = fmt.Sprintf("Resolved: *%s*.", OptionLabel(d.Option))
	}
	if err := a.Slack.UpdateResolved(ctx, secrets.BotToken, channel, ts, a.heldMessage(act), resolution); err != nil {
		a.Log.Warn("could not update Slack message", "action", actionID, "error", err)
	}
}

func (a *App) heldMessage(act store.Action) notify.HeldMessage {
	msg := notify.HeldMessage{
		ActionID: act.ID, AgentName: act.AgentName, Tool: act.Tool, Principal: act.Principal.Display,
		Request: act.Request, ReviewURL: a.reviewURL(act.ID),
	}
	if act.ReviewDeadline != nil {
		msg.Deadline = *act.ReviewDeadline
	}
	for _, r := range act.HoldReasons {
		msg.Reasons = append(msg.Reasons, ReasonLabel(r))
	}
	for i, o := range act.Ranking {
		spec, ok := act.Option(o)
		if o == domain.OptionStop {
			ok = true
		}
		if !ok {
			continue
		}
		msg.Options = append(msg.Options, notify.SlackOption{
			Key: string(o), Label: OptionLabel(o), Changes: spec.Changes, Recommended: i == 0,
		})
	}
	return msg
}

// OptionLabel names an option for people.
func OptionLabel(o domain.Option) string {
	switch o {
	case domain.OptionProceed:
		return "Run as proposed"
	case domain.OptionProceedSafer:
		return "Run the safer version"
	default:
		return "Stop"
	}
}

// ReasonLabel describes a reason code for people.
func ReasonLabel(r domain.Reason) string {
	labels := map[string]string{
		domain.ReasonPolicyForbid:      "blocked by policy",
		domain.ReasonPolicyReview:      "policy requires review",
		domain.ReasonPolicyAllow:       "allowed by policy",
		domain.ReasonRateLimited:       "over the hourly limit",
		domain.ReasonModelUnavailable:  "decision model unavailable",
		domain.ReasonModelDisabled:     "no decision model configured",
		domain.ReasonFailOpen:          "allowed because the tool fails open",
		domain.ReasonLowConfidence:     "model not confident enough",
		domain.ReasonRiskAboveCeiling:  "risk above the tool's ceiling",
		domain.ReasonGuard:             "guard",
		domain.ReasonRecheckDisagreed:  "double check disagreed",
		domain.ReasonCriticalTool:      "critical tool",
		domain.ReasonUnregisteredTool:  "tool not yet classified",
		domain.ReasonConfident:         "model confident",
		domain.ReasonAgentDisabled:     "agent disabled",
		domain.ReasonReviewerDecision:  "reviewer decision",
		domain.ReasonReviewExpired:     "review expired",
		domain.ReasonSaferUnavailable:  "safer version unavailable",
		domain.ReasonOptionUnavailable: "option unavailable",
	}
	label, ok := labels[r.Code]
	if !ok {
		label = strings.ReplaceAll(r.Code, "_", " ")
	}
	if r.Code == domain.ReasonGuard && r.Detail != "" {
		return r.Detail
	}
	if r.Detail != "" && (r.Code == domain.ReasonPolicyForbid || r.Code == domain.ReasonPolicyReview) {
		return label + " (" + r.Detail + ")"
	}
	return label
}

// Remind re-announces a held action halfway to its deadline by sending its
// action.pending_review webhook again with "reminder": true.
func (a *App) Remind(ctx context.Context, ref store.PendingRef) error {
	return a.Store.Tx(ctx, func(q *store.Queries) error {
		act, err := q.GetActionForUpdate(ctx, ref.WorkspaceID, ref.ActionID)
		if err != nil {
			return err
		}
		if act.Status != domain.StatusPendingReview {
			return nil
		}
		if err := q.MarkReminded(ctx, act.WorkspaceID, act.ID); err != nil {
			return err
		}
		data := a.webhookAction(&act, act.HoldReasons, act.ReviewDeadline)
		data["reminder"] = true
		return enqueueWebhook(ctx, q, act.WorkspaceID, notify.EventActionPending, data)
	})
}

// claimRun consumes the action's grant on Veto's own behalf before Veto
// performs the call itself (connections and the MCP proxy). It fails with a
// conflict if the action already ran.
func (a *App) claimRun(ctx context.Context, actor Actor, act store.Action, d store.Decision, via string) error {
	return a.Store.Tx(ctx, func(q *store.Queries) error {
		grants, err := q.ListGrants(ctx, act.WorkspaceID, act.ID)
		if err != nil {
			return err
		}
		for _, g := range grants {
			if g.ConsumedAt != nil {
				return conflictf("action already ran")
			}
		}
		var gid string
		for i := len(grants) - 1; i >= 0; i-- {
			if grants[i].ExpiresAt.After(a.now()) {
				gid = grants[i].ID
				break
			}
		}
		if gid == "" {
			_, g, err := a.issueGrant(ctx, q, act.WorkspaceID, act.ID, d.Option, d.FinalTool, d.FinalArgs)
			if err != nil {
				return err
			}
			gid = g.ID
		}
		if err := q.ConsumeGrant(ctx, act.WorkspaceID, gid); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return conflictf("action already ran")
			}
			return err
		}
		return record(ctx, q, act.WorkspaceID, ledger.KindGrantConsumed, act.ID, actor, map[string]any{"grant_id": gid, "via": via})
	})
}

// ClaimProxiedRun prepares an approved action for the MCP proxy to forward:
// it returns the decided tool and arguments and consumes the grant.
func (a *App) ClaimProxiedRun(ctx context.Context, actor Actor, actionID string) (store.Decision, error) {
	act, err := a.Store.GetAction(ctx, actor.WorkspaceID, actionID)
	if err != nil || act.AgentID != actor.AgentID {
		return store.Decision{}, notFoundf("action not found")
	}
	if act.Status != domain.StatusApproved {
		return store.Decision{}, conflictf("action is %s, not approved", act.Status)
	}
	d, err := a.Store.GetDecision(ctx, act.WorkspaceID, act.ID)
	if err != nil {
		return d, err
	}
	return d, a.claimRun(ctx, actor, act, d, "mcp_proxy")
}

// RecordProxiedOutcome records what an upstream server returned for a call
// Veto forwarded.
func (a *App) RecordProxiedOutcome(ctx context.Context, actor Actor, actionID string, status domain.OutcomeStatus, detail map[string]any) error {
	act, err := a.Store.GetAction(ctx, actor.WorkspaceID, actionID)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(detail)
	o := store.Outcome{ID: id.New(id.Outcome), ActionID: act.ID, WorkspaceID: act.WorkspaceID, Status: status, Detail: raw, ReportedBy: "veto:mcp"}
	err = a.Store.Tx(ctx, func(q *store.Queries) error {
		if err := record(ctx, q, act.WorkspaceID, ledger.KindActionExecuted, act.ID, actor, detail); err != nil {
			return err
		}
		return a.saveOutcome(ctx, q, &act, &o, System(act.WorkspaceID))
	})
	if err == nil {
		a.Metrics.Outcomes.Inc(string(status))
	}
	return err
}

// MCPUpstream returns an MCP server's upstream URL and the credential to send
// to it, if the server is bound to a connection.
func (a *App) MCPUpstream(ctx context.Context, actor Actor, name string) (store.MCPServer, *connector.Connection, error) {
	m, err := a.Store.GetMCPServerByName(ctx, actor.WorkspaceID, name)
	if err != nil || m.DisabledAt != nil {
		return m, nil, notFoundf("MCP server %q not found", name)
	}
	if m.ConnectionID == nil {
		return m, nil, nil
	}
	c, err := a.Store.GetConnection(ctx, actor.WorkspaceID, *m.ConnectionID)
	if err != nil {
		return m, nil, err
	}
	conn := &connector.Connection{Name: c.Name, BaseURL: c.BaseURL, AuthScheme: c.AuthScheme, HeaderName: c.HeaderName}
	if len(c.SecretEnc) > 0 {
		if conn.Secret, err = a.Keyring.Open(c.SecretEnc, "connection:"+c.ID); err != nil {
			return m, nil, fmt.Errorf("app: open connection secret: %w", err)
		}
	}
	return m, conn, nil
}
