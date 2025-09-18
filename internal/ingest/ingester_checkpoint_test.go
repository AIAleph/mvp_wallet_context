package ingest

import (
	"context"
	"testing"
)

func TestLoadCheckpointReturnsCachedValue(t *testing.T) {
	ing := New("0xabc", Options{})
	want := addressCheckpoint{Address: "0xabc", LastSyncedBlock: 42, LastBackfillAt: fmtDT64(1000), LastDeltaAt: fmtDT64(2000), UpdatedAt: fmtDT64(3000)}
	ing.saveCheckpoint(want)

	cp, existed, err := ing.loadCheckpoint(context.Background())
	if err != nil {
		t.Fatalf("load checkpoint err: %v", err)
	}
	if !existed {
		t.Fatal("expected cache hit")
	}
	if cp.LastSyncedBlock != want.LastSyncedBlock {
		t.Fatalf("last_synced_block=%d want %d", cp.LastSyncedBlock, want.LastSyncedBlock)
	}
}

func TestLoadCheckpointNormalizesEmptyFields(t *testing.T) {
	ing := NewWithProvider("0xABC", Options{ClickHouseDSN: "http://localhost:8123/db"}, stubCursorProvider{head: 10})
	row := `{"address":"","last_synced_block":5,"last_backfill_at":"","last_delta_at":"","updated_at":""}`
	ing.ch.SetTransport(&cursorRoundTripper{t: t, selectResponse: row + "\n"})

	cp, existed, err := ing.loadCheckpoint(context.Background())
	if err != nil {
		t.Fatalf("load checkpoint err: %v", err)
	}
	if !existed {
		t.Fatal("expected checkpoint from storage")
	}
	if cp.Address != "0xabc" {
		t.Fatalf("address normalized incorrectly: %s", cp.Address)
	}
	if cp.LastBackfillAt != fmtDT64(0) || cp.LastDeltaAt != fmtDT64(0) || cp.UpdatedAt != fmtDT64(0) {
		t.Fatalf("expected default timestamps, got %+v", cp)
	}
}

func TestLoadCheckpointInvalidJSON(t *testing.T) {
	ing := NewWithProvider("0xabc", Options{ClickHouseDSN: "http://localhost:8123/db"}, stubCursorProvider{head: 1})
	ing.ch.SetTransport(&cursorRoundTripper{t: t, selectResponse: "{not-json}\n"})
	if _, _, err := ing.loadCheckpoint(context.Background()); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestFetchCheckpointDecodeError(t *testing.T) {
	ing := NewWithProvider("0xabc", Options{ClickHouseDSN: "http://localhost:8123/db"}, stubCursorProvider{head: 1})
	row := `{"address":"0xabc","last_synced_block":"oops","last_backfill_at":"","last_delta_at":"","updated_at":""}`
	ing.ch.SetTransport(&cursorRoundTripper{t: t, selectResponse: row + "\n"})
	if _, err := ing.fetchCheckpoint(context.Background()); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestFetchCheckpointQueryError(t *testing.T) {
	ing := NewWithProvider("0xabc", Options{ClickHouseDSN: "http://localhost:8123/db"}, stubCursorProvider{head: 1})
	ing.ch.SetTransport(&cursorRoundTripper{t: t, selectStatus: 500, selectBody: "boom"})
	if _, err := ing.fetchCheckpoint(context.Background()); err == nil {
		t.Fatal("expected query error")
	}
}

func TestPersistCheckpointInsertError(t *testing.T) {
	ing := NewWithProvider("0xabc", Options{ClickHouseDSN: "http://localhost:8123/db"}, stubCursorProvider{head: 1})
	rt := &cursorRoundTripper{t: t, insertStatus: 500, insertBody: "fail"}
	ing.ch.SetTransport(rt)
	ckpt := addressCheckpoint{Address: "0xabc"}
	if err := ing.persistCheckpoint(context.Background(), ckpt, checkpointBackfill, 1); err == nil {
		t.Fatal("expected insert error")
	}
}

func TestProcessRangeDevSchemaInsertError(t *testing.T) {
	prov := devSchemaProvider{}
	ing := NewWithProvider("0xabc", Options{ClickHouseDSN: "http://localhost:8123/db", Schema: "dev"}, prov)
	ing.ch.SetTransport(queryFailingTransport{t: t, matcher: "dev_logs"})
	if err := ing.processRange(context.Background(), 5, 5); err == nil {
		t.Fatal("expected insert error")
	}
}
