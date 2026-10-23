package poad

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/account"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/block"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/transaction"
)

func TestLoadBlockEvidenceBundleVerified(t *testing.T) {
	bundle := BlockEvidenceBundle{
		TransactionHash: "0xabc123",
		Decisions: []SignedDecision{{
			Decision: Decision{
				AgentID: "agent-1", TransactionHash: "abc123",
				Result: Accept, PolicyVersion: DefaultPolicyVersion,
			},
		}},
		Finalization: BlockFinalizationRecord{
			TransactionHash: "abc123",
			PolicyVersion: DefaultPolicyVersion,
		},
	}
	data, err := MarshalBlockEvidenceBundle(bundle)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadBlockEvidenceBundleVerified(bytes.NewReader(data), EvidenceBundleDigest(data))
	if err != nil {
		t.Fatalf("load verified evidence bundle: %v", err)
	}
	if loaded.TransactionHash != bundle.TransactionHash {
		t.Fatalf("unexpected transaction hash: got %q", loaded.TransactionHash)
	}

	tampered := append([]byte(nil), data...)
	tampered[len(tampered)-2] ^= 1
	if _, err := LoadBlockEvidenceBundleVerified(bytes.NewReader(tampered), EvidenceBundleDigest(data)); err == nil {
		t.Fatal("expected digest mismatch for tampered bundle")
	}
}

func TestLoadBlockEvidenceBundleRejectsTransactionMismatch(t *testing.T) {
	bundle := BlockEvidenceBundle{
		TransactionHash: "abc123",
		Decisions: []SignedDecision{{
			Decision: Decision{AgentID: "agent-1", TransactionHash: "different", Result: Accept},
		}},
		Finalization: BlockFinalizationRecord{TransactionHash: "abc123"},
	}
	data, err := MarshalBlockEvidenceBundle(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBlockEvidenceBundleVerified(bytes.NewReader(data), EvidenceBundleDigest(data)); err == nil {
		t.Fatal("expected mismatched decision transaction to be rejected")
	}
}

func TestAttachVerifiedBlockEvidenceBindsBlockAndStateRoot(t *testing.T) {
	registry := NewIdentityRegistry()
	publicKey, privateKey, err := GenerateAgentKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("agent-1", publicKey); err != nil {
		t.Fatal(err)
	}

	tx := transaction.Transaction{
		Sender: account.Address{1}, Recipient: account.Address{2},
		Value: big.NewInt(3), PriorityFee: big.NewInt(0), GasLimit: 21000,
	}
	txHashBytes, err := tx.Hash()
	if err != nil {
		t.Fatal(err)
	}
	txHash := hex.EncodeToString(txHashBytes)
	b := block.NewBlock(
		block.Header{StateRoot: []byte{0x01, 0x02}},
		block.Body{TxList: []transaction.Transaction{tx}},
		block.MigrationOpt{},
	)
	decision := Decision{
		AgentID: "agent-1", TransactionHash: txHash, Result: Accept,
		PolicyVersion: DefaultPolicyVersion,
	}
	signed, err := SignDecision(decision, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	finalization, err := PrepareBlockFinalization(
		registry, []SignedDecision{signed}, txHash, DefaultPolicyVersion, 1, hex.EncodeToString(b.StateRoot),
	)
	if err != nil {
		t.Fatal(err)
	}
	bundle := BlockEvidenceBundle{
		TransactionHash: txHash, Decisions: []SignedDecision{signed}, Finalization: finalization,
	}
	if err := AttachVerifiedBlockEvidence(b, registry, bundle); err != nil {
		t.Fatalf("attach evidence to matching block: %v", err)
	}
	if err := VerifyBlockPoADMetadata(b, registry); err != nil {
		t.Fatalf("verify attached evidence: %v", err)
	}

	other := block.NewBlock(
		block.Header{StateRoot: append([]byte(nil), b.StateRoot...)},
		block.Body{TxList: []transaction.Transaction{{
			Sender: account.Address{1}, Recipient: account.Address{3},
			Value: big.NewInt(3), PriorityFee: big.NewInt(0), GasLimit: 21000,
		}}},
		block.MigrationOpt{},
	)
	if err := AttachVerifiedBlockEvidence(other, registry, bundle); err == nil {
		t.Fatal("expected evidence for another transaction to be rejected")
	}

}
