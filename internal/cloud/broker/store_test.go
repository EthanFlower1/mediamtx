package broker

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func TestCreateTenantAndLookup(t *testing.T) {
	s := newTestStore(t)

	id, err := s.CreateTenant("Acme Corp", "admin@acme.com")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	// Lookup by ID.
	got, err := s.GetTenant(id)
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if got.Name != "Acme Corp" || got.Email != "admin@acme.com" {
		t.Fatalf("unexpected tenant: %+v", got)
	}

	// Lookup by email.
	got2, err := s.GetTenantByEmail("admin@acme.com")
	if err != nil {
		t.Fatalf("get tenant by email: %v", err)
	}
	if got2.ID != id {
		t.Fatalf("email lookup returned wrong id: got %s, want %s", got2.ID, id)
	}

	// Duplicate email should fail.
	_, err = s.CreateTenant("Other", "admin@acme.com")
	if err == nil {
		t.Fatal("expected error on duplicate email")
	}
}

func TestCreateAPIKeyAndValidate(t *testing.T) {
	s := newTestStore(t)

	tenantID, err := s.CreateTenant("Test Co", "test@example.com")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	plainKey, err := s.CreateAPIKey(tenantID, "my-key")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Key must have the expected prefix format.
	if !strings.HasPrefix(plainKey, "kvue_") {
		t.Fatalf("key missing kvue_ prefix: %s", plainKey)
	}
	if len(plainKey) != 45 { // "kvue_" (5) + 40 hex chars
		t.Fatalf("unexpected key length: got %d, want 45", len(plainKey))
	}

	// Validate returns the correct tenant.
	gotTenant, err := s.ValidateAPIKey(plainKey)
	if err != nil {
		t.Fatalf("validate api key: %v", err)
	}
	if gotTenant != tenantID {
		t.Fatalf("validate returned wrong tenant: got %s, want %s", gotTenant, tenantID)
	}
}

func TestValidateInvalidKey(t *testing.T) {
	s := newTestStore(t)

	_, err := s.ValidateAPIKey("kvue_0000000000000000000000000000000000000000")
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got: %v", err)
	}
}

func TestListTenantKeys(t *testing.T) {
	s := newTestStore(t)

	tid, err := s.CreateTenant("List Co", "list@example.com")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	// Create two keys.
	_, err = s.CreateAPIKey(tid, "key-a")
	if err != nil {
		t.Fatalf("create key a: %v", err)
	}
	_, err = s.CreateAPIKey(tid, "key-b")
	if err != nil {
		t.Fatalf("create key b: %v", err)
	}

	keys, err := s.ListAPIKeys(tid)
	if err != nil {
		t.Fatalf("list keys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}

	// Verify names are as expected (ordered by created_at).
	if keys[0].Name != "key-a" || keys[1].Name != "key-b" {
		t.Fatalf("unexpected key names: %+v", keys)
	}

	// Verify no secrets are leaked (prefix should be short).
	for _, k := range keys {
		if len(k.Prefix) != 10 {
			t.Fatalf("unexpected prefix length: %d", len(k.Prefix))
		}
		if k.TenantID != tid {
			t.Fatalf("wrong tenant id in key info")
		}
	}

	// List for non-existent tenant returns empty.
	empty, err := s.ListAPIKeys("nonexistent")
	if err != nil {
		t.Fatalf("list keys for missing tenant: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(empty))
	}
}
