package ingest

import (
	"context"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AIAleph/mvp_wallet_context/internal/eth"
)

type cursorRoundTripper struct {
	t              *testing.T
	selectResponse string
	selectStatus   int
	selectBody     string
	inserts        []string
	insertStatus   int
	insertBody     string
}

func (rt *cursorRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	query := r.URL.Query().Get("query")
	switch {
	case strings.Contains(query, "SELECT"):
		status := rt.selectStatus
		if status == 0 {
			status = 200
		}
		body := rt.selectBody
		if body == "" {
			body = rt.selectResponse
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
	case strings.Contains(query, "INSERT INTO addresses"):
		buf, err := io.ReadAll(r.Body)
		if err != nil {
			rt.t.Fatalf("read body: %v", err)
		}
		rt.inserts = append(rt.inserts, string(buf))
		status := rt.insertStatus
		if status == 0 {
			status = 200
		}
		body := rt.insertBody
		if body == "" {
			body = "ok"
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
	default:
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	}
}

type stubCursorProvider struct{ head uint64 }

func (p stubCursorProvider) BlockNumber(ctx context.Context) (uint64, error) { return p.head, nil }
func (p stubCursorProvider) BlockTimestamp(ctx context.Context, block uint64) (int64, error) {
	return int64(block) * 1000, nil
}
func (p stubCursorProvider) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	return nil, nil
}
func (p stubCursorProvider) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	return nil, nil
}

type captureProv struct {
	head  uint64
	calls []struct{ from, to uint64 }
}

func (p *captureProv) BlockNumber(ctx context.Context) (uint64, error) { return p.head, nil }
func (p *captureProv) BlockTimestamp(ctx context.Context, block uint64) (int64, error) {
	return int64(block) * 1000, nil
}
func (p *captureProv) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	p.calls = append(p.calls, struct{ from, to uint64 }{from: from, to: to})
	return nil, nil
}
func (p *captureProv) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	return nil, nil
}

type panicRangeProvider struct {
	head uint64
	t    *testing.T
}

func (p *panicRangeProvider) BlockNumber(ctx context.Context) (uint64, error) { return p.head, nil }
func (p *panicRangeProvider) BlockTimestamp(ctx context.Context, block uint64) (int64, error) {
	return 0, nil
}
func (p *panicRangeProvider) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	if p.t != nil {
		p.t.Fatalf("unexpected GetLogs call: %d-%d", from, to)
	}
	return nil, nil
}
func (p *panicRangeProvider) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	if p.t != nil {
		p.t.Fatalf("unexpected TraceBlock call: %d-%d", from, to)
	}
	return nil, nil
}

type maxHeadProvider struct{}

func (maxHeadProvider) BlockNumber(ctx context.Context) (uint64, error) { return math.MaxUint64, nil }
func (maxHeadProvider) BlockTimestamp(ctx context.Context, block uint64) (int64, error) {
	return 0, nil
}
func (maxHeadProvider) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	return nil, nil
}
func (maxHeadProvider) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	return nil, nil
}

type devSchemaProvider struct{}

func (devSchemaProvider) BlockNumber(ctx context.Context) (uint64, error) { return 0, nil }
func (devSchemaProvider) BlockTimestamp(ctx context.Context, block uint64) (int64, error) {
	return int64(block) * 1000, nil
}
func (devSchemaProvider) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
	return []eth.Log{{TxHash: "0x1", Index: 0, Address: address, BlockNum: from, TsMillis: 0}}, nil
}
func (devSchemaProvider) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
	return []eth.Trace{{TxHash: "0x1", TraceID: "0", From: address, To: "0x2", BlockNum: from, TsMillis: 0}}, nil
}

type queryFailingTransport struct {
	t       *testing.T
	status  int
	body    string
	matcher string
}

func (rt queryFailingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	query := r.URL.Query().Get("query")
	if strings.Contains(query, rt.matcher) {
		status := rt.status
		if status == 0 {
			status = 500
		}
		body := rt.body
		if body == "" {
			body = "boom"
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
}

func withTimeNow(t *testing.T, now time.Time) func() {
	t.Helper()
	prev := timeNow
	timeNow = func() time.Time { return now }
	return func() { timeNow = prev }
}
