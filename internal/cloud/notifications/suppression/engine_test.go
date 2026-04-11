package suppression_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/notifications/suppression"
)

func openTestDB(t *testing.T) *clouddb.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := "sqlite://" + filepath.Join(dir, "cloud.db")
	d, err := clouddb.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

var seqID int

func testIDGen() string {
	seqID++
	return fmt.Sprintf("test-%04d", seqID)
}

func newEngine(t *testing.T, opts ...func(*suppression.Config)) *suppression.Engine {
	t.Helper()
	db := openTestDB(t)
	cfg := suppression.Config{
		DB:            db,
		IDGen:         testIDGen,
		ClusterWindow: 5 * time.Minute,
	}
	for _, o := range opts {
		o(&cfg)
	}
	eng, err := suppression.NewEngine(cfg)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	return eng
}

func TestEngine_NilDB(t *testing.T) {
	_, err := suppression.NewEngine(suppression.Config{})
	if err == nil {
		t.Fatal("expected error for nil DB")
	}
}

func TestSensitivity_DefaultAndSet(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()

	// Default sensitivity should be 0.5.
	sens, err := eng.GetSensitivity(ctx, "t1", "cam1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if sens != 0.5 {
		t.Errorf("expected default 0.5, got %f", sens)
	}

	// Set to 0.8.
	if err := eng.SetSensitivity(ctx, "t1", "cam1", 0.8); err != nil {
		t.Fatalf("set: %v", err)
	}
	sens, err = eng.GetSensitivity(ctx, "t1", "cam1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if sens != 0.8 {
		t.Errorf("expected 0.8, got %f", sens)
	}
}

func TestSensitivity_OutOfRange(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()

	if err := eng.SetSensitivity(ctx, "t1", "cam1", 1.5); err == nil {
		t.Error("expected error for sensitivity > 1.0")
	}
	if err := eng.SetSensitivity(ctx, "t1", "cam1", -0.1); err == nil {
		t.Error("expected error for sensitivity < 0.0")
	}
}

func TestClustering_GroupsRelatedEvents(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()

	// Default sensitivity 0.5 enables clustering.
	base := time.Date(2026, 4, 10, 14, 0, 0, 0, time.UTC)

	// First event: should not be suppressed (starts a new cluster).
	d1, err := eng.Evaluate(ctx, suppression.Event{
		EventID:   "e1",
		TenantID:  "t1",
		CameraID:  "cam1",
		EventType: "motion",
		Timestamp: base,
		Payload:   `{"zone":"loading_dock"}`,
	})
	if err != nil {
		t.Fatalf("evaluate e1: %v", err)
	}
	if d1.Suppress {
		t.Error("first event should not be suppressed")
	}

	// Second event 1 min later, same camera+type: should be clustered.
	d2, err := eng.Evaluate(ctx, suppression.Event{
		EventID:   "e2",
		TenantID:  "t1",
		CameraID:  "cam1",
		EventType: "motion",
		Timestamp: base.Add(1 * time.Minute),
		Payload:   `{"zone":"loading_dock"}`,
	})
	if err != nil {
		t.Fatalf("evaluate e2: %v", err)
	}
	if !d2.Suppress {
		t.Fatal("second event should be suppressed (clustered)")
	}
	if d2.Reason != suppression.ReasonClustered {
		t.Errorf("expected reason 'clustered', got %q", d2.Reason)
	}
	if d2.ClusterSize != 2 {
		t.Errorf("expected cluster size 2, got %d", d2.ClusterSize)
	}

	// Third event 2 min later.
	d3, err := eng.Evaluate(ctx, suppression.Event{
		EventID:   "e3",
		TenantID:  "t1",
		CameraID:  "cam1",
		EventType: "motion",
		Timestamp: base.Add(3 * time.Minute),
		Payload:   `{"zone":"loading_dock"}`,
	})
	if err != nil {
		t.Fatalf("evaluate e3: %v", err)
	}
	if !d3.Suppress {
		t.Fatal("third event should be suppressed (clustered)")
	}
	if d3.ClusterSize != 3 {
		t.Errorf("expected cluster size 3, got %d", d3.ClusterSize)
	}
	if d3.ClusterSummary == "" {
		t.Error("expected non-empty cluster summary")
	}
}

func TestClustering_DifferentCameraNotGrouped(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()

	base := time.Date(2026, 4, 10, 14, 0, 0, 0, time.UTC)

	eng.Evaluate(ctx, suppression.Event{
		EventID: "e1", TenantID: "t1", CameraID: "cam1",
		EventType: "motion", Timestamp: base,
	})

	// Different camera should start a new cluster.
	d, err := eng.Evaluate(ctx, suppression.Event{
		EventID: "e2", TenantID: "t1", CameraID: "cam2",
		EventType: "motion", Timestamp: base.Add(1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if d.Suppress {
		t.Error("different camera should not be suppressed")
	}
}

func TestClustering_WindowExpiry(t *testing.T) {
	eng := newEngine(t, func(c *suppression.Config) {
		c.ClusterWindow = 2 * time.Minute
	})
	ctx := context.Background()

	base := time.Date(2026, 4, 10, 14, 0, 0, 0, time.UTC)

	eng.Evaluate(ctx, suppression.Event{
		EventID: "e1", TenantID: "t1", CameraID: "cam1",
		EventType: "motion", Timestamp: base,
	})

	// Event 3 minutes later should start a new cluster (window is 2 min).
	d, err := eng.Evaluate(ctx, suppression.Event{
		EventID: "e2", TenantID: "t1", CameraID: "cam1",
		EventType: "motion", Timestamp: base.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if d.Suppress {
		t.Error("event beyond window should not be suppressed")
	}
}

func TestHighActivity_Suppresses(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()

	// Set up a baseline that indicates high activity.
	err := eng.UpdateBaseline(ctx, suppression.Baseline{
		TenantID:    "t1",
		CameraID:    "cam1",
		HourOfDay:   9,
		DayOfWeek:   1, // Monday
		EventType:   "motion",
		AvgCount:    20.0, // Well above any threshold
		StddevCount: 3.0,
		SampleDays:  14,
	})
	if err != nil {
		t.Fatalf("update baseline: %v", err)
	}

	// Monday at 9am.
	ts := time.Date(2026, 4, 13, 9, 30, 0, 0, time.UTC) // Monday
	d, err := eng.Evaluate(ctx, suppression.Event{
		EventID: "e1", TenantID: "t1", CameraID: "cam1",
		EventType: "motion", Timestamp: ts,
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !d.Suppress {
		t.Error("should suppress during high activity window")
	}
	if d.Reason != suppression.ReasonHighActivity {
		t.Errorf("expected reason 'high_activity', got %q", d.Reason)
	}
}

func TestHighActivity_LowBaseline_NoSuppression(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()

	// Low baseline.
	eng.UpdateBaseline(ctx, suppression.Baseline{
		TenantID: "t1", CameraID: "cam1", HourOfDay: 9, DayOfWeek: 1,
		EventType: "motion", AvgCount: 1.0, StddevCount: 0.5, SampleDays: 14,
	})

	ts := time.Date(2026, 4, 13, 9, 30, 0, 0, time.UTC)
	d, err := eng.Evaluate(ctx, suppression.Event{
		EventID: "e1", TenantID: "t1", CameraID: "cam1",
		EventType: "motion", Timestamp: ts,
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if d.Suppress {
		t.Error("low baseline should not suppress")
	}
}

func TestFalsePositive_SuppressesRecurring(t *testing.T) {
	eng := newEngine(t, func(c *suppression.Config) {
		c.MinDismissals = 3
	})
	ctx := context.Background()

	// Record enough dismissals to trigger FP detection.
	for i := 0; i < 5; i++ {
		if err := eng.RecordDismissal(ctx, "t1", "cam1", "shadow"); err != nil {
			t.Fatalf("dismiss %d: %v", i, err)
		}
	}

	ts := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	d, err := eng.Evaluate(ctx, suppression.Event{
		EventID: "e1", TenantID: "t1", CameraID: "cam1",
		EventType: "shadow", Timestamp: ts,
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !d.Suppress {
		t.Error("recurring false positive should be suppressed")
	}
	if d.Reason != suppression.ReasonFalsePos {
		t.Errorf("expected reason 'false_positive', got %q", d.Reason)
	}
}

func TestFalsePositive_BelowThreshold_NoSuppression(t *testing.T) {
	eng := newEngine(t, func(c *suppression.Config) {
		c.MinDismissals = 10
	})
	ctx := context.Background()

	// Only 2 dismissals, well below threshold.
	for i := 0; i < 2; i++ {
		eng.RecordDismissal(ctx, "t1", "cam1", "shadow")
	}

	ts := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	d, err := eng.Evaluate(ctx, suppression.Event{
		EventID: "e1", TenantID: "t1", CameraID: "cam1",
		EventType: "shadow", Timestamp: ts,
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if d.Suppress {
		t.Error("should not suppress below dismissal threshold")
	}
}

func TestSensitivityZero_NoSuppression(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()

	// Disable suppression for this camera.
	eng.SetSensitivity(ctx, "t1", "cam1", 0.0)

	// Set up conditions that would normally trigger suppression.
	for i := 0; i < 10; i++ {
		eng.RecordDismissal(ctx, "t1", "cam1", "motion")
	}

	base := time.Date(2026, 4, 10, 14, 0, 0, 0, time.UTC)

	// First event.
	eng.Evaluate(ctx, suppression.Event{
		EventID: "e1", TenantID: "t1", CameraID: "cam1",
		EventType: "motion", Timestamp: base,
	})

	// Second event should NOT be suppressed because sensitivity is 0.
	d, err := eng.Evaluate(ctx, suppression.Event{
		EventID: "e2", TenantID: "t1", CameraID: "cam1",
		EventType: "motion", Timestamp: base.Add(1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if d.Suppress {
		t.Error("sensitivity 0.0 should disable all suppression")
	}
}

func TestListSuppressed(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()

	base := time.Date(2026, 4, 10, 14, 0, 0, 0, time.UTC)

	// Generate cluster suppression.
	eng.Evaluate(ctx, suppression.Event{
		EventID: "e1", TenantID: "t1", CameraID: "cam1",
		EventType: "motion", Timestamp: base,
	})
	eng.Evaluate(ctx, suppression.Event{
		EventID: "e2", TenantID: "t1", CameraID: "cam1",
		EventType: "motion", Timestamp: base.Add(1 * time.Minute),
		Payload: `{"test":true}`,
	})

	alerts, err := eng.ListSuppressed(ctx, "t1", "", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 suppressed alert, got %d", len(alerts))
	}
	if alerts[0].Reason != suppression.ReasonClustered {
		t.Errorf("expected reason 'clustered', got %q", alerts[0].Reason)
	}
}

func TestListSuppressed_FilterByCamera(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()

	base := time.Date(2026, 4, 10, 14, 0, 0, 0, time.UTC)

	// Create suppressed alerts for two cameras.
	for _, cam := range []string{"cam1", "cam2"} {
		eng.Evaluate(ctx, suppression.Event{
			EventID: cam + "-e1", TenantID: "t1", CameraID: cam,
			EventType: "motion", Timestamp: base,
		})
		eng.Evaluate(ctx, suppression.Event{
			EventID: cam + "-e2", TenantID: "t1", CameraID: cam,
			EventType: "motion", Timestamp: base.Add(1 * time.Minute),
		})
	}

	alerts, err := eng.ListSuppressed(ctx, "t1", "cam1", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 suppressed alert for cam1, got %d", len(alerts))
	}
	if alerts[0].CameraID != "cam1" {
		t.Errorf("expected cam1, got %s", alerts[0].CameraID)
	}
}

// TestSampleStream_50PercentReduction demonstrates that the suppression engine
// achieves 50%+ notification reduction on a sample event stream.
func TestSampleStream_50PercentReduction(t *testing.T) {
	eng := newEngine(t, func(c *suppression.Config) {
		c.ClusterWindow = 5 * time.Minute
		c.MinDismissals = 3
	})
	ctx := context.Background()

	// Set up false positive history for "shadow" events on cam2.
	for i := 0; i < 6; i++ {
		eng.RecordDismissal(ctx, "t1", "cam2", "shadow")
	}

	// Set up high activity baseline for cam3 at 8am on weekdays.
	for dow := 1; dow <= 5; dow++ {
		eng.UpdateBaseline(ctx, suppression.Baseline{
			TenantID: "t1", CameraID: "cam3", HourOfDay: 8, DayOfWeek: dow,
			EventType: "motion", AvgCount: 25.0, StddevCount: 4.0, SampleDays: 30,
		})
	}

	// Generate a sample event stream of 20 events.
	base := time.Date(2026, 4, 13, 8, 0, 0, 0, time.UTC) // Monday 8am
	events := []suppression.Event{
		// Cluster 1: cam1 motion burst (5 events in 4 min) -> 4 suppressed
		{EventID: "e01", TenantID: "t1", CameraID: "cam1", EventType: "motion", Timestamp: base},
		{EventID: "e02", TenantID: "t1", CameraID: "cam1", EventType: "motion", Timestamp: base.Add(1 * time.Minute)},
		{EventID: "e03", TenantID: "t1", CameraID: "cam1", EventType: "motion", Timestamp: base.Add(2 * time.Minute)},
		{EventID: "e04", TenantID: "t1", CameraID: "cam1", EventType: "motion", Timestamp: base.Add(3 * time.Minute)},
		{EventID: "e05", TenantID: "t1", CameraID: "cam1", EventType: "motion", Timestamp: base.Add(4 * time.Minute)},

		// False positive: cam2 shadow events (3 events) -> all suppressed
		{EventID: "e06", TenantID: "t1", CameraID: "cam2", EventType: "shadow", Timestamp: base.Add(5 * time.Minute)},
		{EventID: "e07", TenantID: "t1", CameraID: "cam2", EventType: "shadow", Timestamp: base.Add(6 * time.Minute)},
		{EventID: "e08", TenantID: "t1", CameraID: "cam2", EventType: "shadow", Timestamp: base.Add(7 * time.Minute)},

		// High activity: cam3 motion at 8am Monday (4 events) -> all suppressed
		{EventID: "e09", TenantID: "t1", CameraID: "cam3", EventType: "motion", Timestamp: base.Add(10 * time.Minute)},
		{EventID: "e10", TenantID: "t1", CameraID: "cam3", EventType: "motion", Timestamp: base.Add(11 * time.Minute)},
		{EventID: "e11", TenantID: "t1", CameraID: "cam3", EventType: "motion", Timestamp: base.Add(12 * time.Minute)},
		{EventID: "e12", TenantID: "t1", CameraID: "cam3", EventType: "motion", Timestamp: base.Add(13 * time.Minute)},

		// Legitimate alerts: different cameras / types (should NOT be suppressed)
		{EventID: "e13", TenantID: "t1", CameraID: "cam4", EventType: "intrusion", Timestamp: base.Add(14 * time.Minute)},
		{EventID: "e14", TenantID: "t1", CameraID: "cam5", EventType: "tamper", Timestamp: base.Add(15 * time.Minute)},
		{EventID: "e15", TenantID: "t1", CameraID: "cam6", EventType: "offline", Timestamp: base.Add(16 * time.Minute)},

		// Another cluster: cam1 line_cross (3 events in 2 min) -> 2 suppressed
		{EventID: "e16", TenantID: "t1", CameraID: "cam1", EventType: "line_cross", Timestamp: base.Add(20 * time.Minute)},
		{EventID: "e17", TenantID: "t1", CameraID: "cam1", EventType: "line_cross", Timestamp: base.Add(21 * time.Minute)},
		{EventID: "e18", TenantID: "t1", CameraID: "cam1", EventType: "line_cross", Timestamp: base.Add(22 * time.Minute)},

		// More legitimate alerts.
		{EventID: "e19", TenantID: "t1", CameraID: "cam7", EventType: "loitering", Timestamp: base.Add(25 * time.Minute)},
		{EventID: "e20", TenantID: "t1", CameraID: "cam8", EventType: "intrusion", Timestamp: base.Add(26 * time.Minute)},
	}

	totalEvents := len(events)
	suppressed := 0

	for _, ev := range events {
		d, err := eng.Evaluate(ctx, ev)
		if err != nil {
			t.Fatalf("evaluate %s: %v", ev.EventID, err)
		}
		if d.Suppress {
			suppressed++
		}
	}

	notificationsSent := totalEvents - suppressed
	reductionPct := float64(suppressed) / float64(totalEvents) * 100

	t.Logf("Total events: %d, Suppressed: %d, Sent: %d, Reduction: %.1f%%",
		totalEvents, suppressed, notificationsSent, reductionPct)

	if reductionPct < 50.0 {
		t.Errorf("expected >= 50%% reduction, got %.1f%% (suppressed %d of %d)",
			reductionPct, suppressed, totalEvents)
	}
}

func TestPruneClusters(t *testing.T) {
	eng := newEngine(t, func(c *suppression.Config) {
		c.ClusterWindow = 2 * time.Minute
	})
	ctx := context.Background()

	base := time.Date(2026, 4, 10, 14, 0, 0, 0, time.UTC)

	// Create a cluster.
	eng.Evaluate(ctx, suppression.Event{
		EventID: "e1", TenantID: "t1", CameraID: "cam1",
		EventType: "motion", Timestamp: base,
	})

	// Prune at base + 5 min should clear the cluster.
	eng.PruneClusters(base.Add(5 * time.Minute))

	// Next event for same camera should start a new cluster (not be suppressed).
	d, err := eng.Evaluate(ctx, suppression.Event{
		EventID: "e2", TenantID: "t1", CameraID: "cam1",
		EventType: "motion", Timestamp: base.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if d.Suppress {
		t.Error("event after prune should start a new cluster, not be suppressed")
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()

	base := time.Date(2026, 4, 10, 14, 0, 0, 0, time.UTC)

	// Tenant 1 starts a cluster.
	eng.Evaluate(ctx, suppression.Event{
		EventID: "e1", TenantID: "t1", CameraID: "cam1",
		EventType: "motion", Timestamp: base,
	})

	// Tenant 2 same camera ID should NOT be suppressed (different tenant).
	d, err := eng.Evaluate(ctx, suppression.Event{
		EventID: "e2", TenantID: "t2", CameraID: "cam1",
		EventType: "motion", Timestamp: base.Add(1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if d.Suppress {
		t.Error("different tenant should not be affected by other tenant's cluster")
	}
}
