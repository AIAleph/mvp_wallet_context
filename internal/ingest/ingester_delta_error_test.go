package ingest

import (
    "context"
    "testing"
)

func TestDelta_PropagatesProcessError(t *testing.T) {
    ing := NewWithProvider("0x", Options{FromBlock: 1, ToBlock: 1, BatchBlocks: 1}, provGetLogsErr{})
    if err := ing.Delta(context.Background()); err == nil { t.Fatal("expected error from processRange during delta") }
}

