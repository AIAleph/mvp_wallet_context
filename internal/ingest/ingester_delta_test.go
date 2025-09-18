package ingest

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestDeltaReprocessesReorgWindowAndUpdatesCursor(t *testing.T) {
	prov := &captureProv{head: 140}
	opts := Options{ClickHouseDSN: "http://localhost:8123/db", Confirmations: 12, BatchBlocks: 200}
	ing := NewWithProvider("0xabc", opts, prov)

	prev := addressCheckpoint{
		Address:         "0xabc",
		LastSyncedBlock: 100,
		LastBackfillAt:  fmtDT64(1_000),
		LastDeltaAt:     fmtDT64(2_000),
		UpdatedAt:       fmtDT64(3_000),
	}
	payload, err := json.Marshal(prev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rt := &cursorRoundTripper{t: t, selectResponse: string(payload) + "\n"}
	ing.ch.SetTransport(rt)

	if err := ing.Delta(context.Background()); err != nil {
		t.Fatalf("delta err: %v", err)
	}
	if len(prov.calls) == 0 {
		t.Fatal("expected GetLogs call")
	}
	if len(prov.txCalls) == 0 {
		t.Fatal("expected Transactions call")
	}
	if got := prov.calls[0].from; got != 89 {
		t.Fatalf("expected reorg start 89, got %d", got)
	}
	if got := prov.txCalls[0].from; got != 89 {
		t.Fatalf("expected tx reorg start 89, got %d", got)
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
	}
	if err := json.Unmarshal([]byte(line), &row); err != nil {
		t.Fatalf("decode insert: %v", err)
	}
	if want := uint64(128); row.LastSyncedBlock != want {
		t.Fatalf("last_synced_block=%d want %d", row.LastSyncedBlock, want)
	}
	if row.LastBackfillAt != prev.LastBackfillAt {
		t.Fatalf("backfill timestamp changed: %s", row.LastBackfillAt)
	}
	if row.LastDeltaAt == prev.LastDeltaAt || row.LastDeltaAt == fmtDT64(0) {
		t.Fatalf("delta timestamp not updated: %s", row.LastDeltaAt)
	}
}

func TestDeltaClampsLastSyncedToSafeHead(t *testing.T) {
	defer withTimeNow(t, time.UnixMilli(6_000))()

	prov := &captureProv{head: 52}
	opts := Options{ClickHouseDSN: "http://localhost:8123/db", Confirmations: 12, BatchBlocks: 500}
	ing := NewWithProvider("0xdef", opts, prov)

	prevCheckpoint := addressCheckpoint{Address: "0xdef", LastSyncedBlock: 50, LastBackfillAt: fmtDT64(1_000), LastDeltaAt: fmtDT64(2_000), UpdatedAt: fmtDT64(3_000)}
	payload, err := json.Marshal(prevCheckpoint)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rt := &cursorRoundTripper{t: t, selectResponse: string(payload) + "\n"}
	ing.ch.SetTransport(rt)

	if err := ing.Delta(context.Background()); err != nil {
		t.Fatalf("delta err: %v", err)
	}
	if len(prov.calls) == 0 {
		t.Fatal("expected GetLogs call")
	}
	if len(prov.txCalls) == 0 {
		t.Fatal("expected Transactions call")
	}
	if prov.calls[0].from != 29 || prov.calls[0].to != 40 {
		t.Fatalf("unexpected range: %+v", prov.calls[0])
	}
	if prov.txCalls[0].from != 29 || prov.txCalls[0].to != 40 {
		t.Fatalf("unexpected tx range: %+v", prov.txCalls[0])
	}
	if len(rt.inserts) == 0 {
		t.Fatal("expected addresses insert")
	}
	var row addressCheckpoint
	if err := json.Unmarshal([]byte(strings.TrimSpace(rt.inserts[len(rt.inserts)-1])), &row); err != nil {
		t.Fatalf("decode insert: %v", err)
	}
	if row.LastSyncedBlock != 40 {
		t.Fatalf("synced block=%d", row.LastSyncedBlock)
	}
	if row.LastDeltaAt != fmtDT64(6_000) {
		t.Fatalf("delta timestamp not updated: %s", row.LastDeltaAt)
	}
	if row.LastBackfillAt != prevCheckpoint.LastBackfillAt {
		t.Fatalf("backfill timestamp changed: %s", row.LastBackfillAt)
	}
}

func TestDeltaLoadCheckpointError(t *testing.T) {
	prov := stubCursorProvider{head: 10}
	ing := NewWithProvider("0xabc", Options{ClickHouseDSN: "http://localhost:8123/db"}, prov)
	ing.ch.SetTransport(&cursorRoundTripper{t: t, selectStatus: 500, selectBody: "boom"})
	if err := ing.Delta(context.Background()); err == nil {
		t.Fatal("expected load checkpoint error")
	}
}

func TestDeltaNoCheckpointNoop(t *testing.T) {
	prov := stubCursorProvider{head: 30}
	ing := NewWithProvider("0xabc", Options{FromBlock: 40, ToBlock: 20}, prov)
	if err := ing.Delta(context.Background()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestDeltaWaitsForConfirmations(t *testing.T) {
	prov := &panicRangeProvider{head: 10, t: t}
	opts := Options{ClickHouseDSN: "http://localhost:8123/db", Confirmations: 12}
	ing := NewWithProvider("0xabc", opts, prov)
	ing.saveCheckpoint(addressCheckpoint{Address: "0xabc", LastSyncedBlock: 5})
	rt := &cursorRoundTripper{t: t, selectResponse: ""}
	ing.ch.SetTransport(rt)

	if err := ing.Delta(context.Background()); err != nil {
		t.Fatalf("delta err: %v", err)
	}
	if len(rt.inserts) == 0 {
		t.Fatal("expected checkpoint timestamp update")
	}
}

func TestDeltaPersistWhenNoRangeButCheckpointExists(t *testing.T) {
	prov := stubCursorProvider{head: 20}
	ing := NewWithProvider("0xabc", Options{ClickHouseDSN: "http://localhost:8123/db", ToBlock: 10}, prov)
	ing.saveCheckpoint(addressCheckpoint{Address: "0xabc", LastSyncedBlock: 20})
	rt := &cursorRoundTripper{t: t}
	ing.ch.SetTransport(rt)
	if err := ing.Delta(context.Background()); err != nil {
		t.Fatalf("delta err: %v", err)
	}
	if len(rt.inserts) == 0 {
		t.Fatal("expected checkpoint insert")
	}
}

func TestDeltaWithoutConfirmationsAdvancesFromCheckpoint(t *testing.T) {
	defer withTimeNow(t, time.UnixMilli(8_000))()

	prov := &captureProv{head: 90}
	opts := Options{ClickHouseDSN: "http://localhost:8123/db", BatchBlocks: 100}
	ing := NewWithProvider("0xfeed", opts, prov)

	prevCheckpoint := addressCheckpoint{Address: "0xfeed", LastSyncedBlock: 40, LastBackfillAt: fmtDT64(1_000), LastDeltaAt: fmtDT64(2_000), UpdatedAt: fmtDT64(3_000)}
	payload, err := json.Marshal(prevCheckpoint)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rt := &cursorRoundTripper{t: t, selectResponse: string(payload) + "\n"}
	ing.ch.SetTransport(rt)

	if err := ing.Delta(context.Background()); err != nil {
		t.Fatalf("delta err: %v", err)
	}
	if len(prov.calls) == 0 {
		t.Fatal("expected GetLogs call")
	}
	if prov.calls[0].from != 41 {
		t.Fatalf("expected start from 41, got %d", prov.calls[0].from)
	}
	if len(rt.inserts) == 0 {
		t.Fatal("expected addresses insert")
	}
	var row addressCheckpoint
	if err := json.Unmarshal([]byte(strings.TrimSpace(rt.inserts[len(rt.inserts)-1])), &row); err != nil {
		t.Fatalf("decode insert: %v", err)
	}
	if row.LastSyncedBlock != 90 {
		t.Fatalf("last_synced_block=%d", row.LastSyncedBlock)
	}
	if row.LastDeltaAt != fmtDT64(8_000) {
		t.Fatalf("delta timestamp not updated: %s", row.LastDeltaAt)
	}
}

func TestDeltaMaxUintCheckpointError(t *testing.T) {
	ing := NewWithProvider("0xdead", Options{ClickHouseDSN: "http://localhost:8123/db"}, maxHeadProvider{})
	prev := addressCheckpoint{Address: "0xdead", LastSyncedBlock: math.MaxUint64}
	payload, err := json.Marshal(prev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	ing.ch.SetTransport(&cursorRoundTripper{t: t, selectResponse: string(payload) + "\n"})

	if err := ing.Delta(context.Background()); err == nil {
		t.Fatal("expected delta error for max last_synced_block")
	}
}
