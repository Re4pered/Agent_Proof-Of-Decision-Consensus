package poad

import (
	"fmt"
	"strings"
)

// Evaluate applies the initial deterministic PoAD policy.
// A known failed check rejects; missing/unknown required information abstains;
// acceptance requires transaction and state checks to pass, with intent/evidence
// passing or explicitly marked not required. This is not an LLM or a proof of reasoning.
func Evaluate(in VerificationInput) (Decision, error) {
	if strings.TrimSpace(in.AgentID) == "" {
		return Decision{}, fmt.Errorf("agent ID is required")
	}
	if strings.TrimSpace(in.TransactionHash) == "" {
		return Decision{}, fmt.Errorf("transaction hash is required")
	}

	d := Decision{
		AgentID:        in.AgentID,
		TransactionHash: in.TransactionHash,
		PolicyVersion:  DefaultPolicyVersion,
		Reasons:        make([]string, 0, 4),
	}

	checks := []struct {
		name     string
		status   CheckStatus
		optional bool
	}{
		{name: "transaction", status: in.Transaction},
		{name: "state", status: in.State},
		{name: "intent", status: in.Intent, optional: true},
		{name: "evidence", status: in.Evidence, optional: true},
	}

	hasFailure, hasUnknown := false, false

	for _, check := range checks {
		switch check.status {
		case Pass:
		case Fail:
			hasFailure = true

			d.Reasons = append(d.Reasons, check.name+"_check_failed")
		case Unknown:
			hasUnknown = true

			d.Reasons = append(d.Reasons, check.name+"_check_unknown")
		case NotRequired:
			if !check.optional {
				hasUnknown = true

				d.Reasons = append(d.Reasons, check.name+"_check_not_applicable")
			}
		default:
			hasUnknown = true

			d.Reasons = append(d.Reasons, check.name+"_check_invalid_status")
		}
	}

	switch {
	case hasFailure:
		d.Result = Reject
	case hasUnknown:
		d.Result = Abstain
	default:
		d.Result = Accept
	}

	return d, nil
}
