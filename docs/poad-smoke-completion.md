# 四分片严格 PoAD：80 笔支付完成性实验

此实验用于验证 **Agent 决策 → PBFT 提交 → Supervisor 正常退出 → 完整报告** 的闭环，不是 10,000 笔性能基准或跨分片测试。

## Windows 一键运行

从仓库根目录运行：

```powershell
.\run_poad_smoke.bat
```

**不要使用** `run_agentemu.bat agentEmuSmokeConfig.yaml`：旧的通用批处理会删除整个 `exp/`，并把评价结果目录写死为默认大规模实验目录。这个专用入口会先编译，保留 `exp/agentemu-results/`（10,000 笔实验），并将上一次 smoke 数据归档到 `diagnostics/poad_smoke_backup_<timestamp>/`。

专用入口保存控制台到 `diagnostics/poad_smoke_<timestamp>.log`，并将 AgentEmulator 退出码保存到 `diagnostics/poad_smoke_exit_<timestamp>.txt`。即使链运行失败，已有的回合输出也会进入诊断指标分析。

## 配置与信任范围

`agentEmuSmokeConfig.yaml` 不修改 `agentEmuConfig.yaml`。它继续使用仓库 `config.yaml` 中的 **4 个分片 × 每片 4 个节点**，启用 `poad_required: true`、两票门槛和本地签名服务自动初始化。

`traces/poad_smoke_80.jsonl` 精确包含 80 个 `pay` 行，没有 `join`、`leave` 或 `raw_tx`。8 个付款账户通过显式的 `experiment.bootstrap_agents` 在**本地 AgentSupervisor 注册表**预先建立，仅供此测试使用；该预注册**不代表链上 DID 注册证明**，也**不等于**本地决策服务的 4 个 PoAD 验证 Agent。每个分片有 20 笔同分片支付，没有跨分片中继，因此不会暗中跳过链上非支付意图或中继的验证要求。默认大规模配置不设置此选项，原有注册交易行为保持不变。

按固定种子 `20260903` 生成 DID，分片按照 `DefaultAccountLoc` 验证，Go 回归测试强制检查每片 20 笔并确保计划与意图清单匹配。首次运行如无 `local-poad/`，仍自动初始化 4 个本地演示签名身份；其私钥目录处于 Git 忽略列表。

> 本地签名服务复用同一进程内的固定策略，并未实现独立执行交易的智能 Agent。其演示签名和规则校验不能被当成自主 Agent 的完整安全性证明。

## 什么叫“成功完成”？

程序退出码为 0 **还不够**。脚本还运行 `scripts/analyze_agentemu_results.py` 与 `scripts/verify_poad_smoke.py`，要求：

1. `rounds_summary.json` 记录正常结束的一个实验轮次，`evaluation_metrics.json` 存在。
2. 交易计划和可信支付意图清单各包含 **80 条不同哈希**，集合完全一致，没有其他类型。
3. 四个分片 0 号节点的区块 CSV 中，各有 **20 笔**已提交单交易区块（合计 80）。
4. 80 个计划交易哈希均有 **至少两票 ACCEPT** 的决策完成记录，且决策中的状态根与已提交区块记录匹配。

结果保存在 `exp/poad-smoke-results/smoke_validation.json`。`status: passed` 才可判定本次 smoke 具备完整实验结果；若结果为 `failed`，里面的 `errors` 会指出缺失文件、分片、决策或交易匹配项。区块状态根匹配是日志/CSV 证据一致性核查，不是对全部 PoAD 签名的独立密码学复验。

可单独重新生成诊断报告：

```powershell
python .\scripts\analyze_agentemu_results.py .\exp\poad-smoke-results
python .\scripts\verify_poad_smoke.py .\exp\poad-smoke-results
Get-Content .\exp\poad-smoke-results\smoke_validation.json
```

如果超过 600 秒超时，保留整个 `exp/poad-smoke-results` 和新生成的 `diagnostics` 日志。**完成性停止条件（本次修复）**：严格 PoAD 的提案器不会产生没有可信交易绑定的空区块，因此默认 Supervisor 的“等待空区块”停止条件不适用。此实验独立启用 `chain.stop_on_all_committed: true`，并通过派生链配置传入 `supervisor.stop_on_all_committed: true`。在 `static_relay` 模式下，Supervisor 只对自己成功注入的、并在区块提交通知中观测到的**唯一交易哈希**计数；全部 80 个哈希确认后正常结束。该模式拒绝跨分片中继阶段数据，并限制只对 `pay` 动作生效。默认 10,000 笔实验仍保留原停止策略。

**状态根校验**：80 个区块不必有 80 个不同状态根。状态根可能重复，完成性校验要求每笔支付的 ACCEPT 决策根能匹配到**所属分片**的实际提交根，不要求状态根全局唯一。不同状态根数量只作为诊断信息保留。

**不要用增大 timeout 代替调查未提交的交易或错误日志**。只有 smoke 完成后，才逐步扩展至非支付意图、跨分片和大规模负载。
