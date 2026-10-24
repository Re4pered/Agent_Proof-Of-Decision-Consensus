// Package poad contains the first, deterministic core of Proof of Agent Decision (PoAD).
//
// This package deliberately has no PBFT/network dependency. It defines the decision
// contract and a rule-based evaluator that can be unit-tested before integration.
package poad

// Result is an Agent Validator's decision for one transaction.
type Result string

const (
	Accept  Result = "ACCEPT"
	Reject  Result = "REJECT"
	Abstain Result = "ABSTAIN"
)

// CheckStatus records the outcome of one verification dimension.
type CheckStatus string

const (
	Pass        CheckStatus = "PASS"
	Fail        CheckStatus = "FAIL"
	Unknown     CheckStatus = "UNKNOWN"
	NotRequired CheckStatus = "NOT_REQUIRED"
)

// VerificationInput contains verification facts supplied to the PoAD evaluator.
// Callers must populate statuses from explicit validation logic; this evaluator does
// not infer intent or independently establish the trustworthiness of external evidence.
type VerificationInput struct {
	AgentID         string
	TransactionHash string
	StateRoot       string
	Transaction     CheckStatus
	State           CheckStatus
	Intent          CheckStatus
	Evidence        CheckStatus
}

// Decision is the deterministic decision record produced by the evaluator.
// Reasons are stable machine-readable codes for experiment aggregation.
type Decision struct {
	AgentID         string
	TransactionHash string
	StateRoot       string
	Result          Result
	Reasons         []string
	PolicyVersion   string
}

const DefaultPolicyVersion = "poad-rules-v1"
