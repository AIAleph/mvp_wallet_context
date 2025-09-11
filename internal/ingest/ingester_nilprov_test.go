package ingest

import (
    "context"
    "testing"
)

func TestBackfillDelta_NoProviderEarlyReturn(t *testing.T) {
    ing := New("0x", Options{})
    if err := ing.Backfill(context.Background()); err != nil { t.Fatal(err) }
    if err := ing.Delta(context.Background()); err != nil { t.Fatal(err) }
}

