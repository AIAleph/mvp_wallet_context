package ingest

import (
	"reflect"
	"strings"
	"testing"

	"github.com/AIAleph/mvp_wallet_context/internal/eth"
	"github.com/AIAleph/mvp_wallet_context/internal/normalize"
)

// normalizeTransactionsForAddress is a small helper, but we still exercise all branches
// to keep coverage guarantees at 100% for the ingestion path.
func TestNormalizeTransactionsForAddress(t *testing.T) {
	t.Run("nil transactions returns nil", func(t *testing.T) {
		if got := normalizeTransactionsForAddress(nil, "0xaaaa"); got != nil {
			t.Fatalf("expected nil slice, got %#v", got)
		}
	})

	t.Run("empty target bypasses filtering", func(t *testing.T) {
		txs := []eth.Transaction{{
			Hash:     "0xfeed",
			From:     "0xABCDEF0000000000000000000000000000000001",
			To:       "0xABCDEF0000000000000000000000000000000002",
			ValueWei: "0x1",
			BlockNum: 42,
			TsMillis: 1234,
			Status:   1,
			GasUsed:  21,
		}}
		got := normalizeTransactionsForAddress(txs, "")
		want := normalize.TransactionsToRows(txs, false)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected rows:\n got %+v\nwant %+v", got, want)
		}
	})

	t.Run("filters matches on from and to addresses case-insensitively", func(t *testing.T) {
		targetAddr := "0xaabbccddeeff00112233445566778899aabbccdd"
		uppercaseTarget := strings.ToUpper(targetAddr)
		txs := []eth.Transaction{
			{
				Hash:     "0xAAA",
				From:     "0x1111111111111111111111111111111111111111",
				To:       targetAddr,
				BlockNum: 10,
				TsMillis: 1000,
			},
			{
				Hash:     "0xBBB",
				From:     uppercaseTarget,
				To:       "0x2222222222222222222222222222222222222222",
				BlockNum: 11,
				TsMillis: 1100,
			},
			{
				Hash:     "0xCCC",
				From:     "0x3333333333333333333333333333333333333333",
				To:       "0x4444444444444444444444444444444444444444",
				BlockNum: 12,
				TsMillis: 1200,
			},
		}

		got := normalizeTransactionsForAddress(txs, uppercaseTarget)
		if len(got) != 2 {
			t.Fatalf("expected 2 matching rows, got %d: %#v", len(got), got)
		}
		if got[0].TxHash != strings.ToLower(txs[0].Hash) {
			t.Fatalf("expected first match from to-address, got %s", got[0].TxHash)
		}
		if got[1].TxHash != strings.ToLower(txs[1].Hash) {
			t.Fatalf("expected second match from from-address, got %s", got[1].TxHash)
		}
	})
}
