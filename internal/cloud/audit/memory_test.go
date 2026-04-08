package audit_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
)

func mustRecord(t *testing.T, r audit.Recorder, e audit.Entry) {
	t.Helper()
	if err := r.Record(context.Background(), e); err != nil {
		t.Fatalf("record: %v", err)
	}
}

func baseEntry(tenant, actor, action string, ts time.Time) audit.Entry {
	return audit.Entry{
		TenantID:     tenant,
		ActorUserID:  actor,
		ActorAgent:   audit.AgentCloud,
		Action:       action,
		ResourceType: "camera",
		ResourceID:   "cam-1",
		Result:       audit.ResultAllow,
		IPAddress:    "1.2.3.4",
		UserAgent:    "test",
		RequestID:    "req-1",
		Timestamp:    ts,
	}
}

func TestMemoryRecorder_RoundTrip(t *testing.T) {
	r := audit.NewMemoryRecorder()
	ctx := context.Background()
	ts := time.Unix(1_700_000_000, 0).UTC()

	mustRecord(t, r, baseEntry("tenant-a", "user-1", "cameras.add", ts))
	mustRecord(t, r, baseEntry("tenant-a", "user-1", "cameras.edit", ts.Add(time.Second)))

	got, err := r.Query(ctx, audit.QueryFilter{TenantID: "tenant-a"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	// Newest first.
	if got[0].Action != "cameras.edit" {
		t.Errorf("ordering: want cameras.edit first, got %s", got[0].Action)
	}
}

func TestMemoryRecorder_TenantIsolation_Seam4(t *testing.T) {
	r := audit.NewMemoryRecorder()
	ctx := context.Background()
	ts := time.Unix(1_700_000_000, 0).UTC()

	mustRecord(t, r, baseEntry("tenant-a", "alice", "cameras.add", ts))
	mustRecord(t, r, baseEntry("tenant-b", "bob", "cameras.add", ts))

	// Alice queries her tenant and must see only her row.
	got, err := r.Query(ctx, audit.QueryFilter{TenantID: "tenant-a"})
	if err != nil || len(got) != 1 || got[0].ActorUserID != "alice" {
		t.Fatalf("tenant-a leakage: %+v err=%v", got, err)
	}
	// Bob queries his tenant and must see only his row.
	got, err = r.Query(ctx, audit.QueryFilter{TenantID: "tenant-b"})
	if err != nil || len(got) != 1 || got[0].ActorUserID != "bob" {
		t.Fatalf("tenant-b leakage: %+v err=%v", got, err)
	}
	// Empty tenant filter is refused.
	if _, err := r.Query(ctx, audit.QueryFilter{}); err == nil {
		t.Error("empty-tenant query was allowed")
	}
}

func TestMemoryRecorder_ChaosCrossTenant(t *testing.T) {
	// Chaos test: register 50 entries spread across 10 tenants and verify
	// that any query for tenant X returns ONLY tenant X entries. This is
	// the Seam #4 guarantee expressed as an invariant over a populated
	// store.
	r := audit.NewMemoryRecorder()
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	tenants := []string{"t0", "t1", "t2", "t3", "t4", "t5", "t6", "t7", "t8", "t9"}
	for i := 0; i < 50; i++ {
		tenant := tenants[i%len(tenants)]
		mustRecord(t, r, baseEntry(tenant, "u", "cameras.add", base.Add(time.Duration(i)*time.Second)))
	}

	for _, tenant := range tenants {
		got, err := r.Query(ctx, audit.QueryFilter{TenantID: tenant})
		if err != nil {
			t.Fatalf("query %s: %v", tenant, err)
		}
		if len(got) != 5 {
			t.Errorf("%s: want 5, got %d", tenant, len(got))
		}
		for _, e := range got {
			if e.TenantID != tenant {
				t.Errorf("LEAK: tenant %s saw entry from %s", tenant, e.TenantID)
			}
		}
	}
}

func TestMemoryRecorder_Impersonation(t *testing.T) {
	r := audit.NewMemoryRecorder()
	ctx := context.Background()
	ts := time.Unix(1_700_000_000, 0).UTC()

	integrator := "integrator-42"
	target := "tenant-x"
	e := baseEntry(target, "alice@integrator", "cameras.add", ts)
	e.ActorAgent = audit.AgentIntegrator
	e.ImpersonatingUserID = &integrator
	e.ImpersonatedTenantID = &target // target == home of data, integrator's home differs
	mustRecord(t, r, e)

	// The target tenant sees it by default.
	got, err := r.Query(ctx, audit.QueryFilter{TenantID: target})
	if err != nil || len(got) != 1 {
		t.Fatalf("target query: got %+v err %v", got, err)
	}
	if got[0].ImpersonatingUserID == nil || *got[0].ImpersonatingUserID != integrator {
		t.Error("impersonating user not preserved")
	}

	// The integrator can audit its own staff by querying its home tenant
	// with IncludeImpersonatedTenant. We seed that entry first.
	e2 := baseEntry("integrator-home", "alice@integrator", "integrator.access", ts.Add(time.Second))
	mustRecord(t, r, e2)

	got, err = r.Query(ctx, audit.QueryFilter{
		TenantID:                  "integrator-home",
		IncludeImpersonatedTenant: true,
	})
	if err != nil {
		t.Fatalf("integrator query: %v", err)
	}
	// Should see the direct entry on integrator-home. (The impersonation
	// row belongs to tenant-x, not integrator-home, so it doesn't show up
	// unless the integrator sets ImpersonatedTenantID == "integrator-home"
	// — this test documents that nuance.)
	if len(got) != 1 {
		t.Fatalf("integrator home: want 1, got %d", len(got))
	}
}

func TestMemoryRecorder_Validation(t *testing.T) {
	r := audit.NewMemoryRecorder()
	ctx := context.Background()

	cases := []audit.Entry{
		{}, // missing everything
		{TenantID: "t", ActorAgent: "bogus"},
		{TenantID: "t", ActorUserID: "u", ActorAgent: audit.AgentCloud, Action: "a", ResourceType: "r", Result: audit.ResultError, Timestamp: time.Now()},
	}
	for i, c := range cases {
		if err := r.Record(ctx, c); err == nil {
			t.Errorf("case %d: wanted validation error", i)
		}
	}
}

func TestMemoryRecorder_Filters(t *testing.T) {
	r := audit.NewMemoryRecorder()
	ctx := context.Background()
	ts := time.Unix(1_700_000_000, 0).UTC()

	mustRecord(t, r, baseEntry("t", "alice", "cameras.add", ts))
	mustRecord(t, r, baseEntry("t", "alice", "cameras.edit", ts.Add(time.Second)))
	e := baseEntry("t", "bob", "users.create", ts.Add(2*time.Second))
	e.Result = audit.ResultDeny
	mustRecord(t, r, e)

	// Action pattern.
	got, err := r.Query(ctx, audit.QueryFilter{TenantID: "t", ActionPattern: "cameras.*"})
	if err != nil {
		t.Fatalf("pattern query: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("pattern: want 2, got %d", len(got))
	}

	// Result filter.
	got, _ = r.Query(ctx, audit.QueryFilter{TenantID: "t", Result: audit.ResultDeny})
	if len(got) != 1 || got[0].ActorUserID != "bob" {
		t.Errorf("result filter broken: %+v", got)
	}

	// Time range.
	got, _ = r.Query(ctx, audit.QueryFilter{TenantID: "t", Since: ts.Add(time.Second), Until: ts.Add(time.Second)})
	if len(got) != 1 || got[0].Action != "cameras.edit" {
		t.Errorf("time filter broken: %+v", got)
	}
}

func TestExport_CSV(t *testing.T) {
	r := audit.NewMemoryRecorder()
	ts := time.Unix(1_700_000_000, 0).UTC()
	mustRecord(t, r, baseEntry("t", "alice", "cameras.add", ts))
	mustRecord(t, r, baseEntry("t", "alice", "cameras.edit", ts.Add(time.Second)))

	var buf bytes.Buffer
	if err := r.Export(context.Background(), audit.QueryFilter{TenantID: "t"}, audit.ExportCSV, &buf); err != nil {
		t.Fatalf("export: %v", err)
	}
	records, err := csv.NewReader(strings.NewReader(buf.String())).ReadAll()
	if err != nil {
		t.Fatalf("csv parse: %v", err)
	}
	if len(records) != 3 { // header + 2 rows
		t.Errorf("want 3 rows (with header), got %d", len(records))
	}
	if records[0][0] != "id" {
		t.Errorf("missing header row: %v", records[0])
	}
}

func TestExport_JSON(t *testing.T) {
	r := audit.NewMemoryRecorder()
	ts := time.Unix(1_700_000_000, 0).UTC()
	mustRecord(t, r, baseEntry("t", "alice", "cameras.add", ts))

	var buf bytes.Buffer
	if err := r.Export(context.Background(), audit.QueryFilter{TenantID: "t"}, audit.ExportJSON, &buf); err != nil {
		t.Fatalf("export: %v", err)
	}
	dec := json.NewDecoder(&buf)
	var got audit.Entry
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if got.Action != "cameras.add" {
		t.Errorf("round-trip failed: %+v", got)
	}
}

func TestMemoryRecorder_Cursor(t *testing.T) {
	r := audit.NewMemoryRecorder()
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

	// Bad cursor.
	if _, err := r.Query(ctx, audit.QueryFilter{TenantID: "t", Cursor: "missing"}); !errors.Is(err, audit.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}
