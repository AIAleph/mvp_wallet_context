package eth

import (
    "net/http"
    "strings"
    "time"
)

// NewProvider constructs a concrete Provider for the given endpoint and wraps it
// with a rate limiter. For now, it returns a minimal stub for http(s) endpoints.
// Validation is centralized in NewHTTPProvider (after trimming whitespace) to keep
// behavior in one place. When real adapters are added (Alchemy/Infura/etc.),
// switch on host/scheme here and retain centralized validation.
func NewProvider(endpoint string, rateLimit int, retries int, backoff time.Duration) (Provider, error) {
    // Validate via concrete provider constructor to keep single source of truth
    base, err := NewHTTPProvider(strings.TrimSpace(endpoint), &http.Client{})
    if err != nil { return nil, err }
    // Tune HTTP retries/backoff if supported
    if hp, ok := base.(*httpProvider); ok {
        if retries > 0 { hp.maxRetries = retries }
        if backoff > 0 { hp.backoffBase = backoff }
    }
    return WrapWithLimiter(base, NewLimiter(rateLimit)), nil
}
