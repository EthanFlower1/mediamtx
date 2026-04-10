package email_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications/email"
)

func openSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newStore(t *testing.T) *email.Store {
	t.Helper()
	db := openSQLite(t)
	s, err := email.NewStore(db, email.DialectSQLite)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := s.ApplyStubSchema(context.Background()); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	return s
}

func seedDomain(t *testing.T, s *email.Store, tenantID, domain string) email.Domain {
	t.Helper()
	now := time.Now().UTC()
	d := email.Domain{
		ID:                "dom-" + tenantID + "-" + domain,
		TenantID:          tenantID,
		Domain:            domain,
		SendGridSubuser:   "sg-" + tenantID,
		ActiveSelector:    email.SelectorS1,
		VerificationState: email.VerificationPending,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.CreateDomain(context.Background(), d); err != nil {
		t.Fatalf("create domain: %v", err)
	}
	return d
}

func TestStore_CreateAndGetDomain(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	d := seedDomain(t, s, "tenant-a", "alerts.acme-security.com")

	got, err := s.GetDomain(ctx, d.TenantID, d.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Domain != d.Domain {
		t.Errorf("domain = %q, want %q", got.Domain, d.Domain)
	}
	if got.ActiveSelector != email.SelectorS1 {
		t.Errorf("selector = %q, want s1", got.ActiveSelector)
	}
	if got.VerificationState != email.VerificationPending {
		t.Errorf("state = %q, want pending", got.VerificationState)
	}
}

// TestStore_TenantIsolation_Seam4 proves every read/write path in the
// store is tenant-scoped: a different tenant cannot see domain A's
// records even if they happen to know its id, and cannot accidentally
// overwrite them.
func TestStore_TenantIsolation_Seam4(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	dA := seedDomain(t, s, "tenant-a", "alerts.acme.com")
	_ = seedDomain(t, s, "tenant-b", "alerts.beta.com")

	// Tenant B lookup of tenant A's domain must fail.
	if _, err := s.GetDomain(ctx, "tenant-b", dA.ID); !errors.Is(err, email.ErrDomainNotFound) {
		t.Errorf("cross-tenant GetDomain: err = %v, want ErrDomainNotFound", err)
	}

	// ListDomains for tenant A must return exactly 1 (not see tenant B).
	listA, err := s.ListDomains(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("list A: %v", err)
	}
	if len(listA) != 1 || listA[0].Domain != "alerts.acme.com" {
		t.Errorf("ListDomains(A) = %+v, want exactly alerts.acme.com", listA)
	}

	// ListDomains for tenant B must return exactly 1 (not see tenant A).
	listB, err := s.ListDomains(ctx, "tenant-b")
	if err != nil {
		t.Fatalf("list B: %v", err)
	}
	if len(listB) != 1 || listB[0].Domain != "alerts.beta.com" {
		t.Errorf("ListDomains(B) = %+v, want exactly alerts.beta.com", listB)
	}

	// Cross-tenant UpdateVerificationState must return not-found
	// rather than silently touching tenant A's row.
	now := time.Now().UTC()
	err = s.UpdateVerificationState(ctx, "tenant-b", dA.ID,
		email.VerificationVerified, &now, &now, &now, now)
	if !errors.Is(err, email.ErrDomainNotFound) {
		t.Errorf("cross-tenant Update: err = %v, want ErrDomainNotFound", err)
	}

	// Confirm tenant A's state is unchanged.
	gotA, err := s.GetDomain(ctx, "tenant-a", dA.ID)
	if err != nil {
		t.Fatalf("re-get A: %v", err)
	}
	if gotA.VerificationState != email.VerificationPending {
		t.Errorf("tenant A state = %q, want pending (cross-tenant update leaked)", gotA.VerificationState)
	}

	// Missing-tenant queries return ErrMissingTenant.
	if _, err := s.GetDomain(ctx, "", dA.ID); !errors.Is(err, email.ErrMissingTenant) {
		t.Errorf("empty tenant GetDomain: err = %v, want ErrMissingTenant", err)
	}
	if _, err := s.ListDomains(ctx, ""); !errors.Is(err, email.ErrMissingTenant) {
		t.Errorf("empty tenant ListDomains: err = %v, want ErrMissingTenant", err)
	}
}

func TestStore_UpdateVerificationState(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	d := seedDomain(t, s, "tenant-a", "alerts.acme.com")
	now := time.Now().UTC()

	if err := s.UpdateVerificationState(ctx, d.TenantID, d.ID,
		email.VerificationVerified, &now, &now, &now, now); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := s.GetDomain(ctx, d.TenantID, d.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.VerificationState != email.VerificationVerified {
		t.Errorf("state = %q, want verified", got.VerificationState)
	}
	if got.SPFVerifiedAt == nil || got.DKIMVerifiedAt == nil || got.DMARCVerifiedAt == nil {
		t.Errorf("verified_at columns should all be set, got %+v", got)
	}
}

func TestStore_InsertAndGetDKIMKey(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	d := seedDomain(t, s, "tenant-a", "alerts.acme.com")
	now := time.Now().UTC()

	key := email.DKIMKey{
		ID:               "key-1",
		TenantID:         d.TenantID,
		DomainID:         d.ID,
		Selector:         email.SelectorS1,
		PublicKeyPEM:     "-----BEGIN PUBLIC KEY-----\nAAAA\n-----END PUBLIC KEY-----",
		CryptostoreKeyID: "cs-key-abc",
		KeySizeBits:      2048,
		Status:           "active",
		CreatedAt:        now,
	}
	if err := s.InsertDKIMKey(ctx, key); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := s.GetDKIMKey(ctx, d.TenantID, d.ID, email.SelectorS1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.CryptostoreKeyID != "cs-key-abc" {
		t.Errorf("cryptostore_key_id = %q, want cs-key-abc", got.CryptostoreKeyID)
	}
	if got.KeySizeBits != 2048 {
		t.Errorf("key_size_bits = %d, want 2048", got.KeySizeBits)
	}
}

func TestStore_InsertDKIMKey_RequiresCryptostoreID(t *testing.T) {
	// This test guards against a regression where someone "just
	// persists the private key in the table" to unblock local dev.
	// KAI-251 is the only legal home for DKIM private keys.
	s := newStore(t)
	ctx := context.Background()
	d := seedDomain(t, s, "tenant-a", "alerts.acme.com")

	err := s.InsertDKIMKey(ctx, email.DKIMKey{
		ID:           "key-x",
		TenantID:     d.TenantID,
		DomainID:     d.ID,
		Selector:     email.SelectorS1,
		PublicKeyPEM: "-----BEGIN PUBLIC KEY-----\nAAAA\n-----END PUBLIC KEY-----",
		// CryptostoreKeyID intentionally empty.
		KeySizeBits: 2048,
		Status:      "active",
		CreatedAt:   time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("insert with empty CryptostoreKeyID must fail")
	}
}
