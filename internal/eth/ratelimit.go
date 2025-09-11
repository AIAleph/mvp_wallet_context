package eth

import (
    "context"
    "time"
)

// Limiter is a minimal interface to rate-limit RPC calls.
type Limiter interface {
    Wait(ctx context.Context) error
}

// nopLimiter allows unlimited throughput.
type nopLimiter struct{}

func (nopLimiter) Wait(ctx context.Context) error { return ctx.Err() }

// qpsLimiter issues 1 token every tick to approximate QPS limiting.
type qpsLimiter struct{
    ch <-chan time.Time
}

func (l qpsLimiter) Wait(ctx context.Context) error {
    select {
    case <-ctx.Done():
        return ctx.Err()
    case <-l.ch:
        return nil
    }
}

// NewLimiter returns a Limiter enforcing req/s. If rate <= 0, returns unlimited.
func NewLimiter(rate int) Limiter {
    if rate <= 0 {
        return nopLimiter{}
    }
    // Avoid division by zero; cap to 1ns minimum period though realistic rates are low.
    period := time.Second / time.Duration(rate)
    if period <= 0 { period = time.Nanosecond }
    // Use NewTicker so callers could stop it in extended implementations
    // (here we expose only the channel; limiter is expected to be long-lived).
    t := time.NewTicker(period)
    return qpsLimiter{ch: t.C}
}
