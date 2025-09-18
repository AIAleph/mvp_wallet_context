package ch

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestQueryJSONEachRow_Success(t *testing.T) {
	c := New("http://localhost:8123/db")
	var gotQuery string
	payload := "{\"address\":\"0x1\"}\n{\"address\":\"0x2\"}\n"
	c.hc = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		u, _ := url.Parse(r.URL.String())
		gotQuery = u.Query().Get("query")
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte(payload)))}, nil
	})}
	rows, err := c.QueryJSONEachRow(context.Background(), "SELECT 1 FORMAT JSONEachRow")
	if err != nil {
		t.Fatalf("query err: %v", err)
	}
	if !strings.Contains(gotQuery, "SELECT 1") {
		t.Fatalf("query missing SELECT: %s", gotQuery)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if string(rows[0]) != "{\"address\":\"0x1\"}" {
		t.Fatalf("unexpected first row: %s", rows[0])
	}
}

func TestQueryJSONEachRow_NoEndpointAndHTTPError(t *testing.T) {
	c := New("")
	rows, err := c.QueryJSONEachRow(context.Background(), "SELECT 1")
	if err != nil {
		t.Fatalf("noop err: %v", err)
	}
	if rows != nil {
		t.Fatalf("expected nil rows, got %v", rows)
	}

	c2 := New("http://localhost:8123/db")
	c2.hc = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader([]byte("boom")))}, nil
	})}
	if _, err := c2.QueryJSONEachRow(context.Background(), "SELECT 1 FORMAT JSONEachRow"); err == nil {
		t.Fatal("expected error")
	}
}

func TestQueryJSONEachRow_RequestCreationError(t *testing.T) {
	c := New("http://localhost:8123/db")
	orig := httpNewRequest
	defer func() { httpNewRequest = orig }()
	httpNewRequest = func(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
		return nil, errors.New("boom")
	}
	if _, err := c.QueryJSONEachRow(context.Background(), "SELECT 1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestQueryJSONEachRow_UnsupportedScheme(t *testing.T) {
	c := New("tcp://localhost:9000/db")
	rows, err := c.QueryJSONEachRow(context.Background(), "SELECT 1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if rows != nil {
		t.Fatalf("expected nil rows, got %v", rows)
	}
}

func TestQueryJSONEachRow_RetrySucceeds(t *testing.T) {
	c := New("http://localhost:8123/db")
	var attempts int32
	responses := []struct {
		status int
		body   string
	}{
		{status: 500, body: "fail"},
		{status: 200, body: "{\"address\":\"0x1\"}\n"},
	}
	c.hc = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		idx := int(atomic.AddInt32(&attempts, 1) - 1)
		resp := responses[idx]
		return &http.Response{StatusCode: resp.status, Body: io.NopCloser(strings.NewReader(resp.body))}, nil
	})}
	rows, err := c.QueryJSONEachRow(context.Background(), "SELECT 1 FORMAT JSONEachRow")
	if err != nil {
		t.Fatalf("query err: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestQueryJSONEachRow_BadEndpointParse(t *testing.T) {
	c := New("::")
	if _, err := c.QueryJSONEachRow(context.Background(), "SELECT 1"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestQueryJSONEachRow_TransportError(t *testing.T) {
	c := New("http://localhost:8123/db")
	c.hc = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("boom")
	})}
	if _, err := c.QueryJSONEachRow(context.Background(), "SELECT 1"); err == nil {
		t.Fatal("expected transport error")
	}
}

func TestQueryJSONEachRow_DecodeError(t *testing.T) {
	c := New("http://localhost:8123/db")
	c.hc = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		payload := "{\"address\":\"0x1\"}\n{" // truncated JSON to trigger decode error
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(payload))}, nil
	})}
	if _, err := c.QueryJSONEachRow(context.Background(), "SELECT 1"); err == nil {
		t.Fatal("expected decode error")
	}
}
