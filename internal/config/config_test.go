package config

import (
    "os"
    "strings"
    "testing"
    "time"
)

func TestBuildClickHouseDSN_PrefersDSN(t *testing.T) {
    t.Setenv("CLICKHOUSE_DSN", "http://user:pass@host:8123/db")
    t.Setenv("CLICKHOUSE_URL", "http://localhost:8123")
    t.Setenv("CLICKHOUSE_DB", "ignored")
    if got := BuildClickHouseDSN(); got != "http://user:pass@host:8123/db" {
        t.Fatalf("got %q", got)
    }
}

func TestBuildClickHouseDSN_FromParts(t *testing.T) {
    t.Setenv("CLICKHOUSE_DSN", "")
    t.Setenv("CLICKHOUSE_URL", "http://localhost:8123")
    t.Setenv("CLICKHOUSE_DB", "wallets")
    t.Setenv("CLICKHOUSE_USER", "alice")
    t.Setenv("CLICKHOUSE_PASS", "s3cr3t")
    got := BuildClickHouseDSN()
    if !strings.Contains(got, "alice:@") && !strings.Contains(got, "alice:s3cr3t@") {
        t.Fatalf("expected credentials in DSN, got %q", got)
    }
    if !strings.HasSuffix(got, "/wallets") {
        t.Fatalf("expected db suffix, got %q", got)
    }
}

func TestRedactDSN(t *testing.T) {
    in := "http://alice:s3cr3t@host:8123/db"
    out := RedactDSN(in)
    if strings.Contains(out, "s3cr3t") {
        t.Fatalf("password not redacted: %q", out)
    }
}

func TestLoadDefaultsAndOverrides(t *testing.T) {
    // Clear all to defaults
    os.Unsetenv("ETH_PROVIDER_URL")
    os.Unsetenv("RATE_LIMIT")
    os.Unsetenv("INGEST_TIMEOUT")
    cfg := Load()
    if cfg.ProviderURL != "" || cfg.RateLimit != 0 || cfg.Timeout != 30*time.Second {
        t.Fatalf("unexpected defaults: %+v", cfg)
    }
    // Overrides
    t.Setenv("ETH_PROVIDER_URL", "https://rpc")
    t.Setenv("RATE_LIMIT", "10")
    t.Setenv("INGEST_TIMEOUT", "150ms")
    cfg2 := Load()
    if cfg2.ProviderURL != "https://rpc" || cfg2.RateLimit != 10 || cfg2.Timeout != 150*time.Millisecond {
        t.Fatalf("overrides not applied: %+v", cfg2)
    }
}

