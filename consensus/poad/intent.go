package poad

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/transaction"
)

// TraceIntentLink mirrors the payment-related fields in AgentEmulator's
// agent_action_txs.jsonl. The expected endpoints are derived from the original
// trace's agent registry, while TxHashes binds the action to compiled transaction(s).
type TraceIntentLink struct {
	Seq               int      `json:"seq"`
	Action            string   `json:"action"`
	AgentID           string   `json:"agent_id"`
	Target            string   `json:"target"`
	Amount            uint64   `json:"amount"`
	RequestID         string   `json:"request_id"`
	ParamsHash        string   `json:"params_hash"`
	TxHashes          []string `json:"tx_hashes"`
	ExpectedSender    string   `json:"expected_sender"`
	ExpectedRecipient string   `json:"expected_recipient"`
}

// LoadTraceIntentLinks reads AgentEmulator's newline-delimited
// agent_action_txs.jsonl manifest.
func LoadTraceIntentLinks(reader io.Reader) ([]TraceIntentLink, error) {
	scanner := bufio.NewScanner(reader)
	links := make([]TraceIntentLink, 0)

	for line := 1; scanner.Scan(); line++ {
		var link TraceIntentLink

		if err := json.Unmarshal(scanner.Bytes(), &link); err != nil {
			return nil, fmt.Errorf("decode intent link line %d: %w", line, err)
		}

		links = append(links, link)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read intent links: %w", err)
	}

	return links, nil
}

// FindIntentForTransaction returns the trace action linked to the transaction hash.
func FindIntentForTransaction(links []TraceIntentLink, tx transaction.Transaction) (TraceIntentLink, error) {
	hash, err := tx.Hash()
	if err != nil {
		return TraceIntentLink{}, fmt.Errorf("hash transaction for intent lookup: %w", err)
	}

	actualHash := fmt.Sprintf("%x", hash)

	for _, link := range links {
		for _, linkedHash := range link.TxHashes {
			if strings.EqualFold(strings.TrimPrefix(linkedHash, "0x"), actualHash) {
				return link, nil
			}
		}
	}

	return TraceIntentLink{}, fmt.Errorf("no trace intent link found for transaction %s", actualHash)
}

// VerifyPaymentIntent checks a compiled payment transaction against its trace link.
// It verifies the action-to-hash binding, endpoints, and amount. It does not yet
// evaluate free-form natural-language intent or external evidence.
func VerifyPaymentIntent(intent TraceIntentLink, tx transaction.Transaction) (CheckStatus, []string, error) {
	if intent.Action != "pay" || strings.TrimSpace(intent.RequestID) == "" ||
		strings.TrimSpace(intent.AgentID) == "" || strings.TrimSpace(intent.Target) == "" ||
		intent.ExpectedSender == "" || intent.ExpectedRecipient == "" || len(intent.TxHashes) == 0 {
		return Unknown, []string{"payment_intent_metadata_missing"}, nil
	}

	hash, err := tx.Hash()
	if err != nil {
		return Unknown, nil, fmt.Errorf("hash transaction for intent check: %w", err)
	}

	actualHash := fmt.Sprintf("%x", hash)

	linked := false

	for _, expectedHash := range intent.TxHashes {
		if strings.EqualFold(strings.TrimPrefix(expectedHash, "0x"), actualHash) {
			linked = true
			break
		}
	}

	if !linked {
		return Fail, []string{"transaction_not_linked_to_request"}, nil
	}

	var reasons []string
	if tx.TxType() != transaction.NormalTxType || len(tx.Data) != 0 {
		reasons = append(reasons, "payment_transaction_type_mismatch")
	}

	if !strings.EqualFold(intent.ExpectedSender, fmt.Sprintf("0x%x", tx.Sender)) {
		reasons = append(reasons, "payment_sender_mismatch")
	}

	if !strings.EqualFold(intent.ExpectedRecipient, fmt.Sprintf("0x%x", tx.Recipient)) {
		reasons = append(reasons, "payment_recipient_mismatch")
	}

	if tx.Value == nil || tx.Value.Cmp(new(big.Int).SetUint64(intent.Amount)) != 0 {
		reasons = append(reasons, "payment_amount_mismatch")
	}

	if len(reasons) > 0 {
		return Fail, reasons, nil
	}

	return Pass, []string{"payment_intent_matched"}, nil
}
