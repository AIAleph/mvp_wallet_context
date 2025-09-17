package ch

import (
    "bytes"
    "context"
    "io"
    "net/http"
    "sync/atomic"
    "testing"
)

func TestInsertJSONEachRow_RetriesThenSucceeds(t *testing.T) {
    c := New("http://localhost:8123/db")
    var calls int32
    c.hc = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
        atomic.AddInt32(&calls, 1)
        if atomic.LoadInt32(&calls) < 2 { // first call fails
            return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader([]byte("err")))}, nil
        }
        return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte("ok")))}, nil
    })}
    if err := c.InsertJSONEachRow(context.Background(), "dev", []any{map[string]any{"x": 1}}); err != nil {
        t.Fatalf("unexpected err after retry: %v", err)
    }
    if atomic.LoadInt32(&calls) < 2 { t.Fatalf("expected at least 2 attempts, got %d", calls) }
}

func TestPing_RetryOn5xx(t *testing.T) {
    c := New("http://localhost:8123/db")
    var calls int32
    c.hc = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
        atomic.AddInt32(&calls, 1)
        if atomic.LoadInt32(&calls) < 2 {
            return &http.Response{StatusCode: 502, Body: io.NopCloser(bytes.NewReader([]byte("bad")))}, nil
        }
        return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte("ok")))}, nil
    })}
    if err := c.Ping(context.Background()); err != nil {
        t.Fatalf("unexpected ping err: %v", err)
    }
}

func TestInsertJSONEachRow_NonRetriableStatus(t *testing.T) {
    c := New("http://localhost:8123/db")
    c.hc = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
        return &http.Response{StatusCode: 400, Body: io.NopCloser(bytes.NewReader([]byte("bad req")))}, nil
    })}
    if err := c.InsertJSONEachRow(context.Background(), "dev", []any{map[string]any{"x": 1}}); err == nil {
        t.Fatal("expected non-retriable error")
    }
}

