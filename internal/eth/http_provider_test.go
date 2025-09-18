package eth

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func mkResp(v any) *http.Response {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": v})
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
