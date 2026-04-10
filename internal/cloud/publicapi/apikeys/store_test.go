package apikeys

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/publicapi"
	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	s := New(db, DialectSQLite)
	ctx := context.Background()
	if err := s.ApplyStubSchema(ctx); err != nil {
		t.Fatalf("stub schema: %v", err)
	}
	return s
}

func TestCreate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	result, err := s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID:  "tenant-1",
		Name:      "My Test Key",
		Scopes:    []string{"cameras:read", "events:read"},
		CreatedBy: "user-1",
		Tier:      publicapi.TierPro,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Raw key has correct prefix and length.
	if !strings.HasPrefix(result.RawKey, publicapi.APIKeyPrefix) {
		t.Errorf("RawKey prefix = %q; want %q prefix", result.RawKey[:5], publicapi.APIKeyPrefix)
	}
	// kvue_ (5) + 40 hex chars = 45
	if len(result.RawKey) != 45 {
		t.Errorf("RawKey length = %d; want 45", len(result.RawKey))
	}

	key := result.Key
	if key.TenantID != "tenant-1" {
		t.Errorf("TenantID = %q; want tenant-1", key.TenantID)
	}
	if key.Name != "My Test Key" {
		t.Errorf("Name = %q; want My Test Key", key.Name)
	}
	if len(key.Scopes) != 2 {
		t.Errorf("Scopes = %v; want 2 items", key.Scopes)
	}
	if key.Tier != publicapi.TierPro {
		t.Errorf("Tier = %q; want pro", key.Tier)
	}
	if key.CreatedBy != "user-1" {
		t.Errorf("CreatedBy = %q; want user-1", key.CreatedBy)
	}
	if key.KeyPrefix == "" {
		t.Error("KeyPrefix is empty")
	}
	if key.KeyHash == "" {
		t.Error("KeyHash is empty")
	}
	if key.ID == "" {
		t.Error("ID is empty")
	}
}

func TestCreateValidation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tests := []struct {
		name string
		req  publicapi.CreateAPIKeyRequest
	}{
		{"missing tenant", publicapi.CreateAPIKeyRequest{Name: "x", CreatedBy: "u"}},
		{"missing name", publicapi.CreateAPIKeyRequest{TenantID: "t", CreatedBy: "u"}},
		{"missing created_by", publicapi.CreateAPIKeyRequest{TenantID: "t", Name: "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.Create(ctx, tt.req)
			if err == nil {
				t.Fatal("expected error; got nil")
			}
		})
	}
}

func TestCreateWithExpiry(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	expiry := time.Now().Add(30 * 24 * time.Hour).UTC()
	result, err := s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID:  "tenant-1",
		Name:      "Expiring Key",
		CreatedBy: "user-1",
		ExpiresAt: expiry,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if result.Key.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be set")
	}
}

func TestGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	result, _ := s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID:  "tenant-1",
		Name:      "Get Test",
		CreatedBy: "user-1",
		Scopes:    []string{"cameras:*"},
	})

	got, err := s.Get(ctx, result.Key.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != result.Key.ID {
		t.Errorf("ID = %q; want %q", got.ID, result.Key.ID)
	}
	if got.Name != "Get Test" {
		t.Errorf("Name = %q; want Get Test", got.Name)
	}
	if len(got.Scopes) != 1 || got.Scopes[0] != "cameras:*" {
		t.Errorf("Scopes = %v; want [cameras:*]", got.Scopes)
	}
}

func TestGetNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Get(ctx, "nonexistent")
	if !errors.Is(err, publicapi.ErrAPIKeyNotFound) {
		t.Errorf("err = %v; want ErrAPIKeyNotFound", err)
	}
}

func TestList(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create 3 keys for tenant-1, 1 for tenant-2.
	for i := 0; i < 3; i++ {
		s.Create(ctx, publicapi.CreateAPIKeyRequest{
			TenantID: "tenant-1", Name: "key", CreatedBy: "u",
		})
	}
	s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID: "tenant-2", Name: "key", CreatedBy: "u",
	})

	keys, err := s.List(ctx, publicapi.ListAPIKeysFilter{TenantID: "tenant-1"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("len = %d; want 3", len(keys))
	}

	// Tenant isolation.
	keys2, _ := s.List(ctx, publicapi.ListAPIKeysFilter{TenantID: "tenant-2"})
	if len(keys2) != 1 {
		t.Errorf("tenant-2 keys = %d; want 1", len(keys2))
	}
}

func TestListExcludesRevoked(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r1, _ := s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID: "t", Name: "active", CreatedBy: "u",
	})
	r2, _ := s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID: "t", Name: "revoked", CreatedBy: "u",
	})
	_ = r1
	s.Revoke(ctx, r2.Key.ID, "u")

	// Default: exclude revoked.
	keys, _ := s.List(ctx, publicapi.ListAPIKeysFilter{TenantID: "t"})
	if len(keys) != 1 {
		t.Errorf("without revoked: len = %d; want 1", len(keys))
	}

	// Include revoked.
	keys, _ = s.List(ctx, publicapi.ListAPIKeysFilter{TenantID: "t", IncludeRevoked: true})
	if len(keys) != 2 {
		t.Errorf("with revoked: len = %d; want 2", len(keys))
	}
}

func TestValidate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	result, _ := s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID:  "tenant-1",
		Name:      "Validate Test",
		Scopes:    []string{"cameras:read"},
		CreatedBy: "user-1",
		Tier:      publicapi.TierStarter,
	})

	key, err := s.Validate(ctx, result.RawKey)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if key.ID != result.Key.ID {
		t.Errorf("ID = %q; want %q", key.ID, result.Key.ID)
	}
	if key.TenantID != "tenant-1" {
		t.Errorf("TenantID = %q; want tenant-1", key.TenantID)
	}
}

func TestValidateInvalidPrefix(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Validate(ctx, "bad_prefix_key_value")
	if !errors.Is(err, publicapi.ErrInvalidAPIKey) {
		t.Errorf("err = %v; want ErrInvalidAPIKey", err)
	}
}

func TestValidateUnknownKey(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Validate(ctx, "kvue_0000000000000000000000000000000000000000")
	if !errors.Is(err, publicapi.ErrInvalidAPIKey) {
		t.Errorf("err = %v; want ErrInvalidAPIKey", err)
	}
}

func TestValidateExpiredKey(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	result, _ := s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID:  "t",
		Name:      "expiring",
		CreatedBy: "u",
		ExpiresAt: time.Now().Add(-1 * time.Hour), // already expired
	})

	_, err := s.Validate(ctx, result.RawKey)
	if !errors.Is(err, publicapi.ErrAPIKeyExpired) {
		t.Errorf("err = %v; want ErrAPIKeyExpired", err)
	}
}

func TestValidateRevokedKey(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	result, _ := s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID: "t", Name: "revokable", CreatedBy: "u",
	})
	s.Revoke(ctx, result.Key.ID, "u")

	_, err := s.Validate(ctx, result.RawKey)
	if !errors.Is(err, publicapi.ErrAPIKeyRevoked) {
		t.Errorf("err = %v; want ErrAPIKeyRevoked", err)
	}
}

func TestRevoke(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	result, _ := s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID: "t", Name: "key", CreatedBy: "u",
	})

	if err := s.Revoke(ctx, result.Key.ID, "admin"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	// Verify revoked.
	key, err := s.Get(ctx, result.Key.ID)
	if err != nil {
		t.Fatalf("Get after revoke: %v", err)
	}
	if !key.IsRevoked() {
		t.Error("key should be revoked")
	}
}

func TestRevokeIdempotencyError(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	result, _ := s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID: "t", Name: "key", CreatedBy: "u",
	})
	s.Revoke(ctx, result.Key.ID, "u")

	err := s.Revoke(ctx, result.Key.ID, "u")
	if !errors.Is(err, publicapi.ErrAPIKeyAlreadyRevoked) {
		t.Errorf("err = %v; want ErrAPIKeyAlreadyRevoked", err)
	}
}

func TestRevokeNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.Revoke(ctx, "nonexistent", "u")
	if !errors.Is(err, publicapi.ErrAPIKeyNotFound) {
		t.Errorf("err = %v; want ErrAPIKeyNotFound", err)
	}
}

func TestRotate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	original, _ := s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID:  "tenant-1",
		Name:      "Rotatable Key",
		Scopes:    []string{"cameras:read"},
		CreatedBy: "user-1",
		Tier:      publicapi.TierPro,
	})

	rotated, err := s.Rotate(ctx, publicapi.RotateAPIKeyRequest{
		KeyID:       original.Key.ID,
		RotatedBy:   "admin",
		GracePeriod: 2 * time.Hour,
	})
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	// New key is valid.
	if !strings.HasPrefix(rotated.RawKey, publicapi.APIKeyPrefix) {
		t.Error("new RawKey missing kvue_ prefix")
	}
	if rotated.NewKey.ID == original.Key.ID {
		t.Error("new key ID should differ from old")
	}
	if rotated.NewKey.RotatedFromID != original.Key.ID {
		t.Errorf("RotatedFromID = %q; want %q", rotated.NewKey.RotatedFromID, original.Key.ID)
	}
	if rotated.NewKey.TenantID != "tenant-1" {
		t.Errorf("TenantID = %q; want tenant-1", rotated.NewKey.TenantID)
	}
	if rotated.NewKey.Name != "Rotatable Key" {
		t.Errorf("Name = %q; want Rotatable Key", rotated.NewKey.Name)
	}
	if len(rotated.NewKey.Scopes) != 1 || rotated.NewKey.Scopes[0] != "cameras:read" {
		t.Errorf("Scopes = %v; want [cameras:read]", rotated.NewKey.Scopes)
	}
	if rotated.NewKey.Tier != publicapi.TierPro {
		t.Errorf("Tier = %q; want pro", rotated.NewKey.Tier)
	}

	// Grace period set on old key.
	if rotated.OldKeyGraceEnd.IsZero() {
		t.Error("OldKeyGraceEnd should be set")
	}
	oldKey, _ := s.Get(ctx, original.Key.ID)
	if oldKey.GraceExpiresAt.IsZero() {
		t.Error("old key GraceExpiresAt should be set")
	}

	// Both old and new keys validate during grace period.
	_, err = s.Validate(ctx, original.RawKey)
	if err != nil {
		t.Errorf("old key should still validate during grace period: %v", err)
	}
	_, err = s.Validate(ctx, rotated.RawKey)
	if err != nil {
		t.Errorf("new key should validate: %v", err)
	}
}

func TestRotateDefaultGracePeriod(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	original, _ := s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID: "t", Name: "key", CreatedBy: "u",
	})

	rotated, err := s.Rotate(ctx, publicapi.RotateAPIKeyRequest{
		KeyID:     original.Key.ID,
		RotatedBy: "admin",
		// GracePeriod: 0 means use default (24h)
	})
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	// Grace end should be ~24h from now.
	expectedMin := time.Now().Add(23 * time.Hour)
	expectedMax := time.Now().Add(25 * time.Hour)
	if rotated.OldKeyGraceEnd.Before(expectedMin) || rotated.OldKeyGraceEnd.After(expectedMax) {
		t.Errorf("OldKeyGraceEnd = %v; want ~24h from now", rotated.OldKeyGraceEnd)
	}
}

func TestRotateRevokedKeyFails(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	result, _ := s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID: "t", Name: "key", CreatedBy: "u",
	})
	s.Revoke(ctx, result.Key.ID, "u")

	_, err := s.Rotate(ctx, publicapi.RotateAPIKeyRequest{
		KeyID:     result.Key.ID,
		RotatedBy: "admin",
	})
	if !errors.Is(err, publicapi.ErrAPIKeyRevoked) {
		t.Errorf("err = %v; want ErrAPIKeyRevoked", err)
	}
}

func TestRotateNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Rotate(ctx, publicapi.RotateAPIKeyRequest{
		KeyID:     "nonexistent",
		RotatedBy: "admin",
	})
	if !errors.Is(err, publicapi.ErrAPIKeyNotFound) {
		t.Errorf("err = %v; want ErrAPIKeyNotFound", err)
	}
}

func TestTouchLastUsed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	result, _ := s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID: "t", Name: "key", CreatedBy: "u",
	})

	// Initially LastUsedAt is zero.
	key, _ := s.Get(ctx, result.Key.ID)
	if !key.LastUsedAt.IsZero() {
		t.Error("LastUsedAt should initially be zero")
	}

	if err := s.TouchLastUsed(ctx, result.Key.ID); err != nil {
		t.Fatalf("TouchLastUsed: %v", err)
	}

	key, _ = s.Get(ctx, result.Key.ID)
	if key.LastUsedAt.IsZero() {
		t.Error("LastUsedAt should be set after TouchLastUsed")
	}
}

func TestListExpiring(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Key expiring in 12 hours.
	s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID:  "t",
		Name:      "soon",
		CreatedBy: "u",
		ExpiresAt: time.Now().Add(12 * time.Hour),
	})

	// Key expiring in 48 hours.
	s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID:  "t",
		Name:      "later",
		CreatedBy: "u",
		ExpiresAt: time.Now().Add(48 * time.Hour),
	})

	// Key with no expiry.
	s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID:  "t",
		Name:      "never",
		CreatedBy: "u",
	})

	// Look for keys expiring within 24 hours.
	expiring, err := s.ListExpiring(ctx, "t", 24*time.Hour)
	if err != nil {
		t.Fatalf("ListExpiring: %v", err)
	}
	if len(expiring) != 1 {
		t.Errorf("len = %d; want 1 (only the 12h key)", len(expiring))
	}
	if len(expiring) > 0 && expiring[0].Name != "soon" {
		t.Errorf("Name = %q; want soon", expiring[0].Name)
	}
}

func TestAuditLog(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	result, _ := s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID: "t", Name: "audited", CreatedBy: "u",
	})

	// Create emits an audit entry.
	entries, err := s.ListAuditLog(ctx, "t", result.Key.ID, 100)
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d; want 1 (create)", len(entries))
	}
	if entries[0].Action != AuditCreate {
		t.Errorf("Action = %q; want create", entries[0].Action)
	}

	// Revoke emits another entry.
	s.Revoke(ctx, result.Key.ID, "admin")
	entries, _ = s.ListAuditLog(ctx, "t", result.Key.ID, 100)
	if len(entries) != 2 {
		t.Fatalf("entries after revoke = %d; want 2", len(entries))
	}
}

func TestAuditLogRotation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	original, _ := s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID: "t", Name: "rotatable", CreatedBy: "u",
	})

	rotated, _ := s.Rotate(ctx, publicapi.RotateAPIKeyRequest{
		KeyID:     original.Key.ID,
		RotatedBy: "admin",
	})

	// Old key should have create + rotate entries.
	oldEntries, _ := s.ListAuditLog(ctx, "t", original.Key.ID, 100)
	if len(oldEntries) != 2 {
		t.Errorf("old key audit entries = %d; want 2 (create + rotate)", len(oldEntries))
	}

	// New key should have a create entry.
	newEntries, _ := s.ListAuditLog(ctx, "t", rotated.NewKey.ID, 100)
	if len(newEntries) != 1 {
		t.Errorf("new key audit entries = %d; want 1 (create)", len(newEntries))
	}
}

func TestSecretNeverStored(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	result, _ := s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID: "t", Name: "secret", CreatedBy: "u",
	})

	// The stored hash must NOT equal the raw key.
	key, _ := s.Get(ctx, result.Key.ID)
	if key.KeyHash == result.RawKey {
		t.Error("KeyHash must not equal RawKey; plaintext must not be stored")
	}

	// The hash must be a valid SHA-256 hex string (64 chars).
	if len(key.KeyHash) != 64 {
		t.Errorf("KeyHash length = %d; want 64 (SHA-256 hex)", len(key.KeyHash))
	}
}

func TestScopeEnforcementViaValidate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	result, _ := s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID:  "t",
		Name:      "scoped",
		Scopes:    []string{"cameras:read", "events:*"},
		CreatedBy: "u",
	})

	key, err := s.Validate(ctx, result.RawKey)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// HasScope from the contract.
	if !key.HasScope("cameras:read") {
		t.Error("should have cameras:read")
	}
	if key.HasScope("cameras:write") {
		t.Error("should NOT have cameras:write")
	}
	if !key.HasScope("events:read") {
		t.Error("should have events:read via wildcard")
	}
	if !key.HasScope("events:write") {
		t.Error("should have events:write via wildcard")
	}
}

// TestRevokedKeyRejectedImmediately verifies the acceptance criterion:
// "Revoked keys rejected within 5s". Since we use synchronous DB writes,
// revocation is immediate.
func TestRevokedKeyRejectedImmediately(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	result, _ := s.Create(ctx, publicapi.CreateAPIKeyRequest{
		TenantID: "t", Name: "key", CreatedBy: "u",
	})

	// Validate succeeds before revocation.
	if _, err := s.Validate(ctx, result.RawKey); err != nil {
		t.Fatalf("pre-revoke Validate: %v", err)
	}

	// Revoke.
	if err := s.Revoke(ctx, result.Key.ID, "admin"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	// Validate fails immediately.
	_, err := s.Validate(ctx, result.RawKey)
	if !errors.Is(err, publicapi.ErrAPIKeyRevoked) {
		t.Errorf("post-revoke Validate: err = %v; want ErrAPIKeyRevoked", err)
	}
}
