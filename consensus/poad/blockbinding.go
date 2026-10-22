package poad

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/block"
)

// BlockPoADEnvelope is the decision material committed by a block header's PoADRoot.
type BlockPoADEnvelope struct {
	Finalization BlockFinalizationRecord `json:"finalization"`
	Decisions    []SignedDecision        `json:"decisions"`
}

// AttachBlockPoADMetadata validates and canonically serializes PoAD evidence into a block.
// The block header commits to the envelope hash; StateRoot is encoded as lowercase hex.
// The caller must still run ordinary block and state-transition validation.
func AttachBlockPoADMetadata(
	b *block.Block,
	registry *IdentityRegistry,
	decisions []SignedDecision,
	finalization BlockFinalizationRecord,
) error {
	if b == nil {
		return fmt.Errorf("block is required")
	}

	if err := requireTransactionInBlock(b, finalization.TransactionHash); err != nil {
		return fmt.Errorf("bind PoAD finalization to block transactions: %w", err)
	}

	stateRoot := hex.EncodeToString(b.StateRoot)
	if err := VerifyBlockFinalization(registry, decisions, finalization, stateRoot); err != nil {
		return fmt.Errorf("verify PoAD finalization before attaching to block: %w", err)
	}

	canonical := append([]SignedDecision(nil), decisions...)
	sort.Slice(canonical, func(i, j int) bool {
		return canonical[i].Decision.AgentID < canonical[j].Decision.AgentID
	})

	payload, err := json.Marshal(BlockPoADEnvelope{
		Finalization: finalization,
		Decisions:    canonical,
	})
	if err != nil {
		return fmt.Errorf("marshal PoAD block envelope: %w", err)
	}

	root := sha256.Sum256(payload)
	b.PoADData = payload
	b.PoADRoot = root[:]

	return nil
}

// VerifyBlockPoADMetadata verifies that the envelope matches the header commitment and
// that its signed decisions reach the recorded quorum for this block's state root.
// It does not replace chain.ValidateBlock or other transaction/state-transition checks.
func VerifyBlockPoADMetadata(b *block.Block, registry *IdentityRegistry) error {
	if b == nil {
		return fmt.Errorf("block is required")
	}

	if len(b.PoADRoot) == 0 && len(b.PoADData) == 0 {
		return nil
	}

	if len(b.PoADRoot) != sha256.Size || len(b.PoADData) == 0 {
		return fmt.Errorf("incomplete PoAD block commitment")
	}

	root := sha256.Sum256(b.PoADData)
	if !equalBytes(root[:], b.PoADRoot) {
		return fmt.Errorf("PoAD envelope does not match block header commitment")
	}

	var envelope BlockPoADEnvelope
	if err := json.Unmarshal(b.PoADData, &envelope); err != nil {
		return fmt.Errorf("decode PoAD block envelope: %w", err)
	}

	if err := requireTransactionInBlock(b, envelope.Finalization.TransactionHash); err != nil {
		return fmt.Errorf("bind committed PoAD finalization to block transactions: %w", err)
	}

	expectedRoot := hex.EncodeToString(b.StateRoot)
	if err := VerifyBlockFinalization(
		registry,
		envelope.Decisions,
		envelope.Finalization,
		expectedRoot,
	); err != nil {
		return fmt.Errorf("verify committed PoAD finalization: %w", err)
	}

	canonical := append([]SignedDecision(nil), envelope.Decisions...)
	sort.Slice(canonical, func(i, j int) bool {
		return canonical[i].Decision.AgentID < canonical[j].Decision.AgentID
	})

	canonicalPayload, err := json.Marshal(BlockPoADEnvelope{
		Finalization: envelope.Finalization,
		Decisions:    canonical,
	})
	if err != nil {
		return fmt.Errorf("canonicalize PoAD block envelope: %w", err)
	}

	if !equalBytes(canonicalPayload, b.PoADData) {
		return fmt.Errorf("PoAD block envelope is not canonically encoded")
	}

	return nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func requireTransactionInBlock(b *block.Block, transactionHash string) error {
	if b == nil {
		return fmt.Errorf("block is required")
	}

	expected := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(transactionHash)), "0x")
	if expected == "" {
		return fmt.Errorf("transaction hash is required")
	}

	for i := range b.TxList {
		hash, err := b.TxList[i].Hash()
		if err != nil {
			return fmt.Errorf("hash block transaction at index %d: %w", i, err)
		}

		if hex.EncodeToString(hash) == expected {
			return nil
		}
	}

	return fmt.Errorf("PoAD transaction hash %q is not present in block", transactionHash)
}

// BlockMetadataValidator adapts the PoAD metadata verifier to the PBFT proposal hook.
type BlockMetadataValidator struct {
	registry    *IdentityRegistry
	intentLinks []TraceIntentLink
}

// NewBlockMetadataValidator creates a validator backed by the supplied trusted identity registry.
func NewBlockMetadataValidator(registry *IdentityRegistry) (*BlockMetadataValidator, error) {
	if registry == nil {
		return nil, fmt.Errorf("identity registry is required")
	}

	return &BlockMetadataValidator{registry: registry}, nil
}

// NewBlockMetadataValidatorWithIntentLinks also binds each committed decision to a
// verified AgentSupervisor payment-intent manifest.
func NewBlockMetadataValidatorWithIntentLinks(
	registry *IdentityRegistry,
	links []TraceIntentLink,
) (*BlockMetadataValidator, error) {
	validator, err := NewBlockMetadataValidator(registry)
	if err != nil {
		return nil, err
	}

	if len(links) == 0 {
		return nil, fmt.Errorf("at least one payment intent link is required")
	}
	validator.intentLinks = append([]TraceIntentLink(nil), links...)

	return validator, nil
}

// ValidatePoADBlock verifies the committed Agent decision envelope and, when
// configured, checks that its finalized transaction matches the payment intent.
func (v *BlockMetadataValidator) ValidatePoADBlock(b *block.Block) error {
	if v == nil || v.registry == nil {
		return fmt.Errorf("PoAD metadata validator is not configured")
	}

	if err := VerifyBlockPoADMetadata(b, v.registry); err != nil {
		return err
	}

	if len(v.intentLinks) == 0 || b == nil || len(b.PoADData) == 0 {
		return nil
	}

	var envelope BlockPoADEnvelope
	if err := json.Unmarshal(b.PoADData, &envelope); err != nil {
		return fmt.Errorf("decode PoAD envelope for intent binding: %w", err)
	}

	for i := range b.TxList {
		hash, err := b.TxList[i].Hash()
		if err != nil {
			return fmt.Errorf("hash transaction for intent binding: %w", err)
		}

		finalizedHash := strings.TrimPrefix(strings.ToLower(envelope.Finalization.TransactionHash), "0x")
		if !strings.EqualFold(hex.EncodeToString(hash), finalizedHash) {
			continue
		}

		intent, err := FindIntentForTransaction(v.intentLinks, b.TxList[i])
		if err != nil {
			return fmt.Errorf("bind finalized transaction to AgentSupervisor intent: %w", err)
		}

		status, reasons, err := VerifyPaymentIntent(intent, b.TxList[i])
		if err != nil {
			return fmt.Errorf("verify AgentSupervisor payment intent: %w", err)
		}

		if status != Pass {
			return fmt.Errorf("AgentSupervisor payment intent did not pass: %s", strings.Join(reasons, ","))
		}
		return nil
	}

	return fmt.Errorf("finalized transaction not found while binding AgentSupervisor intent")
}
