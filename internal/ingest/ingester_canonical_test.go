package ingest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/AIAleph/mvp_wallet_context/internal/eth"
	"github.com/AIAleph/mvp_wallet_context/internal/normalize"
)

// Provider returns one log and one trace to drive canonical insert paths
type provCanon struct{}

func (provCanon) BlockNumber(ctx context.Context) (uint64, error)                 { return 1, nil }
func (provCanon) BlockTimestamp(ctx context.Context, block uint64) (int64, error) { return 1_000, nil }
func (provCanon) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	return []eth.Trace{{TxHash: "0x1", TraceID: "root", From: address, To: address, ValueWei: "0x1", BlockNum: from, TsMillis: 1_000}}, nil
}
func (provCanon) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	return []eth.Log{{TxHash: "0x2", Index: 0, Address: address, Topics: []string{"0xddf252ad", "0x"}, DataHex: "0x1", BlockNum: from, TsMillis: 2_000}}, nil
}
func (provCanon) Transactions(ctx context.Context, address string, from, to uint64) ([]eth.Transaction, error) {
	return []eth.Transaction{{Hash: "0x4", From: address, To: address, BlockNum: from, TsMillis: 3_000, ValueWei: "0x1", Status: 1}}, nil
}

func TestProcessRange_WritesCanonicalTables(t *testing.T) {
	ing := NewWithProvider("0xabc", Options{Schema: "canonical"}, provCanon{})
	if err := ing.processRange(context.Background(), 1, 1); err != nil {
		t.Fatal(err)
	}
}

type provCanonFull struct{}

func (provCanonFull) BlockNumber(ctx context.Context) (uint64, error) { return 1, nil }
func (provCanonFull) BlockTimestamp(ctx context.Context, block uint64) (int64, error) {
	return 1_000, nil
}
func (provCanonFull) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	return []eth.Trace{{TxHash: "0x3", TraceID: "root", From: address, To: address, ValueWei: "0x1", BlockNum: from, TsMillis: 1_000}}, nil
}
func (provCanonFull) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	pad := func(a string) string {
		return "0x" + strings.Repeat("0", 24) + strings.TrimPrefix(strings.ToLower(a), "0x")
	}
	token := "0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	fromA := "0x1111111111111111111111111111111111111111"
	toA := "0x2222222222222222222222222222222222222222"
	spender := "0x3333333333333333333333333333333333333333"
	// ERC-20 transfer
	l20 := eth.Log{TxHash: "0xa", Index: 0, Address: token, Topics: []string{"0xddf252ad", pad(fromA), pad(toA)}, DataHex: "0x" + strings.Repeat("0", 63) + "1", BlockNum: from, TsMillis: 1_000}
	// Approval (ERC-20)
	lApprove := eth.Log{TxHash: "0xb", Index: 1, Address: token, Topics: []string{"0x8c5be1e5", pad(fromA), pad(spender)}, DataHex: "0x" + strings.Repeat("0", 63) + "2", BlockNum: from, TsMillis: 1_000}
	// ApprovalForAll (ERC-721 style)
	lForAll := eth.Log{TxHash: "0xc", Index: 2, Address: token, Topics: []string{"0x17307eab", pad(fromA), pad(spender)}, DataHex: "0x" + strings.Repeat("0", 63) + "1", BlockNum: from, TsMillis: 1_000}
	return []eth.Log{l20, lApprove, lForAll}, nil
}
func (provCanonFull) Transactions(ctx context.Context, address string, from, to uint64) ([]eth.Transaction, error) {
	toAddr := "0xbeefbeefbeefbeefbeefbeefbeefbeefbeefbeef"
	return []eth.Transaction{{Hash: "0x4", From: address, To: toAddr, BlockNum: from, TsMillis: 3_000, ValueWei: "0x1", Status: 1, InputHex: "0xa9059cbb"}}, nil
}

func TestProcessRange_WritesCanonicalTokenAndApprovalRows(t *testing.T) {
	prov := provCanonFull{}
	logs, err := prov.GetLogs(context.Background(), "0xabc", 1, 1, nil)
	if err != nil {
		t.Fatalf("GetLogs error: %v", err)
	}
	transfers, approvals := normalize.DecodeTokenEvents(logs)
	if len(transfers) == 0 || len(approvals) == 0 {
		t.Fatalf("expected transfers and approvals, got %d/%d", len(transfers), len(approvals))
	}

	ing := NewWithProvider("0xabc", Options{Schema: "canonical"}, prov)
	if err := ing.processRange(context.Background(), 1, 1); err != nil {
		t.Fatal(err)
	}
}

func TestProcessRange_CanonicalInsertsSuccess(t *testing.T) {
	prov := provCanonFull{}
	calls := map[string]int{}
	ing := NewWithProvider("0xabc", Options{Schema: "canonical", ClickHouseDSN: "http://localhost:8123/db"}, prov)
	ing.ch.SetTransport(rtFunc(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query().Get("query")
		switch {
		case strings.Contains(q, "INSERT INTO logs"):
			calls["logs"]++
		case strings.Contains(q, "INSERT INTO token_transfers"):
			calls["token_transfers"]++
		case strings.Contains(q, "INSERT INTO approvals"):
			calls["approvals"]++
		case strings.Contains(q, "INSERT INTO traces"):
			calls["traces"]++
		case strings.Contains(q, "INSERT INTO transactions"):
			calls["transactions"]++
		}
		return &http.Response{StatusCode: 200, Body: ioNopCloser("ok")}, nil
	}))
	if err := ing.processRange(context.Background(), 1, 1); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"logs", "token_transfers", "approvals", "traces", "transactions"} {
		if calls[table] == 0 {
			t.Fatalf("expected %s insert", table)
		}
	}
}

func TestProcessRange_CanonicalTransactionsRow(t *testing.T) {
	ing := NewWithProvider("0xabc", Options{Schema: "canonical", ClickHouseDSN: "http://localhost:8123/db"}, provCanonFull{})
	var payload string
	ing.ch.SetTransport(rtFunc(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query().Get("query")
		if strings.Contains(q, "INSERT INTO transactions") {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			payload = strings.TrimSpace(string(body))
		}
		return &http.Response{StatusCode: 200, Body: ioNopCloser("ok")}, nil
	}))
	if err := ing.processRange(context.Background(), 1, 1); err != nil {
		t.Fatal(err)
	}
	if payload == "" {
		t.Fatal("expected transactions insert payload")
	}
	var row struct {
		InputMethod string `json:"input_method"`
		ValueRaw    string `json:"value_raw"`
		IsInternal  uint8  `json:"is_internal"`
	}
	if err := json.Unmarshal([]byte(payload), &row); err != nil {
		t.Fatalf("decode insert: %v", err)
	}
	if row.InputMethod != "transfer" {
		t.Fatalf("input_method=%s", row.InputMethod)
	}
	if row.ValueRaw != "1" {
		t.Fatalf("value_raw=%s", row.ValueRaw)
	}
	if row.IsInternal != 0 {
		t.Fatalf("expected external transaction, got %d", row.IsInternal)
	}
}

type provCanonTraceTx struct{}

func (provCanonTraceTx) BlockNumber(ctx context.Context) (uint64, error) { return 1, nil }
func (provCanonTraceTx) BlockTimestamp(ctx context.Context, block uint64) (int64, error) {
	return 1_000, nil
}
func (provCanonTraceTx) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	return nil, nil
}
func (provCanonTraceTx) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	return nil, nil
}
func (provCanonTraceTx) Transactions(ctx context.Context, address string, from, to uint64) ([]eth.Transaction, error) {
	return []eth.Transaction{{
		Hash:     "0x5",
		From:     address,
		To:       address,
		ValueWei: "0x1",
		Status:   1,
		BlockNum: from,
		TsMillis: 4_000,
		TraceID:  "trace-1",
	}}, nil
}

func TestProcessRange_CanonicalTransactionTraceID(t *testing.T) {
	ing := NewWithProvider("0xabc", Options{Schema: "canonical", ClickHouseDSN: "http://localhost:8123/db"}, provCanonTraceTx{})
	var payload string
	ing.ch.SetTransport(rtFunc(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query().Get("query")
		if strings.Contains(q, "INSERT INTO transactions") {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			payload = strings.TrimSpace(string(body))
		}
		return &http.Response{StatusCode: 200, Body: ioNopCloser("ok")}, nil
	}))
	if err := ing.processRange(context.Background(), 1, 1); err != nil {
		t.Fatal(err)
	}
	if payload == "" {
		t.Fatal("expected transactions insert payload")
	}
	var row struct {
		TraceID string `json:"trace_id"`
	}
	if err := json.Unmarshal([]byte(payload), &row); err != nil {
		t.Fatalf("decode insert: %v", err)
	}
	if row.TraceID != "trace-1" {
		t.Fatalf("expected trace_id trace-1, got %s", row.TraceID)
	}
}
