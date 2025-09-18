package ingest

import (
	"context"
	"errors"
	"testing"

	"github.com/AIAleph/mvp_wallet_context/internal/eth"
)

type tsErrProv struct{}

func (tsErrProv) BlockNumber(ctx context.Context) (uint64, error) { return 0, nil }
func (tsErrProv) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	return nil, nil
}
func (tsErrProv) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	return nil, nil
}
func (tsErrProv) Transactions(ctx context.Context, address string, from, to uint64) ([]eth.Transaction, error) {
	return nil, nil
}
func (tsErrProv) BlockTimestamp(ctx context.Context, block uint64) (int64, error) {
	return 0, errors.New("ts")
}

func TestGetBlockTs_ErrorPath(t *testing.T) {
	ing := NewWithProvider("0x", Options{}, tsErrProv{})
	if _, ok := ing.getBlockTs(context.Background(), 5); ok {
		t.Fatal("expected false when provider returns error")
	}
}
