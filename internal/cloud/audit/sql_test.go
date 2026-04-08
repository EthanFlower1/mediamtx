package audit_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
)

func openSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newSQLRecorder(t *testing.T) *audit.SQLRecorder {
	t.Helper()
	db := openSQLite(t)
	r := audit.NewSQLRecorder(db, audit.DialectSQLite)
	if err := r.ApplyStubSchema(context.Background()); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return r
}

func TestSQLRecorder_RoundTrip(t *testing.T) {
	r := newSQLRecorder(t)
	ctx := context.Background()
	ts := time.Unix(1_700_000_000, 0).UTC()

	mustRecord(t, r, baseEntry("tenant-a", "alice", "cameras.add", ts))
	mustRecord(t, r, baseEntry("tenant-a", "alice", "cameras.edit", ts.Add(time.Second)))

	got, err := r.Query(ctx, audit.QueryFilter{TenantID: "tenant-a"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	if got[0].Action != "cameras.edit" {
		t.Errorf("ordering broken: %+v", got)
	}
}

func TestSQLRecorder_TenantIsolation_Seam4(t *testing.T) {
	r := newSQLRecorder(t)
	ctx := context.Background()
	ts := time.Unix(1_700_000_000, 0).UTC()

	mustRecord(t, r, baseEntry("tenant-a", "alice", "cameras.add", ts))
	mustRecord(t, r, baseEntry("tenant-b", "bob", "cameras.add", ts))

	got, err := r.Query(ctx, audit.QueryFilter{TenantID: "tenant-a"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 1 || got[0].TenantID != "tenant-a" {
		t.Errorf("leak: %+v", got)
	}
	got, err = r.Query(ctx, audit.QueryFilter{TenantID: "tenant-b"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 1 || got[0].TenantID != "tenant-b" {
		t.Errorf("leak: %+v", got)
	}

	if _, err := r.Query(ctx, audit.QueryFilter{}); err == nil {
		t.Error("empty tenant filter must be rejected")
	}
}

func TestSQLRecorder_ChaosCrossTenant(t *testing.T) {
	r := newSQLRecorder(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	tenants := []string{"t0", "t1", "t2", "t3", "t4"}
	for i := 0; i < 50; i++ {
		tenant := tenants[i%len(tenants)]
		mustRecord(t, r, baseEntry(tenant, "u", "cameras.add", base.Add(time.Duration(i)*time.Second)))
	}

	for _, tenant := range tenants {
		got, err := r.Query(ctx, audit.QueryFilter{TenantID: tenant})
		if err != nil {
			t.Fatalf("query %s: %v", tenant, err)
		}
		if len(got) != 10 {
			t.Errorf("%s: want 10, got %d", tenant, len(got))
		}
		for _, e := range got {
			if e.TenantID != tenant {
				t.Errorf("LEAK: %s saw %s", tenant, e.TenantID)
			}
		}
	}
}

func TestSQLRecorder_Impersonation_CrossLookup(t *testing.T) {
	r := newSQLRecorder(t)
	ctx := context.Background()
	ts := time.Unix(1_700_000_000, 0).UTC()

	integratorHome := "integrator-home"
	target := "tenant-x"

	e := baseEntry(target, "alice@integrator", "cameras.add", ts)
	e.ActorAgent = audit.AgentIntegrator
	e.ImpersonatingUserID = &integratorHome
	e.ImpersonatedTenantID = &integratorHome
	mustRecord(t, r, e)

	// The *target tenant* sees the record under its own scope.
	got, err := r.Query(ctx, audit.QueryFilter{TenantID: target})
	if err != nil || len(got) != 1 {
		t.Fatalf("target: %+v err=%v", got, err)
	}

	// The integrator's home tenant can audit its staff's cross-tenant work
	// by opting in to IncludeImpersonatedTenant.
	got, err = r.Query(ctx, audit.QueryFilter{
		TenantID:                  integratorHome,
		IncludeImpersonatedTenant: true,
	})
	if err != nil {
		t.Fatalf("integrator: %v", err)
	}
	if len(got) != 1 || got[0].ActorUserID != "alice@integrator" {
		t.Errorf("integrator home audit broken: %+v", got)
	}
}

func TestSQLRecorder_Filters(t *testing.T) {
	r := newSQLRecorder(t)
	ctx := context.Background()
	ts := time.Unix(1_700_000_000, 0).UTC()

	mustRecord(t, r, baseEntry("t", "alice", "cameras.add", ts))
	mustRecord(t, r, baseEntry("t", "alice", "cameras.edit", ts.Add(time.Second)))
	deny := baseEntry("t", "bob", "users.create", ts.Add(2*time.Second))
	deny.Result = audit.ResultDeny
	mustRecord(t, r, deny)

	got, err := r.Query(ctx, audit.QueryFilter{TenantID: "t", ActionPattern: "cameras.*"})
	if err != nil || len(got) != 2 {
		t.Errorf("pattern: %+v err=%v", got, err)
	}

	got, _ = r.Query(ctx, audit.QueryFilter{TenantID: "t", Result: audit.ResultDeny})
	if len(got) != 1 || got[0].ActorUserID != "bob" {
		t.Errorf("deny filter: %+v", got)
	}

	got, _ = r.Query(ctx, audit.QueryFilter{TenantID: "t", ActorUserID: "alice"})
	if len(got) != 2 {
		t.Errorf("actor filter: %+v", got)
	}

	got, _ = r.Query(ctx, audit.QueryFilter{TenantID: "t", Limit: 1})
	if len(got) != 1 {
		t.Errorf("limit: %+v", got)
	}
}

func TestSQLRecorder_Cursor(t *testing.T) {
	r := newSQLRecorder(t)
	ctx := context.Background()
	ts := time.Unix(1_700_000_000, 0).UTC()
	for i := 0; i < 5; i++ {
		mustRecord(t, r, baseEntry("t", "alice", "cameras.add", ts.Add(time.Duration(i)*time.Second)))
	}

	page1, err := r.Query(ctx, audit.QueryFilter{TenantID: "t", Limit: 2})
	if err != nil || len(page1) != 2 {
		t.Fatalf("page1: %+v err=%v", page1, err)
	}
	page2, err := r.Query(ctx, audit.QueryFilter{TenantID: "t", Limit: 2, Cursor: page1[len(page1)-1].ID})
	if err != nil || len(page2) != 2 {
		t.Fatalf("page2: %+v err=%v", page2, err)
	}
	if page1[1].ID == page2[0].ID {
		t.Error("cursor did not advance")
	}
}

func TestRetentionStore(t *testing.T) {
	db := openSQLite(t)
	r := audit.NewSQLRecorder(db, audit.DialectSQLite)
	if err := r.ApplyStubSchema(context.Background()); err != nil {
		t.Fatalf("schema: %v", err)
	}

	rs := audit.NewRetentionStore(db, audit.DialectSQLite)
	ctx := context.Background()

	// Default when nothing set.
	got, err := rs.Get(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("get default: %v", err)
	}
	if got != audit.DefaultRetention {
		t.Errorf("default: want %v, got %v", audit.DefaultRetention, got)
	}

	// Override.
	if err := rs.Set(ctx, "tenant-a", 10*365*24*time.Hour); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err = rs.Get(ctx, "tenant-a")
	if err != nil || got != 10*365*24*time.Hour {
		t.Errorf("override: %v %v", got, err)
	}
}

func TestPartitionName(t *testing.T) {
	ts := time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)
	got := audit.PartitionName("audit_log", ts)
	want := "audit_log_2026_04"
	if got != want {
		t.Errorf("want %s, got %s", want, got)
	}
}

func TestPartitionManager_SQLite_NoOp(t *testing.T) {
	// Under the SQLite dialect the partition manager must be a no-op so
	// production code can call it unconditionally in tests.
	db := openSQLite(t)
	pm := audit.NewPartitionManager(db, audit.DialectSQLite)

	if err := pm.CreateNextMonthPartition(context.Background(), time.Now()); err != nil {
		t.Errorf("CreateNextMonthPartition: %v", err)
	}
	if err := pm.CreatePartitionForMonth(context.Background(), time.Now()); err != nil {
		t.Errorf("CreatePartitionForMonth: %v", err)
	}
	dropped, err := pm.DropExpiredPartitions(context.Background(), time.Now(), audit.DefaultRetention)
	if err != nil {
		t.Errorf("DropExpiredPartitions: %v", err)
	}
	if len(dropped) != 0 {
		t.Errorf("want 0 dropped, got %d", len(dropped))
	}
}
