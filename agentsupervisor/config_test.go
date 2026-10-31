package agentsupervisor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const testAgentEmuConfig = `base:
  blockemulator_config: ./config.yaml
  result_dir: ./results
experiment:
  trace: ./trace.jsonl
chain:
  enabled: true
  poad_registry_path: ./poad-identities.json
  poad_registry_sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
  poad_decision_service_config_path: ./local-poad/agents.private.json
  poad_required: true
protocols:
  pay:
    plugin: direct-pay
  identity:
    plugin: did-simple
`

func writeAgentEmuConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "agentEmuConfig.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func TestLoadConfigLoadsStrictPoADSettings(t *testing.T) {
	cfg, err := LoadConfig(writeAgentEmuConfig(t, testAgentEmuConfig))
	require.NoError(t, err)
	require.Equal(t, "./poad-identities.json", cfg.Chain.PoADRegistryPath)
	require.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", cfg.Chain.PoADRegistrySHA256)
	require.True(t, cfg.Chain.PoADRequired)
	require.Equal(t, "./local-poad/agents.private.json", cfg.Chain.PoADDecisionServiceConfigPath)
	require.Equal(t, 2, cfg.Chain.PoADRequiredVotes)
	require.Equal(t, 15, cfg.Chain.PoADDecisionTimeoutSeconds)
}

func TestLoadConfigRejectsRegistryWithoutDigest(t *testing.T) {
	content := `base:
  result_dir: ./results
experiment:
  trace: ./trace.jsonl
chain:
  poad_registry_path: ./poad-identities.json
protocols:
  pay:
    plugin: direct-pay
  identity:
    plugin: did-simple
`

	_, err := LoadConfig(writeAgentEmuConfig(t, content))
	require.ErrorContains(t, err, "must be configured together")
}

func TestLoadConfigRejectsStrictPoADWithoutChainExecution(t *testing.T) {
	content := `base:
  result_dir: ./results
experiment:
  trace: ./trace.jsonl
chain:
  enabled: false
  poad_registry_path: ./poad-identities.json
  poad_registry_sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
  poad_required: true
protocols:
  pay:
    plugin: direct-pay
  identity:
    plugin: did-simple
`

	_, err := LoadConfig(writeAgentEmuConfig(t, content))
	require.ErrorContains(t, err, "needs chain.enabled=true")
}

func TestLoadConfigRejectsStrictPoADWithoutLocalDecisionService(t *testing.T) {
	content := `base:
  blockemulator_config: ./config.yaml
  result_dir: ./results
experiment:
  trace: ./trace.jsonl
chain:
  enabled: true
  poad_registry_path: ./poad-identities.json
  poad_registry_sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
  poad_required: true
protocols:
  pay:
    plugin: direct-pay
  identity:
    plugin: did-simple
`
	_, err := LoadConfig(writeAgentEmuConfig(t, content))
	require.ErrorContains(t, err, "poad_decision_service_config_path")
}
