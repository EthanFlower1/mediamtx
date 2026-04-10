package models_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/models"
	"github.com/bluenviron/mediamtx/internal/shared/inference"
)

// openTestDB opens a fresh SQLite-backed cloud DB with all migrations applied.
// The models table has no FK dependencies on other tables, so no fixture
// seeding is required.
func openTestDB(t *testing.T) *clouddb.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := "sqlite://" + filepath.Join(dir, "cloud_models_test.db")
	d, err := clouddb.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

const (
	tenantAlpha = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	tenantBravo = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	ownerUser   = "uuuuuuuu-uuuu-4uuu-8uuu-uuuuuuuuuuuu"
)

func newInput(name, version string) models.CreateModelInput {
	return models.CreateModelInput{
		TenantID:    tenantAlpha,
		Name:        name,
		Version:     version,
		Framework:   models.FrameworkONNX,
		FileRef:     "s3://models/" + name + "/" + version + ".onnx",
		FileSHA256:  "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		SizeBytes:   1024 * 1024,
		OwnerUserID: ownerUser,
	}
}

// -----------------------------------------------------------------------
// Create + Get round-trip
// -----------------------------------------------------------------------

func TestCreateGetRoundTrip(t *testing.T) {
	d := openTestDB(t)
	store := models.NewStore(d)
	ctx := context.Background()

	created, err := store.Create(ctx, newInput("yolo-v8", "1.0.0"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if created.ApprovalState != models.StateDraft {
		t.Errorf("approval_state = %q, want draft", created.ApprovalState)
	}

	got, err := store.Get(ctx, tenantAlpha, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "yolo-v8" {
		t.Errorf("name = %q", got.Name)
	}
	if got.Version != "1.0.0" {
		t.Errorf("version = %q", got.Version)
	}
	if got.Framework != models.FrameworkONNX {
		t.Errorf("framework = %q", got.Framework)
	}
	if got.TenantID != tenantAlpha {
		t.Errorf("tenant_id = %q", got.TenantID)
	}
}

// -----------------------------------------------------------------------
// Duplicate version
// -----------------------------------------------------------------------

func TestCreateDuplicateVersion(t *testing.T) {
	d := openTestDB(t)
	store := models.NewStore(d)
	ctx := context.Background()

	input := newInput("yolo-v8", "1.0.0")
	if _, err := store.Create(ctx, input); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	_, err := store.Create(ctx, input)
	if !errors.Is(err, models.ErrDuplicateVersion) {
		t.Errorf("expected ErrDuplicateVersion, got %v", err)
	}
}

// -----------------------------------------------------------------------
// List with tenant isolation
// -----------------------------------------------------------------------

func TestListTenantIsolation(t *testing.T) {
	d := openTestDB(t)
	store := models.NewStore(d)
	ctx := context.Background()

	// Create model under Alpha.
	if _, err := store.Create(ctx, newInput("yolo-v8", "1.0.0")); err != nil {
		t.Fatalf("Create Alpha: %v", err)
	}

	// Create model under Bravo.
	bravoInput := newInput("clip-vit", "2.0.0")
	bravoInput.TenantID = tenantBravo
	if _, err := store.Create(ctx, bravoInput); err != nil {
		t.Fatalf("Create Bravo: %v", err)
	}

	// Alpha should see only its own model.
	alphaModels, err := store.List(ctx, models.ListFilter{TenantID: tenantAlpha})
	if err != nil {
		t.Fatalf("List Alpha: %v", err)
	}
	if len(alphaModels) != 1 {
		t.Errorf("Alpha model count = %d, want 1", len(alphaModels))
	}

	// Bravo should see only its own model.
	bravoModels, err := store.List(ctx, models.ListFilter{TenantID: tenantBravo})
	if err != nil {
		t.Fatalf("List Bravo: %v", err)
	}
	if len(bravoModels) != 1 {
		t.Errorf("Bravo model count = %d, want 1", len(bravoModels))
	}
	if bravoModels[0].Name != "clip-vit" {
		t.Errorf("Bravo model name = %q, want clip-vit", bravoModels[0].Name)
	}
}

// -----------------------------------------------------------------------
// List with state filter
// -----------------------------------------------------------------------

func TestListWithStateFilter(t *testing.T) {
	d := openTestDB(t)
	store := models.NewStore(d)
	ctx := context.Background()

	// Create two models.
	m1, err := store.Create(ctx, newInput("yolo-v8", "1.0.0"))
	if err != nil {
		t.Fatalf("Create m1: %v", err)
	}
	if _, err := store.Create(ctx, newInput("clip-vit", "1.0.0")); err != nil {
		t.Fatalf("Create m2: %v", err)
	}

	// Move m1 to in_review.
	if _, err := store.UpdateApproval(ctx, models.UpdateApprovalInput{
		ModelID:    m1.ID,
		TenantID:   tenantAlpha,
		NewState:   models.StateInReview,
		ApprovedBy: ownerUser,
	}); err != nil {
		t.Fatalf("UpdateApproval: %v", err)
	}

	// Filter by draft — should return only m2.
	state := models.StateDraft
	drafts, err := store.List(ctx, models.ListFilter{TenantID: tenantAlpha, ApprovalState: &state})
	if err != nil {
		t.Fatalf("List drafts: %v", err)
	}
	if len(drafts) != 1 {
		t.Errorf("draft count = %d, want 1", len(drafts))
	}

	// Filter by in_review — should return only m1.
	state = models.StateInReview
	inReview, err := store.List(ctx, models.ListFilter{TenantID: tenantAlpha, ApprovalState: &state})
	if err != nil {
		t.Fatalf("List in_review: %v", err)
	}
	if len(inReview) != 1 {
		t.Errorf("in_review count = %d, want 1", len(inReview))
	}
}

// -----------------------------------------------------------------------
// UpdateApproval valid transitions
// -----------------------------------------------------------------------

func TestUpdateApprovalValidTransitions(t *testing.T) {
	d := openTestDB(t)
	store := models.NewStore(d)
	ctx := context.Background()

	m, err := store.Create(ctx, newInput("yolo-v8", "1.0.0"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// draft -> in_review
	m, err = store.UpdateApproval(ctx, models.UpdateApprovalInput{
		ModelID:    m.ID,
		TenantID:   tenantAlpha,
		NewState:   models.StateInReview,
		ApprovedBy: ownerUser,
	})
	if err != nil {
		t.Fatalf("draft -> in_review: %v", err)
	}
	if m.ApprovalState != models.StateInReview {
		t.Errorf("state = %q, want in_review", m.ApprovalState)
	}

	// in_review -> approved
	m, err = store.UpdateApproval(ctx, models.UpdateApprovalInput{
		ModelID:    m.ID,
		TenantID:   tenantAlpha,
		NewState:   models.StateApproved,
		ApprovedBy: ownerUser,
	})
	if err != nil {
		t.Fatalf("in_review -> approved: %v", err)
	}
	if m.ApprovalState != models.StateApproved {
		t.Errorf("state = %q, want approved", m.ApprovalState)
	}
}

// -----------------------------------------------------------------------
// UpdateApproval invalid transition
// -----------------------------------------------------------------------

func TestUpdateApprovalInvalidTransition(t *testing.T) {
	d := openTestDB(t)
	store := models.NewStore(d)
	ctx := context.Background()

	m, err := store.Create(ctx, newInput("yolo-v8", "1.0.0"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// draft -> approved directly (invalid)
	_, err = store.UpdateApproval(ctx, models.UpdateApprovalInput{
		ModelID:    m.ID,
		TenantID:   tenantAlpha,
		NewState:   models.StateApproved,
		ApprovedBy: ownerUser,
	})
	if !errors.Is(err, models.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

// -----------------------------------------------------------------------
// UpdateApproval sets approved_by and approved_at
// -----------------------------------------------------------------------

func TestUpdateApprovalSetsApproverFields(t *testing.T) {
	d := openTestDB(t)
	store := models.NewStore(d)
	ctx := context.Background()

	m, err := store.Create(ctx, newInput("yolo-v8", "1.0.0"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// draft -> in_review
	m, err = store.UpdateApproval(ctx, models.UpdateApprovalInput{
		ModelID:  m.ID, TenantID: tenantAlpha,
		NewState: models.StateInReview, ApprovedBy: ownerUser,
	})
	if err != nil {
		t.Fatalf("draft -> in_review: %v", err)
	}
	// approved_by should NOT be set for in_review.
	if m.ApprovedBy != nil {
		t.Errorf("approved_by should be nil for in_review, got %q", *m.ApprovedBy)
	}

	// in_review -> approved
	before := time.Now().UTC().Add(-time.Second)
	m, err = store.UpdateApproval(ctx, models.UpdateApprovalInput{
		ModelID:    m.ID,
		TenantID:   tenantAlpha,
		NewState:   models.StateApproved,
		ApprovedBy: ownerUser,
	})
	if err != nil {
		t.Fatalf("in_review -> approved: %v", err)
	}
	if m.ApprovedBy == nil || *m.ApprovedBy != ownerUser {
		t.Errorf("approved_by = %v, want %q", m.ApprovedBy, ownerUser)
	}
	if m.ApprovedAt == nil {
		t.Fatal("approved_at is nil")
	}
	if m.ApprovedAt.Before(before) {
		t.Errorf("approved_at %v is before test start %v", m.ApprovedAt, before)
	}
}

// -----------------------------------------------------------------------
// ResolveApproved
// -----------------------------------------------------------------------

func TestResolveApprovedReturnsLatest(t *testing.T) {
	d := openTestDB(t)
	store := models.NewStore(d)
	ctx := context.Background()

	// Create and approve v1.
	m1, err := store.Create(ctx, newInput("yolo-v8", "1.0.0"))
	if err != nil {
		t.Fatalf("Create v1: %v", err)
	}
	approveModel(t, store, ctx, m1.ID)

	// Small delay so created_at differs.
	time.Sleep(10 * time.Millisecond)

	// Create and approve v2.
	m2, err := store.Create(ctx, newInput("yolo-v8", "2.0.0"))
	if err != nil {
		t.Fatalf("Create v2: %v", err)
	}
	approveModel(t, store, ctx, m2.ID)

	// ResolveApproved should return v2 (latest by created_at).
	resolved, err := store.ResolveApproved(ctx, tenantAlpha, "yolo-v8")
	if err != nil {
		t.Fatalf("ResolveApproved: %v", err)
	}
	if resolved.Version != "2.0.0" {
		t.Errorf("resolved version = %q, want 2.0.0", resolved.Version)
	}
}

func TestResolveApprovedNotFoundWhenNoApproved(t *testing.T) {
	d := openTestDB(t)
	store := models.NewStore(d)
	ctx := context.Background()

	// Create a draft model — not approved.
	if _, err := store.Create(ctx, newInput("yolo-v8", "1.0.0")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err := store.ResolveApproved(ctx, tenantAlpha, "yolo-v8")
	if !errors.Is(err, models.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// -----------------------------------------------------------------------
// Delete
// -----------------------------------------------------------------------

func TestDelete(t *testing.T) {
	d := openTestDB(t)
	store := models.NewStore(d)
	ctx := context.Background()

	m, err := store.Create(ctx, newInput("yolo-v8", "1.0.0"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.Delete(ctx, tenantAlpha, m.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = store.Get(ctx, tenantAlpha, m.ID)
	if !errors.Is(err, models.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteCrossTenant(t *testing.T) {
	d := openTestDB(t)
	store := models.NewStore(d)
	ctx := context.Background()

	m, err := store.Create(ctx, newInput("yolo-v8", "1.0.0"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Bravo tries to delete Alpha's model.
	err = store.Delete(ctx, tenantBravo, m.ID)
	if !errors.Is(err, models.ErrNotFound) {
		t.Errorf("cross-tenant Delete: expected ErrNotFound, got %v", err)
	}

	// Model must still exist under Alpha.
	if _, err := store.Get(ctx, tenantAlpha, m.ID); err != nil {
		t.Errorf("Alpha model deleted by Bravo: %v", err)
	}
}

// -----------------------------------------------------------------------
// Platform builtin tenant
// -----------------------------------------------------------------------

func TestPlatformBuiltinModels(t *testing.T) {
	d := openTestDB(t)
	store := models.NewStore(d)
	ctx := context.Background()

	// Create a platform-builtin model.
	builtinInput := newInput("yolo-v8-base", "0.1.0")
	builtinInput.TenantID = models.PlatformBuiltinTenantID
	m, err := store.Create(ctx, builtinInput)
	if err != nil {
		t.Fatalf("Create builtin: %v", err)
	}
	approveModelForTenant(t, store, ctx, models.PlatformBuiltinTenantID, m.ID)

	// Resolve via the RegistryAdapter for Alpha — should fall back to builtin.
	adapter := models.NewRegistryAdapter(store, tenantAlpha)
	_, version, err := adapter.Resolve(ctx, "yolo-v8-base")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if version != "0.1.0" {
		t.Errorf("version = %q, want 0.1.0", version)
	}
}

func TestRegistryAdapterTenantOverridesBuiltin(t *testing.T) {
	d := openTestDB(t)
	store := models.NewStore(d)
	ctx := context.Background()

	// Create and approve a platform-builtin model.
	builtinInput := newInput("yolo-v8", "0.1.0")
	builtinInput.TenantID = models.PlatformBuiltinTenantID
	mb, err := store.Create(ctx, builtinInput)
	if err != nil {
		t.Fatalf("Create builtin: %v", err)
	}
	approveModelForTenant(t, store, ctx, models.PlatformBuiltinTenantID, mb.ID)

	// Create and approve a tenant-specific override.
	tenantInput := newInput("yolo-v8", "3.0.0")
	mt, err := store.Create(ctx, tenantInput)
	if err != nil {
		t.Fatalf("Create tenant: %v", err)
	}
	approveModel(t, store, ctx, mt.ID)

	// Adapter should prefer tenant-specific over builtin.
	adapter := models.NewRegistryAdapter(store, tenantAlpha)
	_, version, err := adapter.Resolve(ctx, "yolo-v8")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if version != "3.0.0" {
		t.Errorf("version = %q, want 3.0.0 (tenant override)", version)
	}
}

func TestRegistryAdapterNotFound(t *testing.T) {
	d := openTestDB(t)
	store := models.NewStore(d)
	ctx := context.Background()

	adapter := models.NewRegistryAdapter(store, tenantAlpha)
	_, _, err := adapter.Resolve(ctx, "nonexistent-model")
	if !errors.Is(err, inference.ErrModelNotFound) {
		t.Errorf("expected inference.ErrModelNotFound, got %v", err)
	}
}

// -----------------------------------------------------------------------
// UpdateMetrics
// -----------------------------------------------------------------------

func TestUpdateMetrics(t *testing.T) {
	d := openTestDB(t)
	store := models.NewStore(d)
	ctx := context.Background()

	m, err := store.Create(ctx, newInput("yolo-v8", "1.0.0"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	newMetrics := json.RawMessage(`{"accuracy": 0.95, "f1": 0.92}`)
	if err := store.UpdateMetrics(ctx, tenantAlpha, m.ID, newMetrics); err != nil {
		t.Fatalf("UpdateMetrics: %v", err)
	}

	got, err := store.Get(ctx, tenantAlpha, m.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	var metrics map[string]float64
	if err := json.Unmarshal(got.Metrics, &metrics); err != nil {
		t.Fatalf("unmarshal metrics: %v", err)
	}
	if metrics["accuracy"] != 0.95 {
		t.Errorf("accuracy = %v, want 0.95", metrics["accuracy"])
	}
}

// -----------------------------------------------------------------------
// helpers
// -----------------------------------------------------------------------

func approveModel(t *testing.T, store models.Store, ctx context.Context, id string) {
	t.Helper()
	approveModelForTenant(t, store, ctx, tenantAlpha, id)
}

func approveModelForTenant(t *testing.T, store models.Store, ctx context.Context, tenantID, id string) {
	t.Helper()
	// draft -> in_review
	if _, err := store.UpdateApproval(ctx, models.UpdateApprovalInput{
		ModelID: id, TenantID: tenantID,
		NewState: models.StateInReview, ApprovedBy: ownerUser,
	}); err != nil {
		t.Fatalf("approve step 1 (draft -> in_review): %v", err)
	}
	// in_review -> approved
	if _, err := store.UpdateApproval(ctx, models.UpdateApprovalInput{
		ModelID: id, TenantID: tenantID,
		NewState: models.StateApproved, ApprovedBy: ownerUser,
	}); err != nil {
		t.Fatalf("approve step 2 (in_review -> approved): %v", err)
	}
}
