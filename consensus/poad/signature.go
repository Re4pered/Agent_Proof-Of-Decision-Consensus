package poad

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
)

// SignedDecision carries an Agent Validator's decision and its Ed25519 signature.
// The public key must be resolved through a trusted AgentID-to-key registry by callers.
type SignedDecision struct {
	Decision  Decision `json:"decision"`
	Signature []byte   `json:"signature"`
}

// SignDecision signs the JSON encoding of the decision with an Ed25519 private key.
// This authenticates the signed bytes, not the truth of the agent's reasoning.
func SignDecision(decision Decision, privateKey ed25519.PrivateKey) (SignedDecision, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return SignedDecision{}, fmt.Errorf("invalid Ed25519 private key length")
	}

	payload, err := json.Marshal(decision)
	if err != nil {
		return SignedDecision{}, fmt.Errorf("marshal PoAD decision: %w", err)
	}

	signature := ed25519.Sign(privateKey, payload)

	return SignedDecision{
		Decision:  decision,
		Signature: signature,
	}, nil
}

// VerifyDecisionSignature checks the signature and expected AgentID.
// The supplied public key must already be trusted as belonging to expectedAgentID.
func VerifyDecisionSignature(
	signed SignedDecision,
	publicKey ed25519.PublicKey,
	expectedAgentID string,
) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid Ed25519 public key length")
	}

	if len(signed.Signature) != ed25519.SignatureSize {
		return fmt.Errorf("invalid PoAD decision signature length")
	}

	if signed.Decision.AgentID != expectedAgentID {
		return fmt.Errorf("decision agent ID does not match expected signer")
	}

	payload, err := json.Marshal(signed.Decision)
	if err != nil {
		return fmt.Errorf("marshal PoAD decision: %w", err)
	}

	if !ed25519.Verify(publicKey, payload, signed.Signature) {
		return fmt.Errorf("invalid PoAD decision signature")
	}

	return nil
}
