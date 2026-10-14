package poad

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// IdentityStatus describes whether an AgentID may authenticate new decisions.
type IdentityStatus string

const (
	IdentityActive  IdentityStatus = "ACTIVE"
	IdentityRevoked IdentityStatus = "REVOKED"
)

// AgentIdentity binds one stable AgentID to its current Ed25519 public key.
// KeyVersion increases whenever the key is rotated.
type AgentIdentity struct {
	AgentID    string            `json:"agent_id"`
	PublicKey  ed25519.PublicKey `json:"public_key"`
	Status     IdentityStatus    `json:"status"`
	KeyVersion uint64            `json:"key_version"`
}

// IdentityRegistry is an in-memory prototype for trusted AgentID-to-key binding.
// Register and Revoke are governance/bootstrap operations; they must not be exposed
// to untrusted callers. Consensus deployments must replicate these changes through
// an agreed, authenticated registry or on-chain governance mechanism.
type IdentityRegistry struct {
	mu         sync.RWMutex
	identities map[string]AgentIdentity
}

// NewIdentityRegistry creates an empty registry.
func NewIdentityRegistry() *IdentityRegistry {
	return &IdentityRegistry{identities: make(map[string]AgentIdentity)}
}

// Register adds a new AgentID. Existing IDs cannot be silently overwritten.
func (r *IdentityRegistry) Register(agentID string, publicKey ed25519.PublicKey) error {
	agentID = strings.TrimSpace(agentID)

	if agentID == "" {
		return fmt.Errorf("agent ID is required")
	}

	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid Ed25519 public key length")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.identities[agentID]; exists {
		return fmt.Errorf("agent identity %q already exists", agentID)
	}

	r.identities[agentID] = AgentIdentity{
		AgentID:    agentID,
		PublicKey:  clonePublicKey(publicKey),
		Status:     IdentityActive,
		KeyVersion: 1,
	}

	return nil
}

// Resolve returns a copy of the active public key for an AgentID.
func (r *IdentityRegistry) Resolve(agentID string) (ed25519.PublicKey, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	identity, exists := r.identities[strings.TrimSpace(agentID)]
	if !exists {
		return nil, fmt.Errorf("unknown agent identity %q", agentID)
	}
	if identity.Status != IdentityActive {
		return nil, fmt.Errorf("agent identity %q is not active", agentID)
	}

	return clonePublicKey(identity.PublicKey), nil
}

// Get returns a defensive copy of an identity record, including revoked records.
func (r *IdentityRegistry) Get(agentID string) (AgentIdentity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	identity, exists := r.identities[strings.TrimSpace(agentID)]
	if !exists {
		return AgentIdentity{}, fmt.Errorf("unknown agent identity %q", agentID)
	}

	identity.PublicKey = clonePublicKey(identity.PublicKey)

	return identity, nil
}

// Revoke disables an identity. This prototype treats revocation as a trusted
// governance operation; production use needs an agreed and auditable authorization path.
func (r *IdentityRegistry) Revoke(agentID string) error {
	agentID = strings.TrimSpace(agentID)

	r.mu.Lock()
	defer r.mu.Unlock()

	identity, exists := r.identities[agentID]
	if !exists {
		return fmt.Errorf("unknown agent identity %q", agentID)
	}

	if identity.Status == IdentityRevoked {
		return fmt.Errorf("agent identity %q is already revoked", agentID)
	}

	identity.Status = IdentityRevoked
	r.identities[agentID] = identity

	return nil
}

// KeyRotation is an authorization to replace an Agent's public key. The current
// key signs AgentID, current key version, and the proposed new public key.
type KeyRotation struct {
	AgentID        string             `json:"agent_id"`
	CurrentVersion uint64             `json:"current_version"`
	NewPublicKey   ed25519.PublicKey `json:"new_public_key"`
	Signature      []byte             `json:"signature"`
}

type keyRotationPayload struct {
	AgentID        string            `json:"agent_id"`
	CurrentVersion uint64            `json:"current_version"`
	NewPublicKey   ed25519.PublicKey `json:"new_public_key"`
}

// SignKeyRotation authorizes a key change using the currently registered private key.
func SignKeyRotation(
	agentID string,
	currentVersion uint64,
	newPublicKey ed25519.PublicKey,
	currentPrivateKey ed25519.PrivateKey,
) (KeyRotation, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return KeyRotation{}, fmt.Errorf("agent ID is required")
	}
	if len(newPublicKey) != ed25519.PublicKeySize {
		return KeyRotation{}, fmt.Errorf("invalid new Ed25519 public key length")
	}
	if len(currentPrivateKey) != ed25519.PrivateKeySize {
		return KeyRotation{}, fmt.Errorf("invalid current Ed25519 private key length")
	}

	payload, err := json.Marshal(keyRotationPayload{
		AgentID:        agentID,
		CurrentVersion: currentVersion,
		NewPublicKey:   clonePublicKey(newPublicKey),
	})
	if err != nil {
		return KeyRotation{}, fmt.Errorf("marshal key rotation: %w", err)
	}
	return KeyRotation{
		AgentID:        agentID,
		CurrentVersion: currentVersion,
		NewPublicKey:   clonePublicKey(newPublicKey),
		Signature:      ed25519.Sign(currentPrivateKey, payload),
	}, nil
}

// Rotate changes an active identity's key only when the current key authorizes
// the exact AgentID, version, and replacement key. Old rotation requests cannot
// be replayed after a successful rotation because the version has advanced.
func (r *IdentityRegistry) Rotate(rotation KeyRotation) error {
	agentID := strings.TrimSpace(rotation.AgentID)
	if agentID == "" {
		return fmt.Errorf("agent ID is required")
	}
	if len(rotation.NewPublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid new Ed25519 public key length")
	}
	if len(rotation.Signature) != ed25519.SignatureSize {
		return fmt.Errorf("invalid key rotation signature length")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	identity, exists := r.identities[agentID]
	if !exists {
		return fmt.Errorf("unknown agent identity %q", agentID)
	}
	if identity.Status != IdentityActive {
		return fmt.Errorf("agent identity %q is not active", agentID)
	}

	if rotation.CurrentVersion != identity.KeyVersion {
		return fmt.Errorf("stale key rotation version: got %d, want %d", rotation.CurrentVersion, identity.KeyVersion)
	}

	payload, err := json.Marshal(keyRotationPayload{
		AgentID:        agentID,
		CurrentVersion: rotation.CurrentVersion,
		NewPublicKey:   clonePublicKey(rotation.NewPublicKey),
	})
	if err != nil {
		return fmt.Errorf("marshal key rotation: %w", err)
	}

	if !ed25519.Verify(identity.PublicKey, payload, rotation.Signature) {
		return fmt.Errorf("invalid key rotation authorization")
	}

	identity.PublicKey = clonePublicKey(rotation.NewPublicKey)
	identity.KeyVersion++
	r.identities[agentID] = identity

	return nil
}

// VerifyDecision resolves the trusted key from the registry before checking the
// signature. It never trusts a public key supplied by the decision submitter.
func (r *IdentityRegistry) VerifyDecision(signed SignedDecision) error {
	publicKey, err := r.Resolve(signed.Decision.AgentID)
	if err != nil {
		return err
	}
	return VerifyDecisionSignature(signed, publicKey, signed.Decision.AgentID)
}

// GenerateAgentKeyPair creates an Ed25519 key pair for tests and local prototypes.
// Production agents should protect private keys in an appropriate secure store.
func GenerateAgentKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate Agent key pair: %w", err)
	}

	return publicKey, privateKey, nil
}

func clonePublicKey(key ed25519.PublicKey) ed25519.PublicKey {
	return append(ed25519.PublicKey(nil), key...)
}
