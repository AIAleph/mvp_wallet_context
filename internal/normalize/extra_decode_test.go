package normalize

import (
    "fmt"
    "strings"
    "testing"

    "github.com/AIAleph/mvp_wallet_context/internal/eth"
)

func TestDecodeTokenEvents_ERC1155Batch_MismatchedLengths(t *testing.T) {
    // Build ABI-encoded data with ids length 2 and values length 1
    pad := func(n int64) string { return fmt.Sprintf("%064x", n) }
    // head: offIds=0x40, offVals=0xa0
    head := pad(0x40) + pad(0xa0)
    ids := pad(2) + pad(11) + pad(22)
    vals := pad(1) + pad(99)
    data := "0x" + head + ids + vals
    padAddr := func(a string) string { return "0x" + strings.Repeat("0", 24) + strings.TrimPrefix(strings.ToLower(a), "0x") }
    from := "0x1111111111111111111111111111111111111111"
    to := "0x2222222222222222222222222222222222222222"
    l := eth.Log{TxHash: "0xabc", Index: 7, Address: "0xdead", Topics: []string{"0x4a39dc06", "0x" + strings.Repeat("0", 64), padAddr(from), padAddr(to)}, DataHex: data}
    transfers, approvals := DecodeTokenEvents([]eth.Log{l})
    if len(approvals) != 0 { t.Fatalf("unexpected approvals: %v", approvals) }
    if len(transfers) != 1 { t.Fatalf("expected 1 transfer (min lens), got %d", len(transfers)) }
    if transfers[0].TokenID != "11" || transfers[0].AmountRaw != "99" { t.Fatalf("unexpected values: %+v", transfers[0]) }
}

func TestDecodeTokenEvents_ApprovalForAll_False(t *testing.T) {
    padAddr := func(a string) string { return "0x" + strings.Repeat("0", 24) + strings.TrimPrefix(strings.ToLower(a), "0x") }
    owner := "0x1111111111111111111111111111111111111111"
    operator := "0x2222222222222222222222222222222222222222"
    l := eth.Log{TxHash: "0xaaf", Index: 9, Address: "0xdead", Topics: []string{"0x17307eab", padAddr(owner), padAddr(operator)}, DataHex: "0x"}
    _, approvals := DecodeTokenEvents([]eth.Log{l})
    if len(approvals) != 1 || approvals[0].IsForAll != 0 { t.Fatalf("expected isForAll=0, got %+v", approvals)
    }
}

func TestParseERC1155Batch_TruncatedArrayBreak(t *testing.T) {
    // Build data where declared length=2 but only one element provided
    pad := func(n int64) string { return fmt.Sprintf("%064x", n) }
    head := pad(0x40) + pad(0xa0)
    ids := pad(2) + pad(7) // declare 2, provide only 1 element, no vals payload present
    data := "0x" + head + ids
    idsOut, valsOut := parseERC1155Batch(data)
    if len(idsOut) != 1 || idsOut[0] != "7" || valsOut != nil {
        t.Fatalf("unexpected parsed arrays: ids=%v vals=%v", idsOut, valsOut)
    }
}

func TestDecodeTokenEvents_SkipNoTopics(t *testing.T) {
    l := eth.Log{TxHash: "0x1", Index: 0, Address: "0xdead", Topics: nil, DataHex: "0x"}
    tr, ap := DecodeTokenEvents([]eth.Log{l})
    if len(tr) != 0 || len(ap) != 0 { t.Fatalf("expected no rows, got tr=%v ap=%v", tr, ap) }
}
