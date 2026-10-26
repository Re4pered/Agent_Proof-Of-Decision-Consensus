package agentsupervisor

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWritePoADPaymentManifestContainsOnlyBoundPayments(t *testing.T) {
	dir := t.TempDir()
	links := []ActionTxLink{
		{Seq: 1, Action: ActionJoin, AgentID: "alice", TxHashes: []string{"join-hash"}},
		{
			Seq: 2, Action: ActionPay, AgentID: "alice", Target: "bob", Amount: 7,
			RequestID: "pay-7", TxHashes: []string{"deadbeef"},
			ExpectedSender: "0x1111", ExpectedRecipient: "0x2222",
		},
		{Seq: 3, Action: ActionRawTx, TxHashes: []string{"raw-hash"}},
	}
	require.NoError(t, writePoADPaymentManifest(dir, links))

	raw, err := os.ReadFile(filepath.Join(dir, PoADPaymentManifestFileName))
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(string(raw), "\n"))
	require.Contains(t, string(raw), "\"request_id\":\"pay-7\"")
	require.Contains(t, string(raw), "\"tx_hashes\":[\"deadbeef\"]")
	require.NotContains(t, string(raw), "join-hash")
	require.NotContains(t, string(raw), "raw-hash")

	digestRaw, err := os.ReadFile(filepath.Join(dir, PoADPaymentManifestDigestFileName))
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	require.Equal(t, hex.EncodeToString(sum[:]), strings.Fields(string(digestRaw))[0])
}

func TestWritePoADPaymentManifestFailsClosedOnIncompletePayment(t *testing.T) {
	err := writePoADPaymentManifest(t.TempDir(), []ActionTxLink{
		{Seq: 4, Action: ActionPay, AgentID: "alice", Target: "bob", TxHashes: []string{"hash"}},
	})
	require.ErrorContains(t, err, "missing PoAD intent fields")
}

func TestCopyPoADPaymentManifestVerifiesDigest(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	links := []ActionTxLink{{
		Seq: 1, Action: ActionPay, AgentID: "alice", Target: "bob", Amount: 7,
		RequestID: "pay-7", TxHashes: []string{"deadbeef"},
		ExpectedSender: "0x1111", ExpectedRecipient: "0x2222",
	}}
	require.NoError(t, writePoADPaymentManifest(sourceDir, links))
	require.NoError(t, copyPoADPaymentManifest(filepath.Join(sourceDir, PoADPaymentManifestFileName), targetDir))

	staged, err := os.ReadFile(filepath.Join(targetDir, PoADPaymentManifestFileName))
	require.NoError(t, err)
	source, err := os.ReadFile(filepath.Join(sourceDir, PoADPaymentManifestFileName))
	require.NoError(t, err)
	require.Equal(t, source, staged)

	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, PoADPaymentManifestFileName), []byte("tampered\n"), 0o644))
	err = copyPoADPaymentManifest(filepath.Join(sourceDir, PoADPaymentManifestFileName), t.TempDir())
	require.ErrorContains(t, err, "digest mismatch")
}
