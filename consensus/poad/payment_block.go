package poad

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"

	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/block"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/transaction"
)

// PaymentValidatorInput is one validator's explicit verification result and signing key.
// The private key must belong to Input.AgentID in the trusted registry. Transaction,
// state, and evidence statuses must be populated by their actual validation procedures.
type PaymentValidatorInput struct {
	Input      VerificationInput
	PrivateKey ed25519.PrivateKey
}

// BuildAndAttachPaymentPoADMetadata evaluates and signs each supplied validator's
// decision, requires an ACCEPT quorum, and commits the signed decision set to the block.
// It orchestrates existing PoAD primitives; it does not itself execute transactions,
// independently validate account state, or establish external evidence.
func BuildAndAttachPaymentPoADMetadata(
	b *block.Block,
	registry *IdentityRegistry,
	tx transaction.Transaction,
	links []TraceIntentLink,
	validators []PaymentValidatorInput,
	requiredVotes int,
) error {
	if b == nil {
		return fmt.Errorf("block is required")
	}

	if registry == nil {
		return fmt.Errorf("trusted Agent identity registry is required")
	}

	if len(b.StateRoot) == 0 {
		return fmt.Errorf("validated block state root is required")
	}

	hashBytes, err := tx.Hash()
	if err != nil {
		return fmt.Errorf("hash payment transaction: %w", err)
	}

	hash := hex.EncodeToString(hashBytes)
	if err := requireTransactionInBlock(b, hash); err != nil {
		return fmt.Errorf("bind payment transaction to block: %w", err)
	}

	decisions := make([]SignedDecision, 0, len(validators))
	for _, validator := range validators {
		input := validator.Input
		if input.TransactionHash != hash {
			return fmt.Errorf("validator %q transaction hash does not match payment transaction", input.AgentID)
		}

		signed, err := EvaluateAndSignPaymentDecision(input, tx, links, validator.PrivateKey)
		if err != nil {
			return fmt.Errorf("evaluate payment decision for %q: %w", input.AgentID, err)
		}

		decisions = append(decisions, signed)
	}

	finalization, err := PrepareBlockFinalization(
		registry,
		decisions,
		hash,
		DefaultPolicyVersion,
		requiredVotes,
		hex.EncodeToString(b.StateRoot),
	)
	if err != nil {
		return fmt.Errorf("prepare PoAD block finalization: %w", err)
	}

	return AttachBlockPoADMetadata(b, registry, decisions, finalization)
}
