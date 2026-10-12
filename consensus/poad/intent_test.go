package poad

import (
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/account"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/transaction"
)

func testAddress(last byte) account.Address {
	var a account.Address
	a[len(a)-1] = last
	return a
}

func paymentFixture() (TraceIntentLink, transaction.Transaction) {
	from, to := testAddress(1), testAddress(2)
	tx := transaction.NewTransaction(from, to, big.NewInt(25), big.NewInt(0), 7, time.Unix(1, 0))
	hash, _ := tx.Hash()
	intent := TraceIntentLink{
		Action: "pay", AgentID: "agent-1", Target: "agent-2", Amount: 25,
		RequestID: "request-123", ParamsHash: "v1", TxHashes: []string{fmtHash(hash)},
		ExpectedSender: "0x" + fmtAddress(from), ExpectedRecipient: "0x" + fmtAddress(to),
	}
	return intent, *tx
}

func fmtHash(b []byte) string { return fmt.Sprintf("%x", b) }
func fmtAddress(a account.Address) string { return fmt.Sprintf("%x", a) }

func TestVerifyPaymentIntentPasses(t *testing.T) {
	intent, tx := paymentFixture()
	status, reasons, err := VerifyPaymentIntent(intent, tx)
	if err != nil { t.Fatal(err) }
	if status != Pass { t.Fatalf("status = %s, reasons = %v; want PASS", status, reasons) }
}

func TestVerifyPaymentIntentRejectsAmountMismatch(t *testing.T) {
	intent, tx := paymentFixture()
	intent.Amount = 26
	status, reasons, err := VerifyPaymentIntent(intent, tx)
	if err != nil { t.Fatal(err) }
	if status != Fail { t.Fatalf("status = %s, reasons = %v; want FAIL", status, reasons) }
}

func TestVerifyPaymentIntentRejectsUnlinkedTransaction(t *testing.T) {
	intent, tx := paymentFixture()
	intent.TxHashes = []string{"deadbeef"}
	status, reasons, err := VerifyPaymentIntent(intent, tx)
	if err != nil { t.Fatal(err) }
	if status != Fail { t.Fatalf("status = %s, reasons = %v; want FAIL", status, reasons) }
}

func TestVerifyPaymentIntentAbstainsOnMissingMetadata(t *testing.T) {
	intent, tx := paymentFixture()
	intent.RequestID = ""
	status, reasons, err := VerifyPaymentIntent(intent, tx)
	if err != nil { t.Fatal(err) }
	if status != Unknown { t.Fatalf("status = %s, reasons = %v; want UNKNOWN", status, reasons) }
}

func TestLoadTraceIntentLinksAndFindTransaction(t *testing.T) {
	intent, tx := paymentFixture()
	raw := fmt.Sprintf("{\"action\":\"%s\",\"agent_id\":\"%s\",\"target\":\"%s\",\"amount\":%d,\"request_id\":\"%s\",\"params_hash\":\"%s\",\"tx_hashes\":[\"%s\"],\"expected_sender\":\"%s\",\"expected_recipient\":\"%s\"}\n",
		intent.Action, intent.AgentID, intent.Target, intent.Amount, intent.RequestID, intent.ParamsHash,
		intent.TxHashes[0], intent.ExpectedSender, intent.ExpectedRecipient)
	links, err := LoadTraceIntentLinks(strings.NewReader(raw))
	if err != nil { t.Fatal(err) }
	found, err := FindIntentForTransaction(links, tx)
	if err != nil { t.Fatal(err) }
	status, reasons, err := VerifyPaymentIntent(found, tx)
	if err != nil { t.Fatal(err) }
	if status != Pass { t.Fatalf("status = %s, reasons = %v; want PASS", status, reasons) }
}


func TestEvaluateWithTraceIntentAcceptsMatchedPayment(t *testing.T) {
	intent, tx := paymentFixture()
	hash, err := tx.Hash()
	if err != nil {
		t.Fatal(err)
	}

	decision, err := EvaluateWithTraceIntent(VerificationInput{
		AgentID:         "validator-1",
		TransactionHash: fmtHash(hash),
		Transaction:     Pass,
		State:           Pass,
		Evidence:        NotRequired,
	}, tx, []TraceIntentLink{intent})
	if err != nil {
		t.Fatal(err)
	}

	if decision.Result != Accept {
		t.Fatalf("result = %s, reasons = %v; want ACCEPT", decision.Result, decision.Reasons)
	}
}

func TestEvaluateWithTraceIntentAbstainsWhenLinkMissing(t *testing.T) {
	_, tx := paymentFixture()
	hash, err := tx.Hash()
	if err != nil {
		t.Fatal(err)
	}

	decision, err := EvaluateWithTraceIntent(VerificationInput{
		AgentID:         "validator-1",
		TransactionHash: fmtHash(hash),
		Transaction:     Pass,
		State:           Pass,
		Evidence:        NotRequired,
	}, tx, nil)
	if err != nil {
		t.Fatal(err)
	}

	if decision.Result != Abstain {
		t.Fatalf("result = %s, reasons = %v; want ABSTAIN", decision.Result, decision.Reasons)
	}
}

func TestFindIntentForTransactionRejectsAmbiguousLinks(t *testing.T) {
	intent, tx := paymentFixture()
	other := intent
	other.RequestID = "different-request"

	_, err := FindIntentForTransaction([]TraceIntentLink{intent, other}, tx)
	if err == nil {
		t.Fatal("FindIntentForTransaction() should reject ambiguous links")
	}
}

func TestEvaluateWithTraceIntentRejectsMismatchedDecisionHash(t *testing.T) {
	intent, tx := paymentFixture()
	_, err := EvaluateWithTraceIntent(VerificationInput{
		AgentID:         "validator-1",
		TransactionHash: "not-the-transaction-hash",
		Transaction:     Pass,
		State:           Pass,
		Evidence:        NotRequired,
	}, tx, []TraceIntentLink{intent})
	if err == nil {
		t.Fatal("EvaluateWithTraceIntent() should reject a mismatched transaction hash")
	}
}
