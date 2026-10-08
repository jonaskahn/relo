package google

import (
	"fmt"
	"sync"
	"testing"
)

// TestReplayCacheEvictsOldest covers the bounded signature memory: the newest
// entries survive and the oldest leave once the bound is passed.
func TestReplayCacheEvictsOldest(t *testing.T) {
	cache := newReplayCache(3)
	for i := range 5 {
		cache.store(fmt.Sprintf("key-%d", i), fmt.Sprintf("sig-%d", i), "session")
	}
	if got := cache.lookup("key-0"); got != "" {
		t.Fatalf("lookup(key-0) = %q, want it evicted", got)
	}
	if got := cache.lookup("key-1"); got != "" {
		t.Fatalf("lookup(key-1) = %q, want it evicted", got)
	}
	if got := cache.lookup("key-4"); got != "sig-4" {
		t.Fatalf("lookup(key-4) = %q, want sig-4", got)
	}
}

// TestReplayCacheConcurrentStore covers concurrent signature writes with the
// race detector, so the lock discipline is proven, not assumed.
func TestReplayCacheConcurrentStore(t *testing.T) {
	cache := newReplayCache(64)
	var wg sync.WaitGroup
	for w := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 50 {
				key := fmt.Sprintf("worker-%d-key-%d", w, i)
				cache.store(key, "sig", "session")
				_ = cache.lookup(key)
			}
		}()
	}
	wg.Wait()
}
