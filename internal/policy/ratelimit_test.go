package policy

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(map[string]int{"svc": 3})

	for i := 0; i < 3; i++ {
		if !rl.Allow("svc") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	if rl.Allow("svc") {
		t.Fatal("4th request should be denied")
	}
}

func TestRateLimiterNoLimit(t *testing.T) {
	rl := NewRateLimiter(map[string]int{})

	for i := 0; i < 100; i++ {
		if !rl.Allow("any") {
			t.Fatal("should always allow when no limit configured")
		}
	}
}

func TestRateLimiterRingBufferWrapAround(t *testing.T) {
	rl := NewRateLimiter(map[string]int{"svc": 3})

	// Fill the window
	for i := 0; i < 3; i++ {
		if !rl.Allow("svc") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if rl.Allow("svc") {
		t.Fatal("4th request should be denied")
	}

	// Manually expire all entries by advancing the window timestamps
	rl.mu.Lock()
	w := rl.windows["svc"]
	past := time.Now().Add(-2 * time.Minute)
	for i := 0; i < len(w.buf); i++ {
		w.buf[i] = past
	}
	rl.mu.Unlock()

	// After expiry, new requests should be allowed — tests ring buffer wrap
	for i := 0; i < 3; i++ {
		if !rl.Allow("svc") {
			t.Fatalf("post-expiry request %d should be allowed", i+1)
		}
	}
	if rl.Allow("svc") {
		t.Fatal("should be denied again after refill")
	}
}

func TestRateLimiterMemoryBounded(t *testing.T) {
	limit := 5
	rl := NewRateLimiter(map[string]int{"svc": limit})

	// Fill, expire, refill many times — buffer size stays constant
	for cycle := 0; cycle < 10; cycle++ {
		for i := 0; i < limit; i++ {
			rl.Allow("svc")
		}
		// Expire
		rl.mu.Lock()
		w := rl.windows["svc"]
		past := time.Now().Add(-2 * time.Minute)
		for i := 0; i < len(w.buf); i++ {
			w.buf[i] = past
		}
		w.count = limit
		rl.mu.Unlock()
	}

	rl.mu.Lock()
	w := rl.windows["svc"]
	if len(w.buf) != limit {
		t.Errorf("buffer capacity should stay at %d, got %d", limit, len(w.buf))
	}
	rl.mu.Unlock()
}
