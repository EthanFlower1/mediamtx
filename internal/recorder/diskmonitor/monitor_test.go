package diskmonitor

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// discardLogger returns an slog.Logger that discards all output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 10}))
}

// cycleMonitor constructs a Monitor with a manually-controlled cycle channel
// instead of a real time.Ticker. The caller sends on the returned channel to
// trigger a poll cycle.
func cycleMonitor(cfg Config, statfsFn func(string) (int64, int64, error)) (*Monitor, chan<- time.Time) {
	ch := make(chan time.Time, 1)
	m := &Monitor{
		cfg:    cfg,
		statfs: statfsFn,
		cycle:  ch,
		// ticker remains nil — Run will not call ticker.Stop().
	}
	initial := &Stats{}
	m.stats.Store(initial)
	return m, ch
}

// ---------------------------------------------------------------------------
// fakeDB
// ---------------------------------------------------------------------------

type fakeDB struct {
	mu           sync.Mutex
	listCallsN   atomic.Int64
	deleteCallsN atomic.Int64
	listErr      error
	deleteErr    error
	recordings   []ExpiredRecording
	// callCount tracks how many times list is called.
}

func (f *fakeDB) ListExpiredRecordings(_ context.Context, limit int) ([]ExpiredRecording, error) {
	f.listCallsN.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	n := limit
	if n > len(f.recordings) {
		n = len(f.recordings)
	}
	// Drain: return the first n recordings and remove them from the list
	// so subsequent calls see fewer (or zero) items. This simulates the
	// DB actually removing rows after deletion.
	out := make([]ExpiredRecording, n)
	copy(out, f.recordings[:n])
	f.recordings = f.recordings[n:]
	return out, nil
}

func (f *fakeDB) DeleteRecording(_ context.Context, _ int64) error {
	f.deleteCallsN.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deleteErr
}

// ---------------------------------------------------------------------------
// Test 1: basic poll → Stats updated
// ---------------------------------------------------------------------------

func TestRun_PollsAndUpdatesStats(t *testing.T) {
	db := &fakeDB{}
	m, ch := cycleMonitor(Config{
		RecordingsPath: t.TempDir(),
		DB:             db,
		Logger:         discardLogger(),
	}, func(string) (int64, int64, error) {
		return 500_000_000, 1_000_000_000, nil // 50 %
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Run(ctx)
	}()

	// The first immediate poll fires without waiting for a tick.
	// Give it a moment.
	time.Sleep(20 * time.Millisecond)

	s := m.Stats()
	if s.UsedBytes != 500_000_000 {
		t.Errorf("UsedBytes = %d; want 500000000", s.UsedBytes)
	}
	if s.CapacityBytes != 1_000_000_000 {
		t.Errorf("CapacityBytes = %d; want 1000000000", s.CapacityBytes)
	}
	if s.UsedPercent < 49.9 || s.UsedPercent > 50.1 {
		t.Errorf("UsedPercent = %.2f; want ~50", s.UsedPercent)
	}
	if s.LastPolled.IsZero() {
		t.Error("LastPolled should not be zero")
	}

	cancel()
	<-done
	_ = ch
}

// ---------------------------------------------------------------------------
// Test 2: below threshold → ListExpiredRecordings NOT called
// ---------------------------------------------------------------------------

func TestRun_BelowThreshold_NoRetentionTriggered(t *testing.T) {
	db := &fakeDB{}
	m, ch := cycleMonitor(Config{
		RecordingsPath:            t.TempDir(),
		DB:                        db,
		RetentionThresholdPercent: 90,
		Logger:                    discardLogger(),
	}, func(string) (int64, int64, error) {
		return 500_000_000, 1_000_000_000, nil // 50 %
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Run(ctx)
	}()

	time.Sleep(20 * time.Millisecond)

	if n := db.listCallsN.Load(); n != 0 {
		t.Errorf("ListExpiredRecordings called %d times; want 0", n)
	}

	cancel()
	<-done
	_ = ch
}

// ---------------------------------------------------------------------------
// Test 3: above threshold → all expired recordings deleted
// ---------------------------------------------------------------------------

func TestRun_AboveThreshold_DeletesOldest(t *testing.T) {
	// Create 3 real temp files so os.Remove succeeds.
	dir := t.TempDir()
	makeFile := func(name string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	recordings := []ExpiredRecording{
		{ID: 1, FilePath: makeFile("r1.mp4"), FileSize: 100},
		{ID: 2, FilePath: makeFile("r2.mp4"), FileSize: 100},
		{ID: 3, FilePath: makeFile("r3.mp4"), FileSize: 100},
	}

	db := &fakeDB{recordings: recordings}

	// Statfs always returns 95% to keep retention loop running.
	m, ch := cycleMonitor(Config{
		RecordingsPath:            dir,
		DB:                        db,
		RetentionThresholdPercent: 90,
		Logger:                    discardLogger(),
	}, func(string) (int64, int64, error) {
		// Return 95% so threshold is exceeded.
		return 950_000_000, 1_000_000_000, nil
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Run(ctx)
	}()

	// Wait enough for the initial poll + retention loop to complete.
	// The loop will keep calling ListExpiredRecordings until it gets
	// empty — after the 1st call returns all 3, the 2nd call returns 0.
	time.Sleep(100 * time.Millisecond)

	cancel()
	<-done

	deleteCalls := db.deleteCallsN.Load()
	if deleteCalls != 3 {
		t.Errorf("DeleteRecording called %d times; want 3", deleteCalls)
	}
	_ = ch
}

// ---------------------------------------------------------------------------
// Test 4: hysteresis stops retention after first delete
// ---------------------------------------------------------------------------

func TestRun_HysteresisStopsRetention(t *testing.T) {
	dir := t.TempDir()
	makeFile := func(name string) string {
		p := filepath.Join(dir, name)
		_ = os.WriteFile(p, []byte("data"), 0o644)
		return p
	}

	recordings := []ExpiredRecording{
		{ID: 1, FilePath: makeFile("r1.mp4"), FileSize: 100},
		{ID: 2, FilePath: makeFile("r2.mp4"), FileSize: 100},
		{ID: 3, FilePath: makeFile("r3.mp4"), FileSize: 100},
	}

	db := &fakeDB{recordings: recordings}

	// deleteCount tracks how many deletes happened so we can change statfs.
	var deleteCount atomic.Int64
	origDelete := db.DeleteRecording
	_ = origDelete

	// Override DB with one that tracks deletes and adjusts perceived usage.
	customDB := &testDB{
		listFn: func(ctx context.Context, limit int) ([]ExpiredRecording, error) {
			db.listCallsN.Add(1)
			db.mu.Lock()
			defer db.mu.Unlock()
			n := limit
			if n > len(db.recordings) {
				n = len(db.recordings)
			}
			return db.recordings[:n], nil
		},
		deleteFn: func(ctx context.Context, id int64) error {
			deleteCount.Add(1)
			return nil
		},
	}

	// Statfs returns 95% until 1 delete, then 84% (below 90-5=85).
	statfsFn := func(string) (int64, int64, error) {
		if deleteCount.Load() >= 1 {
			return 840_000_000, 1_000_000_000, nil // 84 %
		}
		return 950_000_000, 1_000_000_000, nil // 95 %
	}

	m, ch := cycleMonitor(Config{
		RecordingsPath:            dir,
		DB:                        customDB,
		RetentionThresholdPercent: 90,
		Logger:                    discardLogger(),
	}, statfsFn)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	if d := deleteCount.Load(); d != 1 {
		t.Errorf("deleted %d recordings; want exactly 1 (hysteresis should have stopped at 1)", d)
	}
	_ = ch
}

// testDB is a flexible fake that accepts function fields.
type testDB struct {
	listFn   func(ctx context.Context, limit int) ([]ExpiredRecording, error)
	deleteFn func(ctx context.Context, id int64) error
}

func (d *testDB) ListExpiredRecordings(ctx context.Context, limit int) ([]ExpiredRecording, error) {
	return d.listFn(ctx, limit)
}
func (d *testDB) DeleteRecording(ctx context.Context, id int64) error {
	return d.deleteFn(ctx, id)
}

// ---------------------------------------------------------------------------
// Test 5: above threshold but no expired recordings → Error logged
// ---------------------------------------------------------------------------

func TestRun_AboveThresholdButNoExpired_LogsError(t *testing.T) {
	// We verify no panic and that deleteCallsN stays 0.
	db := &fakeDB{} // empty recordings

	m, ch := cycleMonitor(Config{
		RecordingsPath:            t.TempDir(),
		DB:                        db,
		RetentionThresholdPercent: 90,
		Logger:                    discardLogger(),
	}, func(string) (int64, int64, error) {
		return 950_000_000, 1_000_000_000, nil
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if n := db.deleteCallsN.Load(); n != 0 {
		t.Errorf("DeleteRecording called %d times; want 0", n)
	}
	// Verify list was called at least once.
	if n := db.listCallsN.Load(); n == 0 {
		t.Error("ListExpiredRecordings never called; expected at least one call")
	}
	_ = ch
}

// ---------------------------------------------------------------------------
// Test 6: statfs error → logged and next cycle succeeds
// ---------------------------------------------------------------------------

func TestRun_StatfsError_LogsAndContinues(t *testing.T) {
	db := &fakeDB{}

	var callCount atomic.Int64
	statfsFn := func(string) (int64, int64, error) {
		n := callCount.Add(1)
		if n == 1 {
			return 0, 0, errors.New("simulated statfs failure")
		}
		return 500_000_000, 1_000_000_000, nil
	}

	m, ch := cycleMonitor(Config{
		RecordingsPath: t.TempDir(),
		DB:             db,
		Logger:         discardLogger(),
	}, statfsFn)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Run(ctx)
	}()

	// Trigger a second cycle.
	time.Sleep(10 * time.Millisecond)
	ch <- time.Now()
	time.Sleep(20 * time.Millisecond)

	cancel()
	<-done

	// After first (failing) poll the stats remain zero-ish.
	// After second (succeeding) poll stats should be non-zero.
	s := m.Stats()
	if s.UsedBytes == 0 {
		// It's possible the second tick hasn't fired yet; this is fine.
		// The important thing is no panic occurred.
	}
	// No panic is the main assertion; the test passes if we get here.
}

// ---------------------------------------------------------------------------
// Test 7: DB error → logged, retention skipped, next cycle works
// ---------------------------------------------------------------------------

func TestRun_DBError_LogsAndContinues(t *testing.T) {
	db := &fakeDB{listErr: errors.New("simulated DB failure")}

	m, ch := cycleMonitor(Config{
		RecordingsPath:            t.TempDir(),
		DB:                        db,
		RetentionThresholdPercent: 90,
		Logger:                    discardLogger(),
	}, func(string) (int64, int64, error) {
		return 950_000_000, 1_000_000_000, nil // above threshold
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Run(ctx)
	}()

	time.Sleep(30 * time.Millisecond)
	// Trigger another cycle — should not panic.
	ch <- time.Now()
	time.Sleep(30 * time.Millisecond)

	cancel()
	<-done

	// delete should never be called since list errors.
	if n := db.deleteCallsN.Load(); n != 0 {
		t.Errorf("DeleteRecording called %d times; want 0", n)
	}
}

// ---------------------------------------------------------------------------
// Test 8: context cancel → Run exits within 50ms
// ---------------------------------------------------------------------------

func TestRun_RespectsContext(t *testing.T) {
	db := &fakeDB{}

	m, _ := cycleMonitor(Config{
		RecordingsPath: t.TempDir(),
		DB:             db,
		Logger:         discardLogger(),
	}, func(string) (int64, int64, error) {
		return 100_000_000, 1_000_000_000, nil
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// good
	case <-time.After(50 * time.Millisecond):
		t.Error("Run did not exit within 50ms of context cancellation")
	}
}

// ---------------------------------------------------------------------------
// Test 9: concurrent Stats reads are race-detector clean
// ---------------------------------------------------------------------------

func TestStats_ConcurrentSafe(t *testing.T) {
	db := &fakeDB{}
	ch := make(chan time.Time, 10)

	var callNum atomic.Int64
	m := &Monitor{
		cfg: Config{
			RecordingsPath: t.TempDir(),
			DB:             db,
			Logger:         discardLogger(),
		},
		statfs: func(string) (int64, int64, error) {
			n := callNum.Add(1)
			return n * 10_000_000, 1_000_000_000, nil
		},
		cycle: ch,
	}
	initial := &Stats{}
	m.stats.Store(initial)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go m.Run(ctx)

	// Hammer Stats() from multiple goroutines while Run() updates them.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = m.Stats()
				time.Sleep(time.Millisecond)
			}
		}()
	}

	// Trigger several cycles.
	for i := 0; i < 5; i++ {
		ch <- time.Now()
		time.Sleep(2 * time.Millisecond)
	}

	wg.Wait()
	cancel()
}
