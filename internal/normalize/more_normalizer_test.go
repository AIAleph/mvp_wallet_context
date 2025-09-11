package normalize

import (
    "reflect"
    "testing"

    "github.com/AIAleph/mvp_wallet_context/internal/eth"
)

func TestLogsToRows_TracesToRows_AsAny_SplitDataWords(t *testing.T) {
    logs := []eth.Log{{TxHash: "0x1", Index: 2, Address: "0xdead", Topics: []string{"0x"}, DataHex: "0x00", BlockNum: 7, TsMillis: 10}}
    lrows := LogsToRows(logs)
    if len(lrows) != 1 || lrows[0].EventUID != "0x1:2" { t.Fatalf("lrows=%+v", lrows) }
    traces := []eth.Trace{{TxHash: "0x2", TraceID: "a", From: "0x", To: "0x", ValueWei: "0x1", BlockNum: 8, TsMillis: 11}}
    trows := TracesToRows(traces)
    if len(trows) != 1 || trows[0].TraceUID != "0x2:a" { t.Fatalf("trows=%+v", trows) }
    anyRows := AsAny(lrows)
    if len(anyRows) != 1 || reflect.TypeOf(anyRows[0]).Name() == "" { t.Fatalf("asan=%T", anyRows[0]) }
    words := splitDataWords("0x" + pad32Hex(1) + pad32Hex(2))
    if len(words) != 2 { t.Fatalf("words=%v", words) }
}

