package ingest

// Covers canonical schema insert paths and their error branches to reach 100%.

import (
    "context"
    "net/http"
    "strings"
    "testing"

    "github.com/AIAleph/mvp_wallet_context/internal/eth"
)

// Provider yielding one ERC-20 transfer, one approval, and one trace.
type provCanonRich struct{}

func (provCanonRich) BlockNumber(ctx context.Context) (uint64, error) { return 1, nil }
func (provCanonRich) BlockTimestamp(ctx context.Context, block uint64) (int64, error) { return 0, nil }
func (provCanonRich) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
    pad := func(a string) string { return "0x" + strings.Repeat("0", 24) + strings.TrimPrefix(strings.ToLower(a), "0x") }
    token := "0xdead"
    fromA := "0x1111111111111111111111111111111111111111"
    toA := "0x2222222222222222222222222222222222222222"
    spender := "0x3333333333333333333333333333333333333333"
    // ERC-20 Transfer
    l1 := eth.Log{TxHash: "0x1", Index: 0, Address: token, Topics: []string{"0xddf252ad", pad(fromA), pad(toA)}, DataHex: "0x1", BlockNum: from}
    // Approval
    l2 := eth.Log{TxHash: "0x2", Index: 1, Address: token, Topics: []string{"0x8c5be1e5", pad(fromA), pad(spender)}, DataHex: "0x1", BlockNum: from}
    return []eth.Log{l1, l2}, nil
}
func (provCanonRich) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
    return []eth.Trace{{TxHash: "0x3", TraceID: "root", From: address, To: address, ValueWei: "0x1", BlockNum: from}}, nil
}

// Helper RoundTripper reused from other tests (ioNopCloser defined there as well)
type rtFunc func(*http.Request) (*http.Response, error)
func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCanonical_InsertLogsError(t *testing.T) {
    old := http.DefaultTransport
    defer func() { http.DefaultTransport = old }()
    http.DefaultTransport = rtFunc(func(r *http.Request) (*http.Response, error) {
        q := r.URL.Query().Get("query")
        if strings.Contains(q, "INSERT INTO logs") { return &http.Response{StatusCode: 500, Body: ioNopCloser("boom")}, nil }
        return &http.Response{StatusCode: 200, Body: ioNopCloser("ok")}, nil
    })
    ing := NewWithProvider("0xabc", Options{Schema: "canonical", ClickHouseDSN: "http://localhost:8123/db"}, provCanonRich{})
    if err := ing.processRange(context.Background(), 1, 1); err == nil { t.Fatal("expected logs insert error") }
}

func TestCanonical_InsertTokenTransfersError(t *testing.T) {
    old := http.DefaultTransport
    defer func() { http.DefaultTransport = old }()
    http.DefaultTransport = rtFunc(func(r *http.Request) (*http.Response, error) {
        q := r.URL.Query().Get("query")
        if strings.Contains(q, "INSERT INTO token_transfers") { return &http.Response{StatusCode: 500, Body: ioNopCloser("boom")}, nil }
        return &http.Response{StatusCode: 200, Body: ioNopCloser("ok")}, nil
    })
    ing := NewWithProvider("0xabc", Options{Schema: "canonical", ClickHouseDSN: "http://localhost:8123/db"}, provCanonRich{})
    if err := ing.processRange(context.Background(), 1, 1); err == nil { t.Fatal("expected token_transfers insert error") }
}

func TestCanonical_InsertApprovalsError(t *testing.T) {
    old := http.DefaultTransport
    defer func() { http.DefaultTransport = old }()
    http.DefaultTransport = rtFunc(func(r *http.Request) (*http.Response, error) {
        q := r.URL.Query().Get("query")
        if strings.Contains(q, "INSERT INTO approvals") { return &http.Response{StatusCode: 500, Body: ioNopCloser("boom")}, nil }
        return &http.Response{StatusCode: 200, Body: ioNopCloser("ok")}, nil
    })
    ing := NewWithProvider("0xabc", Options{Schema: "canonical", ClickHouseDSN: "http://localhost:8123/db"}, provCanonRich{})
    if err := ing.processRange(context.Background(), 1, 1); err == nil { t.Fatal("expected approvals insert error") }
}

func TestCanonical_InsertTracesError(t *testing.T) {
    old := http.DefaultTransport
    defer func() { http.DefaultTransport = old }()
    http.DefaultTransport = rtFunc(func(r *http.Request) (*http.Response, error) {
        q := r.URL.Query().Get("query")
        if strings.Contains(q, "INSERT INTO traces") { return &http.Response{StatusCode: 500, Body: ioNopCloser("boom")}, nil }
        return &http.Response{StatusCode: 200, Body: ioNopCloser("ok")}, nil
    })
    ing := NewWithProvider("0xabc", Options{Schema: "canonical", ClickHouseDSN: "http://localhost:8123/db"}, provCanonRich{})
    if err := ing.processRange(context.Background(), 1, 1); err == nil { t.Fatal("expected traces insert error") }
}
