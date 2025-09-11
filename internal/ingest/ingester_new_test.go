package ingest

import "testing"

func TestNew_ConstructsCHClientVariants(t *testing.T) {
    if ing := New("0x", Options{}); ing == nil || ing.ch == nil { t.Fatal("nil ch client") }
    if ing := New("0x", Options{ClickHouseDSN: "http://h/db"}); ing == nil || ing.ch == nil { t.Fatal("nil ch client with DSN") }
}

func TestNewWithProvider_ConstructsCHClientVariants(t *testing.T) {
    p := &tsProv{}
    if ing := NewWithProvider("0x", Options{}, p); ing == nil || ing.ch == nil { t.Fatal("nil ch client") }
    if ing := NewWithProvider("0x", Options{ClickHouseDSN: "http://h/db"}, p); ing == nil || ing.ch == nil { t.Fatal("nil ch client with DSN") }
}

