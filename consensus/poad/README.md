# PoAD Core: initial rule-based version

This package is the first isolated implementation of the Proof of Agent Decision (PoAD)
decision contract. It is independent from PBFT so the policy can be tested without
changing consensus messages or block commit behavior.

## Decision rule

- ACCEPT: transaction and state checks pass, and intent/evidence checks pass or are not required.
- REJECT: at least one verification dimension is known to have failed.
- ABSTAIN: no known failure exists, but a required check is unknown or invalid.

VerificationInput carries statuses for transaction, state, intent, and evidence. This
first version does not claim to perform semantic intent inference or to prove hidden
agent reasoning. Those capabilities require explicit input schemas and separately tested
verifiers/models.

## Next integration step

After running go test ./consensus/poad, trace the current transaction metadata path and
decide how intent/evidence fields should be associated with a transaction hash. Only then
should this package be wired into ShardInsideOp.ValidateProposal; ordinary block validity
checks must remain in place.


## Trace-linked payment intent verification (phase 2)

AgentEmulator's `agent_action_txs.jsonl` now also records the trace-derived expected
sender and recipient addresses for `pay` actions. The `poad.VerifyPaymentIntent` helper
checks that a compiled transaction hash is linked to the originating request and that
its transfer type, sender, recipient, and amount match the trace-derived intent.

This is an off-chain linkage/checking step. It is not yet wired into PBFT proposal
validation, and the JSONL manifest is not itself authenticated consensus evidence.
The next local step now composes this helper with the deterministic policy through
`EvaluateWithTraceIntent`: a matching payment intent contributes PASS/FAIL to the PoAD
decision, while missing or ambiguous links result in UNKNOWN and normally ABSTAIN.
The helper also checks that the decision's transaction hash matches the transaction being
evaluated.

This remains an off-chain decision path. It does not authenticate the JSONL manifest and
is not wired into PBFT proposal validation. Before consensus integration, validators must
obtain the same manifest from a defined trusted/committed source and verify its integrity;
ordinary transaction and state-transition checks must remain mandatory.
