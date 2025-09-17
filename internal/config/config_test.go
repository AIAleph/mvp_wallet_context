package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
}

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
	unsetEnv(t, "ETH_PROVIDER_URL")
	unsetEnv(t, "RATE_LIMIT")
	unsetEnv(t, "INGEST_TIMEOUT")
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

func TestLoadClampsValues(t *testing.T) {
	t.Setenv("SYNC_CONFIRMATIONS", "100000")
	t.Setenv("BATCH_BLOCKS", "999999")
	t.Setenv("RATE_LIMIT", "10000")
	t.Setenv("INGEST_TIMEOUT", "7200s")
	cfg := Load()
	if cfg.SyncConfirmations != maxSyncConfirmations {
		t.Fatalf("sync confirmations not clamped: %d", cfg.SyncConfirmations)
	}
	if cfg.BatchBlocks != maxBatchBlocks {
		t.Fatalf("batch not clamped: %d", cfg.BatchBlocks)
	}
	if cfg.RateLimit != maxRateLimit {
		t.Fatalf("rate limit not clamped: %d", cfg.RateLimit)
	}
	if cfg.Timeout != maxIngestTimeout {
		t.Fatalf("timeout not clamped: %v", cfg.Timeout)
	}

	t.Setenv("RATE_LIMIT", "-5")
	t.Setenv("BATCH_BLOCKS", "0")
	t.Setenv("SYNC_CONFIRMATIONS", "-1")
	t.Setenv("INGEST_TIMEOUT", "10ms")
	cfg2 := Load()
	if cfg2.RateLimit != minRateLimit {
		t.Fatalf("negative rate limit not clamped: %d", cfg2.RateLimit)
	}
	if cfg2.BatchBlocks != minBatchBlocks {
		t.Fatalf("batch lower bound not enforced: %d", cfg2.BatchBlocks)
	}
	if cfg2.SyncConfirmations != minSyncConfirmations {
		t.Fatalf("sync confirmations lower bound not enforced: %d", cfg2.SyncConfirmations)
	}
	if cfg2.Timeout != minIngestTimeout {
		t.Fatalf("timeout lower bound not enforced: %v", cfg2.Timeout)
	}
}
