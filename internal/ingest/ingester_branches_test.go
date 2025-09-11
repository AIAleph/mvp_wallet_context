package ingest

import (
    "context"
    "errors"
    "strings"
    "testing"

    "github.com/AIAleph/mvp_wallet_context/internal/eth"
)

type provRich struct{}

func (provRich) BlockNumber(ctx context.Context) (uint64, error) { return 100, nil }
func (provRich) BlockTimestamp(ctx context.Context, block uint64) (int64, error) { return int64(block) * 1000, nil }
func (provRich) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) {
    return []eth.Trace{{TxHash: "0x1", TraceID: "root", From: address, To: address, ValueWei: "0x1", BlockNum: from}}, nil
}
func (provRich) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) {
    padAddr := func(a string) string { return "0x" + strings.Repeat("0", 24) + strings.TrimPrefix(strings.ToLower(a), "0x") }
    fromA := "0x1111111111111111111111111111111111111111"
    toA := "0x2222222222222222222222222222222222222222"
    spender := "0x3333333333333333333333333333333333333333"
    token := "0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
    // ERC20 Transfer
    l1 := eth.Log{TxHash: "0xa", Index: 0, Address: token, Topics: []string{"0xddf252ad", padAddr(fromA), padAddr(toA)}, DataHex: "0x" + strings.Repeat("0", 63) + "a", BlockNum: from}
    // ApprovalForAll
    l2 := eth.Log{TxHash: "0xb", Index: 1, Address: token, Topics: []string{"0x17307eab", padAddr(fromA), padAddr(spender)}, DataHex: "0x" + strings.Repeat("0", 63) + "1", BlockNum: from}
    // ERC1155 TransferSingle id=1, val=2
    data := "0x" + strings.Repeat("0", 63) + "1" + strings.Repeat("0", 63) + "2"
    l3 := eth.Log{TxHash: "0xc", Index: 2, Address: token, Topics: []string{"0xc3d58168", "0x"+strings.Repeat("0",64), padAddr(fromA), padAddr(toA)}, DataHex: data, BlockNum: from}
    return []eth.Log{l1, l2, l3}, nil
}

func TestProcessRange_WritesAllDevTables(t *testing.T) {
    // With empty ClickHouse DSN, inserts are no-ops but code paths execute
    ing := NewWithProvider("0xabc", Options{}, provRich{})
    if err := ing.processRange(context.Background(), 10, 10); err != nil {
        t.Fatal(err)
    }
}

type provTraceErr struct{}
func (provTraceErr) BlockNumber(ctx context.Context) (uint64, error) { return 100, nil }
func (provTraceErr) BlockTimestamp(ctx context.Context, block uint64) (int64, error) { return 0, nil }
func (provTraceErr) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]eth.Log, error) { return nil, nil }
func (provTraceErr) TraceBlock(ctx context.Context, from, to uint64, address string) ([]eth.Trace, error) { return nil, errors.New("trace") }

func TestProcessRange_TraceUnexpectedError(t *testing.T) {
    ing := NewWithProvider("0x", Options{}, provTraceErr{})
    if err := ing.processRange(context.Background(), 1, 1); err == nil {
        t.Fatal("expected error from TraceBlock")
    }
}
