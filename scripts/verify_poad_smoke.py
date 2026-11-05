#!/usr/bin/env python3
"""Check a completed 80-payment, four-shard strict PoAD smoke run.

This checks local experimental evidence consistency; it is not an independent
cryptographic proof of Agent decisions or a general performance benchmark.
"""

import argparse
import csv
import json
import re
from collections import defaultdict
from pathlib import Path

EXPECTED_PAYMENTS = 80
EXPECTED_PER_SHARD = 20


def read_jsonl(path):
    return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line.strip()]


def check(root):
    round_dir = root / "round_001"
    chain_dir = round_dir / "chain"
    errors = []
    report = {"status": "failed", "expected_payments": EXPECTED_PAYMENTS,
              "expected_per_shard": EXPECTED_PER_SHARD}

    def need(path):
        if not path.is_file():
            errors.append(f"missing: {path}")
            return False
        return True

    summary = root / "rounds_summary.json"
    metrics = root / "evaluation_metrics.json"
    has_summary = need(summary)
    need(metrics)

    plan_path = round_dir / "agent_transactions.jsonl"
    manifest_path = round_dir / "poad_payment_intents.jsonl"
    if not all(need(p) for p in [plan_path, manifest_path]):
        report["errors"] = errors
        return report

    plan = read_jsonl(plan_path)
    manifest = read_jsonl(manifest_path)
    plan_hashes = {r["hash"].lower() for r in plan}
    # The receiver's partition is not used: smoke traffic is all intra-shard.
    # Match decision roots against the sender's ACTUAL committed shard, not
    # merely any root anywhere in the cluster.
    hash_shards = {
        row["hash"].lower(): int(row["sender"][-8:], 16) % 4
        for row in plan
    }
    intent_hashes = {h.lower() for row in manifest for h in row.get("tx_hashes", [])}
    report["planned_transactions"] = len(plan)
    report["manifest_payments"] = len(manifest)

    if len(plan) != EXPECTED_PAYMENTS or len(plan_hashes) != EXPECTED_PAYMENTS:
        errors.append("expected 80 unique planned payment transactions")
    if len(manifest) != EXPECTED_PAYMENTS or len(intent_hashes) != EXPECTED_PAYMENTS:
        errors.append("expected 80 unique payment intent links")
    if plan_hashes != intent_hashes:
        errors.append("plan transaction hashes and payment intent hashes differ")
    if any(row.get("action") != "pay" for row in manifest):
        errors.append("non-payment action in strict smoke manifest")

    decisions_log = chain_dir / "logs" / "poad_decision_service.log"
    decisions = defaultdict(set)
    if need(decisions_log):
        for line in decisions_log.read_text(encoding="utf-8", errors="replace").splitlines():
            if "poad_decision_request_completed" not in line:
                continue
            tx = re.search(r"\btransaction_hash=([0-9a-fA-F]{64})", line)
            state_root = re.search(r"\bstate_root=([0-9a-fA-F]{64})", line)
            accepts = re.search(r"\baccept_count=(\d+)", line)
            rejects = re.search(r"\breject_count=(\d+)", line)
            if (tx and state_root and accepts and rejects
                    and int(accepts.group(1)) >= 2 and int(rejects.group(1)) == 0):
                decisions[tx.group(1).lower()].add(state_root.group(1).lower())

    missing_decisions = plan_hashes - decisions.keys()
    report["accepted_payment_hashes"] = len(set(decisions) & plan_hashes)
    if missing_decisions:
        errors.append(f"missing ACCEPT decisions for {len(missing_decisions)} planned transactions")

    block_roots = set()
    roots_by_shard = defaultdict(set)
    per_shard = {}
    for shard in range(4):
        csv_path = (chain_dir / "data" / "block_record" /
                    f"shard={shard}_node=0" / "block_record.csv")
        count = 0
        if need(csv_path):
            with csv_path.open(encoding="utf-8-sig", newline="") as f:
                for row in csv.DictReader(f):
                    tx_count = int(row["TxBodyLen"])
                    if tx_count > 0:
                        count += tx_count
                        if tx_count != 1:
                            errors.append(f"shard {shard} contains non-single-payment block")
                        root = row["StateRoot"].lower()
                        block_roots.add(root)
                        roots_by_shard[shard].add(root)
        per_shard[str(shard)] = count
        if count != EXPECTED_PER_SHARD:
            errors.append(f"shard {shard}: expected 20 committed payments, got {count}")

    report["committed_payment_counts_by_shard"] = per_shard
    # Identical state roots are valid: a root is a state snapshot, not a
    # per-block unique identifier. Keep the count for diagnosis only.
    report["distinct_committed_state_roots"] = len(block_roots)

    matching_hashes = {
        tx for tx, roots in decisions.items()
        if tx in hash_shards and roots & roots_by_shard[hash_shards[tx]]
    }
    report["planned_hashes_with_accepted_decision_and_committed_state_root"] = len(matching_hashes)
    if len(matching_hashes) != EXPECTED_PAYMENTS:
        errors.append(f"only {len(matching_hashes)} / 80 planned payments have accepted decisions bound to committed roots")

    if has_summary:
        payload = json.loads(summary.read_text(encoding="utf-8"))
        if payload.get("round_num") != 1:
            errors.append("rounds_summary.json does not confirm one finished round")

    report["errors"] = errors
    report["status"] = "passed" if not errors else "failed"
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("result_dir", nargs="?", default="exp/poad-smoke-results")
    args = parser.parse_args()
    root = Path(args.result_dir)
    report = check(root)
    root.mkdir(parents=True, exist_ok=True)
    dest = root / "smoke_validation.json"
    dest.write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(f"PoAD smoke check: {report['status']} ({dest})")
    for issue in report["errors"]:
        print(f"  - {issue}")
    return 0 if report["status"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
