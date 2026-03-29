package metrics

import (
	"testing"
	"time"
)

// TestCollectorBasic verifies that the collector produces samples over time,
// that timestamps are monotonically increasing, and that Current matches the
// last entry returned by History.
func TestCollectorBasic(t *testing.T) {
	c := New(10, 100*time.Millisecond)
	c.Start()
	time.Sleep(350 * time.Millisecond)
	c.Stop()

	history := c.History()
	if len(history) < 3 {
		t.Fatalf("expected at least 3 samples, got %d", len(history))
	}

	for i := 1; i < len(history); i++ {
		if history[i].Timestamp < history[i-1].Timestamp {
			t.Errorf("timestamp regression at index %d: %d < %d",
				i, history[i].Timestamp, history[i-1].Timestamp)
		}
	}

	current := c.Current()
	last := history[len(history)-1]
	if current.Timestamp != last.Timestamp {
		t.Errorf("Current().Timestamp=%d, want %d", current.Timestamp, last.Timestamp)
	}
}

// TestCollectorRingBufferWrap verifies that once the ring buffer is full, it
// wraps and always returns exactly maxSize samples in oldest-first order.
func TestCollectorRingBufferWrap(t *testing.T) {
	const maxSize = 5
	c := New(maxSize, 50*time.Millisecond)
	c.Start()
	time.Sleep(400 * time.Millisecond) // ~8 samples collected
	c.Stop()

	history := c.History()
	if len(history) != maxSize {
		t.Fatalf("expected exactly %d samples after wrap, got %d", maxSize, len(history))
	}

	// Samples must be in chronological (oldest-first) order.
	for i := 1; i < len(history); i++ {
		if history[i].Timestamp < history[i-1].Timestamp {
			t.Errorf("samples not in order at index %d: %d < %d",
				i, history[i].Timestamp, history[i-1].Timestamp)
		}
	}
}

// TestCollectorEmpty verifies the zero state: History returns an empty slice
// and Current returns a zero-value Sample when no data has been collected.
func TestCollectorEmpty(t *testing.T) {
	c := New(10, 1*time.Hour) // interval so long it will never tick in tests

	history := c.History()
	if len(history) != 0 {
		t.Fatalf("expected empty history, got %d samples", len(history))
	}

	current := c.Current()
	var zero Sample
	if current != zero {
		t.Errorf("expected zero Sample from Current(), got %+v", current)
	}
}
