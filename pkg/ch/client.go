package ch

import (
    "context"
)

// Client is a thin ClickHouse HTTP client wrapper. The real implementation will
// manage a shared http.Client with timeouts, retries and JSON encoding for
// inserts/queries as needed.
type Client struct{}

func New(_dsn string) *Client { return &Client{} }

func (c *Client) Ping(_ context.Context) error { return nil }

