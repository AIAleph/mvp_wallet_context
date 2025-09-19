package normalize

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/AIAleph/mvp_wallet_context/internal/eth"
)

// Package normalize converts heterogeneous RPC payloads (tx, logs, traces)
// into canonical ClickHouse rows. Focus: no floats, UTC millis, stable IDs.

// LogRow represents a normalized log/event row for dev ingestion.
type LogRow struct {
	EventUID string   `json:"event_uid"`
	TxHash   string   `json:"tx_hash"`
	LogIndex uint32   `json:"log_index"`
	Address  string   `json:"address"`
	Topics   []string `json:"topics"`
	DataHex  string   `json:"data_hex"`
	BlockNum uint64   `json:"block_number"`
	TsMillis int64    `json:"ts_millis"`
}

// TraceRow represents a normalized internal trace row for dev ingestion.
type TraceRow struct {
	TraceUID string `json:"trace_uid"`
	TxHash   string `json:"tx_hash"`
	TraceID  string `json:"trace_id"`
	From     string `json:"from_addr"`
	To       string `json:"to_addr"`
	ValueRaw string `json:"value_raw"`
	BlockNum uint64 `json:"block_number"`
	TsMillis int64  `json:"ts_millis"`
}

// TransactionRow represents a normalized transaction row (external or internal).
type TransactionRow struct {
	TxHash      string `json:"tx_hash"`
	BlockNum    uint64 `json:"block_number"`
	TsMillis    int64  `json:"ts_millis"`
	From        string `json:"from_addr"`
	To          string `json:"to_addr"`
	ValueRaw    string `json:"value_raw"`
	GasUsed     uint64 `json:"gas_used"`
	Status      uint8  `json:"status"`
	InputMethod string `json:"input_method"`
	IsInternal  uint8  `json:"is_internal"`
	TraceID     string `json:"trace_id"`
}

// LogsToRows maps eth.Log to normalized LogRow with stable event_uid.
func LogsToRows(in []eth.Log) []LogRow {
	out := make([]LogRow, 0, len(in))
	for _, l := range in {
		out = append(out, LogRow{
			EventUID: fmt.Sprintf("%s:%d", l.TxHash, l.Index),
			TxHash:   l.TxHash,
			LogIndex: l.Index,
			Address:  l.Address,
			Topics:   append([]string(nil), l.Topics...),
			DataHex:  l.DataHex,
			BlockNum: l.BlockNum,
			TsMillis: l.TsMillis,
		})
	}
	return out
}

// TracesToRows maps eth.Trace to normalized TraceRow with stable trace_uid.
func TracesToRows(in []eth.Trace) []TraceRow {
	out := make([]TraceRow, 0, len(in))
	for _, t := range in {
		out = append(out, TraceRow{
			TraceUID: fmt.Sprintf("%s:%s", t.TxHash, t.TraceID),
			TxHash:   t.TxHash,
			TraceID:  t.TraceID,
			From:     t.From,
			To:       t.To,
			ValueRaw: t.ValueWei,
			BlockNum: t.BlockNum,
			TsMillis: t.TsMillis,
		})
	}
	return out
}

// DecodeInputMethod maps calldata selectors to short method labels. Unknown
// selectors return the 4-byte hex prefix, empty/short inputs return "".
func DecodeInputMethod(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	if len(input) <= len("0x") || !strings.HasPrefix(input, "0x") {
		return ""
	}
	if len(input) < 10 {
		return ""
	}
	selector := input[:10]
	if selector == "0x00000000" {
		return ""
	}
	if name, ok := selectorNames[selector]; ok {
		return name
	}
	return selector
}

func valueToDecimalString(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "0"
	}
	if strings.HasPrefix(strings.ToLower(s), "0x") {
		return hexToBigIntString(s)
	}
	// Assume decimal if parseable; otherwise return raw for visibility.
	if _, ok := new(big.Int).SetString(s, 10); ok {
		return s
	}
	return s
}

// TransactionsToRows normalizes provider transactions to canonical rows.
func TransactionsToRows(in []eth.Transaction, isInternal bool) []TransactionRow {
	out := make([]TransactionRow, 0, len(in))
	internalFlag := uint8(0)
	if isInternal {
		internalFlag = 1
	}
	for _, tx := range in {
		row := TransactionRow{
			TxHash:      strings.ToLower(tx.Hash),
			BlockNum:    tx.BlockNum,
			TsMillis:    tx.TsMillis,
			From:        strings.ToLower(tx.From),
			To:          strings.ToLower(tx.To),
			ValueRaw:    valueToDecimalString(tx.ValueWei),
			GasUsed:     tx.GasUsed,
			Status:      tx.Status,
			InputMethod: "",
			IsInternal:  internalFlag,
			TraceID:     tx.TraceID,
		}
		if tx.To == "" {
			row.To = ""
		}
		if m := DecodeInputMethod(tx.InputHex); m != "" {
			row.InputMethod = m
		}
		out = append(out, row)
	}
	return out
}

// AsAny converts a typed slice into []any for generic encoders.
func AsAny[T any](in []T) []any {
	out := make([]any, len(in))
	for i := range in {
		out[i] = in[i]
	}
	return out
}

// Token and approval decoding

type TokenTransferRow struct {
	EventUID  string `json:"event_uid"`
	TxHash    string `json:"tx_hash"`
	LogIndex  uint32 `json:"log_index"`
	Token     string `json:"token"`
	From      string `json:"from_addr"`
	To        string `json:"to_addr"`
	AmountRaw string `json:"amount_raw"`
	TokenID   string `json:"token_id"`
	BatchOrd  uint16 `json:"batch_ordinal"`
	Standard  string `json:"standard"` // erc20|erc721|erc1155
	BlockNum  uint64 `json:"block_number"`
	TsMillis  int64  `json:"ts_millis"`
}

type ApprovalRow struct {
	EventUID  string `json:"event_uid"`
	TxHash    string `json:"tx_hash"`
	LogIndex  uint32 `json:"log_index"`
	Token     string `json:"token"`
	Owner     string `json:"owner"`
	Spender   string `json:"spender"`
	AmountRaw string `json:"amount_raw"`
	TokenID   string `json:"token_id"`
	IsForAll  uint8  `json:"is_approval_for_all"`
	Standard  string `json:"standard"`
	BlockNum  uint64 `json:"block_number"`
	TsMillis  int64  `json:"ts_millis"`
}

func hexToBigIntString(s string) string {
	s = strings.TrimPrefix(s, "0x")
	if s == "" {
		return "0"
	}
	b := new(big.Int)
	b.SetString(s, 16)
	return b.String()
}

// DecodeTokenEvents extracts token transfers and approvals from logs.
func DecodeTokenEvents(logs []eth.Log) (transfers []TokenTransferRow, approvals []ApprovalRow) {
	for _, l := range logs {
		if len(l.Topics) == 0 {
			continue
		}
		t0 := strings.ToLower(l.Topics[0])
		switch {
		case topicMatches(t0, topicTransferFull):
			// ERC20 vs ERC721 heuristic
			// ERC20: topics[1]=from, topics[2]=to, data=amount
			// ERC721: topics[1]=from, topics[2]=to, topics[3]=tokenId, data empty
			var amountRaw, tokenID, standard string
			if len(l.Topics) >= 3 && len(l.DataHex) >= 2 {
				amountRaw = hexToBigIntString(l.DataHex)
				tokenID = ""
				standard = "erc20"
			}
			if len(l.Topics) >= 4 && (l.DataHex == "0x" || l.DataHex == "") {
				tokenID = hexToBigIntString(l.Topics[3])
				amountRaw = "1"
				standard = "erc721"
			}
			transfers = append(transfers, TokenTransferRow{
				EventUID:  fmt.Sprintf("%s:%d", l.TxHash, l.Index),
				TxHash:    l.TxHash,
				LogIndex:  l.Index,
				Token:     l.Address,
				From:      addrFromTopic(l.Topics, 1),
				To:        addrFromTopic(l.Topics, 2),
				AmountRaw: amountRaw,
				TokenID:   tokenID,
				Standard:  standard,
				BlockNum:  l.BlockNum,
				TsMillis:  l.TsMillis,
			})
		case topicMatches(t0, topicApprovalFull):
			// ERC20: topics[1]=owner, topics[2]=spender, data=amount
			// ERC721: topics[1]=owner, topics[2]=approved, topics[3]=tokenId, data empty
			var amt, tokenID, standard string
			var isForAll uint8
			if len(l.Topics) >= 3 && len(l.DataHex) >= 2 {
				amt = hexToBigIntString(l.DataHex)
				standard = "erc20"
			}
			if len(l.Topics) >= 4 && (l.DataHex == "0x" || l.DataHex == "") {
				tokenID = hexToBigIntString(l.Topics[3])
				amt = "1"
				standard = "erc721"
			}
			approvals = append(approvals, ApprovalRow{
				EventUID:  fmt.Sprintf("%s:%d", l.TxHash, l.Index),
				TxHash:    l.TxHash,
				LogIndex:  l.Index,
				Token:     l.Address,
				Owner:     addrFromTopic(l.Topics, 1),
				Spender:   addrFromTopic(l.Topics, 2),
				AmountRaw: amt,
				TokenID:   tokenID,
				IsForAll:  isForAll,
				Standard:  standard,
				BlockNum:  l.BlockNum,
				TsMillis:  l.TsMillis,
			})
		case topicMatches(t0, topicApprovalForAllFull):
			// owner, operator in topics; data is bool
			isForAll := uint8(0)
			if strings.HasSuffix(strings.ToLower(l.DataHex), strings.Repeat("0", 63)+"1") {
				isForAll = 1
			}
			approvals = append(approvals, ApprovalRow{
				EventUID:  fmt.Sprintf("%s:%d", l.TxHash, l.Index),
				TxHash:    l.TxHash,
				LogIndex:  l.Index,
				Token:     l.Address,
				Owner:     addrFromTopic(l.Topics, 1),
				Spender:   addrFromTopic(l.Topics, 2),
				AmountRaw: "0",
				TokenID:   "",
				IsForAll:  isForAll,
				Standard:  "erc721",
				BlockNum:  l.BlockNum,
				TsMillis:  l.TsMillis,
			})
		case topicMatches(t0, topicERC1155SingleFull):
			// topics: [sig, operator, from, to]; data: id, value
			fields := splitDataWords(l.DataHex)
			var id, val string
			if len(fields) >= 2 {
				id = hexToBigIntString(fields[0])
				val = hexToBigIntString(fields[1])
			}
			transfers = append(transfers, TokenTransferRow{
				EventUID:  fmt.Sprintf("%s:%d", l.TxHash, l.Index),
				TxHash:    l.TxHash,
				LogIndex:  l.Index,
				Token:     l.Address,
				From:      addrFromTopic(l.Topics, 2),
				To:        addrFromTopic(l.Topics, 3),
				AmountRaw: val,
				TokenID:   id,
				Standard:  "erc1155",
				BlockNum:  l.BlockNum,
				TsMillis:  l.TsMillis,
			})
		case topicMatches(t0, topicERC1155BatchFull):
			ids, vals := parseERC1155Batch(l.DataHex)
			n := len(ids)
			if len(vals) < n {
				n = len(vals)
			}
			for k := 0; k < n; k++ {
				transfers = append(transfers, TokenTransferRow{
					EventUID:  fmt.Sprintf("%s:%d:%d", l.TxHash, l.Index, k),
					TxHash:    l.TxHash,
					LogIndex:  l.Index,
					Token:     l.Address,
					From:      addrFromTopic(l.Topics, 2),
					To:        addrFromTopic(l.Topics, 3),
					AmountRaw: vals[k],
					TokenID:   ids[k],
					BatchOrd:  uint16(k),
					Standard:  "erc1155",
					BlockNum:  l.BlockNum,
					TsMillis:  l.TsMillis,
				})
			}
		}
	}
	return
}

func topicMatches(topic, full string) bool {
	if full == "" {
		return false
	}
	topic = strings.ToLower(topic)
	full = strings.ToLower(full)
	if topic == full {
		return true
	}
	if len(topic) >= 10 && len(full) >= 10 && strings.HasPrefix(topic, full[:10]) {
		return true
	}
	return false
}

func addrFromTopic(topics []string, idx int) string {
	if idx >= len(topics) {
		return ""
	}
	t := topics[idx]
	if len(t) == 66 { // 0x + 64
		return "0x" + strings.ToLower(t[26:])
	}
	if strings.HasPrefix(t, "0x") && len(t) >= 42 {
		return strings.ToLower(t[:42])
	}
	return strings.ToLower(t)
}

func splitDataWords(data string) []string {
	d := strings.TrimPrefix(data, "0x")
	if d == "" {
		return nil
	}
	var out []string
	for i := 0; i+64 <= len(d); i += 64 {
		out = append(out, "0x"+d[i:i+64])
	}
	return out
}

// parseERC1155Batch decodes ABI-encoded arrays (ids, values) from data for TransferBatch.
func parseERC1155Batch(data string) (ids []string, vals []string) {
	d := strings.TrimPrefix(data, "0x")
	if len(d) < 64*2 {
		return nil, nil
	}
	// Offsets are in bytes from start
	offIds := wordToInt(d[0:64])
	offVals := wordToInt(d[64:128])
	// Helper to read dynamic array at offset
	readArray := func(offBytes int) []string {
		// Convert byte offset to hex index (2 hex chars per byte)
		idx := offBytes * 2
		if idx%64 != 0 || idx < 0 || idx+64 > len(d) {
			return nil
		}
		// length at offset
		length := wordToInt(d[idx : idx+64])
		out := make([]string, 0, length)
		base := idx + 64
		for i := 0; i < length; i++ {
			start := base + i*64
			end := start + 64
			if end > len(d) {
				break
			}
			out = append(out, hexToBigIntString("0x"+d[start:end]))
		}
		return out
	}
	ids = readArray(offIds)
	vals = readArray(offVals)
	return ids, vals
}

func wordToInt(word string) int {
	// word is 64 hex chars (no 0x)
	s := strings.TrimPrefix(word, "0x")
	if s == "" {
		return 0
	}
	b := new(big.Int)
	b.SetString(s, 16)
	if !b.IsInt64() {
		return 0
	}
	return int(b.Int64())
}
