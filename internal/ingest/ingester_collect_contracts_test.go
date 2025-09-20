package ingest

import (
	"testing"

	"github.com/AIAleph/mvp_wallet_context/internal/eth"
)

func TestCollectContractCreations(t *testing.T) {
	txs := []eth.Transaction{
		{Hash: "0xhash1", From: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", To: "", ContractAddress: "0x1111111111111111111111111111111111111111", BlockNum: 10},
		{Hash: "0xhash2", From: "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", To: "", ContractAddress: "0x1111111111111111111111111111111111111111", BlockNum: 9},
		{Hash: "0xhash0", From: "0xabababababababababababababababababababab", To: "", ContractAddress: "0x1111111111111111111111111111111111111111", BlockNum: 9},
		{Hash: "0xhash3", From: "0xcccccccccccccccccccccccccccccccccccccccc", To: "0x123", ContractAddress: "0x2222222222222222222222222222222222222222", BlockNum: 8},
		{Hash: "0xhash4", From: "0xdddddddddddddddddddddddddddddddddddddddd", To: "", ContractAddress: "", BlockNum: 12},
		{Hash: "0xhash5", From: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", To: "", ContractAddress: "0xZZ", BlockNum: 13},
		{Hash: "0xhash6", From: "0xfafafafafafafafafafafafafafafafafafafafa", To: "", ContractAddress: "  ", BlockNum: 14},
	}
	traces := []eth.Trace{
		{TxHash: "0xtrace1", Type: "call", CreatedContract: "0x3333333333333333333333333333333333333333", BlockNum: 15},
		{TxHash: "0xtrace2", Type: "create", CreatedContract: "0x4444444444444444444444444444444444444444", BlockNum: 7, From: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"},
		{TxHash: "0xtrace3", Type: "create2", CreatedContract: "0x5555555555555555555555555555555555555555", BlockNum: 5},
		{TxHash: "0xtrace4", Type: "create", CreatedContract: "", BlockNum: 6},
		{TxHash: "0xtrace5", Type: "create", CreatedContract: "0xZZ", BlockNum: 8},
	}

	out := collectContractCreations(txs, traces, "")
	if len(out) != 3 {
		t.Fatalf("expected 3 creations, got %d", len(out))
	}
	if out[0].address != "0x5555555555555555555555555555555555555555" || out[0].blockNumber != 5 || out[0].txHash != "0xtrace3" {
		t.Fatalf("unexpected first entry: %+v", out[0])
	}
	if out[1].address != "0x4444444444444444444444444444444444444444" || out[1].blockNumber != 7 {
		t.Fatalf("unexpected second entry: %+v", out[1])
	}
	if out[2].address != "0x1111111111111111111111111111111111111111" || out[2].blockNumber != 9 || out[2].txHash != "0xhash0" {
		t.Fatalf("unexpected third entry: %+v", out[2])
	}
}

func TestCollectContractCreationsWithTarget(t *testing.T) {
	target := "0xffffffffffffffffffffffffffffffffffffffff"
	txs := []eth.Transaction{
		{Hash: "0xhash1", From: target, To: "", ContractAddress: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", BlockNum: 3},
		{Hash: "0xhash2", From: "0x1234567890123456789012345678901234567890", To: "", ContractAddress: "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", BlockNum: 4},
	}
	traces := []eth.Trace{
		{TxHash: "0xtrace1", Type: "create", CreatedContract: "0xcccccccccccccccccccccccccccccccccccccccc", BlockNum: 6, From: target},
		{TxHash: "0xtrace2", Type: "create2", CreatedContract: "0xdddddddddddddddddddddddddddddddddddddddd", BlockNum: 5, To: target},
		{TxHash: "0xtrace3", Type: "create", CreatedContract: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", BlockNum: 7, From: "0x1111111111111111111111111111111111111111"},
	}

	out := collectContractCreations(txs, traces, target)
	if len(out) != 3 {
		t.Fatalf("expected 3 creations, got %d", len(out))
	}
	if out[0].address != "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || out[0].blockNumber != 3 {
		t.Fatalf("unexpected tx-derived creation: %+v", out[0])
	}
	if out[1].address != "0xdddddddddddddddddddddddddddddddddddddddd" {
		t.Fatalf("expected to include trace matching to-address: %+v", out[1])
	}
	if out[2].address != "0xcccccccccccccccccccccccccccccccccccccccc" {
		t.Fatalf("expected to include trace matching from-address: %+v", out[2])
	}
}

func TestCollectContractCreationsEmptyInput(t *testing.T) {
	if out := collectContractCreations(nil, nil, ""); out != nil {
		t.Fatalf("expected nil for empty input, got %v", out)
	}
}

func TestCollectContractCreationsSortByAddress(t *testing.T) {
	txs := []eth.Transaction{
		{Hash: "0x2", From: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", To: "", ContractAddress: "0x2222222222222222222222222222222222222222", BlockNum: 5},
		{Hash: "0x1", From: "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", To: "", ContractAddress: "0x1111111111111111111111111111111111111111", BlockNum: 5},
	}
	out := collectContractCreations(txs, nil, "")
	if len(out) != 2 {
		t.Fatalf("expected 2 creations, got %d", len(out))
	}
	if out[0].address != "0x1111111111111111111111111111111111111111" {
		t.Fatalf("expected lexical order when block numbers match: %+v", out)
	}
}

func TestCollectContractCreationsInvalidAddresses(t *testing.T) {
	txs := []eth.Transaction{{Hash: "0xhash", From: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", To: "", ContractAddress: "0xnothex", BlockNum: 1}}
	traces := []eth.Trace{{TxHash: "0xtrace", Type: "create", CreatedContract: "0xnothex", BlockNum: 2}}
	if out := collectContractCreations(txs, traces, ""); out != nil {
		t.Fatalf("expected nil due to invalid addresses, got %v", out)
	}
}
