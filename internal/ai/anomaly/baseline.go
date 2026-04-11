package anomaly

import (
	"math"
	"sync"
	"time"
)

// runningStats maintains online mean and variance using Welford's algorithm.
// This avoids storing all samples and gives O(1) memory per hour bucket.
type runningStats struct {
	count int
	mean  float64
	m2    float64 // sum of squares of differences from the current mean
}

// Update adds a new sample using Welford's online algorithm.
func (s *runningStats) Update(x float64) {
	s.count++
	delta := x - s.mean
	s.mean += delta / float64(s.count)
	delta2 := x - s.mean
	s.m2 += delta * delta2
}

// Mean returns the running mean.
func (s *runningStats) Mean() float64 {
	if s.count == 0 {
		return 0
	}
	return s.mean
}

// StdDev returns the population standard deviation.
func (s *runningStats) StdDev() float64 {
	if s.count < 2 {
		return 0
	}
	return math.Sqrt(s.m2 / float64(s.count))
}

// Baseline maintains per-camera, per-hour activity statistics. It accumulates
// observations during a learning phase and then provides expected value
// ranges for anomaly scoring.
//
// Thread-safe: all methods are safe for concurrent use.
type Baseline struct {
	mu sync.RWMutex

	cameraID  string
	startedAt time.Time

	// Per-hour total-object-count statistics (24 buckets).
	hours [HoursPerDay]*runningStats

	// Per-class, per-hour statistics.
	classHours map[string][HoursPerDay]*runningStats

	// Total samples ingested.
	totalSamples int
}

// NewBaseline creates a new Baseline for the given camera.
func NewBaseline(cameraID string) *Baseline {
	b := &Baseline{
		cameraID:   cameraID,
		startedAt:  time.Now(),
		classHours: make(map[string][HoursPerDay]*runningStats),
	}
	for i := range b.hours {
		b.hours[i] = &runningStats{}
	}
	return b
}

// Observe records an observation: the total object count and per-class counts
// at the given timestamp. This is called on every detection frame.
func (b *Baseline) Observe(ts time.Time, totalCount int, classCounts map[string]int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	hour := ts.Hour()
	b.hours[hour].Update(float64(totalCount))
	b.totalSamples++

	for class, count := range classCounts {
		arr, ok := b.classHours[class]
		if !ok {
			for i := range arr {
				arr[i] = &runningStats{}
			}
			b.classHours[class] = arr
		}
		arr[hour].Update(float64(count))
	}
}

// IsLearning returns true if the baseline hasn't accumulated enough data.
// learningDays=0 means learning is always complete (useful for testing).
func (b *Baseline) IsLearning(learningDays int) bool {
	if learningDays <= 0 {
		return false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	elapsed := time.Since(b.startedAt)
	return elapsed < time.Duration(learningDays)*24*time.Hour
}

// DaysLearned returns the number of full days since the baseline started.
func (b *Baseline) DaysLearned() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return int(time.Since(b.startedAt).Hours() / 24)
}

// SampleCount returns the total number of observations recorded.
func (b *Baseline) SampleCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.totalSamples
}

// HourMean returns the mean object count for the given hour.
func (b *Baseline) HourMean(hour int) float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if hour < 0 || hour >= HoursPerDay {
		return 0
	}
	return b.hours[hour].Mean()
}

// HourStdDev returns the standard deviation of object counts for the given hour.
func (b *Baseline) HourStdDev(hour int) float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if hour < 0 || hour >= HoursPerDay {
		return 0
	}
	return b.hours[hour].StdDev()
}

// HourSampleCount returns the number of observations for the given hour.
func (b *Baseline) HourSampleCount(hour int) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if hour < 0 || hour >= HoursPerDay {
		return 0
	}
	return b.hours[hour].count
}

// Snapshot returns a serialisable snapshot of the baseline state.
func (b *Baseline) Snapshot() BaselineSnapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()

	snap := BaselineSnapshot{
		CameraID:    b.cameraID,
		StartedAt:   b.startedAt,
		SampleCount: b.totalSamples,
		Beta:        Beta,
	}
	for i, h := range b.hours {
		snap.Hours[i] = HourStats{
			Mean:   h.Mean(),
			StdDev: h.StdDev(),
			Count:  h.count,
		}
	}
	if len(b.classHours) > 0 {
		snap.ClassHours = make(map[string][HoursPerDay]HourStats)
		for class, arr := range b.classHours {
			var hourStats [HoursPerDay]HourStats
			for i, h := range arr {
				if h != nil {
					hourStats[i] = HourStats{
						Mean:   h.Mean(),
						StdDev: h.StdDev(),
						Count:  h.count,
					}
				}
			}
			snap.ClassHours[class] = hourStats
		}
	}
	return snap
}

// Score computes an anomaly score for the given observation at the given time.
// The score is in [0, 1] where 0 = perfectly normal and 1 = extreme anomaly.
//
// The score is based on the z-score (number of standard deviations from the
// mean) for the hour, mapped through a sigmoid to bound it in [0, 1].
//
// If insufficient data exists for the hour, score returns 0 (no anomaly).
func (b *Baseline) Score(ts time.Time, totalCount int, classCounts map[string]int) (score float64, details map[string]ClassAnomaly) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	hour := ts.Hour()
	h := b.hours[hour]

	// Need at least 3 samples to have meaningful statistics.
	if h.count < 3 {
		return 0, nil
	}

	mean := h.Mean()
	stddev := h.StdDev()

	// If stddev is near zero, any difference from mean is notable.
	var z float64
	if stddev < 0.001 {
		diff := math.Abs(float64(totalCount) - mean)
		if diff < 0.5 {
			z = 0
		} else {
			z = diff * 3 // synthetic high z for any deviation from flat baseline
		}
	} else {
		z = math.Abs(float64(totalCount)-mean) / stddev
	}

	// Map z-score to [0, 1] via sigmoid: score = 2/(1+exp(-z/2)) - 1
	// This gives 0 at z=0 and approaches 1 for large z.
	score = 2.0/(1.0+math.Exp(-z/2.0)) - 1.0

	// Per-class details.
	if len(classCounts) > 0 {
		details = make(map[string]ClassAnomaly, len(classCounts))
		for class, count := range classCounts {
			ca := ClassAnomaly{Observed: count}
			arr, ok := b.classHours[class]
			if ok && arr[hour] != nil && arr[hour].count >= 3 {
				ca.Mean = arr[hour].Mean()
				ca.StdDev = arr[hour].StdDev()
				if ca.StdDev > 0.001 {
					ca.ZScore = math.Abs(float64(count)-ca.Mean) / ca.StdDev
				}
			}
			details[class] = ca
		}
	}

	return score, details
}

// SensitivityToThreshold converts a sensitivity value [0, 1] to an anomaly
// score threshold. Higher sensitivity => lower threshold => more alerts.
//
// sensitivity=0.0 => threshold=0.95 (only extreme anomalies)
// sensitivity=0.5 => threshold=0.55
// sensitivity=1.0 => threshold=0.15 (flag minor deviations)
func SensitivityToThreshold(sensitivity float64) float64 {
	if sensitivity < 0 {
		sensitivity = 0
	}
	if sensitivity > 1 {
		sensitivity = 1
	}
	// Linear mapping: threshold = 0.95 - 0.80 * sensitivity
	return 0.95 - 0.80*sensitivity
}
