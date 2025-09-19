package normalize

import (
	"encoding/json"
	"strings"
	"testing"
)

func snapshotSelectors() map[string]string {
	copyMap := make(map[string]string, len(selectorNames))
	for k, v := range selectorNames {
		copyMap[k] = v
	}
	return copyMap
}

func restoreSelectors(m map[string]string) {
	selectorNames = make(map[string]string, len(m))
	for k, v := range m {
		selectorNames[k] = v
	}
}

func TestEnsureTopicDefaultsFillsAndPreserves(t *testing.T) {
	oldTransfer := topicTransferFull
	oldApproval := topicApprovalFull
	oldApprovalForAll := topicApprovalForAllFull
	oldSingle := topicERC1155SingleFull
	oldBatch := topicERC1155BatchFull
	origSelectors := snapshotSelectors()
	defer func() {
		topicTransferFull = oldTransfer
		topicApprovalFull = oldApproval
		topicApprovalForAllFull = oldApprovalForAll
		topicERC1155SingleFull = oldSingle
		topicERC1155BatchFull = oldBatch
		restoreSelectors(origSelectors)
	}()

	// Force zero values to cover the fill branches.
	topicTransferFull = ""
	topicApprovalFull = ""
	topicApprovalForAllFull = ""
	topicERC1155SingleFull = ""
	topicERC1155BatchFull = ""
	selectorNames = make(map[string]string)
	ensureTopicDefaults()

	expect := map[string]string{
		"transfer":       mustEventTopic("Transfer", []string{"address", "address", "uint256"}),
		"approval":       mustEventTopic("Approval", []string{"address", "address", "uint256"}),
		"approvalforall": mustEventTopic("ApprovalForAll", []string{"address", "address", "bool"}),
		"transfersingle": mustEventTopic("TransferSingle", []string{"address", "address", "address", "uint256", "uint256"}),
		"transferbatch":  mustEventTopic("TransferBatch", []string{"address", "address", "address", "uint256[]", "uint256[]"}),
	}

	if topicTransferFull != expect["transfer"] {
		t.Fatalf("expected transfer topic fill, got %s", topicTransferFull)
	}
	if topicApprovalFull != expect["approval"] {
		t.Fatalf("expected approval topic fill, got %s", topicApprovalFull)
	}
	if topicApprovalForAllFull != expect["approvalforall"] {
		t.Fatalf("expected approvalForAll topic fill, got %s", topicApprovalForAllFull)
	}
	if topicERC1155SingleFull != expect["transfersingle"] {
		t.Fatalf("expected ERC1155 single topic fill, got %s", topicERC1155SingleFull)
	}
	if topicERC1155BatchFull != expect["transferbatch"] {
		t.Fatalf("expected ERC1155 batch topic fill, got %s", topicERC1155BatchFull)
	}
	for _, sel := range []string{"0xa9059cbb", "0x095ea7b3", "0x23b872dd"} {
		if _, ok := selectorNames[sel]; !ok {
			t.Fatalf("expected default selector %s to be inserted", sel)
		}
	}

	// Ensure the function short-circuits when topics are already present.
	topicTransferFull = "custom"
	topicApprovalFull = "keep"
	topicApprovalForAllFull = "stay"
	topicERC1155SingleFull = "hold"
	topicERC1155BatchFull = "guard"
	ensureTopicDefaults()
	if topicTransferFull != "custom" || topicApprovalFull != "keep" || topicApprovalForAllFull != "stay" || topicERC1155SingleFull != "hold" || topicERC1155BatchFull != "guard" {
		t.Fatalf("expected existing topics to be preserved")
	}
}

func TestLoadStandardABIEdgeCases(t *testing.T) {
	origSelectors := snapshotSelectors()
	defer restoreSelectors(origSelectors)

	origTransfer := topicTransferFull
	origApproval := topicApprovalFull
	defer func() {
		topicTransferFull = origTransfer
		topicApprovalFull = origApproval
	}()

	// Empty payload should be ignored.
	loadStandardABI("noop", nil)

	payload := []map[string]any{
		{"type": "function", "name": "   ", "inputs": []any{}},
		{"type": "function", "name": "broken", "inputs": []map[string]string{{"type": " "}}},
		{"type": "event", "name": "Untracked", "inputs": []map[string]string{}},
		{"type": "event", "name": "Transfer", "inputs": []map[string]string{{"type": " "}}},
		{"type": "function", "name": "setSomething", "inputs": []map[string]string{{"type": "uint256"}}},
		{"type": "function", "name": "setSomething", "inputs": []map[string]string{{"type": "uint256"}}},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	loadStandardABI("custom", encoded)

	expectSelector := functionSelector("setSomething", []abiArgument{{Type: "uint256"}})
	if selectorNames[expectSelector] != "setSomething" {
		t.Fatalf("expected selector entry for setSomething, got %q", selectorNames[expectSelector])
	}

	if len(selectorNames) != len(origSelectors)+1 {
		t.Fatalf("expected exactly one new selector, got delta %d", len(selectorNames)-len(origSelectors))
	}

	if topicTransferFull != origTransfer || topicApprovalFull != origApproval {
		t.Fatalf("unexpected mutation of known topics")
	}
}

func TestFunctionAndEventSelectorGuards(t *testing.T) {
	if got := functionSelector("   ", nil); got != "" {
		t.Fatalf("expected blank selector, got %s", got)
	}
	if got := eventTopic("   ", nil); got != "" {
		t.Fatalf("expected blank topic, got %s", got)
	}
	if got := functionSelector("transfer", []abiArgument{{Type: ""}}); got != "" {
		t.Fatalf("expected selector rejection for empty type, got %s", got)
	}
	if got := eventTopic("Transfer", []abiArgument{{Type: ""}}); got != "" {
		t.Fatalf("expected topic rejection for empty type, got %s", got)
	}

	selector := functionSelector("transfer", []abiArgument{{Type: " address "}, {Type: "uint256"}})
	if selector != "0xa9059cbb" {
		t.Fatalf("unexpected selector: %s", selector)
	}

	topic := eventTopic("Transfer", []abiArgument{{Type: "address"}, {Type: "address"}, {Type: "uint256"}})
	if topic != expectSelectorTopic("transfer") {
		t.Fatalf("unexpected topic: %s", topic)
	}
}

func expectSelectorTopic(key string) string {
	switch key {
	case "transfer":
		return "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
	case "approval":
		return "0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925"
	case "approvalforall":
		return "0x17307eab39ab6107e8899845ad3d59bd9653f200f220920489ca2b5937696c31"
	case "transfersingle":
		return "0xc3d58168c5ae7397731d063d5bbf3d657854427343f4c083240f7aacaa2d0f62"
	case "transferbatch":
		return "0x4a39dc06d4c0dbc64b70af90fd698a233a518aa5d07e595d983b8c0526c8f7fb"
	default:
		return ""
	}
}

func TestMustEventTopic(t *testing.T) {
	got := mustEventTopic("Sample", []string{"uint256"})
	want := eventTopic("Sample", []abiArgument{{Type: "uint256"}})
	if got != want {
		t.Fatalf("mustEventTopic mismatch: got %s want %s", got, want)
	}
}

func TestKeccakHexClampSize(t *testing.T) {
	got := keccakHex("transfer(address,address,uint256)", 128)
	if len(strings.TrimPrefix(got, "0x"))/2 != 32 {
		t.Fatalf("expected 32-byte digest, got %s", got)
	}
}

func TestTopicMatchesVariants(t *testing.T) {
	full := expectSelectorTopic("transfer")
	if topicMatches("", full) {
		t.Fatalf("expected empty topic to fail match")
	}
	if topicMatches("0x", "") {
		t.Fatalf("expected empty full topic to fail match")
	}
	short := full[:10]
	if !topicMatches(short, full) {
		t.Fatalf("expected prefix topic to match")
	}
	if topicMatches("0xdeadbeef", full) {
		t.Fatalf("expected mismatch for unrelated topic")
	}
}

func TestLoadStandardABIInvalidJSONPanics(t *testing.T) {
	orig := snapshotSelectors()
	defer restoreSelectors(orig)
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for invalid JSON")
		}
	}()
	loadStandardABI("broken", []byte("not-json"))
}
