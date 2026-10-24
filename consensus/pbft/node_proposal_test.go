package pbft

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/HuangLab-SYSU/block-emulator-x/config"
	"github.com/HuangLab-SYSU/block-emulator-x/consensus/pbft/basicstructs"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/block"
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
	validateCalls int
}

func (*replicaProposalOp) BuildProposal(context.Context) (*message.Proposal, error) {
	return nil, fmt.Errorf("not used in replica ingress test")
}

func (op *replicaProposalOp) ValidateProposal(context.Context, *message.Proposal) error {
	op.validateCalls++
	return op.validationErr
}

func (*replicaProposalOp) ProposalCommitAndDeliver(context.Context, bool, *message.Proposal) error {
	return nil
}
