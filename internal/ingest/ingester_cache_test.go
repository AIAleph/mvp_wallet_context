package ingest

import (
	"context"
	"testing"

	"github.com/AIAleph/mvp_wallet_context/internal/eth"
)

type tsProv struct{ calls int }

func (p *tsProv) BlockNumber(ctx context.Context) (uint64, error) { return 100, nil }
func (p *tsProv) BlockTimestamp(ctx context.Context, block uint64) (int64, error) {
	p.calls++
	return int64(block) * 1000, nil
}
func (p *tsProv) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	return nil, nil
}
func (p *tsProv) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	return nil, nil
}
func (p *tsProv) Transactions(ctx context.Context, address string, from, to uint64) ([]eth.Transaction, error) {
	return nil, nil
}

func TestGetBlockTsCaches(t *testing.T) {
	p := &tsProv{}
	ing := NewWithProvider("0x", Options{}, p)
	ts1, ok1 := ing.getBlockTs(context.Background(), 10)
	ts2, ok2 := ing.getBlockTs(context.Background(), 10)
	if !ok1 || !ok2 || ts1 != ts2 || p.calls != 1 {
		t.Fatalf("cache failed: ts1=%d ts2=%d ok1=%v ok2=%v calls=%d", ts1, ts2, ok1, ok2, p.calls)
	}
}

func TestGetBlockTs_NoProvider(t *testing.T) {
	ing := New("0x", Options{})
	if _, ok := ing.getBlockTs(context.Background(), 10); ok {
		t.Fatal("expected false with no provider")
	}
}
