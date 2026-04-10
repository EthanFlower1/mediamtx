package statuspage_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/statuspage"
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

func newService(t *testing.T) *statuspage.Service {
	t.Helper()
	db := openTestDB(t)
	svc, err := statuspage.NewService(statuspage.Config{
		DB:    db,
		IDGen: testIDGen,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func TestUpsertAndListHealthChecks(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	hc := statuspage.HealthCheck{
		TenantID:    "tenant-1",
		ServiceName: "cloud_api",
		DisplayName: "Cloud API",
		Status:      statuspage.StatusOperational,
		Metadata:    "{}",
		Enabled:     true,
	}
	created, err := svc.UpsertHealthCheck(ctx, hc)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if created.CheckID == "" {
		t.Fatal("expected check_id to be set")
	}

	checks, err := svc.ListHealthChecks(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if checks[0].ServiceName != "cloud_api" {
		t.Errorf("expected cloud_api, got %s", checks[0].ServiceName)
	}
}

func TestUpdateServiceStatus(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	hc, _ := svc.UpsertHealthCheck(ctx, statuspage.HealthCheck{
		TenantID:    "tenant-1",
		ServiceName: "recording",
		DisplayName: "Recording Pipeline",
		Status:      statuspage.StatusOperational,
		Metadata:    "{}",
		Enabled:     true,
	})

	if err := svc.UpdateServiceStatus(ctx, "tenant-1", hc.CheckID, statuspage.StatusDegraded); err != nil {
		t.Fatalf("update: %v", err)
	}

	checks, _ := svc.ListHealthChecks(ctx, "tenant-1")
	if len(checks) != 1 || checks[0].Status != statuspage.StatusDegraded {
		t.Errorf("expected degraded, got %v", checks)
	}
}

func TestUpdateServiceStatusNotFound(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	err := svc.UpdateServiceStatus(ctx, "tenant-1", "nonexistent", statuspage.StatusDegraded)
	if err != statuspage.ErrCheckNotFound {
		t.Errorf("expected ErrCheckNotFound, got %v", err)
	}
}

func TestDeleteHealthCheck(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	hc, _ := svc.UpsertHealthCheck(ctx, statuspage.HealthCheck{
		TenantID:    "tenant-1",
		ServiceName: "live_view",
		DisplayName: "Live View",
		Status:      statuspage.StatusOperational,
		Metadata:    "{}",
		Enabled:     true,
	})

	if err := svc.DeleteHealthCheck(ctx, "tenant-1", hc.CheckID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	err := svc.DeleteHealthCheck(ctx, "tenant-1", hc.CheckID)
	if err != statuspage.ErrCheckNotFound {
		t.Errorf("expected ErrCheckNotFound, got %v", err)
	}
}

func TestIncidentLifecycle(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	inc, err := svc.CreateIncident(ctx, statuspage.Incident{
		TenantID:         "tenant-1",
		Title:            "Cloud API degraded",
		Severity:         statuspage.SeverityMajor,
		Status:           statuspage.IncidentInvestigating,
		AffectedServices: `["cloud_api"]`,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if inc.IncidentID == "" {
		t.Fatal("expected incident_id")
	}

	// List active
	active, err := svc.ListActiveIncidents(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active, got %d", len(active))
	}

	// Post update: identified
	upd, err := svc.UpdateIncidentStatus(ctx, "tenant-1", inc.IncidentID, statuspage.IncidentIdentified, "Root cause identified: database connection pool exhausted")
	if err != nil {
		t.Fatalf("update to identified: %v", err)
	}
	if upd.UpdateID == "" {
		t.Fatal("expected update_id")
	}

	// Resolve
	_, err = svc.UpdateIncidentStatus(ctx, "tenant-1", inc.IncidentID, statuspage.IncidentResolved, "Connection pool increased, service restored")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// No longer active
	active, _ = svc.ListActiveIncidents(ctx, "tenant-1")
	if len(active) != 0 {
		t.Fatalf("expected 0 active after resolve, got %d", len(active))
	}

	// Updates list
	updates, err := svc.ListIncidentUpdates(ctx, "tenant-1", inc.IncidentID)
	if err != nil {
		t.Fatalf("list updates: %v", err)
	}
	if len(updates) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(updates))
	}
}

func TestUpdateIncidentNotFound(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	_, err := svc.UpdateIncidentStatus(ctx, "tenant-1", "nonexistent", statuspage.IncidentResolved, "nope")
	if err != statuspage.ErrIncidentNotFound {
		t.Errorf("expected ErrIncidentNotFound, got %v", err)
	}
}

func TestGetStatusSummary(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	// Two services: one operational, one degraded
	svc.UpsertHealthCheck(ctx, statuspage.HealthCheck{
		TenantID: "tenant-1", ServiceName: "cloud_api", DisplayName: "Cloud API",
		Status: statuspage.StatusOperational, Metadata: "{}", Enabled: true,
	})
	svc.UpsertHealthCheck(ctx, statuspage.HealthCheck{
		TenantID: "tenant-1", ServiceName: "recording", DisplayName: "Recording",
		Status: statuspage.StatusDegraded, Metadata: "{}", Enabled: true,
	})

	// One active incident
	svc.CreateIncident(ctx, statuspage.Incident{
		TenantID: "tenant-1", Title: "Recording slow", Severity: statuspage.SeverityMinor,
		Status: statuspage.IncidentInvestigating, AffectedServices: `["recording"]`,
	})

	summary, err := svc.GetStatusSummary(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.OverallStatus != statuspage.StatusDegraded {
		t.Errorf("expected degraded overall, got %s", summary.OverallStatus)
	}
	if len(summary.Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(summary.Services))
	}
	if len(summary.ActiveIncidents) != 1 {
		t.Errorf("expected 1 active incident, got %d", len(summary.ActiveIncidents))
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	svc.UpsertHealthCheck(ctx, statuspage.HealthCheck{
		TenantID: "tenant-1", ServiceName: "cloud_api", DisplayName: "Cloud API",
		Status: statuspage.StatusOperational, Metadata: "{}", Enabled: true,
	})
	svc.UpsertHealthCheck(ctx, statuspage.HealthCheck{
		TenantID: "tenant-2", ServiceName: "recording", DisplayName: "Recording",
		Status: statuspage.StatusMajorOut, Metadata: "{}", Enabled: true,
	})

	ch1, _ := svc.ListHealthChecks(ctx, "tenant-1")
	ch2, _ := svc.ListHealthChecks(ctx, "tenant-2")

	if len(ch1) != 1 || ch1[0].ServiceName != "cloud_api" {
		t.Errorf("tenant-1 should only see cloud_api")
	}
	if len(ch2) != 1 || ch2[0].ServiceName != "recording" {
		t.Errorf("tenant-2 should only see recording")
	}

	// Incident isolation
	svc.CreateIncident(ctx, statuspage.Incident{
		TenantID: "tenant-1", Title: "Tenant 1 issue", Severity: statuspage.SeverityMinor,
		Status: statuspage.IncidentInvestigating, AffectedServices: "[]",
	})
	inc1, _ := svc.ListActiveIncidents(ctx, "tenant-1")
	inc2, _ := svc.ListActiveIncidents(ctx, "tenant-2")
	if len(inc1) != 1 {
		t.Errorf("tenant-1 should have 1 incident")
	}
	if len(inc2) != 0 {
		t.Errorf("tenant-2 should have 0 incidents")
	}
}
