# 本地 PoAD 决策服务（实验原型）

该命令提供与 PBFT `-poad-agent-supervisor-endpoint` 兼容的本地 HTTP 服务。它从受信任的 payment-intent manifest 读取意图，将每笔请求与清单中的交易哈希关联，再使用本地配置的 Ed25519 Agent 私钥签署确定性 PoAD 决策。

> **实验边界：** 这是本地原型，不是生产级分布式 Agent 网络，也不包含 LLM 推理。当前配置中的多个 Agent 身份由同一个本地进程托管，签名并不代表独立的 Agent 运行环境。服务使用调用方传入的已执行状态根进行绑定，不会自行执行交易或独立重算状态根。普通交易执行和状态转换校验仍必须由区块链节点完成。

## 1. 生成本地测试身份

在仓库根目录执行：

```sh
go run ./cmd/poad-decision-service -init-dir ./local-poad -agents 4
```

将生成：

- `local-poad/agents.private.json`：本地私钥，权限限制为当前用户可读写。**不要提交到 Git，也不要分享。**
- `local-poad/registry.public.json`：可提供给各个共识节点的公钥注册表。
- `local-poad/registry.public.json.sha256`：注册表文件的 SHA-256 摘要。

初始化命令拒绝覆盖已有文件。如果需要重建，请先自行备份并移走旧目录。

## 2. 配置 AgentEmulator

编辑 `agentEmuConfig.yaml` 的 `chain` 部分，填写初始化命令生成的摘要（64 位十六进制字符串）：

```yaml
chain:
  enabled: true
  poad_registry_path: ./local-poad/registry.public.json
  poad_registry_sha256: <复制初始化命令输出的 SHA-256>
  poad_decision_service_config_path: ./local-poad/agents.private.json
  poad_required: true
  poad_required_votes: 2
  poad_decision_timeout_seconds: 15
```

不要把尖括号占位符原样写进配置。启用 `poad_required: true` 后，AgentEmulator 会在每轮运行时：

1. 生成交易计划和 payment-intent manifest；
2. 将 manifest 及其摘要暂存到该轮独立的 chain 目录；
3. 启动仅绑定 `127.0.0.1:38881` 的本地决策服务；
4. 将决策 endpoint、超时和所需 ACCEPT 票数传给各个共识节点；
5. 在整轮结束时停止本地服务。

服务只接受 loopback 监听地址；不要通过端口转发或代理把它暴露给其他主机。服务私钥配置路径必须指向本机文件。

## 3. 运行实验

```sh
go run ./cmd/agentemu -config agentEmuConfig.yaml
```

如果使用已经构建好的 AgentEmulator 二进制，也可以按仓库原有的 `run_agentemu.sh` 或 `run_agentemu.bat` 启动。首次运行前先确认本地私钥文件和公钥注册表都已生成，并且配置摘要匹配注册表的精确文件字节。

决策 endpoint 提供：

- `GET /healthz`：服务就绪状态、已加载 Agent 数量和意图链接数量。
- `POST /decisions`：返回 `{"decisions":[...]}` 格式的签名决策，供现有 PoAD provider 消费。

每次决策都会通过结构化日志记录交易哈希、请求 ID、Agent ID、决策结果、理由、策略版本和验证耗时，供后续实验指标采集使用。

## 4. 决策规则和解释

- 交易哈希必须能匹配可信 manifest 中的 intent link；请求携带的 intent 必须与该链接完全一致，否则返回 HTTP 422。
- `VerifyPaymentIntent` 检查支付动作的交易类型、发送方、接收方、金额和交易哈希关联。
- 服务将普通交易执行和状态转换验证视为节点已经完成的前置步骤；只把请求中的非空 state root 绑定到签名决策。这不是服务独立验证状态正确性的证明。
- 当前支付策略把外部 evidence 标记为 `NOT_REQUIRED`；这不代表已经实现外部证据验证。
- Agent 私钥只用于签名决策，节点仍需通过可信公钥注册表校验签名、交易哈希、状态根和 ACCEPT quorum。

若出现意图清单不匹配、服务启动失败、私钥与注册表公钥不匹配或签名验证失败，严格模式应中止对应提案；不要为了让实验通过而关闭 fail-closed 检查。

## 5. 实验结果解释

该服务支持先验证“决策请求—签名—PoAD 提案构建—PBFT 校验”的端到端软件路径，但由于多个 Agent 密钥由单一服务托管，**不能将它当作独立智能体之间的真实拜占庭容错决策实验**。论文中应明确标注这是本地功能集成/机制原型。后续要研究独立 Agent 节点、独立证据产生、超时与不可用处理，并对传统 PBFT 与 PoAD 使用相同交易负载做对照实验。
