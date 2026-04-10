package metering_test

import (
	"context"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/metering"
)

type captureReporter struct {
	got []metering.Aggregate
}

func (c *captureReporter) ReportAggregate(_ context.Context, agg metering.Aggregate) error {
	c.got = append(c.got, agg)
	return nil
}

func TestReportPeriod_TenantScoped(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()
	end := base.Add(24 * time.Hour)

	mustRecord(t, s, metering.Event{TenantID: "tenant-a", Metric: metering.MetricCameraHours, Value: 1, Timestamp: base})
	mustRecord(t, s, metering.Event{TenantID: "tenant-b", Metric: metering.MetricCameraHours, Value: 99, Timestamp: base})

	if err := metering.NewAggregator(s).Run(ctx, base, end, []string{"tenant-a", "tenant-b"}); err != nil {
		t.Fatalf("aggregator: %v", err)
	}

	cap := &captureReporter{}
	if err := metering.ReportPeriod(ctx, s, cap, metering.QueryFilter{TenantID: "tenant-a"}); err != nil {
		t.Fatalf("report: %v", err)
	}
	if len(cap.got) != 1 || cap.got[0].TenantID != "tenant-a" {
		t.Errorf("reporter saw cross-tenant data: %+v", cap.got)
	}
}

func TestFanoutReporter(t *testing.T) {
	a := &captureReporter{}
	b := &captureReporter{}
	f := metering.NewFanoutReporter(a, b)
	agg := metering.Aggregate{
		TenantID: "t", Metric: metering.MetricCameraHours, Sum: 1, Max: 1, SnapshotCount: 1,
	}
	if err := f.ReportAggregate(context.Background(), agg); err != nil {
		t.Fatalf("fanout: %v", err)
	}
	if len(a.got) != 1 || len(b.got) != 1 {
		t.Errorf("fanout missed a target: a=%v b=%v", a.got, b.got)
	}
}
