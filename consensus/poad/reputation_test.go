package poad

import "testing"

func newTestReputationLedger(t *testing.T) *ReputationLedger {
	t.Helper()

	ledger, err := NewReputationLedger(ReputationConfig{
		InitialScore: 500,
		MinScore:     0,
		MaxScore:     1000,
		Reward:       25,
		Penalty:      40,
	})
	if err != nil {
		t.Fatal(err)
	}

	return ledger
}

func reputationDecision(agentID, txHash string, result Result) Decision {
	return Decision{
		AgentID:         agentID,
		TransactionHash: txHash,
		Result:          result,
		PolicyVersion:   DefaultPolicyVersion,
	}
}

func TestReputationLedgerRewardsMatchingFinalizedDecision(t *testing.T) {
	ledger := newTestReputationLedger(t)

	got, err := ledger.ApplyFinalizedDecision(reputationDecision("agent-1", "tx-1", Accept), Accept)
	if err != nil {
		t.Fatal(err)
	}

	if got.Score != 525 || got.Updates != 1 {
		t.Fatalf("unexpected reputation record: %+v", got)
	}
}

func TestReputationLedgerPenalizesConflictingDecision(t *testing.T) {
	ledger := newTestReputationLedger(t)

	got, err := ledger.ApplyFinalizedDecision(reputationDecision("agent-1", "tx-1", Accept), Reject)
	if err != nil {
		t.Fatal(err)
	}

	if got.Score != 460 {
		t.Fatalf("expected score 460, got %d", got.Score)
	}
}

func TestReputationLedgerAbstentionIsNeutral(t *testing.T) {
	ledger := newTestReputationLedger(t)

	got, err := ledger.ApplyFinalizedDecision(reputationDecision("agent-1", "tx-1", Abstain), Accept)
	if err != nil {
		t.Fatal(err)
	}

	if got.Score != 500 || got.Updates != 1 {
		t.Fatalf("unexpected reputation record: %+v", got)
	}
}

func TestReputationLedgerClampsScoreBounds(t *testing.T) {
	ledger, err := NewReputationLedger(ReputationConfig{
		InitialScore: 990,
		MinScore:     0,
		MaxScore:     1000,
		Reward:       25,
		Penalty:      40,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := ledger.ApplyFinalizedDecision(reputationDecision("agent-1", "tx-1", Accept), Accept)
	if err != nil {
		t.Fatal(err)
	}

	if got.Score != 1000 {
		t.Fatalf("expected score to clamp at 1000, got %d", got.Score)
	}
}

func TestReputationLedgerRejectsDuplicateEvent(t *testing.T) {
	ledger := newTestReputationLedger(t)
	decision := reputationDecision("agent-1", "tx-1", Accept)

	if _, err := ledger.ApplyFinalizedDecision(decision, Accept); err != nil {
		t.Fatal(err)
	}

	if _, err := ledger.ApplyFinalizedDecision(decision, Accept); err == nil {
		t.Fatal("expected duplicate reputation event to be rejected")
	}
}

func TestReputationLedgerRequiresFinalizedOutcome(t *testing.T) {
	ledger := newTestReputationLedger(t)

	if _, err := ledger.ApplyFinalizedDecision(reputationDecision("agent-1", "tx-1", Accept), Abstain); err == nil {
		t.Fatal("expected ABSTAIN final outcome to be rejected")
	}
}

func TestNewReputationLedgerRejectsInvalidConfig(t *testing.T) {
	tests := []ReputationConfig{
		{InitialScore: 500, MinScore: 10, MaxScore: 10, Reward: 1, Penalty: 1},
		{InitialScore: 1001, MinScore: 0, MaxScore: 1000, Reward: 1, Penalty: 1},
		{InitialScore: 500, MinScore: 0, MaxScore: 1000, Reward: -1, Penalty: 1},
	}

	for _, config := range tests {
		if _, err := NewReputationLedger(config); err == nil {
			t.Fatalf("expected invalid config to be rejected: %+v", config)
		}
	}
}


func TestApplyFinalizedDecisionsUpdatesOnlyAfterVerifiedFinalization(t *testing.T) {
	registry := NewIdentityRegistry()
	decisions := []SignedDecision{
		signedTestDecision(t, registry, "agent-1", Accept, "tx-1", DefaultPolicyVersion),
		signedTestDecision(t, registry, "agent-2", Accept, "tx-1", DefaultPolicyVersion),
		signedTestDecision(t, registry, "agent-3", Abstain, "tx-1", DefaultPolicyVersion),
	}
	finalization, err := PrepareBlockFinalization(
		registry,
		decisions,
		"tx-1",
		DefaultPolicyVersion,
		2,
		"state-root-123",
	)
	if err != nil {
		t.Fatal(err)
	}

	ledger := newTestReputationLedger(t)
	records, err := ledger.ApplyFinalizedDecisions(registry, decisions, finalization, "state-root-123")
	if err != nil {
		t.Fatal(err)
	}

	if len(records) != 3 {
		t.Fatalf("expected 3 reputation records, got %d", len(records))
	}

	if records[0].Score != 525 || records[1].Score != 525 || records[2].Score != 500 {
		t.Fatalf("unexpected reputation scores: %+v", records)
	}
}

func TestApplyFinalizedDecisionsRejectsInvalidStateRootWithoutPartialUpdates(t *testing.T) {
	registry := NewIdentityRegistry()
	decisions := []SignedDecision{
		signedTestDecision(t, registry, "agent-1", Accept, "tx-1", DefaultPolicyVersion),
	}
	finalization, err := PrepareBlockFinalization(
		registry,
		decisions,
		"tx-1",
		DefaultPolicyVersion,
		1,
		"state-root-123",
	)
	if err != nil {
		t.Fatal(err)
	}

	ledger := newTestReputationLedger(t)
	if _, err := ledger.ApplyFinalizedDecisions(registry, decisions, finalization, "wrong-root"); err == nil {
		t.Fatal("expected invalid state root to be rejected")
	}

	if _, exists := ledger.Get("agent-1"); exists {
		t.Fatal("reputation must not change when finalization verification fails")
	}
}

func TestApplyFinalizedDecisionsRejectsReplayWithoutPartialUpdates(t *testing.T) {
	registry := NewIdentityRegistry()
	decisions := []SignedDecision{
		signedTestDecision(t, registry, "agent-1", Accept, "tx-1", DefaultPolicyVersion),
		signedTestDecision(t, registry, "agent-2", Accept, "tx-1", DefaultPolicyVersion),
	}
	finalization, err := PrepareBlockFinalization(
		registry,
		decisions,
		"tx-1",
		DefaultPolicyVersion,
		2,
		"state-root-123",
	)
	if err != nil {
		t.Fatal(err)
	}

	ledger := newTestReputationLedger(t)
	if _, err := ledger.ApplyFinalizedDecisions(registry, decisions, finalization, "state-root-123"); err != nil {
		t.Fatal(err)
	}

	if _, err := ledger.ApplyFinalizedDecisions(registry, decisions, finalization, "state-root-123"); err == nil {
		t.Fatal("expected replayed finalization to be rejected")
	}

	for _, agentID := range []string{"agent-1", "agent-2"} {
		record, exists := ledger.Get(agentID)
		if !exists || record.Score != 525 || record.Updates != 1 {
			t.Fatalf("replayed finalization changed %s reputation: %+v", agentID, record)
		}
	}
}
