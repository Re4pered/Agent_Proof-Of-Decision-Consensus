package poad

import (
	"testing"
)

func signedTestDecision(t *testing.T, registry *IdentityRegistry, agentID string, result Result, txHash, policy string, stateRoot ...string) SignedDecision {
	t.Helper()

	publicKey, privateKey := newTestKeyPair(t)
	root := ""
	if len(stateRoot) > 0 {
		root = stateRoot[0]
	}

	if err := registry.Register(agentID, publicKey); err != nil {
		t.Fatal(err)
	}

	signed, err := SignDecision(Decision{
		AgentID:         agentID,
		TransactionHash: txHash,
		StateRoot:       root,
		Result:          result,
		PolicyVersion:   policy,
	}, privateKey)
	if err != nil {
		t.Fatal(err)
	}

	return signed
}

func TestAggregateSignedDecisionsAcceptQuorum(t *testing.T) {
	registry := NewIdentityRegistry()
	decisions := []SignedDecision{
		signedTestDecision(t, registry, "agent-1", Accept, "tx-1", DefaultPolicyVersion),
		signedTestDecision(t, registry, "agent-2", Accept, "tx-1", DefaultPolicyVersion),
		signedTestDecision(t, registry, "agent-3", Abstain, "tx-1", DefaultPolicyVersion),
	}

	got, err := AggregateSignedDecisions(registry, decisions, "tx-1", DefaultPolicyVersion, 2)
	if err != nil {
		t.Fatal(err)
	}

	if got.Result != Accept || got.AcceptVotes != 2 || got.AbstainVotes != 1 || got.ValidVotes != 3 {
		t.Fatalf("unexpected agreement summary: %+v", got)
	}
}

func TestAggregateSignedDecisionsRejectQuorum(t *testing.T) {
	registry := NewIdentityRegistry()
	decisions := []SignedDecision{
		signedTestDecision(t, registry, "agent-1", Reject, "tx-1", DefaultPolicyVersion),
		signedTestDecision(t, registry, "agent-2", Reject, "tx-1", DefaultPolicyVersion),
	}

	got, err := AggregateSignedDecisions(registry, decisions, "tx-1", DefaultPolicyVersion, 2)
	if err != nil {
		t.Fatal(err)
	}

	if got.Result != Reject || got.RejectVotes != 2 {
		t.Fatalf("unexpected agreement summary: %+v", got)
	}
}

func TestAggregateSignedDecisionsAbstainsWithoutQuorum(t *testing.T) {
	registry := NewIdentityRegistry()
	decisions := []SignedDecision{
		signedTestDecision(t, registry, "agent-1", Accept, "tx-1", DefaultPolicyVersion),
		signedTestDecision(t, registry, "agent-2", Reject, "tx-1", DefaultPolicyVersion),
	}

	got, err := AggregateSignedDecisions(registry, decisions, "tx-1", DefaultPolicyVersion, 2)
	if err != nil {
		t.Fatal(err)
	}

	if got.Result != Abstain || len(got.Reasons) != 1 || got.Reasons[0] != "quorum_not_reached" {
		t.Fatalf("unexpected agreement summary: %+v", got)
	}
}

func TestAggregateSignedDecisionsRejectsDuplicateAgent(t *testing.T) {
	registry := NewIdentityRegistry()
	vote := signedTestDecision(t, registry, "agent-1", Accept, "tx-1", DefaultPolicyVersion)

	if _, err := AggregateSignedDecisions(registry, []SignedDecision{vote, vote}, "tx-1", DefaultPolicyVersion, 2); err == nil {
		t.Fatal("expected duplicate AgentID to be rejected")
	}
}

func TestAggregateSignedDecisionsRejectsMismatchedTransaction(t *testing.T) {
	registry := NewIdentityRegistry()
	vote := signedTestDecision(t, registry, "agent-1", Accept, "tx-other", DefaultPolicyVersion)

	if _, err := AggregateSignedDecisions(registry, []SignedDecision{vote}, "tx-1", DefaultPolicyVersion, 1); err == nil {
		t.Fatal("expected transaction hash mismatch to be rejected")
	}
}

func TestAggregateSignedDecisionsRejectsMismatchedPolicy(t *testing.T) {
	registry := NewIdentityRegistry()
	vote := signedTestDecision(t, registry, "agent-1", Accept, "tx-1", "poad-rules-old")

	if _, err := AggregateSignedDecisions(registry, []SignedDecision{vote}, "tx-1", DefaultPolicyVersion, 1); err == nil {
		t.Fatal("expected policy version mismatch to be rejected")
	}
}

func TestAggregateSignedDecisionsRequiresPositiveThreshold(t *testing.T) {
	registry := NewIdentityRegistry()
	if _, err := AggregateSignedDecisions(registry, nil, "tx-1", DefaultPolicyVersion, 0); err == nil {
		t.Fatal("expected non-positive threshold to be rejected")
	}
}
