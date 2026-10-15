package csvsource

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const excludeContractTxs = true

func TestCSVSource_ReadTxs(t *testing.T) {
	datasetPath := filepath.Join(t.TempDir(), "testdata.csv")
	dataset, err := os.Create(datasetPath)
	require.NoError(t, err)

	writer := csv.NewWriter(dataset)
	header := make([]string, 13)
	for i := range header {
		header[i] = fmt.Sprintf("column_%d", i)
	}
	require.NoError(t, writer.Write(header))

	for i := 0; i < 5; i++ {
		record := make([]string, 13)
		record[3] = fmt.Sprintf("0x%040x", i+1)
		record[4] = fmt.Sprintf("0x%040x", i+101)
		record[5] = ""
		record[6] = "0"
		record[7] = "0"
		record[8] = "1"
		record[12] = "none"
		require.NoError(t, writer.Write(record))
	}

	writer.Flush()
	require.NoError(t, writer.Error())
	require.NoError(t, dataset.Close())

	cs, err := NewCSVSource(datasetPath, excludeContractTxs)
	require.NoError(t, err)

	txs, err := cs.ReadTxs(10)
	require.NoError(t, err)
	require.Len(t, txs, 5)

	txs, err = cs.ReadTxs(10)
	require.NoError(t, err)
	require.Empty(t, txs)
}
