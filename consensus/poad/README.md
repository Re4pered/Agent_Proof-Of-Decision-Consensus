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
