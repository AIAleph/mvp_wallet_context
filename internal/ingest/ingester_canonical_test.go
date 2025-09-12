package ingest

import (
    "context"
    "testing"

    "github.com/AIAleph/mvp_wallet_context/internal/eth"
)

// Provider returns one log and one trace to drive canonical insert paths
type provCanon struct{}
func (provCanon) BlockNumber(ctx context.Context) (uint64, error) { return 1, nil }
func (provCanon) BlockTimestamp(ctx context.Context, block uint64) (int64, error) { return 1_000, nil }
func (provCanon) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
    return []eth.Trace{{TxHash: "0x1", TraceID: "root", From: address, To: address, ValueWei: "0x1", BlockNum: from, TsMillis: 1_000}}, nil
}
func (provCanon) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
    return []eth.Log{{TxHash: "0x2", Index: 0, Address: address, Topics: []string{"0xddf252ad", "0x"}, DataHex: "0x1", BlockNum: from, TsMillis: 2_000}}, nil
}

func TestProcessRange_WritesCanonicalTables(t *testing.T) {
    ing := NewWithProvider("0xabc", Options{Schema: "canonical"}, provCanon{})
    if err := ing.processRange(context.Background(), 1, 1); err != nil {
        t.Fatal(err)
    }
}

