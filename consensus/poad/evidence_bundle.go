package poad

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/block"
)

// BlockEvidenceBundle is an externally produced, pre-signed PoAD evidence package.
// It deliberately contains no private keys. A separate Agent decision service can
// produce this bundle; PBFT nodes only verify it against their trusted registry.
type BlockEvidenceBundle struct {
	TransactionHash string                  `json:"transaction_hash"`
	Decisions       []SignedDecision        `json:"decisions"`
	Finalization    BlockFinalizationRecord `json:"finalization"`
}

// LoadBlockEvidenceBundleVerified loads a bundle only if its exact bytes match an
// independently provisioned digest. Do not trust a digest shipped beside an
// untrusted bundle; provision the expected digest through a trusted channel.
func LoadBlockEvidenceBundleVerified(reader io.Reader, expectedSHA256 string) (BlockEvidenceBundle, error) {
	expected := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(expectedSHA256)), "0x")

	if len(expected) != sha256.Size*2 {
		return BlockEvidenceBundle{}, fmt.Errorf(
			"expected evidence bundle SHA-256 must contain 64 hexadecimal characters",
		)
	}

	if _, err := hex.DecodeString(expected); err != nil {
		return BlockEvidenceBundle{}, fmt.Errorf("decode expected evidence bundle SHA-256: %w", err)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return BlockEvidenceBundle{}, fmt.Errorf("read PoAD evidence bundle: %w", err)
	}

	actual := sha256.Sum256(data)
	if hex.EncodeToString(actual[:]) != expected {
		return BlockEvidenceBundle{}, fmt.Errorf("PoAD evidence bundle SHA-256 mismatch")
	}

	var bundle BlockEvidenceBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return BlockEvidenceBundle{}, fmt.Errorf("decode PoAD evidence bundle: %w", err)
	}

	if strings.TrimSpace(bundle.TransactionHash) == "" {
		return BlockEvidenceBundle{}, fmt.Errorf("evidence bundle transaction hash is required")
	}

	if len(bundle.Decisions) == 0 {
		return BlockEvidenceBundle{}, fmt.Errorf("evidence bundle must contain signed Agent decisions")
	}

	if !strings.EqualFold(
		normalizeHash(bundle.TransactionHash),
		normalizeHash(bundle.Finalization.TransactionHash),
	) {
		return BlockEvidenceBundle{}, fmt.Errorf("evidence bundle transaction hash does not match finalization")
	}

	for i, signed := range bundle.Decisions {
		if !strings.EqualFold(
			normalizeHash(signed.Decision.TransactionHash),
			normalizeHash(bundle.TransactionHash),
		) {
			return BlockEvidenceBundle{}, fmt.Errorf("decision %d references a different transaction", i)
		}
	}

	return bundle, nil
}

// AttachVerifiedBlockEvidence attaches a previously digest-verified evidence bundle
// after block execution has produced its state root. AttachBlockPoADMetadata rechecks
// signatures, quorum, transaction inclusion, and state-root binding before mutation.
// The caller must still independently validate ordinary transaction/state execution.
func AttachVerifiedBlockEvidence(b *block.Block, registry *IdentityRegistry, bundle BlockEvidenceBundle) error {
	if b == nil {
		return fmt.Errorf("block is required")
	}

	if !strings.EqualFold(
		normalizeHash(bundle.TransactionHash),
		normalizeHash(bundle.Finalization.TransactionHash),
	) {
		return fmt.Errorf("evidence bundle transaction hash does not match finalization")
	}

	if err := requireTransactionInBlock(b, bundle.TransactionHash); err != nil {
		return fmt.Errorf("bind evidence bundle to proposed block: %w", err)
	}

	if err := AttachBlockPoADMetadata(b, registry, bundle.Decisions, bundle.Finalization); err != nil {
		return fmt.Errorf("attach verified PoAD evidence: %w", err)
	}

	return nil
}

func normalizeHash(value string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "0x")
}

// EvidenceBundleDigest returns the digest of the exact serialized bytes callers
// provision out-of-band. It is intended for tooling and tests, not as a trust anchor
// when the bundle and digest are both received from the same untrusted source.
func EvidenceBundleDigest(data []byte) string {
	digest := sha256.Sum256(data)

	return hex.EncodeToString(digest[:])
}

// MarshalBlockEvidenceBundle serializes a bundle deterministically using encoding/json.
// Callers should store and distribute the resulting exact bytes with a trusted digest.
func MarshalBlockEvidenceBundle(bundle BlockEvidenceBundle) ([]byte, error) {
	data, err := json.Marshal(bundle)
	if err != nil {
		return nil, fmt.Errorf("marshal PoAD evidence bundle: %w", err)
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		return nil, fmt.Errorf("canonicalize PoAD evidence bundle: %w", err)
	}

	return compact.Bytes(), nil
}
