package eth

import (
    "context"
    "testing"
    "time"
)

func TestNewLimiter_Unlimited(t *testing.T) {
    l := NewLimiter(0)
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
    defer cancel()
    // nop limiter just checks context; not canceled so Wait should return nil quickly
    if err := l.Wait(ctx); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
}

func TestLimiter_Cancel(t *testing.T) {
    l := NewLimiter(1) // 1 req/s
    ctx, cancel := context.WithCancel(context.Background())
    cancel()
    if err := l.Wait(ctx); err == nil {
        t.Fatalf("expected error on canceled context")
    }
}

func TestLimiter_ImmediateTick(t *testing.T) {
    // Very high rate -> period may truncate to 0 -> set to 1ns
    l := NewLimiter(2000000000)
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
    defer cancel()
    if err := l.Wait(ctx); err != nil {
        t.Fatalf("unexpected wait error: %v", err)
    }
}
