package poad

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func registryFixture(t *testing.T) ([]byte, string, string) {
	t.Helper()
	publicKey, _, err := GenerateAgentKeyPair()
	if err != nil {
		t.Fatalf("generate Agent key pair: %v", err)
	}
	entries := []identityRegistryFileEntry{{
		AgentID:      "agent-1",
		PublicKeyHex: hex.EncodeToString(publicKey),
		Status:       IdentityActive,
		KeyVersion:   2,
	}}
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal registry fixture: %v", err)
	}
	digest := sha256.Sum256(data)
	return data, hex.EncodeToString(digest[:]), hex.EncodeToString(publicKey)
}

func TestLoadIdentityRegistryJSONLoadsTrustedSnapshot(t *testing.T) {
	data, digest, publicKeyHex := registryFixture(t)
	registry, err := LoadIdentityRegistryJSON(data, digest)
	if err != nil {
		t.Fatalf("LoadIdentityRegistryJSON() error = %v", err)
	}

	identity, err := registry.Get("agent-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if hex.EncodeToString(identity.PublicKey) != publicKeyHex {
		t.Fatal("loaded public key does not match registry snapshot")
	}
	if identity.KeyVersion != 2 {
		t.Fatalf("KeyVersion = %d, want 2", identity.KeyVersion)
	}
}

func TestLoadIdentityRegistryJSONRejectsDigestMismatch(t *testing.T) {
	data, _, _ := registryFixture(t)
	if _, err := LoadIdentityRegistryJSON(data, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil {
		t.Fatal("expected registry digest mismatch to be rejected")
	}
}

func TestLoadIdentityRegistryJSONRejectsDuplicateAgent(t *testing.T) {
	data, _, _ := registryFixture(t)
	var entries []identityRegistryFileEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	entries = append(entries, entries[0])
	duplicateData, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal duplicate fixture: %v", err)
	}
	actual := sha256.Sum256(duplicateData)
	_, err = LoadIdentityRegistryJSON(duplicateData, hex.EncodeToString(actual[:]))
	if err == nil {
		t.Fatal("expected duplicate Agent identity to be rejected")
	}
}

func TestLoadIdentityRegistryJSONRejectsInvalidPublicKey(t *testing.T) {
	entries := []identityRegistryFileEntry{{AgentID: "agent-1", PublicKeyHex: "abcd"}}
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal registry fixture: %v", err)
	}
	digest := sha256.Sum256(data)
	if _, err := LoadIdentityRegistryJSON(data, hex.EncodeToString(digest[:])); err == nil {
		t.Fatal("expected invalid public key length to be rejected")
	}
}
