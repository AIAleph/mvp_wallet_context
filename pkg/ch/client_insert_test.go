package ch

import (
    "bytes"
    "context"
    "io"
    "net/http"
    "net/url"
    "strings"
    "testing"
)

type rtFunc func(*http.Request) (*http.Response, error)
func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPingHTTPAndNoopSchemes(t *testing.T) {
    // No-op scheme: clickhouse:// should early return
    c := New("clickhouse://localhost/db")
    if err := c.Ping(context.Background()); err != nil { t.Fatalf("noop ping err: %v", err) }

    // HTTP success
    c2 := New("http://localhost:8123/db")
    c2.hc = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
        if r.Method != http.MethodGet { t.Fatalf("method=%s", r.Method) }
        u, _ := url.Parse(r.URL.String())
        if q := u.Query().Get("query"); !strings.Contains(q, "SELECT 1") { t.Fatalf("query=%s", q) }
        return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte("1\n")))}, nil
    })}
    if err := c2.Ping(context.Background()); err != nil { t.Fatalf("http ping err: %v", err) }
}

func TestInsertJSONEachRowEncodesAndSanitizes(t *testing.T) {
    c := New("http://localhost:8123/db")
    var gotQuery string
    var body bytes.Buffer
    c.hc = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
        u, _ := url.Parse(r.URL.String())
        gotQuery = u.Query().Get("query")
        io.Copy(&body, r.Body)
        return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte("ok")))}, nil
    })}
    rows := []any{map[string]any{"a": 1}, map[string]any{"b": 2}}
    // Intentionally include unsafe chars in table name
    if err := c.InsertJSONEachRow(context.Background(), "dev;drop table", rows); err != nil {
        t.Fatalf("insert err: %v", err)
    }
    if strings.Contains(gotQuery, ";") { t.Fatalf("query not sanitized: %s", gotQuery) }
    lines := strings.Split(strings.TrimSpace(body.String()), "\n")
    if len(lines) != 2 { t.Fatalf("expected 2 lines, got %d", len(lines)) }
}

func TestInsertJSONEachRow_HTTPErrorAndEmptyRows(t *testing.T) {
    c := New("http://localhost:8123/db")
    c.hc = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
        return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader([]byte("oops")))}, nil
    })}
    if err := c.InsertJSONEachRow(context.Background(), "dev", []any{map[string]any{"x": 1}}); err == nil {
        t.Fatal("expected error")
    }
    // Empty rows are no-op
    if err := c.InsertJSONEachRow(context.Background(), "dev", nil); err != nil { t.Fatalf("empty rows err: %v", err) }
}

func TestInsertJSONEachRow_UnsupportedSchemeNoop(t *testing.T) {
    c := New("clickhouse://host/db")
    // Should be no-op, not calling HTTP
    if err := c.InsertJSONEachRow(context.Background(), "dev", []any{map[string]any{"x": 1}}); err != nil {
        t.Fatalf("unexpected err: %v", err)
    }
}

func TestPing_HTTPErrorPath(t *testing.T) {
    c := New("http://localhost:8123/db")
    c.hc = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
        return &http.Response{StatusCode: 404, Body: io.NopCloser(bytes.NewReader([]byte("no")))}, nil
    })}
    if err := c.Ping(context.Background()); err == nil { t.Fatal("expected error") }
}

func TestPing_BadURLParse(t *testing.T) {
    c := New("http://[")
    if err := c.Ping(context.Background()); err == nil {
        t.Fatal("expected parse error")
    }
}

func TestInsertJSONEachRow_EncodeError(t *testing.T) {
    c := New("http://localhost:8123/db")
    // Force JSON encode error with unsupported type
    bad := make(chan int)
    if err := c.InsertJSONEachRow(context.Background(), "dev", []any{bad}); err == nil {
        t.Fatal("expected encode error")
    }
}
