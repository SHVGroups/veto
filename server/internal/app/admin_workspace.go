package app

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shvgroups/veto/server/internal/decide"
	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/ledger"
	"github.com/shvgroups/veto/server/internal/notify"
	"github.com/shvgroups/veto/server/internal/store"
)

// WorkspaceInput updates the workspace's name and settings.
type WorkspaceInput struct {
	Name     string          `json:"name"`
	Settings domain.Settings `json:"settings"`
}

// UpdateWorkspace changes the workspace name and settings.
func (a *App) UpdateWorkspace(ctx context.Context, actor Actor, in WorkspaceInput) (store.Workspace, error) {
	if err := require(actor, domain.PermManageMembers); err != nil {
		return store.Workspace{}, err
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 100 {
		return store.Workspace{}, invalidf("workspace name must be 1 to 100 characters")
	}
	if err := in.Settings.Validate(); err != nil {
		return store.Workspace{}, invalidf("%v", err)
	}
	err := a.Store.Tx(ctx, func(q *store.Queries) error {
		before, err := q.GetWorkspace(ctx, actor.WorkspaceID)
		if err != nil {
			return err
		}
		if err := q.UpdateWorkspace(ctx, actor.WorkspaceID, in.Name, in.Settings); err != nil {
			return err
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindSettingsUpdated, "", actor, map[string]any{
			"name": in.Name, "settings": in.Settings, "previous": before.Settings,
		})
	})
	if err != nil {
		return store.Workspace{}, err
	}
	return a.Store.GetWorkspace(ctx, actor.WorkspaceID)
}

// Range names the time windows the dashboard offers.
type Range string

// Window returns the start, end and bucket size of a range ending now.
func (r Range) Window(now time.Time) (from, to time.Time, bucket time.Duration, err error) {
	to = now.Add(time.Minute).Truncate(time.Minute)
	switch r {
	case "1h":
		return to.Add(-time.Hour), to, 5 * time.Minute, nil
	case "24h", "":
		return to.Add(-24 * time.Hour), to, time.Hour, nil
	case "7d":
		return to.Add(-7 * 24 * time.Hour), to, 6 * time.Hour, nil
	case "30d":
		return to.Add(-30 * 24 * time.Hour), to, 24 * time.Hour, nil
	case "90d":
		return to.Add(-90 * 24 * time.Hour), to, 24 * time.Hour, nil
	}
	return from, to, 0, invalidf("range must be 1h, 24h, 7d, 30d or 90d")
}

// Proportion is a rate with a 95% Wilson score interval.
type Proportion struct {
	Value float64 `json:"value"`
	Low   float64 `json:"low"`
	High  float64 `json:"high"`
	N     int     `json:"n"`
}

// wilson returns the proportion k/n with its 95% interval, or nil for n = 0.
func wilson(k, n int) *Proportion {
	if n == 0 {
		return nil
	}
	const z = 1.959963984540054
	p := float64(k) / float64(n)
	nf := float64(n)
	denom := 1 + z*z/nf
	center := (p + z*z/(2*nf)) / denom
	half := z * math.Sqrt(p*(1-p)/nf+z*z/(4*nf*nf)) / denom
	return &Proportion{Value: p, Low: math.Max(0, center-half), High: math.Min(1, center+half), N: n}
}

func ratio(k, n int) *float64 {
	if n == 0 {
		return nil
	}
	v := float64(k) / float64(n)
	return &v
}

// OverviewRates are the dashboard's headline rates.
type OverviewRates struct {
	AutoResolution *float64    `json:"auto_resolution"`
	HoldRate       *float64    `json:"hold_rate"`
	SaferTake      *float64    `json:"safer_take"`
	SaferOffered   *float64    `json:"safer_offered"`
	FalseApproval  *Proportion `json:"false_approval"`
	ModelErrorRate *float64    `json:"model_error_rate"`
	StopRate       *float64    `json:"stop_rate"`
}

// OverviewView is the overview page's data.
type OverviewView struct {
	*store.Overview
	Range Range         `json:"range"`
	Rates OverviewRates `json:"rates"`
	Model ModelStatus   `json:"model"`
}

// ModelStatus describes the decision model configuration.
type ModelStatus struct {
	Configured bool   `json:"configured"`
	Name       string `json:"name"`
}

// Overview computes the dashboard's overview for a range.
func (a *App) Overview(ctx context.Context, actor Actor, r Range) (*OverviewView, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	from, to, bucket, err := r.Window(a.now())
	if err != nil {
		return nil, err
	}
	o, err := a.Store.Overview(ctx, actor.WorkspaceID, from, to, bucket)
	if err != nil {
		return nil, err
	}
	c := o.Counts
	if r == "" {
		r = "24h"
	}
	return &OverviewView{
		Overview: o,
		Range:    r,
		Model:    ModelStatus{Configured: a.ModelConfigured(), Name: a.ModelName},
		Rates: OverviewRates{
			AutoResolution: ratio(c.ByModel+c.ByPolicy+c.ByFailsafe, c.Decided),
			HoldRate:       ratio(c.Held, c.Received),
			SaferTake:      ratio(c.OptSafer, c.Approved),
			SaferOffered:   ratio(c.OptSafer, c.SaferOffered),
			FalseApproval:  wilson(c.AuditsWrong, c.AuditsReviewed),
			ModelErrorRate: ratio(c.ModelErrors, c.ModelCalls),
			StopRate:       ratio(c.OptStop, c.Decided),
		},
	}, nil
}

// CalibrationBand is one tenth of the confidence scale.
type CalibrationBand struct {
	Low       float64     `json:"low"`
	High      float64     `json:"high"`
	Labeled   int         `json:"labeled"`
	Agreed    int         `json:"agreed"`
	Auto      int         `json:"auto"`
	Held      int         `json:"held"`
	Agreement *Proportion `json:"agreement"`
}

// ToolCalibration is the calibration report for one tool.
type ToolCalibration struct {
	Tool      string            `json:"tool"`
	RiskClass domain.RiskClass  `json:"risk_class"`
	Current   domain.Thresholds `json:"current"`
	Labeled   int               `json:"labeled"`
	Agreed    int               `json:"agreed"`
	Agreement *Proportion       `json:"agreement"`
	Bands     []CalibrationBand `json:"bands"`
	// Suggested is the lowest auto-confidence threshold whose decisions,
	// taken together, agree with people at the workspace's target rate with
	// 95% confidence. Nil when there is not enough evidence.
	Suggested *float64 `json:"suggested"`
	Note      string   `json:"note"`
}

// CalibrationView is the calibration page's data.
type CalibrationView struct {
	Range   Range             `json:"range"`
	Target  float64           `json:"target"`
	MinN    int               `json:"min_n"`
	Overall []CalibrationBand `json:"overall"`
	Tools   []ToolCalibration `json:"tools"`
}

// calibrationMinN is how many labelled decisions a band of confidence must
// hold before Veto suggests a threshold from it.
const calibrationMinN = 30

// Calibration reports how often the model's choice matched people's, by
// tool and confidence, and suggests evidence-backed thresholds.
func (a *App) Calibration(ctx context.Context, actor Actor, r Range, tool string) (*CalibrationView, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	if r == "" {
		r = "30d"
	}
	from, to, _, err := r.Window(a.now())
	if err != nil {
		return nil, err
	}
	ws, err := a.Store.GetWorkspace(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	bins, err := a.Store.CalibrationBins(ctx, actor.WorkspaceID, from, to, tool)
	if err != nil {
		return nil, err
	}
	tools, err := a.Store.ListTools(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	profiles := map[string]domain.Tool{}
	for _, t := range tools {
		profiles[t.Name] = t.Tool
	}

	view := &CalibrationView{Range: r, Target: ws.Settings.CalibrationTarget, MinN: calibrationMinN, Overall: emptyBands(), Tools: []ToolCalibration{}}
	byTool := map[string]*ToolCalibration{}
	var order []string
	for _, b := range bins {
		tc, ok := byTool[b.Tool]
		if !ok {
			p, known := profiles[b.Tool]
			if !known {
				p = domain.UnregisteredTool(actor.WorkspaceID, b.Tool)
			}
			tc = &ToolCalibration{Tool: b.Tool, RiskClass: p.RiskClass, Current: p.Thresholds(), Bands: emptyBands()}
			byTool[b.Tool] = tc
			order = append(order, b.Tool)
		}
		if b.Bin < 0 || b.Bin > 9 {
			continue
		}
		for _, bands := range [][]CalibrationBand{tc.Bands, view.Overall} {
			band := &bands[b.Bin]
			band.Labeled += b.Labeled
			band.Agreed += b.Agreed
			band.Auto += b.Auto
			band.Held += b.Held
		}
		tc.Labeled += b.Labeled
		tc.Agreed += b.Agreed
	}
	for i := range view.Overall {
		view.Overall[i].Agreement = wilson(view.Overall[i].Agreed, view.Overall[i].Labeled)
	}
	slices.Sort(order)
	for _, name := range order {
		tc := byTool[name]
		tc.Agreement = wilson(tc.Agreed, tc.Labeled)
		for i := range tc.Bands {
			tc.Bands[i].Agreement = wilson(tc.Bands[i].Agreed, tc.Bands[i].Labeled)
		}
		tc.Suggested, tc.Note = suggestThreshold(tc.Bands, view.Target)
		view.Tools = append(view.Tools, *tc)
	}
	return view, nil
}

func emptyBands() []CalibrationBand {
	bands := make([]CalibrationBand, 10)
	for i := range bands {
		bands[i].Low, bands[i].High = float64(i)/10, float64(i+1)/10
	}
	return bands
}

// suggestThreshold walks down from the most confident band, accumulating
// labelled decisions, and returns the lowest band edge where the
// accumulated agreement's lower 95% bound still meets the target.
func suggestThreshold(bands []CalibrationBand, target float64) (*float64, string) {
	labeled, agreed := 0, 0
	var best *float64
	for i := len(bands) - 1; i >= 0; i-- {
		labeled += bands[i].Labeled
		agreed += bands[i].Agreed
		if labeled < calibrationMinN {
			continue
		}
		p := wilson(agreed, labeled)
		if p.Low >= target {
			v := bands[i].Low
			best = &v
		} else {
			break
		}
	}
	if best != nil {
		return best, "Decisions at or above this confidence agreed with reviewers at the target rate."
	}
	if labeled < calibrationMinN {
		return nil, "Not enough labelled decisions yet. Reviews and audits add labels."
	}
	return nil, "Reviewers disagreed too often to suggest a lower threshold. Keep the current one."
}

// VerifyReport is the result of checking a workspace's ledger.
type VerifyReport struct {
	Entries    int64          `json:"entries"`
	HeadSeq    int64          `json:"head_seq"`
	HeadHash   string         `json:"head_hash"`
	Break      *ledger.Break  `json:"break"`
	Checkpoint *CheckpointRef `json:"checkpoint"`
	VerifiedAt time.Time      `json:"verified_at"`
	DurationMs int64          `json:"duration_ms"`
}

// CheckpointRef describes the latest signed checkpoint and whether its
// signature and hash still match.
type CheckpointRef struct {
	Seq       int64     `json:"seq"`
	Hash      string    `json:"hash"`
	KeyID     string    `json:"key_id"`
	Signature string    `json:"signature"`
	Valid     bool      `json:"valid"`
	CreatedAt time.Time `json:"created_at"`
}

// VerifyLedger recomputes every hash in the workspace's chain.
func (a *App) VerifyLedger(ctx context.Context, actor Actor) (*VerifyReport, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	return a.verifyLedger(ctx, actor.WorkspaceID)
}

func (a *App) verifyLedger(ctx context.Context, ws string) (*VerifyReport, error) {
	start := a.now()
	headSeq, headHash, err := a.Store.LedgerHead(ctx, ws)
	if err != nil {
		return nil, err
	}
	rep := &VerifyReport{HeadSeq: headSeq, HeadHash: hex.EncodeToString(headHash)}
	prev := ledger.Genesis(ws)
	const batch = 2000
	for from := int64(1); from <= headSeq; from += batch {
		entries, err := a.Store.LedgerRange(ctx, ws, from, min(from+batch-1, headSeq))
		if err != nil {
			return nil, err
		}
		last, brk := ledger.Verify(entries, from, prev)
		rep.Entries += int64(len(entries))
		if brk != nil {
			rep.Break = brk
			break
		}
		prev = last
	}
	if rep.Break == nil && headSeq > 0 && string(prev) != string(headHash) {
		rep.Break = &ledger.Break{Seq: headSeq, Reason: "chain head does not match the last entry"}
	}
	cps, err := a.Store.ListCheckpoints(ctx, ws, 1)
	if err != nil {
		return nil, err
	}
	if len(cps) == 1 {
		cp := cps[0]
		ref := &CheckpointRef{Seq: cp.Seq, Hash: hex.EncodeToString(cp.Hash), KeyID: cp.KeyID, Signature: hex.EncodeToString(cp.Signature), CreatedAt: cp.CreatedAt}
		if sk, err := a.Store.SigningKeyByKeyID(ctx, ws, cp.KeyID); err == nil {
			sigOK := ed25519.Verify(sk.PublicKey, ledger.CheckpointMessage(ws, cp.Seq, cp.Hash), cp.Signature)
			entries, err := a.Store.LedgerRange(ctx, ws, cp.Seq, cp.Seq)
			hashOK := err == nil && len(entries) == 1 && string(entries[0].Hash) == string(cp.Hash)
			ref.Valid = sigOK && hashOK
		}
		rep.Checkpoint = ref
		if !ref.Valid && rep.Break == nil {
			rep.Break = &ledger.Break{Seq: cp.Seq, Reason: "signed checkpoint no longer matches the chain"}
		}
	}
	rep.VerifiedAt = a.now()
	rep.DurationMs = a.now().Sub(start).Milliseconds()
	if rep.Break != nil {
		a.Log.Error("ledger verification failed", "workspace", ws, "seq", rep.Break.Seq, "reason", rep.Break.Reason)
		_ = a.Store.Tx(ctx, func(q *store.Queries) error {
			return enqueueWebhook(ctx, q, ws, notify.EventLedgerBreak, map[string]any{"seq": rep.Break.Seq, "reason": rep.Break.Reason})
		})
	}
	return rep, nil
}

// Checkpoint signs the current head of a workspace's ledger if it moved
// since the last checkpoint.
func (a *App) Checkpoint(ctx context.Context, ws string) error {
	seq, hash, err := a.Store.LedgerHead(ctx, ws)
	if err != nil || seq == 0 {
		return err
	}
	cps, err := a.Store.ListCheckpoints(ctx, ws, 1)
	if err != nil {
		return err
	}
	if len(cps) == 1 && cps[0].Seq == seq {
		return nil
	}
	key, err := a.signer(ctx, ws)
	if err != nil {
		return err
	}
	sig := ed25519.Sign(key.private, ledger.CheckpointMessage(ws, seq, hash))
	return a.Store.InsertCheckpoint(ctx, ws, store.Checkpoint{Seq: seq, Hash: hash, KeyID: key.id, Signature: sig})
}

// LedgerPage is a page of ledger entries with checkpoints.
type LedgerPage struct {
	Entries     []ledger.Entry  `json:"entries"`
	Checkpoints []CheckpointRef `json:"checkpoints"`
	HeadSeq     int64           `json:"head_seq"`
	NextBefore  int64           `json:"next_before"`
}

// Ledger lists entries newest first.
func (a *App) Ledger(ctx context.Context, actor Actor, f store.LedgerFilter) (*LedgerPage, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	entries, err := a.Store.ListLedger(ctx, actor.WorkspaceID, f)
	if err != nil {
		return nil, err
	}
	head, _, err := a.Store.LedgerHead(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	page := &LedgerPage{Entries: entries, HeadSeq: head, Checkpoints: []CheckpointRef{}}
	if entries == nil {
		page.Entries = []ledger.Entry{}
	}
	if len(entries) == f.Limit {
		page.NextBefore = entries[len(entries)-1].Seq
	}
	cps, err := a.Store.ListCheckpoints(ctx, actor.WorkspaceID, 20)
	if err != nil {
		return nil, err
	}
	for _, cp := range cps {
		page.Checkpoints = append(page.Checkpoints, CheckpointRef{Seq: cp.Seq, Hash: hex.EncodeToString(cp.Hash), KeyID: cp.KeyID, Signature: hex.EncodeToString(cp.Signature), CreatedAt: cp.CreatedAt, Valid: true})
	}
	return page, nil
}

// GrantView describes an issued grant without its token.
type GrantView struct {
	ID         string     `json:"id"`
	Tool       string     `json:"tool"`
	ArgsHash   string     `json:"args_hash"`
	KeyID      string     `json:"key_id"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ConsumedAt *time.Time `json:"consumed_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// ActionDetail is everything recorded about one action.
type ActionDetail struct {
	Action      store.Action       `json:"action"`
	ArgsHash    string             `json:"args_hash"`
	Agent       store.Agent        `json:"agent"`
	Tool        *domain.Tool       `json:"tool"`
	Decision    *store.Decision    `json:"decision"`
	Assessment  *decide.Assessment `json:"assessment"`
	Evaluations []store.Evaluation `json:"evaluations"`
	Grants      []GrantView        `json:"grants"`
	Outcomes    []store.Outcome    `json:"outcomes"`
	Audit       *store.Audit       `json:"audit"`
	Ledger      []ledger.Entry     `json:"ledger"`
	ReviewURL   string             `json:"review_url,omitempty"`
}

// GetActionDetail returns the full record of an action for the console.
func (a *App) GetActionDetail(ctx context.Context, actor Actor, actionID string) (*ActionDetail, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	ws := actor.WorkspaceID
	act, err := a.Store.GetAction(ctx, ws, actionID)
	if err != nil {
		return nil, mapStoreErr(err, "action")
	}
	d := &ActionDetail{Action: act, ArgsHash: hex.EncodeToString(act.ArgsHash), ReviewURL: a.reviewURL(act.ID)}
	if d.Agent, err = a.Store.GetAgent(ctx, ws, act.AgentID); err != nil {
		return nil, err
	}
	if t, err := a.Store.GetTool(ctx, ws, act.Tool); err == nil {
		d.Tool = &t
	}
	if act.Status.Final() {
		dec, err := a.Store.GetDecision(ctx, ws, act.ID)
		if err != nil {
			return nil, err
		}
		d.Decision = &dec
	}
	if d.Evaluations, err = a.Store.ListEvaluations(ctx, ws, act.ID); err != nil {
		return nil, err
	}
	if d.Assessment, err = a.modelAssessment(ctx, a.Store.Queries, ws, act.ID); err != nil {
		return nil, err
	}
	grants, err := a.Store.ListGrants(ctx, ws, act.ID)
	if err != nil {
		return nil, err
	}
	d.Grants = []GrantView{}
	for _, g := range grants {
		d.Grants = append(d.Grants, GrantView{ID: g.ID, Tool: g.Tool, ArgsHash: hex.EncodeToString(g.ArgsHash), KeyID: g.KeyID, ExpiresAt: g.ExpiresAt, ConsumedAt: g.ConsumedAt, CreatedAt: g.CreatedAt})
	}
	if d.Outcomes, err = a.Store.ListOutcomes(ctx, ws, act.ID); err != nil {
		return nil, err
	}
	if au, err := a.Store.GetAuditForAction(ctx, ws, act.ID); err == nil {
		d.Audit = &au
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	entries, err := a.Store.ListLedger(ctx, ws, store.LedgerFilter{ActionID: act.ID, Limit: 200})
	if err != nil {
		return nil, err
	}
	slices.Reverse(entries)
	d.Ledger = entries
	if d.Ledger == nil {
		d.Ledger = []ledger.Entry{}
	}
	if d.Evaluations == nil {
		d.Evaluations = []store.Evaluation{}
	}
	if d.Outcomes == nil {
		d.Outcomes = []store.Outcome{}
	}
	return d, nil
}

// SystemStatus describes this server for the settings page.
type SystemStatus struct {
	Version       string      `json:"version"`
	SchemaVersion string      `json:"schema_version"`
	Model         ModelStatus `json:"model"`
	PublicURL     string      `json:"public_url"`
	Slack         bool        `json:"slack_available"`
	PrivateConns  bool        `json:"private_connections_allowed"`
	DecisionMs    int64       `json:"decision_timeout_ms"`
	ServerTime    time.Time   `json:"server_time"`
	SigningKeys   []KeyView   `json:"signing_keys"`
}

// KeyView is a public signing key.
type KeyView struct {
	KeyID     string     `json:"key_id"`
	PublicKey string     `json:"public_key"`
	CreatedAt time.Time  `json:"created_at"`
	RetiredAt *time.Time `json:"retired_at"`
}

// Status reports this server's configuration.
func (a *App) Status(ctx context.Context, actor Actor) (*SystemStatus, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	v, err := a.Store.SchemaVersion(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := a.PublicKeys(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return &SystemStatus{
		Version: Version, SchemaVersion: v, Model: ModelStatus{Configured: a.ModelConfigured(), Name: a.ModelName},
		PublicURL: a.PublicURL, Slack: a.Slack != nil, PrivateConns: a.PrivateConnector != nil,
		DecisionMs: a.DecisionTimeout.Milliseconds(), ServerTime: a.now().UTC(), SigningKeys: keys,
	}, nil
}

// PublicKeys returns a workspace's grant and checkpoint verification keys.
func (a *App) PublicKeys(ctx context.Context, ws string) ([]KeyView, error) {
	ks, err := a.Store.ListSigningKeys(ctx, ws)
	if err != nil {
		return nil, err
	}
	out := []KeyView{}
	for _, k := range ks {
		out = append(out, KeyView{KeyID: k.KeyID, PublicKey: hex.EncodeToString(k.PublicKey), CreatedAt: k.CreatedAt, RetiredAt: k.RetiredAt})
	}
	return out, nil
}

// VerifyWorkspaceLedger verifies a workspace's chain for background jobs.
func (a *App) VerifyWorkspaceLedger(ctx context.Context, ws string) (*VerifyReport, error) {
	return a.verifyLedger(ctx, ws)
}
