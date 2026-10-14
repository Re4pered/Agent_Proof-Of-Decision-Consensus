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
