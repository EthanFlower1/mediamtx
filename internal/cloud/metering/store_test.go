package metering_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/bluenviron/mediamtx/internal/cloud/metering"
)

func openSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newStore(t *testing.T) *metering.Store {
	t.Helper()
	db := openSQLite(t)
	s, err := metering.NewStore(db, metering.DialectSQLite)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := s.ApplyStubSchema(context.Background()); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	return s
}

func mustRecord(t *testing.T, s *metering.Store, e metering.Event) {
	t.Helper()
	if err := s.Record(context.Background(), e); err != nil {
		t.Fatalf("record: %v", err)
	}
}

func TestStore_RecordAndList(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	mustRecord(t, s, metering.Event{
		TenantID: "tenant-a", Metric: metering.MetricCameraHours,
		Value: 2.5, Timestamp: base,
	})
	mustRecord(t, s, metering.Event{
		TenantID: "tenant-a", Metric: metering.MetricCameraHours,
		Value: 1.0, Timestamp: base.Add(time.Hour),
	})
	mustRecord(t, s, metering.Event{
		TenantID: "tenant-a", Metric: metering.MetricRecordingBytes,
		Value: 1024, Timestamp: base,
	})

	got, err := s.ListEvents(ctx, metering.QueryFilter{TenantID: "tenant-a"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 events, got %d", len(got))
	}

	// Filter by metric.
	hrs, err := s.ListEvents(ctx, metering.QueryFilter{
		TenantID: "tenant-a", Metric: metering.MetricCameraHours,
	})
	if err != nil {
		t.Fatalf("list hours: %v", err)
	}
	if len(hrs) != 2 {
		t.Errorf("want 2 camera_hours events, got %d", len(hrs))
	}
}

func TestStore_TenantIsolation_Seam4(t *testing.T) {
	// The seam #4 contract: no cross-tenant read ever succeeds. This test
	// wires two tenants into the same store, writes events for both, and
	// asserts every query path returns only the requesting tenant's rows.
	s := newStore(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	mustRecord(t, s, metering.Event{
		TenantID: "tenant-a", Metric: metering.MetricCameraHours,
		Value: 10, Timestamp: base,
	})
	mustRecord(t, s, metering.Event{
		TenantID: "tenant-b", Metric: metering.MetricCameraHours,
		Value: 999, Timestamp: base,
	})

	a, err := s.ListEvents(ctx, metering.QueryFilter{TenantID: "tenant-a"})
	if err != nil {
		t.Fatalf("list a: %v", err)
	}
	if len(a) != 1 || a[0].TenantID != "tenant-a" || a[0].Value != 10 {
		t.Errorf("tenant-a leaked: %+v", a)
	}

	b, err := s.ListEvents(ctx, metering.QueryFilter{TenantID: "tenant-b"})
	if err != nil {
		t.Fatalf("list b: %v", err)
	}
	if len(b) != 1 || b[0].TenantID != "tenant-b" || b[0].Value != 999 {
		t.Errorf("tenant-b leaked: %+v", b)
	}

	// Missing tenant is fail-closed.
	if _, err := s.ListEvents(ctx, metering.QueryFilter{}); !errors.Is(err, metering.ErrMissingTenant) {
		t.Errorf("want ErrMissingTenant, got %v", err)
	}
	if err := s.Record(ctx, metering.Event{Metric: metering.MetricCameraHours, Value: 1}); !errors.Is(err, metering.ErrMissingTenant) {
		t.Errorf("record want ErrMissingTenant, got %v", err)
	}
}

func TestStore_RecordValidation(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	cases := []struct {
		name string
		e    metering.Event
		want error
	}{
		{
			name: "unknown metric",
			e:    metering.Event{TenantID: "t", Metric: metering.Metric("bogus"), Value: 1},
			want: metering.ErrUnknownMetric,
		},
		{
			name: "negative value",
			e:    metering.Event{TenantID: "t", Metric: metering.MetricCameraHours, Value: -1},
			want: metering.ErrNegativeValue,
		},
		{
			name: "missing tenant",
			e:    metering.Event{Metric: metering.MetricCameraHours, Value: 1},
			want: metering.ErrMissingTenant,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := s.Record(ctx, tc.e); !errors.Is(err, tc.want) {
				t.Errorf("want %v, got %v", tc.want, err)
			}
		})
	}
}

func TestStore_TimeFilter(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	mustRecord(t, s, metering.Event{TenantID: "t", Metric: metering.MetricCameraHours, Value: 1, Timestamp: base})
	mustRecord(t, s, metering.Event{TenantID: "t", Metric: metering.MetricCameraHours, Value: 2, Timestamp: base.Add(1 * time.Hour)})
	mustRecord(t, s, metering.Event{TenantID: "t", Metric: metering.MetricCameraHours, Value: 3, Timestamp: base.Add(2 * time.Hour)})

	got, err := s.ListEvents(ctx, metering.QueryFilter{
		TenantID: "t",
		Since:    base.Add(30 * time.Minute),
		Until:    base.Add(90 * time.Minute),
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Value != 2 {
		t.Errorf("time filter wrong: %+v", got)
	}
}
