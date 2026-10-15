package poad

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func TestSignedDecisionRoundTrip(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	decision := Decision{
		AgentID:         "validator-1",
		TransactionHash: "0123456789abcdef",
		Result:          Accept,
		Reasons:         []string{"transaction_check_passed", "state_check_passed"},
		PolicyVersion:   DefaultPolicyVersion,
	}
	signed, err := SignDecision(decision, privateKey)
	if err != nil {
		t.Fatal(err)
	}

	if err := VerifyDecisionSignature(signed, publicKey, "validator-1"); err != nil {
		t.Fatalf("VerifyDecisionSignature() error = %v", err)
	}
}

func TestSignedDecisionRejectsTampering(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	signed, err := SignDecision(Decision{
		AgentID:         "validator-1",
		TransactionHash: "0123456789abcdef",
		Result:          Accept,
		PolicyVersion:   DefaultPolicyVersion,
	}, privateKey)
	if err != nil {
		t.Fatal(err)
	}

	signed.Decision.Result = Reject
	if err := VerifyDecisionSignature(signed, publicKey, "validator-1"); err == nil {
		t.Fatal("VerifyDecisionSignature() should reject a modified decision")
	}
}

func TestSignedDecisionRejectsWrongAgentID(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	signed, err := SignDecision(Decision{AgentID: "validator-1", Result: Accept}, privateKey)
	if err != nil {
		t.Fatal(err)
	}

	if err := VerifyDecisionSignature(signed, publicKey, "validator-2"); err == nil {
		t.Fatal("VerifyDecisionSignature() should reject an unexpected agent ID")
	}
}
