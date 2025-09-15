package ingest

import "testing"

func TestSchemaMode_DefaultsAndUnknown(t *testing.T) {
    if got := New("0x", Options{}).SchemaMode(); got != "dev" {
        t.Fatalf("SchemaMode default got %q, want dev", got)
    }
    if got := New("0x", Options{Schema: "canonical"}).SchemaMode(); got != "canonical" {
        t.Fatalf("SchemaMode canonical got %q, want canonical", got)
    }
    if got := New("0x", Options{Schema: "something"}).SchemaMode(); got != "dev" {
        t.Fatalf("SchemaMode unknown got %q, want dev", got)
    }
}

func TestFmtDT64_Edges(t *testing.T) {
    if got := fmtDT64(0); got != "1970-01-01 00:00:00.000" {
        t.Fatalf("fmtDT64(0) = %q", got)
    }
    if got := fmtDT64(1_234); got == "1970-01-01 00:00:00.000" {
        t.Fatalf("fmtDT64(>0) unexpected epoch: %q", got)
    }
}

