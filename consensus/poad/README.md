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


## Trace manifest integrity check (phase 3)

`LoadTraceIntentLinksVerified` verifies the SHA-256 digest of the exact JSONL bytes
before parsing the manifest. The expected digest must be obtained from an independently
trusted or committed source. Placing a digest next to an untrusted manifest is not
authentication, and this helper alone does not prove who created the trace.

The helper is still off-chain: no PBFT message or block validation path consumes this
digest yet. Before consensus integration, define how all validators agree on the trusted
digest and manifest version, and retain the ordinary transaction/state validation path.


## Signed decision records (phase 4)

`SignDecision` and `VerifyDecisionSignature` use Ed25519 to authenticate a decision's
serialized fields and detect post-signing modification. Verification also checks the
expected AgentID. The verifier's public key must be resolved from a trusted identity
registry; accepting a public key supplied only by the decision itself would not establish
the signer's identity.

A valid signature proves that the holder of the corresponding private key signed these
decision bytes. It does not prove that the transaction, state, intent, evidence, or hidden
reasoning was evaluated correctly. These helpers are not yet wired into PBFT messages or
block validation.


## Agent identity and public-key registry (phase 5)

`IdentityRegistry` binds each stable AgentID to an Ed25519 public key and tracks
ACTIVE/REVOKED status plus a monotonically increasing key version.

- `Register` rejects empty IDs, malformed keys, and duplicate IDs; it never silently overwrites an existing identity.
- `Resolve` returns only active identities and returns a defensive copy of the key.
- `Rotate` requires the currently registered private key to authorize the exact new key and current version. Successful rotation increments the version, making old rotation requests stale.
- `Revoke` disables future decision verification. In this prototype, registration and revocation are trusted governance/bootstrap operations.
- `IdentityRegistry.VerifyDecision` resolves the key from the registry and then verifies the signed decision; it does not accept a submitter-supplied public key.

This registry is an in-memory prototype, not yet persistent or replicated across validators.
Before consensus integration, define an authenticated, deterministic way to distribute registration,
rotation, and revocation updates (for example, committed governance transactions or a versioned
on-chain registry). Validators must agree on the same registry state at the same consensus height.
Private keys are never stored in the registry. A valid signature authenticates the decision bytes,
not the correctness of the decision or the truth of an Agent's reasoning.


## Cross-Agent Agreement (phase 6)

`AggregateSignedDecisions` verifies each Agent decision through the trusted identity registry,
requires all votes to reference the same transaction hash and policy version, rejects duplicate
AgentIDs, and applies an explicit vote threshold. It returns ACCEPT or REJECT only when the
corresponding threshold is reached; otherwise it returns ABSTAIN. If both thresholds are reached
under an improperly configured threshold, it abstains with `conflicting_quorums`.

The threshold is an input to this helper, not a claim that a particular committee size or
threshold tolerates Byzantine faults. A deployment must define authenticated committee membership,
a consistent registry snapshot, and a threshold justified by its fault model. This helper does
not replace PBFT, is not yet wired into proposal validation, and cannot establish the truth of
an Agent's reasoning.


## Reputation evolution (phase 7)

`ReputationLedger` is a deterministic in-memory prototype for updating Agent scores after
a transaction receives a finalized ACCEPT or REJECT outcome. A vote matching the finalized
outcome earns the configured reward, a conflicting ACCEPT/REJECT vote incurs the configured
penalty, and ABSTAIN is neutral. Scores are clamped to configured bounds, and the ledger
rejects a repeated update for the same AgentID, transaction hash, and policy version.

The scoring values are explicit configuration, not learned or experimentally validated
parameters. A mismatch with the finalized result is not, by itself, proof of maliciousness:
an honest minority can disagree, and a faulty or colluding quorum can finalize the wrong
outcome. Any production design should distinguish malicious behavior from good-faith
disagreement, define an auditable appeal/evidence policy, and evaluate reward/penalty
settings experimentally.

This ledger is local memory only. It is not persisted, replicated, or committed with blocks,
so different validators could diverge if they used it independently. Do not use it to select
validators or alter consensus weights until reputation updates are derived from the same
finalized block history and applied deterministically by all validators.


## Decision-aware block finalization gate (phase 7)

`PrepareBlockFinalization` re-verifies the signed Agent decisions through the identity
registry and the cross-Agent agreement helper. It creates a finalization record only when
an ACCEPT quorum is reached, and binds the record to the transaction hash, policy version,
vote counts, a canonicalized SHA-256 digest of the signed decision set, and a supplied state
root. The state root must come from the repository's ordinary deterministic execution and
state-transition validation path; this helper does not execute transactions or calculate
the root itself.

`VerifyBlockFinalization` recomputes the quorum and decision-set digest rather than trusting
the record's vote counters, and requires the caller to supply an independently validated
expected state root so the record cannot authenticate its own state root. This is still an
isolated PoAD module: it is not wired into PBFT
proposal/commit handling or the block data structure. Existing block, transaction, and
state-transition checks must remain mandatory. The in-memory identity registry also means
validators must first establish the same authenticated registry snapshot.
