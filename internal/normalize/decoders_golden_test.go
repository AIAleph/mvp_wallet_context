package normalize

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/AIAleph/mvp_wallet_context/internal/eth"
)

type goldenLog struct {
	TxHash      string   `json:"tx_hash"`
	LogIndex    uint32   `json:"log_index"`
	Address     string   `json:"address"`
	Topics      []string `json:"topics"`
	Data        string   `json:"data"`
	BlockNumber uint64   `json:"block_number"`
	TsMillis    int64    `json:"ts_millis"`
}

type tokenEventsFixture struct {
	Logs      []goldenLog        `json:"logs"`
	Transfers []TokenTransferRow `json:"transfers"`
	Approvals []ApprovalRow      `json:"approvals"`
}

func loadTokenEventsFixture(t *testing.T) tokenEventsFixture {
	t.Helper()
	data, err := os.ReadFile(fixturePath("token_events_golden.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fx tokenEventsFixture
	if err := json.Unmarshal(data, &fx); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	return fx
}

func TestDecodeTokenEvents_GoldenFixture(t *testing.T) {
	fx := loadTokenEventsFixture(t)
	logs := make([]eth.Log, len(fx.Logs))
	for i, l := range fx.Logs {
		logs[i] = eth.Log{
			TxHash:   l.TxHash,
			Index:    l.LogIndex,
			Address:  l.Address,
			Topics:   append([]string(nil), l.Topics...),
			DataHex:  l.Data,
			BlockNum: l.BlockNumber,
			TsMillis: l.TsMillis,
		}
	}

	transfers, approvals := DecodeTokenEvents(logs)
	if !reflect.DeepEqual(transfers, fx.Transfers) {
		got, want := mustJSON(transfers), mustJSON(fx.Transfers)
		t.Fatalf("transfers mismatch\nwant=%s\n got=%s", want, got)
	}
	if !reflect.DeepEqual(approvals, fx.Approvals) {
		got, want := mustJSON(approvals), mustJSON(fx.Approvals)
		t.Fatalf("approvals mismatch\nwant=%s\n got=%s", want, got)
	}
}

type inputMethodFixture struct {
	Cases []struct {
		Input  string `json:"input"`
		Method string `json:"method"`
	} `json:"cases"`
}

func TestDecodeInputMethod_GoldenFixture(t *testing.T) {
	data, err := os.ReadFile(fixturePath("input_methods_golden.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fx inputMethodFixture
	if err := json.Unmarshal(data, &fx); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	for _, c := range fx.Cases {
		if got := DecodeInputMethod(c.Input); got != c.Method {
			t.Fatalf("DecodeInputMethod(%s)=%s want %s", c.Input, got, c.Method)
		}
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func fixturePath(name string) string {
	return filepath.Join("..", "..", "testdata", "normalize", name)
}
