package poad

import (
	"crypto/ed25519"
	"fmt"

	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/transaction"
)

// EvaluateAndSignPaymentDecision evaluates one transaction against the trace-linked
// payment intent and signs the resulting deterministic decision.
//
// Transaction, state, and evidence statuses remain explicit inputs: this function
// does not claim to validate account state or external evidence on its own. Callers
// must populate those statuses only from the corresponding checks. The returned
// signature authenticates the decision bytes, not the correctness of the checks.
func EvaluateAndSignPaymentDecision(
	in VerificationInput,
	tx transaction.Transaction,
	links []TraceIntentLink,
	privateKey ed25519.PrivateKey,
) (SignedDecision, error) {
	decision, err := EvaluateWithTraceIntent(in, tx, links)
	if err != nil {
		return SignedDecision{}, fmt.Errorf("evaluate payment intent decision: %w", err)
	}

	signed, err := SignDecision(decision, privateKey)
	if err != nil {
		return SignedDecision{}, fmt.Errorf("sign payment intent decision: %w", err)
	}

	return signed, nil
}
