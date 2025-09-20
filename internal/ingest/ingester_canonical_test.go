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
		case strings.Contains(q, "INSERT INTO contracts"):
			calls["contracts"]++
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
	type txRow struct {
		InputMethod string `json:"input_method"`
		ValueRaw    string `json:"value_raw"`
		IsInternal  uint8  `json:"is_internal"`
	}
	var external *txRow
	for _, line := range strings.Split(strings.TrimSpace(payload), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var candidate txRow
		if err := json.Unmarshal([]byte(line), &candidate); err != nil {
			t.Fatalf("decode insert: %v", err)
		}
		if candidate.IsInternal == 0 {
			external = &candidate
			break
		}
	}
	if external == nil {
		t.Fatal("expected external transaction row")
	}
	if external.InputMethod != "transfer" {
		t.Fatalf("input_method=%s", external.InputMethod)
	}
	if external.ValueRaw != "1" {
		t.Fatalf("value_raw=%s", external.ValueRaw)
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

type provCanonInternalOnly struct{}

func (provCanonInternalOnly) BlockNumber(ctx context.Context) (uint64, error) { return 1, nil }
func (provCanonInternalOnly) BlockTimestamp(ctx context.Context, block uint64) (int64, error) {
	return 2_000, nil
}
func (provCanonInternalOnly) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	return []eth.Trace{{
		TxHash:   "0xABCDEF",
		TraceID:  "0-1",
		From:     address,
		To:       "0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
		ValueWei: "0x5",
		BlockNum: from,
		TsMillis: 0,
	}}, nil
}
func (provCanonInternalOnly) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	return nil, nil
}
func (provCanonInternalOnly) Transactions(ctx context.Context, address string, from, to uint64) ([]eth.Transaction, error) {
	return nil, nil
}

func TestProcessRange_CanonicalInternalTraceTransactions(t *testing.T) {
	const addr = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	ing := NewWithProvider(addr, Options{Schema: "canonical", ClickHouseDSN: "http://localhost:8123/db"}, provCanonInternalOnly{})
	var rows []map[string]any
	ing.ch.SetTransport(rtFunc(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query().Get("query")
		if strings.Contains(q, "INSERT INTO transactions") {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			payload := strings.TrimSpace(string(body))
			for _, line := range strings.Split(payload, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				var m map[string]any
				if err := json.Unmarshal([]byte(line), &m); err != nil {
					t.Fatalf("decode insert: %v", err)
				}
				rows = append(rows, m)
			}
		}
		return &http.Response{StatusCode: 200, Body: ioNopCloser("ok")}, nil
	}))
	if err := ing.processRange(context.Background(), 1, 1); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 transaction row, got %d", len(rows))
	}
	row := rows[0]
	if isInternal, ok := row["is_internal"].(float64); !ok || isInternal != 1 {
		t.Fatalf("expected is_internal 1, got %v", row["is_internal"])
	}
	traceID, ok := row["trace_id"].(string)
	if !ok || traceID != "0-1" {
		t.Fatalf("expected trace_id 0-1, got %v", row["trace_id"])
	}
	if txHash, ok := row["tx_hash"].(string); !ok || txHash != strings.ToLower("0xABCDEF") {
		t.Fatalf("unexpected tx_hash %v", row["tx_hash"])
	}
}

func TestNormalizeInternalTracesForAddress(t *testing.T) {
	traces := []eth.Trace{
		{TxHash: "0x1", TraceID: "root", From: "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", To: "0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB", ValueWei: "0x1", BlockNum: 10, TsMillis: 1_000},
		{TxHash: "0x2", TraceID: "0-1", From: "0xCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC", To: "0xDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD", ValueWei: "0x2", BlockNum: 11, TsMillis: 2_000},
	}
	rows := normalizeInternalTracesForAddress(traces, "")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row without filter, got %d", len(rows))
	}
	row := rows[0]
	if row.IsInternal != 1 {
		t.Fatalf("expected is_internal flag set, got %d", row.IsInternal)
	}
	if row.TraceID == "root" {
		t.Fatalf("expected non-root trace, got %s", row.TraceID)
	}
	target := "0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	filtered := normalizeInternalTracesForAddress(traces, target)
	if len(filtered) != 0 {
		t.Fatalf("expected 0 rows after filtering root trace, got %d", len(filtered))
	}
	otherTarget := "0xDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD"
	filtered = normalizeInternalTracesForAddress(traces, otherTarget)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 row for non-root trace, got %d", len(filtered))
	}
	if filtered[0].TraceID != "0-1" {
		t.Fatalf("unexpected trace id %s", filtered[0].TraceID)
	}
}

type provCanonFilterTx struct{}

func (provCanonFilterTx) BlockNumber(ctx context.Context) (uint64, error) { return 1, nil }
func (provCanonFilterTx) BlockTimestamp(ctx context.Context, block uint64) (int64, error) {
	return 5_000, nil
}
func (provCanonFilterTx) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	return nil, nil
}
func (provCanonFilterTx) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	return nil, nil
}
func (provCanonFilterTx) Transactions(ctx context.Context, address string, from, to uint64) ([]eth.Transaction, error) {
	return []eth.Transaction{
		{
			Hash:     "0xaaa",
			From:     address,
			To:       "0x1111111111111111111111111111111111111111",
			ValueWei: "0x1",
			Status:   1,
			GasUsed:  21_000,
			InputHex: "0x095ea7b3",
			BlockNum: from,
			TsMillis: 5_000,
		},
		{
			Hash:     "0xbbb",
			From:     "0x9999999999999999999999999999999999999999",
			To:       "0x8888888888888888888888888888888888888888",
			ValueWei: "0x2",
			Status:   1,
			GasUsed:  30_000,
			InputHex: "0xdeadbeef",
			BlockNum: from,
			TsMillis: 5_000,
		},
	}, nil
}

func TestProcessRange_CanonicalTransactionsFilterByAddress(t *testing.T) {
	const addr = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	ing := NewWithProvider(addr, Options{Schema: "canonical", ClickHouseDSN: "http://localhost:8123/db"}, provCanonFilterTx{})
	var rows []map[string]any
	ing.ch.SetTransport(rtFunc(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query().Get("query")
		if strings.Contains(q, "INSERT INTO transactions") {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			payload := strings.TrimSpace(string(body))
			for _, line := range strings.Split(payload, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				var m map[string]any
				if err := json.Unmarshal([]byte(line), &m); err != nil {
					t.Fatalf("decode insert: %v", err)
				}
				rows = append(rows, m)
			}
		}
		return &http.Response{StatusCode: 200, Body: ioNopCloser("ok")}, nil
	}))
	if err := ing.processRange(context.Background(), 1, 1); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 transaction row, got %d", len(rows))
	}
	row := rows[0]
	fromVal, ok := row["from_addr"].(string)
	if !ok {
		t.Fatalf("from_addr missing or not a string: %+v", row)
	}
	if fromVal != strings.ToLower(addr) {
		t.Fatalf("unexpected from_addr %s", fromVal)
	}
	if method, ok := row["input_method"].(string); !ok || method != "approve" {
		t.Fatalf("expected input_method approve, got %v", row["input_method"])
	}
}

type provCanonContractTx struct{}

func (provCanonContractTx) BlockNumber(ctx context.Context) (uint64, error) { return 1, nil }
func (provCanonContractTx) BlockTimestamp(ctx context.Context, block uint64) (int64, error) {
	return 1_000, nil
}
func (provCanonContractTx) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	return nil, nil
}
func (provCanonContractTx) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	return nil, nil
}
func (provCanonContractTx) Transactions(ctx context.Context, address string, from, to uint64) ([]eth.Transaction, error) {
	return []eth.Transaction{{
		Hash:            "0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		From:            address,
		To:              "",
		ValueWei:        "0x0",
		Status:          1,
		BlockNum:        from,
		TsMillis:        1_000,
		ContractAddress: "0x1234567890abcdef1234567890abcdef12345678",
	}}, nil
}

func TestProcessRange_CanonicalContractsFromTransaction(t *testing.T) {
	const addr = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	var payload string
	ing := NewWithProvider(addr, Options{Schema: "canonical", ClickHouseDSN: "http://localhost:8123/db"}, provCanonContractTx{})
	ing.ch.SetTransport(rtFunc(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query().Get("query")
		if strings.Contains(q, "INSERT INTO contracts") {
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
		t.Fatal("expected contracts insert payload")
	}
	var rows []string
	for _, line := range strings.Split(payload, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rows = append(rows, line)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 contracts row, got %d", len(rows))
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(rows[0]), &row); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if got, ok := row["address"].(string); !ok || got != "0x1234567890abcdef1234567890abcdef12345678" {
		t.Fatalf("unexpected contract address %v", row["address"])
	}
	if got, ok := row["created_at_tx"].(string); !ok || got != "0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef" {
		t.Fatalf("unexpected created_at_tx %v", row["created_at_tx"])
	}
	if got, ok := row["first_seen_block"].(float64); !ok || got != 1 {
		t.Fatalf("unexpected first_seen_block %v", row["first_seen_block"])
	}
	if got, ok := row["is_contract"].(float64); !ok || got != 1 {
		t.Fatalf("unexpected is_contract %v", row["is_contract"])
	}
}

type provCanonContractTrace struct{}

func (provCanonContractTrace) BlockNumber(ctx context.Context) (uint64, error) { return 1, nil }
func (provCanonContractTrace) BlockTimestamp(ctx context.Context, block uint64) (int64, error) {
	return 2_000, nil
}
func (provCanonContractTrace) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	return []eth.Trace{{
		TxHash:          "0xbeefbeefbeefbeefbeefbeefbeefbeefbeefbeefbeefbeefbeefbeefbeefbeef",
		TraceID:         "0-0",
		From:            address,
		To:              "",
		ValueWei:        "0x0",
		BlockNum:        from,
		TsMillis:        2_000,
		Type:            "create",
		CreatedContract: "0xcafebabecafebabecafebabecafebabecafebabe",
	}}, nil
}
func (provCanonContractTrace) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	return nil, nil
}
func (provCanonContractTrace) Transactions(ctx context.Context, address string, from, to uint64) ([]eth.Transaction, error) {
	return nil, nil
}

func TestProcessRange_CanonicalContractsFromTrace(t *testing.T) {
	const addr = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	var payload string
	ing := NewWithProvider(addr, Options{Schema: "canonical", ClickHouseDSN: "http://localhost:8123/db"}, provCanonContractTrace{})
	ing.ch.SetTransport(rtFunc(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query().Get("query")
		if strings.Contains(q, "INSERT INTO contracts") {
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
		t.Fatal("expected contracts insert payload")
	}
	var rows []string
	for _, line := range strings.Split(payload, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rows = append(rows, line)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 contracts row, got %d", len(rows))
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(rows[0]), &row); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if got, ok := row["address"].(string); !ok || got != "0xcafebabecafebabecafebabecafebabecafebabe" {
		t.Fatalf("unexpected contract address %v", row["address"])
	}
	if got, ok := row["created_at_tx"].(string); !ok || got != "0xbeefbeefbeefbeefbeefbeefbeefbeefbeefbeefbeefbeefbeefbeefbeefbeef" {
		t.Fatalf("unexpected created_at_tx %v", row["created_at_tx"])
	}
	if got, ok := row["first_seen_block"].(float64); !ok || got != 1 {
		t.Fatalf("unexpected first_seen_block %v", row["first_seen_block"])
	}
	if got, ok := row["is_contract"].(float64); !ok || got != 1 {
		t.Fatalf("unexpected is_contract %v", row["is_contract"])
	}
}

func TestProcessRange_CanonicalContractsInsertError(t *testing.T) {
	const addr = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	ing := NewWithProvider(addr, Options{Schema: "canonical", ClickHouseDSN: "http://localhost:8123/db"}, provCanonContractTx{})
	ing.ch.SetTransport(rtFunc(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query().Get("query")
		if strings.Contains(q, "INSERT INTO contracts") {
			return &http.Response{StatusCode: 500, Body: ioNopCloser("boom")}, nil
		}
		return &http.Response{StatusCode: 200, Body: ioNopCloser("ok")}, nil
	}))
	if err := ing.processRange(context.Background(), 1, 1); err == nil || !strings.Contains(err.Error(), "inserting contracts") {
		t.Fatalf("expected contracts insert error, got %v", err)
	}
}
