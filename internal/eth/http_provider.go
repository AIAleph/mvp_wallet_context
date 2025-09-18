package eth

import (
	"bytes"
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/AIAleph/mvp_wallet_context/internal/logging"
)

var ErrUnsupported = errors.New("method not supported by provider")

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// httpProvider is a minimal JSON-RPC client for Ethereum endpoints.
// It intentionally leaves rate limiting/retries to wrappers (RLProvider, etc.).
type httpProvider struct {
	endpoint    string
	providerLbl string
	hc          httpDoer
	maxRetries  int
	backoffBase time.Duration
	blkCache    *timestampCache
}

// NewHTTPProvider constructs a JSON-RPC provider using the given http.Client (or a default one if nil).
func NewHTTPProvider(endpoint string, client *http.Client) (Provider, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("empty endpoint")
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &httpProvider{
		endpoint:    endpoint,
		providerLbl: deriveProviderLabel(endpoint),
		hc:          client,
		maxRetries:  2,
		backoffBase: 100 * time.Millisecond,
		blkCache:    newTimestampCache(defaultBlockTimestampCacheSize, defaultBlockTimestampTTL),
	}, nil
}

type rpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int64       `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
	ID      int64           `json:"id"`
}

const (
	defaultBlockTimestampCacheSize = 2048
	defaultBlockTimestampTTL       = 15 * time.Minute
)

type timestampCacheEntry struct {
	key       uint64
	value     int64
	expiresAt time.Time
}

type timestampCache struct {
	mu      sync.Mutex
	max     int
	ttl     time.Duration
	entries map[uint64]*list.Element
	ordered *list.List
}

func newTimestampCache(max int, ttl time.Duration) *timestampCache {
	if max <= 0 {
		max = defaultBlockTimestampCacheSize
	}
	if ttl <= 0 {
		ttl = defaultBlockTimestampTTL
	}
	return &timestampCache{
		max:     max,
		ttl:     ttl,
		entries: make(map[uint64]*list.Element, max),
		ordered: list.New(),
	}
}

func (c *timestampCache) get(block uint64, now time.Time) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[block]; ok {
		e := el.Value.(*timestampCacheEntry)
		if !now.Before(e.expiresAt) {
			c.removeElement(el)
			return 0, false
		}
		c.ordered.MoveToFront(el)
		return e.value, true
	}
	return 0, false
}

func (c *timestampCache) add(block uint64, value int64, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[block]; ok {
		e := el.Value.(*timestampCacheEntry)
		e.value = value
		e.expiresAt = now.Add(c.ttl)
		c.ordered.MoveToFront(el)
		return
	}
	entry := &timestampCacheEntry{key: block, value: value, expiresAt: now.Add(c.ttl)}
	el := c.ordered.PushFront(entry)
	c.entries[block] = el
	c.evict(now)
}

func (c *timestampCache) evict(now time.Time) {
	if c.ordered.Len() == 0 {
		return
	}
	for el := c.ordered.Back(); el != nil; el = el.Prev() {
		e := el.Value.(*timestampCacheEntry)
		if now.Before(e.expiresAt) {
			break
		}
		c.removeElement(el)
	}
	for c.ordered.Len() > c.max {
		el := c.ordered.Back()
		c.removeElement(el)
	}
}

func (c *timestampCache) removeElement(el *list.Element) {
	entry := el.Value.(*timestampCacheEntry)
	delete(c.entries, entry.key)
	c.ordered.Remove(el)
}

func deriveProviderLabel(endpoint string) string {
	if endpoint == "" {
		return ""
	}
	if u, err := url.Parse(endpoint); err == nil {
		u.User = nil
		if u.Host != "" {
			return u.Host
		}
		if u.Scheme == "" {
			return endpoint
		}
		return u.String()
	}
	return endpoint
}

func (p *httpProvider) call(ctx context.Context, method string, params interface{}, out interface{}) error {
	reqBody, _ := json.Marshal(rpcRequest{JSONRPC: "2.0", Method: method, Params: params, ID: 1})
	var lastErr error
	attempts := p.maxRetries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(reqBody))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := p.hc.Do(req)
		if err != nil {
			lastErr = err
		} else {
			func() {
				defer func() {
					_ = resp.Body.Close()
				}()
				if resp.StatusCode/100 != 2 {
					b, _ := io.ReadAll(resp.Body)
					lastErr = fmt.Errorf("http %d: %s", resp.StatusCode, string(b))
				} else {
					var rr rpcResponse
					if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
						lastErr = err
					} else if rr.Error != nil {
						// Surface JSON-RPC errors; treat as non-retriable by default (HTTP 200)
						lastErr = fmt.Errorf("rpc %d: %s", rr.Error.Code, rr.Error.Message)
						return
					} else {
						if out != nil {
							lastErr = json.Unmarshal(rr.Result, out)
						} else {
							lastErr = nil
						}
					}
				}
			}()
			if lastErr == nil {
				return nil
			}
			// For non-2xx with 5xx or 429, retry; else break
			if resp != nil {
				if sc := resp.StatusCode; sc != 429 && sc < 500 {
					break
				}
			}
		}
		// Backoff before next attempt
		if attempt < attempts-1 {
			d := p.backoffBase * (1 << attempt)
			t := time.NewTimer(d)
			select {
			case <-ctx.Done():
				t.Stop()
				return ctx.Err()
			case <-t.C:
			}
		}
	}
	return lastErr
}

// hexToUint64 parses an Ethereum hex quantity (e.g., "0x2a") into uint64.
func hexToUint64(s string) (uint64, error) {
	var v uint64
	if _, err := fmt.Sscanf(s, "0x%x", &v); err != nil {
		return 0, fmt.Errorf("invalid hex quantity: %q", s)
	}
	return v, nil
}

func toHex(n uint64) string { return fmt.Sprintf("0x%x", n) }

func (p *httpProvider) BlockNumber(ctx context.Context) (uint64, error) {
	var res string
	if err := p.call(ctx, "eth_blockNumber", []interface{}{}, &res); err != nil {
		return 0, err
	}
	return hexToUint64(res)
}

func (p *httpProvider) BlockTimestamp(ctx context.Context, block uint64) (int64, error) {
	return p.blockTimestampMillis(ctx, block)
}

type rpcLog struct {
	TxHash      string   `json:"transactionHash"`
	LogIndexHex string   `json:"logIndex"`
	Address     string   `json:"address"`
	Topics      []string `json:"topics"`
	Data        string   `json:"data"`
	BlockHex    string   `json:"blockNumber"`
}

// GetLogs implements a minimal eth_getLogs call.
func (p *httpProvider) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]Log, error) {
	// Build topics param: each position may be null, string, or array of strings.
	var topicsParam []interface{}
	for _, group := range topics {
		if len(group) == 0 {
			topicsParam = append(topicsParam, nil)
			continue
		}
		if len(group) == 1 {
			topicsParam = append(topicsParam, group[0])
			continue
		}
		arr := make([]string, len(group))
		copy(arr, group)
		topicsParam = append(topicsParam, arr)
	}
	params := []interface{}{
		map[string]interface{}{
			"address":   address,
			"fromBlock": toHex(from),
			"toBlock":   toHex(to),
			"topics":    topicsParam,
		},
	}
	var raw []rpcLog
	if err := p.call(ctx, "eth_getLogs", params, &raw); err != nil {
		return nil, err
	}
	out := make([]Log, 0, len(raw))
	uniqBlocks := map[uint64]struct{}{}
	for _, l := range raw {
		idx, _ := hexToUint64(l.LogIndexHex)
		blk, _ := hexToUint64(l.BlockHex)
		uniqBlocks[blk] = struct{}{}
		out = append(out, Log{
			TxHash:   l.TxHash,
			Index:    uint32(idx),
			Address:  l.Address,
			Topics:   l.Topics,
			DataHex:  l.Data,
			BlockNum: blk,
			TsMillis: 0, // enriched below
		})
	}
	// Enrich timestamps: one eth_getBlockByNumber per unique block
	tsMap := make(map[uint64]int64, len(uniqBlocks))
	for blk := range uniqBlocks {
		if ts, err := p.blockTimestampMillis(ctx, blk); err == nil {
			tsMap[blk] = ts
		}
	}
	for i := range out {
		if ts, ok := tsMap[out[i].BlockNum]; ok {
			out[i].TsMillis = ts
		}
	}
	return out, nil
}

// TraceBlock attempts to use trace_filter with pagination, mapping to Trace.
// Providers that do not support it will return an error.
func (p *httpProvider) TraceBlock(ctx context.Context, from, to uint64, address string) ([]Trace, error) {
	page := 1000
	after := 0
	var all []Trace
	for {
		params := []interface{}{
			map[string]interface{}{
				"fromBlock":   toHex(from),
				"toBlock":     toHex(to),
				"fromAddress": []string{address},
				"toAddress":   []string{address},
				"after":       after,
				"count":       page,
			},
		}
		var raw []struct {
			TxHash       string `json:"transactionHash"`
			BlockHex     string `json:"blockNumber"`
			TraceAddress []int  `json:"traceAddress"`
			Action       struct {
				From  string `json:"from"`
				To    string `json:"to"`
				Value string `json:"value"`
			} `json:"action"`
		}
		if err := p.call(ctx, "trace_filter", params, &raw); err != nil {
			if strings.Contains(err.Error(), "rpc -32601") || strings.Contains(err.Error(), "trace_filter") {
				return nil, ErrUnsupported
			}
			return nil, err
		}
		if len(raw) == 0 {
			break
		}
		for _, t := range raw {
			blk, _ := hexToUint64(t.BlockHex)
			// Compose a simple trace ID from traceAddress path or "root" when empty
			traceID := "root"
			if len(t.TraceAddress) > 0 {
				// Convert []int to "a-b-c"
				buf := make([]byte, 0, len(t.TraceAddress)*2)
				for i, v := range t.TraceAddress {
					if i > 0 {
						buf = append(buf, '-')
					}
					buf = append(buf, []byte(fmt.Sprintf("%d", v))...)
				}
				traceID = string(buf)
			}
			all = append(all, Trace{
				TxHash:   t.TxHash,
				TraceID:  traceID,
				From:     t.Action.From,
				To:       t.Action.To,
				ValueWei: t.Action.Value,
				BlockNum: blk,
				TsMillis: 0, // optional enrichment later
			})
		}
		if len(raw) < page {
			break
		}
		after += page
	}
	// Enrich timestamps per unique block
	uniq := make(map[uint64]struct{}, len(all))
	for _, t := range all {
		uniq[t.BlockNum] = struct{}{}
	}
	tsMap := make(map[uint64]int64, len(uniq))
	for blk := range uniq {
		if ts, err := p.blockTimestampMillis(ctx, blk); err == nil {
			tsMap[blk] = ts
		}
	}
	for i := range all {
		if ts, ok := tsMap[all[i].BlockNum]; ok {
			all[i].TsMillis = ts
		}
	}
	return all, nil
}

// Transactions currently returns ErrUnsupported; external adapters can wrap
// provider-specific transaction endpoints (e.g., Alchemy Transfers API).
func (p *httpProvider) Transactions(ctx context.Context, address string, from, to uint64) (result []Transaction, err error) {
	if from > to {
		return nil, nil
	}
	lowerAddr := strings.ToLower(address)
	start := time.Now()
	receiptCalls := 0
	blockCalls := 0
	txExamined := 0
	txMatched := 0
	span := to - from
	if span != math.MaxUint64 {
		span++
	}
	logger := logging.Logger()
	defer func() {
		if logger == nil {
			return
		}
		fields := []any{
			"component", "eth.http_provider.transactions",
			"provider", p.providerLbl,
			"address", lowerAddr,
			"from_block", from,
			"to_block", to,
			"block_span", span,
			"receipt_calls", receiptCalls,
			"block_calls", blockCalls,
			"tx_examined", txExamined,
			"tx_matched", txMatched,
			"tx_returned", len(result),
			"elapsed_ms", time.Since(start).Milliseconds(),
		}
		if err != nil {
			logger.Warn("receipt_lookup_failed", append(fields, "error", err.Error())...)
			return
		}
		logger.Info("receipt_lookup", fields...)
	}()
	for blk := from; blk <= to; blk++ {
		var block struct {
			Timestamp    string `json:"timestamp"`
			Transactions []struct {
				Hash  string  `json:"hash"`
				From  string  `json:"from"`
				To    *string `json:"to"`
				Input string  `json:"input"`
				Value string  `json:"value"`
			} `json:"transactions"`
		}
		params := []interface{}{toHex(blk), true}
		blockCalls++
		if callErr := p.call(ctx, "eth_getBlockByNumber", params, &block); callErr != nil {
			err = callErr
			return nil, err
		}
		tsSec, err := hexToUint64(block.Timestamp)
		if err != nil {
			return nil, err
		}
		tsMillis := int64(tsSec) * 1000
		for _, tx := range block.Transactions {
			txExamined++
			fromLower := strings.ToLower(tx.From)
			toLower := ""
			if tx.To != nil {
				toLower = strings.ToLower(*tx.To)
			}
			if fromLower != lowerAddr && toLower != lowerAddr {
				continue
			}
			txMatched++
			var receipt struct {
				Status  string `json:"status"`
				GasUsed string `json:"gasUsed"`
			}
			receiptCalls++
			if callErr := p.call(ctx, "eth_getTransactionReceipt", []interface{}{tx.Hash}, &receipt); callErr != nil {
				err = callErr
				return nil, err
			}
			gasUsed, err := hexToUint64(receipt.GasUsed)
			if err != nil {
				return nil, err
			}
			statusVal := uint8(1)
			if receipt.Status != "" {
				s, err := hexToUint64(receipt.Status)
				if err != nil {
					return nil, err
				}
				statusVal = uint8(s)
			}
			result = append(result, Transaction{
				Hash:     tx.Hash,
				From:     fromLower,
				To:       toLower,
				ValueWei: tx.Value,
				InputHex: tx.Input,
				GasUsed:  gasUsed,
				Status:   statusVal,
				BlockNum: blk,
				TsMillis: int64(tsMillis),
			})
		}
	}
	return result, nil
}

// blockTimestampMillis fetches the block and returns timestamp in milliseconds.
func (p *httpProvider) blockTimestampMillis(ctx context.Context, block uint64) (int64, error) {
	if p.blkCache != nil {
		if ts, ok := p.blkCache.get(block, time.Now()); ok {
			return ts, nil
		}
	}
	var blk struct {
		Timestamp string `json:"timestamp"`
	}
	params := []interface{}{toHex(block), false}
	if err := p.call(ctx, "eth_getBlockByNumber", params, &blk); err != nil {
		return 0, err
	}
	sec, err := hexToUint64(blk.Timestamp)
	if err != nil {
		return 0, err
	}
	ts := int64(sec) * 1000
	if p.blkCache != nil {
		p.blkCache.add(block, ts, time.Now())
	}
	return ts, nil
}
