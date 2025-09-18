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
	endpoint             string
	providerLbl          string
	hc                   httpDoer
	maxRetries           int
	backoffBase          time.Duration
	blkCache             *timestampCache
	receiptWorkers       int
	blockReceiptsMu      sync.Mutex
	blockReceiptsSupport receiptSupportState
}

type receiptSupportState int

const (
	receiptSupportUnknown receiptSupportState = iota
	receiptSupportAvailable
	receiptSupportUnavailable
)

type receiptLite struct {
	gasUsed uint64
	status  uint8
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
		endpoint:             endpoint,
		providerLbl:          deriveProviderLabel(endpoint),
		hc:                   client,
		maxRetries:           2,
		backoffBase:          100 * time.Millisecond,
		blkCache:             newTimestampCache(defaultBlockTimestampCacheSize, defaultBlockTimestampTTL),
		receiptWorkers:       4,
		blockReceiptsSupport: receiptSupportUnknown,
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

// Transactions walks blocks in the inclusive range and surfaces external
// transactions touching the address. It opportunistically batches receipt
// lookups and tolerates per-block/receipt failures, logging them as warnings
// while still returning partial results when possible.
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
	receiptFailures := 0
	blockFailures := 0
	txSkipped := 0
	span := to - from
	if span != math.MaxUint64 {
		span++
	}
	logger := logging.Logger()
	var partialErr error
	var partialErrs []error
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
			"block_failures", blockFailures,
			"receipt_failures", receiptFailures,
			"tx_skipped", txSkipped,
			"elapsed_ms", time.Since(start).Milliseconds(),
		}
		if err != nil {
			logger.Warn("receipt_lookup_failed", append(fields, "error", err.Error())...)
			return
		}
		if partialErr != nil {
			logger.Warn("receipt_lookup_partial", append(fields, "error", partialErr.Error())...)
			return
		}
		logger.Info("receipt_lookup", fields...)
	}()

	type pendingTx struct {
		hash      string
		hashLower string
		from      string
		to        string
		input     string
		value     string
		blockNum  uint64
		tsMillis  int64
	}

	for blk := from; blk <= to; blk++ {
		if ctxErr := ctx.Err(); ctxErr != nil {
			partialErrs = append(partialErrs, ctxErr)
			break
		}
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
			blockFailures++
			partialErrs = append(partialErrs, fmt.Errorf("block %d: %w", blk, callErr))
			if blk == math.MaxUint64 {
				break
			}
			continue
		}
		tsSec, tsErr := hexToUint64(block.Timestamp)
		if tsErr != nil {
			blockFailures++
			partialErrs = append(partialErrs, fmt.Errorf("block %d timestamp: %w", blk, tsErr))
			if blk == math.MaxUint64 {
				break
			}
			continue
		}
		tsMillis := int64(tsSec) * 1000
		pending := make([]pendingTx, 0, len(block.Transactions))
		hashes := make([]string, 0, len(block.Transactions))
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
			hashLower := strings.ToLower(tx.Hash)
			pending = append(pending, pendingTx{
				hash:      tx.Hash,
				hashLower: hashLower,
				from:      fromLower,
				to:        toLower,
				input:     tx.Input,
				value:     tx.Value,
				blockNum:  blk,
				tsMillis:  tsMillis,
			})
			hashes = append(hashes, tx.Hash)
		}
		if len(pending) == 0 {
			if blk == math.MaxUint64 {
				break
			}
			continue
		}
		receipts, calls, failures, recErr := p.fetchReceiptsForBlock(ctx, blk, hashes)
		receiptCalls += calls
		receiptFailures += failures
		if recErr != nil {
			partialErrs = append(partialErrs, fmt.Errorf("block %d receipts: %w", blk, recErr))
		}
		for _, tx := range pending {
			rec, ok := receipts[tx.hashLower]
			if !ok {
				txSkipped++
				continue
			}
			result = append(result, Transaction{
				Hash:     tx.hash,
				From:     tx.from,
				To:       tx.to,
				ValueWei: tx.value,
				InputHex: tx.input,
				GasUsed:  rec.gasUsed,
				Status:   rec.status,
				BlockNum: tx.blockNum,
				TsMillis: tx.tsMillis,
			})
		}
		if blk == math.MaxUint64 {
			break
		}
	}
	if len(result) == 0 && len(partialErrs) > 0 {
		err = errors.Join(partialErrs...)
		return nil, err
	}
	if len(partialErrs) > 0 {
		partialErr = errors.Join(partialErrs...)
	}
	return result, nil
}

func (p *httpProvider) fetchReceiptsForBlock(ctx context.Context, block uint64, hashes []string) (map[string]receiptLite, int, int, error) {
	out := make(map[string]receiptLite, len(hashes))
	if len(hashes) == 0 {
		return out, 0, 0, nil
	}
	hashSet := make(map[string]struct{}, len(hashes))
	for _, h := range hashes {
		hashSet[strings.ToLower(h)] = struct{}{}
	}
	useBlockReceipts := p.shouldUseBlockReceipts(len(hashes))
	totalCalls := 0
	failures := 0
	var joinedErr error
	if useBlockReceipts {
		recs, err := p.callBlockReceipts(ctx, block, hashSet)
		totalCalls++
		if err == nil {
			for k, v := range recs {
				out[k] = v
			}
			missing := make([]string, 0)
			for _, h := range hashes {
				if _, ok := out[strings.ToLower(h)]; !ok {
					missing = append(missing, h)
				}
			}
			p.setBlockReceiptsState(receiptSupportAvailable)
			if len(missing) == 0 {
				return out, totalCalls, 0, nil
			}
			perTx, calls, perFailures, perErr := p.fetchReceiptsIndividually(ctx, missing)
			for k, v := range perTx {
				out[k] = v
			}
			totalCalls += calls
			failures += perFailures
			if perErr != nil {
				joinedErr = errors.Join(joinedErr, perErr)
			}
			return out, totalCalls, failures, joinedErr
		}
		if isMethodNotFound(err) {
			p.setBlockReceiptsState(receiptSupportUnavailable)
		} else {
			joinedErr = err
		}
	}
	perTx, calls, perFailures, err := p.fetchReceiptsIndividually(ctx, hashes)
	for k, v := range perTx {
		out[k] = v
	}
	totalCalls += calls
	failures += perFailures
	if err != nil {
		joinedErr = errors.Join(joinedErr, err)
	}
	return out, totalCalls, failures, joinedErr
}

func (p *httpProvider) fetchReceiptsIndividually(ctx context.Context, hashes []string) (map[string]receiptLite, int, int, error) {
	out := make(map[string]receiptLite, len(hashes))
	if len(hashes) == 0 {
		return out, 0, 0, nil
	}
	workers := p.receiptWorkers
	if workers <= 0 {
		workers = 1
	}
	sem := make(chan struct{}, workers)
	type result struct {
		hashLower string
		receipt   receiptLite
		err       error
	}
	resCh := make(chan result, len(hashes))
	var wg sync.WaitGroup
	for _, h := range hashes {
		hash := h
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := ctx.Err(); err != nil {
				resCh <- result{err: err}
				return
			}
			sem <- struct{}{}
			defer func() { <-sem }()
			var receipt struct {
				Status  string `json:"status"`
				GasUsed string `json:"gasUsed"`
			}
			if callErr := p.call(ctx, "eth_getTransactionReceipt", []interface{}{hash}, &receipt); callErr != nil {
				resCh <- result{err: fmt.Errorf("receipt %s: %w", hash, callErr)}
				return
			}
			gasUsed, gasErr := hexToUint64(receipt.GasUsed)
			if gasErr != nil {
				resCh <- result{err: fmt.Errorf("receipt %s gasUsed: %w", hash, gasErr)}
				return
			}
			statusVal := uint8(1)
			if receipt.Status != "" {
				s, statusErr := hexToUint64(receipt.Status)
				if statusErr != nil {
					resCh <- result{err: fmt.Errorf("receipt %s status: %w", hash, statusErr)}
					return
				}
				statusVal = uint8(s)
			}
			hashLower := strings.ToLower(hash)
			resCh <- result{hashLower: hashLower, receipt: receiptLite{gasUsed: gasUsed, status: statusVal}}
		}()
	}
	wg.Wait()
	close(resCh)
	failures := 0
	var errs []error
	for res := range resCh {
		if res.err != nil {
			failures++
			errs = append(errs, res.err)
			continue
		}
		out[res.hashLower] = res.receipt
	}
	var joined error
	if len(errs) > 0 {
		joined = errors.Join(errs...)
	}
	return out, len(hashes), failures, joined
}

func (p *httpProvider) callBlockReceipts(ctx context.Context, block uint64, filter map[string]struct{}) (map[string]receiptLite, error) {
	var recs []struct {
		TxHash  string `json:"transactionHash"`
		Status  string `json:"status"`
		GasUsed string `json:"gasUsed"`
	}
	if err := p.call(ctx, "eth_getBlockReceipts", []interface{}{toHex(block)}, &recs); err != nil {
		return nil, err
	}
	out := make(map[string]receiptLite, len(recs))
	for _, rec := range recs {
		hashLower := strings.ToLower(rec.TxHash)
		if len(filter) > 0 {
			if _, ok := filter[hashLower]; !ok {
				continue
			}
		}
		gasUsed, err := hexToUint64(rec.GasUsed)
		if err != nil {
			return nil, fmt.Errorf("block receipt %s gasUsed: %w", rec.TxHash, err)
		}
		statusVal := uint8(1)
		if rec.Status != "" {
			s, err := hexToUint64(rec.Status)
			if err != nil {
				return nil, fmt.Errorf("block receipt %s status: %w", rec.TxHash, err)
			}
			statusVal = uint8(s)
		}
		out[hashLower] = receiptLite{gasUsed: gasUsed, status: statusVal}
	}
	return out, nil
}

func (p *httpProvider) shouldUseBlockReceipts(matchCount int) bool {
	p.blockReceiptsMu.Lock()
	defer p.blockReceiptsMu.Unlock()
	switch p.blockReceiptsSupport {
	case receiptSupportAvailable:
		return true
	case receiptSupportUnknown:
		return matchCount > 1
	default:
		return false
	}
}

func (p *httpProvider) setBlockReceiptsState(state receiptSupportState) {
	p.blockReceiptsMu.Lock()
	if p.blockReceiptsSupport != state {
		p.blockReceiptsSupport = state
	}
	p.blockReceiptsMu.Unlock()
}

func isMethodNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "-32601") || strings.Contains(msg, "method not found")
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
