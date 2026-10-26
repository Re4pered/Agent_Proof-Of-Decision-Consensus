package poad

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// identityRegistryFileEntry is the public, non-secret bootstrap representation for
// an Agent identity. PublicKeyHex is an Ed25519 public key encoded as lowercase or
// uppercase hexadecimal; private keys must never be placed in this file.
type identityRegistryFileEntry struct {
	AgentID      string         `json:"agent_id"`
	PublicKeyHex string         `json:"public_key_hex"`
	Status       IdentityStatus `json:"status,omitempty"`
	KeyVersion   uint64         `json:"key_version,omitempty"`
}

// LoadIdentityRegistryJSON loads a deterministic identity-registry snapshot only
// after its exact bytes match an independently provisioned SHA-256 digest. Every
// PBFT validator must be provisioned with the same trusted digest and file bytes.
// A digest stored only beside an untrusted file does not authenticate the file.
func LoadIdentityRegistryJSON(data []byte, expectedSHA256 string) (*IdentityRegistry, error) {
	expected := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(expectedSHA256)), "0x")
	if len(expected) != sha256.Size*2 {
		return nil, fmt.Errorf("expected registry SHA-256 must contain 64 hexadecimal characters")
	}

	if _, err := hex.DecodeString(expected); err != nil {
		return nil, fmt.Errorf("decode expected registry SHA-256: %w", err)
	}

	actual := sha256.Sum256(data)
	if hex.EncodeToString(actual[:]) != expected {
		return nil, fmt.Errorf("identity registry SHA-256 mismatch")
	}

	var entries []identityRegistryFileEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("decode identity registry: %w", err)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("identity registry must contain at least one Agent")
	}

	registry := NewIdentityRegistry()

	for i, entry := range entries {
		agentID := strings.TrimSpace(entry.AgentID)

		if agentID == "" {
			return nil, fmt.Errorf("identity registry entry %d has an empty agent_id", i)
		}

		if _, exists := registry.identities[agentID]; exists {
			return nil, fmt.Errorf("duplicate Agent identity %q", agentID)
		}

		publicKeyBytes, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(entry.PublicKeyHex), "0x"))
		if err != nil {
			return nil, fmt.Errorf("decode public_key_hex for agent %q: %w", agentID, err)
		}

		if len(publicKeyBytes) != 32 {
			return nil, fmt.Errorf("agent %q public key must be a 32-byte Ed25519 key", agentID)
		}

		status := entry.Status

		if status == "" {
			status = IdentityActive
		}

		if status != IdentityActive && status != IdentityRevoked {
			return nil, fmt.Errorf("agent %q has unsupported identity status %q", agentID, status)
		}

		keyVersion := entry.KeyVersion

		if keyVersion == 0 {
			keyVersion = 1
		}

		registry.identities[agentID] = AgentIdentity{
			AgentID:    agentID,
			PublicKey:  clonePublicKey(ed25519.PublicKey(publicKeyBytes)),
			Status:     status,
			KeyVersion: keyVersion,
		}
	}

	return registry, nil
}
