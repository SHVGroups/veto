// Package decide turns a proposed call into a decision. It builds the state
// and questions for the decision model, reads the typed answers into a risk
// assessment, and applies Veto's decision rules. The rules are pure functions
// of their inputs so every decision can be replayed and tested.
package decide

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/jev"
)

// State keys a pack can refer to in instructions and require on signals.
const (
	KeyRequest      = "request"
	KeyAction       = "action"
	KeySaferVariant = "safer_variant"
	KeyUntrusted    = "untrusted_content"
	KeyTool         = "tool"
	KeyAgent        = "agent"
)

var stateKeys = map[string]bool{
	KeyRequest: true, KeyAction: true, KeySaferVariant: true,
	KeyUntrusted: true, KeyTool: true, KeyAgent: true,
}

// Pack is a versioned set of questions for one category of tool.
type Pack struct {
	Name        string      `json:"name"`
	Version     int         `json:"version"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Disposition Disposition `json:"disposition"`
	Signals     []Signal    `json:"signals"`
	Guards      []Guard     `json:"guards"`
}

// Disposition is the Choice question that picks among the three options.
type Disposition struct {
	Instructions string                   `json:"instructions"`
	Criteria     map[domain.Option]string `json:"criteria"`
}

// Signal is one supporting judgment. Signals with a risk weight contribute to
// the composite risk score; signals without one are recorded for reviewers.
type Signal struct {
	ID           string           `json:"id"`
	Type         jev.QuestionType `json:"type"`
	Instructions string           `json:"instructions"`
	// Levels are the ordered levels of a score signal.
	Levels []string `json:"levels,omitempty"`
	// Choices are the options of a choice signal, each with the risk it
	// implies between 0 and 1.
	Choices []SignalChoice `json:"choices,omitempty"`
	// Yes and No optionally describe what a noul's answers mean.
	Yes string `json:"yes,omitempty"`
	No  string `json:"no,omitempty"`
	// Requires lists state keys that must be present to ask this question.
	Requires []string    `json:"requires,omitempty"`
	Risk     *RiskWeight `json:"risk,omitempty"`
}

// SignalChoice is one option of a choice signal.
type SignalChoice struct {
	Key         string  `json:"key"`
	Description string  `json:"description"`
	Risk        float64 `json:"risk"`
}

// RiskWeight says how a signal feeds the composite risk.
type RiskWeight struct {
	Weight float64 `json:"weight"`
	// Invert treats a high value as low risk, for questions phrased so that
	// yes is the safe answer.
	Invert bool `json:"invert,omitempty"`
}

// Guard forbids options outright when a signal crosses a line, whatever the
// disposition says.
type Guard struct {
	Signal string          `json:"signal"`
	Above  *float64        `json:"above,omitempty"`
	Below  *float64        `json:"below,omitempty"`
	Forbid []domain.Option `json:"forbid"`
	Reason string          `json:"reason"`
}

var signalID = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// Validate checks a pack for mistakes that would make its questions invalid
// or its scores meaningless.
func (p *Pack) Validate() error {
	var errs []error
	add := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	if !signalID.MatchString(p.Name) {
		add("name %q must be lowercase letters, digits and underscores", p.Name)
	}
	if p.Version < 1 {
		add("version must be at least 1")
	}
	if strings.TrimSpace(p.Disposition.Instructions) == "" {
		add("disposition needs instructions")
	}
	for _, o := range domain.Options {
		if strings.TrimSpace(p.Disposition.Criteria[o]) == "" {
			add("disposition needs a description for %q", o)
		}
	}
	for o := range p.Disposition.Criteria {
		if !o.Valid() {
			add("disposition has unknown option %q", o)
		}
	}
	seen := map[string]bool{"disposition": true}
	for i, s := range p.Signals {
		where := fmt.Sprintf("signal %d (%s)", i+1, s.ID)
		if !signalID.MatchString(s.ID) {
			add("%s: id must be lowercase letters, digits and underscores", where)
		}
		if seen[s.ID] {
			add("%s: duplicate id", where)
		}
		seen[s.ID] = true
		if strings.TrimSpace(s.Instructions) == "" {
			add("%s: needs instructions", where)
		}
		for _, k := range s.Requires {
			if !stateKeys[k] {
				add("%s: requires unknown state key %q", where, k)
			}
		}
		switch s.Type {
		case jev.TypeNoul:
		case jev.TypeScore:
			if len(s.Levels) < jev.MinScoreLevels || len(s.Levels) > jev.MaxScoreLevels {
				add("%s: score needs %d to %d levels", where, jev.MinScoreLevels, jev.MaxScoreLevels)
			}
		case jev.TypeChoice:
			if len(s.Choices) < 2 || len(s.Choices) > jev.MaxChoiceOptions {
				add("%s: choice needs 2 to %d options", where, jev.MaxChoiceOptions)
			}
			keys := map[string]bool{}
			for _, c := range s.Choices {
				if c.Key == "" || keys[c.Key] {
					add("%s: choice keys must be unique and non-empty", where)
				}
				keys[c.Key] = true
				if c.Risk < 0 || c.Risk > 1 {
					add("%s: choice %q risk must be between 0 and 1", where, c.Key)
				}
			}
		default:
			add("%s: unknown type %q", where, s.Type)
		}
		if s.Risk != nil && (s.Risk.Weight <= 0 || s.Risk.Weight > 1) {
			add("%s: risk weight must be in (0, 1]", where)
		}
	}
	for i, g := range p.Guards {
		where := fmt.Sprintf("guard %d", i+1)
		if !seen[g.Signal] || g.Signal == "disposition" {
			add("%s: unknown signal %q", where, g.Signal)
		}
		if (g.Above == nil) == (g.Below == nil) {
			add("%s: set exactly one of above or below", where)
		}
		if len(g.Forbid) == 0 {
			add("%s: forbid at least one option", where)
		}
		for _, o := range g.Forbid {
			if !o.Valid() || o == domain.OptionStop {
				add("%s: can only forbid proceed or proceed_safer, not %q", where, o)
			}
		}
		if strings.TrimSpace(g.Reason) == "" {
			add("%s: needs a reason", where)
		}
	}
	return errors.Join(errs...)
}

// signal returns the signal with the given id.
func (p *Pack) signal(id string) (Signal, bool) {
	for _, s := range p.Signals {
		if s.ID == id {
			return s, true
		}
	}
	return Signal{}, false
}

// ParsePack decodes and validates a pack definition.
func ParsePack(raw []byte) (*Pack, error) {
	var p Pack
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("decide: invalid pack: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("decide: invalid pack %q: %w", p.Name, err)
	}
	return &p, nil
}

//go:embed packs/*.json
var builtinFS embed.FS

// Builtin returns the question packs that ship with Veto, keyed by name.
func Builtin() (map[string]*Pack, error) {
	entries, err := fs.ReadDir(builtinFS, "packs")
	if err != nil {
		return nil, err
	}
	packs := make(map[string]*Pack, len(entries))
	for _, e := range entries {
		raw, err := builtinFS.ReadFile("packs/" + e.Name())
		if err != nil {
			return nil, err
		}
		p, err := ParsePack(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if p.Name+".json" != e.Name() {
			return nil, fmt.Errorf("%s: pack name %q does not match file name", e.Name(), p.Name)
		}
		packs[p.Name] = p
	}
	return packs, nil
}

// BuiltinNames returns the names of the shipped packs in sorted order.
func BuiltinNames() []string {
	packs, err := Builtin()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(packs))
	for n := range packs {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
