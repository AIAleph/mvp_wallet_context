package ingest

import (
    "context"
    "io"
    "net/http"
    "net/url"
    "strings"
    "testing"

    "github.com/AIAleph/mvp_wallet_context/internal/eth"
)


type provHeadOK struct{ h uint64 }
func (p provHeadOK) BlockNumber(ctx context.Context) (uint64, error) { return p.h, nil }
func (p provHeadOK) BlockTimestamp(ctx context.Context, block uint64) (int64, error) { return 0, nil }
func (p provHeadOK) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) { return nil, nil }
func (p provHeadOK) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) { return nil, nil }

type provLogsErr struct{ h uint64 }
func (p provLogsErr) BlockNumber(ctx context.Context) (uint64, error) { return p.h, nil }
func (p provLogsErr) BlockTimestamp(ctx context.Context, block uint64) (int64, error) { return 0, nil }
func (p provLogsErr) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) { return nil, context.Canceled }
func (p provLogsErr) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) { return nil, nil }

// Provider that returns one transfer and one approval and one trace
type provMixed struct{}
func (provMixed) BlockNumber(ctx context.Context) (uint64, error) { return 0, nil }
func (provMixed) BlockTimestamp(ctx context.Context, block uint64) (int64, error) { return 0, nil }
func (provMixed) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
    padAddr := func(a string) string { return "0x" + strings.Repeat("0", 24) + strings.TrimPrefix(strings.ToLower(a), "0x") }
    token := "0xdead"
    fromA := "0x1111111111111111111111111111111111111111"
    toA := "0x2222222222222222222222222222222222222222"
    spender := "0x3333333333333333333333333333333333333333"
    // ERC-20 transfer
    l1 := eth.Log{TxHash: "0x1", Index: 0, Address: token, Topics: []string{"0xddf252ad", padAddr(fromA), padAddr(toA)}, DataHex: "0x1"}
    // Approval
    l2 := eth.Log{TxHash: "0x2", Index: 1, Address: token, Topics: []string{"0x8c5be1e5", padAddr(fromA), padAddr(spender)}, DataHex: "0x1"}
    return []eth.Log{l1, l2}, nil
}
func (provMixed) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
    return []eth.Trace{{TxHash: "0x3", TraceID: "root", From: address, To: address, ValueWei: "0x1", BlockNum: from}}, nil
}

// Provider only approvals log
type provOnlyApproval struct{}
func (provOnlyApproval) BlockNumber(context.Context) (uint64, error) { return 0, nil }
func (provOnlyApproval) BlockTimestamp(context.Context, uint64) (int64, error) { return 0, nil }
func (provOnlyApproval) GetLogs(context.Context, string, uint64, uint64, [][]string) ([]eth.Log, error) {
    padAddr := func(a string) string { return "0x" + strings.Repeat("0", 24) + strings.TrimPrefix(strings.ToLower(a), "0x") }
    owner := "0x1111111111111111111111111111111111111111"
    spender := "0x3333333333333333333333333333333333333333"
    l := eth.Log{TxHash: "0x4", Index: 0, Address: "0xdead", Topics: []string{"0x8c5be1e5", padAddr(owner), padAddr(spender)}, DataHex: "0x1"}
    return []eth.Log{l}, nil
}
func (provOnlyApproval) TraceBlock(context.Context, uint64, uint64, string) ([]eth.Trace, error) {
    return []eth.Trace{{TxHash: "0x3", TraceID: "root", From: "0x", To: "0x", ValueWei: "0x1", BlockNum: 1}}, nil
}

// Provider only traces
type provOnlyTrace struct{}
func (provOnlyTrace) BlockNumber(context.Context) (uint64, error) { return 0, nil }
func (provOnlyTrace) BlockTimestamp(context.Context, uint64) (int64, error) { return 0, nil }
func (provOnlyTrace) GetLogs(context.Context, string, uint64, uint64, [][]string) ([]eth.Log, error) { return nil, nil }
func (provOnlyTrace) TraceBlock(context.Context, uint64, uint64, string) ([]eth.Trace, error) {
    return []eth.Trace{{TxHash: "0x5", TraceID: "root", From: "0x", To: "0x", ValueWei: "0x1", BlockNum: 1}}, nil
}

func TestBackfill_ToZeroUsesHead_AndPropagatesProcessError(t *testing.T) {
    // to=0 uses head; processRange returns error -> Backfill returns error
    ing := NewWithProvider("0x", Options{FromBlock: 1, ToBlock: 0, BatchBlocks: 1}, provLogsErr{h: 2})
    if err := ing.Backfill(context.Background()); err == nil {
        t.Fatal("expected error propagated from processRange")
    }
}

func TestDelta_ProviderError_AndDefaultBatch(t *testing.T) {
    // Provider error path
    ing := NewWithProvider("0x", Options{}, provErr{})
    if err := ing.Delta(context.Background()); err == nil { t.Fatal("expected provider error") }
    // Default batch path: batch=0
    ing2 := NewWithProvider("0x", Options{FromBlock: 1, ToBlock: 1, BatchBlocks: 0}, provHeadOK{h: 1})
    if err := ing2.Delta(context.Background()); err != nil { t.Fatal(err) }
}

func TestProcessRange_ErrorPaths_TokenApprovalsAndTraces(t *testing.T) {
    // Override default transport so ClickHouse HTTP sees our stub.
    old := http.DefaultTransport
    defer func() { http.DefaultTransport = old }()
    http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
        u, _ := url.Parse(r.URL.String())
        q := u.Query().Get("query")
        // dev_logs succeeds, dev_token_transfers fails, approvals succeeds, traces fails
        if strings.Contains(q, "dev_token_transfers") || strings.Contains(q, "dev_traces") {
            return &http.Response{StatusCode: 500, Body: ioNopCloser("oops")}, nil
        }
        return &http.Response{StatusCode: 200, Body: ioNopCloser("ok")}, nil
    })

    ing := NewWithProvider("0xabc", Options{ClickHouseDSN: "http://localhost:8123/db"}, provMixed{})
    // First call should fail on token transfers insert
    if err := ing.processRange(context.Background(), 1, 1); err == nil { t.Fatal("expected token transfers insert error") }

    // Change stub to fail on approvals insert to hit that path; traces still fail too.
    http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
        u, _ := url.Parse(r.URL.String())
        q := u.Query().Get("query")
        if strings.Contains(q, "dev_approvals") || strings.Contains(q, "dev_traces") {
            return &http.Response{StatusCode: 500, Body: ioNopCloser("oops")}, nil
        }
        return &http.Response{StatusCode: 200, Body: ioNopCloser("ok")}, nil
    })
    // To skip token transfers error, provide logs with only approvals this time
    ing2 := NewWithProvider("0xabc", Options{ClickHouseDSN: "http://localhost:8123/db"}, provOnlyApproval{})
    if err := ing2.processRange(context.Background(), 1, 1); err == nil { t.Fatal("expected approvals insert error") }

    // Finally, ensure traces insert error path executes. Provide empty logs and traces non-empty.
    http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
        u, _ := url.Parse(r.URL.String())
        q := u.Query().Get("query")
        if strings.Contains(q, "dev_traces") {
            return &http.Response{StatusCode: 500, Body: ioNopCloser("oops")}, nil
        }
        return &http.Response{StatusCode: 200, Body: ioNopCloser("ok")}, nil
    })
    ing3 := NewWithProvider("0xabc", Options{ClickHouseDSN: "http://localhost:8123/db"}, provOnlyTrace{})
    if err := ing3.processRange(context.Background(), 1, 1); err == nil { t.Fatal("expected traces insert error") }
}

// Helper RoundTripper and io.ReadCloser
type roundTripFunc func(*http.Request) (*http.Response, error)
func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type rc struct{ s string }
func (r rc) Read(p []byte) (int, error) { copy(p, r.s); if len(r.s) <= len(p) { return len(r.s), io.EOF } ; return len(p), nil }
func (r rc) Close() error { return nil }
func ioNopCloser(s string) io.ReadCloser { return rc{s: s} }
