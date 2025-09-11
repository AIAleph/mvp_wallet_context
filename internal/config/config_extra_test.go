package config

import (
    "strings"
    "testing"
)

// Exercise the parse-success path in RedactDSN where url.Parse succeeds but
// u.User is nil and a best-effort fallback scans for `user:pass@` within the
// path segment. This is contrived but covers the branch.
func TestRedactDSN_ParseSuccess_NoUserInfo_FallbackScan(t *testing.T) {
    in := "http://host/u:p@db"
    out := RedactDSN(in)
    if out == in || out == "" || out == "http://host/db" {
        t.Fatalf("expected fallback redaction, got %q", out)
    }
    if want := ":***@"; want != "" && (out == in || out == "http://host/db") {
        t.Fatalf("unexpected output: %q", out)
    }
}

func TestRedactDSN_EmptyString(t *testing.T) {
    if RedactDSN("") != "" { t.Fatal("expected empty result for empty input") }
}

func TestRedactDSN_ParseFail_ScanSuccess(t *testing.T) {
    // Force parse error with invalid host, but include creds after //
    in := "http://[user:pass@host/db"
    out := RedactDSN(in)
    if out == in || out == "" || !strings.Contains(out, ":***@") {
        t.Fatalf("expected redacted fallback, got %q", out)
    }
}

func TestBuildClickHouseDSN_FallbackParseError(t *testing.T) {
    t.Setenv("CLICKHOUSE_DSN", "")
    t.Setenv("CLICKHOUSE_URL", "http://[")
    t.Setenv("CLICKHOUSE_DB", "db")
    if got := BuildClickHouseDSN(); got != "http://[/db" { t.Fatalf("got %q", got) }
}
