package ingest

import (
	"context"
	"errors"
	"testing"

	"github.com/AIAleph/mvp_wallet_context/internal/eth"
)

type provGetLogsErr struct{}

func (provGetLogsErr) BlockNumber(ctx context.Context) (uint64, error)                 { return 100, nil }
func (provGetLogsErr) BlockTimestamp(ctx context.Context, block uint64) (int64, error) { return 0, nil }
func (provGetLogsErr) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	return nil, errors.New("logs")
}
func (provGetLogsErr) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	return nil, nil
}
func (provGetLogsErr) Transactions(ctx context.Context, address string, from, to uint64) ([]eth.Transaction, error) {
	return nil, nil
}

type provTransactionsErr struct{}

func (provTransactionsErr) BlockNumber(ctx context.Context) (uint64, error) { return 100, nil }
func (provTransactionsErr) BlockTimestamp(ctx context.Context, block uint64) (int64, error) {
	return 0, nil
}
func (provTransactionsErr) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	return nil, nil
}
func (provTransactionsErr) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	return nil, nil
}
func (provTransactionsErr) Transactions(ctx context.Context, address string, from, to uint64) ([]eth.Transaction, error) {
	return nil, errors.New("tx")
}

func TestProcessRange_LogsError(t *testing.T) {
	ing := NewWithProvider("0x", Options{}, provGetLogsErr{})
	if err := ing.processRange(context.Background(), 1, 1); err == nil {
		t.Fatal("expected error from GetLogs")
	}
}

func TestProcessRange_TransactionsError(t *testing.T) {
	ing := NewWithProvider("0x", Options{}, provTransactionsErr{})
	if err := ing.processRange(context.Background(), 1, 1); err == nil {
		t.Fatal("expected error from Transactions")
	}
}
