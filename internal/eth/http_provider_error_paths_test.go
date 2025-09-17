package eth

// Covers HTTP provider retries, decode errors, and enrichment paths.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestHTTPProvider_Call_JSONDecodeError(t *testing.T) {
	// Return 200 with invalid JSON to exercise decode error + retry path
	calls := 0
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte("{bad json")))}, nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	if hp, ok := p.(*httpProvider); ok {
		hp.maxRetries = 1
		hp.backoffBase = 1
	}
	var out any
	if err := p.(*httpProvider).call(context.Background(), "eth_blockNumber", nil, &out); err == nil {
		t.Fatal("expected decode error")
	}
	if calls == 0 {
		t.Fatal("expected at least one call")
	}
}

func TestHTTPProvider_500RetryThenSuccess(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls < 2 {
			return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader([]byte("oops")))}, nil
		}
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": "0x2a"})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b))}, nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	if hp, ok := p.(*httpProvider); ok {
		hp.maxRetries = 2
		hp.backoffBase = 1
	}
	n, err := p.BlockNumber(context.Background())
	if err != nil || n != 42 || calls != 2 {
		t.Fatalf("n=%d err=%v calls=%d", n, err, calls)
	}
}

func TestHTTPProvider_BlockNumber_BadHex(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": "0xzz"})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b))}, nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	if _, err := p.BlockNumber(context.Background()); err == nil {
		t.Fatal("expected hex parse error")
	}
}

func TestHexToUint64_Invalid(t *testing.T) {
	if _, err := hexToUint64("bad"); err == nil {
		t.Fatal("expected error")
	}
}

func TestHTTPProvider_Call_OutNil(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		// Return any valid JSON-RPC result; out == nil means we just ignore it
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": 123})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b))}, nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	if err := p.(*httpProvider).call(context.Background(), "x", nil, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestHTTPProvider_BlockNumber_CallErrorPath(t *testing.T) {
	// Transport returns network error; set retries to 0 so call returns error
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) { return nil, errors.New("net") })}
	p, _ := NewHTTPProvider("http://unit-test", client)
	if hp, ok := p.(*httpProvider); ok {
		hp.maxRetries = 0
	}
	if _, err := p.BlockNumber(context.Background()); err == nil {
		t.Fatal("expected call error")
	}
}

func TestHTTPProvider_BlockTimestamp_HexError(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] == "eth_getBlockByNumber" {
			b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{"timestamp": "0xzz"}})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b))}, nil
		}
		return mkResp(nil), nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	if _, err := p.BlockTimestamp(context.Background(), 123); err == nil {
		t.Fatal("expected hex error")
	}
}

func TestHTTPProvider_ContextCancelDuringBackoff(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 429, Body: io.NopCloser(bytes.NewReader([]byte("rate")))}, nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	if hp, ok := p.(*httpProvider); ok {
		hp.maxRetries = 2
		hp.backoffBase = 2 * time.Millisecond
	}
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel before the backoff wait to take the ctx.Done branch
	cancel()
	if err := p.(*httpProvider).call(ctx, "x", nil, nil); err == nil {
		t.Fatal("expected context error")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call before cancel, got %d", calls)
	}
}

func TestHTTPProvider_GetLogs_TopicsParamShapes(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] == "eth_getLogs" {
			params := req["params"].([]any)
			obj := params[0].(map[string]any)
			topics := obj["topics"].([]any)
			if topics[0] != nil {
				t.Fatalf("expected nil in first topic group, got %v", topics[0])
			}
			if s, ok := topics[1].(string); !ok || s != "0x01" {
				t.Fatalf("expected single string in second, got %T %v", topics[1], topics[1])
			}
			if arr, ok := topics[2].([]any); !ok || len(arr) != 2 {
				t.Fatalf("expected array in third, got %T %v", topics[2], topics[2])
			}
			// Return empty logs
			b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": []any{}})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b))}, nil
		}
		// Block fetch should not be called because no logs
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{"timestamp": "0x0"}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b))}, nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	if hp, ok := p.(*httpProvider); ok {
		hp.backoffBase = 1
	}
	out, err := p.GetLogs(context.Background(), "0xdead", 1, 2, [][]string{nil, []string{"0x01"}, []string{"0x02", "0x03"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("expected no logs, got %d", len(out))
	}
}

func TestHTTPProvider_Call_NewRequestError(t *testing.T) {
	// Invalid endpoint triggers http.NewRequest error
	p, _ := NewHTTPProvider("http://[", &http.Client{})
	if err := p.(*httpProvider).call(context.Background(), "x", nil, nil); err == nil {
		t.Fatal("expected error from NewRequestWithContext")
	}
}

func TestHTTPProvider_GetLogs_CallError(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] == "eth_getLogs" {
			return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader([]byte("no")))}, nil
		}
		return mkResp(nil), nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	if _, err := p.GetLogs(context.Background(), "0x", 1, 2, nil); err == nil {
		t.Fatal("expected getLogs call error")
	}
}

func TestHTTPProvider_TraceBlock_CallError(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] == "trace_filter" {
			return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader([]byte("no")))}, nil
		}
		return mkResp(nil), nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	if _, err := p.TraceBlock(context.Background(), 1, 2, "0x"); err == nil {
		t.Fatal("expected trace_filter call error")
	}
}

func TestHTTPProvider_TraceBlock_AfterIncrementPaging(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "trace_filter":
			params := req["params"].([]any)
			obj := params[0].(map[string]any)
			after := int(obj["after"].(float64))
			if after == 0 {
				// return exactly 1000 items to trigger after+=page
				arr := make([]map[string]any, 1000)
				for i := 0; i < 1000; i++ {
					arr[i] = map[string]any{
						"transactionHash": "0x1",
						"blockNumber":     "0x10",
						"traceAddress":    []int{i},
						"action":          map[string]any{"from": "0x", "to": "0x", "value": "0x1"},
					}
				}
				return mkResp(arr), nil
			}
			return mkResp([]any{}), nil
		case "eth_getBlockByNumber":
			return mkResp(map[string]any{"timestamp": "0x1"}), nil
		}
		return mkResp(nil), nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	out, err := p.TraceBlock(context.Background(), 1, 2, "0x")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1000 {
		t.Fatalf("expected 1000 traces, got %d", len(out))
	}
}

func TestHTTPProvider_GetLogs_TimestampEnrichmentError(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "eth_getLogs":
			logs := []map[string]any{{
				"transactionHash": "0xabc",
				"logIndex":        "0x0",
				"address":         "0xdead",
				"topics":          []string{"0x01"},
				"data":            "0x",
				"blockNumber":     "0x10",
			}}
			return mkResp(logs), nil
		case "eth_getBlockByNumber":
			// Return invalid timestamp to force enrichment skip
			b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{"timestamp": "0xzz"}})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b))}, nil
		default:
			return mkResp(nil), nil
		}
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	out, err := p.GetLogs(context.Background(), "0xdead", 1, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].TsMillis != 0 {
		t.Fatalf("expected TsMillis=0 on enrichment error, got %+v", out)
	}
}

func TestHTTPProvider_TraceBlock_TimestampEnrichmentError(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "trace_filter":
			if calls == 0 {
				calls++
				traces := []map[string]any{{
					"transactionHash": "0x1", "blockNumber": "0x10", "traceAddress": []int{}, "action": map[string]any{"from": "0x", "to": "0x", "value": "0x1"},
				}}
				return mkResp(traces), nil
			}
			return mkResp([]any{}), nil
		case "eth_getBlockByNumber":
			// invalid timestamp
			b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{"timestamp": "0xzz"}})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b))}, nil
		}
		return mkResp(nil), nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	out, err := p.TraceBlock(context.Background(), 1, 2, "0xdead")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].TsMillis != 0 {
		t.Fatalf("expected TsMillis=0 on enrichment error: %+v", out)
	}
}

func TestHTTPProvider_BlockTimestamp_CallError(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader([]byte("fail")))}, nil
	})}
	p, _ := NewHTTPProvider("http://unit-test", client)
	if _, err := p.BlockTimestamp(context.Background(), 1); err == nil {
		t.Fatal("expected call error")
	}
}
