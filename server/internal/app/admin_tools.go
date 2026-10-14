package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"unicode/utf8"

	"github.com/shvgroups/veto/server/internal/decide"
	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/id"
	"github.com/shvgroups/veto/server/internal/ledger"
	"github.com/shvgroups/veto/server/internal/policy"
	"github.com/shvgroups/veto/server/internal/safer"
	"github.com/shvgroups/veto/server/internal/store"
)

// ToolInput creates or replaces a tool profile.
type ToolInput struct {
	Name            string           `json:"name"`
	Description     string           `json:"description"`
	RiskClass       domain.RiskClass `json:"risk_class"`
	Reversible      bool             `json:"reversible"`
	Pack            string           `json:"pack"`
	Safer           json.RawMessage  `json:"safer"`
	AutoConfidence  *float64         `json:"auto_confidence"`
	RiskCeiling     *float64         `json:"risk_ceiling"`
	BlockConfidence *float64         `json:"block_confidence"`
	DoubleCheck     *bool            `json:"double_check"`
	FailMode        domain.FailMode  `json:"fail_mode"`
	MaxPerHour      *int             `json:"max_per_hour"`
	StepUp          bool             `json:"step_up"`
}

func (a *App) validateTool(ctx context.Context, ws string, in *ToolInput) error {
	if !ValidToolName(in.Name) {
		return invalidf("tool name must be 1 to 200 characters of letters, digits and _ . : / -")
	}
	if utf8.RuneCountInString(in.Description) > 500 {
		return invalidf("description must be at most 500 characters")
	}
	if !in.RiskClass.Valid() {
		return invalidf("risk_class must be read, low, medium, high or critical")
	}
	if in.Pack == "" {
		in.Pack = "general"
	}
	if _, ok := a.packs[in.Pack]; !ok {
		overrides, err := a.Store.ActivePackOverrides(ctx, ws)
		if err != nil {
			return err
		}
		found := false
		for _, o := range overrides {
			found = found || o.Name == in.Pack
		}
		if !found {
			return invalidf("pack %q does not exist", in.Pack)
		}
	}
	if len(in.Safer) == 0 || string(in.Safer) == "null" {
		in.Safer = json.RawMessage("[]")
	}
	if _, err := safer.ParseRules(in.Safer); err != nil {
		return invalidf("%v", err)
	}
	for name, v := range map[string]*float64{"auto_confidence": in.AutoConfidence, "risk_ceiling": in.RiskCeiling, "block_confidence": in.BlockConfidence} {
		if v != nil && (*v < 0 || *v > 1) {
			return invalidf("%s must be between 0 and 1", name)
		}
	}
	if in.FailMode == "" {
		in.FailMode = domain.FailClosed
	}
	if in.FailMode != domain.FailClosed && in.FailMode != domain.FailOpen {
		return invalidf("fail_mode must be closed or open")
	}
	if in.FailMode == domain.FailOpen && (in.RiskClass == domain.RiskHigh || in.RiskClass == domain.RiskCritical) {
		return invalidf("high and critical tools must fail closed")
	}
	if in.MaxPerHour != nil && (*in.MaxPerHour < 1 || *in.MaxPerHour > 1_000_000) {
		return invalidf("max_per_hour must be between 1 and 1000000")
	}
	return nil
}

// SaveTool creates or replaces a tool profile and marks it classified.
func (a *App) SaveTool(ctx context.Context, actor Actor, in ToolInput) (domain.Tool, error) {
	if err := require(actor, domain.PermConfigure); err != nil {
		return domain.Tool{}, err
	}
	if err := a.validateTool(ctx, actor.WorkspaceID, &in); err != nil {
		return domain.Tool{}, err
	}
	t := domain.Tool{
		WorkspaceID: actor.WorkspaceID, Name: in.Name, Description: in.Description, RiskClass: in.RiskClass,
		Reversible: in.Reversible, Pack: in.Pack, Safer: in.Safer, AutoConfidence: in.AutoConfidence,
		RiskCeiling: in.RiskCeiling, BlockConfidence: in.BlockConfidence, DoubleCheck: in.DoubleCheck,
		FailMode: in.FailMode, MaxPerHour: in.MaxPerHour, StepUp: in.StepUp, Registered: true,
	}
	err := a.Store.Tx(ctx, func(q *store.Queries) error {
		if err := q.UpsertTool(ctx, t); err != nil {
			return err
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindToolUpdated, "", actor, map[string]any{
			"tool": t.Name, "risk_class": t.RiskClass, "pack": t.Pack, "registered": true,
			"thresholds": t.Thresholds(), "fail_mode": t.FailMode, "max_per_hour": t.MaxPerHour, "step_up": t.StepUp,
			"safer_rules": json.RawMessage(t.Safer),
		})
	})
	if err != nil {
		return domain.Tool{}, err
	}
	return a.Store.GetTool(ctx, actor.WorkspaceID, t.Name)
}

// DeleteTool removes a tool profile. The next call to it registers it again
// as unclassified.
func (a *App) DeleteTool(ctx context.Context, actor Actor, name string) error {
	if err := require(actor, domain.PermConfigure); err != nil {
		return err
	}
	return a.Store.Tx(ctx, func(q *store.Queries) error {
		if err := q.DeleteTool(ctx, actor.WorkspaceID, name); err != nil {
			return mapStoreErr(err, "tool")
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindToolDeleted, "", actor, map[string]any{"tool": name})
	})
}

// PreviewSafer shows what a tool's safer-variant rules would produce for a
// sample call, so admins can check rules before saving them.
func (a *App) PreviewSafer(actor Actor, tool string, args, rules json.RawMessage) (domain.OptionSpec, error) {
	if err := require(actor, domain.PermView); err != nil {
		return domain.OptionSpec{}, err
	}
	parsed, err := safer.ParseRules(rules)
	if err != nil {
		return domain.OptionSpec{}, invalidf("%v", err)
	}
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	spec, err := safer.Build(tool, args, parsed)
	if err != nil {
		return domain.OptionSpec{}, invalidf("%v", err)
	}
	return spec, nil
}

// PolicyCheck is the result of compiling a policy document.
type PolicyCheck struct {
	Valid       bool                `json:"valid"`
	Diagnostics []policy.Diagnostic `json:"diagnostics"`
	Policies    []policy.Summary    `json:"policies"`
}

// CheckPolicy compiles a document without saving it.
func (a *App) CheckPolicy(actor Actor, source string) (*PolicyCheck, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	if len(source) > 256<<10 {
		return nil, invalidf("policy document must be at most 256 KiB")
	}
	set, err := policy.Compile(source, 0)
	if err != nil {
		var ce *policy.CompileError
		if errors.As(err, &ce) {
			return &PolicyCheck{Valid: false, Diagnostics: ce.Diagnostics, Policies: []policy.Summary{}}, nil
		}
		return nil, err
	}
	return &PolicyCheck{Valid: true, Diagnostics: []policy.Diagnostic{}, Policies: set.Policies()}, nil
}

// SavePolicy stores a new policy version and optionally activates it. A
// document that does not compile is rejected.
func (a *App) SavePolicy(ctx context.Context, actor Actor, source, note string, activate bool) (int, error) {
	if err := require(actor, domain.PermConfigure); err != nil {
		return 0, err
	}
	check, err := a.CheckPolicy(actor, source)
	if err != nil {
		return 0, err
	}
	if !check.Valid {
		return 0, &Error{Code: CodeInvalid, Message: "policy document does not compile", Details: check.Diagnostics}
	}
	if utf8.RuneCountInString(note) > 500 {
		return 0, invalidf("note must be at most 500 characters")
	}
	var v int
	err = a.Store.Tx(ctx, func(q *store.Queries) error {
		uid := actor.ID
		var err error
		if v, err = q.CreatePolicyVersion(ctx, actor.WorkspaceID, source, note, &uid); err != nil {
			return err
		}
		if err := record(ctx, q, actor.WorkspaceID, ledger.KindPolicyCreated, "", actor, map[string]any{
			"version": v, "note": note, "policies": len(check.Policies), "sha256": sha256Hex(source),
		}); err != nil {
			return err
		}
		if activate {
			return a.activatePolicy(ctx, q, actor, v)
		}
		return nil
	})
	return v, err
}

// ActivatePolicy makes a stored version the one the gate evaluates.
func (a *App) ActivatePolicy(ctx context.Context, actor Actor, version int) error {
	if err := require(actor, domain.PermConfigure); err != nil {
		return err
	}
	return a.Store.Tx(ctx, func(q *store.Queries) error {
		pv, err := q.GetPolicyVersion(ctx, actor.WorkspaceID, version)
		if err != nil {
			return mapStoreErr(err, "policy version")
		}
		if _, err := policy.Compile(pv.Source, pv.Version); err != nil {
			return invalidf("version %d no longer compiles: %v", version, err)
		}
		return a.activatePolicy(ctx, q, actor, version)
	})
}

func (a *App) activatePolicy(ctx context.Context, q *store.Queries, actor Actor, version int) error {
	if err := q.ActivatePolicyVersion(ctx, actor.WorkspaceID, version); err != nil {
		return err
	}
	return record(ctx, q, actor.WorkspaceID, ledger.KindPolicyActivated, "", actor, map[string]any{"version": version})
}

// PackInfo describes a question pack and where it comes from.
type PackInfo struct {
	Name       string       `json:"name"`
	Title      string       `json:"title"`
	Builtin    bool         `json:"builtin"`
	Overridden bool         `json:"overridden"`
	Version    int          `json:"version"`
	Definition *decide.Pack `json:"definition"`
	Tools      []string     `json:"tools"`
}

// ListPacks returns every pack the workspace can use.
func (a *App) ListPacks(ctx context.Context, actor Actor) ([]PackInfo, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	overrides, err := a.Store.ActivePackOverrides(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	tools, err := a.Store.ListTools(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	byPack := map[string][]string{}
	for _, t := range tools {
		byPack[t.Pack] = append(byPack[t.Pack], t.Name)
	}
	infos := map[string]*PackInfo{}
	for name, p := range a.packs {
		infos[name] = &PackInfo{Name: name, Title: p.Title, Builtin: true, Version: p.Version, Definition: p}
	}
	for _, o := range overrides {
		p, err := decide.ParsePack(o.Definition)
		if err != nil {
			continue
		}
		info, ok := infos[o.Name]
		if !ok {
			info = &PackInfo{Name: o.Name}
			infos[o.Name] = info
		}
		info.Title, info.Overridden, info.Version, info.Definition = p.Title, true, o.Version, p
	}
	out := make([]PackInfo, 0, len(infos))
	for name, info := range infos {
		info.Tools = byPack[name]
		if info.Tools == nil {
			info.Tools = []string{}
		}
		out = append(out, *info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// SavePack stores a workspace override of a pack, or a new custom pack.
func (a *App) SavePack(ctx context.Context, actor Actor, name string, definition json.RawMessage) (int, error) {
	if err := require(actor, domain.PermConfigure); err != nil {
		return 0, err
	}
	p, err := decide.ParsePack(definition)
	if err != nil {
		return 0, invalidf("%v", err)
	}
	if p.Name != name {
		return 0, invalidf("pack name in the definition (%q) must match %q", p.Name, name)
	}
	canon, err := json.Marshal(p)
	if err != nil {
		return 0, err
	}
	var v int
	err = a.Store.Tx(ctx, func(q *store.Queries) error {
		uid := actor.ID
		if v, err = q.CreatePackVersion(ctx, actor.WorkspaceID, name, canon, &uid); err != nil {
			return err
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindPackUpdated, "", actor, map[string]any{
			"pack": name, "version": v, "signals": len(p.Signals), "guards": len(p.Guards), "sha256": sha256Hex(string(canon)),
		})
	})
	a.invalidatePacks(actor.WorkspaceID)
	return v, err
}

// ResetPack removes a workspace override so the built-in pack applies again.
func (a *App) ResetPack(ctx context.Context, actor Actor, name string) error {
	if err := require(actor, domain.PermConfigure); err != nil {
		return err
	}
	err := a.Store.Tx(ctx, func(q *store.Queries) error {
		if err := q.DeactivatePack(ctx, actor.WorkspaceID, name); err != nil {
			return err
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindPackUpdated, "", actor, map[string]any{"pack": name, "reset": true})
	})
	a.invalidatePacks(actor.WorkspaceID)
	return err
}

// SimulateRequest runs the decision pipeline on a hypothetical call.
type SimulateRequest struct {
	AgentID   string             `json:"agent_id"`
	Tool      string             `json:"tool"`
	Args      json.RawMessage    `json:"args"`
	Request   string             `json:"request"`
	Untrusted []domain.Untrusted `json:"untrusted"`
	Principal string             `json:"principal"`
	// CallModel sends the questions to the decision model. Without it the
	// simulation stops after showing the request it would send.
	CallModel bool `json:"call_model"`
}

// SimulateResult shows every step the pipeline would take.
type SimulateResult struct {
	Tool         domain.Tool         `json:"tool"`
	Gate         policy.Result       `json:"gate"`
	Options      []domain.OptionSpec `json:"options"`
	Pack         string              `json:"pack"`
	ModelRequest json.RawMessage     `json:"model_request"`
	Assessment   *decide.Assessment  `json:"assessment"`
	ModelError   string              `json:"model_error,omitempty"`
	Verdict      decide.Verdict      `json:"verdict"`
	Thresholds   domain.Thresholds   `json:"thresholds"`
}

// Simulate runs the pipeline without recording anything, for the console's
// playground. Rate limits are not applied.
func (a *App) Simulate(ctx context.Context, actor Actor, req SimulateRequest) (*SimulateResult, error) {
	if err := require(actor, domain.PermConfigure); err != nil {
		return nil, err
	}
	sub := SubmitRequest{Tool: req.Tool, Args: req.Args, Request: req.Request, Untrusted: req.Untrusted}
	if err := sub.validate(); err != nil {
		return nil, err
	}
	ws, err := a.Store.GetWorkspace(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	agent, err := a.Store.GetAgent(ctx, ws.ID, req.AgentID)
	if err != nil {
		return nil, mapStoreErr(err, "agent")
	}
	tool, err := a.Store.GetTool(ctx, ws.ID, req.Tool)
	if errors.Is(err, store.ErrNotFound) {
		tool = domain.UnregisteredTool(ws.ID, req.Tool)
	} else if err != nil {
		return nil, err
	}
	act := &store.Action{
		ID: id.New(id.Action), WorkspaceID: ws.ID, AgentID: agent.ID, AgentName: agent.Name, Tool: req.Tool,
		Args: sub.Args, Request: req.Request, Untrusted: req.Untrusted, Source: domain.SourceAPI,
		Principal: domain.Principal{ID: req.Principal},
	}
	tool.MaxPerHour = nil

	res := &SimulateResult{Tool: tool, Thresholds: tool.Thresholds()}
	pack, err := a.pack(ctx, ws.ID, tool.Pack)
	if err != nil {
		return nil, err
	}
	res.Pack = pack.Name

	model := a.Model
	if !req.CallModel {
		model = nil
	}
	ev := a.evaluate(ctx, model, ws, agent, tool, act)
	res.Gate, res.Options, res.Assessment, res.Verdict = ev.gate, ev.options, ev.assessment, ev.verdict
	if ev.modelErr != nil {
		res.ModelError = ev.modelErr.Error()
	}
	if decide.NeedsModel(ev.gate) {
		r, _, _ := decide.BuildRequest(pack, decide.Subject{
			ActionID: act.ID, Request: act.Request, Agent: agent.Name, Tool: tool, Args: act.Args,
			Safer: ev.options[1], Untrusted: act.Untrusted, UntrustedMaxChars: ws.Settings.UntrustedMaxChars,
		})
		r.Model = a.ModelName
		res.ModelRequest, _ = json.Marshal(r)
	}
	return res, nil
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
