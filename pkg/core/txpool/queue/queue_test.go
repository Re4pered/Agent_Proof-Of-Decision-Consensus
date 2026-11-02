package queue

import (
	"math/big"
	"testing"
	"time"

	"github.com/HuangLab-SYSU/block-emulator-x/config"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/account"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/transaction"
)

func TestPackOneTxPopsExactlyOneForEachPoolLimitMode(t *testing.T) {
	for _, poolType := range []string{config.TxPoolNumType, config.TxPoolByteType} {
		t.Run(poolType, func(t *testing.T) {
			pool, err := NewTxPool(config.TxPoolCfg{Type: poolType})
			if err != nil {
				t.Fatal(err)
			}

			txs := make([]transaction.Transaction, 3)
			for i := range txs {
				from, to := account.Address{}, account.Address{}
				from[len(from)-1] = byte(i + 1)
				to[len(to)-1] = byte(i + 10)
				txs[i] = *transaction.NewTransaction(from, to, big.NewInt(int64(i+1)), big.NewInt(0), uint64(i), time.Unix(int64(i+1), 0))
			}
			if err := pool.AddTxs(txs); err != nil {
				t.Fatal(err)
			}

			first, err := pool.PackOneTx()
			if err != nil {
				t.Fatal(err)
			}
			if len(first) != 1 {
				t.Fatalf("PackOneTx returned %d transactions, want 1", len(first))
			}

			second, err := pool.PackOneTx()
			if err != nil {
				t.Fatal(err)
			}
			third, err := pool.PackOneTx()
			if err != nil {
				t.Fatal(err)
			}
			if len(second) != 1 || len(third) != 1 {
				t.Fatalf("subsequent PackOneTx lengths = %d, %d; want 1, 1", len(second), len(third))
			}
			empty, err := pool.PackOneTx()
			if err != nil {
				t.Fatal(err)
			}
			if len(empty) != 0 {
				t.Fatalf("PackOneTx on an empty pool returned %d transactions, want 0", len(empty))
			}

			firstHash, err := first[0].Hash()
			if err != nil {
				t.Fatal(err)
			}
			secondHash, err := second[0].Hash()
			if err != nil {
				t.Fatal(err)
			}
			if string(firstHash) == string(secondHash) {
				t.Fatal("PackOneTx should preserve queue order and remove only one transaction")
			}
		})
	}
}
