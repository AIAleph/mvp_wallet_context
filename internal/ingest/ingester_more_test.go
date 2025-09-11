package ingest

import (
    "context"
    "testing"

    "github.com/AIAleph/mvp_wallet_context/internal/eth"
)

type fakeProv struct{
    bn uint64
    logs []struct{from,to uint64; items []string}
    traces []struct{from,to uint64; items []string}
    tsCalls int
}

func (p *fakeProv) BlockNumber(ctx context.Context) (uint64, error) { return p.bn, nil }
func (p *fakeProv) BlockTimestamp(ctx context.Context, block uint64) (int64, error) { p.tsCalls++; return int64(block)*1000, nil }
func (p *fakeProv) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
    out := []eth.Log{}
    for _, l := range p.logs {
        if l.from == from && l.to == to {
            for i := range l.items {
                out = append(out, eth.Log{TxHash: "0x", Index: uint32(i), Address: "0xdead", Topics: []string{"0xddf252ad"}, DataHex: "0x", BlockNum: from, TsMillis: 0})
            }
        }
    }
    return out, nil
}

func (p *fakeProv) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
    out := []eth.Trace{}
    for _, t := range p.traces {
        if t.from == from && t.to == to {
            for i := range t.items {
                out = append(out, eth.Trace{TxHash: "0x", TraceID: string(rune('a'+i)), From: address, To: address, ValueWei: "0x1", BlockNum: from, TsMillis: 0})
            }
        }
    }
    return out, nil
}

func TestBackfillDeltaAndTsCache(t *testing.T) {
    p := &fakeProv{bn: 12}
    p.logs = []struct{from,to uint64; items []string}{{from: 10, to: 10, items: []string{"l1"}}, {from: 11, to: 11, items: []string{"l2"}}}
    p.traces = []struct{from,to uint64; items []string}{{from: 10, to: 10, items: []string{"t1"}}}
    opts := Options{FromBlock: 10, ToBlock: 11, BatchBlocks: 1, Confirmations: 1}
    ing := NewWithProvider("0xabc", opts, p)

    // Backfill covers both blocks
    if err := ing.Backfill(context.Background()); err != nil { t.Fatal(err) }
    callsAfterBackfill := p.tsCalls
    if callsAfterBackfill == 0 { t.Fatal("expected timestamp calls") }

    // Delta with confirmations trims head and still processes range; run again
    if err := ing.Delta(context.Background()); err != nil { t.Fatal(err) }
}
