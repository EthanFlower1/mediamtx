package nonce

import (
	"crypto/rand"
	"encoding/binary"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock is a manually-advanced Clock for deterministic rotation tests.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(t time.Time) *fakeClock { return &fakeClock{now: t} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func mkNonce(i uint64) []byte {
	b := make([]byte, 16)
	binary.BigEndian.PutUint64(b, i)
	binary.BigEndian.PutUint64(b[8:], ^i)
	return b
}

func TestNew_Sizing(t *testing.T) {
	f := New(1_000_000, 0.001, 5*time.Minute, WithoutBackgroundRotation())
	defer f.Close()
	s := f.Stats()
	// m ≈ 14_377_587 bits, k = 10
	if s.BitsPerWindow < 14_000_000 || s.BitsPerWindow > 15_000_000 {
		t.Errorf("expected ~14.4M bits, got %d", s.BitsPerWindow)
	}
	if s.HashFunctions != 10 {
		t.Errorf("expected 10 hash functions, got %d", s.HashFunctions)
	}
}

func TestCheckAndAdd_BasicInsertionLookup(t *testing.T) {
	f := New(10_000, 0.001, time.Minute, WithoutBackgroundRotation())
	defer f.Close()

	n := mkNonce(42)
	if !f.CheckAndAdd(n) {
		t.Fatal("first CheckAndAdd should report wasNew=true")
	}
	if f.CheckAndAdd(n) {
		t.Fatal("second CheckAndAdd should report wasNew=false (replay)")
	}
	if !f.Check(n) {
		t.Fatal("Check should report seen=true after Add")
	}
}

func TestDistinctNonces_AllUnique(t *testing.T) {
	f := New(50_000, 0.001, time.Minute, WithoutBackgroundRotation())
	defer f.Close()

	const N = 10_000
	for i := uint64(0); i < N; i++ {
		if !f.CheckAndAdd(mkNonce(i)) {
			t.Fatalf("nonce %d falsely reported as replayed", i)
		}
	}
}

func TestRotation_OldNoncesExpire(t *testing.T) {
	clk := newFakeClock(time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC))
	ttl := 2 * time.Minute
	f := New(10_000, 0.001, ttl, WithClock(clk), WithoutBackgroundRotation())
	defer f.Close()

	old := mkNonce(1)
	if !f.CheckAndAdd(old) {
		t.Fatal("first insert should be new")
	}
	// Still within active window: must be detected as replay.
	if f.CheckAndAdd(old) {
		t.Fatal("immediate replay should be detected")
	}

	// Rotate once: active becomes previous; old is still queryable via previous.
	clk.Advance(ttl / 2)
	f.Rotate()
	if f.CheckAndAdd(old) {
		t.Fatal("after one rotation old nonce should still be detected (sliding window)")
	}

	// Rotate again: previous (which held `old`) is discarded.
	clk.Advance(ttl / 2)
	f.Rotate()
	// Insert a different nonce so the active window isn't empty.
	fresh := mkNonce(99)
	if !f.CheckAndAdd(fresh) {
		t.Fatal("fresh nonce should be new")
	}
	// Now `old` should have aged out.
	if !f.CheckAndAdd(old) {
		t.Fatal("after two rotations old nonce should have expired")
	}
}

func TestFalsePositiveRate_Loose(t *testing.T) {
	// Insert 100K nonces; query 100K different nonces; assert FPR < 1%
	// (loose vs the configured 0.1% — under-loaded filter should easily beat).
	f := New(100_000, 0.001, time.Minute, WithoutBackgroundRotation())
	defer f.Close()

	const inserted = 100_000
	for i := uint64(0); i < inserted; i++ {
		f.CheckAndAdd(mkNonce(i))
	}

	const probes = 100_000
	falsePositives := 0
	for i := uint64(inserted); i < inserted+probes; i++ {
		// Use Check (not CheckAndAdd): we don't want probes to pollute the
		// filter and inflate the measured FPR.
		if f.Check(mkNonce(i)) {
			falsePositives++
		}
	}
	rate := float64(falsePositives) / float64(probes)
	t.Logf("false positives: %d / %d = %.4f%% (target 0.1%%)", falsePositives, probes, rate*100)
	// Loose bound at 1% per the acceptance criteria; tight 0.1% is the design
	// target and is comfortably hit at full capacity in practice.
	if rate >= 0.01 {
		t.Errorf("false positive rate %.4f exceeds 1%% loose bound", rate)
	}
}

func TestConcurrent_CheckAndAdd_RaceSafe(t *testing.T) {
	f := New(100_000, 0.001, time.Minute, WithoutBackgroundRotation())
	defer f.Close()

	const workers = 16
	const perWorker = 5_000

	var wg sync.WaitGroup
	var newCount int64

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				// Fully disjoint nonce space across workers => every insert
				// must report wasNew=true.
				n := mkNonce(uint64(w)*1_000_000 + uint64(i))
				if f.CheckAndAdd(n) {
					atomic.AddInt64(&newCount, 1)
				}
			}
		}(w)
	}
	wg.Wait()
	expected := int64(workers * perWorker)
	// Allow up to 0.5% bloom-filter false-positive collisions across the
	// total disjoint workload — a few "wasNew=false" results are expected
	// at the configured 0.1% FPR. A real race bug would manifest as data
	// corruption under -race long before this slack matters.
	maxFP := expected / 200
	if newCount < expected-maxFP || newCount > expected {
		t.Errorf("expected ~%d new inserts (allow %d FP slack), got %d",
			expected, maxFP, newCount)
	}
}

func TestConcurrent_DuplicateRejection(t *testing.T) {
	// Two goroutines racing on the SAME nonce: exactly one must win.
	f := New(1_000, 0.001, time.Minute, WithoutBackgroundRotation())
	defer f.Close()

	const trials = 500
	for i := 0; i < trials; i++ {
		n := mkNonce(uint64(i))
		var wins int64
		var wg sync.WaitGroup
		for r := 0; r < 4; r++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if f.CheckAndAdd(n) {
					atomic.AddInt64(&wins, 1)
				}
			}()
		}
		wg.Wait()
		if wins != 1 {
			t.Fatalf("trial %d: expected exactly 1 winner, got %d", i, wins)
		}
	}
}

func TestNilFilter_FailClosed(t *testing.T) {
	var f *Filter
	if !f.Check([]byte("anything")) {
		t.Error("nil filter Check must return true (fail-closed)")
	}
	if f.CheckAndAdd([]byte("anything")) {
		t.Error("nil filter CheckAndAdd must return wasNew=false (fail-closed)")
	}
	f.Add([]byte("anything")) // must not panic
}

func TestClosed_FailClosed(t *testing.T) {
	f := New(1_000, 0.001, time.Minute, WithoutBackgroundRotation())
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !f.Check([]byte("x")) {
		t.Error("closed filter Check must return true")
	}
	if f.CheckAndAdd([]byte("x")) {
		t.Error("closed filter CheckAndAdd must return wasNew=false")
	}
	// Idempotent Close.
	if err := f.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestBackgroundRotation_Runs(t *testing.T) {
	// Smoke test: with a real clock and short TTL the goroutine should
	// rotate without deadlock or panic, and Close must shut it down.
	f := New(1_000, 0.001, 40*time.Millisecond)
	// Add some nonces.
	for i := uint64(0); i < 100; i++ {
		f.CheckAndAdd(mkNonce(i))
	}
	time.Sleep(120 * time.Millisecond) // ~6 rotation intervals
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestCheckAndAddString(t *testing.T) {
	f := New(1_000, 0.001, time.Minute, WithoutBackgroundRotation())
	defer f.Close()
	if !f.CheckAndAddString("nonce-abc") {
		t.Fatal("first should be new")
	}
	if f.CheckAndAddString("nonce-abc") {
		t.Fatal("second should be replay")
	}
}

// Random nonce sanity: ensures hashing on truly random input behaves.
func TestRandomNonces_NoDuplicatesReported(t *testing.T) {
	f := New(50_000, 0.001, time.Minute, WithoutBackgroundRotation())
	defer f.Close()

	const N = 5_000
	nonces := make([][]byte, N)
	for i := range nonces {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			t.Fatal(err)
		}
		nonces[i] = b
	}
	dups := 0
	for i, n := range nonces {
		if !f.CheckAndAdd(n) {
			dups++
			if dups < 5 {
				t.Logf("dup at %d: %x", i, n)
			}
		}
	}
	// With 5K random 256-bit nonces in a 50K-capacity filter, FPR << 0.1%
	// so we expect zero collisions. Allow up to 5 to avoid flaky CI.
	if dups > 5 {
		t.Errorf("too many false-positive collisions: %d", dups)
	}
}

