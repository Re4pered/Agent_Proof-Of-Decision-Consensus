package poad

import (
	"testing"
)

func TestPrepareAndVerifyBlockFinalization(t *testing.T) {
	registry := NewIdentityRegistry()
	decisions := []SignedDecision{
		signedTestDecision(t, registry, "agent-1", Accept, "tx-1", DefaultPolicyVersion),
		signedTestDecision(t, registry, "agent-2", Accept, "tx-1", DefaultPolicyVersion),
		signedTestDecision(t, registry, "agent-3", Abstain, "tx-1", DefaultPolicyVersion),
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
		signedTestDecision(t, registry, "agent-1", Accept, "tx-1", DefaultPolicyVersion),
		signedTestDecision(t, registry, "agent-2", Reject, "tx-1", DefaultPolicyVersion),
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
		signedTestDecision(t, registry, "agent-1", Accept, "tx-1", DefaultPolicyVersion),
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
		signedTestDecision(t, registry, "agent-1", Accept, "tx-1", DefaultPolicyVersion),
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
