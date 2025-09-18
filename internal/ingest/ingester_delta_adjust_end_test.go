package ingest

// Covers delta end adjustment and default batching behavior.

import (
	"context"
	"testing"

	"github.com/AIAleph/mvp_wallet_context/internal/eth"
)

type provHead struct{ h uint64 }

func (p provHead) BlockNumber(ctx context.Context) (uint64, error)                 { return p.h, nil }
func (p provHead) BlockTimestamp(ctx context.Context, block uint64) (int64, error) { return 0, nil }
func (p provHead) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	return nil, nil
}
func (p provHead) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	return nil, nil
}
func (p provHead) Transactions(ctx context.Context, address string, from, to uint64) ([]eth.Transaction, error) {
	return nil, nil
}

func TestDelta_AdjustsEndAndSkipsWhenFromGreater(t *testing.T) {
	// head=10, conf=5 => end=5; from=8, to=20 => adjusted to=5 => skip
	ing := NewWithProvider("0x", Options{FromBlock: 8, ToBlock: 20, Confirmations: 5}, provHead{h: 10})
	if err := ing.Delta(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestBackfill_DefaultBatch(t *testing.T) {
	// batch=0 defaults internally to 1000; should process single range without error
	ing := NewWithProvider("0x", Options{FromBlock: 1, ToBlock: 2, BatchBlocks: 0}, provHead{h: 2})
	if err := ing.Backfill(context.Background()); err != nil {
		t.Fatal(err)
	}
}
