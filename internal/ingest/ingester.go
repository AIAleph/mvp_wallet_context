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
    // Schema selects target tables: "dev" (dev_*) or "canonical" (schema.sql tables)
    Schema        string
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
    // Normalize and write according to schema mode
    mode := i.SchemaMode()
    if mode == "canonical" {
        // Logs
        lrows := normalize.LogsToRows(logs)
        if len(lrows) > 0 {
            rows := make([]any, 0, len(lrows))
            for _, r := range lrows {
                rows = append(rows, map[string]any{
                    "event_uid":    r.EventUID,
                    "tx_hash":      r.TxHash,
                    "log_index":    r.LogIndex,
                    "address":      r.Address,
                    "topics":       r.Topics,
                    "data_hex":     r.DataHex,
                    "block_number": r.BlockNum,
                    "ts":           fmtDT64(r.TsMillis),
                })
            }
            if err := i.ch.InsertJSONEachRow(ctx, "logs", rows); err != nil { return err }
        }
        // Token events
        tTransfers, tApprovals := normalize.DecodeTokenEvents(logs)
        if len(tTransfers) > 0 {
            rows := make([]any, 0, len(tTransfers))
            for _, r := range tTransfers {
                rows = append(rows, map[string]any{
                    "event_uid":    r.EventUID,
                    "tx_hash":      r.TxHash,
                    "log_index":    r.LogIndex,
                    "token":        r.Token,
                    "from_addr":    r.From,
                    "to_addr":      r.To,
                    "amount_raw":   r.AmountRaw,
                    "token_id":     r.TokenID,
                    "standard":     r.Standard,
                    "block_number": r.BlockNum,
                    "ts":           fmtDT64(r.TsMillis),
                })
            }
            if err := i.ch.InsertJSONEachRow(ctx, "token_transfers", rows); err != nil { return err }
        }
        if len(tApprovals) > 0 {
            rows := make([]any, 0, len(tApprovals))
            for _, r := range tApprovals {
                rows = append(rows, map[string]any{
                    "event_uid":          r.EventUID,
                    "tx_hash":            r.TxHash,
                    "log_index":          r.LogIndex,
                    "token":              r.Token,
                    "owner":              r.Owner,
                    "spender":            r.Spender,
                    "amount_raw":         r.AmountRaw,
                    "token_id":           r.TokenID,
                    "is_approval_for_all": r.IsForAll,
                    "standard":           r.Standard,
                    "block_number":       r.BlockNum,
                    "ts":                 fmtDT64(r.TsMillis),
                })
            }
            if err := i.ch.InsertJSONEachRow(ctx, "approvals", rows); err != nil { return err }
        }
        if traces != nil && len(traces) > 0 {
            trows := normalize.TracesToRows(traces)
            rows := make([]any, 0, len(trows))
            for _, r := range trows {
                rows = append(rows, map[string]any{
                    "trace_uid":    r.TraceUID,
                    "tx_hash":      r.TxHash,
                    "trace_id":     r.TraceID,
                    "from_addr":    r.From,
                    "to_addr":      r.To,
                    "value_raw":    r.ValueRaw,
                    "block_number": r.BlockNum,
                    "ts":           fmtDT64(r.TsMillis),
                })
            }
            if err := i.ch.InsertJSONEachRow(ctx, "traces", rows); err != nil { return err }
        }
    } else {
        // dev schema (existing behavior)
        lrows := normalize.LogsToRows(logs)
        if err := i.ch.InsertJSONEachRow(ctx, "dev_logs", normalize.AsAny(lrows)); err != nil { return err }
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

// SchemaMode returns the normalized schema mode (dev or canonical).
func (i *Ingester) SchemaMode() string {
    m := i.opts.Schema
    if m == "" { return "dev" }
    switch m {
    case "dev", "canonical":
        return m
    default:
        return "dev"
    }
}

// fmtDT64 formats milliseconds since epoch to ClickHouse-compatible DateTime64(3) string (UTC).
func fmtDT64(ms int64) string {
    if ms <= 0 { return "1970-01-01 00:00:00.000" }
    sec := ms / 1000
    nsec := (ms % 1000) * int64(time.Millisecond)
    t := time.Unix(sec, nsec).UTC()
    // 2006-01-02 15:04:05.000
    return t.Format("2006-01-02 15:04:05.000")
}
