package policy

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shvgroups/veto/server/internal/canonical"
	"github.com/shvgroups/veto/server/internal/domain"
)

const testPolicies = `
@id("no-wires-on-weekends")
forbid (principal, action, resource == Tool::"bank.wire")
when { context.weekday == "saturday" || context.weekday == "sunday" };

@id("big-refunds-need-review")
@veto("review")
permit (principal, action, resource == Tool::"stripe.refund")
when { context.args has amount_cents && context.args.amount_cents > 50000 };

@id("reads-skip-model")
@veto("allow")
permit (principal, action, resource)
when { resource.risk_class == "read" };

@id("support-bot-small-refunds")
@veto("allow")
permit (principal, action, resource == Tool::"stripe.refund")
when { principal.name == "support-bot" && context.args has amount_cents && context.args.amount_cents <= 2000 };
`

func eval(t *testing.T, s *Set, tool domain.Tool, agentName, args string, now time.Time) Result {
	t.Helper()
	v, err := canonical.Decode([]byte(args))
	if err != nil {
		t.Fatal(err)
	}
	return s.Evaluate(Input{
		Agent: Agent{ID: "agt_1", Name: agentName},
		Tool:  tool,
		Args:  v,
		Now:   now,
	})
}

func TestEvaluate(t *testing.T) {
	s, err := Compile(testPolicies, 1)
	if err != nil {
		t.Fatal(err)
	}
	monday := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	saturday := time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)
	refund := domain.Tool{Name: "stripe.refund", RiskClass: domain.RiskHigh, Registered: true}
	wire := domain.Tool{Name: "bank.wire", RiskClass: domain.RiskCritical, Registered: true}
	read := domain.Tool{Name: "crm.lookup", RiskClass: domain.RiskRead, Registered: true}

	tests := []struct {
		name     string
		tool     domain.Tool
		agent    string
		args     string
		now      time.Time
		want     Outcome
		policies []string
	}{
		{"forbid wins", wire, "ops", `{"amount":10}`, saturday, OutcomeDeny, []string{"no-wires-on-weekends"}},
		{"weekday wire continues", wire, "ops", `{"amount":10}`, monday, OutcomeContinue, nil},
		{"big refund reviewed", refund, "support-bot", `{"amount_cents":90000}`, monday, OutcomeReview, []string{"big-refunds-need-review"}},
		{"small refund allowed for support bot", refund, "support-bot", `{"amount_cents":1500}`, monday, OutcomeAllow, []string{"support-bot-small-refunds"}},
		{"small refund from another agent continues", refund, "sales-bot", `{"amount_cents":1500}`, monday, OutcomeContinue, nil},
		{"read allowed", read, "any", `{"q":"x"}`, monday, OutcomeAllow, []string{"reads-skip-model"}},
		{"missing arg does not error", refund, "support-bot", `{}`, monday, OutcomeContinue, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eval(t, s, tt.tool, tt.agent, tt.args, tt.now)
			if got.Outcome != tt.want {
				t.Fatalf("outcome %s want %s (%+v)", got.Outcome, tt.want, got)
			}
			if strings.Join(got.Policies, ",") != strings.Join(tt.policies, ",") {
				t.Errorf("policies %v want %v", got.Policies, tt.policies)
			}
		})
	}
}

func TestReviewBeatsAllow(t *testing.T) {
	s, err := Compile(`
@id("a") @veto("allow") permit (principal, action, resource);
@id("r") @veto("review") permit (principal, action, resource);
`, 1)
	if err != nil {
		t.Fatal(err)
	}
	got := eval(t, s, domain.Tool{Name: "x", RiskClass: domain.RiskLow}, "a", `{}`, time.Now())
	if got.Outcome != OutcomeReview {
		t.Fatalf("got %s", got.Outcome)
	}
}

func TestErroringForbidHoldsForReview(t *testing.T) {
	// Comparing a string with a number is a runtime type error in Cedar.
	s, err := Compile(`@id("f") forbid (principal, action, resource) when { context.args.amount > 5 };`, 1)
	if err != nil {
		t.Fatal(err)
	}
	got := eval(t, s, domain.Tool{Name: "x", RiskClass: domain.RiskLow}, "a", `{"amount":"lots"}`, time.Now())
	if got.Outcome != OutcomeReview || len(got.Errors) != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestCompileRejectsUnannotatedPermit(t *testing.T) {
	_, err := Compile("permit (principal, action, resource);", 1)
	var ce *CompileError
	if !errors.As(err, &ce) || !strings.Contains(ce.Diagnostics[0].Message, `@veto("allow")`) {
		t.Fatalf("got %v", err)
	}
	if ce.Diagnostics[0].Line != 1 {
		t.Errorf("line %d", ce.Diagnostics[0].Line)
	}
}

func TestCompileReportsSyntaxErrorPosition(t *testing.T) {
	_, err := Compile("@id(\"x\")\n@veto(\"allow\")\npermit (principal, action, resource) when { context.args. };", 1)
	var ce *CompileError
	if !errors.As(err, &ce) {
		t.Fatalf("got %v", err)
	}
	if ce.Diagnostics[0].Line != 3 {
		t.Errorf("diagnostic %+v", ce.Diagnostics[0])
	}
}

func TestCompileRejectsDuplicateIDs(t *testing.T) {
	_, err := Compile(`@id("x") forbid (principal, action, resource); @id("x") forbid (principal, action, resource);`, 1)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("got %v", err)
	}
}

func TestDefaultSourceCompiles(t *testing.T) {
	s, err := Compile(DefaultSource, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Policies()) != 3 {
		t.Fatalf("got %d policies", len(s.Policies()))
	}
	unregistered := domain.UnregisteredTool("ws", "new.tool")
	if got := eval(t, s, unregistered, "a", `{}`, time.Now()); got.Outcome != OutcomeReview {
		t.Fatalf("unregistered tool: %s", got.Outcome)
	}
}
