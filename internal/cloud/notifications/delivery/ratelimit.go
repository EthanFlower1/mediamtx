package delivery

import (
	"sync"
	"time"
)

// RateLimiter enforces per-tenant, per-channel sliding window rate limits.
// It uses an in-memory counter with configurable window and max count.
// Production deployments should back this with Redis (KAI-217) for
// multi-instance consistency; this implementation is correct for
// single-instance and testing.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	timestamps []time.Time
	window     time.Duration
	maxCount   int
}

// NewRateLimiter creates a new in-memory rate limiter.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*bucket),
	}
}

// Allow checks whether a delivery is allowed under the rate limit.
// key is typically "tenantID:channelType". If no limit is configured
// for this key, it always allows.
func (r *RateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	b, ok := r.buckets[key]
	if !ok {
		return true
	}

	now := time.Now()
	cutoff := now.Add(-b.window)

	// Trim expired entries.
	valid := b.timestamps[:0]
	for _, t := range b.timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	b.timestamps = valid

	if len(b.timestamps) >= b.maxCount {
		return false
	}

	b.timestamps = append(b.timestamps, now)
	return true
}

// Configure sets the rate limit for a key. This replaces any existing
// configuration but preserves the existing sliding window state.
func (r *RateLimiter) Configure(key string, windowSeconds, maxCount int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	b, ok := r.buckets[key]
	if !ok {
		b = &bucket{}
		r.buckets[key] = b
	}
	b.window = time.Duration(windowSeconds) * time.Second
	b.maxCount = maxCount
}

// Reset clears the sliding window for a key. Useful in tests.
func (r *RateLimiter) Reset(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.buckets, key)
}
