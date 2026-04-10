package metering_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/metering"
)

func TestAggregator_RollupSumMaxCount(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()
	end := base.Add(24 * time.Hour)

	// Three events for tenant-a / camera_hours: values 1, 2, 4 -> sum=7 max=4 count=3
	mustRecord(t, s, metering.Event{TenantID: "tenant-a", Metric: metering.MetricCameraHours, Value: 1, Timestamp: base})
	mustRecord(t, s, metering.Event{TenantID: "tenant-a", Metric: metering.MetricCameraHours, Value: 2, Timestamp: base.Add(time.Hour)})
	mustRecord(t, s, metering.Event{TenantID: "tenant-a", Metric: metering.MetricCameraHours, Value: 4, Timestamp: base.Add(2 * time.Hour)})
	// One bytes event -> sum=1024 max=1024 count=1
	mustRecord(t, s, metering.Event{TenantID: "tenant-a", Metric: metering.MetricRecordingBytes, Value: 1024, Timestamp: base})
	// Cross-tenant noise — must not appear in tenant-a's aggregates.
	mustRecord(t, s, metering.Event{TenantID: "tenant-b", Metric: metering.MetricCameraHours, Value: 999, Timestamp: base})

	agg := metering.NewAggregator(s)
	if err := agg.Run(ctx, base, end, []string{"tenant-a"}); err != nil {
		t.Fatalf("run: %v", err)
	}

	rows, err := s.ListAggregates(ctx, metering.QueryFilter{TenantID: "tenant-a"})
	if err != nil {
		t.Fatalf("list aggregates: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 aggregate rows, got %d: %+v", len(rows), rows)
	}

	byMetric := map[metering.Metric]metering.Aggregate{}
	for _, r := range rows {
		byMetric[r.Metric] = r
	}
	hrs := byMetric[metering.MetricCameraHours]
	if hrs.Sum != 7 || hrs.Max != 4 || hrs.SnapshotCount != 3 {
		t.Errorf("camera_hours rollup wrong: %+v", hrs)
	}
	bytesAgg := byMetric[metering.MetricRecordingBytes]
	if bytesAgg.Sum != 1024 || bytesAgg.Max != 1024 || bytesAgg.SnapshotCount != 1 {
		t.Errorf("bytes rollup wrong: %+v", bytesAgg)
	}

	// Cross-tenant isolation: tenant-b aggregates must NOT include
	// tenant-a's rollup even though Run only covered tenant-a.
	bRows, err := s.ListAggregates(ctx, metering.QueryFilter{TenantID: "tenant-b"})
	if err != nil {
		t.Fatalf("list b aggregates: %v", err)
	}
	if len(bRows) != 0 {
		t.Errorf("tenant-b has stray aggregates: %+v", bRows)
	}
}

func TestAggregator_Idempotent(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()
	end := base.Add(24 * time.Hour)

	mustRecord(t, s, metering.Event{TenantID: "t", Metric: metering.MetricCameraHours, Value: 5, Timestamp: base})

	agg := metering.NewAggregator(s)
	for i := 0; i < 3; i++ {
		if err := agg.Run(ctx, base, end, []string{"t"}); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}

	rows, err := s.ListAggregates(ctx, metering.QueryFilter{TenantID: "t"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row after idempotent re-run, got %d", len(rows))
	}
	if rows[0].Sum != 5 || rows[0].SnapshotCount != 1 {
		t.Errorf("unexpected row after re-run: %+v", rows[0])
	}
}

func TestAggregator_InvalidPeriod(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	agg := metering.NewAggregator(s)
	err := agg.Run(ctx, base, base, []string{"t"})
	if !errors.Is(err, metering.ErrInvalidPeriod) {
		t.Errorf("want ErrInvalidPeriod, got %v", err)
	}
}
