package poad

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/block"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/transaction"
)

func TestBuildAndAttachPaymentPoADMetadata(t *testing.T) {
	intent, tx := paymentFixture()
	hashBytes, err := tx.Hash()
	if err != nil {
		t.Fatal(err)
	}
	hash := hex.EncodeToString(hashBytes)

	registry := NewIdentityRegistry()
	validators := make([]PaymentValidatorInput, 0, 2)
	for _, agentID := range []string{"validator-1", "validator-2"} {
		publicKey, privateKey, err := GenerateAgentKeyPair()
		if err != nil {
			t.Fatal(err)
		}
		if err := registry.Register(agentID, publicKey); err != nil {
			t.Fatal(err)
		}
		validators = append(validators, PaymentValidatorInput{
			Input: VerificationInput{
				AgentID:         agentID,
				TransactionHash: hash,
				Transaction:     Pass,
				State:           Pass,
				Evidence:        NotRequired,
			},
			PrivateKey: privateKey,
		})
	}

	b := block.NewBlock(
		block.Header{StateRoot: []byte{0x01, 0x02, 0x03}},
		block.Body{TxList: []transaction.Transaction{tx}},
		block.MigrationOpt{},
	)
	if err := BuildAndAttachPaymentPoADMetadata(b, registry, tx, []TraceIntentLink{intent}, validators, 2); err != nil {
		t.Fatalf("BuildAndAttachPaymentPoADMetadata() error = %v", err)
	}

	validator, err := NewBlockMetadataValidatorWithIntentLinks(registry, []TraceIntentLink{intent})
	if err != nil {
		t.Fatal(err)
	}
	if err := validator.ValidatePoADBlock(b); err != nil {
		t.Fatalf("ValidatePoADBlock() error = %v", err)
	}
}

func TestBuildAndAttachPaymentPoADMetadataRejectsIntentMismatch(t *testing.T) {
	intent, tx := paymentFixture()
	intent.Amount++
	hashBytes, err := tx.Hash()
	if err != nil {
		t.Fatal(err)
	}
	hash := hex.EncodeToString(hashBytes)

	registry := NewIdentityRegistry()
	validators := make([]PaymentValidatorInput, 0, 2)
	for _, agentID := range []string{"validator-1", "validator-2"} {
		publicKey, privateKey, err := GenerateAgentKeyPair()
		if err != nil {
			t.Fatal(err)
		}
		if err := registry.Register(agentID, publicKey); err != nil {
			t.Fatal(err)
		}
		validators = append(validators, PaymentValidatorInput{
			Input: VerificationInput{
				AgentID:         agentID,
				TransactionHash: hash,
				Transaction:     Pass,
				State:           Pass,
				Evidence:        NotRequired,
			},
			PrivateKey: privateKey,
		})
	}

	b := block.NewBlock(
		block.Header{StateRoot: []byte{0x01, 0x02, 0x03}},
		block.Body{TxList: []transaction.Transaction{tx}},
		block.MigrationOpt{},
	)
	err = BuildAndAttachPaymentPoADMetadata(b, registry, tx, []TraceIntentLink{intent}, validators, 2)
	if err == nil {
		t.Fatal("expected intent mismatch to prevent block finalization")
	}
	if len(b.PoADRoot) != 0 || len(b.PoADData) != 0 {
		t.Fatal("failed decision quorum must not attach PoAD metadata")
	}
}

func TestBuildAndAttachPaymentPoADMetadataRejectsTransactionHashMismatch(t *testing.T) {
	intent, tx := paymentFixture()
	publicKey, privateKey, err := GenerateAgentKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	registry := NewIdentityRegistry()
	if err := registry.Register("validator-1", publicKey); err != nil {
		t.Fatal(err)
	}

	b := block.NewBlock(
		block.Header{StateRoot: []byte{1}},
		block.Body{TxList: []transaction.Transaction{tx}},
		block.MigrationOpt{},
	)
	err = BuildAndAttachPaymentPoADMetadata(b, registry, tx, []TraceIntentLink{intent}, []PaymentValidatorInput{{
		Input: VerificationInput{
			AgentID:         "validator-1",
			TransactionHash: fmt.Sprintf("%064x", 1),
			Transaction:     Pass,
			State:           Pass,
			Evidence:        NotRequired,
		},
		PrivateKey: privateKey,
	}}, 1)
	if err == nil {
		t.Fatal("expected mismatched validator transaction hash to be rejected")
	}
}
