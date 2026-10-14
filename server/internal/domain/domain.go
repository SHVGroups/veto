// Package domain defines the vocabulary shared by every part of Veto: tools
// and their risk classes, actions and their lifecycle, the three options, and
// the decisions that resolve them. It holds plain data and pure rules only;
// nothing here performs I/O.
package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// RiskClass ranks how much damage a mistaken call to a tool can do. It sets
// the default thresholds a tool is held to.
type RiskClass string

const (
	RiskRead     RiskClass = "read"
	RiskLow      RiskClass = "low"
	RiskMedium   RiskClass = "medium"
	RiskHigh     RiskClass = "high"
	RiskCritical RiskClass = "critical"
)

// RiskClasses lists every risk class from least to most severe.
var RiskClasses = []RiskClass{RiskRead, RiskLow, RiskMedium, RiskHigh, RiskCritical}

// Valid reports whether r is a known risk class.
func (r RiskClass) Valid() bool {
	for _, c := range RiskClasses {
		if r == c {
			return true
		}
	}
	return false
}

// Option is one of the three ways Veto can resolve a proposed action.
type Option string

const (
	// OptionProceed runs the call exactly as the agent proposed it.
	OptionProceed Option = "proceed"
	// OptionProceedSafer runs a constrained variant built by the tool's
	// safer-variant rules.
	OptionProceedSafer Option = "proceed_safer"
	// OptionStop runs nothing.
	OptionStop Option = "stop"
)

// Options lists the three options in their canonical order.
var Options = []Option{OptionProceed, OptionProceedSafer, OptionStop}

// Valid reports whether o is one of the three options.
func (o Option) Valid() bool {
	return o == OptionProceed || o == OptionProceedSafer || o == OptionStop
}

// ActionStatus is the lifecycle state of an action.
type ActionStatus string

const (
	StatusEvaluating    ActionStatus = "evaluating"
	StatusPendingReview ActionStatus = "pending_review"
	StatusApproved      ActionStatus = "approved"
	StatusDenied        ActionStatus = "denied"
	StatusExpired       ActionStatus = "expired"
)

// Final reports whether no further decision can change the status.
func (s ActionStatus) Final() bool {
	return s == StatusApproved || s == StatusDenied || s == StatusExpired
}

// DecidedBy records which part of the system produced a decision.
type DecidedBy string

const (
	DecidedByModel    DecidedBy = "model"
	DecidedByHuman    DecidedBy = "human"
	DecidedByPolicy   DecidedBy = "policy"
	DecidedByFailsafe DecidedBy = "failsafe"
	DecidedByExpiry   DecidedBy = "expiry"
)

// Source records the surface an action arrived through.
type Source string

const (
	SourceAPI        Source = "api"
	SourceMCP        Source = "mcp"
	SourceConnection Source = "connection"
)

// OutcomeStatus records what happened after an approved action ran.
type OutcomeStatus string

const (
	OutcomeSucceeded OutcomeStatus = "succeeded"
	OutcomeFailed    OutcomeStatus = "failed"
	OutcomeReversed  OutcomeStatus = "reversed"
	OutcomeComplaint OutcomeStatus = "complaint"
)

// Valid reports whether s is a known outcome status.
func (s OutcomeStatus) Valid() bool {
	switch s {
	case OutcomeSucceeded, OutcomeFailed, OutcomeReversed, OutcomeComplaint:
		return true
	}
	return false
}

// AuditVerdict is a reviewer's judgment of an automatic decision after the
// fact. Anything other than VerdictCorrect counts as a false approval.
type AuditVerdict string

const (
	VerdictCorrect       AuditVerdict = "correct"
	VerdictShouldBeSafer AuditVerdict = "should_be_safer"
	VerdictShouldHold    AuditVerdict = "should_hold"
	VerdictShouldBlock   AuditVerdict = "should_block"
)

// Valid reports whether v is a known verdict.
func (v AuditVerdict) Valid() bool {
	switch v {
	case VerdictCorrect, VerdictShouldBeSafer, VerdictShouldHold, VerdictShouldBlock:
		return true
	}
	return false
}

// FailMode decides what happens to a tool's calls when the decision model
// cannot be reached.
type FailMode string

const (
	// FailClosed holds calls for a human when the model is unavailable.
	FailClosed FailMode = "closed"
	// FailOpen lets calls proceed and records that no model judged them.
	FailOpen FailMode = "open"
)

// Thresholds are the numbers a tool's automatic decisions are held to.
type Thresholds struct {
	// AutoConfidence is the minimum disposition confidence for Veto to run
	// option A or B without a human.
	AutoConfidence float64 `json:"auto_confidence"`
	// RiskCeiling is the maximum composite risk for an automatic proceed.
	RiskCeiling float64 `json:"risk_ceiling"`
	// BlockConfidence is the minimum confidence for Veto to stop a call
	// without a human.
	BlockConfidence float64 `json:"block_confidence"`
	// DoubleCheck asks the model twice with the options reversed and requires
	// both answers to agree before acting automatically.
	DoubleCheck bool `json:"double_check"`
	// AutoProceed is false for critical tools: a human must approve any run.
	AutoProceed bool `json:"auto_proceed"`
}

// DefaultThresholds returns the thresholds for a risk class. They are
// deliberately conservative; the calibration report shows where a workspace
// can loosen them with evidence.
func DefaultThresholds(c RiskClass) Thresholds {
	switch c {
	case RiskRead:
		return Thresholds{AutoConfidence: 0.40, RiskCeiling: 0.85, BlockConfidence: 0.80, AutoProceed: true}
	case RiskLow:
		return Thresholds{AutoConfidence: 0.60, RiskCeiling: 0.60, BlockConfidence: 0.80, AutoProceed: true}
	case RiskMedium:
		return Thresholds{AutoConfidence: 0.75, RiskCeiling: 0.45, BlockConfidence: 0.85, AutoProceed: true}
	case RiskHigh:
		return Thresholds{AutoConfidence: 0.85, RiskCeiling: 0.30, BlockConfidence: 0.90, DoubleCheck: true, AutoProceed: true}
	default:
		return Thresholds{AutoConfidence: 1, RiskCeiling: 0, BlockConfidence: 0.95, DoubleCheck: true, AutoProceed: false}
	}
}

// Tool is a workspace's profile of one tool its agents can call.
type Tool struct {
	WorkspaceID string    `json:"-"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	RiskClass   RiskClass `json:"risk_class"`
	Reversible  bool      `json:"reversible"`
	// Pack names the question pack used to judge this tool's calls.
	Pack string `json:"pack"`
	// Safer is the ordered list of rules that build option B.
	Safer json.RawMessage `json:"safer"`
	// Overrides replace individual values from DefaultThresholds when set.
	AutoConfidence  *float64  `json:"auto_confidence"`
	RiskCeiling     *float64  `json:"risk_ceiling"`
	BlockConfidence *float64  `json:"block_confidence"`
	DoubleCheck     *bool     `json:"double_check"`
	FailMode        FailMode  `json:"fail_mode"`
	MaxPerHour      *int      `json:"max_per_hour"`
	StepUp          bool      `json:"step_up"`
	Registered      bool      `json:"registered"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Thresholds returns the tool's effective thresholds.
func (t Tool) Thresholds() Thresholds {
	th := DefaultThresholds(t.RiskClass)
	if t.AutoConfidence != nil {
		th.AutoConfidence = *t.AutoConfidence
	}
	if t.RiskCeiling != nil {
		th.RiskCeiling = *t.RiskCeiling
	}
	if t.BlockConfidence != nil {
		th.BlockConfidence = *t.BlockConfidence
	}
	if t.DoubleCheck != nil {
		th.DoubleCheck = *t.DoubleCheck
	}
	return th
}

// UnregisteredTool is the profile Veto applies to a tool it has never seen.
// Unknown tools are treated as high risk until someone classifies them.
func UnregisteredTool(workspaceID, name string) Tool {
	return Tool{
		WorkspaceID: workspaceID,
		Name:        name,
		RiskClass:   RiskHigh,
		Pack:        "general",
		Safer:       json.RawMessage("[]"),
		FailMode:    FailClosed,
		Registered:  false,
	}
}

// Untrusted is content the agent read while working that did not come from
// the person it acts for: emails, web pages, tool output. It is shown to the
// model labelled as untrusted so instructions inside it can be spotted.
type Untrusted struct {
	Source  string `json:"source"`
	Content string `json:"content"`
}

// Principal identifies the person or system an agent acts for.
type Principal struct {
	ID      string `json:"id"`
	Display string `json:"display"`
}

// OptionSpec is one concrete, runnable form of an option.
type OptionSpec struct {
	Key       Option          `json:"key"`
	Available bool            `json:"available"`
	Tool      string          `json:"tool,omitempty"`
	Args      json.RawMessage `json:"args,omitempty"`
	// Changes describes how the option differs from the proposed call, one
	// line per change, for reviewers.
	Changes []string `json:"changes,omitempty"`
}

// Reason is a machine-readable explanation attached to a decision.
type Reason struct {
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

func (r Reason) String() string {
	if r.Detail == "" {
		return r.Code
	}
	return fmt.Sprintf("%s: %s", r.Code, r.Detail)
}

// Reason codes. The console maps each to a label; adding a code means adding
// its label there too.
const (
	ReasonPolicyForbid      = "policy_forbid"
	ReasonPolicyReview      = "policy_review"
	ReasonPolicyAllow       = "policy_allow"
	ReasonRateLimited       = "rate_limited"
	ReasonModelUnavailable  = "model_unavailable"
	ReasonModelDisabled     = "model_not_configured"
	ReasonFailOpen          = "fail_open"
	ReasonLowConfidence     = "low_confidence"
	ReasonRiskAboveCeiling  = "risk_above_ceiling"
	ReasonGuard             = "guard"
	ReasonRecheckDisagreed  = "recheck_disagreed"
	ReasonCriticalTool      = "critical_tool"
	ReasonUnregisteredTool  = "unregistered_tool"
	ReasonConfident         = "model_confident"
	ReasonAgentDisabled     = "agent_disabled"
	ReasonReviewerDecision  = "reviewer_decision"
	ReasonReviewExpired     = "review_expired"
	ReasonSaferUnavailable  = "safer_unavailable"
	ReasonOptionUnavailable = "option_unavailable"
)
