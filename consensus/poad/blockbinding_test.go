package poad

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/account"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/block"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/transaction"
)

func buildPoADTestBlock(t *testing.T, registry *IdentityRegistry) (*block.Block, []SignedDecision, BlockFinalizationRecord) {
	t.Helper()

	tx := transaction.Transaction{
		Sender:      account.Address{1},
		Recipient:   account.Address{2},
		Value:       big.NewInt(1),
		PriorityFee: big.NewInt(0),
		GasLimit:    21000,
	}
	txHashBytes, err := tx.Hash()
	if err != nil {
		t.Fatal(err)
	}

	txHash := hex.EncodeToString(txHashBytes)
	decisions := []SignedDecision{
		signedTestDecision(t, registry, "agent-1", Accept, txHash, DefaultPolicyVersion),
		signedTestDecision(t, registry, "agent-2", Accept, txHash, DefaultPolicyVersion),
	}
	b := block.NewBlock(
		block.Header{StateRoot: []byte{0x01, 0x02, 0x03}},
		block.Body{TxList: []transaction.Transaction{tx}},
		block.MigrationOpt{},
	)

	record, err := PrepareBlockFinalization(
		registry,
		decisions,
		txHash,
		DefaultPolicyVersion,
		2,
		hex.EncodeToString(b.StateRoot),
	)
	if err != nil {
		t.Fatal(err)
	}

	return b, decisions, record
}

func TestAttachAndVerifyBlockPoADMetadata(t *testing.T) {
	registry := NewIdentityRegistry()
	b, decisions, record := buildPoADTestBlock(t, registry)

	if err := AttachBlockPoADMetadata(b, registry, decisions, record); err != nil {
		t.Fatal(err)
	}

	if len(b.PoADRoot) == 0 || len(b.PoADData) == 0 {
		t.Fatal("expected PoAD metadata and header commitment to be attached")
	}

	if err := VerifyBlockPoADMetadata(b, registry); err != nil {
		t.Fatalf("expected PoAD metadata to verify: %v", err)
	}

	b.PoADData[0] ^= 0xff
	if err := VerifyBlockPoADMetadata(b, registry); err == nil {
		t.Fatal("expected tampered PoAD envelope to be rejected")
	}
}

func TestVerifyBlockPoADMetadataRejectsTamperedHeaderRoot(t *testing.T) {
	registry := NewIdentityRegistry()
	b, decisions, record := buildPoADTestBlock(t, registry)
	if err := AttachBlockPoADMetadata(b, registry, decisions, record); err != nil {
		t.Fatal(err)
	}

	b.PoADRoot[0] ^= 0xff
	if err := VerifyBlockPoADMetadata(b, registry); err == nil {
		t.Fatal("expected tampered PoAD root to be rejected")
	}
}

func TestVerifyBlockPoADMetadataRejectsIncompleteCommitment(t *testing.T) {
	b := block.NewBlock(block.Header{PoADRoot: []byte{1}}, block.Body{}, block.MigrationOpt{})
	if err := VerifyBlockPoADMetadata(b, NewIdentityRegistry()); err == nil {
		t.Fatal("expected incomplete PoAD commitment to be rejected")
	}
}

func TestAttachBlockPoADMetadataCanonicalizesDecisionOrder(t *testing.T) {
	registry := NewIdentityRegistry()
	b1, decisions, record := buildPoADTestBlock(t, registry)
	b2 := block.NewBlock(
		block.Header{StateRoot: append([]byte(nil), b1.StateRoot...)},
		block.Body{TxList: append([]transaction.Transaction(nil), b1.TxList...)},
		block.MigrationOpt{},
	)

	if err := AttachBlockPoADMetadata(b1, registry, decisions, record); err != nil {
		t.Fatal(err)
	}

	reversed := []SignedDecision{decisions[1], decisions[0]}
	if err := AttachBlockPoADMetadata(b2, registry, reversed, record); err != nil {
		t.Fatal(err)
	}

	if !equalBytes(b1.PoADRoot, b2.PoADRoot) {
		t.Fatal("canonical PoAD commitment should not depend on decision input order")
	}
}

func TestAttachBlockPoADMetadataRejectsTransactionNotInBlock(t *testing.T) {
	registry := NewIdentityRegistry()
	b, decisions, record := buildPoADTestBlock(t, registry)
	record.TransactionHash = "deadbeef"

	if err := AttachBlockPoADMetadata(b, registry, decisions, record); err == nil {
		t.Fatal("expected finalization for a transaction outside the block to be rejected")
	}
}
