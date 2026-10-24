package poad

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/block"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/transaction"
)

type testSignedDecisionProvider struct {
	decisions []SignedDecision
	err       error
	calls     int
}

func (p *testSignedDecisionProvider) CollectSignedDecisions(_ context.Context, _ transaction.Transaction, _ TraceIntentLink, _ string) ([]SignedDecision, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	return p.decisions, nil
}

func makeSignedPaymentDecisions(t *testing.T, tx transaction.Transaction, count int, stateRoot []byte) (*IdentityRegistry, []SignedDecision) {
	t.Helper()
	hashBytes, err := tx.Hash()
	if err != nil {
		t.Fatal(err)
	}
	registry := NewIdentityRegistry()
	decisions := make([]SignedDecision, 0, count)
	for i := 0; i < count; i++ {
		agentID := fmt.Sprintf("agent-%d", i+1)
		publicKey, privateKey, err := GenerateAgentKeyPair()
		if err != nil {
			t.Fatal(err)
		}
		if err := registry.Register(agentID, publicKey); err != nil {
			t.Fatal(err)
		}
		decision, err := Evaluate(VerificationInput{
			AgentID:         agentID,
			TransactionHash: hex.EncodeToString(hashBytes),
			StateRoot:       hex.EncodeToString(stateRoot),
			Transaction:     Pass,
			State:           Pass,
			Intent:          Pass,
			Evidence:        NotRequired,
		})
		if err != nil {
			t.Fatal(err)
		}
		signed, err := SignDecision(decision, privateKey)
		if err != nil {
			t.Fatal(err)
		}
		decisions = append(decisions, signed)
	}
	return registry, decisions
}

func TestPaymentProposalBuilderAttachesIndependentlySignedDecisions(t *testing.T) {
	intent, tx := paymentFixture()
	registry, decisions := makeSignedPaymentDecisions(t, tx, 2, []byte{1, 2, 3})
	provider := &testSignedDecisionProvider{decisions: decisions}
	builder, err := NewPaymentProposalBuilder(registry, []TraceIntentLink{intent}, provider, 2)
	if err != nil {
		t.Fatal(err)
	}
	b := block.NewBlock(
		block.Header{Type: block.TxBlockType, StateRoot: []byte{1, 2, 3}},
		block.Body{TxList: []transaction.Transaction{tx}},
		block.MigrationOpt{},
	)
	if err := builder.BuildPoADMetadata(context.Background(), b); err != nil {
		t.Fatalf("BuildPoADMetadata() error = %v", err)
	}
	if len(b.PoADRoot) == 0 || len(b.PoADData) == 0 {
		t.Fatal("expected PoAD envelope to be attached")
	}
	validator, err := NewBlockMetadataValidatorWithIntentLinks(registry, []TraceIntentLink{intent})
	if err != nil {
		t.Fatal(err)
	}
	if err := validator.ValidatePoADBlock(b); err != nil {
		t.Fatalf("ValidatePoADBlock() error = %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
}

func TestPaymentProposalBuilderFailsClosedWithoutProvider(t *testing.T) {
	registry := NewIdentityRegistry()
	if _, err := NewPaymentProposalBuilder(registry, []TraceIntentLink{{RequestID: "r"}}, nil, 1); err == nil {
		t.Fatal("expected missing provider to be rejected")
	}
}

func TestPaymentProposalBuilderPropagatesProviderFailure(t *testing.T) {
	intent, tx := paymentFixture()
	registry, _ := makeSignedPaymentDecisions(t, tx, 1, []byte{1})
	provider := &testSignedDecisionProvider{err: errors.New("agent supervisor unavailable")}
	builder, err := NewPaymentProposalBuilder(registry, []TraceIntentLink{intent}, provider, 1)
	if err != nil {
		t.Fatal(err)
	}
	b := block.NewBlock(
		block.Header{Type: block.TxBlockType, StateRoot: []byte{1}},
		block.Body{TxList: []transaction.Transaction{tx}},
		block.MigrationOpt{},
	)
	if err := builder.BuildPoADMetadata(context.Background(), b); err == nil {
		t.Fatal("expected provider failure to abort proposal creation")
	}
	if len(b.PoADRoot) != 0 || len(b.PoADData) != 0 {
		t.Fatal("failed proposal builder must not attach PoAD metadata")
	}
}

func TestPaymentProposalBuilderRejectsTransactionIntentMismatch(t *testing.T) {
	intent, tx := paymentFixture()
	intent.Amount++
	registry, decisions := makeSignedPaymentDecisions(t, tx, 1, []byte{1})
	provider := &testSignedDecisionProvider{decisions: decisions}
	builder, err := NewPaymentProposalBuilder(registry, []TraceIntentLink{intent}, provider, 1)
	if err != nil {
		t.Fatal(err)
	}
	b := block.NewBlock(
		block.Header{Type: block.TxBlockType, StateRoot: []byte{1}},
		block.Body{TxList: []transaction.Transaction{tx}},
		block.MigrationOpt{},
	)
	if err := builder.BuildPoADMetadata(context.Background(), b); err == nil {
		t.Fatal("expected intent mismatch to abort proposal creation")
	}
	if provider.calls != 0 {
		t.Fatal("provider must not be called after intent mismatch")
	}
}

func TestPaymentProposalBuilderRejectsDecisionForDifferentStateRoot(t *testing.T) {
	intent, tx := paymentFixture()
	registry, decisions := makeSignedPaymentDecisions(t, tx, 1, []byte{9})
	provider := &testSignedDecisionProvider{decisions: decisions}
	builder, err := NewPaymentProposalBuilder(registry, []TraceIntentLink{intent}, provider, 1)
	if err != nil {
		t.Fatal(err)
	}

	b := block.NewBlock(
		block.Header{Type: block.TxBlockType, StateRoot: []byte{1}},
		block.Body{TxList: []transaction.Transaction{tx}},
		block.MigrationOpt{},
	)
	if err := builder.BuildPoADMetadata(context.Background(), b); err == nil {
		t.Fatal("expected a decision bound to a different state root to be rejected")
	}
	if len(b.PoADRoot) != 0 || len(b.PoADData) != 0 {
		t.Fatal("state-root mismatch must not attach PoAD metadata")
	}
}
