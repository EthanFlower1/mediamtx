package db

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// TenantRefType is the polymorphic discriminator for "who owns this row".
// Seam #4: every tenant-scoped helper requires a (TenantRefType, id) pair —
// callers cannot read or write a tenant-scoped row without naming its tenant.
type TenantRefType string

const (
	TenantIntegrator     TenantRefType = "integrator"
	TenantCustomerTenant TenantRefType = "customer_tenant"
)

// TenantRef bundles the (type, id, region) triple. Region is included so
// queries can stay region-scoped as soon as region #2 lights up.
type TenantRef struct {
	Type   TenantRefType
	ID     string
	Region string
}

// Validate checks that a TenantRef is fully populated. An empty tenant ref is
// always a programming error — fail loudly rather than silently querying
// across tenants.
func (t TenantRef) Validate() error {
	if t.Type != TenantIntegrator && t.Type != TenantCustomerTenant {
		return fmt.Errorf("invalid tenant ref type %q", t.Type)
	}
	if t.ID == "" {
		return errors.New("tenant ref id is required")
	}
	if t.Region == "" {
		return errors.New("tenant ref region is required")
	}
	return nil
}

// Integrator is a row in the `integrators` table.
type Integrator struct {
	ID                       string
	ParentIntegratorID       *string
	DisplayName              string
	LegalName                *string
	ContactEmail             *string
	BillingMode              string
	WholesaleDiscountPercent float64
	BrandConfigID            *string
	BillingAccountID         *string
	Status                   string
	Region                   string
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// CustomerTenant is a row in the `customer_tenants` table.
type CustomerTenant struct {
	ID                 string
	DisplayName        string
	BillingMode        string
	HomeIntegratorID   *string
	SignupSource       *string
	BrandOverrideID    *string
	BillingAccountID   *string
	Status             string
	Region             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// User is a row in the `users` table. TenantRefType / TenantRefID form the
// polymorphic pointer to either an integrator or a customer tenant.
type User struct {
	ID             string
	TenantRefType  TenantRefType
	TenantRefID    string
	Email          string
	DisplayName    *string
	Status         string
	ZitadelUserID  *string
	Region         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// OnPremDirectory is a row in the `on_prem_directories` table.
type OnPremDirectory struct {
	ID               string
	CustomerTenantID string
	DisplayName      string
	SiteLabel        string
	DeploymentMode   string
	PairedAt         *time.Time
	LastCheckinAt    *time.Time
	SoftwareVersion  *string
	Status           string
	Region           string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// CustomerIntegratorRelationship is a row in the join table.
type CustomerIntegratorRelationship struct {
	CustomerTenantID  string
	IntegratorID      string
	ScopedPermissions string // raw JSON; parsed by callers
	RoleTemplate      string
	MarkupPercent     float64
	Status            string
	GrantedAt         time.Time
	GrantedByUserID   *string
	RevokedAt         *time.Time
}

// placeholder returns the driver-specific placeholder for position i (1-based).
func (d *DB) placeholder(i int) string {
	if d.dialect == DialectPostgres {
		return fmt.Sprintf("$%d", i)
	}
	return "?"
}

// ---------- integrators ----------

// InsertIntegrator inserts a new integrator. Caller supplies a UUID.
func (d *DB) InsertIntegrator(ctx context.Context, i Integrator) error {
	if i.ID == "" {
		return errors.New("integrator id is required")
	}
	if i.Region == "" {
		i.Region = DefaultRegion
	}
	q := fmt.Sprintf(
		`INSERT INTO integrators
            (id, parent_integrator_id, display_name, legal_name, contact_email,
             billing_mode, wholesale_discount_percent, brand_config_id,
             billing_account_id, status, region)
         VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)`,
		d.placeholder(1), d.placeholder(2), d.placeholder(3), d.placeholder(4),
		d.placeholder(5), d.placeholder(6), d.placeholder(7), d.placeholder(8),
		d.placeholder(9), d.placeholder(10), d.placeholder(11),
	)
	_, err := d.ExecContext(ctx, q,
		i.ID, i.ParentIntegratorID, i.DisplayName, i.LegalName, i.ContactEmail,
		i.BillingMode, i.WholesaleDiscountPercent, i.BrandConfigID,
		i.BillingAccountID, i.Status, i.Region,
	)
	return err
}

// GetIntegrator fetches an integrator by id, scoped by region. Seam #9: the
// region must match, otherwise the row is treated as non-existent.
func (d *DB) GetIntegrator(ctx context.Context, id, region string) (*Integrator, error) {
	q := fmt.Sprintf(
		`SELECT id, parent_integrator_id, display_name, legal_name, contact_email,
                billing_mode, wholesale_discount_percent, brand_config_id,
                billing_account_id, status, region, created_at, updated_at
         FROM integrators
         WHERE id = %s AND region = %s`,
		d.placeholder(1), d.placeholder(2),
	)
	row := d.QueryRowContext(ctx, q, id, region)
	var i Integrator
	if err := row.Scan(
		&i.ID, &i.ParentIntegratorID, &i.DisplayName, &i.LegalName, &i.ContactEmail,
		&i.BillingMode, &i.WholesaleDiscountPercent, &i.BrandConfigID,
		&i.BillingAccountID, &i.Status, &i.Region, &i.CreatedAt, &i.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &i, nil
}

// ---------- customer_tenants ----------

// InsertCustomerTenant inserts a new customer tenant.
func (d *DB) InsertCustomerTenant(ctx context.Context, c CustomerTenant) error {
	if c.ID == "" {
		return errors.New("customer tenant id is required")
	}
	if c.Region == "" {
		c.Region = DefaultRegion
	}
	q := fmt.Sprintf(
		`INSERT INTO customer_tenants
            (id, display_name, billing_mode, home_integrator_id, signup_source,
             brand_override_id, billing_account_id, status, region)
         VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)`,
		d.placeholder(1), d.placeholder(2), d.placeholder(3), d.placeholder(4),
		d.placeholder(5), d.placeholder(6), d.placeholder(7), d.placeholder(8),
		d.placeholder(9),
	)
	_, err := d.ExecContext(ctx, q,
		c.ID, c.DisplayName, c.BillingMode, c.HomeIntegratorID, c.SignupSource,
		c.BrandOverrideID, c.BillingAccountID, c.Status, c.Region,
	)
	return err
}

// GetCustomerTenant fetches a customer tenant, region-scoped.
func (d *DB) GetCustomerTenant(ctx context.Context, id, region string) (*CustomerTenant, error) {
	q := fmt.Sprintf(
		`SELECT id, display_name, billing_mode, home_integrator_id, signup_source,
                brand_override_id, billing_account_id, status, region,
                created_at, updated_at
         FROM customer_tenants
         WHERE id = %s AND region = %s`,
		d.placeholder(1), d.placeholder(2),
	)
	row := d.QueryRowContext(ctx, q, id, region)
	var c CustomerTenant
	if err := row.Scan(
		&c.ID, &c.DisplayName, &c.BillingMode, &c.HomeIntegratorID, &c.SignupSource,
		&c.BrandOverrideID, &c.BillingAccountID, &c.Status, &c.Region,
		&c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &c, nil
}

// ---------- users ----------

// InsertUser inserts a user under a given tenant. The tenant ref is required
// (seam #4): no user can exist without a named tenant.
func (d *DB) InsertUser(ctx context.Context, tenant TenantRef, u User) error {
	if err := tenant.Validate(); err != nil {
		return err
	}
	if u.ID == "" {
		return errors.New("user id is required")
	}
	u.TenantRefType = tenant.Type
	u.TenantRefID = tenant.ID
	u.Region = tenant.Region

	q := fmt.Sprintf(
		`INSERT INTO users
            (id, tenant_ref_type, tenant_ref_id, email, display_name,
             status, zitadel_user_id, region)
         VALUES (%s, %s, %s, %s, %s, %s, %s, %s)`,
		d.placeholder(1), d.placeholder(2), d.placeholder(3), d.placeholder(4),
		d.placeholder(5), d.placeholder(6), d.placeholder(7), d.placeholder(8),
	)
	_, err := d.ExecContext(ctx, q,
		u.ID, string(u.TenantRefType), u.TenantRefID, u.Email, u.DisplayName,
		u.Status, u.ZitadelUserID, u.Region,
	)
	return err
}

// ListUsersForTenant lists users in a tenant. Seam #4: callers MUST pass a
// fully-populated TenantRef; cross-tenant reads are impossible through this
// helper and must go through the scoped-token flow (KAI-224).
func (d *DB) ListUsersForTenant(ctx context.Context, tenant TenantRef) ([]User, error) {
	if err := tenant.Validate(); err != nil {
		return nil, err
	}
	q := fmt.Sprintf(
		`SELECT id, tenant_ref_type, tenant_ref_id, email, display_name,
                status, zitadel_user_id, region, created_at, updated_at
         FROM users
         WHERE region = %s AND tenant_ref_type = %s AND tenant_ref_id = %s
         ORDER BY created_at ASC`,
		d.placeholder(1), d.placeholder(2), d.placeholder(3),
	)
	rows, err := d.QueryContext(ctx, q, tenant.Region, string(tenant.Type), tenant.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []User
	for rows.Next() {
		var u User
		var trType string
		if err := rows.Scan(
			&u.ID, &trType, &u.TenantRefID, &u.Email, &u.DisplayName,
			&u.Status, &u.ZitadelUserID, &u.Region, &u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		u.TenantRefType = TenantRefType(trType)
		out = append(out, u)
	}
	return out, rows.Err()
}

// ---------- customer_integrator_relationships ----------

// InsertCustomerIntegratorRelationship records a new grant. Tenant-scoped to
// the customer side: the caller names the customer tenant it's granting access
// within.
func (d *DB) InsertCustomerIntegratorRelationship(
	ctx context.Context,
	customer TenantRef,
	r CustomerIntegratorRelationship,
) error {
	if err := customer.Validate(); err != nil {
		return err
	}
	if customer.Type != TenantCustomerTenant {
		return fmt.Errorf("InsertCustomerIntegratorRelationship: tenant ref must be customer_tenant, got %q", customer.Type)
	}
	r.CustomerTenantID = customer.ID
	if r.IntegratorID == "" {
		return errors.New("integrator id is required")
	}
	if r.ScopedPermissions == "" {
		r.ScopedPermissions = "{}"
	}

	q := fmt.Sprintf(
		`INSERT INTO customer_integrator_relationships
            (customer_tenant_id, integrator_id, scoped_permissions, role_template,
             markup_percent, status, granted_at, granted_by_user_id, revoked_at)
         VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)`,
		d.placeholder(1), d.placeholder(2), d.placeholder(3), d.placeholder(4),
		d.placeholder(5), d.placeholder(6), d.placeholder(7), d.placeholder(8),
		d.placeholder(9),
	)
	granted := r.GrantedAt
	if granted.IsZero() {
		granted = time.Now().UTC()
	}
	_, err := d.ExecContext(ctx, q,
		r.CustomerTenantID, r.IntegratorID, r.ScopedPermissions, r.RoleTemplate,
		r.MarkupPercent, r.Status, granted, r.GrantedByUserID, r.RevokedAt,
	)
	return err
}

// ---------- on_prem_directories ----------

// InsertOnPremDirectory registers a new Directory under a customer tenant.
func (d *DB) InsertOnPremDirectory(
	ctx context.Context,
	customer TenantRef,
	dir OnPremDirectory,
) error {
	if err := customer.Validate(); err != nil {
		return err
	}
	if customer.Type != TenantCustomerTenant {
		return fmt.Errorf("InsertOnPremDirectory: tenant ref must be customer_tenant, got %q", customer.Type)
	}
	if dir.ID == "" {
		return errors.New("directory id is required")
	}
	dir.CustomerTenantID = customer.ID
	dir.Region = customer.Region
	if dir.DeploymentMode == "" {
		dir.DeploymentMode = "cloud_connected"
	}
	if dir.Status == "" {
		dir.Status = "pending_pairing"
	}

	q := fmt.Sprintf(
		`INSERT INTO on_prem_directories
            (id, customer_tenant_id, display_name, site_label, deployment_mode,
             paired_at, last_checkin_at, software_version, status, region)
         VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)`,
		d.placeholder(1), d.placeholder(2), d.placeholder(3), d.placeholder(4),
		d.placeholder(5), d.placeholder(6), d.placeholder(7), d.placeholder(8),
		d.placeholder(9), d.placeholder(10),
	)
	_, err := d.ExecContext(ctx, q,
		dir.ID, dir.CustomerTenantID, dir.DisplayName, dir.SiteLabel, dir.DeploymentMode,
		dir.PairedAt, dir.LastCheckinAt, dir.SoftwareVersion, dir.Status, dir.Region,
	)
	return err
}

// ListDirectoriesForTenant lists directories for a customer tenant. Seam #4.
func (d *DB) ListDirectoriesForTenant(ctx context.Context, customer TenantRef) ([]OnPremDirectory, error) {
	if err := customer.Validate(); err != nil {
		return nil, err
	}
	if customer.Type != TenantCustomerTenant {
		return nil, fmt.Errorf("ListDirectoriesForTenant: tenant ref must be customer_tenant, got %q", customer.Type)
	}
	q := fmt.Sprintf(
		`SELECT id, customer_tenant_id, display_name, site_label, deployment_mode,
                paired_at, last_checkin_at, software_version, status, region,
                created_at, updated_at
         FROM on_prem_directories
         WHERE region = %s AND customer_tenant_id = %s
         ORDER BY created_at ASC`,
		d.placeholder(1), d.placeholder(2),
	)
	rows, err := d.QueryContext(ctx, q, customer.Region, customer.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OnPremDirectory
	for rows.Next() {
		var r OnPremDirectory
		if err := rows.Scan(
			&r.ID, &r.CustomerTenantID, &r.DisplayName, &r.SiteLabel, &r.DeploymentMode,
			&r.PairedAt, &r.LastCheckinAt, &r.SoftwareVersion, &r.Status, &r.Region,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
