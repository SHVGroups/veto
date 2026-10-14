// Package policy is Veto's deterministic gate. Workspaces write rules in
// Cedar (https://www.cedarpolicy.com) and Veto evaluates them before any
// model sees a call.
//
// Veto gives the two Cedar effects these meanings:
//
//   - forbid stops the call. Nothing else is consulted.
//   - permit must carry @veto("allow") or @veto("review"). Allow lets the call
//     skip the model and run; review sends it to a person. Review wins when
//     both match.
//
// A call that matches no policy continues to the model. A permit without a
// @veto annotation is rejected at compile time so that a careless rule can
// never silently wave calls through.
package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cedar-policy/cedar-go"

	"github.com/shvgroups/veto/server/internal/domain"
)

// Outcome is the gate's verdict.
type Outcome string

const (
	OutcomeContinue Outcome = "continue"
	OutcomeAllow    Outcome = "allow"
	OutcomeReview   Outcome = "review"
	OutcomeDeny     Outcome = "deny"
)

// Entity and action names used in requests. Policies refer to them, so they
// are part of Veto's public contract.
const (
	EntityAgent  = "Agent"
	EntityTool   = "Tool"
	EntityAction = "Action"
	ActionCall   = "call"
)

const annotationVeto = "veto"

// Diagnostic is a compile or evaluation problem tied to a policy.
type Diagnostic struct {
	PolicyID string `json:"policy_id,omitempty"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
	Message  string `json:"message"`
}

// CompileError reports every problem found in a policy document.
type CompileError struct {
	Diagnostics []Diagnostic
}

func (e *CompileError) Error() string {
	msgs := make([]string, len(e.Diagnostics))
	for i, d := range e.Diagnostics {
		if d.Line > 0 {
			msgs[i] = fmt.Sprintf("line %d:%d: %s", d.Line, d.Column, d.Message)
		} else {
			msgs[i] = d.Message
		}
	}
	return "policy: " + strings.Join(msgs, "; ")
}

// Summary describes one compiled policy for the console.
type Summary struct {
	ID     string `json:"id"`
	Effect string `json:"effect"`
	Line   int    `json:"line"`
}

// Set is a compiled policy document.
type Set struct {
	ps       *cedar.PolicySet
	effect   map[string]Outcome
	summary  []Summary
	Source   string
	Version  int
	compiled time.Time
}

// Compile parses a Cedar document and checks Veto's annotation rules.
func Compile(source string, version int) (*Set, error) {
	list, err := cedar.NewPolicyListFromBytes("policies.cedar", []byte(source))
	if err != nil {
		return nil, &CompileError{Diagnostics: []Diagnostic{parseDiagnostic(err)}}
	}
	set := &Set{
		ps:       cedar.NewPolicySet(),
		effect:   make(map[string]Outcome, len(list)),
		Source:   source,
		Version:  version,
		compiled: time.Now(),
	}
	var diags []Diagnostic
	for i, p := range list {
		ann := p.Annotations()
		pid := fmt.Sprintf("policy%d", i)
		if v, ok := ann["id"]; ok && v != "" {
			pid = string(v)
		}
		pos := p.Position()
		fail := func(msg string) {
			diags = append(diags, Diagnostic{PolicyID: pid, Line: pos.Line, Column: pos.Column, Message: msg})
		}
		if _, dup := set.effect[pid]; dup {
			fail(fmt.Sprintf("duplicate policy id %q", pid))
			continue
		}
		veto, hasVeto := ann[annotationVeto]
		switch p.Effect() {
		case cedar.Forbid:
			if hasVeto {
				fail(`forbid policies always stop the call; remove the @veto annotation`)
				continue
			}
			set.effect[pid] = OutcomeDeny
		case cedar.Permit:
			switch string(veto) {
			case "allow":
				set.effect[pid] = OutcomeAllow
			case "review":
				set.effect[pid] = OutcomeReview
			default:
				if hasVeto {
					fail(fmt.Sprintf(`@veto(%q) is not valid; use "allow" or "review"`, veto))
				} else {
					fail(`permit policies must say what they do: add @veto("allow") or @veto("review")`)
				}
				continue
			}
		}
		set.ps.Add(cedar.PolicyID(pid), p)
		set.summary = append(set.summary, Summary{ID: pid, Effect: string(set.effect[pid]), Line: pos.Line})
	}
	if len(diags) > 0 {
		return nil, &CompileError{Diagnostics: diags}
	}
	return set, nil
}

// Policies lists the compiled policies in document order.
func (s *Set) Policies() []Summary { return slices.Clone(s.summary) }

// Input is everything the gate knows about a call.
type Input struct {
	Agent       Agent
	Tool        domain.Tool
	Args        any
	PrincipalID string
	Source      domain.Source
	Now         time.Time
}

// Agent is the subset of an agent visible to policies.
type Agent struct {
	ID     string
	Name   string
	Vendor string
}

// Result is the gate's verdict with the policies that produced it.
type Result struct {
	Outcome  Outcome      `json:"outcome"`
	Policies []string     `json:"policies"`
	Errors   []Diagnostic `json:"errors,omitempty"`
}

// Evaluate runs the policies against one call.
func (s *Set) Evaluate(in Input) Result {
	if s == nil {
		return Result{Outcome: OutcomeContinue}
	}
	agentUID := cedar.NewEntityUID(EntityAgent, cedar.String(in.Agent.ID))
	toolUID := cedar.NewEntityUID(EntityTool, cedar.String(in.Tool.Name))
	entities := cedar.EntityMap{
		agentUID: {
			UID: agentUID,
			Attributes: cedar.NewRecord(cedar.RecordMap{
				"name":   cedar.String(in.Agent.Name),
				"vendor": cedar.String(in.Agent.Vendor),
			}),
		},
		toolUID: {
			UID: toolUID,
			Attributes: cedar.NewRecord(cedar.RecordMap{
				"risk_class": cedar.String(in.Tool.RiskClass),
				"reversible": cedar.Boolean(in.Tool.Reversible),
				"registered": cedar.Boolean(in.Tool.Registered),
			}),
		},
	}
	now := in.Now.UTC()
	ctx := cedar.RecordMap{
		"args":      toCedar(in.Args),
		"principal": cedar.String(in.PrincipalID),
		"source":    cedar.String(in.Source),
		"hour_utc":  cedar.Long(now.Hour()),
		"weekday":   cedar.String(strings.ToLower(now.Weekday().String())),
		"now":       cedar.NewDatetime(now),
	}
	req := cedar.Request{
		Principal: agentUID,
		Action:    cedar.NewEntityUID(EntityAction, ActionCall),
		Resource:  toolUID,
		Context:   cedar.NewRecord(ctx),
	}
	_, diag := s.ps.IsAuthorized(entities, req)

	res := Result{Outcome: OutcomeContinue}
	var allow, review, deny []string
	for _, r := range diag.Reasons {
		pid := string(r.PolicyID)
		switch s.effect[pid] {
		case OutcomeDeny:
			deny = append(deny, pid)
		case OutcomeReview:
			review = append(review, pid)
		case OutcomeAllow:
			allow = append(allow, pid)
		}
	}
	for _, e := range diag.Errors {
		pid := string(e.PolicyID)
		res.Errors = append(res.Errors, Diagnostic{PolicyID: pid, Line: e.Position.Line, Column: e.Position.Column, Message: e.Message})
		// A forbid that could not be evaluated might have matched. Hold the
		// call for a person rather than let it through.
		if s.effect[pid] == OutcomeDeny {
			review = append(review, pid)
		}
	}
	switch {
	case len(deny) > 0:
		res.Outcome, res.Policies = OutcomeDeny, deny
	case len(review) > 0:
		res.Outcome, res.Policies = OutcomeReview, review
	case len(allow) > 0:
		res.Outcome, res.Policies = OutcomeAllow, allow
	}
	sort.Strings(res.Policies)
	return res
}

// toCedar converts decoded JSON to Cedar values. Integers become Long,
// fractions become Decimal (four places), null fields are dropped because
// Cedar has no null, and values that do not fit become strings.
func toCedar(v any) cedar.Value {
	switch x := v.(type) {
	case nil:
		return cedar.NewRecord(cedar.RecordMap{})
	case bool:
		return cedar.Boolean(x)
	case string:
		return cedar.String(x)
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return cedar.Long(n)
		}
		if f, err := x.Float64(); err == nil && !math.IsInf(f, 0) {
			if d, err := cedar.NewDecimalFromFloat(f); err == nil {
				return d
			}
		}
		return cedar.String(x.String())
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1<<53 {
			return cedar.Long(int64(x))
		}
		if d, err := cedar.NewDecimalFromFloat(x); err == nil {
			return d
		}
		return cedar.String(fmt.Sprint(x))
	case []any:
		vals := make([]cedar.Value, 0, len(x))
		for _, e := range x {
			if e != nil {
				vals = append(vals, toCedar(e))
			}
		}
		return cedar.NewSet(vals...)
	case map[string]any:
		m := make(cedar.RecordMap, len(x))
		for k, e := range x {
			if e != nil {
				m[cedar.String(k)] = toCedar(e)
			}
		}
		return cedar.NewRecord(m)
	}
	return cedar.String(fmt.Sprint(v))
}

// parsePosition matches the "<file>:line:column" cedar-go puts in parse errors.
var parsePosition = regexp.MustCompile(`at [^:\s]+:(\d+):(\d+)\s*(.*)$`)

func parseDiagnostic(err error) Diagnostic {
	msg := err.Error()
	d := Diagnostic{Message: strings.TrimPrefix(msg, "parser error: ")}
	if m := parsePosition.FindStringSubmatch(msg); m != nil {
		d.Line, _ = strconv.Atoi(m[1])
		d.Column, _ = strconv.Atoi(m[2])
		if m[3] != "" {
			d.Message = m[3]
		}
	}
	return d
}

// ErrEmpty is returned when a document contains no policies. An empty
// document is valid; callers use this only to warn.
var ErrEmpty = errors.New("policy: document contains no policies")

// DefaultSource is the policy document every new workspace starts with.
const DefaultSource = `// Veto policies, written in Cedar.
//
// forbid  stops a call outright.
// permit  must say what it does:
//   @veto("allow")   the call skips the model and runs
//   @veto("review")  the call is held for a person
// A call that matches nothing goes to the decision model.
//
// The request is: principal = Agent, action = Action::"call",
// resource = Tool. context.args holds the call's arguments.

@id("read-tools-skip-the-model")
@veto("allow")
permit (principal, action, resource)
when { resource.risk_class == "read" };

@id("critical-tools-need-a-person")
@veto("review")
permit (principal, action, resource)
when { resource.risk_class == "critical" };

@id("unclassified-tools-need-a-person")
@veto("review")
permit (principal, action, resource)
when { !resource.registered };
`
