package monitoring

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// -----------------------------------------------------------------------
// Config validation tests
// -----------------------------------------------------------------------

func TestConfigValidate(t *testing.T) {
	t.Run("default config is valid", func(t *testing.T) {
		cfg := DefaultConfig()
		if err := cfg.Validate(); err != nil {
			t.Fatalf("default config should be valid: %v", err)
		}
	})

	t.Run("zero drift interval is invalid", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DriftCheckInterval = 0
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for zero DriftCheckInterval")
		}
	})

	t.Run("negative KL threshold is invalid", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.KLDivergenceThreshold = -1
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for negative KLDivergenceThreshold")
		}
	})

	t.Run("bins < 2 is invalid", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.HistogramBins = 1
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for HistogramBins < 2")
		}
	})

	t.Run("accuracy out of range", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AccuracyFloorPct = 101
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for AccuracyFloorPct > 100")
		}
	})
}

// -----------------------------------------------------------------------
// ModelKey tests
// -----------------------------------------------------------------------

func TestModelKeyString(t *testing.T) {
	key := ModelKey{TenantID: "t1", ModelID: "yolo-v8", Version: "1.0"}
	got := key.String()
	if got != "t1/yolo-v8:1.0" {
		t.Fatalf("unexpected key string: %s", got)
	}
}

// -----------------------------------------------------------------------
// Distribution tracker tests
// -----------------------------------------------------------------------

func TestDistributionTracker_SetBaseline(t *testing.T) {
	dt := NewDistributionTracker(10)

	t.Run("empty values returns error", func(t *testing.T) {
		err := dt.SetBaseline("feat1", nil)
		if err == nil {
			t.Fatal("expected error for empty values")
		}
	})

	t.Run("valid baseline succeeds", func(t *testing.T) {
		err := dt.SetBaseline("feat1", generateNormal(1000, 0, 1))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		features := dt.Features()
		if len(features) != 1 || features[0] != "feat1" {
			t.Fatalf("expected [feat1], got %v", features)
		}
	})
}

func TestDistributionTracker_NoDrift(t *testing.T) {
	dt := NewDistributionTracker(20)
	baseline := generateNormal(5000, 0, 1)
	if err := dt.SetBaseline("feat1", baseline); err != nil {
		t.Fatal(err)
	}

	// Observe from same distribution.
	live := generateNormal(5000, 0, 1)
	dt.ObserveBatch("feat1", live)

	drifts, err := dt.ComputeDrift()
	if err != nil {
		t.Fatal(err)
	}

	fd := drifts["feat1"]
	// Same distribution should have very low KL and PSI.
	if fd.KLDivergence > 0.05 {
		t.Errorf("KL divergence too high for same distribution: %.4f", fd.KLDivergence)
	}
	if fd.PSI > 0.05 {
		t.Errorf("PSI too high for same distribution: %.4f", fd.PSI)
	}
}

func TestDistributionTracker_SyntheticDrift(t *testing.T) {
	dt := NewDistributionTracker(20)

	// Baseline: N(0, 1)
	baseline := generateNormal(5000, 0, 1)
	if err := dt.SetBaseline("feat1", baseline); err != nil {
		t.Fatal(err)
	}

	// Live: N(2, 1.5) — significant distribution shift.
	live := generateNormal(5000, 2.0, 1.5)
	dt.ObserveBatch("feat1", live)

	drifts, err := dt.ComputeDrift()
	if err != nil {
		t.Fatal(err)
	}

	fd := drifts["feat1"]
	t.Logf("Synthetic drift: KL=%.4f, PSI=%.4f", fd.KLDivergence, fd.PSI)

	// Shifted distribution should produce significant KL and PSI.
	if fd.KLDivergence < 0.1 {
		t.Errorf("KL divergence should be significant for shifted distribution: %.4f", fd.KLDivergence)
	}
	if fd.PSI < 0.1 {
		t.Errorf("PSI should be significant for shifted distribution: %.4f", fd.PSI)
	}
}

func TestDistributionTracker_MultipleFeatures(t *testing.T) {
	dt := NewDistributionTracker(15)

	// Feature 1: no drift.
	_ = dt.SetBaseline("brightness", generateNormal(3000, 128, 30))
	dt.ObserveBatch("brightness", generateNormal(3000, 128, 30))

	// Feature 2: significant drift.
	_ = dt.SetBaseline("contrast", generateNormal(3000, 50, 10))
	dt.ObserveBatch("contrast", generateNormal(3000, 80, 15))

	drifts, err := dt.ComputeDrift()
	if err != nil {
		t.Fatal(err)
	}

	if len(drifts) != 2 {
		t.Fatalf("expected 2 feature drifts, got %d", len(drifts))
	}

	// Contrast should show more drift than brightness.
	if drifts["contrast"].KLDivergence <= drifts["brightness"].KLDivergence {
		t.Error("contrast should have higher KL divergence than brightness")
	}
}

func TestDistributionTracker_NoBaseline(t *testing.T) {
	dt := NewDistributionTracker(10)
	_, err := dt.ComputeDrift()
	if err != ErrNoBaseline {
		t.Fatalf("expected ErrNoBaseline, got %v", err)
	}
}

func TestDistributionTracker_ResetLive(t *testing.T) {
	dt := NewDistributionTracker(10)
	_ = dt.SetBaseline("f1", generateNormal(100, 0, 1))
	dt.ObserveBatch("f1", generateNormal(100, 5, 1))

	dt.ResetLive()

	// After reset, no live data, ComputeDrift should return empty map.
	drifts, err := dt.ComputeDrift()
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 0 {
		t.Fatalf("expected empty drifts after reset, got %d", len(drifts))
	}
}

// -----------------------------------------------------------------------
// KL Divergence and PSI unit tests
// -----------------------------------------------------------------------

func TestKLDivergence_Identical(t *testing.T) {
	p := []float64{0.25, 0.25, 0.25, 0.25}
	kl := klDivergence(p, p)
	if math.Abs(kl) > 1e-10 {
		t.Errorf("KL(P||P) should be 0, got %.10f", kl)
	}
}

func TestPSI_Identical(t *testing.T) {
	p := []float64{0.25, 0.25, 0.25, 0.25}
	psi := populationStabilityIndex(p, p)
	if math.Abs(psi) > 1e-10 {
		t.Errorf("PSI(P,P) should be 0, got %.10f", psi)
	}
}

func TestPSI_KnownShift(t *testing.T) {
	p := []float64{0.4, 0.3, 0.2, 0.1}
	q := []float64{0.1, 0.2, 0.3, 0.4}
	psi := populationStabilityIndex(p, q)
	if psi < 0.1 {
		t.Errorf("PSI should indicate significant shift, got %.4f", psi)
	}
}

// -----------------------------------------------------------------------
// Collector tests
// -----------------------------------------------------------------------

func TestCollector_RecordInference(t *testing.T) {
	reg := prometheus.NewRegistry()
	coll := NewCollector(reg)

	key := ModelKey{TenantID: "t1", ModelID: "yolo", Version: "1.0"}
	coll.RecordInference(key, 0.05)
	coll.RecordInference(key, 0.10)

	// Gather metrics and verify.
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "kaivue_model_inference_total" {
			found = true
			if len(mf.GetMetric()) == 0 {
				t.Fatal("no metrics for inference_total")
			}
			val := mf.GetMetric()[0].GetCounter().GetValue()
			if val != 2 {
				t.Errorf("expected 2 inferences, got %.0f", val)
			}
		}
	}
	if !found {
		t.Fatal("kaivue_model_inference_total metric not found")
	}
}

func TestCollector_RecordClassification(t *testing.T) {
	reg := prometheus.NewRegistry()
	coll := NewCollector(reg)

	key := ModelKey{TenantID: "t1", ModelID: "detector", Version: "2.0"}
	coll.RecordClassification(key, 80, 5, 10, 5) // accuracy = 90%, FP rate = 33%

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}

	for _, mf := range mfs {
		if mf.GetName() == "kaivue_model_accuracy_pct" {
			val := mf.GetMetric()[0].GetGauge().GetValue()
			if math.Abs(val-90.0) > 0.1 {
				t.Errorf("expected accuracy ~90%%, got %.2f%%", val)
			}
		}
	}
}

func TestCollector_RecordDrift(t *testing.T) {
	reg := prometheus.NewRegistry()
	coll := NewCollector(reg)

	result := DriftResult{
		Key:          ModelKey{TenantID: "t1", ModelID: "m1", Version: "v1"},
		KLDivergence: 0.35,
		PSI:          0.42,
		Drifted:      true,
	}
	coll.RecordDrift(result)

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}

	var foundKL, foundPSI, foundAlerts bool
	for _, mf := range mfs {
		switch mf.GetName() {
		case "kaivue_model_drift_kl_divergence":
			foundKL = true
			val := mf.GetMetric()[0].GetGauge().GetValue()
			if val != 0.35 {
				t.Errorf("expected KL 0.35, got %.4f", val)
			}
		case "kaivue_model_drift_psi":
			foundPSI = true
			val := mf.GetMetric()[0].GetGauge().GetValue()
			if val != 0.42 {
				t.Errorf("expected PSI 0.42, got %.4f", val)
			}
		case "kaivue_model_drift_alerts_total":
			foundAlerts = true
			val := mf.GetMetric()[0].GetCounter().GetValue()
			if val != 1 {
				t.Errorf("expected 1 drift alert, got %.0f", val)
			}
		}
	}
	if !foundKL || !foundPSI || !foundAlerts {
		t.Error("missing drift metrics")
	}
}

// -----------------------------------------------------------------------
// DriftDetector tests
// -----------------------------------------------------------------------

func TestDriftDetector_SyntheticShift(t *testing.T) {
	reg := prometheus.NewRegistry()
	coll := NewCollector(reg)
	alertMgr := NewAlertManager(0)
	auditExp := NewAuditExporter(DefaultConfig())

	cfg := DefaultConfig()
	cfg.KLDivergenceThreshold = 0.1
	cfg.PSIThreshold = 0.15

	detector, err := NewDriftDetector(cfg, alertMgr, coll, auditExp)
	if err != nil {
		t.Fatal(err)
	}

	key := ModelKey{TenantID: "tenant-1", ModelID: "person-detector", Version: "3.0"}
	tracker := detector.Register(key)

	// Set baseline: N(0, 1)
	if err := tracker.SetBaseline("confidence", generateNormal(5000, 0, 1)); err != nil {
		t.Fatal(err)
	}

	// Observe shifted distribution: N(2, 1.5)
	tracker.ObserveBatch("confidence", generateNormal(5000, 2.0, 1.5))

	// Run drift check.
	results := detector.RunOnce(context.Background())
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]
	if !result.Drifted {
		t.Fatalf("expected drift to be detected (KL=%.4f, PSI=%.4f)", result.KLDivergence, result.PSI)
	}

	t.Logf("Drift detected: KL=%.4f, PSI=%.4f, reason=%s", result.KLDivergence, result.PSI, result.Reason)

	// Verify alert was fired.
	alerts := alertMgr.FiredAlerts()
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Type != "drift" {
		t.Errorf("expected drift alert, got %s", alerts[0].Type)
	}
	if alerts[0].Key.ModelID != "person-detector" {
		t.Errorf("unexpected model in alert: %s", alerts[0].Key.ModelID)
	}

	// Verify audit record was created.
	records := auditExp.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(records))
	}
	if records[0].EventType != "drift_check" {
		t.Errorf("expected drift_check event, got %s", records[0].EventType)
	}
	if !records[0].AlertFired {
		t.Error("expected AlertFired=true in audit record")
	}
}

func TestDriftDetector_NoDrift(t *testing.T) {
	cfg := DefaultConfig()
	cfg.KLDivergenceThreshold = 0.2
	cfg.PSIThreshold = 0.25

	detector, err := NewDriftDetector(cfg, NewAlertManager(0), nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	key := ModelKey{TenantID: "t1", ModelID: "m1", Version: "1.0"}
	tracker := detector.Register(key)

	// Same distribution for baseline and live.
	baseline := generateNormal(5000, 0, 1)
	_ = tracker.SetBaseline("f1", baseline)
	tracker.ObserveBatch("f1", generateNormal(5000, 0, 1))

	results := detector.RunOnce(context.Background())
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Drifted {
		t.Errorf("should not detect drift for same distribution (KL=%.4f, PSI=%.4f)",
			results[0].KLDivergence, results[0].PSI)
	}
}

func TestDriftDetector_AccuracyAlert(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AccuracyFloorPct = 95.0

	alertMgr := NewAlertManager(0)
	detector, err := NewDriftDetector(cfg, alertMgr, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	key := ModelKey{TenantID: "t1", ModelID: "m1", Version: "1.0"}

	// Above floor: no alert.
	alert := detector.CheckAccuracy(key, 96.0)
	if alert != nil {
		t.Error("should not alert when accuracy is above floor")
	}

	// Below floor: alert.
	alert = detector.CheckAccuracy(key, 88.5)
	if alert == nil {
		t.Fatal("should alert when accuracy is below floor")
	}
	if alert.Type != "accuracy" {
		t.Errorf("expected accuracy alert, got %s", alert.Type)
	}
	if alert.Severity != SeverityCritical {
		t.Errorf("expected critical severity, got %s", alert.Severity)
	}

	fired := alertMgr.FiredAlerts()
	if len(fired) != 1 {
		t.Fatalf("expected 1 fired alert, got %d", len(fired))
	}
}

func TestDriftDetector_FPRateAlert(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FPRateCeilingPct = 5.0

	alertMgr := NewAlertManager(0)
	detector, err := NewDriftDetector(cfg, alertMgr, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	key := ModelKey{TenantID: "t1", ModelID: "m1", Version: "1.0"}
	alert := detector.CheckFPRate(key, 7.5)
	if alert == nil {
		t.Fatal("should alert when FP rate exceeds ceiling")
	}
	if alert.Type != "fp_rate" {
		t.Errorf("expected fp_rate alert, got %s", alert.Type)
	}
}

func TestDriftDetector_LatencyAlert(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LatencyP99Ceiling = 100 * time.Millisecond

	alertMgr := NewAlertManager(0)
	detector, err := NewDriftDetector(cfg, alertMgr, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	key := ModelKey{TenantID: "t1", ModelID: "m1", Version: "1.0"}

	// Under ceiling: no alert.
	alert := detector.CheckLatency(key, 50*time.Millisecond)
	if alert != nil {
		t.Error("should not alert when latency is under ceiling")
	}

	// Over ceiling: alert.
	alert = detector.CheckLatency(key, 200*time.Millisecond)
	if alert == nil {
		t.Fatal("should alert when latency exceeds ceiling")
	}
	if alert.Type != "latency" {
		t.Errorf("expected latency alert, got %s", alert.Type)
	}
}

func TestDriftDetector_Unregister(t *testing.T) {
	cfg := DefaultConfig()
	detector, err := NewDriftDetector(cfg, NewAlertManager(0), nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	key := ModelKey{TenantID: "t1", ModelID: "m1", Version: "1.0"}
	detector.Register(key)
	detector.Unregister(key)

	results := detector.RunOnce(context.Background())
	if len(results) != 0 {
		t.Fatalf("expected 0 results after unregister, got %d", len(results))
	}
}

// -----------------------------------------------------------------------
// AlertManager tests
// -----------------------------------------------------------------------

func TestAlertManager_Cooldown(t *testing.T) {
	am := NewAlertManager(1 * time.Hour) // 1hr cooldown

	var received int
	am.AddHandler(func(_ Alert) error {
		received++
		return nil
	})

	alert := Alert{
		Type:     "drift",
		Severity: SeverityWarning,
		Key:      ModelKey{TenantID: "t1", ModelID: "m1", Version: "1.0"},
	}

	// First fire should go through.
	_ = am.Fire(alert)
	if received != 1 {
		t.Fatalf("expected 1 handler call, got %d", received)
	}

	// Second fire within cooldown should be suppressed.
	_ = am.Fire(alert)
	if received != 1 {
		t.Fatalf("expected still 1 handler call (cooldown), got %d", received)
	}
}

func TestAlertManager_NoCooldown(t *testing.T) {
	am := NewAlertManager(0)

	var received int
	am.AddHandler(func(_ Alert) error {
		received++
		return nil
	})

	alert := Alert{
		Type:     "drift",
		Severity: SeverityWarning,
		Key:      ModelKey{TenantID: "t1", ModelID: "m1", Version: "1.0"},
	}

	_ = am.Fire(alert)
	_ = am.Fire(alert)
	if received != 2 {
		t.Fatalf("expected 2 handler calls (no cooldown), got %d", received)
	}
}

// -----------------------------------------------------------------------
// Dashboard config tests
// -----------------------------------------------------------------------

func TestDashboardConfig_GenerateModelDashboard(t *testing.T) {
	dc := NewDashboardConfig(DefaultConfig())
	key := ModelKey{TenantID: "tenant-1", ModelID: "yolo-v8", Version: "2.0"}

	data, err := dc.GenerateModelDashboard(key)
	if err != nil {
		t.Fatal(err)
	}

	var dashboard map[string]any
	if err := json.Unmarshal(data, &dashboard); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	title, ok := dashboard["title"].(string)
	if !ok || !strings.Contains(title, "yolo-v8") {
		t.Errorf("expected title containing model ID, got %q", title)
	}

	panels, ok := dashboard["panels"].([]any)
	if !ok || len(panels) < 8 {
		t.Errorf("expected at least 8 panels, got %d", len(panels))
	}

	tags, ok := dashboard["tags"].([]any)
	if !ok {
		t.Fatal("expected tags array")
	}
	foundAutoProvisioned := false
	for _, tag := range tags {
		if tag.(string) == "auto-provisioned" {
			foundAutoProvisioned = true
		}
	}
	if !foundAutoProvisioned {
		t.Error("expected auto-provisioned tag")
	}
}

func TestDashboardConfig_GenerateOverviewDashboard(t *testing.T) {
	dc := NewDashboardConfig(DefaultConfig())

	data, err := dc.GenerateOverviewDashboard("tenant-1")
	if err != nil {
		t.Fatal(err)
	}

	var dashboard map[string]any
	if err := json.Unmarshal(data, &dashboard); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	title := dashboard["title"].(string)
	if !strings.Contains(title, "tenant-1") {
		t.Errorf("expected title containing tenant ID, got %q", title)
	}
}

func TestDashboardConfig_InvalidInputs(t *testing.T) {
	dc := NewDashboardConfig(DefaultConfig())

	_, err := dc.GenerateModelDashboard(ModelKey{})
	if err != ErrInvalidTenantID {
		t.Errorf("expected ErrInvalidTenantID, got %v", err)
	}

	_, err = dc.GenerateModelDashboard(ModelKey{TenantID: "t1"})
	if err != ErrInvalidModelID {
		t.Errorf("expected ErrInvalidModelID, got %v", err)
	}

	_, err = dc.GenerateOverviewDashboard("")
	if err != ErrInvalidTenantID {
		t.Errorf("expected ErrInvalidTenantID, got %v", err)
	}
}

// -----------------------------------------------------------------------
// Audit exporter tests
// -----------------------------------------------------------------------

func TestAuditExporter_RecordAndExport(t *testing.T) {
	ae := NewAuditExporter(DefaultConfig())
	now := time.Now().UTC()
	kl := 0.35

	ae.Record(AuditRecord{
		ID:           "rec-1",
		Timestamp:    now,
		TenantID:     "t1",
		ModelID:      "m1",
		ModelVersion: "v1",
		EventType:    "drift_check",
		Details:      "test drift",
		KLDivergence: &kl,
		AlertFired:   true,
		ExportedAt:   now,
	})

	ae.Record(AuditRecord{
		ID:           "rec-2",
		Timestamp:    now,
		TenantID:     "t2",
		ModelID:      "m2",
		ModelVersion: "v1",
		EventType:    "drift_check",
		Details:      "no drift",
		AlertFired:   false,
		ExportedAt:   now,
	})

	t.Run("all records", func(t *testing.T) {
		records := ae.Records()
		if len(records) != 2 {
			t.Fatalf("expected 2 records, got %d", len(records))
		}
	})

	t.Run("filter by tenant", func(t *testing.T) {
		records := ae.RecordsByTenant("t1")
		if len(records) != 1 {
			t.Fatalf("expected 1 record for t1, got %d", len(records))
		}
		if records[0].ID != "rec-1" {
			t.Errorf("unexpected record ID: %s", records[0].ID)
		}
	})

	t.Run("export JSON", func(t *testing.T) {
		var buf bytes.Buffer
		if err := ae.ExportJSON(&buf); err != nil {
			t.Fatal(err)
		}
		var records []AuditRecord
		if err := json.Unmarshal(buf.Bytes(), &records); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("expected 2 records in JSON, got %d", len(records))
		}
	})

	t.Run("export JSON for tenant", func(t *testing.T) {
		var buf bytes.Buffer
		if err := ae.ExportJSONForTenant(&buf, "t2"); err != nil {
			t.Fatal(err)
		}
		var records []AuditRecord
		if err := json.Unmarshal(buf.Bytes(), &records); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if len(records) != 1 {
			t.Fatalf("expected 1 record for t2, got %d", len(records))
		}
	})

	t.Run("export CSV", func(t *testing.T) {
		var buf bytes.Buffer
		if err := ae.ExportCSV(&buf); err != nil {
			t.Fatal(err)
		}
		reader := csv.NewReader(&buf)
		rows, err := reader.ReadAll()
		if err != nil {
			t.Fatalf("invalid CSV: %v", err)
		}
		// 1 header + 2 data rows.
		if len(rows) != 3 {
			t.Fatalf("expected 3 CSV rows, got %d", len(rows))
		}
		// Verify header.
		if rows[0][0] != "id" {
			t.Errorf("unexpected first column header: %s", rows[0][0])
		}
	})

	t.Run("time range filter", func(t *testing.T) {
		start := now.Add(-1 * time.Hour)
		end := now.Add(1 * time.Hour)
		records := ae.RecordsByTimeRange(start, end)
		if len(records) != 2 {
			t.Fatalf("expected 2 records in range, got %d", len(records))
		}

		// Range excluding all records.
		future := now.Add(1 * time.Hour)
		records = ae.RecordsByTimeRange(future, future.Add(1*time.Hour))
		if len(records) != 0 {
			t.Fatalf("expected 0 records outside range, got %d", len(records))
		}
	})

	t.Run("prune", func(t *testing.T) {
		// Add an old record.
		oldTime := now.Add(-400 * 24 * time.Hour)
		ae.Record(AuditRecord{
			ID:        "old-1",
			Timestamp: oldTime,
			TenantID:  "t1",
		})

		cutoff := now.Add(-365 * 24 * time.Hour)
		pruned := ae.PruneOlderThan(cutoff)
		if pruned != 1 {
			t.Fatalf("expected 1 pruned, got %d", pruned)
		}
		if len(ae.Records()) != 2 {
			t.Fatalf("expected 2 remaining records, got %d", len(ae.Records()))
		}
	})
}

// -----------------------------------------------------------------------
// End-to-end integration test
// -----------------------------------------------------------------------

func TestEndToEnd_DriftPipeline(t *testing.T) {
	// Set up the full pipeline.
	reg := prometheus.NewRegistry()
	coll := NewCollector(reg)
	alertMgr := NewAlertManager(0)
	auditExp := NewAuditExporter(DefaultConfig())

	cfg := DefaultConfig()
	cfg.KLDivergenceThreshold = 0.15
	cfg.PSIThreshold = 0.20
	cfg.AccuracyFloorPct = 90.0
	cfg.FPRateCeilingPct = 5.0
	cfg.LatencyP99Ceiling = 100 * time.Millisecond
	cfg.OnCallRotationID = "rotation-123"

	var alertLog []Alert
	alertMgr.AddHandler(func(a Alert) error {
		alertLog = append(alertLog, a)
		return nil
	})

	detector, err := NewDriftDetector(cfg, alertMgr, coll, auditExp)
	if err != nil {
		t.Fatal(err)
	}

	key := ModelKey{TenantID: "acme-corp", ModelID: "vehicle-classifier", Version: "4.2"}
	tracker := detector.Register(key)

	// Phase 1: Set baselines with training data distribution.
	_ = tracker.SetBaseline("bbox_area", generateNormal(10000, 5000, 1000))
	_ = tracker.SetBaseline("confidence", generateNormal(10000, 0.85, 0.05))

	// Phase 2: Simulate normal operation (no drift).
	tracker.ObserveBatch("bbox_area", generateNormal(5000, 5000, 1000))
	tracker.ObserveBatch("confidence", generateNormal(5000, 0.85, 0.05))

	coll.RecordInference(key, 0.045)
	coll.RecordInference(key, 0.052)
	coll.RecordClassification(key, 90, 3, 5, 2)

	results := detector.RunOnce(context.Background())
	if results[0].Drifted {
		t.Error("should not detect drift during normal operation")
	}

	// Phase 3: Simulate distribution shift (camera angle change).
	tracker.ObserveBatch("bbox_area", generateNormal(5000, 8000, 2000))
	tracker.ObserveBatch("confidence", generateNormal(5000, 0.60, 0.15))

	results = detector.RunOnce(context.Background())
	if !results[0].Drifted {
		t.Fatal("should detect drift after distribution shift")
	}

	// Phase 4: Check performance degradation alerts.
	detector.CheckAccuracy(key, 82.0) // below 90% floor
	detector.CheckFPRate(key, 8.5)    // above 5% ceiling
	detector.CheckLatency(key, 200*time.Millisecond) // above 100ms ceiling

	// Verify all alerts.
	allAlerts := alertMgr.FiredAlerts()
	alertTypes := make(map[string]bool)
	for _, a := range allAlerts {
		alertTypes[a.Type] = true
		if a.OnCallRotationID != "rotation-123" {
			t.Errorf("alert missing on-call rotation ID: %s", a.OnCallRotationID)
		}
	}

	for _, expected := range []string{"drift", "accuracy", "fp_rate", "latency"} {
		if !alertTypes[expected] {
			t.Errorf("missing %s alert", expected)
		}
	}

	// Verify audit trail.
	records := auditExp.Records()
	if len(records) < 2 {
		t.Errorf("expected at least 2 audit records, got %d", len(records))
	}

	// Verify Prometheus metrics are populated.
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	metricNames := make(map[string]bool)
	for _, mf := range mfs {
		metricNames[mf.GetName()] = true
	}
	for _, expected := range []string{
		"kaivue_model_inference_total",
		"kaivue_model_drift_kl_divergence",
		"kaivue_model_drift_psi",
		"kaivue_model_accuracy_pct",
	} {
		if !metricNames[expected] {
			t.Errorf("missing Prometheus metric: %s", expected)
		}
	}

	// Verify dashboard generation.
	dc := NewDashboardConfig(cfg)
	dashJSON, err := dc.GenerateModelDashboard(key)
	if err != nil {
		t.Fatal(err)
	}
	if len(dashJSON) == 0 {
		t.Error("empty dashboard JSON")
	}

	// Verify audit export.
	var csvBuf bytes.Buffer
	if err := auditExp.ExportCSV(&csvBuf); err != nil {
		t.Fatal(err)
	}
	if csvBuf.Len() == 0 {
		t.Error("empty CSV export")
	}

	t.Logf("End-to-end pipeline: %d alerts fired, %d audit records, %d Prometheus metrics",
		len(allAlerts), len(records), len(mfs))
}

// -----------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------

func generateNormal(n int, mean, stddev float64) []float64 {
	rng := rand.New(rand.NewSource(42)) //nolint:gosec
	values := make([]float64, n)
	for i := range values {
		values[i] = rng.NormFloat64()*stddev + mean
	}
	return values
}
