package agentsupervisor

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// PoADPaymentManifestFileName is the payment-only intent manifest consumed
	// by the consensus runtime. It is derived from the same action-to-tx map
	// that was written with the transaction plan.
	PoADPaymentManifestFileName       = "poad_payment_intents.jsonl"
	PoADPaymentManifestDigestFileName = PoADPaymentManifestFileName + ".sha256"
)

// writePoADPaymentManifest writes only payment actions using the JSON shape
// consumed by consensus/poad.TraceIntentLink. Join/leave and raw transfers are
// intentionally excluded: they do not carry a trace-linked payment intent.
func writePoADPaymentManifest(dir string, links []ActionTxLink) error {
	path := filepath.Join(dir, PoADPaymentManifestFileName)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create PoAD payment manifest: %w", err)
	}

	w := bufio.NewWriter(f)

	for _, link := range links {
		if link.Action != ActionPay {
			continue
		}

		if strings.TrimSpace(link.AgentID) == "" ||
			strings.TrimSpace(link.Target) == "" ||
			link.ExpectedSender == "" ||
			link.ExpectedRecipient == "" ||
			len(link.TxHashes) == 0 {
			_ = f.Close()
			return fmt.Errorf("payment action seq %d is missing PoAD intent fields", link.Seq)
		}

		b, err := json.Marshal(link)
		if err != nil {
			_ = f.Close()
			return fmt.Errorf("encode PoAD payment intent seq %d: %w", link.Seq, err)
		}
		if _, err := w.Write(append(b, '\n')); err != nil {
			_ = f.Close()
			return fmt.Errorf("write PoAD payment intent seq %d: %w", link.Seq, err)
		}
	}

	if err := w.Flush(); err != nil {
		_ = f.Close()
		return fmt.Errorf("flush PoAD payment manifest: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close PoAD payment manifest: %w", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read PoAD payment manifest for digest: %w", err)
	}
	digest := sha256.Sum256(raw)

	digestText := hex.EncodeToString(digest[:]) + "  " + PoADPaymentManifestFileName + "\n"
	if err := os.WriteFile(filepath.Join(dir, PoADPaymentManifestDigestFileName), []byte(digestText), 0o644); err != nil {
		return fmt.Errorf("write PoAD payment manifest digest: %w", err)
	}

	return nil
}
