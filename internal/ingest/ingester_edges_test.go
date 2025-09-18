package ingest

import (
	"context"
	"errors"
	"testing"

	"github.com/AIAleph/mvp_wallet_context/internal/eth"
)

type provErrUnsup struct{}

func (provErrUnsup) BlockNumber(ctx context.Context) (uint64, error)                 { return 10, nil }
func (provErrUnsup) BlockTimestamp(ctx context.Context, block uint64) (int64, error) { return 0, nil }
func (provErrUnsup) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	return nil, nil
}
func (provErrUnsup) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	return nil, eth.ErrUnsupported
}

type provErr struct{}

func (provErr) BlockNumber(ctx context.Context) (uint64, error)                 { return 0, errors.New("boom") }
func (provErr) BlockTimestamp(ctx context.Context, block uint64) (int64, error) { return 0, nil }
func (provErr) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	return nil, nil
}
func (provErr) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	return nil, nil
}

func TestBackfillFromGreaterThanTo(t *testing.T) {
	ing := NewWithProvider("0x", Options{FromBlock: 10, ToBlock: 5, BatchBlocks: 1}, provErrUnsup{})
	if err := ing.Backfill(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeltaNothingToDoAndErrUnsupported(t *testing.T) {
	// head=10, confirmations=12 -> end=0, from=5 -> nothing
	ing := NewWithProvider("0x", Options{FromBlock: 5, ToBlock: 0, Confirmations: 12, BatchBlocks: 10}, provErrUnsup{})
	if err := ing.Delta(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestBackfillProviderError(t *testing.T) {
	ing := NewWithProvider("0x", Options{FromBlock: 0, ToBlock: 0, BatchBlocks: 1}, provErr{})
	if err := ing.Backfill(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}
