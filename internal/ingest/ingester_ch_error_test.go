package ingest

import (
	"context"
	"testing"

	"github.com/AIAleph/mvp_wallet_context/internal/eth"
)

type provOneLog struct{}

func (provOneLog) BlockNumber(ctx context.Context) (uint64, error)                 { return 0, nil }
func (provOneLog) BlockTimestamp(ctx context.Context, block uint64) (int64, error) { return 0, nil }
func (provOneLog) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	return nil, nil
}
func (provOneLog) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	return []eth.Log{{TxHash: "0x1", Index: 0, Address: address, Topics: []string{"0xddf252ad"}, DataHex: "0x", BlockNum: 1}}, nil
}
func (provOneLog) Transactions(ctx context.Context, address string, from, to uint64) ([]eth.Transaction, error) {
	return nil, nil
}

func TestProcessRange_CHInsertErrorSurfaces(t *testing.T) {
	// Use an invalid DSN that fails url.Parse inside ClickHouse client
	ing := NewWithProvider("0xabc", Options{ClickHouseDSN: "http://["}, provOneLog{})
	if err := ing.processRange(context.Background(), 1, 1); err == nil {
		t.Fatal("expected insert error to surface")
	}
}

// (Note: We force a parse error rather than mocking HTTP to avoid reaching
// into unexported ClickHouse client internals from another package.)
