package normalize

// Covers ERC-1155 single transfer decoding and helper utilities.

import (
	"strings"
	"testing"

	"github.com/AIAleph/mvp_wallet_context/internal/eth"
)

func TestDecodeERC1155Single(t *testing.T) {
	from := "0x1111111111111111111111111111111111111111"
	to := "0x2222222222222222222222222222222222222222"
	padAddr := func(a string) string {
		return "0x" + strings.Repeat("0", 24) + strings.TrimPrefix(strings.ToLower(a), "0x")
	}
	// data: id=7, value=99
	data := "0x" + pad32Hex(7) + pad32Hex(99)
	l := eth.Log{TxHash: "0x1", Index: 1, Address: "0xdead", Topics: []string{topicERC1155SingleFull, "0x" + strings.Repeat("0", 64), padAddr(from), padAddr(to)}, DataHex: data}
	transfers, approvals := DecodeTokenEvents([]eth.Log{l})
	if len(approvals) != 0 || len(transfers) != 1 {
		t.Fatalf("unexpected counts")
	}
	if transfers[0].Standard != "erc1155" || transfers[0].TokenID != "7" || transfers[0].AmountRaw != "99" {
		t.Fatalf("mismatch: %+v", transfers[0])
	}
}

func TestSplitDataWordsEmpty(t *testing.T) {
	if words := splitDataWords("0x"); len(words) != 0 {
		t.Fatalf("expected empty words")
	}
}

func TestWordToIntOverflow(t *testing.T) {
	// 80 hex 'f' is > int64 -> returns 0
	if got := wordToInt(strings.Repeat("f", 80)); got != 0 {
		t.Fatalf("overflow got %d", got)
	}
}
