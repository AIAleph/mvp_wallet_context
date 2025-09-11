package config

import (
    "testing"
    "time"
)

func TestParseIntEnvVariants(t *testing.T) {
    t.Setenv("X_OK", "42")
    if parseIntEnv("X_OK", 0) != 42 { t.Fatal("ok parse") }
    t.Setenv("X_BAD", "no")
    if parseIntEnv("X_BAD", 7) != 7 { t.Fatal("bad default") }
    if parseIntEnv("X_MISSING", 9) != 9 { t.Fatal("missing default") }
}

func TestParseDurEnvVariants(t *testing.T) {
    t.Setenv("D_OK", "250ms")
    if parseDurEnv("D_OK", time.Second) != 250*time.Millisecond { t.Fatal("dur ok") }
    t.Setenv("D_BAD", "nope")
    if parseDurEnv("D_BAD", time.Second) != time.Second { t.Fatal("dur bad") }
    if parseDurEnv("D_MISS", 2*time.Second) != 2*time.Second { t.Fatal("dur missing") }
}

func TestBuildClickHouseDSN_PathAppend(t *testing.T) {
    t.Setenv("CLICKHOUSE_DSN", "")
    t.Setenv("CLICKHOUSE_URL", "http://host:8123/base")
    t.Setenv("CLICKHOUSE_DB", "wallets")
    got := BuildClickHouseDSN()
    if got != "http://host:8123/base/wallets" { t.Fatalf("got %q", got) }
}

func TestRedactDSN_UserOnly(t *testing.T) {
    got := RedactDSN("http://alice@host/db")
    if got == "http://alice@host/db" || got == "" { t.Fatalf("not redacted: %q", got) }
}

