package agentsupervisor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/HuangLab-SYSU/block-emulator-x/supervisor/txsource/tracesource"
)

const baseConfigFixture = `system:
  shard_num: 2
  node_num: 4
  consensus_type: "static_relay"
  log:
    log_dir: "./exp/"
consensus_node:
  blockchain:
    storage:
      block_storage_type: "bolt"
      bolt:
        file_path_dir: "./exp/boltdb/"
      eth_storage:
        level_file_path_dir: "./exp/trie_db/"
  block_record_dir: "./exp/block_record/"
supervisor:
  tx_number: 100000
  result_output_dir: "./exp/results/"
  tx_source:
    tx_source_type: "random_source"
    tx_source_file: ""
network:
  communication_mode: "direct"
`

func newTestChainRunner(t *testing.T) *ChainRunner {
	t.Helper()

	dir := t.TempDir()
	base := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(base, []byte(baseConfigFixture), 0o644))

	return &ChainRunner{ModuleRoot: dir, BaseConfig: base, WorkRoot: filepath.Join(dir, "results")}
}

func TestChainRunnerPrepareDerivesConfigAndIPTable(t *testing.T) {
	c := newTestChainRunner(t)

	chainDir := filepath.Join(c.WorkRoot, roundDirName(1), "chain")
	require.NoError(t, os.MkdirAll(chainDir, 0o755))

	planPath := filepath.Join(c.WorkRoot, roundDirName(1), PlanFileName)
	require.NoError(t, os.WriteFile(planPath, []byte("{}\n"), 0o644))

	cfgPath, tablePath, resultDir, err := c.prepare(chainDir, RoundSpec{Round: 1, PlanPath: planPath, TxCount: 7})
	require.NoError(t, err)

	raw, err := os.ReadFile(cfgPath)
	require.NoError(t, err)

	var derived map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &derived))

	require.Equal(t, tracesource.Key, derived["supervisor"].(map[string]any)["tx_source"].(map[string]any)["tx_source_type"])

	absPlan, err := filepath.Abs(planPath)
	require.NoError(t, err)
	require.Equal(t, absPlan, derived["supervisor"].(map[string]any)["tx_source"].(map[string]any)["tx_source_file"])

	// tx_number is pinned to the plan size so the supervisor can stop.
	require.EqualValues(t, 7, derived["supervisor"].(map[string]any)["tx_number"])

	// Mutable paths are redirected into the chain dir's logs/data/results
	// subdirectories; network settings are kept.
	require.Equal(t, filepath.Join(chainDir, "logs"), derived["system"].(map[string]any)["log"].(map[string]any)["log_dir"])
	require.Equal(t, filepath.Join(chainDir, "data", "boltdb"), derived["consensus_node"].(map[string]any)["blockchain"].(map[string]any)["storage"].(map[string]any)["bolt"].(map[string]any)["file_path_dir"])
	require.Equal(t, filepath.Join(chainDir, "data", "trie_db"), derived["consensus_node"].(map[string]any)["blockchain"].(map[string]any)["storage"].(map[string]any)["eth_storage"].(map[string]any)["level_file_path_dir"])
	require.Equal(t, filepath.Join(chainDir, "data", "block_record"), derived["consensus_node"].(map[string]any)["block_record_dir"])
	require.Equal(t, filepath.Join(chainDir, "results"), resultDir)
	require.Equal(t, resultDir, derived["supervisor"].(map[string]any)["result_output_dir"])
	require.Equal(t, "direct", derived["network"].(map[string]any)["communication_mode"])

	// IP table covers every shard/node plus the supervisor entry.
	tableRaw, err := os.ReadFile(tablePath)
	require.NoError(t, err)

	var table map[string]map[string]string
	require.NoError(t, json.Unmarshal(tableRaw, &table))

	require.Len(t, table, 3)
	require.Equal(t, "127.0.0.1:32217", table["0"]["0"])
	require.Equal(t, "127.0.0.1:32247", table["0"]["3"])
	require.Equal(t, "127.0.0.1:32347", table["1"]["3"])
	require.Equal(t, "127.0.0.1:38800", table["2147483647"]["0"])
}

func TestChainRunnerClusterShape(t *testing.T) {
	c := newTestChainRunner(t)

	shardNum, nodeNum, err := c.clusterShape()
	require.NoError(t, err)
	require.EqualValues(t, 2, shardNum)
	require.EqualValues(t, 4, nodeNum)
}

func TestChainRunnerStagesPoADPaymentManifest(t *testing.T) {
	c := newTestChainRunner(t)
	roundDir := filepath.Join(c.WorkRoot, roundDirName(1))
	require.NoError(t, os.MkdirAll(roundDir, 0o755))

	manifestDir := t.TempDir()
	require.NoError(t, writePoADPaymentManifest(manifestDir, []ActionTxLink{{
		Seq: 1, Action: ActionPay, AgentID: "alice", Target: "bob", Amount: 7,
		RequestID: "pay-7", TxHashes: []string{"deadbeef"},
		ExpectedSender: "0x1111", ExpectedRecipient: "0x2222",
	}}))
	manifestPath := filepath.Join(manifestDir, PoADPaymentManifestFileName)
	planPath := filepath.Join(roundDir, PlanFileName)
	require.NoError(t, os.WriteFile(planPath, []byte("{}\\n"), 0o644))

	chainDir := filepath.Join(roundDir, "chain")
	require.NoError(t, os.MkdirAll(chainDir, 0o755))
	_, _, _, err := c.prepare(chainDir, RoundSpec{
		Round: 1, PlanPath: planPath, IntentManifestPath: manifestPath, TxCount: 1,
	})
	require.NoError(t, err)

	staged, err := os.ReadFile(filepath.Join(chainDir, PoADPaymentManifestFileName))
	require.NoError(t, err)
	source, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	require.Equal(t, source, staged)
	require.FileExists(t, filepath.Join(chainDir, PoADPaymentManifestDigestFileName))
}

func TestChainRunnerNodeStartupArgsStrictPoAD(t *testing.T) {
	c := newTestChainRunner(t)
	c.PoADRequired = true
	c.PoADRegistryPath = "registry.public.json"
	c.PoADRegistrySHA256 = strings.Repeat("a", 64)
	c.PoADDecisionServiceConfigPath = "agents.private.json"

	chainDir := t.TempDir()
	manifestPath := filepath.Join(chainDir, PoADPaymentManifestFileName)
	require.NoError(t, os.WriteFile(manifestPath+".sha256", []byte(strings.Repeat("b", 64)+"\\n"), 0o600))

	args, err := c.nodeStartupArgs("config.yaml", "ip_table.json", chainDir)
	require.NoError(t, err)
	require.Contains(t, args, "-poad-required=true")
	require.Contains(t, args, "-poad-intent-manifest")
	require.Contains(t, args, manifestPath)
	require.Contains(t, args, "-poad-intent-manifest-sha256")
	require.Contains(t, args, "-poad-registry")
}

func TestChainRunnerNodeStartupArgsOptionalPoADOmitsManifest(t *testing.T) {
	c := newTestChainRunner(t)
	c.PoADRegistryPath = "registry.public.json"
	c.PoADRegistrySHA256 = strings.Repeat("a", 64)

	args, err := c.nodeStartupArgs("config.yaml", "ip_table.json", t.TempDir())
	require.NoError(t, err)
	require.Contains(t, args, "-poad-registry")
	require.NotContains(t, args, "-poad-required=true")
	require.NotContains(t, args, "-poad-intent-manifest")
}

func TestChainRunnerNodeStartupArgsRejectsIncompleteStrictPoAD(t *testing.T) {
	c := newTestChainRunner(t)
	c.PoADRequired = true

	_, err := c.nodeStartupArgs("config.yaml", "ip_table.json", t.TempDir())
	require.ErrorContains(t, err, "strict PoAD requires")
}

func TestChainRunnerSmokeStopModeIsExplicitOptIn(t *testing.T) {
	c := newTestChainRunner(t)
	c.StopOnAllCommitted = true
	chainDir := filepath.Join(c.WorkRoot, "round_001", "chain")
	require.NoError(t, os.MkdirAll(chainDir, 0o755))
	planPath := filepath.Join(c.WorkRoot, "round_001", PlanFileName)
	require.NoError(t, os.WriteFile(planPath, []byte("{}\n"), 0o644))

	cfgPath, _, _, err := c.prepare(chainDir, RoundSpec{Round: 1, PlanPath: planPath, TxCount: 80})
	require.NoError(t, err)
	raw, err := os.ReadFile(cfgPath)
	require.NoError(t, err)

	var derived map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &derived))
	spv := derived["supervisor"].(map[string]any)
	require.Equal(t, true, spv["stop_on_all_committed"])
	require.EqualValues(t, 80, spv["tx_number"])
}
