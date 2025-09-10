package ingest

import (
    "context"
    "time"
)

// Options configure a run of the ingester.
type Options struct {
    ProviderURL   string
    ClickHouseDSN string
    FromBlock     uint64
    ToBlock       uint64
    Confirmations int
    BatchBlocks   int
    DryRun        bool
    Timeout       time.Duration
}

// Ingester coordinates fetching, normalization and persistence for a single
// address. It is intentionally minimal for scaffolding.
type Ingester struct {
    address string
    opts    Options
}

func New(address string, opts Options) *Ingester {
    return &Ingester{address: address, opts: opts}
}

// Backfill performs the initial, historical sync for the configured address.
// MVP scaffold: implement block-ranged fetchers in follow-up tasks.
func (i *Ingester) Backfill(ctx context.Context) error {
    _ = ctx
    // TODO: implement backfill (P0 M1). This stub exists to unblock wiring.
    return nil
}

// Delta performs a recent delta update with N confirmations.
func (i *Ingester) Delta(ctx context.Context) error {
    _ = ctx
    // TODO: implement delta (P0 M1). This stub exists to unblock wiring.
    return nil
}
