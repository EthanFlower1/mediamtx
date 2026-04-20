package legacydb

import (
	"os"
	"testing"
	"time"
)

func openAPIKeysTestDB(t *testing.T) *DB {
	t.Helper()
	tmp := t.TempDir()
	d, err := Open(tmp + "/test.db")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestAPIKeyCreateAndGet(t *testing.T) {
	d := openAPIKeysTestDB(t)

	k := &APIKey{
		Name:          "test-key",
		KeyPrefix:     "abcd1234",
		KeyHash:       "deadbeef",
		Scope:         "read-only",
		CustomerScope: "customer-1",
		CreatedBy:     "user-1",
	}
	if err := d.CreateAPIKey(k); err != nil {
		t.Fatalf("create: %v", err)
	}
	if k.ID == "" {
		t.Fatal("expected auto-generated ID")
	}

	got, err := d.GetAPIKey(k.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Name != "test-key" || got.Scope != "read-only" || got.CustomerScope != "customer-1" {
		t.Errorf("unexpected values: %+v", got)
	}

	got2, err := d.GetAPIKeyByHash("deadbeef")
	if err != nil {
		t.Fatalf("get by hash: %v", err)
	}
	if got2.ID != k.ID {
		t.Errorf("hash lookup returned wrong key: got %s want %s", got2.ID, k.ID)
	}
}

func TestAPIKeyList(t *testing.T) {
	d := openAPIKeysTestDB(t)

	for i := 0; i < 3; i++ {
		_ = d.CreateAPIKey(&APIKey{
			Name:      "key",
			KeyPrefix: "p",
			KeyHash:   "h" + string(rune('0'+i)),
			Scope:     "read-only",
			CreatedBy: "user-1",
		})
	}
	_ = d.CreateAPIKey(&APIKey{
		Name:      "other",
		KeyPrefix: "p",
		KeyHash:   "h9",
		Scope:     "read-write",
		CreatedBy: "user-2",
	})

	all, err := d.ListAPIKeys("")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("expected 4 keys, got %d", len(all))
	}

	filtered, err := d.ListAPIKeys("user-1")
	if err != nil {
		t.Fatalf("list filtered: %v", err)
	}
	if len(filtered) != 3 {
		t.Errorf("expected 3 keys for user-1, got %d", len(filtered))
	}
}

func TestAPIKeyRevoke(t *testing.T) {
	d := openAPIKeysTestDB(t)

	k := &APIKey{Name: "rk", KeyPrefix: "p", KeyHash: "rh", Scope: "read-only", CreatedBy: "u"}
	_ = d.CreateAPIKey(k)

	if err := d.RevokeAPIKey(k.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	got, _ := d.GetAPIKey(k.ID)
	if got.RevokedAt == "" {
		t.Error("expected revoked_at to be set")
	}

	// Double revoke returns ErrNotFound (already revoked).
	if err := d.RevokeAPIKey(k.ID); err != ErrNotFound {
		t.Errorf("expected ErrNotFound on double revoke, got %v", err)
	}
}

func TestAPIKeyGracePeriod(t *testing.T) {
	d := openAPIKeysTestDB(t)

	k := &APIKey{Name: "gk", KeyPrefix: "p", KeyHash: "gh", Scope: "read-only", CreatedBy: "u"}
	_ = d.CreateAPIKey(k)

	grace := time.Now().UTC().Add(24 * time.Hour)
	if err := d.SetAPIKeyGraceExpiry(k.ID, grace); err != nil {
		t.Fatalf("set grace: %v", err)
	}

	got, _ := d.GetAPIKey(k.ID)
	if got.GraceExpiresAt == "" {
		t.Error("expected grace_expires_at to be set")
	}
}

func TestAPIKeyAudit(t *testing.T) {
	d := openAPIKeysTestDB(t)

	k := &APIKey{Name: "ak", KeyPrefix: "p", KeyHash: "ah", Scope: "read-only", CreatedBy: "u"}
	_ = d.CreateAPIKey(k)

	_ = d.InsertAPIKeyAudit(&APIKeyAuditEntry{
		APIKeyID:      k.ID,
		Action:        "created",
		ActorID:       "u",
		ActorUsername: "admin",
		IPAddress:     "127.0.0.1",
		Details:       "test",
	})
	_ = d.InsertAPIKeyAudit(&APIKeyAuditEntry{
		APIKeyID:      k.ID,
		Action:        "revoked",
		ActorID:       "u",
		ActorUsername: "admin",
	})

	entries, err := d.ListAPIKeyAudit(k.ID)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
	// Most recent first.
	if entries[0].Action != "revoked" {
		t.Errorf("expected most recent action first, got %s", entries[0].Action)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
