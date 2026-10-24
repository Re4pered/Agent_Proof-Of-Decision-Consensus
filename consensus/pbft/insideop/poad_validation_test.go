package insideop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/HuangLab-SYSU/block-emulator-x/consensus/poad"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/account"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/block"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/transaction"
)

type fakePoADProposalBuilder struct {
	calls int
	err   error
}

func (b *fakePoADProposalBuilder) BuildPoADMetadata(_ context.Context, block *block.Block) error {
	b.calls++
	if b.err != nil {
		return b.err
	}
	block.PoADRoot = []byte{1}
	block.PoADData = []byte{1}
	return nil
}

type fakePoADBlockValidator struct {
	calls int
	err   error
}

func (v *fakePoADBlockValidator) ValidatePoADBlock(_ *block.Block) error {
	v.calls++

	return v.err
}

func TestBuildPoADProposalMetadataRequiresBuilderWhenPoADRequired(t *testing.T) {
	b := block.NewBlock(block.Header{Type: block.TxBlockType}, block.Body{}, block.MigrationOpt{})
	if err := buildPoADProposalMetadata(context.Background(), b, nil, true); err == nil {
		t.Fatal("expected required PoAD mode to reject missing proposal builder")
	}
}

func TestBuildPoADProposalMetadataInvokesBuilder(t *testing.T) {
	b := block.NewBlock(block.Header{Type: block.TxBlockType}, block.Body{}, block.MigrationOpt{})
	builder := &fakePoADProposalBuilder{}
	if err := buildPoADProposalMetadata(context.Background(), b, builder, true); err != nil {
		t.Fatalf("expected proposal builder to succeed: %v", err)
	}
	if builder.calls != 1 || len(b.PoADRoot) == 0 || len(b.PoADData) == 0 {
		t.Fatal("expected builder to attach PoAD metadata exactly once")
	}
}

func TestBuildPoADProposalMetadataPropagatesBuilderFailure(t *testing.T) {
	b := block.NewBlock(block.Header{Type: block.TxBlockType}, block.Body{}, block.MigrationOpt{})
	builder := &fakePoADProposalBuilder{err: errors.New("agent quorum unavailable")}
	if err := buildPoADProposalMetadata(context.Background(), b, builder, true); err == nil {
		t.Fatal("expected builder failure to abort proposal creation")
	}
}

func TestBuildPoADProposalMetadataExemptsMigrationBlock(t *testing.T) {
	b := block.NewBlock(block.Header{Type: block.MigrationBlockType}, block.Body{}, block.MigrationOpt{})
	if err := buildPoADProposalMetadata(context.Background(), b, nil, true); err != nil {
		t.Fatalf("expected migration block to bypass transaction PoAD builder: %v", err)
	}
}

func TestValidatePoADProposalAllowsLegacyBlockWhenOptional(t *testing.T) {
	b := block.NewBlock(block.Header{Type: block.TxBlockType}, block.Body{}, block.MigrationOpt{})
	if err := validatePoADProposal(b, nil, false); err != nil {
		t.Fatalf("expected legacy block to be allowed when PoAD is optional: %v", err)
	}
}

func TestValidatePoADProposalRequiresMetadataWhenConfigured(t *testing.T) {
	b := block.NewBlock(block.Header{Type: block.TxBlockType}, block.Body{}, block.MigrationOpt{})
	if err := validatePoADProposal(b, &fakePoADBlockValidator{}, true); err == nil {
		t.Fatal("expected transaction proposal without PoAD metadata to be rejected")
	}
}


func TestValidatePoADProposalRejectsMultiTransactionBlockWhenRequired(t *testing.T) {
	b := block.NewBlock(
		block.Header{Type: block.TxBlockType},
		block.Body{TxList: []transaction.Transaction{{}, {}}},
		block.MigrationOpt{},
	)

	if err := validatePoADProposal(b, &fakePoADBlockValidator{}, true); err == nil {
		t.Fatal("expected required PoAD mode to reject a multi-transaction block")
	}
}

func TestValidatePoADProposalRequiresValidatorForPresentMetadata(t *testing.T) {
	b := block.NewBlock(
		block.Header{Type: block.TxBlockType, PoADRoot: []byte{1}},
		block.Body{},
		block.MigrationOpt{},
	)
	b.PoADData = []byte{2}

	if err := validatePoADProposal(b, nil, false); err == nil {
		t.Fatal("expected PoAD metadata without a verifier to be rejected")
	}
}

func TestValidatePoADProposalInvokesConfiguredValidator(t *testing.T) {
	b := block.NewBlock(
		block.Header{Type: block.TxBlockType, PoADRoot: []byte{1}},
		block.Body{},
		block.MigrationOpt{},
	)
	b.PoADData = []byte{2}
	validator := &fakePoADBlockValidator{}

	if err := validatePoADProposal(b, validator, false); err != nil {
		t.Fatalf("expected configured validator to accept test block: %v", err)
	}

	if validator.calls != 1 {
		t.Fatalf("expected validator to be called once, got %d", validator.calls)
	}
}

func TestValidatePoADProposalPropagatesVerifierFailure(t *testing.T) {
	b := block.NewBlock(
		block.Header{Type: block.TxBlockType, PoADRoot: []byte{1}},
		block.Body{},
		block.MigrationOpt{},
	)
	b.PoADData = []byte{2}
	validator := &fakePoADBlockValidator{err: errors.New("bad signature")}

	if err := validatePoADProposal(b, validator, false); err == nil {
		t.Fatal("expected verifier error to reject proposal")
	}
}

func TestValidatePoADProposalExemptsMigrationBlockFromRequiredMetadata(t *testing.T) {
	b := block.NewBlock(block.Header{Type: block.MigrationBlockType}, block.Body{}, block.MigrationOpt{})
	if err := validatePoADProposal(b, &fakePoADBlockValidator{}, true); err != nil {
		t.Fatalf("expected migration block to be exempt from transaction PoAD policy: %v", err)
	}
}


func buildRealPoADValidationFixture(t *testing.T) (*block.Block, *poad.BlockMetadataValidator) {
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
	stateRoot := []byte{1, 2, 3}
	txHash := hex.EncodeToString(txHashBytes)
	stateRootHex := hex.EncodeToString(stateRoot)
	registry := poad.NewIdentityRegistry()
	decisions := make([]poad.SignedDecision, 0, 2)

	for i := 1; i <= 2; i++ {
		agentID := fmt.Sprintf("agent-%d", i)
		publicKey, privateKey, err := poad.GenerateAgentKeyPair()
		if err != nil {
			t.Fatal(err)
		}
		if err := registry.Register(agentID, publicKey); err != nil {
			t.Fatal(err)
		}
		decision, err := poad.Evaluate(poad.VerificationInput{
			AgentID:         agentID,
			TransactionHash: txHash,
			StateRoot:       stateRootHex,
			Transaction:     poad.Pass,
			State:           poad.Pass,
			Intent:          poad.Pass,
			Evidence:        poad.NotRequired,
		})
		if err != nil {
			t.Fatal(err)
		}
		signed, err := poad.SignDecision(decision, privateKey)
		if err != nil {
			t.Fatal(err)
		}
		decisions = append(decisions, signed)
	}

	finalization, err := poad.PrepareBlockFinalization(
		registry, decisions, txHash, poad.DefaultPolicyVersion, 2, stateRootHex,
	)
	if err != nil {
		t.Fatal(err)
	}
	proposalBlock := block.NewBlock(
		block.Header{Type: block.TxBlockType, StateRoot: stateRoot},
		block.Body{TxList: []transaction.Transaction{tx}},
		block.MigrationOpt{},
	)
	if err := poad.AttachBlockPoADMetadata(proposalBlock, registry, decisions, finalization); err != nil {
		t.Fatal(err)
	}
	validator, err := poad.NewBlockMetadataValidator(registry)
	if err != nil {
		t.Fatal(err)
	}
	return proposalBlock, validator
}

func TestValidatePoADProposalAcceptsRealSignedDecisionEnvelope(t *testing.T) {
	proposalBlock, validator := buildRealPoADValidationFixture(t)
	if err := validatePoADProposal(proposalBlock, validator, true); err != nil {
		t.Fatalf("expected real signed PoAD envelope to pass: %v", err)
	}
}

func TestValidatePoADProposalRejectsTamperedAgentSignature(t *testing.T) {
	proposalBlock, validator := buildRealPoADValidationFixture(t)

	var envelope poad.BlockPoADEnvelope
	if err := json.Unmarshal(proposalBlock.PoADData, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Decisions[0].Signature[0] ^= 0xff
	tampered, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	root := sha256.Sum256(tampered)
	proposalBlock.PoADData = tampered
	proposalBlock.PoADRoot = root[:]

	if err := validatePoADProposal(proposalBlock, validator, true); err == nil {
		t.Fatal("expected PBFT PoAD validation to reject a tampered Agent signature")
	}
}
