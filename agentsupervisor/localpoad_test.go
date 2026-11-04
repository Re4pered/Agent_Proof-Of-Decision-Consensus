package agentsupervisor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func newAutoLocalPoADConfig(t *testing.T) Config {
	t.Helper()

	dir := t.TempDir()
	var cfg Config
	cfg.Base.ModuleRoot = ".."
	cfg.Chain.Enabled = true
	cfg.Chain.PoADRequired = true
	cfg.Chain.PoADAutoInitLocalIdentities = true
	cfg.Chain.PoADRegistryPath = filepath.Join(dir, "registry.public.json")
	cfg.Chain.PoADDecisionServiceConfigPath = filepath.Join(dir, "agents.private.json")

	return cfg
}

func TestPrepareLocalPoADIdentitiesCreatesThenReusesDemoKeys(t *testing.T) {
	cfg := newAutoLocalPoADConfig(t)

	require.NoError(t, cfg.PrepareLocalPoADIdentities(context.Background()))
	require.FileExists(t, cfg.Chain.PoADRegistryPath)
	require.FileExists(t, cfg.Chain.PoADRegistryPath+".sha256")
	require.FileExists(t, cfg.Chain.PoADDecisionServiceConfigPath)

	registry, err := os.ReadFile(cfg.Chain.PoADRegistryPath)
	require.NoError(t, err)

	privateBefore, err := os.ReadFile(cfg.Chain.PoADDecisionServiceConfigPath)
	require.NoError(t, err)

	sum := sha256.Sum256(registry)
	require.Equal(t, hex.EncodeToString(sum[:]), cfg.Chain.PoADRegistrySHA256)

	cfg.Chain.PoADRegistrySHA256 = ""
	require.NoError(t, cfg.PrepareLocalPoADIdentities(context.Background()))

	privateAfter, err := os.ReadFile(cfg.Chain.PoADDecisionServiceConfigPath)
	require.NoError(t, err)
	require.Equal(t, privateBefore, privateAfter)
	require.Equal(t, hex.EncodeToString(sum[:]), cfg.Chain.PoADRegistrySHA256)
}

func TestPrepareLocalPoADIdentitiesRejectsPartialFiles(t *testing.T) {
	cfg := newAutoLocalPoADConfig(t)
	require.NoError(t, os.WriteFile(cfg.Chain.PoADRegistryPath, []byte("[]\n"), 0o644))

	err := cfg.PrepareLocalPoADIdentities(context.Background())
	require.ErrorContains(t, err, "identity files are incomplete")
	require.Empty(t, cfg.Chain.PoADRegistrySHA256)
}

func TestPrepareLocalPoADIdentitiesRejectsDigestMismatch(t *testing.T) {
	cfg := newAutoLocalPoADConfig(t)
	require.NoError(t, os.WriteFile(cfg.Chain.PoADRegistryPath, []byte("[]\n"), 0o644))
	require.NoError(t, os.WriteFile(cfg.Chain.PoADRegistryPath+".sha256",
		[]byte(strings.Repeat("0", 64)+"  registry.public.json\n"), 0o644))
	require.NoError(t, os.WriteFile(cfg.Chain.PoADDecisionServiceConfigPath, []byte("{}\n"), 0o600))

	err := cfg.PrepareLocalPoADIdentities(context.Background())
	require.ErrorContains(t, err, "SHA-256 mismatch")
	require.Empty(t, cfg.Chain.PoADRegistrySHA256)
}
