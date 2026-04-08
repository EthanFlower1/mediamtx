package db_test

import (
	"context"
	"path/filepath"
	"testing"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/db/fixtures"
)

// openTestDB opens a fresh SQLite-backed cloud DB in the test's temp dir and
// runs migrations. All CRUD in these tests exercises the same helpers used in
// production; only the dialect differs. Postgres-specific features (JSONB
// operators, partitioned tables, pg_partman) are validated manually against a
// local Postgres container in KAI-216 integration tests.
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

func TestMigrationsApplyInOrder(t *testing.T) {
	d := openTestDB(t)

	versions, err := d.AppliedVersions(context.Background())
	if err != nil {
		t.Fatalf("applied versions: %v", err)
	}
	// 0001..0005 are SQLite-compatible. 0006 is the postgres-only audit_log
	// parent table; its body is stripped but the version row is still recorded.
	// 0007 (lpr_watchlists) and 0008 (cameras_lpr_enabled) are postgres-only
	// no-ops in SQLite. 0009..0013 are the KAI-249 directory schema: recorders,
	// recording_schedules, retention_policies, cameras, and camera_segment_index
	// (postgres partition block stripped; SQLite fallback table created).
	// 0014 is KAI-254 ai_events (postgres-only partition stripped) +
	// camera_state + segment_index_stub (SQLite-compatible).
	// 0015 is KAI-362 billing column tightening (postgres-only ALTERs; SQLite no-op).
	// 0016 is KAI-364 per-tenant usage_events + usage_aggregates metering tables
	// (postgres partman block stripped in SQLite; fallback tables created).
	// 0017 is KAI-357 integrator_email_domains + dkim_keys (per-tenant sender
	// domain + DKIM keypair metadata; private key bytes live in KAI-251 cryptostore).
	// 0018 is KAI-429 behavioral_config (SQLite-compatible; JSONB → TEXT,
	// TIMESTAMPTZ → DATETIME, BOOLEAN → INTEGER via translateToSQLite).
	want := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18}
	if len(versions) != len(want) {
		t.Fatalf("applied versions = %v, want %v", versions, want)
	}
	for i, v := range want {
		if versions[i] != v {
			t.Errorf("versions[%d] = %d, want %d", i, versions[i], v)
		}
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	d := openTestDB(t)

	// Re-running migrations on an already-migrated DB should no-op.
	if err := d.Migrate(context.Background()); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
}

func TestSeedFixturesAndTenantScopedReads(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	if err := fixtures.Seed(ctx, d); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Integrator round-trip, region-scoped.
	i, err := d.GetIntegrator(ctx, fixtures.IntegratorAcmeID, clouddb.DefaultRegion)
	if err != nil {
		t.Fatalf("get integrator: %v", err)
	}
	if i.DisplayName != "Acme Security Integrators" {
		t.Errorf("integrator display name = %q", i.DisplayName)
	}
	if i.BillingMode != "via_integrator" {
		t.Errorf("integrator billing_mode = %q", i.BillingMode)
	}

	// Wrong region → row is invisible (seam #9).
	if _, err := d.GetIntegrator(ctx, fixtures.IntegratorAcmeID, "eu-west-1"); err == nil {
		t.Error("expected not-found for wrong region, got nil error")
	}

	// Customer tenants — one via_integrator, one direct.
	beta, err := d.GetCustomerTenant(ctx, fixtures.CustomerTenantBetaID, clouddb.DefaultRegion)
	if err != nil {
		t.Fatalf("get beta: %v", err)
	}
	if beta.BillingMode != "via_integrator" || beta.HomeIntegratorID == nil {
		t.Errorf("beta billing_mode/home_integrator = %q/%v", beta.BillingMode, beta.HomeIntegratorID)
	}
	gamma, err := d.GetCustomerTenant(ctx, fixtures.CustomerTenantGammaID, clouddb.DefaultRegion)
	if err != nil {
		t.Fatalf("get gamma: %v", err)
	}
	if gamma.BillingMode != "direct" || gamma.HomeIntegratorID != nil {
		t.Errorf("gamma billing_mode/home_integrator = %q/%v", gamma.BillingMode, gamma.HomeIntegratorID)
	}

	// Users scoped to Beta — exactly one result, and it's the Beta operator.
	betaRef := clouddb.TenantRef{
		Type:   clouddb.TenantCustomerTenant,
		ID:     fixtures.CustomerTenantBetaID,
		Region: clouddb.DefaultRegion,
	}
	users, err := d.ListUsersForTenant(ctx, betaRef)
	if err != nil {
		t.Fatalf("list users beta: %v", err)
	}
	if len(users) != 1 || users[0].ID != fixtures.UserBetaOperatorID {
		t.Fatalf("beta users = %+v, want exactly UserBetaOperatorID", users)
	}

	// Cross-tenant isolation: users under Acme (integrator tenant) must not
	// include the Beta operator (seam #4). This is the cheap local echo of the
	// multi-tenant isolation chaos test (KAI-235).
	acmeRef := clouddb.TenantRef{
		Type:   clouddb.TenantIntegrator,
		ID:     fixtures.IntegratorAcmeID,
		Region: clouddb.DefaultRegion,
	}
	acmeUsers, err := d.ListUsersForTenant(ctx, acmeRef)
	if err != nil {
		t.Fatalf("list users acme: %v", err)
	}
	for _, u := range acmeUsers {
		if u.ID == fixtures.UserBetaOperatorID {
			t.Fatal("Beta operator leaked into Acme integrator's user list")
		}
	}

	// Directories scoped to Beta.
	dirs, err := d.ListDirectoriesForTenant(ctx, betaRef)
	if err != nil {
		t.Fatalf("list dirs: %v", err)
	}
	if len(dirs) != 1 || dirs[0].ID != fixtures.DirectoryBetaSiteOneID {
		t.Fatalf("beta directories = %+v, want exactly DirectoryBetaSiteOneID", dirs)
	}

	// Directories scoped to Gamma (no directory seeded) — must return empty.
	gammaRef := clouddb.TenantRef{
		Type:   clouddb.TenantCustomerTenant,
		ID:     fixtures.CustomerTenantGammaID,
		Region: clouddb.DefaultRegion,
	}
	gammaDirs, err := d.ListDirectoriesForTenant(ctx, gammaRef)
	if err != nil {
		t.Fatalf("list dirs gamma: %v", err)
	}
	if len(gammaDirs) != 0 {
		t.Fatalf("gamma directories = %+v, want empty", gammaDirs)
	}
}

func TestTenantRefValidate(t *testing.T) {
	cases := []struct {
		name    string
		ref     clouddb.TenantRef
		wantErr bool
	}{
		{"empty", clouddb.TenantRef{}, true},
		{"missing id", clouddb.TenantRef{Type: clouddb.TenantIntegrator, Region: "us-east-2"}, true},
		{"missing region", clouddb.TenantRef{Type: clouddb.TenantIntegrator, ID: "x"}, true},
		{"bad type", clouddb.TenantRef{Type: "platform", ID: "x", Region: "us-east-2"}, true},
		{
			"ok",
			clouddb.TenantRef{Type: clouddb.TenantCustomerTenant, ID: "x", Region: "us-east-2"},
			false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.ref.Validate()
			if (err != nil) != c.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

func TestInsertUserRequiresTenant(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	// Without any integrator/customer row seeded, the tenant ref must still
	// be validated client-side before the INSERT runs. Missing region is a
	// programming error (seam #4).
	err := d.InsertUser(ctx, clouddb.TenantRef{}, clouddb.User{
		ID:    "00000000-0000-4000-8000-000000000000",
		Email: "nobody@example.test",
	})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}
