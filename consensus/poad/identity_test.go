package poad

import (
	"crypto/ed25519"
	"testing"
)

func newTestKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	publicKey, privateKey, err := GenerateAgentKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	return publicKey, privateKey
}

func TestIdentityRegistryRegisterResolveAndDuplicate(t *testing.T) {
	registry := NewIdentityRegistry()
	publicKey, _ := newTestKeyPair(t)

	if err := registry.Register(" agent-1 ", publicKey); err != nil {
		t.Fatal(err)
	}
	resolved, err := registry.Resolve("agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.PublicKey(resolved).Equal(publicKey) {
		t.Fatal("Resolve() returned a different public key")
	}
	if err := registry.Register("agent-1", publicKey); err == nil {
		t.Fatal("Register() should reject duplicate AgentID")
	}
}

func TestIdentityRegistryRejectsInvalidRegistration(t *testing.T) {
	registry := NewIdentityRegistry()
	if err := registry.Register("agent-1", ed25519.PublicKey{1, 2}); err == nil {
		t.Fatal("Register() should reject an invalid public key")
	}
	if err := registry.Register(" ", make(ed25519.PublicKey, ed25519.PublicKeySize)); err == nil {
		t.Fatal("Register() should reject an empty AgentID")
	}
}

func TestIdentityRegistryVerifiesDecisionUsingRegisteredKey(t *testing.T) {
	registry := NewIdentityRegistry()
	publicKey, privateKey := newTestKeyPair(t)
	if err := registry.Register("agent-1", publicKey); err != nil {
		t.Fatal(err)
	}
	signed, err := SignDecision(Decision{
		AgentID:         "agent-1",
		TransactionHash: "tx-1",
		Result:          Accept,
		PolicyVersion:   DefaultPolicyVersion,
	}, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.VerifyDecision(signed); err != nil {
		t.Fatalf("VerifyDecision() error = %v", err)
	}
}

func TestIdentityRegistryRevocationBlocksResolutionAndVerification(t *testing.T) {
	registry := NewIdentityRegistry()
	publicKey, privateKey := newTestKeyPair(t)
	if err := registry.Register("agent-1", publicKey); err != nil {
		t.Fatal(err)
	}
	signed, err := SignDecision(Decision{AgentID: "agent-1", Result: Accept}, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Revoke("agent-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve("agent-1"); err == nil {
		t.Fatal("Resolve() should reject a revoked identity")
	}
	if err := registry.VerifyDecision(signed); err == nil {
		t.Fatal("VerifyDecision() should reject a revoked identity")
	}
}

func TestIdentityRegistryRotatesKeyWithOldKeyAuthorization(t *testing.T) {
	registry := NewIdentityRegistry()
	oldPublicKey, oldPrivateKey := newTestKeyPair(t)
	newPublicKey, _ := newTestKeyPair(t)
	if err := registry.Register("agent-1", oldPublicKey); err != nil {
		t.Fatal(err)
	}
	rotation, err := SignKeyRotation("agent-1", 1, newPublicKey, oldPrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Rotate(rotation); err != nil {
		t.Fatal(err)
	}
	identity, err := registry.Get("agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if identity.KeyVersion != 2 {
		t.Fatalf("KeyVersion = %d, want 2", identity.KeyVersion)
	}
	resolved, err := registry.Resolve("agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.Equal(newPublicKey) {
		t.Fatal("Resolve() returned the old public key after rotation")
	}
	if err := registry.Rotate(rotation); err == nil {
		t.Fatal("Rotate() should reject replay of an old rotation")
	}
}

func TestIdentityRegistryRejectsUnauthorizedRotation(t *testing.T) {
	registry := NewIdentityRegistry()
	publicKey, _ := newTestKeyPair(t)
	_, attackerPrivateKey := newTestKeyPair(t)
	newPublicKey, _ := newTestKeyPair(t)
	if err := registry.Register("agent-1", publicKey); err != nil {
		t.Fatal(err)
	}
	rotation, err := SignKeyRotation("agent-1", 1, newPublicKey, attackerPrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Rotate(rotation); err == nil {
		t.Fatal("Rotate() should reject a rotation not authorized by the current key")
	}
}

func TestIdentityRegistryReturnsDefensiveKeyCopies(t *testing.T) {
	registry := NewIdentityRegistry()
	publicKey, _ := newTestKeyPair(t)
	if err := registry.Register("agent-1", publicKey); err != nil {
		t.Fatal(err)
	}
	resolved, err := registry.Resolve("agent-1")
	if err != nil {
		t.Fatal(err)
	}
	resolved[0] ^= 0xff
	again, err := registry.Resolve("agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if again[0] != publicKey[0] {
		t.Fatal("mutating a resolved key should not mutate registry state")
	}
}
