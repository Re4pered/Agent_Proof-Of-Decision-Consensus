package committee

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/HuangLab-SYSU/block-emulator-x/config"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/account"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/transaction"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/message"
)

func smokePayment(sender byte) transaction.Transaction {
	var from, to account.Address
	from[19] = sender
	to[19] = sender + 1

	return *transaction.NewTransaction(
		from, to, big.NewInt(10), big.NewInt(0), 0, time.UnixMilli(int64(sender)),
	)
}

func smokeTxHash(t *testing.T, tx transaction.Transaction) string {
	t.Helper()
	hash, err := tx.Hash()
	require.NoError(t, err)
	return string(hash)
}

func TestStaticRelaySmokeStopsOnlyOnUniqueInjectedCommittedPayments(t *testing.T) {
	tx1, tx2, other := smokePayment(1), smokePayment(3), smokePayment(5)
	key1, key2 := smokeTxHash(t, tx1), smokeTxHash(t, tx2)

	s := &StaticRelayCommittee{
		cfg: config.SupervisorCfg{
			TxNumber:           2,
			StopOnAllCommitted: true,
		},
		unsentTxNum:     1,
		smokeInjected:  map[string]struct{}{key1: {}, key2: {}},
		smokeCommitted: make(map[string]struct{}),
	}

	receive := func(txs ...transaction.Transaction) {
		t.Helper()
		wrapped, err := message.WrapMsg(&message.RelayBlockInfoMsg{InnerShardTxs: txs})
		require.NoError(t, err)
		require.NoError(t, s.HandleMsg(context.Background(), wrapped))
	}

	require.False(t, s.ShouldStop(), "still injecting")
	s.unsentTxNum = 0
	require.False(t, s.ShouldStop(), "no commit confirmations")

	receive(tx1)
	receive(tx1) // duplicate delivery must not count twice
	receive(other) // not in the injected set must not count
	require.Len(t, s.smokeCommitted, 1)
	require.False(t, s.ShouldStop(), "one payment still pending")

	receive(tx2)
	require.True(t, s.ShouldStop())
	require.Len(t, s.smokeCommitted, 2)
}

func TestStaticRelaySmokeRejectsUnexpectedRelayStage(t *testing.T) {
	s := &StaticRelayCommittee{
		cfg: config.SupervisorCfg{TxNumber: 1, StopOnAllCommitted: true},
		smokeInjected: make(map[string]struct{}),
		smokeCommitted: make(map[string]struct{}),
	}
	wrapped, err := message.WrapMsg(&message.RelayBlockInfoMsg{
		Relay1Txs: []transaction.Transaction{smokePayment(8)},
	})
	require.NoError(t, err)
	require.ErrorContains(t, s.HandleMsg(context.Background(), wrapped), "pay-only smoke received relay")
}

func TestStaticRelayStandardStopLogicUnchanged(t *testing.T) {
	s := &StaticRelayCommittee{
		cfg: config.SupervisorCfg{TxNumber: 2},
		sl: stopLogic{stopThreshold: 20, stopCnt: 19},
		unsentTxNum: 0,
	}
	require.False(t, s.ShouldStop())
	s.sl.stopCnt++
	require.True(t, s.ShouldStop())
}
