package poad

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/transaction"
)

// AgentSupervisorDecisionProvider requests signed decisions from an AgentSupervisor
// decision-collection endpoint. The endpoint must return decisions signed by the
// individual Agent identities; this adapter never signs or fabricates decisions.
//
// Request JSON contains transaction, intent, and state_root. Response JSON contains
// decisions, an array of SignedDecision values. The PaymentProposalBuilder remains
// responsible for verifying every signature and enforcing the ACCEPT quorum.
type AgentSupervisorDecisionProvider struct {
	endpoint string
	client   *http.Client
}

type agentSupervisorDecisionRequest struct {
	Transaction transaction.Transaction `json:"transaction"`
	Intent      TraceIntentLink         `json:"intent"`
	StateRoot   string                  `json:"state_root"`
}

type agentSupervisorDecisionResponse struct {
	Decisions []SignedDecision `json:"decisions"`
}

// NewAgentSupervisorDecisionProvider creates a bounded-time HTTP adapter for an
// AgentSupervisor endpoint. The endpoint must be an absolute HTTP(S) URL.
func NewAgentSupervisorDecisionProvider(endpoint string, timeout time.Duration) (*AgentSupervisorDecisionProvider, error) {
	endpoint = strings.TrimSpace(endpoint)

	if endpoint == "" {
		return nil, fmt.Errorf("AgentSupervisor decision endpoint is required")
	}

	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		return nil, fmt.Errorf("AgentSupervisor decision endpoint must use HTTP or HTTPS")
	}

	if timeout <= 0 {
		return nil, fmt.Errorf("AgentSupervisor decision timeout must be positive")
	}

	return &AgentSupervisorDecisionProvider{
		endpoint: endpoint,
		client:   &http.Client{Timeout: timeout},
	}, nil
}

// CollectSignedDecisions requests decisions for one exact transaction, intent, and
// validated state root. Signature, transaction binding, state-root binding, duplicate
// Agent IDs, and quorum checks are enforced by the caller's PoAD verification path.
func (p *AgentSupervisorDecisionProvider) CollectSignedDecisions(
	ctx context.Context,
	tx transaction.Transaction,
	intent TraceIntentLink,
	stateRoot string,
) ([]SignedDecision, error) {
	if p == nil || p.client == nil || p.endpoint == "" {
		return nil, fmt.Errorf("AgentSupervisor decision provider is not configured")
	}

	if strings.TrimSpace(stateRoot) == "" {
		return nil, fmt.Errorf("state root is required")
	}

	payload, err := json.Marshal(agentSupervisorDecisionRequest{
		Transaction: tx,
		Intent:      intent,
		StateRoot:   stateRoot,
	})
	if err != nil {
		return nil, fmt.Errorf("encode AgentSupervisor decision request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create AgentSupervisor decision request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request AgentSupervisor decisions: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	const maxResponseBytes = 4 << 20

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))

	if err != nil {
		return nil, fmt.Errorf("read AgentSupervisor decision response: %w", err)
	}

	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("AgentSupervisor decision response exceeds %d bytes", maxResponseBytes)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("AgentSupervisor decision endpoint returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result agentSupervisorDecisionResponse

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode AgentSupervisor decision response: %w", err)
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("AgentSupervisor decision response contains trailing JSON data")
	}

	if len(result.Decisions) == 0 {
		return nil, fmt.Errorf("AgentSupervisor returned no signed decisions")
	}

	return result.Decisions, nil
}
