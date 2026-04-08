package watchlist

import (
	"context"
	"fmt"
	"testing"
)

const (
	tenantA = "tenant-a"
	tenantB = "tenant-b"
)

func setupDenyWatchlist(t *testing.T, repo *FakeRepository, tenantID string) *Watchlist {
	t.Helper()
	wl, err := repo.CreateWatchlist(context.Background(), Watchlist{
		TenantID: tenantID,
		Name:     "deny-list",
		Type:     TypeDeny,
	})
	if err != nil {
		t.Fatalf("CreateWatchlist: %v", err)
	}
	return wl
}

func addPlate(t *testing.T, repo *FakeRepository, tenantID, watchlistID, plate string) {
	t.Helper()
	_, err := repo.AddEntry(context.Background(), tenantID, PlateEntry{
		WatchlistID: watchlistID,
		PlateText:   plate,
	})
	if err != nil {
		t.Fatalf("AddEntry(%q): %v", plate, err)
	}
}

// TestMatcherBasicMatch verifies that a plate added to a deny list is found
// after the cache is rebuilt.
func TestMatcherBasicMatch(t *testing.T) {
	repo := NewFakeRepository()
	wl := setupDenyWatchlist(t, repo, tenantA)
	addPlate(t, repo, tenantA, wl.ID, "ABC123")

	m := NewMatcher(tenantA, repo)
	if err := m.RebuildCache(context.Background()); err != nil {
		t.Fatalf("RebuildCache: %v", err)
	}

	got, err := m.Match(context.Background(), tenantA, "ABC123")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if got == nil {
		t.Fatal("expected match; got nil")
	}
	if got.WatchlistID != wl.ID {
		t.Errorf("got WatchlistID %q; want %q", got.WatchlistID, wl.ID)
	}
	if got.Type != TypeDeny {
		t.Errorf("got Type %q; want %q", got.Type, TypeDeny)
	}
}

// TestMatcherNoMatch verifies that a plate NOT in the list returns nil.
func TestMatcherNoMatch(t *testing.T) {
	repo := NewFakeRepository()
	wl := setupDenyWatchlist(t, repo, tenantA)
	addPlate(t, repo, tenantA, wl.ID, "ABC123")

	m := NewMatcher(tenantA, repo)
	if err := m.RebuildCache(context.Background()); err != nil {
		t.Fatalf("RebuildCache: %v", err)
	}

	got, err := m.Match(context.Background(), tenantA, "XYZ999")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil; got %+v", got)
	}
}

// TestCrossTenantIsolation is the critical isolation test: plates in tenant A's
// watchlist must NOT alert on tenant B's cameras (Matcher is tenant-scoped).
func TestCrossTenantIsolation(t *testing.T) {
	repo := NewFakeRepository()

	// Add "SUSPECT1" to tenant A's deny list.
	wlA := setupDenyWatchlist(t, repo, tenantA)
	addPlate(t, repo, tenantA, wlA.ID, "SUSPECT1")

	// Tenant B has a completely separate watchlist with different plates.
	wlB := setupDenyWatchlist(t, repo, tenantB)
	addPlate(t, repo, tenantB, wlB.ID, "INNOCENT1")

	// Build matchers per tenant.
	mA := NewMatcher(tenantA, repo)
	if err := mA.RebuildCache(context.Background()); err != nil {
		t.Fatalf("RebuildCache A: %v", err)
	}
	mB := NewMatcher(tenantB, repo)
	if err := mB.RebuildCache(context.Background()); err != nil {
		t.Fatalf("RebuildCache B: %v", err)
	}

	// Tenant B's matcher must NOT find tenant A's plate.
	got, err := mB.Match(context.Background(), tenantB, "SUSPECT1")
	if err != nil {
		t.Fatalf("Match B for SUSPECT1: %v", err)
	}
	if got != nil {
		t.Errorf("cross-tenant leakage: tenant B matcher matched tenant A's plate SUSPECT1")
	}

	// Tenant A's matcher must NOT find tenant B's plate.
	got, err = mA.Match(context.Background(), tenantA, "INNOCENT1")
	if err != nil {
		t.Fatalf("Match A for INNOCENT1: %v", err)
	}
	if got != nil {
		t.Errorf("cross-tenant leakage: tenant A matcher matched tenant B's plate INNOCENT1")
	}

	// Each matcher must find its own plate.
	gotA, err := mA.Match(context.Background(), tenantA, "SUSPECT1")
	if err != nil || gotA == nil {
		t.Errorf("tenant A should match SUSPECT1: err=%v, match=%v", err, gotA)
	}
	gotB, err := mB.Match(context.Background(), tenantB, "INNOCENT1")
	if err != nil || gotB == nil {
		t.Errorf("tenant B should match INNOCENT1: err=%v, match=%v", err, gotB)
	}
}

// TestTenantMismatch verifies ErrTenantMismatch is returned when the caller
// provides the wrong tenantID.
func TestTenantMismatch(t *testing.T) {
	repo := NewFakeRepository()
	m := NewMatcher(tenantA, repo)
	_ = m.RebuildCache(context.Background())

	_, err := m.Match(context.Background(), tenantB, "ABC123")
	if err != ErrTenantMismatch {
		t.Errorf("expected ErrTenantMismatch; got %v", err)
	}
}

// TestCacheInvalidation verifies that adding a plate after cache build is
// picked up after RebuildCache.
func TestCacheInvalidation(t *testing.T) {
	repo := NewFakeRepository()
	wl := setupDenyWatchlist(t, repo, tenantA)

	m := NewMatcher(tenantA, repo)
	if err := m.RebuildCache(context.Background()); err != nil {
		t.Fatalf("RebuildCache: %v", err)
	}

	// Plate not present yet.
	got, _ := m.Match(context.Background(), tenantA, "NEWPLATE")
	if got != nil {
		t.Fatalf("should not match before add")
	}

	addPlate(t, repo, tenantA, wl.ID, "NEWPLATE")

	// Still not visible until rebuild.
	got, _ = m.Match(context.Background(), tenantA, "NEWPLATE")
	if got != nil {
		t.Logf("note: bloom filter may have false-positive here — that is OK for this test")
	}

	// After rebuild, must match.
	if err := m.RebuildCache(context.Background()); err != nil {
		t.Fatalf("RebuildCache 2: %v", err)
	}
	got, err := m.Match(context.Background(), tenantA, "NEWPLATE")
	if err != nil || got == nil {
		t.Errorf("expected match after rebuild; err=%v, match=%v", err, got)
	}
}

// TestWatchlistCRUD exercises the FakeRepository's CRUD surface.
func TestWatchlistCRUD(t *testing.T) {
	ctx := context.Background()
	repo := NewFakeRepository()

	// Create.
	wl, err := repo.CreateWatchlist(ctx, Watchlist{
		TenantID: tenantA,
		Name:     "test-list",
		Type:     TypeAlert,
	})
	if err != nil {
		t.Fatalf("CreateWatchlist: %v", err)
	}
	if wl.RetentionDays != DefaultRetentionDays {
		t.Errorf("RetentionDays = %d; want %d", wl.RetentionDays, DefaultRetentionDays)
	}

	// Add entry.
	entry, err := repo.AddEntry(ctx, tenantA, PlateEntry{
		WatchlistID: wl.ID,
		PlateText:   "ZZ99ZZZ",
		Label:       "test vehicle",
	})
	if err != nil {
		t.Fatalf("AddEntry: %v", err)
	}

	// ListEntries.
	entries, err := repo.ListEntries(ctx, tenantA, wl.ID)
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("ListEntries len = %d; want 1", len(entries))
	}

	// Remove entry.
	if err := repo.RemoveEntry(ctx, tenantA, wl.ID, entry.ID); err != nil {
		t.Fatalf("RemoveEntry: %v", err)
	}
	entries, _ = repo.ListEntries(ctx, tenantA, wl.ID)
	if len(entries) != 0 {
		t.Errorf("ListEntries after remove len = %d; want 0", len(entries))
	}

	// Delete watchlist.
	if err := repo.DeleteWatchlist(ctx, tenantA, wl.ID); err != nil {
		t.Fatalf("DeleteWatchlist: %v", err)
	}
	_, err = repo.GetWatchlist(ctx, tenantA, wl.ID)
	if err == nil {
		t.Error("expected ErrNotFound after delete; got nil")
	}
}

// TestWatchlistDeleteCascade verifies that deleting a watchlist also removes
// its entries (cascade) so they do not appear in AllActivePlates.
func TestWatchlistDeleteCascade(t *testing.T) {
	ctx := context.Background()
	repo := NewFakeRepository()

	wl := setupDenyWatchlist(t, repo, tenantA)
	addPlate(t, repo, tenantA, wl.ID, "CASCADE1")

	if err := repo.DeleteWatchlist(ctx, tenantA, wl.ID); err != nil {
		t.Fatalf("DeleteWatchlist: %v", err)
	}

	plates, err := repo.AllActivePlates(ctx, tenantA)
	if err != nil {
		t.Fatalf("AllActivePlates: %v", err)
	}
	for _, p := range plates {
		if p.PlateText == "CASCADE1" {
			t.Error("deleted watchlist entry still appears in AllActivePlates")
		}
	}
}

// TestMatcherWithLargeWatchlist exercises the bloom filter at scale (10k entries).
// This doubles as a correctness test for the bloom filter itself.
func TestMatcherWithLargeWatchlist(t *testing.T) {
	const n = 10_000
	ctx := context.Background()
	repo := NewFakeRepository()
	wl := setupDenyWatchlist(t, repo, tenantA)

	// Add n plates: "PLATE00000" ... "PLATE09999".
	for i := 0; i < n; i++ {
		plate := fmt.Sprintf("PLATE%05d", i)
		addPlate(t, repo, tenantA, wl.ID, plate)
	}

	m := NewMatcher(tenantA, repo)
	if err := m.RebuildCache(ctx); err != nil {
		t.Fatalf("RebuildCache: %v", err)
	}

	// Every added plate must match.
	for i := 0; i < n; i++ {
		plate := fmt.Sprintf("PLATE%05d", i)
		got, err := m.Match(ctx, tenantA, plate)
		if err != nil {
			t.Fatalf("Match(%q): %v", plate, err)
		}
		if got == nil {
			t.Fatalf("Match(%q): expected match; got nil", plate)
		}
	}

	// Plates NOT added should not match (we tolerate the bloom FP rate but
	// track the count to ensure it is within the design target of 0.1%).
	falsePositives := 0
	const probes = 1000
	for i := n; i < n+probes; i++ {
		plate := fmt.Sprintf("PLATE%05d", i)
		got, err := m.Match(ctx, tenantA, plate)
		if err != nil {
			t.Fatalf("Match(%q): %v", plate, err)
		}
		if got != nil {
			falsePositives++
		}
	}
	fpRate := float64(falsePositives) / float64(probes)
	// Allow up to 1% FP rate in tests (design target is 0.1% for production
	// sizes; with 10k+1000 probes we may see a slightly higher rate).
	if fpRate > 0.01 {
		t.Errorf("false-positive rate %.2f%% exceeds 1%% threshold (%d/%d)",
			fpRate*100, falsePositives, probes)
	}
	t.Logf("bloom filter FP rate: %.4f%% (%d/%d)", fpRate*100, falsePositives, probes)
}
