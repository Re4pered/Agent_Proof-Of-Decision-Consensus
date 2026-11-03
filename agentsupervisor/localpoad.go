package agentsupervisor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PrepareLocalPoADIdentities provisions demo keys only for explicitly opted-in
// local strict PoAD experiments. It never overwrites existing identity files.
// The local SHA-256 sidecar protects against accidental mismatches, but it is
// not an independently provisioned trust anchor.
func (cfg *Config) PrepareLocalPoADIdentities(ctx context.Context) error {
	if !cfg.Chain.PoADAutoInitLocalIdentities {
		return nil
	}

	registryPath := filepath.Clean(cfg.Chain.PoADRegistryPath)
	privatePath := filepath.Clean(cfg.Chain.PoADDecisionServiceConfigPath)
	identityDir := filepath.Dir(registryPath)

	if filepath.Base(registryPath) != "registry.public.json" ||
		filepath.Base(privatePath) != "agents.private.json" ||
		filepath.Dir(privatePath) != identityDir {
		return fmt.Errorf(
			"automatic local PoAD identities require registry.public.json and agents.private.json in one directory",
		)
	}

	digestPath := registryPath + ".sha256"
	paths := []string{registryPath, digestPath, privatePath}
	present := 0

	for _, path := range paths {
		_, err := os.Stat(path)
		if err == nil {
			present++

			continue
		}

		if !os.IsNotExist(err) {
			return fmt.Errorf("inspect local PoAD identity file %s: %w", path, err)
		}
	}

	if present != 0 && present != len(paths) {
		return fmt.Errorf(
			"local PoAD identity files are incomplete in %s; restore the missing files or back up and remove the set",
			identityDir,
		)
	}

	if present == 0 {
		cmd := exec.CommandContext(
			ctx, "go", "run", "./cmd/poad-decision-service",
			"-init-dir", identityDir, "-agents", "4",
		)
		cmd.Dir = cfg.Base.ModuleRoot

		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("initialize local PoAD demo identities: %w: %s", err, strings.TrimSpace(string(output)))
		}
	}

	registryBytes, err := os.ReadFile(registryPath)
	if err != nil {
		return fmt.Errorf("read local PoAD public registry: %w", err)
	}

	digestBytes, err := os.ReadFile(digestPath)
	if err != nil {
		return fmt.Errorf("read local PoAD registry digest: %w", err)
	}

	fields := strings.Fields(string(digestBytes))
	if len(fields) == 0 {
		return fmt.Errorf("local PoAD registry digest is empty")
	}

	sum := sha256.Sum256(registryBytes)
	actualDigest := hex.EncodeToString(sum[:])
	if !strings.EqualFold(fields[0], actualDigest) {
		return fmt.Errorf("local PoAD registry SHA-256 mismatch; refusing to start strict consensus")
	}

	cfg.Chain.PoADRegistrySHA256 = actualDigest

	return nil
}
