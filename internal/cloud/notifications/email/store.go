package email

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Store is the persistence layer for sender domains + DKIM key
// metadata. It is intentionally narrow: CRUD + tenant-scoped reads,
// no business logic. Verification state transitions and rotation
// sequencing live in [Service].
type Store struct {
	db      *sql.DB
	dialect Dialect
}

// NewStore constructs a Store. dialect must be DialectPostgres (prod)
// or DialectSQLite (tests).
func NewStore(db *sql.DB, dialect Dialect) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("email: NewStore: db required")
	}
	if dialect != DialectPostgres && dialect != DialectSQLite {
		return nil, ErrUnknownDialect
	}
	return &Store{db: db, dialect: dialect}, nil
}

// placeholder returns $n for postgres, ? for sqlite. Mirrors the
// pattern from internal/cloud/audit/sql.go and internal/cloud/metering.
func (s *Store) placeholder(n int) string {
	if s.dialect == DialectPostgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// ApplyStubSchema creates the integrator_email_domains + dkim_keys
// tables in the connected database. Exists so unit tests can run
// against an empty SQLite file without running the full cloud
// migration bundle. Production goes through the migrations at
// internal/cloud/db/migrations/0017_integrator_email_domains.up.sql.
func (s *Store) ApplyStubSchema(ctx context.Context) error {
	var ddl []string
	if s.dialect == DialectSQLite {
		ddl = []string{
			`CREATE TABLE IF NOT EXISTS integrator_email_domains (
				id                  TEXT PRIMARY KEY,
				tenant_id           TEXT NOT NULL,
				domain              TEXT NOT NULL,
				sendgrid_subuser    TEXT NOT NULL,
				active_selector     TEXT NOT NULL DEFAULT 's1',
				verification_state  TEXT NOT NULL DEFAULT 'pending',
				spf_verified_at     DATETIME,
				dkim_verified_at    DATETIME,
				dmarc_verified_at   DATETIME,
				last_checked_at     DATETIME,
				created_at          DATETIME NOT NULL,
				updated_at          DATETIME NOT NULL
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_integrator_email_domains_tenant_domain
				ON integrator_email_domains (tenant_id, domain)`,
			`CREATE TABLE IF NOT EXISTS dkim_keys (
				id                  TEXT PRIMARY KEY,
				tenant_id           TEXT NOT NULL,
				domain_id           TEXT NOT NULL,
				selector            TEXT NOT NULL,
				public_key_pem      TEXT NOT NULL,
				cryptostore_key_id  TEXT NOT NULL,
				key_size_bits       INTEGER NOT NULL,
				status              TEXT NOT NULL DEFAULT 'active',
				rotated_at          DATETIME,
				created_at          DATETIME NOT NULL
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_dkim_keys_domain_selector
				ON dkim_keys (domain_id, selector)`,
		}
	} else {
		ddl = []string{
			`CREATE TABLE IF NOT EXISTS integrator_email_domains (
				id                  TEXT PRIMARY KEY,
				tenant_id           UUID NOT NULL,
				domain              TEXT NOT NULL,
				sendgrid_subuser    TEXT NOT NULL,
				active_selector     TEXT NOT NULL DEFAULT 's1',
				verification_state  TEXT NOT NULL DEFAULT 'pending',
				spf_verified_at     TIMESTAMPTZ,
				dkim_verified_at    TIMESTAMPTZ,
				dmarc_verified_at   TIMESTAMPTZ,
				last_checked_at     TIMESTAMPTZ,
				created_at          TIMESTAMPTZ NOT NULL,
				updated_at          TIMESTAMPTZ NOT NULL
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_integrator_email_domains_tenant_domain
				ON integrator_email_domains (tenant_id, domain)`,
			`CREATE TABLE IF NOT EXISTS dkim_keys (
				id                  TEXT PRIMARY KEY,
				tenant_id           UUID NOT NULL,
				domain_id           TEXT NOT NULL,
				selector            TEXT NOT NULL,
				public_key_pem      TEXT NOT NULL,
				cryptostore_key_id  TEXT NOT NULL,
				key_size_bits       INTEGER NOT NULL,
				status              TEXT NOT NULL DEFAULT 'active',
				rotated_at          TIMESTAMPTZ,
				created_at          TIMESTAMPTZ NOT NULL
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_dkim_keys_domain_selector
				ON dkim_keys (domain_id, selector)`,
		}
	}
	for _, stmt := range ddl {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("email: apply stub schema: %w", err)
		}
	}
	return nil
}

// CreateDomain inserts a new sender domain for a tenant. It fails if
// the (tenant_id, domain) pair already exists.
func (s *Store) CreateDomain(ctx context.Context, d Domain) error {
	if strings.TrimSpace(d.TenantID) == "" {
		return ErrMissingTenant
	}
	if strings.TrimSpace(d.Domain) == "" {
		return ErrMissingDomain
	}
	if !ValidVerificationState(d.VerificationState) {
		return ErrVerificationState
	}
	if !ValidSelector(d.ActiveSelector) {
		return ErrSelectorInvalid
	}
	query := fmt.Sprintf(`
		INSERT INTO integrator_email_domains
			(id, tenant_id, domain, sendgrid_subuser, active_selector,
			 verification_state, created_at, updated_at)
		VALUES (%s, %s, %s, %s, %s, %s, %s, %s)`,
		s.placeholder(1), s.placeholder(2), s.placeholder(3), s.placeholder(4),
		s.placeholder(5), s.placeholder(6), s.placeholder(7), s.placeholder(8),
	)
	_, err := s.db.ExecContext(ctx, query,
		d.ID, d.TenantID, d.Domain, d.SendGridSubuser,
		string(d.ActiveSelector), string(d.VerificationState),
		d.CreatedAt, d.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("email: create domain: %w", err)
	}
	return nil
}

// GetDomain returns a single domain by (tenant_id, id). Tenant id is
// always the first WHERE predicate (Seam #4).
func (s *Store) GetDomain(ctx context.Context, tenantID, id string) (Domain, error) {
	if strings.TrimSpace(tenantID) == "" {
		return Domain{}, ErrMissingTenant
	}
	query := fmt.Sprintf(`
		SELECT id, tenant_id, domain, sendgrid_subuser, active_selector,
		       verification_state, spf_verified_at, dkim_verified_at,
		       dmarc_verified_at, last_checked_at, created_at, updated_at
		FROM integrator_email_domains
		WHERE tenant_id = %s AND id = %s`,
		s.placeholder(1), s.placeholder(2),
	)
	row := s.db.QueryRowContext(ctx, query, tenantID, id)
	return scanDomain(row)
}

// ListDomains returns all domains for a tenant.
func (s *Store) ListDomains(ctx context.Context, tenantID string) ([]Domain, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, ErrMissingTenant
	}
	query := fmt.Sprintf(`
		SELECT id, tenant_id, domain, sendgrid_subuser, active_selector,
		       verification_state, spf_verified_at, dkim_verified_at,
		       dmarc_verified_at, last_checked_at, created_at, updated_at
		FROM integrator_email_domains
		WHERE tenant_id = %s
		ORDER BY created_at ASC`,
		s.placeholder(1),
	)
	rows, err := s.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("email: list domains: %w", err)
	}
	defer rows.Close()

	var out []Domain
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpdateVerificationState transitions a domain's verification_state
// and updates the relevant *_verified_at timestamp. Tenant id is the
// first WHERE predicate.
func (s *Store) UpdateVerificationState(
	ctx context.Context,
	tenantID, domainID string,
	state VerificationState,
	spfAt, dkimAt, dmarcAt *time.Time,
	checkedAt time.Time,
) error {
	if strings.TrimSpace(tenantID) == "" {
		return ErrMissingTenant
	}
	if !ValidVerificationState(state) {
		return ErrVerificationState
	}
	query := fmt.Sprintf(`
		UPDATE integrator_email_domains
		SET verification_state = %s,
		    spf_verified_at    = %s,
		    dkim_verified_at   = %s,
		    dmarc_verified_at  = %s,
		    last_checked_at    = %s,
		    updated_at         = %s
		WHERE tenant_id = %s AND id = %s`,
		s.placeholder(1), s.placeholder(2), s.placeholder(3),
		s.placeholder(4), s.placeholder(5), s.placeholder(6),
		s.placeholder(7), s.placeholder(8),
	)
	res, err := s.db.ExecContext(ctx, query,
		string(state), spfAt, dkimAt, dmarcAt, checkedAt, checkedAt,
		tenantID, domainID,
	)
	if err != nil {
		return fmt.Errorf("email: update verification state: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrDomainNotFound
	}
	return nil
}

// InsertDKIMKey persists DKIM key metadata. Caller MUST have already
// stored the matching private key in the cryptostore and passed the
// cryptostore key id.
func (s *Store) InsertDKIMKey(ctx context.Context, k DKIMKey) error {
	if strings.TrimSpace(k.TenantID) == "" {
		return ErrMissingTenant
	}
	if !ValidSelector(k.Selector) {
		return ErrSelectorInvalid
	}
	if strings.TrimSpace(k.CryptostoreKeyID) == "" {
		return fmt.Errorf("email: DKIMKey.CryptostoreKeyID required (private keys must live in KAI-251)")
	}
	query := fmt.Sprintf(`
		INSERT INTO dkim_keys
			(id, tenant_id, domain_id, selector, public_key_pem,
			 cryptostore_key_id, key_size_bits, status, created_at)
		VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)`,
		s.placeholder(1), s.placeholder(2), s.placeholder(3),
		s.placeholder(4), s.placeholder(5), s.placeholder(6),
		s.placeholder(7), s.placeholder(8), s.placeholder(9),
	)
	_, err := s.db.ExecContext(ctx, query,
		k.ID, k.TenantID, k.DomainID, string(k.Selector),
		k.PublicKeyPEM, k.CryptostoreKeyID, k.KeySizeBits,
		k.Status, k.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("email: insert dkim key: %w", err)
	}
	return nil
}

// GetDKIMKey returns a single DKIM key by (tenant_id, domain_id, selector).
func (s *Store) GetDKIMKey(ctx context.Context, tenantID, domainID string, selector Selector) (DKIMKey, error) {
	if strings.TrimSpace(tenantID) == "" {
		return DKIMKey{}, ErrMissingTenant
	}
	if !ValidSelector(selector) {
		return DKIMKey{}, ErrSelectorInvalid
	}
	query := fmt.Sprintf(`
		SELECT id, tenant_id, domain_id, selector, public_key_pem,
		       cryptostore_key_id, key_size_bits, status, rotated_at, created_at
		FROM dkim_keys
		WHERE tenant_id = %s AND domain_id = %s AND selector = %s`,
		s.placeholder(1), s.placeholder(2), s.placeholder(3),
	)
	row := s.db.QueryRowContext(ctx, query, tenantID, domainID, string(selector))
	return scanDKIMKey(row)
}

// rowScanner lets us scan from *sql.Row and *sql.Rows with the same code.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanDomain(r rowScanner) (Domain, error) {
	var d Domain
	var selector, state string
	var spfAt, dkimAt, dmarcAt, checkedAt sql.NullTime
	err := r.Scan(
		&d.ID, &d.TenantID, &d.Domain, &d.SendGridSubuser,
		&selector, &state, &spfAt, &dkimAt, &dmarcAt, &checkedAt,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return Domain{}, ErrDomainNotFound
		}
		return Domain{}, fmt.Errorf("email: scan domain: %w", err)
	}
	d.ActiveSelector = Selector(selector)
	d.VerificationState = VerificationState(state)
	if spfAt.Valid {
		t := spfAt.Time
		d.SPFVerifiedAt = &t
	}
	if dkimAt.Valid {
		t := dkimAt.Time
		d.DKIMVerifiedAt = &t
	}
	if dmarcAt.Valid {
		t := dmarcAt.Time
		d.DMARCVerifiedAt = &t
	}
	if checkedAt.Valid {
		t := checkedAt.Time
		d.LastCheckedAt = &t
	}
	return d, nil
}

func scanDKIMKey(r rowScanner) (DKIMKey, error) {
	var k DKIMKey
	var selector string
	var rotatedAt sql.NullTime
	err := r.Scan(
		&k.ID, &k.TenantID, &k.DomainID, &selector, &k.PublicKeyPEM,
		&k.CryptostoreKeyID, &k.KeySizeBits, &k.Status, &rotatedAt, &k.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return DKIMKey{}, fmt.Errorf("email: dkim key not found: %w", err)
		}
		return DKIMKey{}, fmt.Errorf("email: scan dkim key: %w", err)
	}
	k.Selector = Selector(selector)
	if rotatedAt.Valid {
		t := rotatedAt.Time
		k.RotatedAt = &t
	}
	return k, nil
}
