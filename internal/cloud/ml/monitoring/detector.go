package monitoring

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// DriftDetector runs periodic drift checks across all registered models and
// fires alerts when thresholds are exceeded.
type DriftDetector struct {
	mu       sync.RWMutex
	cfg      Config
	models   map[string]*trackedModel // key: ModelKey.String()
	alertMgr *AlertManager
	coll     *Collector
	auditExp *AuditExporter

	stopCh chan struct{}
	done   chan struct{}
}

// trackedModel holds the per-model state needed for drift detection.
type trackedModel struct {
	key     ModelKey
	tracker *DistributionTracker
}

// NewDriftDetector creates a DriftDetector with the given configuration.
func NewDriftDetector(cfg Config, alertMgr *AlertManager, coll *Collector, auditExp *AuditExporter) (*DriftDetector, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &DriftDetector{
		cfg:      cfg,
		models:   make(map[string]*trackedModel),
		alertMgr: alertMgr,
		coll:     coll,
		auditExp: auditExp,
		stopCh:   make(chan struct{}),
		done:     make(chan struct{}),
	}, nil
}

// Register adds a model for drift monitoring and returns its
// DistributionTracker. The caller should set baselines and observe values
// on the returned tracker.
func (d *DriftDetector) Register(key ModelKey) *DistributionTracker {
	d.mu.Lock()
	defer d.mu.Unlock()

	k := key.String()
	if tm, ok := d.models[k]; ok {
		return tm.tracker
	}

	tracker := NewDistributionTracker(d.cfg.HistogramBins)
	d.models[k] = &trackedModel{key: key, tracker: tracker}
	return tracker
}

// Unregister removes a model from drift monitoring.
func (d *DriftDetector) Unregister(key ModelKey) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.models, key.String())
}

// Start begins the periodic drift check loop. It blocks until Stop is called
// or the context is cancelled.
func (d *DriftDetector) Start(ctx context.Context) {
	defer close(d.done)
	ticker := time.NewTicker(d.cfg.DriftCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.runChecks(ctx)
		}
	}
}

// Stop signals the detector to stop its periodic loop.
func (d *DriftDetector) Stop() {
	select {
	case <-d.stopCh:
	default:
		close(d.stopCh)
	}
	<-d.done
}

// RunOnce performs a single drift check cycle across all registered models.
// This is useful for testing and manual invocation.
func (d *DriftDetector) RunOnce(ctx context.Context) []DriftResult {
	return d.runChecks(ctx)
}

func (d *DriftDetector) runChecks(ctx context.Context) []DriftResult {
	d.mu.RLock()
	models := make([]*trackedModel, 0, len(d.models))
	for _, tm := range d.models {
		models = append(models, tm)
	}
	d.mu.RUnlock()

	var results []DriftResult
	for _, tm := range models {
		if ctx.Err() != nil {
			break
		}
		result := d.checkModel(ctx, tm)
		results = append(results, result)
	}
	return results
}

func (d *DriftDetector) checkModel(_ context.Context, tm *trackedModel) DriftResult {
	now := time.Now().UTC()
	result := DriftResult{
		Key:       tm.key,
		Timestamp: now,
	}

	featureDrifts, err := tm.tracker.ComputeDrift()
	if err != nil {
		// No baseline or no live data yet; skip silently.
		return result
	}

	result.FeatureDrifts = featureDrifts

	// Aggregate: use maximum drift across features.
	var maxKL, maxPSI float64
	for _, fd := range featureDrifts {
		if fd.KLDivergence > maxKL {
			maxKL = fd.KLDivergence
		}
		if fd.PSI > maxPSI {
			maxPSI = fd.PSI
		}
	}
	result.KLDivergence = maxKL
	result.PSI = maxPSI

	// Mark per-feature drift.
	for name, fd := range featureDrifts {
		fd.Drifted = fd.KLDivergence > d.cfg.KLDivergenceThreshold ||
			fd.PSI > d.cfg.PSIThreshold
		result.FeatureDrifts[name] = fd
	}

	// Determine overall drift.
	if maxKL > d.cfg.KLDivergenceThreshold || maxPSI > d.cfg.PSIThreshold {
		result.Drifted = true
		result.Reason = fmt.Sprintf(
			"distribution shift detected: KL=%.4f (threshold=%.4f), PSI=%.4f (threshold=%.4f)",
			maxKL, d.cfg.KLDivergenceThreshold, maxPSI, d.cfg.PSIThreshold,
		)
	}

	// Record metrics.
	if d.coll != nil {
		d.coll.RecordDrift(result)
	}

	// Fire alert if drifted.
	if result.Drifted && d.alertMgr != nil {
		severity := SeverityWarning
		if maxKL > d.cfg.KLDivergenceThreshold*2 || maxPSI > d.cfg.PSIThreshold*2 {
			severity = SeverityCritical
		}
		alert := Alert{
			ID:               fmt.Sprintf("drift-%s-%d", tm.key.String(), now.UnixMilli()),
			Severity:         severity,
			Key:              tm.key,
			Type:             "drift",
			Message:          result.Reason,
			Value:            maxKL,
			Threshold:        d.cfg.KLDivergenceThreshold,
			Timestamp:        now,
			OnCallRotationID: d.cfg.OnCallRotationID,
		}
		if err := d.alertMgr.Fire(alert); err != nil {
			log.Printf("monitoring: failed to fire drift alert for %s: %v", tm.key, err)
		}
	}

	// Record audit evidence.
	if d.auditExp != nil {
		kl := result.KLDivergence
		psi := result.PSI
		d.auditExp.Record(AuditRecord{
			ID:           fmt.Sprintf("check-%s-%d", tm.key.String(), now.UnixMilli()),
			Timestamp:    now,
			TenantID:     tm.key.TenantID,
			ModelID:      tm.key.ModelID,
			ModelVersion: tm.key.Version,
			EventType:    "drift_check",
			Details:      result.Reason,
			KLDivergence: &kl,
			PSI:          &psi,
			AlertFired:   result.Drifted,
			ExportedAt:   now,
		})
	}

	// Reset live distributions for next window.
	tm.tracker.ResetLive()

	return result
}

// CheckAccuracy evaluates model accuracy and fires alerts if below baseline.
func (d *DriftDetector) CheckAccuracy(key ModelKey, accuracy float64) *Alert {
	if accuracy >= d.cfg.AccuracyFloorPct {
		return nil
	}

	now := time.Now().UTC()
	alert := Alert{
		ID:               fmt.Sprintf("accuracy-%s-%d", key.String(), now.UnixMilli()),
		Severity:         SeverityCritical,
		Key:              key,
		Type:             "accuracy",
		Message:          fmt.Sprintf("accuracy %.2f%% below floor %.2f%%", accuracy, d.cfg.AccuracyFloorPct),
		Value:            accuracy,
		Threshold:        d.cfg.AccuracyFloorPct,
		Timestamp:        now,
		OnCallRotationID: d.cfg.OnCallRotationID,
	}

	if d.alertMgr != nil {
		if err := d.alertMgr.Fire(alert); err != nil {
			log.Printf("monitoring: failed to fire accuracy alert for %s: %v", key, err)
		}
	}
	return &alert
}

// CheckFPRate evaluates model false positive rate and fires alerts if above ceiling.
func (d *DriftDetector) CheckFPRate(key ModelKey, fpRate float64) *Alert {
	if fpRate <= d.cfg.FPRateCeilingPct {
		return nil
	}

	now := time.Now().UTC()
	alert := Alert{
		ID:               fmt.Sprintf("fprate-%s-%d", key.String(), now.UnixMilli()),
		Severity:         SeverityWarning,
		Key:              key,
		Type:             "fp_rate",
		Message:          fmt.Sprintf("FP rate %.2f%% above ceiling %.2f%%", fpRate, d.cfg.FPRateCeilingPct),
		Value:            fpRate,
		Threshold:        d.cfg.FPRateCeilingPct,
		Timestamp:        now,
		OnCallRotationID: d.cfg.OnCallRotationID,
	}

	if d.alertMgr != nil {
		if err := d.alertMgr.Fire(alert); err != nil {
			log.Printf("monitoring: failed to fire FP rate alert for %s: %v", key, err)
		}
	}
	return &alert
}

// CheckLatency evaluates model p99 latency and fires alerts if above ceiling.
func (d *DriftDetector) CheckLatency(key ModelKey, p99 time.Duration) *Alert {
	if p99 <= d.cfg.LatencyP99Ceiling {
		return nil
	}

	now := time.Now().UTC()
	alert := Alert{
		ID:               fmt.Sprintf("latency-%s-%d", key.String(), now.UnixMilli()),
		Severity:         SeverityWarning,
		Key:              key,
		Type:             "latency",
		Message:          fmt.Sprintf("p99 latency %v above ceiling %v", p99, d.cfg.LatencyP99Ceiling),
		Value:            float64(p99.Milliseconds()),
		Threshold:        float64(d.cfg.LatencyP99Ceiling.Milliseconds()),
		Timestamp:        now,
		OnCallRotationID: d.cfg.OnCallRotationID,
	}

	if d.alertMgr != nil {
		if err := d.alertMgr.Fire(alert); err != nil {
			log.Printf("monitoring: failed to fire latency alert for %s: %v", key, err)
		}
	}
	return &alert
}
