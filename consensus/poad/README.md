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
The next phase must define how every validator obtains the same manifest and verifies
its integrity before allowing the decision to affect consensus.
