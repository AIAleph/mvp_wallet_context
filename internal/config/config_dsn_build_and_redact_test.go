package config

// Covers DSN building with envs and redaction behavior.

import "testing"

func TestBuildClickHouseDSN_EmptyBaseOrDB(t *testing.T) {
    t.Setenv("CLICKHOUSE_DSN", "")
    t.Setenv("CLICKHOUSE_URL", "")
    t.Setenv("CLICKHOUSE_DB", "")
    if got := BuildClickHouseDSN(); got != "" {
        t.Fatalf("expected empty DSN, got %q", got)
    }
}

func TestBuildClickHouseDSN_FallbackInvalidURL(t *testing.T) {
    t.Setenv("CLICKHOUSE_DSN", "")
    t.Setenv("CLICKHOUSE_URL", "http//bad") // missing ':' after http
    t.Setenv("CLICKHOUSE_DB", "x")
    if got := BuildClickHouseDSN(); got != "http//bad/x" {
        t.Fatalf("fallback DSN got %q", got)
    }
}

func TestRedactDSN_NoCredsUnchanged(t *testing.T) {
    if RedactDSN("http://host/db") != "http://host/db" {
        t.Fatalf("unexpected redaction on no-creds")
    }
}

func TestBuildClickHouseDSN_UserPass(t *testing.T) {
    t.Setenv("CLICKHOUSE_DSN", "")
    t.Setenv("CLICKHOUSE_URL", "http://localhost:8123")
    t.Setenv("CLICKHOUSE_DB", "db")
    t.Setenv("CLICKHOUSE_USER", "alice")
    t.Setenv("CLICKHOUSE_PASS", "secret")
    got := BuildClickHouseDSN()
    if got != "http://alice:secret@localhost:8123/db" {
        t.Fatalf("got %q", got)
    }
}

func TestRedactDSN_ParseAndFallback(t *testing.T) {
    out := RedactDSN("http://alice:secret@host/db")
    if out == "" || out == "http://alice:secret@host/db" || out == "http://host/db" {
        t.Fatalf("parse redaction failed: %q", out)
    }
    // Fallback branch when url.Parse cannot parse
    fall := RedactDSN("http//alice:secret@host/db")
    if fall == "" || fall == "http//alice:secret@host/db" {
        t.Fatalf("fallback redaction failed: %q", fall)
    }
}

func TestBuildClickHouseDSN_TrailingSlashAndNoDuplicateDB(t *testing.T) {
    t.Setenv("CLICKHOUSE_DSN", "")
    // Trailing slash
    t.Setenv("CLICKHOUSE_URL", "http://host:8123/")
    t.Setenv("CLICKHOUSE_DB", "db")
    if got := BuildClickHouseDSN(); got != "http://host:8123/db" {
        t.Fatalf("trailing slash got %q", got)
    }
    // Already has db path
    t.Setenv("CLICKHOUSE_URL", "http://host:8123/base/db")
    t.Setenv("CLICKHOUSE_DB", "db")
    if got := BuildClickHouseDSN(); got != "http://host:8123/base/db" {
        t.Fatalf("no-duplicate db got %q", got)
    }
    // Intermediate path with trailing slash
    t.Setenv("CLICKHOUSE_URL", "http://host/base/")
    t.Setenv("CLICKHOUSE_DB", "db")
    if got := BuildClickHouseDSN(); got != "http://host/base/db" {
        t.Fatalf("base trailing got %q", got)
    }
}

func TestRedactDSN_FallbackNoColonCreds(t *testing.T) {
    in := "http//alice@host/db"
    if RedactDSN(in) != in { t.Fatalf("expected unchanged for no-colon creds") }
}

func TestRedactDSN_UserOnlyAndPassOnly(t *testing.T) {
    if out := RedactDSN("http://alice@host/db"); out == "http://alice@host/db" || out == "" {
        t.Fatalf("user-only redaction failed: %q", out)
    }
    if out := RedactDSN("http://:secret@host/db"); out == "http://:secret@host/db" || out == "" {
        t.Fatalf("pass-only redaction failed: %q", out)
    }
}

func TestBuildClickHouseDSN_EnvOverride(t *testing.T) {
    t.Setenv("CLICKHOUSE_DSN", "http://u:p@h:8123/db")
    t.Setenv("CLICKHOUSE_URL", "http://x")
    t.Setenv("CLICKHOUSE_DB", "y")
    if got := BuildClickHouseDSN(); got != "http://u:p@h:8123/db" { t.Fatalf("override got %q", got) }
}

func TestBuildClickHouseDSN_UserOnly(t *testing.T) {
    t.Setenv("CLICKHOUSE_DSN", "")
    t.Setenv("CLICKHOUSE_URL", "http://host")
    t.Setenv("CLICKHOUSE_DB", "db")
    t.Setenv("CLICKHOUSE_USER", "alice")
    t.Setenv("CLICKHOUSE_PASS", "")
    if got := BuildClickHouseDSN(); got != "http://alice@host/db" { t.Fatalf("user-only got %q", got) }
}

func TestBuildClickHouseDSN_FallbackTrailingSlash(t *testing.T) {
    t.Setenv("CLICKHOUSE_DSN", "")
    t.Setenv("CLICKHOUSE_URL", "http//bad/")
    t.Setenv("CLICKHOUSE_DB", "db")
    if got := BuildClickHouseDSN(); got != "http//bad/db" { t.Fatalf("fallback trailing got %q", got) }
}
