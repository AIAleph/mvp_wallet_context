package ingest

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestBackfillUpdatesAddressCursor(t *testing.T) {
	prov := stubCursorProvider{head: 20}
	opts := Options{ClickHouseDSN: "http://localhost:8123/db", Confirmations: 12, BatchBlocks: 5}
	ing := NewWithProvider("0xABCDEF", opts, prov)
	rt := &cursorRoundTripper{t: t}
	ing.ch.SetTransport(rt)

	if err := ing.Backfill(context.Background()); err != nil {
		t.Fatalf("backfill err: %v", err)
	}
	if len(rt.inserts) == 0 {
		t.Fatal("expected addresses insert")
	}
	line := strings.TrimSpace(rt.inserts[len(rt.inserts)-1])
	var row struct {
		Address         string `json:"address"`
		LastSyncedBlock uint64 `json:"last_synced_block"`
		LastBackfillAt  string `json:"last_backfill_at"`
		LastDeltaAt     string `json:"last_delta_at"`
		UpdatedAt       string `json:"updated_at"`
	}
	if err := json.Unmarshal([]byte(line), &row); err != nil {
		t.Fatalf("decode insert: %v", err)
	}
	if want := uint64(8); row.LastSyncedBlock != want {
		t.Fatalf("last_synced_block=%d want %d", row.LastSyncedBlock, want)
	}
	if got := strings.ToLower(row.Address); got != "0xabcdef" {
		t.Fatalf("address=%s", got)
	}
	if row.LastBackfillAt == fmtDT64(0) {
		t.Fatal("expected last_backfill_at updated")
	}
	if row.LastDeltaAt != fmtDT64(0) {
		t.Fatal("delta timestamp should remain default")
	}
}

func TestBackfillWithNilProviderIsNoop(t *testing.T) {
	ing := New("0xabc", Options{FromBlock: 5, ToBlock: 10})
	if err := ing.Backfill(context.Background()); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestBackfillSkipsWhenFromExceedsSafeHead(t *testing.T) {
	defer withTimeNow(t, time.UnixMilli(5_000))()

	prov := &panicRangeProvider{head: 210, t: t}
	opts := Options{ClickHouseDSN: "http://localhost:8123/db", Confirmations: 12, FromBlock: 200, BatchBlocks: 50}
	ing := NewWithProvider("0xABCDEF", opts, prov)

	checkpoint := addressCheckpoint{Address: "0xABCDEF", LastSyncedBlock: 175, LastBackfillAt: fmtDT64(1000), LastDeltaAt: fmtDT64(2000), UpdatedAt: fmtDT64(3000)}
	payload, err := json.Marshal(checkpoint)
	if err != nil {
		t.Fatalf("marshal checkpoint: %v", err)
	}
	rt := &cursorRoundTripper{t: t, selectResponse: string(payload) + "\n"}
	ing.ch.SetTransport(rt)

	if err := ing.Backfill(context.Background()); err != nil {
		t.Fatalf("backfill err: %v", err)
	}
	if len(rt.inserts) == 0 {
		t.Fatal("expected checkpoint insert")
	}
	line := strings.TrimSpace(rt.inserts[len(rt.inserts)-1])
	var row addressCheckpoint
	if err := json.Unmarshal([]byte(line), &row); err != nil {
		t.Fatalf("decode insert: %v", err)
	}
	if row.LastSyncedBlock != checkpoint.LastSyncedBlock {
		t.Fatalf("last_synced_block changed: got %d want %d", row.LastSyncedBlock, checkpoint.LastSyncedBlock)
	}
	if row.LastBackfillAt != fmtDT64(5_000) {
		t.Fatalf("unexpected last_backfill_at: %s", row.LastBackfillAt)
	}
	if row.LastDeltaAt != checkpoint.LastDeltaAt {
		t.Fatalf("delta timestamp modified: %s", row.LastDeltaAt)
	}
}

func TestBackfillResumesFromCheckpoint(t *testing.T) {
	defer withTimeNow(t, time.UnixMilli(7_000))()

	prov := &captureProv{head: 210}
	opts := Options{ClickHouseDSN: "http://localhost:8123/db", BatchBlocks: 100}
	ing := NewWithProvider("0xabc", opts, prov)

	prevCheckpoint := addressCheckpoint{Address: "0xabc", LastSyncedBlock: 150, LastBackfillAt: fmtDT64(1_000), LastDeltaAt: fmtDT64(2_000), UpdatedAt: fmtDT64(3_000)}
	payload, err := json.Marshal(prevCheckpoint)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rt := &cursorRoundTripper{t: t, selectResponse: string(payload) + "\n"}
	ing.ch.SetTransport(rt)

	if err := ing.Backfill(context.Background()); err != nil {
		t.Fatalf("backfill err: %v", err)
	}
	if len(prov.calls) == 0 {
		t.Fatal("expected GetLogs call")
	}
	if prov.calls[0].from != 151 {
		t.Fatalf("expected resume from 151, got %d", prov.calls[0].from)
	}
	if len(rt.inserts) == 0 {
		t.Fatal("expected checkpoint insert")
	}
	var row addressCheckpoint
	if err := json.Unmarshal([]byte(strings.TrimSpace(rt.inserts[len(rt.inserts)-1])), &row); err != nil {
		t.Fatalf("decode insert: %v", err)
	}
	if row.LastSyncedBlock != 210 {
		t.Fatalf("last_synced_block=%d", row.LastSyncedBlock)
	}
	if row.LastBackfillAt != fmtDT64(7_000) {
		t.Fatalf("backfill timestamp not updated: %s", row.LastBackfillAt)
	}
}

func TestBackfillMaxUintCheckpointError(t *testing.T) {
	prov := stubCursorProvider{head: math.MaxUint64}
	ing := NewWithProvider("0xAbc", Options{}, prov)
	ing.saveCheckpoint(addressCheckpoint{Address: "0xabc", LastSyncedBlock: math.MaxUint64})

	err := ing.Backfill(context.Background())
	if err == nil {
		t.Fatal("expected error for max last_synced_block")
	}
	if !strings.Contains(err.Error(), "last_synced_block at max value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBackfillLoadCheckpointError(t *testing.T) {
	prov := stubCursorProvider{head: 10}
	ing := NewWithProvider("0xabc", Options{ClickHouseDSN: "http://localhost:8123/db"}, prov)
	ing.ch.SetTransport(&cursorRoundTripper{t: t, selectStatus: 500, selectBody: "boom"})
	if err := ing.Backfill(context.Background()); err == nil {
		t.Fatal("expected load checkpoint error")
	}
}

func TestBackfillWaitsForConfirmations(t *testing.T) {
	prov := &panicRangeProvider{head: 10, t: t}
	opts := Options{ClickHouseDSN: "http://localhost:8123/db", Confirmations: 12}
	ing := NewWithProvider("0xabc", opts, prov)
	rt := &cursorRoundTripper{t: t, selectResponse: ""}
	ing.ch.SetTransport(rt)

	if err := ing.Backfill(context.Background()); err != nil {
		t.Fatalf("backfill err: %v", err)
	}
	if len(rt.inserts) != 0 {
		t.Fatalf("expected no checkpoint insert, got %d", len(rt.inserts))
	}
}

func TestBackfillWaitsForConfirmationsWithCheckpoint(t *testing.T) {
	prov := &panicRangeProvider{head: 10, t: t}
	opts := Options{ClickHouseDSN: "http://localhost:8123/db", Confirmations: 12}
	ing := NewWithProvider("0xabc", opts, prov)
	prev := addressCheckpoint{Address: "0xabc", LastSyncedBlock: 5}
	payload, err := json.Marshal(prev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rt := &cursorRoundTripper{t: t, selectResponse: string(payload) + "\n"}
	ing.ch.SetTransport(rt)

	if err := ing.Backfill(context.Background()); err != nil {
		t.Fatalf("backfill err: %v", err)
	}
	if len(rt.inserts) == 0 {
		t.Fatal("expected checkpoint insert")
	}
}

func TestBackfillNoCheckpointNoop(t *testing.T) {
	prov := stubCursorProvider{head: 20}
	ing := NewWithProvider("0xabc", Options{FromBlock: 10, ToBlock: 5, BatchBlocks: 1}, prov)
	if err := ing.Backfill(context.Background()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestBackfillUsesDefaultBatchSize(t *testing.T) {
	prov := &captureProv{head: 120}
	ing := NewWithProvider("0xabc", Options{ClickHouseDSN: "http://localhost:8123/db"}, prov)
	ing.ch.SetTransport(&cursorRoundTripper{t: t})
	if err := ing.Backfill(context.Background()); err != nil {
		t.Fatalf("backfill err: %v", err)
	}
	if len(prov.calls) != 1 {
		t.Fatalf("expected single batch, got %d", len(prov.calls))
	}
	call := prov.calls[0]
	if call.from != 0 || call.to != 120 {
		t.Fatalf("unexpected range: %+v", call)
	}
}

func TestFinalizeBackfillProcessed(t *testing.T) {
	ing := NewWithProvider("0xabc", Options{ClickHouseDSN: "http://localhost:8123/db"}, stubCursorProvider{head: 0})
	rt := &cursorRoundTripper{t: t}
	ing.ch.SetTransport(rt)
	ckpt := addressCheckpoint{Address: "0xabc", LastSyncedBlock: 5}
	if err := ing.finalizeBackfill(context.Background(), ckpt, true, true, 10); err != nil {
		t.Fatalf("finalize err: %v", err)
	}
	if len(rt.inserts) == 0 {
		t.Fatal("expected checkpoint insert")
	}
}

func TestFinalizeBackfillNoCheckpoint(t *testing.T) {
	ing := New("0xabc", Options{})
	if err := ing.finalizeBackfill(context.Background(), addressCheckpoint{}, false, false, 0); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestFinalizeBackfillExistingCheckpoint(t *testing.T) {
	ing := NewWithProvider("0xabc", Options{ClickHouseDSN: "http://localhost:8123/db"}, stubCursorProvider{head: 0})
	rt := &cursorRoundTripper{t: t}
	ing.ch.SetTransport(rt)
	ckpt := addressCheckpoint{Address: "0xabc", LastSyncedBlock: 7}
	if err := ing.finalizeBackfill(context.Background(), ckpt, true, false, 0); err != nil {
		t.Fatalf("finalize err: %v", err)
	}
	if len(rt.inserts) == 0 {
		t.Fatal("expected checkpoint insert")
	}
}
