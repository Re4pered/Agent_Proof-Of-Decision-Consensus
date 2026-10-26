package poad

import (
	"fmt"
	"testing"
)

func TestEvaluateAndSignPaymentDecision(t *testing.T) {
	intent, tx := paymentFixture()
	hash, err := tx.Hash()
	if err != nil {
		t.Fatal(err)
	}

	publicKey, privateKey, err := GenerateAgentKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	registry := NewIdentityRegistry()
	if err := registry.Register("validator-1", publicKey); err != nil {
		t.Fatal(err)
	}

	input := VerificationInput{
		AgentID:         "validator-1",
		TransactionHash: fmt.Sprintf("%x", hash),
		Transaction:     Pass,
		State:           Pass,
		Evidence:        NotRequired,
	}

	signed, err := EvaluateAndSignPaymentDecision(input, tx, []TraceIntentLink{intent}, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if signed.Decision.Result != Accept {
		t.Fatalf("decision = %s, reasons = %v; want ACCEPT", signed.Decision.Result, signed.Decision.Reasons)
	}
	if err := registry.VerifyDecision(signed); err != nil {
		t.Fatalf("VerifyDecision() error = %v", err)
	}
}

func TestEvaluateAndSignPaymentDecisionSignsIntentRejection(t *testing.T) {
	intent, tx := paymentFixture()
	intent.Amount++

	hash, err := tx.Hash()
	if err != nil {
		t.Fatal(err)
	}

	_, privateKey, err := GenerateAgentKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	signed, err := EvaluateAndSignPaymentDecision(VerificationInput{
		AgentID:         "validator-1",
		TransactionHash: fmt.Sprintf("%x", hash),
		Transaction:     Pass,
		State:           Pass,
		Evidence:        NotRequired,
	}, tx, []TraceIntentLink{intent}, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if signed.Decision.Result != Reject {
		t.Fatalf("decision = %s, reasons = %v; want REJECT", signed.Decision.Result, signed.Decision.Reasons)
	}
}

func TestEvaluateAndSignPaymentDecisionAbstainsWithoutIntentLink(t *testing.T) {
	_, tx := paymentFixture()
	hash, err := tx.Hash()
	if err != nil {
		t.Fatal(err)
	}

	_, privateKey, err := GenerateAgentKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	signed, err := EvaluateAndSignPaymentDecision(VerificationInput{
		AgentID:         "validator-1",
		TransactionHash: fmt.Sprintf("%x", hash),
		Transaction:     Pass,
		State:           Pass,
		Evidence:        NotRequired,
	}, tx, nil, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if signed.Decision.Result != Abstain {
		t.Fatalf("decision = %s, reasons = %v; want ABSTAIN", signed.Decision.Result, signed.Decision.Reasons)
	}
}
