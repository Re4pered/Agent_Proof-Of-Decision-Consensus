package poad

import (
	"fmt"
	"strings"
	"sync"
)

// ReputationConfig defines deterministic, bounded score updates.
// Scores are prototype policy parameters and must be agreed by all validators before use.
type ReputationConfig struct {
	InitialScore int64
	MinScore     int64
	MaxScore     int64
	Reward       int64
	Penalty      int64
}

// ReputationRecord is the current in-memory reputation for one Agent.
type ReputationRecord struct {
	AgentID string
	Score   int64
	Updates uint64
}

// ReputationLedger applies at most one reputation update per Agent, transaction, and policy version.
// This in-memory implementation is a local prototype, not replicated consensus state.
type ReputationLedger struct {
	mu        sync.RWMutex
	config    ReputationConfig
	records   map[string]ReputationRecord
	processed map[string]struct{}
}

// NewReputationLedger creates an empty ledger after validating its scoring policy.
func NewReputationLedger(config ReputationConfig) (*ReputationLedger, error) {
	if config.MinScore < 0 || config.MaxScore <= config.MinScore {
		return nil, fmt.Errorf("invalid reputation score bounds")
	}

	if config.InitialScore < config.MinScore || config.InitialScore > config.MaxScore {
		return nil, fmt.Errorf("initial reputation score is outside configured bounds")
	}

	if config.Reward < 0 || config.Penalty < 0 {
		return nil, fmt.Errorf("reputation reward and penalty must be non-negative")
	}

	return &ReputationLedger{
		config:    config,
		records:   make(map[string]ReputationRecord),
		processed: make(map[string]struct{}),
	}, nil
}

// ApplyFinalizedDecision updates reputation only after a final ACCEPT or REJECT outcome.
// Matching votes earn Reward, conflicting votes incur Penalty, and ABSTAIN is neutral.
// Duplicate Agent/transaction/policy events are rejected to prevent repeated score changes.
func (l *ReputationLedger) ApplyFinalizedDecision(
	decision Decision,
	finalizedResult Result,
) (ReputationRecord, error) {
	if l == nil {
		return ReputationRecord{}, fmt.Errorf("reputation ledger is required")
	}

	if strings.TrimSpace(decision.AgentID) == "" {
		return ReputationRecord{}, fmt.Errorf("agent ID is required")
	}

	if strings.TrimSpace(decision.TransactionHash) == "" {
		return ReputationRecord{}, fmt.Errorf("transaction hash is required")
	}

	if strings.TrimSpace(decision.PolicyVersion) == "" {
		return ReputationRecord{}, fmt.Errorf("policy version is required")
	}

	if finalizedResult != Accept && finalizedResult != Reject {
		return ReputationRecord{}, fmt.Errorf("finalized result must be ACCEPT or REJECT")
	}

	if decision.Result != Accept && decision.Result != Reject && decision.Result != Abstain {
		return ReputationRecord{}, fmt.Errorf("invalid Agent decision result %q", decision.Result)
	}

	eventKey := decision.AgentID + "\x00" + decision.TransactionHash + "\x00" + decision.PolicyVersion

	l.mu.Lock()
	defer l.mu.Unlock()

	if _, exists := l.processed[eventKey]; exists {
		return ReputationRecord{}, fmt.Errorf("reputation event already applied for AgentID %q", decision.AgentID)
	}

	record, exists := l.records[decision.AgentID]
	if !exists {
		record = ReputationRecord{AgentID: decision.AgentID, Score: l.config.InitialScore}
	}

	switch decision.Result {
	case Abstain:
		// Abstaining is neutral: it neither rewards nor punishes the Agent.
	case finalizedResult:
		record.Score = increaseScore(record.Score, l.config.Reward, l.config.MaxScore)
	default:
		record.Score = decreaseScore(record.Score, l.config.Penalty, l.config.MinScore)
	}

	record.Updates++
	l.records[decision.AgentID] = record
	l.processed[eventKey] = struct{}{}

	return record, nil
}

// ApplyFinalizedDecisions verifies the signed decision set and finalization record before
// atomically applying reputation updates for all participating Agents. The state root must
// already have been independently validated by the caller. This local ledger is not replicated.
func (l *ReputationLedger) ApplyFinalizedDecisions(
	registry *IdentityRegistry,
	decisions []SignedDecision,
	finalization BlockFinalizationRecord,
	expectedStateRoot string,
) ([]ReputationRecord, error) {
	if l == nil {
		return nil, fmt.Errorf("reputation ledger is required")
	}

	if err := VerifyBlockFinalization(registry, decisions, finalization, expectedStateRoot); err != nil {
		return nil, fmt.Errorf("verify block finalization before reputation update: %w", err)
	}

	eventKeys := make([]string, len(decisions))
	for i, signed := range decisions {
		decision := signed.Decision
		if strings.TrimSpace(decision.AgentID) == "" {
			return nil, fmt.Errorf("agent ID is required")
		}

		eventKeys[i] = decision.AgentID + "\x00" + decision.TransactionHash + "\x00" + decision.PolicyVersion
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	seen := make(map[string]struct{}, len(eventKeys))
	for i, eventKey := range eventKeys {
		if _, exists := seen[eventKey]; exists {
			return nil, fmt.Errorf("duplicate reputation event for AgentID %q", decisions[i].Decision.AgentID)
		}

		seen[eventKey] = struct{}{}

		if _, exists := l.processed[eventKey]; exists {
			return nil, fmt.Errorf("reputation event already applied for AgentID %q", decisions[i].Decision.AgentID)
		}
	}

	updated := make([]ReputationRecord, len(decisions))
	for i, signed := range decisions {
		decision := signed.Decision

		record, exists := l.records[decision.AgentID]
		if !exists {
			record = ReputationRecord{AgentID: decision.AgentID, Score: l.config.InitialScore}
		}

		switch decision.Result {
		case Abstain:
			// Abstention is neutral.
		case finalization.Result:
			record.Score = increaseScore(record.Score, l.config.Reward, l.config.MaxScore)
		default:
			record.Score = decreaseScore(record.Score, l.config.Penalty, l.config.MinScore)
		}

		record.Updates++
		updated[i] = record
	}

	for i, signed := range decisions {
		l.records[signed.Decision.AgentID] = updated[i]
		l.processed[eventKeys[i]] = struct{}{}
	}

	return updated, nil
}

// Get returns the current record for an Agent, or false when no finalized event was applied.
func (l *ReputationLedger) Get(agentID string) (ReputationRecord, bool) {
	if l == nil {
		return ReputationRecord{}, false
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	record, exists := l.records[agentID]

	return record, exists
}

func increaseScore(score, reward, maxScore int64) int64 {
	if reward > maxScore-score {
		return maxScore
	}

	return score + reward
}

func decreaseScore(score, penalty, minScore int64) int64 {
	if penalty > score-minScore {
		return minScore
	}

	return score - penalty
}
