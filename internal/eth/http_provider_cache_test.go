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

func TestTimestampCacheUpdateExtendsEntry(t *testing.T) {
	cache := newTimestampCache(2, 50*time.Millisecond)
	now := time.Now()
	cache.add(5, 500, now)
	if v, ok := cache.get(5, now); !ok || v != 500 {
		t.Fatalf("expected initial value 500, got ok=%v value=%d", ok, v)
	}
	updated := now.Add(10 * time.Millisecond)
	cache.add(5, 600, updated)
	if v, ok := cache.get(5, updated); !ok || v != 600 {
		t.Fatalf("expected updated value 600, got ok=%v value=%d", ok, v)
	}
	future := updated.Add(40 * time.Millisecond)
	if v, ok := cache.get(5, future); !ok || v != 600 {
		t.Fatalf("expected entry to persist after refresh, got ok=%v value=%d", ok, v)
	}
}

func TestTimestampCacheDefaultCapacityAndTTL(t *testing.T) {
	cache := newTimestampCache(0, 0)
	now := time.Now()
	for i := 0; i < defaultBlockTimestampCacheSize+1; i++ {
		cache.add(uint64(i), int64(i), now)
	}
	if _, ok := cache.get(0, now); ok {
		t.Fatalf("expected oldest entry to be evicted when exceeding default capacity")
	}
	cache = newTimestampCache(-1, -1)
	cache.add(42, 4200, now)
	later := now.Add(defaultBlockTimestampTTL + time.Millisecond)
	if _, ok := cache.get(42, later); ok {
		t.Fatalf("expected entry to expire using default TTL")
	}
}

func TestTimestampCacheEvictsExpiredEntriesOnAdd(t *testing.T) {
	cache := newTimestampCache(4, 10*time.Millisecond)
	base := time.Now().Add(-time.Second)
	cache.add(1, 100, base)
	cache.add(2, 200, base)
	cache.add(3, 300, base)
	future := base.Add(50 * time.Millisecond)
	cache.add(4, 400, future)
	if _, ok := cache.get(1, future); ok {
		t.Fatalf("expected expired entry 1 to be removed during eviction")
	}
	if _, ok := cache.get(2, future); ok {
		t.Fatalf("expected expired entry 2 to be removed during eviction")
	}
}

func TestTimestampCacheEvictHandlesEmptyList(t *testing.T) {
	cache := newTimestampCache(1, time.Second)
	cache.evict(time.Now())
}
