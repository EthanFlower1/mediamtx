// Package nonce — sliding-window bloom filter implementation.
//
// SECURITY: review required by lead-security before any merge to main.
package nonce

import (
	"hash/maphash"
	"math"
	"sync"
	"time"
)

// Clock abstracts time for tests.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// window is a single bloom filter bit array with its creation time.
type window struct {
	bits      []uint64
	mBits     uint64 // size in bits (== len(bits)*64)
	createdAt time.Time
}

func newWindow(mBits uint64, now time.Time) *window {
	words := (mBits + 63) / 64
	return &window{
		bits:      make([]uint64, words),
		mBits:     mBits,
		createdAt: now,
	}
}

func (w *window) setBit(idx uint64) {
	i := idx % w.mBits
	w.bits[i>>6] |= 1 << (i & 63)
}

func (w *window) hasBit(idx uint64) bool {
	i := idx % w.mBits
	return w.bits[i>>6]&(1<<(i&63)) != 0
}

// Filter is a sliding-window bloom filter for single-use nonces.
//
// It is composed of two rotating bloom windows. Lookups query both windows
// (logical OR); inserts go into the active window. Every ttl/2 the windows
// rotate: the previous window is discarded and the active window slides
// down to take its place, with a fresh empty window becoming active. This
// guarantees that any nonce inserted is visible for at least ttl/2 and at
// most ttl before it is expired.
//
// Filter is safe for concurrent use. The zero value is not usable; use
// New.
type Filter struct {
	mu       sync.Mutex
	mBits    uint64
	k        int
	ttl      time.Duration
	clock    Clock
	seeds    []maphash.Seed
	previous *window // older window (may be nil)
	active   *window // current window for inserts
	closed   bool
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// Option configures a Filter.
type Option func(*filterOpts)

type filterOpts struct {
	clock    Clock
	noTicker bool // disable background rotation goroutine (tests)
}

// WithClock injects a custom clock (for tests).
func WithClock(c Clock) Option {
	return func(o *filterOpts) { o.clock = c }
}

// WithoutBackgroundRotation disables the rotation goroutine; the caller
// must invoke Rotate manually. Intended for deterministic tests.
func WithoutBackgroundRotation() Option {
	return func(o *filterOpts) { o.noTicker = true }
}

// New constructs a Filter sized for the given capacity and target false
// positive rate, with the given sliding-window TTL. Capacity must be > 0,
// fpRate must be in (0, 1), and ttl must be > 0.
//
// For capacity=1_000_000, fpRate=0.001, ttl=5*time.Minute the filter uses
// approximately 3.6 MB of memory and 10 hash functions per nonce.
func New(capacity int, fpRate float64, ttl time.Duration, opts ...Option) *Filter {
	if capacity <= 0 {
		capacity = 1
	}
	if fpRate <= 0 || fpRate >= 1 {
		fpRate = 0.001
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}

	o := filterOpts{clock: realClock{}}
	for _, opt := range opts {
		opt(&o)
	}

	// m = -n * ln(p) / (ln(2)^2)
	mBits := uint64(math.Ceil(-float64(capacity) * math.Log(fpRate) / (math.Ln2 * math.Ln2)))
	if mBits < 64 {
		mBits = 64
	}
	// k = (m/n) * ln(2)
	k := int(math.Round(float64(mBits) / float64(capacity) * math.Ln2))
	if k < 1 {
		k = 1
	}
	if k > 32 {
		k = 32
	}

	seeds := make([]maphash.Seed, k)
	for i := range seeds {
		seeds[i] = maphash.MakeSeed()
	}

	now := o.clock.Now()
	f := &Filter{
		mBits:  mBits,
		k:      k,
		ttl:    ttl,
		clock:  o.clock,
		seeds:  seeds,
		active: newWindow(mBits, now),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}

	if !o.noTicker {
		go f.rotateLoop()
	} else {
		close(f.doneCh)
	}
	return f
}

// Close stops the background rotation goroutine. After Close, the Filter
// is fail-closed: every CheckAndAdd returns wasNew=false and every Check
// returns true (already-seen).
func (f *Filter) Close() error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil
	}
	f.closed = true
	f.mu.Unlock()
	close(f.stopCh)
	<-f.doneCh
	return nil
}

func (f *Filter) rotateLoop() {
	defer close(f.doneCh)
	interval := f.ttl / 2
	if interval <= 0 {
		interval = time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-f.stopCh:
			return
		case <-t.C:
			f.Rotate()
		}
	}
}

// Rotate forces a rotation: the previous window is discarded, the active
// window becomes previous, and a fresh empty window becomes active. Tests
// call this directly when WithoutBackgroundRotation is set.
func (f *Filter) Rotate() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return
	}
	f.previous = f.active
	f.active = newWindow(f.mBits, f.clock.Now())
}

// hashIndices computes k bit indices for nonce, using maphash with k
// distinct seeds. Each seed yields one 64-bit hash; we then derive the
// final index modulo m inside set/has.
func (f *Filter) hashIndices(nonce []byte, out []uint64) {
	var h maphash.Hash
	for i, s := range f.seeds {
		h.SetSeed(s)
		h.Reset()
		_, _ = h.Write(nonce)
		out[i] = h.Sum64()
	}
}

// Check returns true if nonce is *possibly* in the filter (i.e. has been
// seen recently). False means definitely not seen. Check does not insert.
//
// A nil or closed Filter returns true (fail-closed: treat as already-seen,
// so replay-protected paths reject the request).
func (f *Filter) Check(nonce []byte) (seen bool) {
	if f == nil {
		return true
	}
	idx := make([]uint64, f.k)
	f.hashIndices(nonce, idx)

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return true
	}
	return f.checkLocked(idx)
}

func (f *Filter) checkLocked(idx []uint64) bool {
	// Check active window first.
	allActive := true
	for _, h := range idx {
		if !f.active.hasBit(h) {
			allActive = false
			break
		}
	}
	if allActive {
		return true
	}
	if f.previous == nil {
		return false
	}
	for _, h := range idx {
		if !f.previous.hasBit(h) {
			return false
		}
	}
	return true
}

// Add records nonce in the active window. Subsequent Check or
// CheckAndAdd calls within the TTL will report it as seen. A nil or
// closed Filter is a no-op.
func (f *Filter) Add(nonce []byte) {
	if f == nil {
		return
	}
	idx := make([]uint64, f.k)
	f.hashIndices(nonce, idx)

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return
	}
	for _, h := range idx {
		f.active.setBit(h)
	}
}

// CheckAndAdd atomically checks whether nonce has been seen and, if not,
// records it. It returns wasNew=true iff the nonce was not previously
// present (subject to the configured false-positive rate).
//
// A nil or closed Filter returns wasNew=false (fail-closed).
func (f *Filter) CheckAndAdd(nonce []byte) (wasNew bool) {
	if f == nil {
		return false
	}
	idx := make([]uint64, f.k)
	f.hashIndices(nonce, idx)

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return false
	}
	if f.checkLocked(idx) {
		return false
	}
	for _, h := range idx {
		f.active.setBit(h)
	}
	return true
}

// CheckAndAddString is a convenience wrapper for string-typed nonces.
func (f *Filter) CheckAndAddString(nonce string) (wasNew bool) {
	// Avoid allocation: hash the underlying bytes via a small adapter.
	// We can't safely cast string->[]byte without an unsafe import that
	// the security review would flag, so accept the copy.
	return f.CheckAndAdd([]byte(nonce))
}

// Stats returns runtime sizing for observability.
type Stats struct {
	BitsPerWindow uint64
	HashFunctions int
	TTL           time.Duration
}

// Stats returns the filter's static sizing parameters.
func (f *Filter) Stats() Stats {
	return Stats{
		BitsPerWindow: f.mBits,
		HashFunctions: f.k,
		TTL:           f.ttl,
	}
}

