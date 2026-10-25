package pbft

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/HuangLab-SYSU/block-emulator-x/config"
	"github.com/HuangLab-SYSU/block-emulator-x/consensus/poad"
	"github.com/HuangLab-SYSU/block-emulator-x/consensus/pbft/basicstructs"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/account"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/block"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/transaction"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/message"
)

type rejectingProposalOp struct {
	validateCalls int
	validationErr error
}

func (op *rejectingProposalOp) BuildProposal(context.Context) (*message.Proposal, error) {
	return &message.Proposal{
		Block: block.NewBlock(block.Header{Number: 1}, block.Body{}, block.MigrationOpt{}),
	}, nil
}

func (op *rejectingProposalOp) ValidateProposal(context.Context, *message.Proposal) error {
	op.validateCalls++
	return op.validationErr
}

func (*rejectingProposalOp) ProposalCommitAndDeliver(context.Context, bool, *message.Proposal) error {
	return nil
}

func TestProposeRejectsInvalidLocalProposalBeforeBroadcast(t *testing.T) {
	wantErr := errors.New("invalid PoAD evidence")
	op := &rejectingProposalOp{validationErr: wantErr}
	meta := newConsensusMeta(config.ConsensusNodeCfg{}, config.LocalParams{})
	meta.lastProposeTime = time.Time{}

	node := &Node{
		iop:      op,
		pbftMeta: meta,
		// conn and resolver are intentionally nil: a rejected proposal must fail
		// before any network broadcast is attempted.
	}

	err := node.propose(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("propose() error = %v, want %v", err, wantErr)
	}
	if op.validateCalls != 1 {
		t.Fatalf("ValidateProposal calls = %d, want 1", op.validateCalls)
	}
}


func TestValidatePreprepareDigestAcceptsMatchingProposalHash(t *testing.T) {
	proposal := &message.Proposal{
		Block: block.NewBlock(block.Header{Number: 1}, block.Body{}, block.MigrationOpt{}),
	}
	digest, err := proposal.Hash()
	if err != nil {
		t.Fatal(err)
	}

	ppMsg := &message.PreprepareMsg{P: *proposal, Digest: digest}
	if err := validatePreprepareDigest(ppMsg); err != nil {
		t.Fatalf("expected matching proposal digest to pass: %v", err)
	}
}

func TestValidatePreprepareDigestRejectsMismatch(t *testing.T) {
	proposal := &message.Proposal{
		Block: block.NewBlock(block.Header{Number: 1}, block.Body{}, block.MigrationOpt{}),
	}
	ppMsg := &message.PreprepareMsg{
		P:      *proposal,
		Digest: []byte("digest-for-another-proposal"),
	}

	if err := validatePreprepareDigest(ppMsg); err == nil {
		t.Fatal("expected preprepare with a mismatched proposal digest to be rejected")
	}
}

func TestValidatePreprepareDigestRejectsNilMessage(t *testing.T) {
	if err := validatePreprepareDigest(nil); err == nil {
		t.Fatal("expected nil preprepare message to be rejected")
	}
}


func TestHandlePreprepareRejectsInvalidPoADBeforeMessagePool(t *testing.T) {
	wantErr := errors.New("invalid PoAD evidence")
	op := &rejectingProposalOp{validationErr: wantErr}
	node := senderValidationNode()
	node.iop = op

	proposal := &message.Proposal{
		Block: block.NewBlock(block.Header{Number: 1}, block.Body{}, block.MigrationOpt{}),
	}
	digest, err := proposal.Hash()
	if err != nil {
		t.Fatal(err)
	}
	ppMsg := message.PreprepareMsg{
		P:       *proposal,
		Digest:  digest,
		ShardID: 1,
		NodeID:  2,
		View:    0,
		Seq:     0,
	}
	var payload bytes.Buffer
	if err := gob.NewEncoder(&payload).Encode(ppMsg); err != nil {
		t.Fatal(err)
	}

	err = node.handlePreprepare(context.Background(), payload.Bytes())
	if !errors.Is(err, wantErr) {
		t.Fatalf("handlePreprepare() error = %v, want %v", err, wantErr)
	}
	if op.validateCalls != 1 {
		t.Fatalf("ValidateProposal calls = %d, want 1", op.validateCalls)
	}
	if got := node.pbftMeta.msgPool.ReadPreprepareMsg(basicstructs.ViewSeq{}); len(got) != 0 {
		t.Fatalf("invalid PoAD preprepare entered message pool: got %d messages", len(got))
	}
	if got := node.pbftMeta.latestViewSeq; got != (basicstructs.ViewSeq{}) {
		t.Fatalf("latest view/sequence advanced after rejected proposal: %+v", got)
	}
}


type acceptingProposalOp struct {
	validateCalls int
}

func (op *acceptingProposalOp) BuildProposal(context.Context) (*message.Proposal, error) {
	return nil, nil
}

func (op *acceptingProposalOp) ValidateProposal(context.Context, *message.Proposal) error {
	op.validateCalls++
	return nil
}

func (*acceptingProposalOp) ProposalCommitAndDeliver(context.Context, bool, *message.Proposal) error {
	return nil
}

func TestThreeReplicasApplyConsistentPreprepareValidationGate(t *testing.T) {
	proposal := &message.Proposal{
		Block: block.NewBlock(block.Header{Number: 1}, block.Body{}, block.MigrationOpt{}),
	}
	digest, err := proposal.Hash()
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name       string
		validation error
		wantAccept bool
	}{
		{name: "valid proposal accepted by every replica", wantAccept: true},
		{name: "invalid PoAD rejected by every replica", validation: errors.New("invalid PoAD evidence")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for nodeID := int64(0); nodeID < 3; nodeID++ {
				node := senderValidationNode()
				node.pbftMeta.lp.NodeID = nodeID
				op := &replicaProposalOp{validationErr: tc.validation}
				node.iop = op

				ppMsg := message.PreprepareMsg{
					P:       *proposal,
					Digest:  digest,
					ShardID: 1,
					NodeID:  0,
					View:    0,
					Seq:     0,
				}
				var payload bytes.Buffer
				if err := gob.NewEncoder(&payload).Encode(ppMsg); err != nil {
					t.Fatal(err)
				}

				err := node.handlePreprepare(context.Background(), payload.Bytes())
				if tc.wantAccept && err != nil {
					t.Errorf("replica %d rejected valid proposal: %v", nodeID, err)
				}
				if !tc.wantAccept && err == nil {
					t.Errorf("replica %d accepted invalid PoAD proposal", nodeID)
				}
				pooled := node.pbftMeta.msgPool.ReadPreprepareMsg(basicstructs.ViewSeq{})
				if tc.wantAccept && len(pooled) != 1 {
					t.Errorf("replica %d pooled %d proposals, want 1", nodeID, len(pooled))
				}
				if !tc.wantAccept && len(pooled) != 0 {
					t.Errorf("replica %d pooled invalid proposal", nodeID)
				}
				if op.validateCalls != 1 {
					t.Errorf("replica %d validation calls = %d, want 1", nodeID, op.validateCalls)
				}
			}
		})
	}
}

type replicaProposalOp struct {
	validationErr error
	validator     func(*block.Block) error
	validateCalls int
}

func (*replicaProposalOp) BuildProposal(context.Context) (*message.Proposal, error) {
	return nil, fmt.Errorf("not used in replica ingress test")
}

func (op *replicaProposalOp) ValidateProposal(_ context.Context, proposal *message.Proposal) error {
	op.validateCalls++
	if op.validationErr != nil {
		return op.validationErr
	}
	if op.validator != nil {
		return op.validator(proposal.Block)
	}
	return nil
}

func buildSignedPoADProposalForPBFTTest(
	t *testing.T,
) (*block.Block, *poad.IdentityRegistry, poad.TraceIntentLink) {
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

	intent := poad.TraceIntentLink{
		Action:           "pay",
		AgentID:          "agent-1",
		Target:           "agent-2",
		Amount:           1,
		RequestID:        "pbft-intent-test",
		ParamsHash:       "v1",
		TxHashes:         []string{txHash},
		ExpectedSender:   "0x" + hex.EncodeToString(tx.Sender[:]),
		ExpectedRecipient: "0x" + hex.EncodeToString(tx.Recipient[:]),
	}

	return proposalBlock, registry, intent
}

func TestThreeReplicasUseRealPoADSignatureValidation(t *testing.T) {
	for _, tc := range []struct {
		name          string
		tamper        bool
		mismatchIntent bool
		wantAccept    bool
	}{
		{name: "valid signed decisions and intent accepted", wantAccept: true},
		{name: "tampered agent signature rejected", tamper: true, wantAccept: false},
		{name: "intent amount mismatch rejected", mismatchIntent: true, wantAccept: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proposalBlock, registry, intent := buildSignedPoADProposalForPBFTTest(t)
			if tc.tamper {
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
			}
			if tc.mismatchIntent {
				intent.Amount = 2
			}

			proposal := &message.Proposal{Block: proposalBlock}
			digest, err := proposal.Hash()
			if err != nil {
				t.Fatal(err)
			}

			for nodeID := int64(0); nodeID < 3; nodeID++ {
				node := senderValidationNode()
				node.pbftMeta.lp.NodeID = nodeID

				validator, err := poad.NewBlockMetadataValidatorWithIntentLinks(
					registry, []poad.TraceIntentLink{intent},
				)
				if err != nil {
					t.Fatal(err)
				}
				op := &replicaProposalOp{validator: validator.ValidatePoADBlock}
				node.iop = op

				ppMsg := message.PreprepareMsg{
					P:       *proposal,
					Digest:  digest,
					ShardID: 1,
					NodeID:  0,
					View:    0,
					Seq:     0,
				}
				var payload bytes.Buffer
				if err := gob.NewEncoder(&payload).Encode(ppMsg); err != nil {
					t.Fatal(err)
				}

				err = node.handlePreprepare(context.Background(), payload.Bytes())
				if tc.wantAccept && err != nil {
					t.Errorf("replica %d rejected valid signed proposal: %v", nodeID, err)
				}
				if !tc.wantAccept && err == nil {
					t.Errorf("replica %d accepted invalid PoAD proposal", nodeID)
				}

				pooled := node.pbftMeta.msgPool.ReadPreprepareMsg(basicstructs.ViewSeq{})
				if tc.wantAccept && len(pooled) != 1 {
					t.Errorf("replica %d pooled %d proposals, want 1", nodeID, len(pooled))
				}
				if !tc.wantAccept && len(pooled) != 0 {
					t.Errorf("replica %d pooled invalid proposal", nodeID)
				}
				if op.validateCalls != 1 {
					t.Errorf("replica %d validation calls = %d, want 1", nodeID, op.validateCalls)
				}
			}
		})
	}
}

func (*replicaProposalOp) ProposalCommitAndDeliver(context.Context, bool, *message.Proposal) error {
	return nil
}


type commitGateProposalOp struct {
	validationErr error
	validateCalls int
	commitCalls   int
}

func (*commitGateProposalOp) BuildProposal(context.Context) (*message.Proposal, error) {
	return nil, errors.New("not used in commit gate test")
}

func (op *commitGateProposalOp) ValidateProposal(context.Context, *message.Proposal) error {
	op.validateCalls++
	return op.validationErr
}

func (op *commitGateProposalOp) ProposalCommitAndDeliver(context.Context, bool, *message.Proposal) error {
	op.commitCalls++
	return nil
}

func TestCommitQuorumRevalidatesProposalBeforeApplyingBlock(t *testing.T) {
	wantErr := errors.New("PoAD state root no longer matches local state")
	op := &commitGateProposalOp{validationErr: wantErr}
	meta := newConsensusMeta(
		config.ConsensusNodeCfg{NodeNum: 4},
		config.LocalParams{NodeID: 1, ShardID: 1},
	)
	meta.stage = stageCommit
	proposal := &message.Proposal{
		Block: block.NewBlock(block.Header{Number: 1}, block.Body{}, block.MigrationOpt{}),
	}
	digest, err := proposal.Hash()
	if err != nil {
		t.Fatal(err)
	}
	meta.curPreprepare = &message.PreprepareMsg{
		P: *proposal, Digest: digest, View: 0, Seq: 0, ShardID: 1, NodeID: 0,
	}
	// Four-node PBFT has f=1 and requires 2f+1=3 commit votes.
	meta.commitSet = map[nodetopo.NodeInfo]struct{}{
		{ShardID: 1, NodeID: 0}: {},
		{ShardID: 1, NodeID: 1}: {},
		{ShardID: 1, NodeID: 2}: {},
	}
	node := &Node{iop: op, pbftMeta: meta}

	err = node.step2NextStage(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("step2NextStage() error = %v, want %v", err, wantErr)
	}
	if op.validateCalls != 1 {
		t.Fatalf("ValidateProposal calls = %d, want 1", op.validateCalls)
	}
	if op.commitCalls != 0 {
		t.Fatalf("ProposalCommitAndDeliver calls = %d, want 0 after failed revalidation", op.commitCalls)
	}
	if node.pbftMeta.stage != stageCommit {
		t.Fatalf("PBFT stage = %d, want stageCommit (%d) after failed validation", node.pbftMeta.stage, stageCommit)
	}
}
