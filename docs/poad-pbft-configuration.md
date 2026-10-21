# PBFT PoAD validation configuration

## Runtime flags

The consensus node supports an opt-in, fail-closed PoAD policy:

- `-poad-required`: reject transaction proposals unless they contain valid PoAD metadata. The current strict policy supports exactly one transaction per non-migration block.
- `-poad-registry`: path to a trusted JSON snapshot of Agent public identities.
- `-poad-registry-sha256`: SHA-256 digest of the exact registry file bytes, provisioned independently of that file.

When `-poad-required` is set without both registry flags, startup fails. If a registry is supplied without strict mode, PoAD metadata is still verified when present, but legacy transaction blocks without PoAD metadata remain permitted. With no registry and no strict mode, the node logs that it is running in legacy/optional mode.

Example:

```sh
sha256sum poad-identities.json
go run ./cmd/consensusnode \
  -config config.yaml \
  -poad-required \
  -poad-registry ./poad-identities.json \
  -poad-registry-sha256 <trusted-64-character-sha256>
```

All validators in a shard must be provisioned with the same exact registry bytes, trusted digest, and `-poad-required` setting. Do not obtain the trusted digest from the same untrusted channel as the registry file. The registry file contains public keys only; never put Agent private keys in it.

Registry format:

```json
[
  {
    "agent_id": "agent-1",
    "public_key_hex": "64-hex-character-ed25519-public-key",
    "status": "ACTIVE",
    "key_version": 1
  }
]
```

`status` may be `ACTIVE` or `REVOKED`; it defaults to `ACTIVE`. `key_version` defaults to 1. Revoked identities are loaded but cannot validate new decisions.

## Important implementation boundary

This configuration installs the verifier before `Node.Start()`. The Leader now validates its own proposal before broadcasting, followers validate Pre-prepare proposals before they can progress to Prepare, and catch-up performs chain-structure preflight followed by sequential stateful validation and persistence.

The current block builder does **not** automatically create Agent decisions or attach PoAD metadata. Therefore strict mode is a fail-closed enforcement switch for deployments that already produce valid PoAD envelopes; enabling it on an unmodified legacy workload will reject transaction proposals until the decision-evidence production path is integrated. Do not describe this switch alone as a complete end-to-end Agent consensus implementation.

Catch-up block validation must remain sequential because each block's state transition is validated against the parent state established by the previous block. The code preflights height and parent-hash continuity for the entire response before writes. The underlying `Chain.AddBlock` still documents that it is not atomic, so a storage/trie failure during the subsequent sequential application can still leave a partially applied batch; fully atomic catch-up requires a storage-layer transaction/staging design.
