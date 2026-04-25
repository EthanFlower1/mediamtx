package recordermetrics_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"

	"github.com/bluenviron/mediamtx/internal/recorder/recordermetrics"
)

// TestNew_AllMetricsRegistered asserts that every named metric appears in the
// registry after construction.
func TestNew_AllMetricsRegistered(t *testing.T) {
	m := recordermetrics.New()

	mfs, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	got := make(map[string]struct{}, len(mfs))
	for _, mf := range mfs {
		got[mf.GetName()] = struct{}{}
	}

	required := []string{
		"cameras_expected_total",
		"cameras_publishing_total",
		"reconcile_errors_total",
		"recovery_scan_scanned_total",
		"recovery_scan_repaired_total",
		"recovery_scan_unrecoverable_total",
		"integrity_verifications_total",
		"integrity_quarantines_total",
		"fragmentbackfill_indexed_total",
		"disk_used_bytes",
		"disk_capacity_bytes",
		"disk_used_percent",
		"recorder_build_info",
	}

	for _, name := range required {
		if _, ok := got[name]; !ok {
			t.Errorf("metric %q not found in registry; got: %v", name, keys(got))
		}
	}
}

// TestUpdateGauges_AppliesSnapshot verifies that UpdateGauges writes the
// correct values into the gauge metrics, including the derived DiskUsedPercent.
func TestUpdateGauges_AppliesSnapshot(t *testing.T) {
	m := recordermetrics.New()

	snap := recordermetrics.Snapshot{
		CamerasExpected:   5,
		CamerasPublishing: 3,
		DiskUsedBytes:     1 << 30, // 1 GiB
		DiskCapacityBytes: 4 << 30, // 4 GiB
	}
	m.UpdateGauges(snap)

	mfs, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	byName := gatherByName(mfs)

	assertGauge(t, byName, "cameras_expected_total", 5)
	assertGauge(t, byName, "cameras_publishing_total", 3)
	assertGauge(t, byName, "disk_used_bytes", float64(1<<30))
	assertGauge(t, byName, "disk_capacity_bytes", float64(4<<30))
	assertGauge(t, byName, "disk_used_percent", 25.0) // 1/4 * 100
}

// TestUpdateGauges_ZeroCapacity verifies that disk_used_percent is 0 when
// capacity is 0 (avoids a divide-by-zero).
func TestUpdateGauges_ZeroCapacity(t *testing.T) {
	m := recordermetrics.New()
	m.UpdateGauges(recordermetrics.Snapshot{DiskCapacityBytes: 0, DiskUsedBytes: 0})

	mfs, _ := m.Registry().Gather()
	byName := gatherByName(mfs)
	assertGauge(t, byName, "disk_used_percent", 0)
}

// TestHandler_ServesMetrics verifies that GET /metrics returns 200 and a
// body that contains the expected metric family names.
func TestHandler_ServesMetrics(t *testing.T) {
	m := recordermetrics.New()
	m.UpdateGauges(recordermetrics.Snapshot{
		CamerasExpected:   2,
		CamerasPublishing: 1,
		DiskUsedBytes:     512,
		DiskCapacityBytes: 1024,
	})

	srv := httptest.NewServer(m.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	bodyStr := string(body)

	for _, name := range []string{
		"cameras_expected_total",
		"cameras_publishing_total",
		"disk_used_bytes",
		"disk_capacity_bytes",
		"disk_used_percent",
		"recorder_build_info",
	} {
		if !strings.Contains(bodyStr, name) {
			t.Errorf("body does not contain metric %q", name)
		}
	}
}

// TestCountersIncrement verifies that counter metrics increment additively.
func TestCountersIncrement(t *testing.T) {
	m := recordermetrics.New()

	m.RecoveryScanScanned.Add(10)
	m.RecoveryScanRepaired.Add(3)
	m.RecoveryScanUnrecoverable.Add(1)
	m.IntegrityVerifications.Inc()
	m.IntegrityVerifications.Inc()
	m.IntegrityQuarantines.Inc()
	m.ReconcileErrors.Add(2)
	m.FragmentBackfillIndexed.Add(7)

	mfs, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	byName := gatherByName(mfs)

	assertCounter(t, byName, "recovery_scan_scanned_total", 10)
	assertCounter(t, byName, "recovery_scan_repaired_total", 3)
	assertCounter(t, byName, "recovery_scan_unrecoverable_total", 1)
	assertCounter(t, byName, "integrity_verifications_total", 2)
	assertCounter(t, byName, "integrity_quarantines_total", 1)
	assertCounter(t, byName, "reconcile_errors_total", 2)
	assertCounter(t, byName, "fragmentbackfill_indexed_total", 7)
}

// --- helpers -----------------------------------------------------------------

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func gatherByName(mfs []*dto.MetricFamily) map[string]*dto.MetricFamily {
	m := make(map[string]*dto.MetricFamily, len(mfs))
	for _, mf := range mfs {
		m[mf.GetName()] = mf
	}
	return m
}

func assertGauge(t *testing.T, byName map[string]*dto.MetricFamily, name string, want float64) {
	t.Helper()
	mf, ok := byName[name]
	if !ok {
		t.Errorf("gauge %q not found", name)
		return
	}
	if len(mf.Metric) == 0 {
		t.Errorf("gauge %q has no metrics", name)
		return
	}
	got := mf.Metric[0].GetGauge().GetValue()
	if got != want {
		t.Errorf("gauge %q = %v, want %v", name, got, want)
	}
}

func assertCounter(t *testing.T, byName map[string]*dto.MetricFamily, name string, want float64) {
	t.Helper()
	mf, ok := byName[name]
	if !ok {
		t.Errorf("counter %q not found", name)
		return
	}
	if len(mf.Metric) == 0 {
		t.Errorf("counter %q has no metrics", name)
		return
	}
	got := mf.Metric[0].GetCounter().GetValue()
	if got != want {
		t.Errorf("counter %q = %v, want %v", name, got, want)
	}
}
