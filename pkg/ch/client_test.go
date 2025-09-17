package ch

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestClientPing(t *testing.T) {
	c := New("clickhouse://localhost")
	if c == nil {
		t.Fatal("New returned nil")
	}
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping error: %v", err)
	}
}

func TestRequestContext(t *testing.T) {
	c := New("http://localhost:8123/db")
	base := context.Background()
	ctx, cancel := c.requestContext(base)
	if ctx == base {
		t.Fatalf("expected derived context when no deadline present")
	}
	if deadline, ok := ctx.Deadline(); !ok || deadline.Before(time.Now()) {
		t.Fatalf("expected future deadline, got %v ok=%v", deadline, ok)
	}
	cancel()

	withDeadline, cancelBase := context.WithTimeout(context.Background(), time.Second)
	defer cancelBase()
	ctx2, cancel2 := c.requestContext(withDeadline)
	if ctx2 != withDeadline {
		t.Fatalf("expected original context when deadline already set")
	}
	cancel2()
}

func TestSetTransport(t *testing.T) {
	c := New("http://localhost:8123/db")
	c.SetTransport(nil) // should be a no-op
	c = New("http://localhost:8123/db")
	var called bool
	c.SetTransport(rtFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	}))
	rows := []any{map[string]any{"x": 1}}
	if err := c.InsertJSONEachRow(context.Background(), "foo", rows); err != nil {
		t.Fatalf("InsertJSONEachRow returned error: %v", err)
	}
	if !called {
		t.Fatalf("expected custom transport to be invoked")
	}
}
