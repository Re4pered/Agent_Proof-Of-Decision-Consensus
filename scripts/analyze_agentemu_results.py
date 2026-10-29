#!/usr/bin/env python3
"""Generate thesis-oriented metrics from AgentEmulator experiment outputs.

Missing evidence is reported as null/not_measured; this script never treats
planned transactions or observed hash counts as successful commits by default.
"""
from __future__ import annotations
import argparse, csv, json, re
from pathlib import Path


def read_json(path):
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return {}


def rows(path):
    if not path.exists():
        return []
    with path.open(newline="", encoding="utf-8-sig") as f:
        return list(csv.DictReader(f))


def count_lines(path):
    if not path.exists():
        return 0
    with path.open(encoding="utf-8", errors="replace") as f:
        return sum(bool(line.strip()) for line in f)


def num(value):
    try:
        x = float(value)
        return x if x == x and abs(x) != float("inf") else None
    except (TypeError, ValueError):
        return None


def mean(values):
    xs = [x for x in values if x is not None]
    return sum(xs) / len(xs) if xs else None


def analyze(round_dir):
    chain = round_dir / "chain"
    result_dir = chain / "results"
    cfg = {}
    try:
        import yaml
        cfg = yaml.safe_load((chain / "config.yaml").read_text(encoding="utf-8")) or {}
    except Exception:
        pass
    log_files = list((chain / "logs").rglob("*.log")) if (chain / "logs").exists() else []
    log = "\n".join(p.read_text(encoding="utf-8", errors="ignore") for p in log_files).lower()
    optional = "poad proposal validation is optional" in log
    no_registry = "no identity registry is configured" in log
    required = cfg.get("poad_required")
    if required is None and optional:
        required = False
    strict = required is True and not optional and not no_registry

    planned = count_lines(round_dir / "agent_transactions.jsonl")
    intents = count_lines(round_dir / "poad_payment_intents.jsonl")
    detail = rows(result_dir / "relay_stats_detail_tx_info.csv")
    brief = rows(result_dir / "relay_stats_brief_info.csv")
    hashes = {r.get("OriginalHash", "").strip() for r in detail if r.get("OriginalHash", "").strip()}
    tps = mean([num(r.get("Avg. TPS of this epoch (txs per second)")) for r in brief])
    tcl_ns = mean([num(r.get("Avg. TCL of this epoch (nanosecond)")) for r in brief])
    inner_ns = mean([num(r.get("Avg. inner-shard TCL of this epoch (nanosecond)")) for r in brief])
    ctx = mean([num(r.get("CTX ratio of this epoch")) for r in brief])
    total_reported = sum(num(r.get("Total tx # in this epoch")) or 0 for r in brief) if brief else None

    block_files = sorted((chain / "data" / "block_record").glob("shard=*/block_record.csv"))
    nonempty = sum(bool(rows(p)) for p in block_files)
    mismatch = len(re.findall(r"consensus message shard mismatch", log))
    preprepare = len(re.findall(r"handle preprepare message failed", log))
    healthy = not mismatch and (not block_files or nonempty == len(block_files))
    return {
        "round": round_dir.name,
        "performance": {
            "average_tps": tps,
            "average_commit_latency_ms": tcl_ns / 1_000_000 if tcl_ns is not None else None,
            "average_inner_shard_commit_latency_ms": inner_ns / 1_000_000 if inner_ns is not None else None,
            "cross_shard_transaction_ratio": ctx,
            "relay_reported_transaction_count": total_reported,
            "planned_transaction_records": planned,
            "intent_manifest_records": intents,
            "relay_detail_rows": len(detail),
            "unique_hashes_in_relay_detail": len(hashes),
            "hash_coverage_of_plan": len(hashes) / planned if planned else None,
            "coverage_warning": "Hash coverage is not a success rate unless plan hashes and relay hashes are verified as the same population."
        },
        "poad": {
            "strict_poad_enabled": strict,
            "effective_poad_required_config": required,
            "optional_validation_log_seen": optional,
            "identity_registry_missing_log_seen": no_registry,
            "intent_violation_rejection_rate": {"value": None, "status": "not_measured", "reason": "Requires labelled intent-violation cases and decision-level rejection records."},
            "invalid_or_missing_decision_evidence_rejection_rate": {"value": None, "status": "not_measured", "reason": "Current run does not provide decision-evidence rejection counters."},
            "valid_transaction_acceptance_rate": {"value": None, "status": "not_measured", "reason": "Planned transactions cannot yet be reliably joined to all committed/rejected outcomes."},
            "decision_verification_latency_ms": {"value": None, "status": "not_measured", "reason": "Per-decision verification timing is not recorded."}
        },
        "consensus_health": {
            "block_record_node_files": len(block_files),
            "nodes_with_block_records": nonempty,
            "block_record_rows": sum(len(rows(p)) for p in block_files),
            "node_block_records_complete": bool(block_files) and nonempty == len(block_files),
            "shard_mismatch_log_occurrences": mismatch,
            "preprepare_failure_log_occurrences": preprepare,
            "cross_node_block_hash_state_root_consistency": {"value": None, "status": "not_verified", "reason": "Cannot verify without complete per-node block records and comparable hashes/state roots."},
            "status": "ok" if healthy else "warning",
            "log_count_note": "Counts log occurrences, not deduplicated incidents; duplicated log files may inflate counts."
        }
    }


def flatten(value, prefix=""):
    out = []
    if isinstance(value, dict):
        for key, child in value.items():
            out.extend(flatten(child, f"{prefix}.{key}" if prefix else key))
    elif isinstance(value, list):
        out.append({"metric": prefix, "value": json.dumps(value, ensure_ascii=False), "status": "observed"})
    else:
        status = "not_measured" if value is None else "observed"
        out.append({"metric": prefix, "value": "" if value is None else str(value), "status": status})
    return out


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("result_dir", nargs="?", default="exp/agentemu-results")
    args = parser.parse_args()
    root = Path(args.result_dir)
    rounds = sorted(p for p in root.glob("round_*") if p.is_dir())
    if not rounds:
        raise SystemExit(f"No round_* directories found under {root}")
    reports = [analyze(p) for p in rounds]
    payload = {
        "report_version": 1,
        "source_result_dir": str(root),
        "round_count": len(reports),
        "interpretation": "Raw experiment files are unchanged. Null/not_measured means current outputs do not contain enough evidence.",
        "rounds": reports
    }
    (root / "evaluation_metrics.json").write_text(json.dumps(payload, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    flat = []
    for report in reports:
        flat.extend(flatten(report, report["round"]))
    with (root / "evaluation_metrics.csv").open("w", newline="", encoding="utf-8-sig") as f:
        writer = csv.DictWriter(f, fieldnames=["metric", "value", "status"])
        writer.writeheader()
        writer.writerows(flat)

    lines = [
        "# AgentEmulator 实验评估报告", "",
        "> 自动生成。本报告不把计划交易数或哈希覆盖数直接当作成功上链数；缺少证据的指标会明确标记为未测得。", ""
    ]
    for r in reports:
        p, a, c = r["performance"], r["poad"], r["consensus_health"]
        def fmt(v, digits=3):
            return "未测得" if v is None else f"{v:.{digits}f}"
        lines += [
            f"## {r['round']}", "", "### 性能指标", "",
            "| 指标 | 结果 |", "|---|---:|",
            f"| 平均 TPS | {fmt(p['average_tps'])} |",
            f"| 平均提交延迟（ms） | {fmt(p['average_commit_latency_ms'])} |",
            f"| 平均同分片提交延迟（ms） | {fmt(p['average_inner_shard_commit_latency_ms'])} |",
            f"| 跨分片交易比例 | {fmt(p['cross_shard_transaction_ratio'])} |",
            f"| 计划交易记录数（非成功数） | {p['planned_transaction_records']} |",
            f"| 意图清单记录数 | {p['intent_manifest_records']} |",
            f"| Relay 明细行数 | {p['relay_detail_rows']} |",
            f"| Relay 明细唯一交易哈希数 | {p['unique_hashes_in_relay_detail']} |",
            f"| 哈希数/计划数覆盖比（非成功率） | {fmt(p['hash_coverage_of_plan'])} |", "",
            "### PoAD 功能与安全指标", "",
            "| 指标 | 结果 |", "|---|---|",
            f"| 严格 PoAD 是否启用 | {'是' if a['strict_poad_enabled'] else '否'} |",
            f"| 意图违规拦截率 | 未测得：{a['intent_violation_rejection_rate']['reason']} |",
            f"| 无效/缺失决策证据拒绝率 | 未测得：{a['invalid_or_missing_decision_evidence_rejection_rate']['reason']} |",
            f"| 有效交易接受率 | 未测得：{a['valid_transaction_acceptance_rate']['reason']} |",
            f"| Agent 决策验证延迟 | 未测得：{a['decision_verification_latency_ms']['reason']} |", "",
            "### 共识运行健康", "",
            "| 指标 | 结果 |", "|---|---|",
            f"| 有区块记录的节点 | {c['nodes_with_block_records']} / {c['block_record_node_files']} |",
            f"| 区块记录行数 | {c['block_record_rows']} |",
            f"| 分片消息不匹配日志出现次数 | {c['shard_mismatch_log_occurrences']} |",
            f"| PrePrepare 失败日志出现次数 | {c['preprepare_failure_log_occurrences']} |",
            f"| 节点区块记录完整 | {'是' if c['node_block_records_complete'] else '否'} |",
            f"| 跨节点区块哈希/状态根一致性 | 未验证：{c['cross_node_block_hash_state_root_consistency']['reason']} |", "",
            "### 解释", "",
            ("本轮存在分片消息错误或节点区块记录不完整，性能数据仅作诊断参考。" if c['status'] == 'warning' else "本轮未发现该脚本定义的运行健康告警；仍需检查指标覆盖完整性。"),
            ("本轮未强制启用 PoAD，不能据此证明 PoAD 意图拦截效果。" if not a['strict_poad_enabled'] else "本轮配置为严格 PoAD；安全指标仍需通过专门的拒绝路径测试验证。"),
            ""
        ]
    (root / "evaluation_report.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(f"Evaluation metrics written under {root}: evaluation_report.md, evaluation_metrics.csv, evaluation_metrics.json")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
