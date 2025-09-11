package eth

import (
    "context"
    "errors"
    "testing"
)

type fakeProvider struct{}

func (fakeProvider) BlockNumber(ctx context.Context) (uint64, error) { return 123, nil }
func (fakeProvider) BlockTimestamp(ctx context.Context, block uint64) (int64, error) { return 1000, nil }
func (fakeProvider) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]Log, error) {
    return []Log{{TxHash: "0x", Index: 0}}, nil
}
func (fakeProvider) TraceBlock(ctx context.Context, from, to uint64, address string) ([]Trace, error) {
    return []Trace{{TxHash: "0x", TraceID: "0"}}, nil
}

type errLimiter struct{}
func (errLimiter) Wait(ctx context.Context) error { return errors.New("rate limited") }

func TestRLProvider_ForwardsOnOK(t *testing.T) {
    p := WrapWithLimiter(fakeProvider{}, NewLimiter(0))
    bn, err := p.BlockNumber(context.Background())
    if err != nil || bn != 123 { t.Fatalf("bn=%d err=%v", bn, err) }
    logs, err := p.GetLogs(context.Background(), "0x", 1, 2, nil)
    if err != nil || len(logs) != 1 { t.Fatalf("logs len=%d err=%v", len(logs), err) }
    tr, err := p.TraceBlock(context.Background(), 1, 2, "0x")
    if err != nil || len(tr) != 1 { t.Fatalf("tr len=%d err=%v", len(tr), err) }
    ts, err := p.BlockTimestamp(context.Background(), 1)
    if err != nil || ts != 1000 { t.Fatalf("ts=%d err=%v", ts, err) }
}

func TestRLProvider_PropagatesLimiterError(t *testing.T) {
    rp := RLProvider{p: fakeProvider{}, l: errLimiter{}}
    if _, err := rp.BlockNumber(context.Background()); err == nil { t.Fatal("expected error") }
    if _, err := rp.GetLogs(context.Background(), "0x", 1, 2, nil); err == nil { t.Fatal("expected error") }
    if _, err := rp.TraceBlock(context.Background(), 1, 2, "0x"); err == nil { t.Fatal("expected error") }
    if _, err := rp.BlockTimestamp(context.Background(), 1); err == nil { t.Fatal("expected error") }
}
