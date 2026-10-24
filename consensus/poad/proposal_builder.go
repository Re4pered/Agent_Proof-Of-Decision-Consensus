package poad

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/block"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/transaction"
)

// SignedDecisionProvider obtains decisions signed by their respective Agent identities.
// Implementations must not sign on behalf of agents; they may collect decisions from
// AgentSupervisor instances or another authenticated decision transport.
type SignedDecisionProvider interface {
	CollectSignedDecisions(context.Context, transaction.Transaction, TraceIntentLink, string) ([]SignedDecision, error)
}

// PaymentProposalBuilder attaches PoAD metadata using independently signed decisions.
// It performs deterministic intent-link checks and quorum/signature verification, but
// relies on the normal execution path for the block state root and on the provider for
// actual per-Agent verification results.
type PaymentProposalBuilder struct {
	registry      *IdentityRegistry
	intentLinks   []TraceIntentLink
	provider      SignedDecisionProvider
	requiredVotes int
}

// NewPaymentProposalBuilder creates a fail-closed proposal builder for single-payment
// transaction blocks. The trusted registry and intent links must be identically provisioned
// on all validators; the provider must return decisions signed by the corresponding agents.
func NewPaymentProposalBuilder(
	registry *IdentityRegistry,
	intentLinks []TraceIntentLink,
	provider SignedDecisionProvider,
	requiredVotes int,
) (*PaymentProposalBuilder, error) {
	if registry == nil {
		return nil, fmt.Errorf("trusted Agent identity registry is required")
	}

	if len(intentLinks) == 0 {
		return nil, fmt.Errorf("trusted payment intent links are required")
	}

	if provider == nil {
		return nil, fmt.Errorf("signed Agent decision provider is required")
	}

	if requiredVotes < 1 {
		return nil, fmt.Errorf("required Agent decision votes must be positive")
	}

	return &PaymentProposalBuilder{
		registry:      registry,
		intentLinks:   append([]TraceIntentLink(nil), intentLinks...),
		provider:      provider,
		requiredVotes: requiredVotes,
	}, nil
}

// BuildPoADMetadata binds independently signed Agent decisions to the block's sole
// payment transaction and state root. Missing input, mismatched intent, invalid signatures,
// or insufficient ACCEPT quorum abort proposal creation.
func (b *PaymentProposalBuilder) BuildPoADMetadata(ctx context.Context, blockValue *block.Block) error {
	if b == nil || b.registry == nil || b.provider == nil {
		return fmt.Errorf("PoAD payment proposal builder is not configured")
	}

	if blockValue == nil {
		return fmt.Errorf("proposal block is required")
	}

	if blockValue.Type == block.MigrationBlockType {
		return nil
	}

	if len(blockValue.TxList) != 1 {
		return fmt.Errorf("PoAD payment proposal requires exactly one transaction")
	}

	if len(blockValue.StateRoot) == 0 {
		return fmt.Errorf("validated block state root is required")
	}

	tx := blockValue.TxList[0]

	hashBytes, err := tx.Hash()
	if err != nil {
		return fmt.Errorf("hash proposal transaction: %w", err)
	}

	txHash := hex.EncodeToString(hashBytes)

	intent, err := FindIntentForTransaction(b.intentLinks, tx)
	if err != nil {
		return fmt.Errorf("resolve AgentSupervisor intent for proposal transaction: %w", err)
	}

	status, reasons, err := VerifyPaymentIntent(intent, tx)
	if err != nil {
		return fmt.Errorf("verify AgentSupervisor payment intent: %w", err)
	}

	if status != Pass {
		return fmt.Errorf("AgentSupervisor payment intent rejected proposal: %v", reasons)
	}

	decisions, err := b.provider.CollectSignedDecisions(ctx, tx, intent, hex.EncodeToString(blockValue.StateRoot))
	if err != nil {
		return fmt.Errorf("collect independently signed Agent decisions: %w", err)
	}

	if len(decisions) == 0 {
		return fmt.Errorf("signed Agent decision provider returned no decisions")
	}

	for i, signed := range decisions {
		if signed.Decision.TransactionHash != txHash {
			return fmt.Errorf("agent decision %d is bound to a different transaction", i)
		}

		if err := b.registry.VerifyDecision(signed); err != nil {
			return fmt.Errorf("verify Agent decision %d: %w", i, err)
		}
	}

	finalization, err := PrepareBlockFinalization(
		b.registry,
		decisions,
		txHash,
		DefaultPolicyVersion,
		b.requiredVotes,
		hex.EncodeToString(blockValue.StateRoot),
	)
	if err != nil {
		return fmt.Errorf("prepare PoAD finalization: %w", err)
	}

	if err := AttachBlockPoADMetadata(blockValue, b.registry, decisions, finalization); err != nil {
		return fmt.Errorf("attach PoAD metadata: %w", err)
	}

	return nil
}
