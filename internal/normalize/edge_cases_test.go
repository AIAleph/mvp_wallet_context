package normalize

import "testing"

func TestParseERC1155Batch_InvalidOffsetsAndShortData(t *testing.T) {
    // Too short => nil slices
    ids, vals := parseERC1155Batch("0xdeadbeef")
    if ids != nil || vals != nil { t.Fatalf("expected nil slices on short data") }

    // Misaligned offset (1 byte) should yield nil from reader
    // Head: offIds=0x1, offVals=0x40 (aligned)
    head := "0000000000000000000000000000000000000000000000000000000000000001" +
            "0000000000000000000000000000000000000000000000000000000000000040"
    // Provide a minimal tail to avoid index panic
    data := "0x" + head + "0000000000000000000000000000000000000000000000000000000000000000"
    ids, vals = parseERC1155Batch(data)
    if ids != nil || vals == nil { t.Fatalf("expected ids=nil and vals possibly nil, got ids=%v vals=%v", ids, vals) }
}

func TestWordToInt_Empty(t *testing.T) {
    if got := wordToInt(""); got != 0 { t.Fatalf("empty wordToInt got %d", got) }
}

func TestAddrFromTopic_OutOfRange(t *testing.T) {
    if got := addrFromTopic([]string{"0x1"}, 3); got != "" { t.Fatalf("want empty, got %q", got) }
}

