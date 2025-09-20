package eth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/AIAleph/mvp_wallet_context/internal/logging"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func mkResp(v any) *http.Response {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": v})
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b)), Header: http.Header{"Content-Type": []string{"application/json"}}}
}

func mkRespErr(code int, msg string) *http.Response {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "error": map[string]any{"code": code, "message": msg}})
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b)), Header: http.Header{"Content-Type": []string{"application/json"}}}
}

func TestHTTPProvider_BlockNumber(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "eth_blockNumber":
			return mkResp("0x2a"), nil
		default:
			return mkResp(nil), nil
		}
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	// Speed up retries in tests
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
	}
	n, err := p.BlockNumber(context.Background())
	if err != nil || n != 42 {
		t.Fatalf("bn=%d err=%v", n, err)
	}
}

func TestHTTPProvider_GetLogs(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "eth_getLogs":
			logs := []map[string]any{{
				"transactionHash": "0xabc",
				"logIndex":        "0x1",
				"address":         "0xdead",
				"topics":          []string{"0x01"},
				"data":            "0x",
				"blockNumber":     "0x10",
			}}
			return mkResp(logs), nil
		case "eth_getBlockByNumber":
			// block 0x10 -> timestamp 0x64 (100 seconds)
			return mkResp(map[string]any{"timestamp": "0x64"}), nil
		default:
			return mkResp(nil), nil
		}
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
	}
	out, err := p.GetLogs(context.Background(), "0xdead", 1, 2, [][]string{{"0x01"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].TxHash != "0xabc" || out[0].Index != 1 || out[0].BlockNum != 16 || out[0].TsMillis != 100000 {
		t.Fatalf("unexpected logs: %+v", out)
	}
}

func TestHTTPProvider_TraceFilterPaging(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "trace_filter":
			// extract 'after'
			params := req["params"].([]any)
			obj := params[0].(map[string]any)
			after := int(obj["after"].(float64))
			if after == 0 {
				traces := []map[string]any{
					{
						"transactionHash": "0x1",
						"blockNumber":     "0x10",
						"traceAddress":    []int{},
						"action": map[string]any{
							"from":  "0xfrom1",
							"to":    "0xto1",
							"value": "0x1",
						},
					},
					{
						"transactionHash": "0x2",
						"blockNumber":     "0x11",
						"traceAddress":    []int{0, 1},
						"action": map[string]any{
							"from":  "0xfrom2",
							"to":    "0xto2",
							"value": "0x2",
						},
					},
				}
				return mkResp(traces), nil
			}
			return mkResp([]any{}), nil
		case "eth_getBlockByNumber":
			// Return timestamps 0x64 and 0x65 when called
			return mkResp(map[string]any{"timestamp": "0x64"}), nil
		default:
			return mkResp(nil), nil
		}
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
	}
	out, err := p.TraceBlock(context.Background(), 1, 100, "0xdead")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 traces, got %d", len(out))
	}
	if out[0].TraceID != "root" || out[1].TraceID != "0-1" {
		t.Fatalf("unexpected trace ids: %+v", out)
	}
	if out[0].TsMillis == 0 || out[1].TsMillis == 0 {
		t.Fatalf("timestamps not enriched: %+v", out)
	}

}

func TestHTTPProvider_BlockTimestampDirect(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] == "eth_getBlockByNumber" {
			return mkResp(map[string]any{"timestamp": "0x2a"}), nil // 42s
		}
		return mkResp(nil), nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
	}
	ts, err := p.BlockTimestamp(context.Background(), 100)
	if err != nil || ts != 42000 {
		t.Fatalf("ts=%d err=%v", ts, err)
	}
}

func TestHTTPProvider_RpcErrorAndNoRetryOn400(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		// First: return RPC error payload
		if calls == 1 {
			b := []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"oops"}}`)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b)), Header: http.Header{"Content-Type": []string{"application/json"}}}, nil
		}
		// Second: return 400 (should not retry)
		return &http.Response{StatusCode: 400, Body: io.NopCloser(bytes.NewReader([]byte("bad")))}, nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
		hp.maxRetries = 2
	}
	// RPC error should surface immediately
	if _, err := p.BlockNumber(context.Background()); err == nil {
		t.Fatal("expected rpc error")
	}
	// 400 should not retry
	calls = 0
	if _, err := p.BlockNumber(context.Background()); err == nil || calls != 1 {
		t.Fatalf("expected single 400 attempt, calls=%d err=%v", calls, err)
	}
}

func TestHTTPProvider_BlockTimestampCache(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] == "eth_getBlockByNumber" {
			calls++
			return mkResp(map[string]any{"timestamp": "0x64"}), nil
		}
		return mkResp(nil), nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
	}
	// Two calls for same block should hit cache second time
	for i := 0; i < 2; i++ {
		_, _ = p.BlockTimestamp(context.Background(), 16)
	}
	if calls != 1 {
		t.Fatalf("expected 1 rpc call, got %d", calls)
	}
}

func TestHTTPProvider_429RetryThenSuccess(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls < 3 {
			return &http.Response{StatusCode: 429, Body: io.NopCloser(bytes.NewReader([]byte("rate")))}, nil
		}
		return mkResp("0x2a"), nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
		hp.maxRetries = 3
	}
	n, err := p.BlockNumber(context.Background())
	if err != nil || n != 42 || calls != 3 {
		t.Fatalf("n=%d err=%v calls=%d", n, err, calls)
	}
}

func TestHTTPProvider_TransactionsFiltersAndReceipts(t *testing.T) {
	addr := "0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "eth_getBlockByNumber":
			params := req["params"].([]any)
			if params[0] != "0x10" {
				t.Fatalf("unexpected block param: %+v", params[0])
			}
			to := strings.ToUpper(addr)
			txs := []map[string]any{
				{
					"hash":  "0xhash1",
					"from":  strings.ToUpper(addr),
					"to":    "0x1111111111111111111111111111111111111111",
					"input": "0xa9059cbb0000000000000000000000000000000000000000000000000000000000000001",
					"value": "0xde",
				},
				{
					"hash":  "0xhash2",
					"from":  "0x2222222222222222222222222222222222222222",
					"to":    to,
					"input": "0x",
					"value": "0x0",
				},
				{
					"hash":  "0xhash3",
					"from":  "0x3333333333333333333333333333333333333333",
					"to":    "0x4444444444444444444444444444444444444444",
					"input": "0x",
					"value": "0x0",
				},
			}
			return mkResp(map[string]any{
				"timestamp":    "0x64",
				"transactions": txs,
			}), nil
		case "eth_getTransactionReceipt":
			params := req["params"].([]any)
			hash := params[0].(string)
			receipts := map[string]map[string]any{
				"0xhash1": {"status": "0x1", "gasUsed": "0x5208"},
				"0xhash2": {"status": "0x0", "gasUsed": "0x0"},
			}
			if rec, ok := receipts[hash]; ok {
				return mkResp(rec), nil
			}
			t.Fatalf("unexpected receipt for hash %s", hash)
		}
		return mkResp(nil), nil
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
	}
	out, err := p.Transactions(context.Background(), addr, 16, 16)
	if err != nil {
		t.Fatalf("transactions err: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(out))
	}
	if out[0].GasUsed != 21000 || out[0].Status != 1 {
		t.Fatalf("unexpected receipt values: %+v", out[0])
	}
	if out[0].TsMillis != 100000 {
		t.Fatalf("timestamp mismatch: %d", out[0].TsMillis)
	}
	if out[0].From != strings.ToLower(addr) {
		t.Fatalf("from lower-case mismatch: %s", out[0].From)
	}
	if out[1].To != strings.ToLower(addr) {
		t.Fatalf("to lower-case mismatch: %s", out[1].To)
	}
	if out[0].InputHex == "" {
		t.Fatalf("expected input preserved")
	}
}

func TestNewHTTPProvider_EmptyEndpointAndDefaultClient(t *testing.T) {
	if _, err := NewHTTPProvider("", nil); err == nil {
		t.Fatal("expected error for empty endpoint")
	}
	if p, err := NewHTTPProvider("http://unit-test", nil); err != nil || p == nil {
		t.Fatalf("new http provider err=%v", err)
	}
}

func TestTraceBlockZeroResults(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] == "trace_filter" {
			return mkResp([]any{}), nil
		}
		return mkResp(nil), nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
	}
	out, err := p.TraceBlock(context.Background(), 1, 10, "0xdead")
	if err != nil || len(out) != 0 {
		t.Fatalf("out=%v err=%v", out, err)
	}
}

func TestHTTPProvider_CallOutNilAndDecodeError(t *testing.T) {
	// Success with out == nil
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		return mkResp("0x1"), nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	hp := p.(*httpProvider)
	if err := hp.call(context.Background(), "eth_blockNumber", []any{}, nil); err != nil {
		t.Fatalf("call out=nil err: %v", err)
	}
	// Decode error
	client2 := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte("{")))}, nil
	})}
	p2, _ := NewHTTPProvider("http://unit-test", client2)
	hp2 := p2.(*httpProvider)
	if err := hp2.call(context.Background(), "eth_blockNumber", []any{}, nil); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestHTTPProvider_BlockNumberInvalidHex(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		return mkResp("0xzz"), nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	if _, err := p.BlockNumber(context.Background()); err == nil {
		t.Fatal("expected hex parse error")
	}
}

func TestHTTPProvider_CallUnmarshalError(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		// result is a string; try to unmarshal into number
		return mkResp("hello"), nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	hp := p.(*httpProvider)
	var out int
	if err := hp.call(context.Background(), "eth_blockNumber", []any{}, &out); err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestHTTPProvider_GetLogsTopicsVariationsAndTsError(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "eth_getLogs":
			logs := []map[string]any{{
				"transactionHash": "0x1", "logIndex": "0x0", "address": "0xdead", "topics": []string{"0x01"}, "data": "0x", "blockNumber": "0x10",
			}, {
				"transactionHash": "0x2", "logIndex": "0x1", "address": "0xdead", "topics": []string{"0x01", "0x02"}, "data": "0x", "blockNumber": "0x11",
			}}
			return mkResp(logs), nil
		case "eth_getBlockByNumber":
			// Return invalid ts for block 0x10, valid for 0x11 to make outcome deterministic
			params := req["params"].([]any)
			blk := params[0].(string)
			if blk == "0x10" {
				return mkResp(map[string]any{"timestamp": "0xzz"}), nil
			}
			return mkResp(map[string]any{"timestamp": "0x64"}), nil
		default:
			return mkResp(nil), nil
		}
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	out, err := p.GetLogs(context.Background(), "0xdead", 1, 2, [][]string{{}, {"0x01", "0x02"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(out))
	}
	// First ts remains 0 due to error; second enriched
	if out[0].TsMillis != 0 || out[1].TsMillis == 0 {
		t.Fatalf("ts enrichment mismatch: %+v", out)
	}
}

func TestHTTPProvider_RetryBackoff(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] != "eth_blockNumber" {
			return mkResp(nil), nil
		}
		calls++
		if calls < 3 {
			// Simulate 500s twice, then success
			return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader([]byte("oops")))}, nil
		}
		return mkResp("0x2a"), nil
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
		hp.maxRetries = 5
	}
	n, err := p.BlockNumber(context.Background())
	if err != nil || n != 42 {
		t.Fatalf("bn=%d err=%v", n, err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestHTTPProvider_DoErrorThenSuccess(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return nil, io.ErrUnexpectedEOF
		}
		return mkResp("0x2a"), nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
		hp.maxRetries = 2
	}
	n, err := p.BlockNumber(context.Background())
	if err != nil || n != 42 || calls != 2 {
		t.Fatalf("n=%d err=%v calls=%d", n, err, calls)
	}
}

func TestHTTPProvider_TransactionsEmptyRange(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected call: %s", r.URL)
		return nil, nil
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	txs, err := p.Transactions(context.Background(), "0xabc", 5, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(txs) != 0 {
		t.Fatalf("expected no transactions, got %d", len(txs))
	}
}

func TestHTTPProvider_TransactionsSuccessVariants(t *testing.T) {
	const target = "0xAbCdEf00000000000000000000000000000000"
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "eth_getBlockByNumber":
			block := map[string]any{
				"timestamp": "0x64",
				"transactions": []map[string]any{
					{
						"hash":  "0xaaa",
						"from":  strings.ToUpper(target),
						"to":    "0x1111111111111111111111111111111111111111",
						"input": "0xa9059cbb00000000000000000000000000000000000000000000000000000064",
						"value": "0xde",
					},
					{
						"hash":  "0xbbb",
						"from":  "0x2222222222222222222222222222222222222222",
						"to":    strings.ToUpper(target),
						"input": "0x",
						"value": "0x1",
					},
					{
						"hash":  "0xccc",
						"from":  strings.ToUpper(target),
						"to":    nil,
						"input": "0xabcdef0123456789",
						"value": "0x2",
					},
					{
						"hash":  "0xddd",
						"from":  "0x3333333333333333333333333333333333333333",
						"to":    "0x4444444444444444444444444444444444444444",
						"input": "0x",
						"value": "0x3",
					},
				},
			}
			return mkResp(block), nil
		case "eth_getBlockReceipts":
			return mkResp([]map[string]any{
				{"transactionHash": "0xaaa", "status": "0x0", "gasUsed": "0x5208"},
				{"transactionHash": "0xbbb", "status": "", "gasUsed": "0x5209"},
				{"transactionHash": "0xccc", "status": "0x1", "gasUsed": "0x1"},
			}), nil
		case "eth_getTransactionReceipt":
			t.Fatalf("unexpected per-transaction receipt call")
			return nil, nil
		default:
			return mkResp(nil), nil
		}
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
	}
	prev := logging.Logger()
	logging.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer logging.SetLogger(prev)
	txs, err := p.Transactions(context.Background(), target, 10, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(txs) != 3 {
		t.Fatalf("expected 3 transactions, got %d", len(txs))
	}
	lowTarget := strings.ToLower(target)
	m := make(map[string]Transaction)
	for _, tx := range txs {
		m[tx.Hash] = tx
		if tx.TsMillis != 100000 {
			t.Fatalf("timestamp mismatch for %s: %d", tx.Hash, tx.TsMillis)
		}
	}
	if tx := m["0xaaa"]; tx.From != lowTarget || tx.Status != 0 || tx.GasUsed != 21000 {
		t.Fatalf("tx 0xaaa mismatch: %+v", tx)
	}
	if tx := m["0xbbb"]; tx.To != lowTarget || tx.Status != 1 || tx.GasUsed != 21001 {
		t.Fatalf("tx 0xbbb mismatch: %+v", tx)
	}
	if tx := m["0xccc"]; tx.To != "" || tx.Status != 1 || tx.GasUsed != 1 {
		t.Fatalf("tx 0xccc mismatch: %+v", tx)
	}
}

func TestHTTPProvider_TransactionsFallbackToPerReceipt(t *testing.T) {
	const target = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	receipts := map[string]map[string]any{
		"0xaaa": {"status": "0x1", "gasUsed": "0x5208"},
		"0xbbb": {"status": "0x1", "gasUsed": "0x5209"},
	}
	blockReceiptCalls := 0
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "eth_getBlockByNumber":
			block := map[string]any{
				"timestamp": "0x64",
				"transactions": []map[string]any{
					{"hash": "0xaaa", "from": target, "to": target, "input": "0x", "value": "0x1"},
					{"hash": "0xbbb", "from": "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "to": target, "input": "0x", "value": "0x2"},
				},
			}
			return mkResp(block), nil
		case "eth_getBlockReceipts":
			blockReceiptCalls++
			return mkRespErr(-32601, "method not found"), nil
		case "eth_getTransactionReceipt":
			params := req["params"].([]any)
			hash := params[0].(string)
			rec, ok := receipts[hash]
			if !ok {
				t.Fatalf("unexpected receipt request: %s", hash)
				return nil, nil
			}
			return mkResp(rec), nil
		default:
			return mkResp(nil), nil
		}
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
		hp.receiptWorkers = 2
	}
	prev := logging.Logger()
	logging.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer logging.SetLogger(prev)

	txs, err := p.Transactions(context.Background(), target, 10, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(txs) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(txs))
	}
	if blockReceiptCalls != 1 {
		t.Fatalf("expected single block receipt attempt, got %d", blockReceiptCalls)
	}

	txs2, err := p.Transactions(context.Background(), target, 10, 10)
	if err != nil {
		t.Fatalf("second call unexpected error: %v", err)
	}
	if len(txs2) != 2 {
		t.Fatalf("expected 2 transactions on second call, got %d", len(txs2))
	}
	if blockReceiptCalls != 1 {
		t.Fatalf("expected no additional block receipt attempts, got %d", blockReceiptCalls)
	}
}

func TestHTTPProvider_TransactionsSkipsMissingReceipts(t *testing.T) {
	const target = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "eth_getBlockByNumber":
			block := map[string]any{
				"timestamp": "0x64",
				"transactions": []map[string]any{
					{"hash": "0xaaa", "from": target, "to": target, "input": "0x", "value": "0x1"},
					{"hash": "0xbbb", "from": target, "to": "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "input": "0x", "value": "0x2"},
				},
			}
			return mkResp(block), nil
		case "eth_getBlockReceipts":
			return mkResp([]map[string]any{
				{"transactionHash": "0xaaa", "status": "0x1", "gasUsed": "0x5208"},
			}), nil
		default:
			return mkResp(nil), nil
		}
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
	}
	prev := logging.Logger()
	logging.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer logging.SetLogger(prev)

	txs, err := p.Transactions(context.Background(), target, 10, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(txs) != 1 {
		t.Fatalf("expected 1 transaction with available receipt, got %d", len(txs))
	}
	if txs[0].Hash != "0xaaa" {
		t.Fatalf("expected only tx 0xaaa, got %+v", txs[0])
	}
}

func TestHTTPProvider_TransactionsContextCancellation(t *testing.T) {
	const target = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	ctx, cancel := context.WithCancel(context.Background())
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "eth_getBlockByNumber":
			params := req["params"].([]any)
			blk := params[0].(string)
			if blk == "0x1" {
				block := map[string]any{
					"timestamp": "0x64",
					"transactions": []map[string]any{
						{"hash": "0xaaa", "from": target, "to": target, "input": "0x", "value": "0x1"},
					},
				}
				return mkResp(block), nil
			}
			if blk == "0x2" {
				return mkResp(map[string]any{
					"timestamp":    "0x65",
					"transactions": []map[string]any{{"hash": "0xbbb", "from": target, "to": target, "input": "0x", "value": "0x2"}},
				}), nil
			}
		case "eth_getTransactionReceipt":
			params := req["params"].([]any)
			hash := params[0].(string)
			if hash == "0xaaa" {
				cancel()
				return mkResp(map[string]any{"status": "0x1", "gasUsed": "0x5208"}), nil
			}
			return mkResp(map[string]any{"status": "0x1", "gasUsed": "0x5209"}), nil
		}
		return mkResp(nil), nil
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
	}
	prev := logging.Logger()
	logging.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer logging.SetLogger(prev)

	txs, err := p.Transactions(ctx, target, 1, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(txs) != 1 {
		t.Fatalf("expected 1 transaction before cancellation, got %d", len(txs))
	}
	if txs[0].Hash != "0xaaa" {
		t.Fatalf("unexpected transaction %+v", txs[0])
	}
}

func TestHTTPProvider_TransactionsBlockError(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] == "eth_getBlockByNumber" {
			return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader([]byte("oops")))}, nil
		}
		return mkResp(nil), nil
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Transactions(context.Background(), "0xbeef", 1, 1); err == nil {
		t.Fatal("expected block call error")
	}
}

func TestHTTPProvider_TransactionsReceiptError(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "eth_getBlockByNumber":
			block := map[string]any{
				"timestamp": "0x1",
				"transactions": []map[string]any{
					{
						"hash":  "0xaaa",
						"from":  "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
						"to":    "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
						"input": "0x",
						"value": "0x1",
					},
				},
			}
			return mkResp(block), nil
		case "eth_getTransactionReceipt":
			return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader([]byte("bad")))}, nil
		default:
			return mkResp(nil), nil
		}
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	prev := logging.Logger()
	logging.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer logging.SetLogger(prev)
	if _, err := p.Transactions(context.Background(), "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 1, 1); err == nil {
		t.Fatal("expected receipt call error")
	}
}

func TestHTTPProvider_BlockReceiptsGasHexError(t *testing.T) {
	const target = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "eth_getBlockByNumber":
			block := map[string]any{
				"timestamp": "0x64",
				"transactions": []map[string]any{
					{"hash": "0xaaa", "from": target, "to": target, "input": "0x", "value": "0x1"},
					{"hash": "0xbbb", "from": target, "to": target, "input": "0x", "value": "0x2"},
				},
			}
			return mkResp(block), nil
		case "eth_getBlockReceipts":
			return mkResp([]map[string]any{
				{"transactionHash": "0xaaa", "status": "0x1", "gasUsed": "0xzz"},
				{"transactionHash": "0xbbb", "status": "0x1", "gasUsed": "0x5208"},
			}), nil
		default:
			return mkResp(nil), nil
		}
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
	}
	prev := logging.Logger()
	logging.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer logging.SetLogger(prev)
	if _, err := p.Transactions(context.Background(), target, 10, 10); err == nil {
		t.Fatal("expected gasUsed hex parse error")
	}
}

func TestHTTPProvider_BlockReceiptsStatusHexError(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] == "eth_getBlockReceipts" {
			return mkResp([]map[string]any{{"transactionHash": "0xaaa", "status": "0xzz", "gasUsed": "0x5208"}}), nil
		}
		return mkResp(nil), nil
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	hp := p.(*httpProvider)
	if _, err := hp.callBlockReceipts(context.Background(), 10, map[string]struct{}{"0xaaa": {}}); err == nil {
		t.Fatal("expected status hex parse error")
	}
}

func TestHTTPProvider_TransactionsSingleMatchUsesPerReceipt(t *testing.T) {
	const target = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	receiptCalls := 0
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "eth_getBlockByNumber":
			block := map[string]any{
				"timestamp":    "0x64",
				"transactions": []map[string]any{{"hash": "0xaaa", "from": target, "to": target, "input": "0x", "value": "0x1"}},
			}
			return mkResp(block), nil
		case "eth_getBlockReceipts":
			t.Fatal("block receipts should not be called for single match")
		case "eth_getTransactionReceipt":
			receiptCalls++
			return mkResp(map[string]any{"status": "0x1", "gasUsed": "0x5208"}), nil
		}
		return mkResp(nil), nil
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
	}
	txs, err := p.Transactions(context.Background(), target, 10, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(txs) != 1 {
		t.Fatalf("expected 1 tx, got %d", len(txs))
	}
	if receiptCalls != 1 {
		t.Fatalf("expected 1 receipt call, got %d", receiptCalls)
	}
}

func TestHTTPProvider_TransactionsMaxUint64(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] == "eth_getBlockByNumber" {
			params := req["params"].([]any)
			if params[0] != "0xffffffffffffffff" {
				t.Fatalf("unexpected block param: %v", params[0])
			}
			return mkResp(map[string]any{"timestamp": "0x64", "transactions": []map[string]any{}}), nil
		}
		return mkResp(nil), nil
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
	}
	txs, err := p.Transactions(context.Background(), "0xabc", math.MaxUint64, math.MaxUint64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(txs) != 0 {
		t.Fatalf("expected no transactions, got %d", len(txs))
	}
}

func TestHTTPProvider_TransactionsPartialBlockFailure(t *testing.T) {
	const target = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "eth_getBlockByNumber":
			params := req["params"].([]any)
			blk := params[0].(string)
			if blk == "0xa" {
				return mkResp(map[string]any{
					"timestamp":    "0x64",
					"transactions": []map[string]any{{"hash": "0xaaa", "from": target, "to": target, "input": "0x", "value": "0x1"}},
				}), nil
			}
			return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader([]byte("boom")))}, nil
		case "eth_getTransactionReceipt":
			return mkResp(map[string]any{"status": "0x1", "gasUsed": "0x5208"}), nil
		}
		return mkResp(nil), nil
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
	}
	txs, err := p.Transactions(context.Background(), target, 10, 11)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(txs) != 1 {
		t.Fatalf("expected partial success, got %d", len(txs))
	}
}

func TestHTTPProvider_CallBlockReceiptsNoFilter(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] == "eth_getBlockReceipts" {
			return mkResp([]map[string]any{{"transactionHash": "0xaaa", "status": "0x1", "gasUsed": "0x5208"}}), nil
		}
		return mkResp(nil), nil
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	hp := p.(*httpProvider)
	recs, callErr := hp.callBlockReceipts(context.Background(), 10, nil)
	if callErr != nil {
		t.Fatalf("unexpected error: %v", callErr)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 receipt, got %d", len(recs))
	}
	if recs["0xaaa"].gasUsed != 0x5208 {
		t.Fatalf("unexpected gas value: %+v", recs["0xaaa"])
	}
}

func TestHTTPProvider_FetchReceiptsForBlockAggregatesErrors(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "eth_getBlockReceipts":
			return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader([]byte("oops")))}, nil
		case "eth_getTransactionReceipt":
			return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader([]byte("bad")))}, nil
		default:
			return mkResp(nil), nil
		}
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	hp := p.(*httpProvider)
	recs, calls, failures, ferr := hp.fetchReceiptsForBlock(context.Background(), 10, []string{"0xaaa", "0xbbb"})
	if len(recs) != 0 {
		t.Fatalf("expected no receipts, got %d", len(recs))
	}
	if calls == 0 {
		t.Fatalf("expected at least one call, got %d", calls)
	}
	if failures != 2 {
		t.Fatalf("failures=%d want 2", failures)
	}
	if ferr == nil {
		t.Fatal("expected aggregated error")
	}
}

func TestHTTPProvider_TransactionsBlockErrorAtMax(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] == "eth_getBlockByNumber" {
			return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader([]byte("boom")))}, nil
		}
		return mkResp(nil), nil
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Transactions(context.Background(), "0xabc", math.MaxUint64, math.MaxUint64); err == nil {
		t.Fatal("expected error for max block failure")
	}
}

func TestHTTPProvider_TransactionsTimestampErrorAtMax(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] == "eth_getBlockByNumber" {
			return mkResp(map[string]any{"timestamp": "0xzz", "transactions": []map[string]any{}}), nil
		}
		return mkResp(nil), nil
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	prev := logging.Logger()
	logging.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer logging.SetLogger(prev)
	if _, err := p.Transactions(context.Background(), "0xabc", math.MaxUint64, math.MaxUint64); err == nil {
		t.Fatal("expected timestamp parse error")
	}
}

func TestHTTPProvider_TransactionsMaxUint64WithTx(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "eth_getBlockByNumber":
			return mkResp(map[string]any{
				"timestamp":    "0x64",
				"transactions": []map[string]any{{"hash": "0xaaa", "from": "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "to": "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "input": "0x", "value": "0x1"}},
			}), nil
		case "eth_getTransactionReceipt":
			return mkResp(map[string]any{"status": "0x1", "gasUsed": "0x5208"}), nil
		default:
			return mkResp(nil), nil
		}
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	txs, err := p.Transactions(context.Background(), "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", math.MaxUint64, math.MaxUint64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(txs) != 1 {
		t.Fatalf("expected 1 tx, got %d", len(txs))
	}
}

func TestHTTPProvider_FetchReceiptsForBlockEmpty(t *testing.T) {
	hp := &httpProvider{}
	recs, calls, failures, err := hp.fetchReceiptsForBlock(context.Background(), 10, nil)
	if len(recs) != 0 || calls != 0 || failures != 0 || err != nil {
		t.Fatalf("unexpected result: recs=%v calls=%d failures=%d err=%v", recs, calls, failures, err)
	}
}

func TestHTTPProvider_FetchReceiptsIndividuallyEmpty(t *testing.T) {
	hp := &httpProvider{}
	recs, calls, failures, err := hp.fetchReceiptsIndividually(context.Background(), nil)
	if len(recs) != 0 || calls != 0 || failures != 0 || err != nil {
		t.Fatalf("unexpected result: recs=%v calls=%d failures=%d err=%v", recs, calls, failures, err)
	}
}

func TestHTTPProvider_CallBlockReceiptsFilterSkip(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] == "eth_getBlockReceipts" {
			return mkResp([]map[string]any{
				{"transactionHash": "0xaaa", "status": "0x1", "gasUsed": "0x5208"},
				{"transactionHash": "0xbbb", "status": "0x1", "gasUsed": "0x5209"},
			}), nil
		}
		return mkResp(nil), nil
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	hp := p.(*httpProvider)
	filter := map[string]struct{}{"0xaaa": {}}
	recs, callErr := hp.callBlockReceipts(context.Background(), 10, filter)
	if callErr != nil {
		t.Fatalf("unexpected error: %v", callErr)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 filtered receipt, got %d", len(recs))
	}
}

func TestHTTPProvider_FetchReceiptsForBlockMissingFallbackError(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "eth_getBlockReceipts":
			return mkResp([]map[string]any{{"transactionHash": "0xaaa", "status": "0x1", "gasUsed": "0x5208"}}), nil
		case "eth_getTransactionReceipt":
			return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader([]byte("fail")))}, nil
		default:
			return mkResp(nil), nil
		}
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	hp := p.(*httpProvider)
	recs, calls, failures, ferr := hp.fetchReceiptsForBlock(context.Background(), 10, []string{"0xaaa", "0xbbb"})
	if len(recs) != 1 {
		t.Fatalf("expected single receipt, got %d", len(recs))
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls (1 block + 1 fallback), got %d", calls)
	}
	if failures != 1 {
		t.Fatalf("expected 1 failure from fallback, got %d", failures)
	}
	if ferr == nil {
		t.Fatal("expected fallback error")
	}
}

func TestHTTPProvider_FetchReceiptsIndividuallyCancellation(t *testing.T) {
	hp := &httpProvider{receiptWorkers: 0}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	receipts, calls, failures, err := hp.fetchReceiptsIndividually(ctx, []string{"0xaaa"})
	if calls != 1 {
		t.Fatalf("calls=%d want 1", calls)
	}
	if failures != 1 {
		t.Fatalf("failures=%d want 1", failures)
	}
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if len(receipts) != 0 {
		t.Fatalf("receipts len=%d want 0", len(receipts))
	}
}

func TestHTTPProvider_ShouldUseBlockReceiptsStates(t *testing.T) {
	p := &httpProvider{}
	if p.shouldUseBlockReceipts(1) {
		t.Fatal("expected false for unknown state and single match")
	}
	if !p.shouldUseBlockReceipts(2) {
		t.Fatal("expected true for unknown state and multiple matches")
	}
	p.setBlockReceiptsState(receiptSupportUnavailable)
	if p.shouldUseBlockReceipts(5) {
		t.Fatal("expected false when marked unavailable")
	}
	p.setBlockReceiptsState(receiptSupportAvailable)
	if !p.shouldUseBlockReceipts(1) {
		t.Fatal("expected true when marked available")
	}
}

func TestHTTPProvider_IsMethodNotFound(t *testing.T) {
	if !isMethodNotFound(fmt.Errorf("rpc -32601: method not supported")) {
		t.Fatal("expected detection for -32601")
	}
	if !isMethodNotFound(fmt.Errorf("Method not found")) {
		t.Fatal("expected detection for method not found text")
	}
	if isMethodNotFound(fmt.Errorf("rpc -32000: error")) {
		t.Fatal("did not expect detection for other errors")
	}
	if isMethodNotFound(nil) {
		t.Fatal("nil error should return false")
	}
}

func TestHTTPProvider_TransactionsLoggerNil(t *testing.T) {
	prev := logging.Logger()
	logging.SetLogger(nil)
	defer logging.SetLogger(prev)
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] == "eth_getBlockByNumber" {
			block := map[string]any{"timestamp": "0x1", "transactions": []map[string]any{}}
			return mkResp(block), nil
		}
		return mkResp(nil), nil
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	txs, err := p.Transactions(context.Background(), "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(txs) != 0 {
		t.Fatalf("expected no transactions, got %d", len(txs))
	}
}

func TestHTTPProvider_TransactionsTimestampHexError(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] == "eth_getBlockByNumber" {
			block := map[string]any{
				"timestamp":    "0xzz",
				"transactions": []map[string]any{},
			}
			return mkResp(block), nil
		}
		return mkResp(nil), nil
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	prev := logging.Logger()
	logging.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer logging.SetLogger(prev)
	if _, err := p.Transactions(context.Background(), "0xabc", 1, 1); err == nil {
		t.Fatal("expected timestamp hex parse error")
	}
}

func TestHTTPProvider_TransactionsGasUsedHexError(t *testing.T) {
	const target = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "eth_getBlockByNumber":
			block := map[string]any{
				"timestamp": "0x1",
				"transactions": []map[string]any{
					{"hash": "0xaaa", "from": target, "to": target, "input": "0x", "value": "0x1"},
				},
			}
			return mkResp(block), nil
		case "eth_getTransactionReceipt":
			return mkResp(map[string]any{"status": "0x1", "gasUsed": "0xzz"}), nil
		default:
			return mkResp(nil), nil
		}
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	prev := logging.Logger()
	logging.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer logging.SetLogger(prev)
	if _, err := p.Transactions(context.Background(), target, 1, 1); err == nil {
		t.Fatal("expected gasUsed hex parse error")
	}
}

func TestHTTPProvider_TransactionsStatusHexError(t *testing.T) {
	const target = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "eth_getBlockByNumber":
			block := map[string]any{
				"timestamp": "0x1",
				"transactions": []map[string]any{
					{"hash": "0xaaa", "from": target, "to": target, "input": "0x", "value": "0x1"},
				},
			}
			return mkResp(block), nil
		case "eth_getTransactionReceipt":
			return mkResp(map[string]any{"status": "0xzz", "gasUsed": "0x1"}), nil
		default:
			return mkResp(nil), nil
		}
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	prev := logging.Logger()
	logging.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer logging.SetLogger(prev)
	if _, err := p.Transactions(context.Background(), target, 1, 1); err == nil {
		t.Fatal("expected status hex parse error")
	}
}

func TestNormalizeContractAddr(t *testing.T) {
	tests := []struct {
		name string
		in   string
		exp  string
	}{
		{"empty", " ", ""},
		{"lower", "0xabc", "0xabc"},
		{"upper", " 0xABCDEF ", "0xabcdef"},
	}
	for _, tc := range tests {
		if got := normalizeContractAddr(tc.in); got != tc.exp {
			t.Fatalf("%s: expected %q got %q", tc.name, tc.exp, got)
		}
	}
}

func TestHTTPProvider_CallBlockReceipts(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] == "eth_getBlockReceipts" {
			recs := []map[string]any{
				{"transactionHash": "0xAAA", "gasUsed": "0x10", "status": "0x1", "contractAddress": " 0xABCD "},
				{"transactionHash": "0xBBB", "gasUsed": "0x20", "status": "0x0", "contractAddress": "0xBEEF"},
				{"transactionHash": "0xCCC", "gasUsed": "0x5"},
			}
			return mkResp(recs), nil
		}
		return mkResp(nil), nil
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := p.(*httpProvider).callBlockReceipts(context.Background(), 1, map[string]struct{}{"0xaaa": {}, "0xccc": {}})
	if err != nil {
		t.Fatalf("callBlockReceipts err: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 receipts, got %d", len(recs))
	}
	first := recs["0xaaa"]
	if first.contractAddress != "0xabcd" || first.status != 1 || first.gasUsed != 16 {
		t.Fatalf("unexpected first receipt: %+v", first)
	}
	third := recs["0xccc"]
	if third.contractAddress != "" || third.gasUsed != 5 {
		t.Fatalf("unexpected third receipt: %+v", third)
	}
	if _, ok := recs["0xbbb"]; ok {
		t.Fatalf("unexpected filtered receipt present")
	}
}

func TestHTTPProvider_FetchReceiptsIndividuallyVariants(t *testing.T) {
	responses := map[string]*http.Response{}
	respFor := func(body any) *http.Response { return mkResp(body) }
	responses["0xAAA"] = respFor(map[string]any{"status": "0x1", "gasUsed": "0x10", "contractAddress": "0xABCDEF"})
	responses["0xBBB"] = respFor(map[string]any{"status": "bad", "gasUsed": "0x1"})
	responses["0xCCC"] = respFor(map[string]any{"gasUsed": "0x2"})
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] == "eth_getTransactionReceipt" {
			params := req["params"].([]any)
			hash := params[0].(string)
			if resp, ok := responses[hash]; ok {
				return resp, nil
			}
		}
		return mkResp(nil), nil
	})}
	p, err := NewHTTPProvider("http://unit-test", client)
	if err != nil {
		t.Fatal(err)
	}
	hp := p.(*httpProvider)
	hp.receiptWorkers = 1
	out, calls, failures, err := hp.fetchReceiptsIndividually(context.Background(), []string{"0xAAA", "0xBBB", "0xCCC"})
	if err == nil {
		t.Fatal("expected joined error")
	}
	if calls != 3 || failures != 1 {
		t.Fatalf("unexpected calls/failures: %d/%d", calls, failures)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 receipts, got %d", len(out))
	}
	if rec := out["0xaaa"]; rec.contractAddress != "0xabcdef" || rec.status != 1 || rec.gasUsed != 16 {
		t.Fatalf("normalized receipt mismatch: %+v", rec)
	}
	if rec := out["0xccc"]; rec.contractAddress != "" || rec.status != 1 || rec.gasUsed != 2 {
		t.Fatalf("nil contract receipt mismatch: %+v", rec)
	}
	if res, calls, failures, err := hp.fetchReceiptsIndividually(context.Background(), nil); err != nil || len(res) != 0 || calls != 0 || failures != 0 {
		t.Fatalf("empty input fast-path mismatch: res=%v calls=%d failures=%d err=%v", res, calls, failures, err)
	}
}
