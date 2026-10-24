package poad

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAgentSupervisorDecisionProviderCollectsSignedDecisions(t *testing.T) {
	intent, tx := paymentFixture()
	var got agentSupervisorDecisionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"decisions":[{"decision":{"agent_id":"agent-1","transaction_hash":"tx","state_root":"root","result":"ACCEPT","policy_version":"poad-rules-v1"},"signature":"AQ=="}]}`))
	}))
	defer server.Close()

	provider, err := NewAgentSupervisorDecisionProvider(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := provider.CollectSignedDecisions(context.Background(), tx, intent, "root")
	if err != nil {
		t.Fatalf("CollectSignedDecisions() error = %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision.AgentID != "agent-1" {
		t.Fatalf("unexpected decisions: %#v", decisions)
	}
	if got.StateRoot != "root" {
		t.Fatalf("request state_root = %q, want root", got.StateRoot)
	}
	if got.Intent.RequestID != intent.RequestID {
		t.Fatalf("request intent ID = %q, want %q", got.Intent.RequestID, intent.RequestID)
	}
}

func TestAgentSupervisorDecisionProviderRejectsEmptyDecisions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"decisions":[]}`))
	}))
	defer server.Close()

	provider, err := NewAgentSupervisorDecisionProvider(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	intent, tx := paymentFixture()
	if _, err := provider.CollectSignedDecisions(context.Background(), tx, intent, "root"); err == nil {
		t.Fatal("expected empty decision list to fail")
	}
}

func TestAgentSupervisorDecisionProviderReportsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "supervisor unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	provider, err := NewAgentSupervisorDecisionProvider(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	intent, tx := paymentFixture()
	_, err = provider.CollectSignedDecisions(context.Background(), tx, intent, "root")
	if err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("expected HTTP 503 error, got %v", err)
	}
}

func TestNewAgentSupervisorDecisionProviderValidatesConfiguration(t *testing.T) {
	for _, tc := range []struct {
		endpoint string
		timeout  time.Duration
	}{
		{"", time.Second},
		{"localhost:8080/decisions", time.Second},
		{"http://localhost:8080/decisions", 0},
	} {
		if _, err := NewAgentSupervisorDecisionProvider(tc.endpoint, tc.timeout); err == nil {
			t.Errorf("expected invalid configuration to fail: endpoint=%q timeout=%s", tc.endpoint, tc.timeout)
		}
	}
}
