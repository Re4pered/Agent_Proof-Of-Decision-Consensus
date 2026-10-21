package pbft

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HuangLab-SYSU/block-emulator-x/config"
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
