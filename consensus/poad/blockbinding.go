package poad

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

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


// BlockMetadataValidator adapts the PoAD metadata verifier to the PBFT proposal hook.
type BlockMetadataValidator struct {
	registry *IdentityRegistry
}

// NewBlockMetadataValidator creates a validator backed by the supplied trusted identity registry.
func NewBlockMetadataValidator(registry *IdentityRegistry) (*BlockMetadataValidator, error) {
	if registry == nil {
		return nil, fmt.Errorf("identity registry is required")
	}

	return &BlockMetadataValidator{registry: registry}, nil
}

// ValidatePoADBlock verifies the block's committed Agent decision envelope.
func (v *BlockMetadataValidator) ValidatePoADBlock(b *block.Block) error {
	if v == nil || v.registry == nil {
		return fmt.Errorf("PoAD metadata validator is not configured")
	}

	return VerifyBlockPoADMetadata(b, v.registry)
}
