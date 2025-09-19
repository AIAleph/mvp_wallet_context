package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/AIAleph/mvp_wallet_context/internal/eth"
	"github.com/AIAleph/mvp_wallet_context/internal/normalize"
	"github.com/AIAleph/mvp_wallet_context/pkg/ch"
)

// Default ingest tunables.
const (
	DefaultSchemaMode  = "canonical"
	DefaultBatchBlocks = 1000
)

var addressPattern = regexp.MustCompile(`^0x[0-9a-f]*$`)

// NormalizeSchema standardizes the ingestion schema selection.
// Accepts "canonical" (default) and "dev"; rejects other inputs.
func NormalizeSchema(schema string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(schema))
	if mode == "" {
		return DefaultSchemaMode, nil
	}
	switch mode {
	case "canonical", "dev":
		return mode, nil
	default:
		return "", fmt.Errorf("invalid schema mode %q", schema)
	}
}

// Options configure a run of the ingester.
type Options struct {
	ProviderURL   string
	ClickHouseDSN string
	FromBlock     uint64
	ToBlock       uint64
	Confirmations int
	BatchBlocks   int
	RateLimit     int    // RPC requests per second (0 = unlimited)
	RedisURL      string // Optional cache endpoint
	DryRun        bool
	Timeout       time.Duration
	// Schema selects target tables: "dev" (dev_*) or "canonical" (schema.sql tables)
	Schema string
}

// Ingester coordinates fetching, normalization and persistence for a single
// address. It is intentionally minimal for scaffolding.
type Ingester struct {
	address string
	opts    Options
	prov    eth.Provider
	ch      *ch.Client
	tsMu    sync.RWMutex
	tsCache map[uint64]int64
	curMu   sync.RWMutex
	cur     *addressCheckpoint // TODO: consider TTL-based invalidation for long-running processes.
}

func New(address string, opts Options) *Ingester {
	opts = mustNormalizeOptions(opts)
	addr := strings.ToLower(address)
	if addr != "" && !addressPattern.MatchString(addr) {
		panic(fmt.Sprintf("invalid address %q", address))
	}
	var c *ch.Client
	if opts.ClickHouseDSN != "" {
		c = ch.New(opts.ClickHouseDSN)
	} else {
		c = ch.New("")
	}
	return &Ingester{address: addr, opts: opts, ch: c, tsCache: make(map[uint64]int64)}
}

// NewWithProvider injects a concrete eth.Provider (already wrapped with
// rate-limiter, retries, etc.). Prefer this in production wiring.
func NewWithProvider(address string, opts Options, p eth.Provider) *Ingester {
	opts = mustNormalizeOptions(opts)
	addr := strings.ToLower(address)
	if addr != "" && !addressPattern.MatchString(addr) {
		panic(fmt.Sprintf("invalid address %q", address))
	}
	var c *ch.Client
	if opts.ClickHouseDSN != "" {
		c = ch.New(opts.ClickHouseDSN)
	} else {
		c = ch.New("")
	}
	return &Ingester{address: addr, opts: opts, prov: p, ch: c, tsCache: make(map[uint64]int64)}
}

var timeNow = time.Now

const (
	checkpointBackfill = "backfill"
	checkpointDelta    = "delta"
)

// Backfill performs the initial, historical sync for the configured address.
// MVP scaffold: implement block-ranged fetchers in follow-up tasks.
func (i *Ingester) Backfill(ctx context.Context) error {
	// Early return if no provider is wired (tests / dry runs)
	if i.prov == nil {
		return nil
	}
	head, err := i.prov.BlockNumber(ctx)
	if err != nil {
		return err
	}
	ckpt, existed, err := i.loadCheckpoint(ctx)
	if err != nil {
		return err
	}
	from := i.opts.FromBlock
	if existed && from <= ckpt.LastSyncedBlock {
		if ckpt.LastSyncedBlock == math.MaxUint64 {
			return fmt.Errorf("address %s last_synced_block at max value", i.address)
		}
		from = ckpt.LastSyncedBlock + 1
	}
	to := i.opts.ToBlock
	if to == 0 {
		to = head
	}
	safeHead, hasSafe := i.safeHead(head)
	if !hasSafe {
		if existed {
			return i.persistCheckpoint(ctx, ckpt, checkpointBackfill, ckpt.LastSyncedBlock)
		}
		return nil
	}
	if to > safeHead {
		to = safeHead
	}
	if from > to {
		if existed {
			return i.persistCheckpoint(ctx, ckpt, checkpointBackfill, ckpt.LastSyncedBlock)
		}
		return nil
	}
	batch := uint64(i.opts.BatchBlocks)
	if batch == 0 {
		batch = DefaultBatchBlocks
	}
	var (
		lastProcessed uint64
		processed     bool
	)
	for cur := from; cur <= to; {
		end := cur + batch - 1
		if end > to {
			end = to
		}
		if err := i.processRange(ctx, cur, end); err != nil {
			return err
		}
		processed = true
		lastProcessed = end
		cur = end + 1
	}
	return i.finalizeBackfill(ctx, ckpt, existed, processed, lastProcessed)
}

// Delta performs a recent delta update with N confirmations.
func (i *Ingester) Delta(ctx context.Context) error {
	if i.prov == nil {
		return nil
	}
	head, err := i.prov.BlockNumber(ctx)
	if err != nil {
		return err
	}
	ckpt, existed, err := i.loadCheckpoint(ctx)
	if err != nil {
		return err
	}
	safeHead, hasSafe := i.safeHead(head)
	to := i.opts.ToBlock
	if to == 0 || to > safeHead {
		to = safeHead
	}
	if !hasSafe {
		if existed {
			return i.persistCheckpoint(ctx, ckpt, checkpointDelta, ckpt.LastSyncedBlock)
		}
		return nil
	}
	if ckpt.LastSyncedBlock > safeHead {
		ckpt.LastSyncedBlock = safeHead
	}
	from := i.opts.FromBlock
	if i.opts.Confirmations > 0 {
		conf := uint64(i.opts.Confirmations)
		var reorgStart uint64
		if ckpt.LastSyncedBlock+1 > conf {
			reorgStart = ckpt.LastSyncedBlock + 1 - conf
		} else {
			reorgStart = 0
		}
		if reorgStart > from {
			from = reorgStart
		}
	} else if from <= ckpt.LastSyncedBlock {
		if ckpt.LastSyncedBlock == math.MaxUint64 {
			return fmt.Errorf("address %s last_synced_block at max value", i.address)
		}
		from = ckpt.LastSyncedBlock + 1
	}
	if from > to {
		if existed {
			return i.persistCheckpoint(ctx, ckpt, checkpointDelta, ckpt.LastSyncedBlock)
		}
		return nil
	}
	batch := uint64(i.opts.BatchBlocks)
	if batch == 0 {
		batch = DefaultBatchBlocks
	}
	var (
		lastProcessed uint64
		processed     bool
	)
	for cur := from; cur <= to; {
		rEnd := cur + batch - 1
		if rEnd > to {
			rEnd = to
		}
		if err := i.processRange(ctx, cur, rEnd); err != nil {
			return err
		}
		processed = true
		lastProcessed = rEnd
		cur = rEnd + 1
	}
	if processed && lastProcessed > ckpt.LastSyncedBlock {
		ckpt.LastSyncedBlock = lastProcessed
	}
	return i.persistCheckpoint(ctx, ckpt, checkpointDelta, ckpt.LastSyncedBlock)
}

// finalizeBackfill consolidates checkpoint persistence logic once the
// backfill loop completes, ensuring we update timestamps consistently.
func (i *Ingester) finalizeBackfill(ctx context.Context, ckpt addressCheckpoint, existed, processed bool, lastProcessed uint64) error {
	if processed {
		if lastProcessed > ckpt.LastSyncedBlock {
			ckpt.LastSyncedBlock = lastProcessed
		}
		return i.persistCheckpoint(ctx, ckpt, checkpointBackfill, ckpt.LastSyncedBlock)
	}
	if !existed {
		return nil
	}
	return i.persistCheckpoint(ctx, ckpt, checkpointBackfill, ckpt.LastSyncedBlock)
}

// processRange fetches logs and traces for the configured address and block range.
func (i *Ingester) processRange(ctx context.Context, from, to uint64) error {
	// Topics nil for now; later pass selectors for token transfers/approvals
	logs, err := i.prov.GetLogs(ctx, i.address, from, to, nil)
	if err != nil {
		return fmt.Errorf("getting logs: %w", err)
	}
	traces, err := i.prov.TraceBlock(ctx, from, to, i.address)
	if err != nil && err != eth.ErrUnsupported {
		return fmt.Errorf("tracing blocks: %w", err)
	}
	txs, err := i.prov.Transactions(ctx, i.address, from, to)
	if err != nil && err != eth.ErrUnsupported {
		return fmt.Errorf("getting transactions: %w", err)
	}
	// Fill timestamps if missing using in-process cache + provider
	for idx := range logs {
		if logs[idx].TsMillis == 0 {
			if ts, ok := i.getBlockTs(ctx, logs[idx].BlockNum); ok {
				logs[idx].TsMillis = ts
			}
		}
	}
	for idx := range traces {
		if traces[idx].TsMillis == 0 {
			if ts, ok := i.getBlockTs(ctx, traces[idx].BlockNum); ok {
				traces[idx].TsMillis = ts
			}
		}
	}
	for idx := range txs {
		if txs[idx].TsMillis == 0 {
			if ts, ok := i.getBlockTs(ctx, txs[idx].BlockNum); ok {
				txs[idx].TsMillis = ts
			}
		}
	}
	// Normalize and write according to schema mode
	mode := i.SchemaMode()
	txRows := normalizeTransactionsForAddress(txs, i.address)
	internalTxRows := normalizeInternalTracesForAddress(traces, i.address)
	if len(internalTxRows) > 0 {
		txRows = append(txRows, internalTxRows...)
	}
	if mode == "canonical" {
		// Logs
		lrows := normalize.LogsToRows(logs)
		if len(lrows) > 0 {
			rows := make([]any, 0, len(lrows))
			for _, r := range lrows {
				rows = append(rows, map[string]any{
					"event_uid":    r.EventUID,
					"tx_hash":      r.TxHash,
					"log_index":    r.LogIndex,
					"address":      r.Address,
					"topics":       r.Topics,
					"data_hex":     r.DataHex,
					"block_number": r.BlockNum,
					"ts":           fmtDT64(r.TsMillis),
				})
			}
			if err := i.ch.InsertJSONEachRow(ctx, "logs", rows); err != nil {
				return fmt.Errorf("inserting logs: %w", err)
			}
		}
		// Token events
		tTransfers, tApprovals := normalize.DecodeTokenEvents(logs)
		rowsTransfers := make([]any, 0, len(tTransfers))
		for _, r := range tTransfers {
			rowsTransfers = append(rowsTransfers, map[string]any{
				"event_uid":    r.EventUID,
				"tx_hash":      r.TxHash,
				"log_index":    r.LogIndex,
				"token":        r.Token,
				"from_addr":    r.From,
				"to_addr":      r.To,
				"amount_raw":   r.AmountRaw,
				"token_id":     r.TokenID,
				"standard":     r.Standard,
				"block_number": r.BlockNum,
				"ts":           fmtDT64(r.TsMillis),
			})
		}
		if err := i.ch.InsertJSONEachRow(ctx, "token_transfers", rowsTransfers); err != nil {
			return fmt.Errorf("inserting token_transfers: %w", err)
		}

		rowsApprovals := make([]any, 0, len(tApprovals))
		for _, r := range tApprovals {
			rowsApprovals = append(rowsApprovals, map[string]any{
				"event_uid":           r.EventUID,
				"tx_hash":             r.TxHash,
				"log_index":           r.LogIndex,
				"token":               r.Token,
				"owner":               r.Owner,
				"spender":             r.Spender,
				"amount_raw":          r.AmountRaw,
				"token_id":            r.TokenID,
				"is_approval_for_all": r.IsForAll,
				"standard":            r.Standard,
				"block_number":        r.BlockNum,
				"ts":                  fmtDT64(r.TsMillis),
			})
		}
		if err := i.ch.InsertJSONEachRow(ctx, "approvals", rowsApprovals); err != nil {
			return fmt.Errorf("inserting approvals: %w", err)
		}
		if len(txRows) > 0 {
			rowsTx := make([]any, 0, len(txRows))
			for _, r := range txRows {
				row := map[string]any{
					"tx_hash":      r.TxHash,
					"block_number": r.BlockNum,
					"ts":           fmtDT64(r.TsMillis),
					"from_addr":    r.From,
					"to_addr":      r.To,
					"value_raw":    r.ValueRaw,
					"gas_used":     r.GasUsed,
					"status":       r.Status,
					"is_internal":  r.IsInternal,
					"trace_id":     nil,
					"input_method": nil,
				}
				if r.TraceID != "" {
					row["trace_id"] = r.TraceID
				}
				if r.InputMethod != "" {
					row["input_method"] = r.InputMethod
				}
				rowsTx = append(rowsTx, row)
			}
			if err := i.ch.InsertJSONEachRow(ctx, "transactions", rowsTx); err != nil {
				return fmt.Errorf("inserting transactions: %w", err)
			}
		}

		trows := normalize.TracesToRows(traces)
		rowsTraces := make([]any, 0, len(trows))
		for _, r := range trows {
			rowsTraces = append(rowsTraces, map[string]any{
				"trace_uid":    r.TraceUID,
				"tx_hash":      r.TxHash,
				"trace_id":     r.TraceID,
				"from_addr":    r.From,
				"to_addr":      r.To,
				"value_raw":    r.ValueRaw,
				"block_number": r.BlockNum,
				"ts":           fmtDT64(r.TsMillis),
			})
		}
		if err := i.ch.InsertJSONEachRow(ctx, "traces", rowsTraces); err != nil {
			return fmt.Errorf("inserting traces: %w", err)
		}
	} else {
		// dev schema (existing behavior)
		lrows := normalize.LogsToRows(logs)
		if err := i.ch.InsertJSONEachRow(ctx, "dev_logs", normalize.AsAny(lrows)); err != nil {
			return fmt.Errorf("inserting dev_logs: %w", err)
		}
		tTransfers, tApprovals := normalize.DecodeTokenEvents(logs)
		if err := i.ch.InsertJSONEachRow(ctx, "dev_token_transfers", normalize.AsAny(tTransfers)); err != nil {
			return fmt.Errorf("inserting dev_token_transfers: %w", err)
		}
		if err := i.ch.InsertJSONEachRow(ctx, "dev_approvals", normalize.AsAny(tApprovals)); err != nil {
			return fmt.Errorf("inserting dev_approvals: %w", err)
		}
		if len(txRows) > 0 {
			if err := i.ch.InsertJSONEachRow(ctx, "dev_transactions", normalize.AsAny(txRows)); err != nil {
				return fmt.Errorf("inserting dev_transactions: %w", err)
			}
		}
		if traces != nil {
			trows := normalize.TracesToRows(traces)
			if err := i.ch.InsertJSONEachRow(ctx, "dev_traces", normalize.AsAny(trows)); err != nil {
				return fmt.Errorf("inserting dev_traces: %w", err)
			}
		}
	}
	return nil
}

// normalizeTransactionsForAddress converts provider transactions to canonical rows
// and filters them for the target address with case-insensitive matching.
func normalizeTransactionsForAddress(txs []eth.Transaction, target string) []normalize.TransactionRow {
	if len(txs) == 0 {
		return nil
	}
	rows := normalize.TransactionsToRows(txs, false)
	return filterTransactionsByAddress(rows, target)
}

func normalizeInternalTracesForAddress(traces []eth.Trace, target string) []normalize.TransactionRow {
	if len(traces) == 0 {
		return nil
	}
	nonRoot := 0
	for _, tr := range traces {
		if strings.EqualFold(tr.TraceID, "root") {
			continue
		}
		nonRoot++
	}
	if nonRoot == 0 {
		return nil
	}
	txs := make([]eth.Transaction, 0, nonRoot)
	for _, tr := range traces {
		// Skip root traces as they represent the original transaction, not an internal call.
		if strings.EqualFold(tr.TraceID, "root") {
			continue
		}
		txs = append(txs, eth.Transaction{
			Hash:     tr.TxHash,
			From:     tr.From,
			To:       tr.To,
			ValueWei: tr.ValueWei,
			BlockNum: tr.BlockNum,
			TsMillis: tr.TsMillis,
			Status:   1,
			TraceID:  tr.TraceID,
		})
	}
	rows := normalize.TransactionsToRows(txs, true)
	return filterTransactionsByAddress(rows, target)
}

func filterTransactionsByAddress(rows []normalize.TransactionRow, target string) []normalize.TransactionRow {
	if target == "" || len(rows) == 0 {
		return rows
	}
	addr := strings.ToLower(target)
	filtered := rows[:0]
	for _, row := range rows {
		if row.From == addr || row.To == addr {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func (i *Ingester) getBlockTs(ctx context.Context, block uint64) (int64, bool) {
	i.tsMu.RLock()
	if ts, ok := i.tsCache[block]; ok {
		i.tsMu.RUnlock()
		return ts, true
	}
	i.tsMu.RUnlock()
	if i.prov == nil {
		return 0, false
	}
	ts, err := i.prov.BlockTimestamp(ctx, block)
	if err != nil {
		return 0, false
	}
	i.tsMu.Lock()
	i.tsCache[block] = ts
	i.tsMu.Unlock()
	return ts, true
}

// safeHead returns the highest block number that satisfies the configured
// confirmation window. The second return value is false when the chain height
// is still within that window and no block should be processed yet.
func (i *Ingester) safeHead(head uint64) (uint64, bool) {
	if i.opts.Confirmations <= 0 {
		return head, true
	}
	conf := uint64(i.opts.Confirmations)
	if head <= conf {
		return 0, false
	}
	return head - conf, true
}

// loadCheckpoint returns a cached checkpoint when available or fetches the
// latest row from storage. The cached copy allows subsequent callers to skip
// the ClickHouse round-trip until a new value is persisted.
func (i *Ingester) loadCheckpoint(ctx context.Context) (addressCheckpoint, bool, error) {
	i.curMu.RLock()
	if i.cur != nil {
		cp := *i.cur
		i.curMu.RUnlock()
		return cp, true, nil
	}
	i.curMu.RUnlock()
	ckpt, err := i.fetchCheckpoint(ctx)
	if err != nil {
		return addressCheckpoint{}, false, err
	}
	if ckpt == nil {
		cp := addressCheckpoint{
			Address:        i.address,
			LastBackfillAt: fmtDT64(0),
			LastDeltaAt:    fmtDT64(0),
			UpdatedAt:      fmtDT64(0),
		}
		i.saveCheckpoint(cp)
		return cp, false, nil
	}
	if ckpt.Address == "" {
		ckpt.Address = i.address
	} else {
		ckpt.Address = strings.ToLower(ckpt.Address)
	}
	if ckpt.LastBackfillAt == "" {
		ckpt.LastBackfillAt = fmtDT64(0)
	}
	if ckpt.LastDeltaAt == "" {
		ckpt.LastDeltaAt = fmtDT64(0)
	}
	if ckpt.UpdatedAt == "" {
		ckpt.UpdatedAt = fmtDT64(0)
	}
	cp := *ckpt
	i.saveCheckpoint(cp)
	return cp, true, nil
}

// fetchCheckpoint queries ClickHouse for the most recent checkpoint row. It
// returns (nil, nil) when no state exists for the address.
func (i *Ingester) fetchCheckpoint(ctx context.Context) (*addressCheckpoint, error) {
	if i.ch == nil || !i.ch.Enabled() {
		return nil, nil
	}
	addr := quoteCHString(i.address)
	query := fmt.Sprintf("SELECT address, last_synced_block, last_backfill_at, last_delta_at, updated_at FROM addresses WHERE address = '%s' ORDER BY updated_at DESC LIMIT 1 FORMAT JSONEachRow SETTINGS output_format_json_quote_64bit_integers = 0", addr)
	rows, err := i.ch.QueryJSONEachRow(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	var ckpt addressCheckpoint
	if err := json.Unmarshal(rows[0], &ckpt); err != nil {
		return nil, fmt.Errorf("decode addresses checkpoint: %w", err)
	}
	return &ckpt, nil
}

// persistCheckpoint writes the checkpoint row, updates timestamps for the
// supplied kind (backfill or delta), and refreshes the cached snapshot.
func (i *Ingester) persistCheckpoint(ctx context.Context, ckpt addressCheckpoint, kind string, synced uint64) error {
	ckpt.Address = i.address
	ckpt.LastSyncedBlock = synced
	now := fmtDT64(timeNow().UTC().UnixMilli())
	switch kind {
	case checkpointBackfill:
		ckpt.LastBackfillAt = now
	case checkpointDelta:
		ckpt.LastDeltaAt = now
	}
	ckpt.UpdatedAt = now
	row := map[string]any{
		"address":           ckpt.Address,
		"last_synced_block": ckpt.LastSyncedBlock,
		"last_backfill_at":  ckpt.LastBackfillAt,
		"last_delta_at":     ckpt.LastDeltaAt,
		"updated_at":        ckpt.UpdatedAt,
	}
	if err := i.ch.InsertJSONEachRow(ctx, "addresses", []any{row}); err != nil {
		return fmt.Errorf("inserting addresses: %w", err)
	}
	i.saveCheckpoint(ckpt)
	return nil
}

// saveCheckpoint caches a copy of the checkpoint for quick reuse.
func (i *Ingester) saveCheckpoint(ckpt addressCheckpoint) {
	i.curMu.Lock()
	copy := ckpt
	i.cur = &copy
	i.curMu.Unlock()
}

// addressCheckpoint mirrors the ClickHouse addresses table for cursor state.
type addressCheckpoint struct {
	Address         string `json:"address"`
	LastSyncedBlock uint64 `json:"last_synced_block"`
	LastBackfillAt  string `json:"last_backfill_at"`
	LastDeltaAt     string `json:"last_delta_at"`
	UpdatedAt       string `json:"updated_at"`
}

// SchemaMode returns the normalized schema mode (dev or canonical).
func (i *Ingester) SchemaMode() string {
	return i.opts.Schema
}

func mustNormalizeOptions(opts Options) Options {
	mode, err := NormalizeSchema(opts.Schema)
	if err != nil {
		panic(err)
	}
	opts.Schema = mode
	return opts
}

// fmtDT64 formats milliseconds since epoch to ClickHouse-compatible DateTime64(3) string (UTC).
func fmtDT64(ms int64) string {
	if ms <= 0 {
		return "1970-01-01 00:00:00.000"
	}
	sec := ms / 1000
	nsec := (ms % 1000) * int64(time.Millisecond)
	t := time.Unix(sec, nsec).UTC()
	// 2006-01-02 15:04:05.000
	return t.Format("2006-01-02 15:04:05.000")
}

func quoteCHString(s string) string {
	replaced := strings.ReplaceAll(s, "\\", "\\\\")
	return strings.ReplaceAll(replaced, "'", "''")
}
