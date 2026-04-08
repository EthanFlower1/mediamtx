package cameras_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/cameras"
	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/db/fixtures"
	"github.com/bluenviron/mediamtx/internal/shared/cryptostore"
)

// openTestDB opens a fresh SQLite-backed cloud DB and seeds the standard
// fixture tenants/directories so FK constraints are satisfied.
func openTestDB(t *testing.T) *clouddb.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := "sqlite://" + filepath.Join(dir, "cloud_cameras_test.db")
	d, err := clouddb.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	// Seed standard fixtures so FK references to customer_tenants and
	// on_prem_directories succeed.
	if err := fixtures.Seed(context.Background(), d); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return d
}

// seedRecorder inserts one recorder row and returns its ID.
func seedRecorder(t *testing.T, d *clouddb.DB, tenantID, id string) {
	t.Helper()
	reg := cameras.NewRecorderRegistry(d)
	if err := reg.Create(context.Background(), cameras.Recorder{
		ID:          id,
		TenantID:    tenantID,
		DirectoryID: fixtures.DirectoryBetaSiteOneID,
		DisplayName: "test-recorder",
		Status:      "online",
		Region:      clouddb.DefaultRegion,
	}); err != nil {
		t.Fatalf("seed recorder: %v", err)
	}
}

// -----------------------------------------------------------------------
// Migration test
// -----------------------------------------------------------------------

func TestMigrationsInclude249Tables(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	versions, err := d.AppliedVersions(ctx)
	if err != nil {
		t.Fatalf("applied versions: %v", err)
	}

	// Expect at least through version 13 (0013_camera_segment_index).
	if len(versions) < 13 {
		t.Fatalf("expected at least 13 migrations, got %d: %v", len(versions), versions)
	}
	// Spot-check the KAI-249 migrations are present.
	want := map[int]bool{9: false, 10: false, 11: false, 12: false, 13: false}
	for _, v := range versions {
		if _, ok := want[v]; ok {
			want[v] = true
		}
	}
	for v, seen := range want {
		if !seen {
			t.Errorf("migration %04d not applied", v)
		}
	}
}

// -----------------------------------------------------------------------
// Camera CRUD and cross-tenant isolation
// -----------------------------------------------------------------------

func TestCameraRoundTrip(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	recorderID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	seedRecorder(t, d, fixtures.CustomerTenantBetaID, recorderID)

	reg := cameras.NewCameraRegistry(d)

	cam := cameras.Camera{
		ID:          "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
		TenantID:    fixtures.CustomerTenantBetaID,
		DirectoryID: fixtures.DirectoryBetaSiteOneID,
		DisplayName: "Front Entrance",
		Status:      "online",
		Region:      clouddb.DefaultRegion,
	}
	if err := reg.Create(ctx, cam); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := reg.Get(ctx, fixtures.CustomerTenantBetaID, cam.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DisplayName != "Front Entrance" {
		t.Errorf("display_name = %q", got.DisplayName)
	}
	if got.TenantID != fixtures.CustomerTenantBetaID {
		t.Errorf("tenant_id = %q", got.TenantID)
	}
}

// TestCameraCreateRequiresTenantID ensures a missing tenant fails fast (seam #4).
func TestCameraCreateRequiresTenantID(t *testing.T) {
	d := openTestDB(t)
	reg := cameras.NewCameraRegistry(d)

	err := reg.Create(context.Background(), cameras.Camera{
		ID:          "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
		DirectoryID: fixtures.DirectoryBetaSiteOneID,
		DisplayName: "No Tenant",
	})
	if !errors.Is(err, cameras.ErrInvalidTenantID) {
		t.Errorf("expected ErrInvalidTenantID, got %v", err)
	}
}

// TestCameraCrossTenantIsolation is the KAI-235 pattern: Tenant A's camera
// must be invisible to Tenant B. This is the cheapest local approximation of
// the chaos test.
func TestCameraCrossTenantIsolation(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	recorderID := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	seedRecorder(t, d, fixtures.CustomerTenantBetaID, recorderID)

	reg := cameras.NewCameraRegistry(d)

	// Insert camera under Beta.
	betaCamID := "11111111-eeee-4eee-8eee-eeeeeeeeeeee"
	if err := reg.Create(ctx, cameras.Camera{
		ID:          betaCamID,
		TenantID:    fixtures.CustomerTenantBetaID,
		DirectoryID: fixtures.DirectoryBetaSiteOneID,
		DisplayName: "Beta Camera",
		Status:      "online",
		Region:      clouddb.DefaultRegion,
	}); err != nil {
		t.Fatalf("Create Beta camera: %v", err)
	}

	// Gamma must not see Beta's camera.
	_, err := reg.Get(ctx, fixtures.CustomerTenantGammaID, betaCamID)
	if !errors.Is(err, cameras.ErrNotFound) {
		t.Errorf("cross-tenant Get: expected ErrNotFound, got %v", err)
	}

	// ListByRecorder under Gamma must return zero rows, not Beta's camera.
	gammaCams, err := reg.ListByRecorder(ctx, fixtures.CustomerTenantGammaID, recorderID)
	if err != nil {
		t.Fatalf("ListByRecorder Gamma: %v", err)
	}
	if len(gammaCams) != 0 {
		t.Errorf("cross-tenant ListByRecorder: Gamma sees %d cameras from Beta", len(gammaCams))
	}

	// List under Gamma must be empty.
	allGamma, err := reg.List(ctx, fixtures.CustomerTenantGammaID)
	if err != nil {
		t.Fatalf("List Gamma: %v", err)
	}
	if len(allGamma) != 0 {
		t.Errorf("cross-tenant List: Gamma sees %d cameras", len(allGamma))
	}
}

// TestCameraDeleteCrossTenant verifies Delete does not remove another tenant's camera.
func TestCameraDeleteCrossTenant(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	recorderID := "cccccccc-0000-4000-8000-000000000001"
	seedRecorder(t, d, fixtures.CustomerTenantBetaID, recorderID)

	reg := cameras.NewCameraRegistry(d)
	betaCamID := "cccccccc-0000-4000-8000-000000000002"
	if err := reg.Create(ctx, cameras.Camera{
		ID:          betaCamID,
		TenantID:    fixtures.CustomerTenantBetaID,
		DirectoryID: fixtures.DirectoryBetaSiteOneID,
		DisplayName: "Protected Camera",
		Status:      "online",
		Region:      clouddb.DefaultRegion,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Gamma attempts to delete Beta's camera — must get ErrNotFound.
	err := reg.Delete(ctx, fixtures.CustomerTenantGammaID, betaCamID)
	if !errors.Is(err, cameras.ErrNotFound) {
		t.Errorf("cross-tenant Delete: expected ErrNotFound, got %v", err)
	}

	// The camera must still exist under Beta.
	if _, err := reg.Get(ctx, fixtures.CustomerTenantBetaID, betaCamID); err != nil {
		t.Errorf("Beta camera was deleted by Gamma: %v", err)
	}
}

// TestRTSPCredentialsEncryptionRoundTrip encrypts RTSP credentials via the
// cryptostore and verifies the stored blob is non-empty and decryptable.
func TestRTSPCredentialsEncryptionRoundTrip(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	// Derive a test subkey from a dummy master.
	master := []byte("00000000000000000000000000000000")
	cs, err := cryptostore.NewFromMaster(master, nil, cryptostore.InfoRTSPCredentials)
	if err != nil {
		t.Fatalf("NewFromMaster: %v", err)
	}

	plaintext := []byte("rtsp://user:s3cr3t@192.168.1.10:554/stream")
	blob, err := cs.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(blob) == 0 {
		t.Fatal("encrypted blob is empty")
	}
	if string(blob) == string(plaintext) {
		t.Fatal("blob is not encrypted")
	}

	recorderID := "dddddddd-0000-4000-8000-000000000003"
	seedRecorder(t, d, fixtures.CustomerTenantBetaID, recorderID)

	reg := cameras.NewCameraRegistry(d)
	camID := "dddddddd-0000-4000-8000-000000000004"
	if err := reg.Create(ctx, cameras.Camera{
		ID:                       camID,
		TenantID:                 fixtures.CustomerTenantBetaID,
		DirectoryID:              fixtures.DirectoryBetaSiteOneID,
		DisplayName:              "Encrypted Cam",
		RTSPCredentialsEncrypted: blob,
		Status:                   "online",
		Region:                   clouddb.DefaultRegion,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := reg.Get(ctx, fixtures.CustomerTenantBetaID, camID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got.RTSPCredentialsEncrypted) == string(plaintext) {
		t.Error("plaintext RTSP credentials stored unencrypted")
	}

	// Decrypt and verify.
	decrypted, err := cs.Decrypt(got.RTSPCredentialsEncrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

// -----------------------------------------------------------------------
// Recorder cross-tenant isolation
// -----------------------------------------------------------------------

func TestRecorderCrossTenantIsolation(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	recID := "eeeeeeee-0000-4000-8000-000000000005"
	seedRecorder(t, d, fixtures.CustomerTenantBetaID, recID)

	reg := cameras.NewRecorderRegistry(d)

	// Gamma must not see Beta's recorder.
	_, err := reg.Get(ctx, fixtures.CustomerTenantGammaID, recID)
	if !errors.Is(err, cameras.ErrNotFound) {
		t.Errorf("cross-tenant Get recorder: expected ErrNotFound, got %v", err)
	}

	gammaRecs, err := reg.List(ctx, fixtures.CustomerTenantGammaID)
	if err != nil {
		t.Fatalf("List Gamma recorders: %v", err)
	}
	if len(gammaRecs) != 0 {
		t.Errorf("Gamma sees %d recorders from Beta", len(gammaRecs))
	}
}

// TestRecorderUpdateStatus verifies the status seam used by recordercontrol.
func TestRecorderUpdateStatus(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	recID := "ffffffff-0000-4000-8000-000000000006"
	seedRecorder(t, d, fixtures.CustomerTenantBetaID, recID)

	reg := cameras.NewRecorderRegistry(d)
	if err := reg.UpdateStatus(ctx, fixtures.CustomerTenantBetaID, recID, "degraded"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, err := reg.Get(ctx, fixtures.CustomerTenantBetaID, recID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != "degraded" {
		t.Errorf("status = %q, want degraded", got.Status)
	}
	if got.LastCheckinAt == nil {
		t.Error("last_checkin_at not set after UpdateStatus")
	}
}

// -----------------------------------------------------------------------
// SegmentIndex
// -----------------------------------------------------------------------

func TestSegmentIndexCrossTenantIsolation(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	recID := "aabbccdd-0000-4000-8000-000000000007"
	seedRecorder(t, d, fixtures.CustomerTenantBetaID, recID)

	camReg := cameras.NewCameraRegistry(d)
	betaCamID := "aabbccdd-0000-4000-8000-000000000008"
	if err := camReg.Create(ctx, cameras.Camera{
		ID:          betaCamID,
		TenantID:    fixtures.CustomerTenantBetaID,
		DirectoryID: fixtures.DirectoryBetaSiteOneID,
		DisplayName: "Segment Test Cam",
		Status:      "online",
		Region:      clouddb.DefaultRegion,
	}); err != nil {
		t.Fatalf("Create camera: %v", err)
	}

	idx := cameras.NewSegmentIndex(d)
	now := time.Now().UTC().Truncate(time.Second)
	seg := cameras.Segment{
		CameraID:      betaCamID,
		RecorderID:    recID,
		TenantID:      fixtures.CustomerTenantBetaID,
		StartTS:       now,
		EndTS:         now.Add(30 * time.Second),
		FilePath:      "/recordings/beta/seg-001.mp4",
		FileSizeBytes: 1024 * 1024,
		StorageTier:   "hot",
		Region:        clouddb.DefaultRegion,
	}
	if err := idx.Append(ctx, seg); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Gamma must not see Beta's segments.
	gammasegs, err := idx.QueryByTimeRange(ctx,
		fixtures.CustomerTenantGammaID,
		betaCamID,
		now.Add(-time.Minute), now.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("QueryByTimeRange Gamma: %v", err)
	}
	if len(gammasegs) != 0 {
		t.Errorf("cross-tenant: Gamma sees %d segments from Beta", len(gammasegs))
	}

	// Beta should see the segment.
	betasegs, err := idx.QueryByTimeRange(ctx,
		fixtures.CustomerTenantBetaID,
		betaCamID,
		now.Add(-time.Minute), now.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("QueryByTimeRange Beta: %v", err)
	}
	if len(betasegs) != 1 {
		t.Errorf("Beta segment count = %d, want 1", len(betasegs))
	}
}

// TestSegmentIndexAppendIdempotent verifies duplicate segment inserts are
// silently ignored (ON CONFLICT DO NOTHING / INSERT OR IGNORE).
func TestSegmentIndexAppendIdempotent(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	recID := "aabbccdd-0001-4000-8000-000000000009"
	seedRecorder(t, d, fixtures.CustomerTenantBetaID, recID)

	camReg := cameras.NewCameraRegistry(d)
	camID := "aabbccdd-0001-4000-8000-00000000000a"
	if err := camReg.Create(ctx, cameras.Camera{
		ID:          camID,
		TenantID:    fixtures.CustomerTenantBetaID,
		DirectoryID: fixtures.DirectoryBetaSiteOneID,
		DisplayName: "Idempotency Test Cam",
		Status:      "online",
		Region:      clouddb.DefaultRegion,
	}); err != nil {
		t.Fatalf("Create camera: %v", err)
	}

	idx := cameras.NewSegmentIndex(d)
	now := time.Now().UTC().Truncate(time.Second)
	seg := cameras.Segment{
		CameraID:    camID,
		RecorderID:  recID,
		TenantID:    fixtures.CustomerTenantBetaID,
		StartTS:     now,
		EndTS:       now.Add(30 * time.Second),
		FilePath:    "/recordings/beta/idem-001.mp4",
		StorageTier: "hot",
		Region:      clouddb.DefaultRegion,
	}

	// First insert.
	if err := idx.Append(ctx, seg); err != nil {
		t.Fatalf("first Append: %v", err)
	}
	// Second insert — must not error.
	if err := idx.Append(ctx, seg); err != nil {
		t.Fatalf("idempotent Append failed: %v", err)
	}
}
