package normalize

// Covers addrFromTopic fallback on non-hex input.

import "testing"

func TestAddrFromTopicFallback(t *testing.T) {
    got := addrFromTopic([]string{"not-a-hex-address"}, 0)
    if got != "not-a-hex-address" {
        t.Fatalf("fallback got %s", got)
    }
}
