package poad

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// BlockFinalizationRecord binds an accepted Agent decision quorum to a transaction
// and the deterministic state root produced by the ordinary execution path.
type BlockFinalizationRecord struct {
	TransactionHash   string `json:"transaction_hash"`
	PolicyVersion     string `json:"policy_version"`
	Result            Result `json:"result"`
	StateRoot         string `json:"state_root"`
	AcceptVotes       int    `json:"accept_votes"`
	RejectVotes       int    `json:"reject_votes"`
	AbstainVotes      int    `json:"abstain_votes"`
	RequiredVotes     int    `json:"required_votes"`
	DecisionSetSHA256 string `json:"decision_set_sha256"`
}

// PrepareBlockFinalization creates a record only when authenticated Agent decisions
// reach an ACCEPT quorum. stateRoot must come from the normal deterministic execution
// and state-transition validation path; this function does not execute transactions.
func PrepareBlockFinalization(
	registry *IdentityRegistry,
	decisions []SignedDecision,
	transactionHash string,
	policyVersion string,
	requiredVotes int,
	stateRoot string,
) (BlockFinalizationRecord, error) {
	if strings.TrimSpace(stateRoot) == "" {
		return BlockFinalizationRecord{}, fmt.Errorf("state root is required")
	}

	summary, err := AggregateSignedDecisions(
		registry,
		decisions,
		transactionHash,
		policyVersion,
		requiredVotes,
	)
	if err != nil {
		return BlockFinalizationRecord{}, fmt.Errorf("aggregate Agent decisions: %w", err)
	}

	if summary.Result != Accept {
		return BlockFinalizationRecord{}, fmt.Errorf(
			"cannot finalize transaction without ACCEPT quorum: %s",
			summary.Result,
		)
	}

	digest, err := decisionSetDigest(decisions)
	if err != nil {
		return BlockFinalizationRecord{}, err
	}

	return BlockFinalizationRecord{
		TransactionHash:   summary.TransactionHash,
		PolicyVersion:     summary.PolicyVersion,
		Result:            summary.Result,
		StateRoot:         stateRoot,
		AcceptVotes:       summary.AcceptVotes,
		RejectVotes:       summary.RejectVotes,
		AbstainVotes:      summary.AbstainVotes,
		RequiredVotes:     summary.RequiredVotes,
		DecisionSetSHA256: digest,
	}, nil
}

// VerifyBlockFinalization recomputes the quorum and signed-decision-set digest instead
// of trusting the record's counters. Callers must still run ordinary block, transaction,
// and state-transition validation before accepting the resulting block.
func VerifyBlockFinalization(
	registry *IdentityRegistry,
	decisions []SignedDecision,
	record BlockFinalizationRecord,
	expectedStateRoot string,
) error {
	if strings.TrimSpace(expectedStateRoot) == "" {
		return fmt.Errorf("expected state root is required")
	}

	if record.StateRoot != expectedStateRoot {
		return fmt.Errorf("finalization state root does not match independently validated state root")
	}

	for i, signed := range decisions {
		if signed.Decision.StateRoot != expectedStateRoot {
			return fmt.Errorf("decision %d is bound to a different state root", i)
		}
	}

	expected, err := PrepareBlockFinalization(
		registry,
		decisions,
		record.TransactionHash,
		record.PolicyVersion,
		record.RequiredVotes,
		record.StateRoot,
	)
	if err != nil {
		return fmt.Errorf("recompute block finalization: %w", err)
	}

	if expected != record {
		return fmt.Errorf("block finalization record does not match verified decisions")
	}

	return nil
}

// decisionSetDigest hashes a canonicalized ordering of the signed decision records.
// Decision contents remain exactly those covered by each Agent's signature.
func decisionSetDigest(decisions []SignedDecision) (string, error) {
	canonical := append([]SignedDecision(nil), decisions...)
	sort.Slice(canonical, func(i, j int) bool {
		return canonical[i].Decision.AgentID < canonical[j].Decision.AgentID
	})

	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("marshal signed decision set: %w", err)
	}

	digest := sha256.Sum256(payload)

	return hex.EncodeToString(digest[:]), nil
}
