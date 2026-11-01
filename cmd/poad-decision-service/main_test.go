package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/HuangLab-SYSU/block-emulator-x/consensus/poad"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/account"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/transaction"
)

func serviceFixture(t *testing.T) (*decisionService, decisionRequest, ed25519.PublicKey) {
	t.Helper()
	from, to := account.Address{}, account.Address{}
	from[len(from)-1], to[len(to)-1] = 1, 2
	tx := transaction.NewTransaction(from, to, big.NewInt(25), big.NewInt(0), 7, time.Unix(1, 0).UTC())
	hash, err := tx.Hash()
	if err != nil {
		t.Fatal(err)
	}
	intent := poad.TraceIntentLink{
		Action: "pay", AgentID: "local-agent-01", Target: "recipient", Amount: 25,
		RequestID: "request-1", ParamsHash: "params-v1",
		TxHashes: []string{hex.EncodeToString(hash)},
		ExpectedSender: "0x" + hex.EncodeToString(from[:]),
		ExpectedRecipient: "0x" + hex.EncodeToString(to[:]),
	}
	publicKey, privateKey, err := poad.GenerateAgentKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	svc := &decisionService{
		agents: map[string]ed25519.PrivateKey{"local-agent-01": privateKey},
		links: []poad.TraceIntentLink{intent},
	}
	return svc, decisionRequest{Transaction: *tx, Intent: intent, StateRoot: "state-root-1"}, publicKey
}

func TestDecisionServiceSignsTrustedPaymentDecision(t *testing.T) {
	svc, request, publicKey := serviceFixture(t)
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest := httptest.NewRequest(http.MethodPost, "/decisions", bytes.NewReader(body))
	httpRequest.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	svc.handleDecisions(recorder, httpRequest)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response decisionResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Decisions) != 1 {
		t.Fatalf("decisions = %d, want 1", len(response.Decisions))
	}
	signed := response.Decisions[0]
	if signed.Decision.Result != poad.Accept {
		t.Fatalf("result = %s, reasons = %v; want ACCEPT", signed.Decision.Result, signed.Decision.Reasons)
	}
	if signed.Decision.StateRoot != request.StateRoot {
		t.Fatalf("state root = %q, want %q", signed.Decision.StateRoot, request.StateRoot)
	}
	if err := poad.VerifyDecisionSignature(signed, publicKey, "local-agent-01"); err != nil {
		t.Fatalf("decision signature did not verify: %v", err)
	}
}

func TestDecisionServiceRejectsIntentNotInTrustedManifest(t *testing.T) {
	svc, request, _ := serviceFixture(t)
	request.Intent.Amount++
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest := httptest.NewRequest(http.MethodPost, "/decisions", bytes.NewReader(body))
	httpRequest.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	svc.handleDecisions(recorder, httpRequest)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "trusted manifest") {
		t.Fatalf("unexpected error response: %s", recorder.Body.String())
	}
}

func TestDecisionServiceRequiresStateRoot(t *testing.T) {
	svc, request, _ := serviceFixture(t)
	request.StateRoot = ""
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest := httptest.NewRequest(http.MethodPost, "/decisions", bytes.NewReader(body))
	httpRequest.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	svc.handleDecisions(recorder, httpRequest)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}
}

func TestLoopbackAddressValidation(t *testing.T) {
	for _, tc := range []struct {
		address string
		want    bool
	}{
		{"127.0.0.1:38881", true},
		{"[::1]:38881", true},
		{"0.0.0.0:38881", false},
		{"example.com:38881", false},
		{"bad-address", false},
	} {
		if got := isLoopbackAddress(tc.address); got != tc.want {
			t.Errorf("isLoopbackAddress(%q) = %t, want %t", tc.address, got, tc.want)
		}
	}
}

func TestLoadServiceConfigRejectsInvalidKey(t *testing.T) {
	path := t.TempDir() + "/keys.json"
	if err := os.WriteFile(path, []byte(`{"agents":[{"agent_id":"agent-1","private_key_hex":"abcd"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadServiceConfig(path); err == nil {
		t.Fatal("expected invalid private key to be rejected")
	}
}

func TestInitializeLocalFilesCreatesLoadableRegistry(t *testing.T) {
	dir := t.TempDir() + "/local-poad"
	if err := initializeLocalFiles(dir, 3); err != nil {
		t.Fatal(err)
	}
	registryBytes, err := os.ReadFile(dir + "/registry.public.json")
	if err != nil {
		t.Fatal(err)
	}
	digestBytes, err := os.ReadFile(dir + "/registry.public.json.sha256")
	if err != nil {
		t.Fatal(err)
	}
	digestFields := strings.Fields(string(digestBytes))
	if len(digestFields) == 0 {
		t.Fatal("registry digest file is empty")
	}
	registry, err := poad.LoadIdentityRegistryJSON(registryBytes, digestFields[0])
	if err != nil {
		t.Fatalf("generated public registry cannot be loaded: %v", err)
	}
	if _, err := os.Stat(dir + "/agents.private.json"); err != nil {
		t.Fatalf("private key file was not created: %v", err)
	}
	if _, err := registry.Get("local-agent-01"); err != nil {
		t.Fatalf("generated Agent identity missing from registry: %v", err)
	}
	if err := initializeLocalFiles(dir, 3); err == nil {
		t.Fatal("initialization should refuse to overwrite existing key files")
	}
}
