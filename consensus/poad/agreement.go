package poad

import (
	"fmt"
	"strings"
)

// AgreementSummary records the result of aggregating authenticated Agent decisions.
// RequiredVotes is an explicit policy input; this helper does not determine committee
// membership or prove that a chosen threshold satisfies Byzantine fault assumptions.
type AgreementSummary struct {
	TransactionHash string
	PolicyVersion   string
	Result          Result
	AcceptVotes     int
	RejectVotes     int
	AbstainVotes    int
	ValidVotes      int
	RequiredVotes   int
	Reasons         []string
}

// AggregateSignedDecisions verifies each signature against the trusted registry, rejects
// duplicate AgentIDs and mismatched transaction/policy records, then applies a fixed vote
// threshold. It does not replace PBFT or establish a globally consistent committee view.
func AggregateSignedDecisions(
	registry *IdentityRegistry,
	decisions []SignedDecision,
	transactionHash string,
	policyVersion string,
	requiredVotes int,
) (AgreementSummary, error) {
	if registry == nil {
		return AgreementSummary{}, fmt.Errorf("identity registry is required")
	}

	if strings.TrimSpace(transactionHash) == "" {
		return AgreementSummary{}, fmt.Errorf("transaction hash is required")
	}

	if strings.TrimSpace(policyVersion) == "" {
		return AgreementSummary{}, fmt.Errorf("policy version is required")
	}

	if requiredVotes < 1 {
		return AgreementSummary{}, fmt.Errorf("required votes must be positive")
	}

	summary := AgreementSummary{
		TransactionHash: transactionHash,
		PolicyVersion:   policyVersion,
		Result:          Abstain,
		RequiredVotes:   requiredVotes,
		Reasons:         make([]string, 0, 1),
	}
	seen := make(map[string]struct{}, len(decisions))

	for _, signed := range decisions {
		decision := signed.Decision
		if err := registry.VerifyDecision(signed); err != nil {
			return AgreementSummary{}, fmt.Errorf("verify decision from %q: %w", decision.AgentID, err)
		}

		if decision.TransactionHash != transactionHash {
			return AgreementSummary{}, fmt.Errorf("decision from %q references a different transaction", decision.AgentID)
		}

		if decision.PolicyVersion != policyVersion {
			return AgreementSummary{}, fmt.Errorf("decision from %q uses a different policy version", decision.AgentID)
		}

		if _, exists := seen[decision.AgentID]; exists {
			return AgreementSummary{}, fmt.Errorf("duplicate decision from AgentID %q", decision.AgentID)
		}
		seen[decision.AgentID] = struct{}{}

		switch decision.Result {
		case Accept:
			summary.AcceptVotes++
		case Reject:
			summary.RejectVotes++
		case Abstain:
			summary.AbstainVotes++
		default:
			return AgreementSummary{}, fmt.Errorf("decision from %q has invalid result %q", decision.AgentID, decision.Result)
		}
	}

	summary.ValidVotes = len(seen)

	acceptQuorum := summary.AcceptVotes >= requiredVotes
	rejectQuorum := summary.RejectVotes >= requiredVotes

	switch {
	case acceptQuorum && rejectQuorum:
		summary.Reasons = append(summary.Reasons, "conflicting_quorums")
	case acceptQuorum:
		summary.Result = Accept
	case rejectQuorum:
		summary.Result = Reject
	default:
		summary.Reasons = append(summary.Reasons, "quorum_not_reached")
	}

	return summary, nil
}
