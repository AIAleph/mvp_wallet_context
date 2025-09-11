package ingest

import (
    "context"
    "testing"
    "time"
)

func TestNewAndMethods(t *testing.T) {
    opts := Options{ProviderURL: "", ClickHouseDSN: "", FromBlock: 1, ToBlock: 2, Confirmations: 12, BatchBlocks: 100, DryRun: false, Timeout: time.Second}
    ing := New("0xabc", opts)
    if ing == nil { t.Fatal("New returned nil") }
    if err := ing.Backfill(context.Background()); err != nil { t.Fatalf("Backfill error: %v", err) }
    if err := ing.Delta(context.Background()); err != nil { t.Fatalf("Delta error: %v", err) }
}

