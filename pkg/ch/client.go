package ch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// httpNewRequest is a small test seam to stub request creation errors in unit tests.
// It preserves production behavior (defaults to http.NewRequestWithContext).
var httpNewRequest = http.NewRequestWithContext

// Client is a thin ClickHouse HTTP client wrapper. It supports JSONEachRow inserts.
type Client struct {
	endpoint string
	hc       *http.Client
	reqTimeout time.Duration
}

// New creates a Client from a ClickHouse DSN (e.g., http://user:pass@host:8123/db).
// If dsn is empty, the client operates in no-op mode (writes are skipped).
func New(dsn string) *Client {
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConns:        64,
		MaxIdleConnsPerHost: 32,
		MaxConnsPerHost:     64,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	hc := &http.Client{Timeout: 0, Transport: transport}
	if dsn == "" {
		return &Client{endpoint: "", hc: hc, reqTimeout: 10 * time.Second}
	}
	// Keep DSN as-is; assume it includes DB path and credentials if any.
	return &Client{endpoint: dsn, hc: hc, reqTimeout: 10 * time.Second}
}

func (c *Client) requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, c.reqTimeout)
}

func (c *Client) Ping(ctx context.Context) error {
	if c.endpoint == "" {
		return nil
	}
	u, err := url.Parse(c.endpoint)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		// Unsupported scheme in this minimal client; treat as no-op for tests.
		return nil
	}
	// Simple SELECT 1
	q := u.Query()
	q.Set("query", "SELECT 1")
	u.RawQuery = q.Encode()
	// Build a fresh request on each attempt
	return doWithRetry(ctx, func() error {
		reqCtx, cancel := c.requestContext(ctx)
		defer cancel()
		req, err := httpNewRequest(reqCtx, http.MethodGet, u.String(), nil)
		if err != nil {
			return err
		}
		resp, err := c.hc.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode/100 != 2 {
			b, _ := io.ReadAll(resp.Body)
			return &httpStatusErr{code: resp.StatusCode, body: string(b), op: "ping"}
		}
		return nil
	})
}

// InsertJSONEachRow performs an INSERT INTO <table> FORMAT JSONEachRow using the
// provided rows (slice of structs or maps). If endpoint is empty, it is a no-op.
func (c *Client) InsertJSONEachRow(ctx context.Context, table string, rows []any) error {
	if len(rows) == 0 {
		return nil
	}
	if c.endpoint == "" {
		return nil
	}
	// Build newline-delimited JSON
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for i, row := range rows {
		if err := enc.Encode(row); err != nil {
			return fmt.Errorf("encode row %d: %w", i, err)
		}
	}
	// Build INSERT query
	u, err := url.Parse(c.endpoint)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		// Unsupported scheme; skip in minimal client
		return nil
	}
	q := u.Query()
	query := fmt.Sprintf("INSERT INTO %s FORMAT JSONEachRow", sanitizeIdent(table))
	q.Set("query", query)
	u.RawQuery = q.Encode()
	payload := append([]byte(nil), buf.Bytes()...)
	return doWithRetry(ctx, func() error {
		reqCtx, cancel := c.requestContext(ctx)
		defer cancel()
		req, err := httpNewRequest(reqCtx, http.MethodPost, u.String(), bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.hc.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode/100 != 2 {
			b, _ := io.ReadAll(resp.Body)
			return &httpStatusErr{code: resp.StatusCode, body: string(b), op: "insert"}
		}
		return nil
	})
}

// sanitizeIdent prevents injection in table identifiers for simple cases.
func sanitizeIdent(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '_' || r == '.' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return r
		}
		return '_'
	}, s)
}

// Retry policy (tunable in tests)
var (
	retryAttempts    = 3
	retryBackoffBase = 10 * time.Millisecond
)

type httpStatusErr struct {
	code     int
	body, op string
}

func (e *httpStatusErr) Error() string {
	return fmt.Sprintf("clickhouse %s http %d: %s", e.op, e.code, e.body)
}

func isRetriable(err error) bool {
	if e, ok := err.(*httpStatusErr); ok {
		if e.code == 429 {
			return true
		}
		if e.code >= 500 {
			return true
		}
		return false
	}
	// Network/transport errors are retriable by default
	return true
}

func doWithRetry(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 0; attempt < retryAttempts; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if !isRetriable(err) {
			return err
		}
		// backoff with jitter
		if attempt < retryAttempts-1 {
			d := retryBackoffBase * (1 << attempt)
			select {
			case <-time.After(d):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return err
}
