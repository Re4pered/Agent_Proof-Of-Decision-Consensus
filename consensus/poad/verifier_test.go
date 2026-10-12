package poad

import "testing"

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name string
		input VerificationInput
		want Result
		reason string
	}{
		{name: "accept when required checks pass", input: VerificationInput{AgentID: "agent-1", TransactionHash: "tx-1", Transaction: Pass, State: Pass, Intent: Pass, Evidence: NotRequired}, want: Accept},
		{name: "reject known transaction failure", input: VerificationInput{AgentID: "agent-1", TransactionHash: "tx-2", Transaction: Fail, State: Pass, Intent: NotRequired, Evidence: NotRequired}, want: Reject, reason: "transaction_check_failed"},
		{name: "reject known intent violation", input: VerificationInput{AgentID: "agent-1", TransactionHash: "tx-3", Transaction: Pass, State: Pass, Intent: Fail, Evidence: Pass}, want: Reject, reason: "intent_check_failed"},
		{name: "abstain when required data is unknown", input: VerificationInput{AgentID: "agent-1", TransactionHash: "tx-4", Transaction: Pass, State: Pass, Intent: Unknown, Evidence: NotRequired}, want: Abstain, reason: "intent_check_unknown"},
		{name: "cannot accept when state check is not applicable", input: VerificationInput{AgentID: "agent-1", TransactionHash: "tx-5", Transaction: Pass, State: NotRequired, Intent: NotRequired, Evidence: NotRequired}, want: Abstain, reason: "state_check_not_applicable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Evaluate(tt.input)
			if err != nil { t.Fatalf("Evaluate() error = %v", err) }
			if got.Result != tt.want { t.Fatalf("Evaluate() result = %q, want %q", got.Result, tt.want) }
			if tt.reason != "" {
				found := false
				for _, reason := range got.Reasons { if reason == tt.reason { found = true; break } }
				if !found { t.Fatalf("Evaluate() reasons = %v, want reason %q", got.Reasons, tt.reason) }
			}
			if got.PolicyVersion != DefaultPolicyVersion { t.Fatalf("PolicyVersion = %q, want %q", got.PolicyVersion, DefaultPolicyVersion) }
		})
	}
}

func TestEvaluateRequiresIdentityAndTransaction(t *testing.T) {
	base := VerificationInput{AgentID: "agent-1", TransactionHash: "tx-1", Transaction: Pass, State: Pass, Intent: NotRequired, Evidence: NotRequired}
	base.AgentID = ""
	if _, err := Evaluate(base); err == nil { t.Fatal("Evaluate() should reject an empty agent ID") }
	base.AgentID, base.TransactionHash = "agent-1", ""
	if _, err := Evaluate(base); err == nil { t.Fatal("Evaluate() should reject an empty transaction hash") }
}
