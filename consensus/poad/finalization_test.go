package poad

import (
	"testing"
)

func TestPrepareAndVerifyBlockFinalization(t *testing.T) {
	registry := NewIdentityRegistry()
	decisions := []SignedDecision{
		signedTestDecision(t, registry, "agent-1", Accept, "tx-1", DefaultPolicyVersion, "state-root-123"),
		signedTestDecision(t, registry, "agent-2", Accept, "tx-1", DefaultPolicyVersion, "state-root-123"),
		signedTestDecision(t, registry, "agent-3", Abstain, "tx-1", DefaultPolicyVersion, "state-root-123"),
	}

	record, err := PrepareBlockFinalization(
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

	if record.Result != Accept || record.AcceptVotes != 2 || record.DecisionSetSHA256 == "" {
		t.Fatalf("unexpected finalization record: %+v", record)
	}

	if err := VerifyBlockFinalization(registry, decisions, record, "state-root-123"); err != nil {
		t.Fatalf("expected record to verify: %v", err)
	}
}

func TestPrepareBlockFinalizationRejectsMissingQuorum(t *testing.T) {
	registry := NewIdentityRegistry()
	decisions := []SignedDecision{
		signedTestDecision(t, registry, "agent-1", Accept, "tx-1", DefaultPolicyVersion, "state-root-123"),
		signedTestDecision(t, registry, "agent-2", Reject, "tx-1", DefaultPolicyVersion, "state-root-123"),
	}

	if _, err := PrepareBlockFinalization(
		registry,
		decisions,
		"tx-1",
		DefaultPolicyVersion,
		2,
		"state-root-123",
	); err == nil {
		t.Fatal("expected finalization without an ACCEPT quorum to fail")
	}
}

func TestPrepareBlockFinalizationRequiresStateRoot(t *testing.T) {
	registry := NewIdentityRegistry()
	decisions := []SignedDecision{
		signedTestDecision(t, registry, "agent-1", Accept, "tx-1", DefaultPolicyVersion, "state-root-123"),
	}

	if _, err := PrepareBlockFinalization(
		registry,
		decisions,
		"tx-1",
		DefaultPolicyVersion,
		1,
		" ",
	); err == nil {
		t.Fatal("expected empty state root to be rejected")
	}
}

func TestVerifyBlockFinalizationRejectsTamperedRecord(t *testing.T) {
	registry := NewIdentityRegistry()
	decisions := []SignedDecision{
		signedTestDecision(t, registry, "agent-1", Accept, "tx-1", DefaultPolicyVersion, "state-root-123"),
	}

	record, err := PrepareBlockFinalization(
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

	record.StateRoot = "tampered-root"
	if err := VerifyBlockFinalization(registry, decisions, record, "state-root-123"); err == nil {
		t.Fatal("expected a tampered state root to be rejected")
	}
}

func TestDecisionSetDigestIsOrderIndependent(t *testing.T) {
	registry := NewIdentityRegistry()
	first := signedTestDecision(t, registry, "agent-1", Accept, "tx-1", DefaultPolicyVersion)
	second := signedTestDecision(t, registry, "agent-2", Accept, "tx-1", DefaultPolicyVersion)

	digestOne, err := decisionSetDigest([]SignedDecision{first, second})
	if err != nil {
		t.Fatal(err)
	}

	digestTwo, err := decisionSetDigest([]SignedDecision{second, first})
	if err != nil {
		t.Fatal(err)
	}

	if digestOne != digestTwo {
		t.Fatalf("decision-set digest depends on input order: %s != %s", digestOne, digestTwo)
	}
}


func TestReplicasDeriveIdenticalFinalizationFromSameDecisionSet(t *testing.T) {
	registry := NewIdentityRegistry()
	decisions := []SignedDecision{
		signedTestDecision(t, registry, "agent-1", Accept, "tx-consistent", DefaultPolicyVersion, "state-root-consistent"),
		signedTestDecision(t, registry, "agent-2", Accept, "tx-consistent", DefaultPolicyVersion, "state-root-consistent"),
		signedTestDecision(t, registry, "agent-3", Abstain, "tx-consistent", DefaultPolicyVersion, "state-root-consistent"),
	}

	// Independent replicas may receive the same authenticated decision set in
	// different message orders; canonical finalization must still be identical.
	replicaOne, err := PrepareBlockFinalization(
		registry, decisions, "tx-consistent", DefaultPolicyVersion, 2, "state-root-consistent",
	)
	if err != nil {
		t.Fatalf("replica one finalization failed: %v", err)
	}
	reordered := []SignedDecision{decisions[2], decisions[0], decisions[1]}
	replicaTwo, err := PrepareBlockFinalization(
		registry, reordered, "tx-consistent", DefaultPolicyVersion, 2, "state-root-consistent",
	)
	if err != nil {
		t.Fatalf("replica two finalization failed: %v", err)
	}
	if replicaOne != replicaTwo {
		t.Fatalf("replicas derived different finalization records:\nreplica one: %+v\nreplica two: %+v", replicaOne, replicaTwo)
	}
	if err := VerifyBlockFinalization(registry, reordered, replicaTwo, "state-root-consistent"); err != nil {
		t.Fatalf("replica two could not independently verify finalization: %v", err)
	}
}

func TestVerifyBlockFinalizationRejectsDecisionBoundToDifferentStateRoot(t *testing.T) {
	registry := NewIdentityRegistry()
	decisions := []SignedDecision{
		signedTestDecision(t, registry, "agent-1", Accept, "tx-state-mismatch", DefaultPolicyVersion, "state-root-expected"),
		signedTestDecision(t, registry, "agent-2", Accept, "tx-state-mismatch", DefaultPolicyVersion, "state-root-expected"),
	}
	record, err := PrepareBlockFinalization(
		registry, decisions, "tx-state-mismatch", DefaultPolicyVersion, 2, "state-root-expected",
	)
	if err != nil {
		t.Fatal(err)
	}

	// A replica that independently executes the block to a different state root
	// must not accept an otherwise valid Agent quorum.
	if err := VerifyBlockFinalization(registry, decisions, record, "state-root-divergent"); err == nil {
		t.Fatal("expected finalization to be rejected when replica state roots diverge")
	}
}

func TestVerifyBlockFinalizationRejectsTamperedQuorumCounters(t *testing.T) {
	registry := NewIdentityRegistry()
	decisions := []SignedDecision{
		signedTestDecision(t, registry, "agent-1", Accept, "tx-counter-tamper", DefaultPolicyVersion, "state-root"),
		signedTestDecision(t, registry, "agent-2", Accept, "tx-counter-tamper", DefaultPolicyVersion, "state-root"),
	}
	record, err := PrepareBlockFinalization(
		registry, decisions, "tx-counter-tamper", DefaultPolicyVersion, 2, "state-root",
	)
	if err != nil {
		t.Fatal(err)
	}
	record.AcceptVotes = 99
	if err := VerifyBlockFinalization(registry, decisions, record, "state-root"); err == nil {
		t.Fatal("expected tampered quorum counters to be rejected")
	}
}
