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


The batch API `ApplyFinalizedDecisions` now verifies the signed decision set and finalization
record (including an independently supplied expected state root) before applying any score
changes. It checks for duplicate/replayed events before mutating the ledger, so a rejected
batch cannot leave a partial update. This strengthens the local prototype's update boundary;
it does not make the ledger persistent or replicated, and the configured minority-vote penalty
remains an experimental policy rather than evidence of malicious intent.


## Block-bound PoAD evidence (phase 8)

The block header now has an optional PoADRoot, and the serialized block carries PoADData.
AttachBlockPoADMetadata verifies the signed decision quorum against the block's state root,
canonically orders the decision records, serializes the finalization envelope, and places its
SHA-256 commitment in the header. Because the header is hashed by the existing block-header
hash function, changing PoADRoot changes the block hash. VerifyBlockPoADMetadata checks the
commitment, canonical encoding, registered signatures, quorum, and state-root binding.

This is a data-model and verification-helper step, not yet runtime PBFT enforcement. The
current static/dynamic proposal validators still call the ordinary chain block validator and
do not invoke VerifyBlockPoADMetadata. Before making PoAD mandatory in PBFT, the network needs
an authenticated, replicated Agent identity registry and a configuration/policy for which
transactions require PoAD. Blocks without PoAD metadata remain accepted by this helper for
backward compatibility; an enforcement layer must explicitly reject missing metadata for
transactions governed by a PoAD policy. Ordinary transaction and state-transition validation
must remain mandatory.


## PBFT proposal-validation hook (phase 9)

Static and dynamic shard proposal validators now retain their ordinary chain.ValidateBlock
check and then call the shared PoAD metadata gate. The gate rejects malformed or partial
PoAD commitments, rejects a PoAD-bearing proposal when no verifier is configured, and can be
configured to require PoAD metadata for transaction blocks. Dynamic-shard migration blocks
are exempt from the transaction-only requirement, but any PoAD metadata they carry is still
verified.

After constructing a PBFT node, the caller can create a poad.BlockMetadataValidator backed by
its trusted IdentityRegistry and pass it to Node.ConfigurePoADValidation before Start. Every
validator must load the same authenticated registry snapshot and use the same required/optional
policy. This is opt-in: unless configured, legacy transaction blocks without PoAD metadata
remain accepted. The current block builder does not yet generate Agent decisions automatically,
and the JSONL intent manifest is not yet fetched or authenticated by PBFT validators. Therefore
this hook is the first proposal-validation integration seam, not a complete production deployment.


PBFT catch-up now runs the same proposal validator on every received historical proposal
before calling AddBlock. This closes a validation bypass where catch-up previously persisted
blocks directly without applying transaction/state checks or the configured PoAD gate. It still
assumes the node is configured with the correct trusted identity registry and PoAD policy before
starting consensus.


Transaction binding limitation: the current finalization envelope covers one PoAD transaction hash.
When strict PoAD mode is enabled, PBFT therefore rejects transaction blocks containing anything
other than exactly one transaction; this prevents a block from using one accepted decision to
implicitly cover unrelated transactions. Optional mode can still accept legacy or multi-transaction
blocks without PoAD, so it should not be described as enforcing intent checks on every transaction.
Supporting multiple PoAD-governed transactions per block requires a per-transaction finalization list.


### Remaining state-root validation limitation

The PBFT adapter currently binds the PoAD finalization record to the state root declared in the
block header. It does not independently recompute that state root. In this repository, the current
chain.ValidateBlock implementation checks the transaction root, bloom filter, and migration root,
but does not independently recompute and compare the resulting state root. Consequently, this
integration step authenticates and commits the Agent decision envelope, but it does not yet prove
that the declared state root is the deterministic result of executing the block. This base-chain
validation gap must be addressed before treating the integrated path as production-secure.


## Repeatable microbenchmarks (phase 10)

The package includes Go benchmarks for signed decision aggregation and finalization-record
verification across committee sizes 4, 7, 10, 13, 25, and 50. Key generation and registry
setup are excluded from the timed section so the measurements focus on verification and
aggregation overhead.

Run them from the repository root:

```sh
go test ./consensus/poad -run '^$' -bench 'Benchmark(AggregateSignedDecisions|VerifyBlockFinalization)$' -benchmem
```

These are microbenchmarks of local cryptographic verification and aggregation helpers, not
end-to-end blockchain TPS or PBFT network latency. They do not include LLM inference, network
round trips, transaction execution, or independently recomputing the block state root. The
committee threshold in the benchmark is a configurable (2f+1)-style threshold derived from
the chosen committee size; production fault tolerance still requires an explicitly defined
committee and Byzantine fault model. Record Go version, CPU, operating system, and repeated
runs when reporting results in the thesis.
