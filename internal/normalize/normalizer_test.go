package normalize

import (
	"fmt"
	"strings"
	"testing"

	"github.com/AIAleph/mvp_wallet_context/internal/eth"
)

func pad32Hex(n int64) string { return fmt.Sprintf("%064x", n) }

func TestDecodeERC1155BatchGolden(t *testing.T) {
	// Build ABI-encoded data for ids=[5,7], values=[100,200]
	// Head: [offset ids=0x40][offset vals=0xa0]
	head := pad32Hex(0x40) + pad32Hex(0xa0)
	ids := pad32Hex(2) + pad32Hex(5) + pad32Hex(7)
	vals := pad32Hex(2) + pad32Hex(100) + pad32Hex(200)
	data := "0x" + head + ids + vals

	from := "0x1111111111111111111111111111111111111111"
	to := "0x2222222222222222222222222222222222222222"
	padAddr := func(a string) string {
		return "0x" + strings.Repeat("0", 24) + strings.TrimPrefix(strings.ToLower(a), "0x")
	}

	l := eth.Log{
		TxHash:   "0xabc",
		Index:    3,
		Address:  "0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		Topics:   []string{"0x4a39dc06", "0x" + strings.Repeat("0", 64), padAddr(from), padAddr(to)},
		DataHex:  data,
		BlockNum: 16,
		TsMillis: 100000,
	}

	transfers, approvals := DecodeTokenEvents([]eth.Log{l})
	if len(approvals) != 0 {
		t.Fatalf("unexpected approvals: %+v", approvals)
	}
	if len(transfers) != 2 {
		t.Fatalf("expected 2 transfers, got %d", len(transfers))
	}

	if transfers[0].EventUID != "0xabc:3:0" || transfers[1].EventUID != "0xabc:3:1" {
		t.Fatalf("unexpected event_uids: %s, %s", transfers[0].EventUID, transfers[1].EventUID)
	}
	if transfers[0].TokenID != "5" || transfers[1].TokenID != "7" {
		t.Fatalf("unexpected token ids: %s, %s", transfers[0].TokenID, transfers[1].TokenID)
	}
	if transfers[0].AmountRaw != "100" || transfers[1].AmountRaw != "200" {
		t.Fatalf("unexpected amounts: %s, %s", transfers[0].AmountRaw, transfers[1].AmountRaw)
	}
	if transfers[0].From != strings.ToLower(from) || transfers[0].To != strings.ToLower(to) {
		t.Fatalf("addr parse mismatch: from=%s to=%s", transfers[0].From, transfers[0].To)
	}
	if transfers[0].Standard != "erc1155" || transfers[1].Standard != "erc1155" {
		t.Fatalf("unexpected standard: %s, %s", transfers[0].Standard, transfers[1].Standard)
	}
}

func TestAddrFromTopicVariants(t *testing.T) {
	// 32-byte padded topic
	addr := "0xABCDEFabcdefABCDEFabcdefABCDEFabcdefABCD"
	padded := "0x" + strings.Repeat("0", 24) + strings.TrimPrefix(strings.ToLower(addr), "0x")
	got := addrFromTopic([]string{padded}, 0)
	if got != strings.ToLower(addr) {
		t.Fatalf("padded parse got %s want %s", got, strings.ToLower(addr))
	}

	// Already 0x-prefixed 40-hex form
	simple := strings.ToLower(addr)
	got2 := addrFromTopic([]string{simple}, 0)
	if got2 != simple[:42] {
		t.Fatalf("simple parse got %s want %s", got2, simple[:42])
	}
}

func TestDecodeERC20AndERC721TransfersAndApprovals(t *testing.T) {
	padAddr := func(a string) string {
		return "0x" + strings.Repeat("0", 24) + strings.TrimPrefix(strings.ToLower(a), "0x")
	}
	addrToken := "0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	from := "0x1111111111111111111111111111111111111111"
	to := "0x2222222222222222222222222222222222222222"
	spender := "0x3333333333333333333333333333333333333333"

	// ERC-20 Transfer
	l20t := eth.Log{
		TxHash:  "0xaaa",
		Index:   1,
		Address: addrToken,
		Topics:  []string{"0xddf252ad", padAddr(from), padAddr(to)},
		DataHex: "0x" + pad32Hex(1234),
	}
	// ERC-721 Transfer
	l721t := eth.Log{
		TxHash:  "0xaab",
		Index:   2,
		Address: addrToken,
		Topics:  []string{"0xddf252ad", padAddr(from), padAddr(to), "0x" + pad32Hex(99)},
		DataHex: "0x",
	}
	// ERC-20 Approval
	l20a := eth.Log{
		TxHash:  "0xaac",
		Index:   3,
		Address: addrToken,
		Topics:  []string{"0x8c5be1e5", padAddr(from), padAddr(spender)},
		DataHex: "0x" + pad32Hex(555),
	}
	// ERC-721 Approval
	l721a := eth.Log{
		TxHash:  "0xaad",
		Index:   4,
		Address: addrToken,
		Topics:  []string{"0x8c5be1e5", padAddr(from), padAddr(spender), "0x" + pad32Hex(42)},
		DataHex: "0x",
	}
	// ApprovalForAll true
	lForAll := eth.Log{
		TxHash:  "0xaae",
		Index:   5,
		Address: addrToken,
		Topics:  []string{"0x17307eab", padAddr(from), padAddr(spender)},
		DataHex: "0x" + strings.Repeat("0", 63) + "1",
	}

	transfers, approvals := DecodeTokenEvents([]eth.Log{l20t, l721t, l20a, l721a, lForAll})
	if len(transfers) != 2 {
		t.Fatalf("transfers=%d want 2", len(transfers))
	}
	if transfers[0].Standard != "erc20" || transfers[0].AmountRaw != "1234" || transfers[0].TokenID != "" {
		t.Fatalf("erc20 transfer mismatch: %+v", transfers[0])
	}
	if transfers[1].Standard != "erc721" || transfers[1].TokenID != "99" || transfers[1].AmountRaw != "1" {
		t.Fatalf("erc721 transfer mismatch: %+v", transfers[1])
	}

	if len(approvals) != 3 {
		t.Fatalf("approvals=%d want 3", len(approvals))
	}
	// ERC-20 approval
	if approvals[0].Standard != "erc20" || approvals[0].AmountRaw != "555" || approvals[0].IsForAll != 0 {
		t.Fatalf("erc20 approval mismatch: %+v", approvals[0])
	}
	// ERC-721 single token approval
	if approvals[1].Standard != "erc721" || approvals[1].TokenID != "42" || approvals[1].IsForAll != 0 {
		t.Fatalf("erc721 approval mismatch: %+v", approvals[1])
	}
	// ApprovalForAll
	if approvals[2].IsForAll != 1 || approvals[2].Standard != "erc721" {
		t.Fatalf("approvalForAll mismatch: %+v", approvals[2])
	}
}

func TestDecodeInputMethod(t *testing.T) {
	cases := map[string]string{
		"0xa9059cbb0000": "transfer",
		"0x095ea7b3dead": "approve",
		"0x123":          "",
		"":               "",
		"0x23b872ddabcd": "transferFrom",
		"0xabcdef012345": "0xabcdef01",
		"0x00000000abcd": "",
	}
	for input, want := range cases {
		if got := DecodeInputMethod(input); got != want {
			t.Fatalf("DecodeInputMethod(%s)=%s want %s", input, got, want)
		}
	}
}

func TestTransactionsToRows(t *testing.T) {
	txs := []eth.Transaction{{
		Hash:     "0xABC",
		From:     "0x1111111111111111111111111111111111111111",
		To:       "0x2222222222222222222222222222222222222222",
		ValueWei: "0xde",
		InputHex: "0xa9059cbb00000000000000000000000000000000000000000000000000000001",
		GasUsed:  21000,
		Status:   1,
		BlockNum: 42,
		TsMillis: 1234,
	}}
	rows := TransactionsToRows(txs, false)
	if len(rows) != 1 {
		t.Fatalf("rows=%d", len(rows))
	}
	row := rows[0]
	if row.TxHash != "0xabc" || row.From != strings.ToLower(txs[0].From) || row.To != strings.ToLower(txs[0].To) {
		t.Fatalf("address/hash normalization failed: %+v", row)
	}
	if row.ValueRaw != "222" { // 0xde hex -> 222 decimal
		t.Fatalf("value normalization failed: %s", row.ValueRaw)
	}
	if row.InputMethod != "transfer" {
		t.Fatalf("input method decode failed: %s", row.InputMethod)
	}
	if row.IsInternal != 0 {
		t.Fatalf("expected external transaction, got %d", row.IsInternal)
	}
}

func TestTransactionsToRowsVariants(t *testing.T) {
	txs := []eth.Transaction{{
		Hash:     "0xdef",
		From:     "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		To:       "",
		ValueWei: "12345",
		InputHex: "0x",
		GasUsed:  42000,
		Status:   0,
		BlockNum: 55,
		TsMillis: 999,
	}}
	rows := TransactionsToRows(txs, true)
	if len(rows) != 1 {
		t.Fatalf("rows=%d", len(rows))
	}
	row := rows[0]
	if row.To != "" {
		t.Fatalf("expected empty to address, got %q", row.To)
	}
	if row.IsInternal != 1 {
		t.Fatalf("expected internal flag set, got %d", row.IsInternal)
	}
	if row.InputMethod != "" {
		t.Fatalf("unexpected input method: %s", row.InputMethod)
	}
	if row.ValueRaw != "12345" {
		t.Fatalf("expected decimal passthrough, got %s", row.ValueRaw)
	}
}

func TestValueToDecimalString(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"0xde", "222"},
		{" 0x2a ", "42"},
		{"98765", "98765"},
		{"", "0"},
		{"not-a-number", "not-a-number"},
		{"0X2a", "0"},
	}
	for _, tc := range cases {
		if got := valueToDecimalString(tc.in); got != tc.want {
			t.Fatalf("valueToDecimalString(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}
