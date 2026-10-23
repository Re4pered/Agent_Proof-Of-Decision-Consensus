package insideop

import (
	"context"
	"fmt"

	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/block"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/message"
)

// PoADBlockValidator verifies the Agent decision envelope committed by a block.
type PoADBlockValidator interface {
	ValidatePoADBlock(*block.Block) error
}

// PoADConfigurable is implemented by shard operations that can enforce PoAD metadata.
type PoADConfigurable interface {
	SetPoADValidation(PoADBlockValidator, bool)
}

// PoADProposalBuilder attaches authenticated Agent decisions before a leader starts PBFT.
// Implementations must use actual transaction, state, and evidence validation results.
type PoADProposalBuilder interface {
	BuildPoADMetadata(context.Context, *block.Block) error
}

// PoADProposalBuildConfigurable exposes proposal-time PoAD metadata generation.
type PoADProposalBuildConfigurable interface {
	SetPoADProposalBuilder(PoADProposalBuilder)
}

// buildPoADProposalMetadata enriches a transaction proposal before PBFT validation.
func buildPoADProposalMetadata(
	ctx context.Context,
	b *block.Block,
	builder PoADProposalBuilder,
	required bool,
) error {
	if b == nil {
		return fmt.Errorf("proposal block is required")
	}

	if b.Type == block.MigrationBlockType {
		return nil
	}

	if builder == nil {
		if required {
			return fmt.Errorf("PoAD proposal builder is required to build transaction proposals")
		}

		return nil
	}

	if err := builder.BuildPoADMetadata(ctx, b); err != nil {
		return fmt.Errorf("build PoAD proposal metadata: %w", err)
	}

	return nil
}

// validatePoADProposal checks metadata presence and invokes the configured validator.
// requirePoAD applies to transaction blocks; migration blocks remain exempt unless they
// explicitly carry PoAD metadata, in which case that metadata must still be verified.
func validatePoADProposal(b *block.Block, validator PoADBlockValidator, requirePoAD bool) error {
	if b == nil {
		return fmt.Errorf("proposal block is required")
	}

	if requirePoAD && b.Type != block.MigrationBlockType && len(b.TxList) != 1 {
		return fmt.Errorf("required PoAD mode currently supports exactly one transaction per block")
	}

	hasRoot := len(b.PoADRoot) > 0

	hasData := len(b.PoADData) > 0

	if !hasRoot && !hasData {
		if requirePoAD && b.Type != block.MigrationBlockType {
			return fmt.Errorf("PoAD metadata is required for transaction proposals")
		}

		return nil
	}

	if !hasRoot || !hasData {
		return fmt.Errorf("incomplete PoAD block commitment")
	}

	if validator == nil {
		return fmt.Errorf("PoAD metadata is present but no PoAD validator is configured")
	}

	if err := validator.ValidatePoADBlock(b); err != nil {
		return fmt.Errorf("verify PoAD block metadata: %w", err)
	}

	return nil
}

// ShardInsideOp defines the operations inside the shard for a consensus node.
type ShardInsideOp interface {
	// BuildProposal build a proposal for a round of the PBFT consensus.
	// Note that, this function is normally called by the leader.
	// If both the returned proposal and the returned error are nil, the leader of PBFT should not propose now.
	BuildProposal(ctx context.Context) (*message.Proposal, error)
	// ValidateProposal validates the proposal.
	ValidateProposal(ctx context.Context, proposal *message.Proposal) error
	// ProposalCommitAndDeliver commits the given proposal and deliver the related messages to the supervisor or other shards.
	ProposalCommitAndDeliver(ctx context.Context, isLeader bool, proposal *message.Proposal) error
}
