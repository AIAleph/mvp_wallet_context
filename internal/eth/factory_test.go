package eth

import (
    "testing"
    "time"
)

func TestFactory_WrapsLimiterAndValidates(t *testing.T) {
    if _, err := NewProvider("", 1, 0, 0); err == nil {
        t.Fatal("expected error for empty endpoint")
    }
    p, err := NewProvider("http://localhost:8545", 5, 3, 50*time.Millisecond)
    if err != nil { t.Fatalf("unexpected err: %v", err) }
    if _, ok := p.(RLProvider); !ok {
        t.Fatalf("expected RLProvider wrapper, got %T", p)
    }
}
