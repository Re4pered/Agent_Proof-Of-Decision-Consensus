// Command poad-decision-service runs a localhost-only experimental PoAD decision
// endpoint. Private keys are loaded from a local config file and must never be
// committed to a repository or exposed to untrusted callers.
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/HuangLab-SYSU/block-emulator-x/consensus/poad"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/transaction"
)

const (
	defaultListenAddress = "127.0.0.1:38881"
	maxRequestBytes      = 1 << 20
)

type agentKeyConfig struct {
	AgentID       string `json:"agent_id"`
	PrivateKeyHex string `json:"private_key_hex"`
}

type serviceConfig struct {
	Agents []agentKeyConfig `json:"agents"`
}

type decisionRequest struct {
	Transaction transaction.Transaction `json:"transaction"`
	Intent      poad.TraceIntentLink    `json:"intent"`
	StateRoot   string                  `json:"state_root"`
}

type decisionResponse struct {
	Decisions []poad.SignedDecision `json:"decisions"`
}

type decisionService struct {
	agents map[string]ed25519.PrivateKey
	links  []poad.TraceIntentLink
}

func main() {
	configPath := flag.String("config", "", "path to local JSON config containing Agent private keys")
	manifestPath := flag.String("manifest", "", "path to the trusted AgentEmulator payment-intent JSONL manifest")
	manifestSHA := flag.String("manifest-sha256", "", "trusted SHA-256 digest of the exact manifest bytes")
	listenAddress := flag.String("listen", defaultListenAddress, "HTTP listen address (keep bound to localhost)")
	initDir := flag.String("init-dir", "", "create local example keys and public registry in this directory, then exit")
	agentCount := flag.Int("agents", 4, "number of local demo identities created by -init-dir")

	flag.Parse()

	if *initDir != "" {
		if err := initializeLocalFiles(*initDir, *agentCount); err != nil {
			slog.Error("initialize local PoAD files", "error", err)
			os.Exit(1)
		}

		return
	}

	if *configPath == "" || *manifestPath == "" || *manifestSHA == "" {
		slog.Error("config, manifest, and manifest-sha256 are required")
		os.Exit(2)
	}

	if !isLoopbackAddress(*listenAddress) {
		slog.Error("local experimental service must bind to a loopback address", "listen", *listenAddress)
		os.Exit(2)
	}

	cfg, err := loadServiceConfig(*configPath)
	if err != nil {
		slog.Error("load local PoAD config", "error", err)
		os.Exit(1)
	}

	manifestFile, err := os.Open(*manifestPath)
	if err != nil {
		slog.Error("open trusted intent manifest", "error", err)
		os.Exit(1)
	}
	links, loadErr := poad.LoadTraceIntentLinksVerified(manifestFile, *manifestSHA)
	closeErr := manifestFile.Close()

	if loadErr != nil {
		slog.Error("load trusted intent manifest", "error", loadErr)
		os.Exit(1)
	}

	if closeErr != nil {
		slog.Error("close trusted intent manifest", "error", closeErr)
		os.Exit(1)
	}

	svc := &decisionService{agents: cfg, links: links}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "agents": len(svc.agents), "intent_links": len(svc.links)})
	})
	mux.HandleFunc("POST /decisions", svc.handleDecisions)

	server := &http.Server{
		Addr:              *listenAddress,
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	listener, err := net.Listen("tcp", *listenAddress)
	if err != nil {
		slog.Error("listen for local PoAD decisions", "error", err)
		os.Exit(1)
	}
	slog.Info("local experimental PoAD decision service ready",
		"listen", *listenAddress,
		"agents", len(svc.agents),
		"intent_links", len(svc.links),
		"policy", poad.DefaultPolicyVersion,
		"warning", "state is bound to the caller-supplied executed state root; this service does not independently execute transactions")

	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("serve local PoAD decisions", "error", err)
		os.Exit(1)
	}
}

func isLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func loadServiceConfig(path string) (map[string]ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg serviceConfig
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if len(cfg.Agents) == 0 {
		return nil, fmt.Errorf("config must contain at least one Agent")
	}
	keys := make(map[string]ed25519.PrivateKey, len(cfg.Agents))
	for _, agent := range cfg.Agents {
		id := strings.TrimSpace(agent.AgentID)
		if id == "" {
			return nil, fmt.Errorf("agent ID must not be empty")
		}

		if _, exists := keys[id]; exists {
			return nil, fmt.Errorf("duplicate Agent ID %q", id)
		}
		keyBytes, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(agent.PrivateKeyHex), "0x"))
		if err != nil {
			return nil, fmt.Errorf("decode private key for %q: %w", id, err)
		}
		if len(keyBytes) != ed25519.PrivateKeySize {
			return nil, fmt.Errorf("private key for %q must be %d bytes", id, ed25519.PrivateKeySize)
		}
		keys[id] = ed25519.PrivateKey(keyBytes)
	}

	return keys, nil
}

func initializeLocalFiles(dir string, count int) error {
	if count < 1 || count > 100 {
		return fmt.Errorf("agent count must be between 1 and 100")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	type publicIdentity struct {
		AgentID      string `json:"agent_id"`
		PublicKeyHex string `json:"public_key_hex"`
		Status       string `json:"status"`
		KeyVersion   uint64 `json:"key_version"`
	}
	cfg := serviceConfig{Agents: make([]agentKeyConfig, 0, count)}

	registry := make([]publicIdentity, 0, count)
	for i := 1; i <= count; i++ {
		publicKey, privateKey, err := poad.GenerateAgentKeyPair()
		if err != nil {
			return err
		}
		id := fmt.Sprintf("local-agent-%02d", i)
		cfg.Agents = append(cfg.Agents, agentKeyConfig{AgentID: id, PrivateKeyHex: hex.EncodeToString(privateKey)})
		registry = append(registry, publicIdentity{AgentID: id, PublicKeyHex: hex.EncodeToString(publicKey), Status: string(poad.IdentityActive), KeyVersion: 1})
	}
	cfgBytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	registryBytes, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	if err := writeNewFile(filepath.Join(dir, "agents.private.json"), append(cfgBytes, '\n'), 0o600); err != nil {
		return err
	}
	if err := writeNewFile(filepath.Join(dir, "registry.public.json"), append(registryBytes, '\n'), 0o644); err != nil {
		return err
	}
	digest := sha256.Sum256(append(registryBytes, '\n'))

	digestText := hex.EncodeToString(digest[:]) + "  registry.public.json\n"
	if err := writeNewFile(filepath.Join(dir, "registry.public.json.sha256"), []byte(digestText), 0o644); err != nil {
		return err
	}

	fmt.Printf("Created %d local demo identities in %s\n", count, dir)
	fmt.Printf("Private keys: %s (keep local; do not commit)\n", filepath.Join(dir, "agents.private.json"))
	fmt.Printf("Public registry: %s\n", filepath.Join(dir, "registry.public.json"))
	fmt.Printf("Registry SHA-256: %s\n", hex.EncodeToString(digest[:]))
	return nil
}

func writeNewFile(path string, data []byte, mode os.FileMode) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("refusing to overwrite existing file %s", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

func (s *decisionService) handleDecisions(w http.ResponseWriter, r *http.Request) {
	started := time.Now()

	if r.Header.Get("Content-Type") != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "Content-Type must be application/json"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)

	defer func() { _ = r.Body.Close() }()

	var req decisionRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request"})
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request must contain exactly one JSON object"})
		return
	}
	if strings.TrimSpace(req.StateRoot) == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "state_root is required"})
		return
	}

	intent, err := poad.FindIntentForTransaction(s.links, req.Transaction)
	if err != nil {
		slog.Warn("poad_decision_request_rejected", "reason", "intent_link_unavailable", "error", err, "elapsed_ms", time.Since(started).Seconds()*1000)
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "transaction is not linked to a trusted intent"})

		return
	}
	if !sameIntent(intent, req.Intent) {
		slog.Warn("poad_decision_request_rejected", "reason", "request_intent_differs_from_trusted_manifest", "transaction_hash", intentHash(req.Transaction), "elapsed_ms", time.Since(started).Seconds()*1000)
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "request intent does not match the trusted manifest"})

		return
	}
	hash, err := req.Transaction.Hash()
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "cannot hash transaction"})
		return
	}
	txHash := hex.EncodeToString(hash)

	decisions := make([]poad.SignedDecision, 0, len(s.agents))
	for agentID, privateKey := range s.agents {
		decision, evalErr := poad.EvaluateWithTraceIntent(poad.VerificationInput{
			AgentID: agentID, TransactionHash: txHash, StateRoot: req.StateRoot,
			Transaction: poad.Pass, State: poad.Pass, Evidence: poad.NotRequired,
		}, req.Transaction, []poad.TraceIntentLink{intent})
		if evalErr != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "evaluate transaction decision"})
			return
		}
		signed, signErr := poad.SignDecision(decision, privateKey)
		if signErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "sign decision"})
			return
		}
		decisions = append(decisions, signed)

		slog.Info("poad_decision",
			"agent_id", agentID,
			"transaction_hash", txHash,
			"request_id", intent.RequestID,
			"state_root", req.StateRoot,
			"result", decision.Result,
			"reasons", decision.Reasons,
			"policy_version", decision.PolicyVersion,
			"verification_latency_ms", time.Since(started).Seconds()*1000)
	}

	writeJSON(w, http.StatusOK, decisionResponse{Decisions: decisions})
}

func sameIntent(a, b poad.TraceIntentLink) bool {
	left, errLeft := json.Marshal(a)
	right, errRight := json.Marshal(b)

	return errLeft == nil && errRight == nil && bytes.Equal(left, right)
}

func intentHash(tx transaction.Transaction) string {
	hash, err := tx.Hash()
	if err != nil {
		return ""
	}
	return hex.EncodeToString(hash)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

