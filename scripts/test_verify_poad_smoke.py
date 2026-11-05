"""Regression tests for PoAD smoke evidence completeness."""

import csv
import json
import tempfile
import unittest
from pathlib import Path

from verify_poad_smoke import check


class SmokeValidatorTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        base = self.root / "round_001"
        logs = base / "chain" / "logs"
        logs.mkdir(parents=True)
        blocks = base / "chain" / "data" / "block_record"
        blocks.mkdir(parents=True)

        planned, intents, decisions = [], [], []
        for shard in range(4):
            folder = blocks / f"shard={shard}_node=0"
            folder.mkdir()
            with (folder / "block_record.csv").open("w", newline="") as fp:
                writer = csv.DictWriter(fp, fieldnames=["TxBodyLen", "StateRoot"])
                writer.writeheader()
                for n in range(20):
                    idx = shard * 20 + n
                    tx_hash = f"{idx + 1:064x}"
                    # Only 8 distinct roots per shard, 32 globally. Valid:
                    # roots are snapshots, not block-unique identifiers.
                    root = f"{shard * 8 + n % 8 + 1:064x}"
                    planned.append({"hash": tx_hash, "sender": f"0x{shard:040x}"})
                    intents.append({"action": "pay", "tx_hashes": [tx_hash]})
                    decisions.append(
                        "poad_decision_request_completed "
                        f"transaction_hash={tx_hash} state_root={root} "
                        "accept_count=4 reject_count=0"
                    )
                    writer.writerow({"TxBodyLen": 1, "StateRoot": root})

        (base / "agent_transactions.jsonl").write_text(
            "".join(json.dumps(row) + "\n" for row in planned), encoding="utf-8"
        )
        (base / "poad_payment_intents.jsonl").write_text(
            "".join(json.dumps(row) + "\n" for row in intents), encoding="utf-8"
        )
        (logs / "poad_decision_service.log").write_text(
            "\n".join(decisions) + "\n", encoding="utf-8"
        )
        (self.root / "rounds_summary.json").write_text(
            '{"round_num":1}', encoding="utf-8"
        )
        (self.root / "evaluation_metrics.json").write_text(
            "{}", encoding="utf-8"
        )

    def test_repeated_roots_do_not_fail_completed_80_payments(self):
        report = check(self.root)
        self.assertEqual(report["status"], "passed", report["errors"])
        self.assertEqual(report["distinct_committed_state_roots"], 32)
        self.assertEqual(
            report["planned_hashes_with_accepted_decision_and_committed_state_root"],
            80,
        )

    def test_missing_summary_is_reported_once(self):
        (self.root / "rounds_summary.json").unlink()
        report = check(self.root)
        self.assertEqual(report["status"], "failed")
        self.assertEqual(
            len([e for e in report["errors"] if "rounds_summary.json" in e]), 1
        )

    def test_wrong_shard_binding_fails(self):
        log = (self.root / "round_001" / "chain" / "logs"
               / "poad_decision_service.log")
        content = log.read_text(encoding="utf-8")
        # Tamper with the FIRST transaction's decision root: shard 0 tx
        # reports a state root from shard 1. Global-root matching would
        # incorrectly accept this; shard-aware matching must reject it.
        content = content.replace(
            "transaction_hash=" + f"{1:064x}" + " state_root=" + f"{1:064x}",
            "transaction_hash=" + f"{1:064x}" + " state_root=" + f"{9:064x}",
            1,
        )
        log.write_text(content, encoding="utf-8")
        report = check(self.root)
        self.assertEqual(report["status"], "failed")
        self.assertLess(
            report["planned_hashes_with_accepted_decision_and_committed_state_root"],
            80,
        )



if __name__ == "__main__":
    unittest.main()
