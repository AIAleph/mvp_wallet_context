package eth

import (
	"testing"
	"time"
)

func TestTimestampCacheEvictsAndExpires(t *testing.T) {
	cache := newTimestampCache(2, 10*time.Millisecond)
	now := time.Now()
	cache.add(1, 100, now)
	cache.add(2, 200, now)
	if v, ok := cache.get(1, now); !ok || v != 100 {
		t.Fatalf("expected cached value for block 1, got ok=%v value=%d", ok, v)
	}
	cache.add(3, 300, now)
	if _, ok := cache.get(2, now); ok {
		t.Fatalf("expected block 2 to be evicted when capacity exceeded")
	}
	later := now.Add(20 * time.Millisecond)
	if _, ok := cache.get(3, later); ok {
		t.Fatalf("expected block 3 to expire after ttl")
	}
}
