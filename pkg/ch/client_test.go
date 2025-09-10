package ch

import (
    "context"
    "testing"
)

func TestClientPing(t *testing.T) {
    c := New("clickhouse://localhost")
    if c == nil { t.Fatal("New returned nil") }
    if err := c.Ping(context.Background()); err != nil {
        t.Fatalf("Ping error: %v", err)
    }
}

