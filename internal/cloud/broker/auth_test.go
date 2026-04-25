package broker

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestAuthenticateValidKey(t *testing.T) {
	db := openTestDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	tenantID, err := store.CreateTenant("Acme Corp", "acme@example.com")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}

	plainKey, err := store.CreateAPIKey(tenantID, "test-key")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	auth := NewAuthenticator(store)
	gotTenantID, ok := auth.Authenticate(plainKey)
	if !ok {
		t.Fatal("Authenticate returned ok=false for a valid key")
	}
	if gotTenantID != tenantID {
		t.Fatalf("tenant ID mismatch: got %q, want %q", gotTenantID, tenantID)
	}
}

func TestAuthenticateInvalidKey(t *testing.T) {
	db := openTestDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	auth := NewAuthenticator(store)
	gotTenantID, ok := auth.Authenticate("kvue_totallyinvalidgarbage1234567890abcdef")
	if ok {
		t.Fatal("Authenticate returned ok=true for an invalid key")
	}
	if gotTenantID != "" {
		t.Fatalf("expected empty tenant ID for invalid key, got %q", gotTenantID)
	}
}
