package behavioral_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/behavioral"
	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
)

// -----------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------

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

// seedTenantAndCamera inserts the minimum rows needed to satisfy the FK
// constraints on behavioral_config (customer_tenants → cameras).
func seedTenantAndCamera(t *testing.T, db *clouddb.DB, tenantID, cameraID string) {
	t.Helper()
	ctx := context.Background()

	// customer_tenants
	_, err := db.ExecContext(ctx,
		`INSERT INTO customer_tenants (id, display_name, billing_mode, status, region)
		 VALUES (?, ?, 'direct', 'active', 'us-east-2')`,
		tenantID, "Test Tenant",
	)
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	// on_prem_directories (needed as FK for cameras)
	dirID := tenantID + "-dir"
	_, err = db.ExecContext(ctx,
		`INSERT INTO on_prem_directories
		    (id, customer_tenant_id, display_name, site_label, deployment_mode, status, region)
		 VALUES (?, ?, 'Test Dir', 'test', 'cloud_connected', 'online', 'us-east-2')`,
		dirID, tenantID,
	)
	if err != nil {
		t.Fatalf("seed directory: %v", err)
	}

	// cameras
	_, err = db.ExecContext(ctx,
		`INSERT INTO cameras
		    (id, tenant_id, directory_id, display_name, ai_features, lpr_enabled, status, region)
		 VALUES (?, ?, ?, 'Test Camera', '{}', 0, 'unconfigured', 'us-east-2')`,
		cameraID, tenantID, dirID,
	)
	if err != nil {
		t.Fatalf("seed camera: %v", err)
	}
}

// -----------------------------------------------------------------------
// Store tests
// -----------------------------------------------------------------------

func TestStore_UpsertAndGet(t *testing.T) {
	db := openTestDB(t)
	tenantID := "tenant-a"
	cameraID := "cam-1"
	seedTenantAndCamera(t, db, tenantID, cameraID)

	store := behavioral.NewStore(db)
	ctx := context.Background()

	cfg := behavioral.Config{
		TenantID:     tenantID,
		CameraID:     cameraID,
		DetectorType: behavioral.DetectorLoitering,
		Params:       `{"roi_polygon":[[0,0],[1,0],[1,1],[0,1]],"threshold_seconds":30}`,
		Enabled:      true,
	}
	if err := store.Upsert(ctx, cfg); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := store.Get(ctx, tenantID, cameraID, behavioral.DetectorLoitering)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.TenantID != tenantID {
		t.Errorf("TenantID = %q, want %q", got.TenantID, tenantID)
	}
	if got.CameraID != cameraID {
		t.Errorf("CameraID = %q, want %q", got.CameraID, cameraID)
	}
	if got.DetectorType != behavioral.DetectorLoitering {
		t.Errorf("DetectorType = %q, want %q", got.DetectorType, behavioral.DetectorLoitering)
	}
	if !got.Enabled {
		t.Error("Enabled should be true")
	}
}

func TestStore_UpsertUpdatesExisting(t *testing.T) {
	db := openTestDB(t)
	tenantID := "tenant-a"
	cameraID := "cam-1"
	seedTenantAndCamera(t, db, tenantID, cameraID)

	store := behavioral.NewStore(db)
	ctx := context.Background()

	cfg := behavioral.Config{
		TenantID:     tenantID,
		CameraID:     cameraID,
		DetectorType: behavioral.DetectorTailgating,
		Params:       `{"threshold_seconds":10}`,
		Enabled:      false,
	}
	if err := store.Upsert(ctx, cfg); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}

	cfg.Enabled = true
	cfg.Params = `{"threshold_seconds":20}`
	if err := store.Upsert(ctx, cfg); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}

	got, err := store.Get(ctx, tenantID, cameraID, behavioral.DetectorTailgating)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.Enabled {
		t.Error("Enabled should be true after update")
	}
}

func TestStore_List(t *testing.T) {
	db := openTestDB(t)
	tenantID := "tenant-a"
	cameraID := "cam-1"
	seedTenantAndCamera(t, db, tenantID, cameraID)

	store := behavioral.NewStore(db)
	ctx := context.Background()

	detectors := []behavioral.DetectorType{
		behavioral.DetectorLoitering,
		behavioral.DetectorLineCrossing,
		behavioral.DetectorROI,
	}
	for _, dt := range detectors {
		p := `{"roi_polygon":[[0,0],[1,0],[1,1]]}`
		if dt == behavioral.DetectorLineCrossing {
			p = `{"line_start":[0,0],"line_end":[1,1]}`
		}
		if err := store.Upsert(ctx, behavioral.Config{
			TenantID:     tenantID,
			CameraID:     cameraID,
			DetectorType: dt,
			Params:       p,
			Enabled:      true,
		}); err != nil {
			t.Fatalf("Upsert %s: %v", dt, err)
		}
	}

	list, err := store.List(ctx, tenantID, cameraID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("List returned %d rows, want 3", len(list))
	}
}

func TestStore_ListEmpty(t *testing.T) {
	db := openTestDB(t)
	tenantID := "tenant-a"
	cameraID := "cam-1"
	seedTenantAndCamera(t, db, tenantID, cameraID)

	store := behavioral.NewStore(db)
	list, err := store.List(context.Background(), tenantID, cameraID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}
}

func TestStore_Delete(t *testing.T) {
	db := openTestDB(t)
	tenantID := "tenant-a"
	cameraID := "cam-1"
	seedTenantAndCamera(t, db, tenantID, cameraID)

	store := behavioral.NewStore(db)
	ctx := context.Background()

	_ = store.Upsert(ctx, behavioral.Config{
		TenantID:     tenantID,
		CameraID:     cameraID,
		DetectorType: behavioral.DetectorFallDetection,
		Params:       "{}",
		Enabled:      true,
	})

	if err := store.Delete(ctx, tenantID, cameraID, behavioral.DetectorFallDetection); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := store.Get(ctx, tenantID, cameraID, behavioral.DetectorFallDetection)
	if err == nil {
		t.Error("expected ErrNotFound after delete")
	}
}

func TestStore_DeleteNotFound(t *testing.T) {
	db := openTestDB(t)
	tenantID := "tenant-a"
	cameraID := "cam-1"
	seedTenantAndCamera(t, db, tenantID, cameraID)

	store := behavioral.NewStore(db)
	err := store.Delete(context.Background(), tenantID, cameraID, behavioral.DetectorCrowdDensity)
	if err == nil {
		t.Error("expected ErrNotFound")
	}
}

// -----------------------------------------------------------------------
// Cross-tenant isolation tests
// -----------------------------------------------------------------------

func TestStore_CrossTenantIsolation_Get(t *testing.T) {
	db := openTestDB(t)
	seedTenantAndCamera(t, db, "tenant-a", "cam-1")
	seedTenantAndCamera(t, db, "tenant-b", "cam-2")

	store := behavioral.NewStore(db)
	ctx := context.Background()

	// Write config for tenant-a/cam-1.
	_ = store.Upsert(ctx, behavioral.Config{
		TenantID:     "tenant-a",
		CameraID:     "cam-1",
		DetectorType: behavioral.DetectorROI,
		Params:       `{"roi_polygon":[[0,0],[1,0],[1,1]]}`,
		Enabled:      true,
	})

	// tenant-b must not be able to read tenant-a's row via cam-1.
	_, err := store.Get(ctx, "tenant-b", "cam-1", behavioral.DetectorROI)
	if err == nil {
		t.Error("cross-tenant Get should return ErrNotFound")
	}
}

func TestStore_CrossTenantIsolation_List(t *testing.T) {
	db := openTestDB(t)
	seedTenantAndCamera(t, db, "tenant-a", "cam-1")
	seedTenantAndCamera(t, db, "tenant-b", "cam-2")

	store := behavioral.NewStore(db)
	ctx := context.Background()

	// Write config for tenant-a/cam-1.
	_ = store.Upsert(ctx, behavioral.Config{
		TenantID:     "tenant-a",
		CameraID:     "cam-1",
		DetectorType: behavioral.DetectorROI,
		Params:       `{"roi_polygon":[[0,0],[1,0],[1,1]]}`,
		Enabled:      true,
	})

	// tenant-b lists their camera — must see zero rows.
	list, err := store.List(ctx, "tenant-b", "cam-2")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("cross-tenant List returned %d rows, want 0", len(list))
	}
}

func TestStore_CrossTenantIsolation_Delete(t *testing.T) {
	db := openTestDB(t)
	seedTenantAndCamera(t, db, "tenant-a", "cam-1")
	seedTenantAndCamera(t, db, "tenant-b", "cam-2")

	store := behavioral.NewStore(db)
	ctx := context.Background()

	_ = store.Upsert(ctx, behavioral.Config{
		TenantID:     "tenant-a",
		CameraID:     "cam-1",
		DetectorType: behavioral.DetectorLoitering,
		Params:       `{"roi_polygon":[[0,0],[1,0],[1,1]],"threshold_seconds":5}`,
		Enabled:      true,
	})

	// tenant-b tries to delete tenant-a's row — must get ErrNotFound.
	err := store.Delete(ctx, "tenant-b", "cam-1", behavioral.DetectorLoitering)
	if err == nil {
		t.Error("cross-tenant Delete should return ErrNotFound")
	}

	// Original row must still exist.
	_, err = store.Get(ctx, "tenant-a", "cam-1", behavioral.DetectorLoitering)
	if err != nil {
		t.Errorf("original row should still exist: %v", err)
	}
}

// -----------------------------------------------------------------------
// Sentinel error tests
// -----------------------------------------------------------------------

func TestStore_GetRequiresTenantID(t *testing.T) {
	db := openTestDB(t)
	store := behavioral.NewStore(db)
	_, err := store.Get(context.Background(), "", "cam-1", behavioral.DetectorROI)
	if err == nil {
		t.Error("expected error on empty tenantID")
	}
}

func TestStore_UpsertRequiresCameraID(t *testing.T) {
	db := openTestDB(t)
	store := behavioral.NewStore(db)
	err := store.Upsert(context.Background(), behavioral.Config{
		TenantID:     "tenant-a",
		DetectorType: behavioral.DetectorROI,
		Params:       "{}",
	})
	if err == nil {
		t.Error("expected error on empty cameraID")
	}
}

// Ensure timestamps are set on write (basic sanity check on the DB round-trip).
func TestStore_TimestampsPopulated(t *testing.T) {
	db := openTestDB(t)
	tenantID := "tenant-a"
	cameraID := "cam-1"
	seedTenantAndCamera(t, db, tenantID, cameraID)

	store := behavioral.NewStore(db)
	ctx := context.Background()

	before := time.Now().Add(-time.Second)
	_ = store.Upsert(ctx, behavioral.Config{
		TenantID:     tenantID,
		CameraID:     cameraID,
		DetectorType: behavioral.DetectorCrowdDensity,
		Params:       `{"max_count":10}`,
	})
	after := time.Now().Add(time.Second)

	got, err := store.Get(ctx, tenantID, cameraID, behavioral.DetectorCrowdDensity)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if got.CreatedAt.Before(before) || got.CreatedAt.After(after) {
		t.Errorf("CreatedAt %v not in expected window [%v, %v]", got.CreatedAt, before, after)
	}
}
