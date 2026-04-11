package anomaly

import (
	"math"
	"testing"
	"time"
)

func TestRunningStats(t *testing.T) {
	s := &runningStats{}

	// Add known values: 2, 4, 4, 4, 5, 5, 7, 9
	values := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	for _, v := range values {
		s.Update(v)
	}

	if s.count != 8 {
		t.Fatalf("expected count=8, got %d", s.count)
	}

	expectedMean := 5.0
	if math.Abs(s.Mean()-expectedMean) > 0.001 {
		t.Errorf("expected mean=%.3f, got %.3f", expectedMean, s.Mean())
	}

	// Population stddev of {2,4,4,4,5,5,7,9} = 2.0
	expectedStdDev := 2.0
	if math.Abs(s.StdDev()-expectedStdDev) > 0.01 {
		t.Errorf("expected stddev=%.3f, got %.3f", expectedStdDev, s.StdDev())
	}
}

func TestRunningStatsEmpty(t *testing.T) {
	s := &runningStats{}
	if s.Mean() != 0 {
		t.Errorf("empty mean should be 0")
	}
	if s.StdDev() != 0 {
		t.Errorf("empty stddev should be 0")
	}
}

func TestRunningStatsSingleValue(t *testing.T) {
	s := &runningStats{}
	s.Update(42)
	if s.Mean() != 42 {
		t.Errorf("single value mean should be 42, got %f", s.Mean())
	}
	// StdDev with count < 2 returns 0
	if s.StdDev() != 0 {
		t.Errorf("single value stddev should be 0, got %f", s.StdDev())
	}
}

func TestBaselineObserveAndScore(t *testing.T) {
	b := NewBaseline("cam1")

	// Simulate a stable baseline: 10 observations at hour 14 with count ~5.
	baseTime := time.Date(2026, 4, 1, 14, 0, 0, 0, time.UTC)
	for i := 0; i < 20; i++ {
		ts := baseTime.Add(time.Duration(i) * time.Minute)
		b.Observe(ts, 5, map[string]int{"person": 3, "car": 2})
	}

	// Verify baseline stats.
	if b.HourSampleCount(14) != 20 {
		t.Fatalf("expected 20 samples at hour 14, got %d", b.HourSampleCount(14))
	}
	if math.Abs(b.HourMean(14)-5.0) > 0.001 {
		t.Errorf("expected mean=5.0 at hour 14, got %.3f", b.HourMean(14))
	}

	// Score a normal observation.
	score, _ := b.Score(baseTime, 5, map[string]int{"person": 3, "car": 2})
	if score > 0.1 {
		t.Errorf("normal observation should have low score, got %.3f", score)
	}

	// Score a clear anomaly: 50 objects instead of ~5.
	score, details := b.Score(baseTime, 50, map[string]int{"person": 30, "car": 20})
	if score < 0.5 {
		t.Errorf("anomalous observation should have high score, got %.3f", score)
	}
	if details == nil {
		t.Error("expected non-nil details for anomaly")
	}
}

func TestBaselineScoreInsufficientData(t *testing.T) {
	b := NewBaseline("cam1")

	ts := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	// Only 2 samples (need >=3).
	b.Observe(ts, 5, nil)
	b.Observe(ts.Add(time.Minute), 5, nil)

	score, _ := b.Score(ts, 100, nil)
	if score != 0 {
		t.Errorf("insufficient data should return score=0, got %.3f", score)
	}
}

func TestBaselineIsLearning(t *testing.T) {
	b := NewBaseline("cam1")

	if !b.IsLearning(7) {
		t.Error("new baseline should be in learning phase")
	}
	if b.IsLearning(0) {
		t.Error("learningDays=0 should never be learning")
	}
}

func TestBaselineSnapshot(t *testing.T) {
	b := NewBaseline("cam1")

	ts := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		b.Observe(ts.Add(time.Duration(i)*time.Minute), 3, map[string]int{"person": 2, "car": 1})
	}

	snap := b.Snapshot()
	if snap.CameraID != "cam1" {
		t.Errorf("expected cameraID=cam1, got %s", snap.CameraID)
	}
	if snap.SampleCount != 10 {
		t.Errorf("expected sampleCount=10, got %d", snap.SampleCount)
	}
	if snap.Hours[8].Count != 10 {
		t.Errorf("expected hour 8 count=10, got %d", snap.Hours[8].Count)
	}
	if !snap.Beta {
		t.Error("snapshot should have beta=true")
	}
}

func TestSensitivityToThreshold(t *testing.T) {
	tests := []struct {
		sensitivity float64
		wantThresh  float64
	}{
		{0.0, 0.95},
		{0.5, 0.55},
		{1.0, 0.15},
		{-0.1, 0.95}, // clamped
		{1.5, 0.15},  // clamped
	}

	for _, tt := range tests {
		got := SensitivityToThreshold(tt.sensitivity)
		if math.Abs(got-tt.wantThresh) > 0.001 {
			t.Errorf("SensitivityToThreshold(%.1f) = %.3f, want %.3f",
				tt.sensitivity, got, tt.wantThresh)
		}
	}
}

func TestBaselineScoreZeroStdDev(t *testing.T) {
	b := NewBaseline("cam1")

	// All identical values => stddev ~ 0.
	ts := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		b.Observe(ts.Add(time.Duration(i)*time.Minute), 5, nil)
	}

	// Same as baseline => low score.
	score, _ := b.Score(ts, 5, nil)
	if score > 0.01 {
		t.Errorf("identical observation with flat baseline should score ~0, got %.3f", score)
	}

	// Different from flat baseline => notable.
	score, _ = b.Score(ts, 20, nil)
	if score < 0.5 {
		t.Errorf("deviation from flat baseline should score high, got %.3f", score)
	}
}

func TestBaselineDifferentHours(t *testing.T) {
	b := NewBaseline("cam1")

	// Hour 8 (morning): ~10 objects.
	// Hour 22 (night): ~1 object.
	morning := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)
	night := time.Date(2026, 4, 1, 22, 0, 0, 0, time.UTC)

	for i := 0; i < 20; i++ {
		b.Observe(morning.Add(time.Duration(i)*time.Minute), 10, nil)
		b.Observe(night.Add(time.Duration(i)*time.Minute), 1, nil)
	}

	// 10 objects at morning is normal.
	score, _ := b.Score(morning, 10, nil)
	if score > 0.1 {
		t.Errorf("normal morning observation should score low, got %.3f", score)
	}

	// 10 objects at night is anomalous.
	score, _ = b.Score(night, 10, nil)
	if score < 0.5 {
		t.Errorf("10 objects at night (baseline=1) should score high, got %.3f", score)
	}
}
