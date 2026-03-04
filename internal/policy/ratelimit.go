package policy

import (
	"sync"
	"time"
)

// window is a fixed-capacity ring buffer of request timestamps.
type window struct {
	buf   []time.Time
	head  int // index of oldest entry
	count int // number of valid entries
}

func newWindow(capacity int) *window {
	return &window{buf: make([]time.Time, capacity)}
}

// evict removes entries older than cutoff.
func (w *window) evict(cutoff time.Time) {
	for w.count > 0 && w.buf[w.head].Before(cutoff) {
		w.head = (w.head + 1) % len(w.buf)
		w.count--
	}
}

// push appends a timestamp. Caller must ensure count < capacity.
func (w *window) push(t time.Time) {
	idx := (w.head + w.count) % len(w.buf)
	w.buf[idx] = t
	w.count++
}

type RateLimiter struct {
	mu      sync.Mutex
	limits  map[string]int
	windows map[string]*window
}

func NewRateLimiter(limits map[string]int) *RateLimiter {
	return &RateLimiter{
		limits:  limits,
		windows: make(map[string]*window),
	}
}

// Allow checks if a request to the given service is within rate limits.
func (rl *RateLimiter) Allow(service string) bool {
	limit, ok := rl.limits[service]
	if !ok || limit <= 0 {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	w, ok := rl.windows[service]
	if !ok {
		w = newWindow(limit)
		rl.windows[service] = w
	}

	w.evict(time.Now().Add(-time.Minute))

	if w.count >= limit {
		return false
	}

	w.push(time.Now())
	return true
}
