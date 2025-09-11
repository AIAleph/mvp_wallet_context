package ingest

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/AIAleph/mvp_wallet_context/internal/eth"
    "github.com/AIAleph/mvp_wallet_context/internal/normalize"
    "github.com/AIAleph/mvp_wallet_context/pkg/ch"
)

// Options configure a run of the ingester.
type Options struct {
    ProviderURL   string
    ClickHouseDSN string
    FromBlock     uint64
    ToBlock       uint64
    Confirmations int
    BatchBlocks   int
    RateLimit     int    // RPC requests per second (0 = unlimited)
    RedisURL      string // Optional cache endpoint
    DryRun        bool
    Timeout       time.Duration
}

// Ingester coordinates fetching, normalization and persistence for a single
// address. It is intentionally minimal for scaffolding.
type Ingester struct {
    address string
    opts    Options
    prov    eth.Provider
    ch      *ch.Client
    tsMu    sync.RWMutex
    tsCache map[uint64]int64
}

func New(address string, opts Options) *Ingester {
    var c *ch.Client
    if opts.ClickHouseDSN != "" { c = ch.New(opts.ClickHouseDSN) } else { c = ch.New("") }
    return &Ingester{address: address, opts: opts, ch: c, tsCache: make(map[uint64]int64)}
}

// NewWithProvider injects a concrete eth.Provider (already wrapped with
// rate-limiter, retries, etc.). Prefer this in production wiring.
func NewWithProvider(address string, opts Options, p eth.Provider) *Ingester {
    var c *ch.Client
    if opts.ClickHouseDSN != "" { c = ch.New(opts.ClickHouseDSN) } else { c = ch.New("") }
    return &Ingester{address: address, opts: opts, prov: p, ch: c, tsCache: make(map[uint64]int64)}
}

// Backfill performs the initial, historical sync for the configured address.
// MVP scaffold: implement block-ranged fetchers in follow-up tasks.
func (i *Ingester) Backfill(ctx context.Context) error {
    // Early return if no provider is wired (tests / dry runs)
    if i.prov == nil {
        return nil
    }
    head, err := i.prov.BlockNumber(ctx)
    if err != nil { return err }
    from := i.opts.FromBlock
    to := i.opts.ToBlock
    if to == 0 { to = head }
    if from > to { return fmt.Errorf("from(%d) > to(%d)", from, to) }
    batch := uint64(i.opts.BatchBlocks)
    if batch == 0 { batch = 1000 }
    for cur := from; cur <= to; {
        end := cur + batch - 1
        if end > to { end = to }
        if err := i.processRange(ctx, cur, end); err != nil { return err }
        cur = end + 1
    }
    return nil
}

// Delta performs a recent delta update with N confirmations.
func (i *Ingester) Delta(ctx context.Context) error {
    if i.prov == nil {
        return nil
    }
    head, err := i.prov.BlockNumber(ctx)
    if err != nil { return err }
    // Apply confirmations window
    end := head
    if i.opts.Confirmations > 0 {
        conf := uint64(i.opts.Confirmations)
        if end > conf { end = end - conf } else { end = 0 }
    }
    from := i.opts.FromBlock
    to := i.opts.ToBlock
    if to == 0 || to > end { to = end }
    if from > to { return nil } // nothing to do
    batch := uint64(i.opts.BatchBlocks)
    if batch == 0 { batch = 1000 }
    for cur := from; cur <= to; {
        rEnd := cur + batch - 1
        if rEnd > to { rEnd = to }
        if err := i.processRange(ctx, cur, rEnd); err != nil { return err }
        cur = rEnd + 1
    }
    return nil
}

// processRange fetches logs and traces for the configured address and block range.
func (i *Ingester) processRange(ctx context.Context, from, to uint64) error {
    // Topics nil for now; later pass selectors for token transfers/approvals
    logs, err := i.prov.GetLogs(ctx, i.address, from, to, nil)
    if err != nil { return err }
    traces, err := i.prov.TraceBlock(ctx, from, to, i.address)
    if err != nil && err != eth.ErrUnsupported { return err }
    // Fill timestamps if missing using in-process cache + provider
    for idx := range logs {
        if logs[idx].TsMillis == 0 {
            if ts, ok := i.getBlockTs(ctx, logs[idx].BlockNum); ok {
                logs[idx].TsMillis = ts
            }
        }
    }
    for idx := range traces {
        if traces[idx].TsMillis == 0 {
            if ts, ok := i.getBlockTs(ctx, traces[idx].BlockNum); ok {
                traces[idx].TsMillis = ts
            }
        }
    }
    // Normalize
    lrows := normalize.LogsToRows(logs)
    if err := i.ch.InsertJSONEachRow(ctx, "dev_logs", normalize.AsAny(lrows)); err != nil { return err }
    // Decode token transfers and approvals and write dev tables
    tTransfers, tApprovals := normalize.DecodeTokenEvents(logs)
    if len(tTransfers) > 0 {
        if err := i.ch.InsertJSONEachRow(ctx, "dev_token_transfers", normalize.AsAny(tTransfers)); err != nil { return err }
    }
    if len(tApprovals) > 0 {
        if err := i.ch.InsertJSONEachRow(ctx, "dev_approvals", normalize.AsAny(tApprovals)); err != nil { return err }
    }
    if traces != nil {
        trows := normalize.TracesToRows(traces)
        if err := i.ch.InsertJSONEachRow(ctx, "dev_traces", normalize.AsAny(trows)); err != nil { return err }
    }
    return nil
}

func (i *Ingester) getBlockTs(ctx context.Context, block uint64) (int64, bool) {
    i.tsMu.RLock()
    if ts, ok := i.tsCache[block]; ok { i.tsMu.RUnlock(); return ts, true }
    i.tsMu.RUnlock()
    if i.prov == nil { return 0, false }
    ts, err := i.prov.BlockTimestamp(ctx, block)
    if err != nil { return 0, false }
    i.tsMu.Lock(); i.tsCache[block] = ts; i.tsMu.Unlock()
    return ts, true
}
