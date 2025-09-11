package config

import "testing"

func TestBuildClickHouseDSN_FallbackParseError_TrailingSlash(t *testing.T) {
    t.Setenv("CLICKHOUSE_DSN", "")
    t.Setenv("CLICKHOUSE_URL", "http://[/")
    t.Setenv("CLICKHOUSE_DB", "db")
    if got := BuildClickHouseDSN(); got != "http://[/db" { t.Fatalf("got %q", got) }
}

func TestRedactDSN_ParseSuccess_NoColonCredsInPath_Unchanged(t *testing.T) {
    in := "http://host/u@db"
    if out := RedactDSN(in); out != in { t.Fatalf("expected unchanged, got %q", out) }
}

func TestRedactDSN_ParseFail_NoAt_Unchanged(t *testing.T) {
    in := "http://["
    if out := RedactDSN(in); out != in { t.Fatalf("expected unchanged, got %q", out) }
}

