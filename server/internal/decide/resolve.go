package decide

import (
	"errors"
	"fmt"
	"slices"

	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/jev"
	"github.com/shvgroups/veto/server/internal/policy"
)

// Route says who makes the final call on an action.
type Route string

const (
	// RouteAuto means Veto decided without a person.
	RouteAuto Route = "auto"
	// RouteHuman means the action waits for a reviewer.
	RouteHuman Route = "human"
)

// Inputs is everything the decision rules look at.
type Inputs struct {
	Gate            policy.Result
	RateLimited     bool
	Thresholds      domain.Thresholds
	FailMode        domain.FailMode
	SaferAvailable  bool
	ModelConfigured bool
	Assessment      *Assessment
	ModelErr        error
}

// Verdict is the outcome of the decision rules.
type Verdict struct {
	Route     Route            `json:"route"`
	Option    domain.Option    `json:"option,omitempty"`
	DecidedBy domain.DecidedBy `json:"decided_by,omitempty"`
	Reasons   []domain.Reason  `json:"reasons"`
	// Ranking orders the options for a reviewer, most recommended first.
	Ranking []domain.Option `json:"ranking"`
}

// NeedsModel reports whether the decision rules can use a model assessment
// for this gate result. A deny or allow from policy is final, so the model is
// not called and nothing is spent.
func NeedsModel(gate policy.Result) bool {
	return gate.Outcome != policy.OutcomeDeny && gate.Outcome != policy.OutcomeAllow
}

// Resolve applies Veto's decision rules. The order of the checks is the
// specification:
//
//  1. A forbid policy stops the call.
//  2. A rate limit or a review policy sends the call to a person.
//  3. An allow policy runs the call.
//  4. Without a usable model answer, the tool's fail mode decides.
//  5. A failed double check sends the call to a person.
//  6. The model's choice runs automatically only if it clears the tool's
//     thresholds and no guard forbids it; otherwise a person decides.
func Resolve(in Inputs) Verdict {
	ranking := defaultRanking(in.SaferAvailable)
	if in.Assessment != nil {
		ranking = in.Assessment.Ranking()
	}
	human := func(reasons ...domain.Reason) Verdict {
		return Verdict{Route: RouteHuman, Reasons: reasons, Ranking: ranking}
	}
	auto := func(o domain.Option, by domain.DecidedBy, reasons ...domain.Reason) Verdict {
		return Verdict{Route: RouteAuto, Option: o, DecidedBy: by, Reasons: reasons, Ranking: ranking}
	}

	switch in.Gate.Outcome {
	case policy.OutcomeDeny:
		return auto(domain.OptionStop, domain.DecidedByPolicy, policyReasons(domain.ReasonPolicyForbid, in.Gate.Policies)...)
	case policy.OutcomeReview:
		reasons := policyReasons(domain.ReasonPolicyReview, in.Gate.Policies)
		if in.RateLimited {
			reasons = append(reasons, domain.Reason{Code: domain.ReasonRateLimited})
		}
		return human(reasons...)
	}
	if in.RateLimited {
		return human(domain.Reason{Code: domain.ReasonRateLimited})
	}
	if in.Gate.Outcome == policy.OutcomeAllow {
		return auto(domain.OptionProceed, domain.DecidedByPolicy, policyReasons(domain.ReasonPolicyAllow, in.Gate.Policies)...)
	}

	a := in.Assessment
	if a == nil {
		reason := domain.Reason{Code: domain.ReasonModelUnavailable, Detail: errorDetail(in.ModelErr)}
		if !in.ModelConfigured {
			reason = domain.Reason{Code: domain.ReasonModelDisabled}
		}
		if in.FailMode == domain.FailOpen && in.Thresholds.AutoProceed {
			return auto(domain.OptionProceed, domain.DecidedByFailsafe, reason, domain.Reason{Code: domain.ReasonFailOpen})
		}
		return human(reason)
	}

	if a.Recheck != nil && !a.Recheck.Agrees {
		detail := "recheck failed"
		if a.Recheck.Choice != "" {
			detail = fmt.Sprintf("first %s, then %s", a.Choice, a.Recheck.Choice)
		}
		return human(domain.Reason{Code: domain.ReasonRecheckDisagreed, Detail: detail})
	}

	th := in.Thresholds
	conf := a.Confidence
	if a.Recheck != nil && a.Recheck.Confidence < conf {
		// Two answers that agree are only as confident as the weaker one.
		conf = a.Recheck.Confidence
	}

	if a.Choice == domain.OptionStop {
		if conf >= th.BlockConfidence {
			return auto(domain.OptionStop, domain.DecidedByModel, domain.Reason{
				Code: domain.ReasonConfident, Detail: fmt.Sprintf("stop at confidence %.2f", conf),
			})
		}
		return human(domain.Reason{Code: domain.ReasonLowConfidence, Detail: fmt.Sprintf("%.2f below %.2f to stop", conf, th.BlockConfidence)})
	}

	var reasons []domain.Reason
	if !th.AutoProceed {
		reasons = append(reasons, domain.Reason{Code: domain.ReasonCriticalTool})
	}
	for _, g := range a.Guards {
		if slices.Contains(g.Forbid, a.Choice) {
			reasons = append(reasons, domain.Reason{Code: domain.ReasonGuard, Detail: g.Reason})
		}
	}
	if conf < th.AutoConfidence {
		reasons = append(reasons, domain.Reason{Code: domain.ReasonLowConfidence, Detail: fmt.Sprintf("%.2f below %.2f", conf, th.AutoConfidence)})
	}
	if a.Risk > th.RiskCeiling {
		reasons = append(reasons, domain.Reason{Code: domain.ReasonRiskAboveCeiling, Detail: fmt.Sprintf("%.2f above %.2f", a.Risk, th.RiskCeiling)})
	}
	if a.Choice == domain.OptionProceedSafer && !in.SaferAvailable {
		reasons = append(reasons, domain.Reason{Code: domain.ReasonSaferUnavailable})
	}
	if len(reasons) > 0 {
		return human(reasons...)
	}
	return auto(a.Choice, domain.DecidedByModel, domain.Reason{
		Code:   domain.ReasonConfident,
		Detail: fmt.Sprintf("%s at confidence %.2f, risk %.2f", a.Choice, conf, a.Risk),
	})
}

func defaultRanking(saferAvailable bool) []domain.Option {
	if saferAvailable {
		return []domain.Option{domain.OptionProceedSafer, domain.OptionProceed, domain.OptionStop}
	}
	return []domain.Option{domain.OptionProceed, domain.OptionStop}
}

func policyReasons(code string, ids []string) []domain.Reason {
	if len(ids) == 0 {
		return []domain.Reason{{Code: code}}
	}
	out := make([]domain.Reason, len(ids))
	for i, id := range ids {
		out[i] = domain.Reason{Code: code, Detail: id}
	}
	return out
}

func errorDetail(err error) string {
	if err == nil {
		return ""
	}
	var apiErr *jev.APIError
	if errors.As(err, &apiErr) {
		return fmt.Sprintf("HTTP %d from %s", apiErr.StatusCode, apiErr.Endpoint)
	}
	if errors.Is(err, jev.ErrNoEndpoints) {
		return "no endpoints"
	}
	msg := err.Error()
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return msg
}
