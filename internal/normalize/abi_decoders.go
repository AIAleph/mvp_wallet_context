package normalize

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	fixtureabi "github.com/AIAleph/mvp_wallet_context/fixtures/abi"
	"golang.org/x/crypto/sha3"
)

// Standard ERC token ABIs are embedded to derive selectors and event topics.
type abiArgument struct {
	Type string `json:"type"`
}

type abiItem struct {
	Type   string        `json:"type"`
	Name   string        `json:"name"`
	Inputs []abiArgument `json:"inputs"`
}

var (
	selectorNames map[string]string

	topicTransferFull       string
	topicApprovalFull       string
	topicApprovalForAllFull string
	topicERC1155SingleFull  string
	topicERC1155BatchFull   string
)

func init() {
	selectorNames = make(map[string]string)
	loadStandardABI("erc20", fixtureabi.ERC20)
	loadStandardABI("erc721", fixtureabi.ERC721)
	loadStandardABI("erc1155", fixtureabi.ERC1155)

	// Manual overrides for common non-standard selectors observed on tokens.
	for sel, name := range map[string]string{
		"0x40c10f19": "mint",
		"0x4e71d92d": "claim",
		"0x0181b8ae": "deposit",
		"0x2e1a7d4d": "withdraw",
	} {
		selLower := strings.ToLower(sel)
		selectorNames[selLower] = name
	}

	ensureTopicDefaults()
}

func loadStandardABI(label string, raw []byte) {
	if len(raw) == 0 {
		return
	}
	var items []abiItem
	if err := json.Unmarshal(raw, &items); err != nil {
		panic(fmt.Sprintf("normalize: unable to parse %s ABI: %v", label, err))
	}
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		switch item.Type {
		case "function":
			selector := functionSelector(name, item.Inputs)
			if selector == "" {
				continue
			}
			// Preserve the first discovered label in case of duplicates across standards.
			if _, exists := selectorNames[selector]; !exists {
				selectorNames[selector] = name
			}
		case "event":
			topic := eventTopic(name, item.Inputs)
			if topic == "" {
				continue
			}
			switch strings.ToLower(name) {
			case "transfer":
				topicTransferFull = topic
			case "approval":
				topicApprovalFull = topic
			case "approvalforall":
				topicApprovalForAllFull = topic
			case "transfersingle":
				topicERC1155SingleFull = topic
			case "transferbatch":
				topicERC1155BatchFull = topic
			}
		}
	}
}

func ensureTopicDefaults() {
	// Ensure event topic constants are always populated even if embeds change.
	if topicTransferFull == "" {
		topicTransferFull = mustEventTopic("Transfer", []string{"address", "address", "uint256"})
	}
	if topicApprovalFull == "" {
		topicApprovalFull = mustEventTopic("Approval", []string{"address", "address", "uint256"})
	}
	if topicApprovalForAllFull == "" {
		topicApprovalForAllFull = mustEventTopic("ApprovalForAll", []string{"address", "address", "bool"})
	}
	if topicERC1155SingleFull == "" {
		topicERC1155SingleFull = mustEventTopic("TransferSingle", []string{"address", "address", "address", "uint256", "uint256"})
	}
	if topicERC1155BatchFull == "" {
		topicERC1155BatchFull = mustEventTopic("TransferBatch", []string{"address", "address", "address", "uint256[]", "uint256[]"})
	}

	// Fill in canonical selectors if ABI parsing failed to provide them.
	for sel, name := range map[string]string{
		"0xa9059cbb": "transfer",
		"0x095ea7b3": "approve",
		"0x23b872dd": "transferFrom",
		"0x42842e0e": "safeTransferFrom",
		"0xb88d4fde": "safeTransferFrom",
		"0x2eb2c2d6": "safeBatchTransferFrom",
		"0xa22cb465": "setApprovalForAll",
	} {
		selLower := strings.ToLower(sel)
		if _, ok := selectorNames[selLower]; !ok {
			selectorNames[selLower] = name
		}
	}
}

func functionSelector(name string, inputs []abiArgument) string {
	sig := signature(name, inputs)
	if sig == "" {
		return ""
	}
	return keccakHex(sig, 4)
}

func eventTopic(name string, inputs []abiArgument) string {
	sig := signature(name, inputs)
	if sig == "" {
		return ""
	}
	return keccakHex(sig, 32)
}

func mustEventTopic(name string, types []string) string {
	args := make([]abiArgument, len(types))
	for i, t := range types {
		args[i] = abiArgument{Type: t}
	}
	return eventTopic(name, args)
}

func signature(name string, inputs []abiArgument) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	types := make([]string, len(inputs))
	for i, arg := range inputs {
		typeName := canonicalType(arg.Type)
		if typeName == "" {
			return ""
		}
		types[i] = typeName
	}
	return fmt.Sprintf("%s(%s)", name, strings.Join(types, ","))
}

func canonicalType(t string) string {
	t = strings.TrimSpace(t)
	t = strings.ReplaceAll(t, " ", "")
	return t
}

func keccakHex(sig string, size int) string {
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write([]byte(sig))
	sum := hasher.Sum(nil)
	if size > len(sum) {
		size = len(sum)
	}
	return "0x" + hex.EncodeToString(sum[:size])
}
