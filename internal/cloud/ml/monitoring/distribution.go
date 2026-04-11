package monitoring

import (
	"fmt"
	"math"
	"sync"
)

// DistributionTracker maintains reference (baseline) and live feature
// histograms for a single model. It computes KL divergence and PSI to
// detect distribution shift.
//
// Thread-safe: all methods are safe for concurrent use.
type DistributionTracker struct {
	mu   sync.RWMutex
	bins int

	// reference holds the baseline distribution per feature.
	reference map[string]*featureHistogram
	// live holds the current observation distribution per feature.
	live map[string]*featureHistogram
}

// featureHistogram is a fixed-bin histogram for a single feature.
type featureHistogram struct {
	counts []float64
	min    float64
	max    float64
	total  float64
}

// NewDistributionTracker creates a tracker with the given number of bins.
func NewDistributionTracker(bins int) *DistributionTracker {
	if bins < 2 {
		bins = 20
	}
	return &DistributionTracker{
		bins:      bins,
		reference: make(map[string]*featureHistogram),
		live:      make(map[string]*featureHistogram),
	}
}

// SetBaseline sets the reference distribution for a feature from a slice of
// observed values. This is typically called once after model deployment with
// validation data.
func (dt *DistributionTracker) SetBaseline(feature string, values []float64) error {
	if len(values) == 0 {
		return fmt.Errorf("monitoring: empty baseline values for feature %q", feature)
	}

	dt.mu.Lock()
	defer dt.mu.Unlock()

	h := dt.buildHistogram(values)
	dt.reference[feature] = h
	return nil
}

// Observe records a single observation for a feature in the live distribution.
func (dt *DistributionTracker) Observe(feature string, value float64) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	h, ok := dt.live[feature]
	if !ok {
		ref, refOK := dt.reference[feature]
		if !refOK {
			// No baseline for this feature; create a placeholder.
			h = &featureHistogram{
				counts: make([]float64, dt.bins),
				min:    value,
				max:    value + 1, // avoid zero-width
			}
		} else {
			// Use reference min/max for bin alignment.
			h = &featureHistogram{
				counts: make([]float64, dt.bins),
				min:    ref.min,
				max:    ref.max,
			}
		}
		dt.live[feature] = h
	}

	bin := dt.binIndex(h, value)
	h.counts[bin]++
	h.total++
}

// ObserveBatch records multiple observations for a feature.
func (dt *DistributionTracker) ObserveBatch(feature string, values []float64) {
	for _, v := range values {
		dt.Observe(feature, v)
	}
}

// ComputeDrift computes drift metrics across all tracked features that have
// both a reference and live distribution.
func (dt *DistributionTracker) ComputeDrift() (map[string]FeatureDrift, error) {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	if len(dt.reference) == 0 {
		return nil, ErrNoBaseline
	}

	results := make(map[string]FeatureDrift)
	for feature, ref := range dt.reference {
		live, ok := dt.live[feature]
		if !ok {
			continue
		}

		refDist := normalize(ref.counts)
		liveDist := normalize(live.counts)

		kl := klDivergence(refDist, liveDist)
		psi := populationStabilityIndex(refDist, liveDist)

		results[feature] = FeatureDrift{
			FeatureName:  feature,
			KLDivergence: kl,
			PSI:          psi,
		}
	}

	return results, nil
}

// ResetLive clears all live distributions, typically after a drift check cycle.
func (dt *DistributionTracker) ResetLive() {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	dt.live = make(map[string]*featureHistogram)
}

// Features returns the names of all tracked features.
func (dt *DistributionTracker) Features() []string {
	dt.mu.RLock()
	defer dt.mu.RUnlock()
	out := make([]string, 0, len(dt.reference))
	for f := range dt.reference {
		out = append(out, f)
	}
	return out
}

// -----------------------------------------------------------------------
// Internal helpers
// -----------------------------------------------------------------------

func (dt *DistributionTracker) buildHistogram(values []float64) *featureHistogram {
	minVal, maxVal := values[0], values[0]
	for _, v := range values[1:] {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	// Avoid zero-width range.
	if maxVal == minVal {
		maxVal = minVal + 1
	}

	h := &featureHistogram{
		counts: make([]float64, dt.bins),
		min:    minVal,
		max:    maxVal,
		total:  float64(len(values)),
	}

	for _, v := range values {
		bin := dt.binIndex(h, v)
		h.counts[bin]++
	}
	return h
}

func (dt *DistributionTracker) binIndex(h *featureHistogram, value float64) int {
	if value <= h.min {
		return 0
	}
	if value >= h.max {
		return dt.bins - 1
	}
	bin := int(float64(dt.bins) * (value - h.min) / (h.max - h.min))
	if bin >= dt.bins {
		bin = dt.bins - 1
	}
	return bin
}

// normalize converts counts to a probability distribution, applying Laplace
// smoothing to avoid zero bins (which would cause infinite KL divergence).
func normalize(counts []float64) []float64 {
	const epsilon = 1e-10
	total := 0.0
	for _, c := range counts {
		total += c + epsilon
	}
	dist := make([]float64, len(counts))
	for i, c := range counts {
		dist[i] = (c + epsilon) / total
	}
	return dist
}

// klDivergence computes KL(P || Q) where P is the reference distribution and
// Q is the live distribution.
func klDivergence(p, q []float64) float64 {
	kl := 0.0
	for i := range p {
		if p[i] > 0 && q[i] > 0 {
			kl += p[i] * math.Log(p[i]/q[i])
		}
	}
	return kl
}

// populationStabilityIndex computes PSI = sum((p_i - q_i) * ln(p_i / q_i)).
func populationStabilityIndex(p, q []float64) float64 {
	psi := 0.0
	for i := range p {
		if p[i] > 0 && q[i] > 0 {
			psi += (p[i] - q[i]) * math.Log(p[i]/q[i])
		}
	}
	return psi
}
