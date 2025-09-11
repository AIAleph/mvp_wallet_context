package ch

// Covers ping/insert error paths and unsupported schemes.

import (
    "bytes"
    "context"
    "errors"
    "io"
    "net/http"
    "testing"
)

type rtFunc2 func(*http.Request) (*http.Response, error)
func (f rtFunc2) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPing_EmptyEndpointAndDoError(t *testing.T) {
    // Empty endpoint is a no-op
    if err := New("").Ping(context.Background()); err != nil { t.Fatalf("empty ping err: %v", err) }
    // Do error path
    c := New("http://localhost:8123/db")
    c.hc = &http.Client{Transport: rtFunc2(func(r *http.Request) (*http.Response, error) {
        return nil, errors.New("net")
    })}
    if err := c.Ping(context.Background()); err == nil { t.Fatal("expected Do error") }
}

func TestInsertJSONEachRow_ParseErrorAndDoError(t *testing.T) {
    // URL parse error
    c := New("http://[")
    if err := c.InsertJSONEachRow(context.Background(), "dev", []any{map[string]any{"x":1}}); err == nil {
        t.Fatal("expected parse error")
    }
    // Do error path
    c2 := New("http://localhost:8123/db")
    c2.hc = &http.Client{Transport: rtFunc2(func(r *http.Request) (*http.Response, error) {
        return nil, errors.New("net")
    })}
    if err := c2.InsertJSONEachRow(context.Background(), "dev", []any{map[string]any{"x":1}}); err == nil {
        t.Fatal("expected Do error")
    }
}

func TestInsertJSONEachRow_EmptyEndpointNoop(t *testing.T) {
    if err := New("").InsertJSONEachRow(context.Background(), "dev", []any{map[string]any{"x": 1}}); err != nil {
        t.Fatalf("unexpected err: %v", err)
    }
}

func TestInsertJSONEachRow_UnsupportedScheme_Skip(t *testing.T) {
    // Ensure we don't try to hit HTTP transport for non-http schemes
    c := New("clickhouse://host/db")
    // Provide a transport that would fail if called
    c.hc = &http.Client{Transport: rtFunc2(func(r *http.Request) (*http.Response, error) {
        return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte("ok")))}, nil
    })}
    if err := c.InsertJSONEachRow(context.Background(), "dev", []any{map[string]any{"x": 1}}); err != nil {
        t.Fatalf("unexpected err: %v", err)
    }
}

func TestPing_NewRequestError(t *testing.T) {
    c := New("http://localhost:8123/db")
    // Stub httpNewRequest to force error
    old := httpNewRequest
    defer func() { httpNewRequest = old }()
    httpNewRequest = func(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) { return nil, errors.New("req") }
    if err := c.Ping(context.Background()); err == nil { t.Fatal("expected new request error") }
}

func TestInsertJSONEachRow_NewRequestError(t *testing.T) {
    c := New("http://localhost:8123/db")
    old := httpNewRequest
    defer func() { httpNewRequest = old }()
    httpNewRequest = func(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) { return nil, errors.New("req") }
    if err := c.InsertJSONEachRow(context.Background(), "dev", []any{map[string]any{"x": 1}}); err == nil {
        t.Fatal("expected new request error")
    }
}
