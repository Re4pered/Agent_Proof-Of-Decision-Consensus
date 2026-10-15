package poad

import (
	"fmt"
	"testing"
)

func benchmarkSignedDecisions(b *testing.B, count int) (*IdentityRegistry, []SignedDecision) {
	b.Helper()

	registry := NewIdentityRegistry()
	decisions := make([]SignedDecision, 0, count)

	for i := 0; i < count; i++ {
		agentID := fmt.Sprintf("agent-%03d", i)
		publicKey, privateKey, err := GenerateAgentKeyPair()
		if err != nil {
			b.Fatal(err)
		}

		if err := registry.Register(agentID, publicKey); err != nil {
			b.Fatal(err)
		}

		decision, err := SignDecision(Decision{
			AgentID:         agentID,
			TransactionHash: "benchmark-transaction",
			Result:          Accept,
			PolicyVersion:   DefaultPolicyVersion,
		}, privateKey)
		if err != nil {
			b.Fatal(err)
		}

		decisions = append(decisions, decision)
	}

	return registry, decisions
}

// BenchmarkAggregateSignedDecisions measures signature verification and threshold aggregation.
// Key generation and registry setup are intentionally excluded from the timed section.
func BenchmarkAggregateSignedDecisions(b *testing.B) {
	for _, committeeSize := range []int{4, 7, 10, 13, 25, 50} {
		b.Run(fmt.Sprintf("committee_%d", committeeSize), func(b *testing.B) {
			registry, decisions := benchmarkSignedDecisions(b, committeeSize)
			requiredVotes := (2 * ((committeeSize - 1) / 3)) + 1

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if _, err := AggregateSignedDecisions(
					registry,
					decisions,
					"benchmark-transaction",
					DefaultPolicyVersion,
					requiredVotes,
				); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkVerifyBlockFinalization measures repeated verification of an already prepared
// signed decision quorum. State-root execution and block construction are outside this helper.
func BenchmarkVerifyBlockFinalization(b *testing.B) {
	for _, committeeSize := range []int{4, 7, 10, 13, 25, 50} {
		b.Run(fmt.Sprintf("committee_%d", committeeSize), func(b *testing.B) {
			registry, decisions := benchmarkSignedDecisions(b, committeeSize)
			requiredVotes := (2 * ((committeeSize - 1) / 3)) + 1
			record, err := PrepareBlockFinalization(
				registry,
				decisions,
				"benchmark-transaction",
				DefaultPolicyVersion,
				requiredVotes,
				"benchmark-state-root",
			)
			if err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if err := VerifyBlockFinalization(
					registry,
					decisions,
					record,
					"benchmark-state-root",
				); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
