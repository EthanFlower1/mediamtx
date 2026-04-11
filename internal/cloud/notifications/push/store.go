package push

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

// IDGen generates a random hex ID.
type IDGen func() string

func defaultIDGen() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// StoreConfig bundles dependencies for Store.
type StoreConfig struct {
	DB    *clouddb.DB
	IDGen IDGen
}

// Store manages device token registration and push credential storage.
type Store struct {
	db    *clouddb.DB
	idGen IDGen
}

// NewStore constructs a Store.
func NewStore(cfg StoreConfig) (*Store, error) {
	if cfg.DB == nil {
		return nil, errors.New("push: DB is required")
	}
	idGen := cfg.IDGen
	if idGen == nil {
		idGen = defaultIDGen
	}
	return &Store{db: cfg.DB, idGen: idGen}, nil
}

// ---------- Device Token CRUD ----------

// RegisterDevice upserts a device token for a user. If the same
// (tenant, user, platform, token) tuple already exists, the record is
// updated (device name, timestamp).
func (s *Store) RegisterDevice(ctx context.Context, dt DeviceToken) (DeviceToken, error) {
	if dt.TenantID == "" {
		return DeviceToken{}, ErrMissingTenantID
	}
	if dt.UserID == "" {
		return DeviceToken{}, ErrMissingUserID
	}
	if dt.DeviceToken == "" {
		return DeviceToken{}, ErrMissingToken
	}
	if !ValidPlatform(dt.Platform) {
		return DeviceToken{}, ErrInvalidPlatform
	}

	now := time.Now().UTC()
	if dt.TokenID == "" {
		dt.TokenID = s.idGen()
	}
	dt.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO device_tokens (token_id, tenant_id, user_id, platform, device_token, device_name, app_bundle_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, user_id, platform, device_token) DO UPDATE SET
			device_name = excluded.device_name,
			app_bundle_id = excluded.app_bundle_id,
			updated_at = excluded.updated_at`,
		dt.TokenID, dt.TenantID, dt.UserID, dt.Platform, dt.DeviceToken,
		dt.DeviceName, dt.AppBundleID, now, now)
	if err != nil {
		return DeviceToken{}, fmt.Errorf("register device: %w", err)
	}
	dt.CreatedAt = now
	return dt, nil
}

// DeregisterDevice removes a specific device token.
func (s *Store) DeregisterDevice(ctx context.Context, tenantID, userID, deviceToken string) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM device_tokens WHERE tenant_id = ? AND user_id = ? AND device_token = ?`,
		tenantID, userID, deviceToken)
	if err != nil {
		return fmt.Errorf("deregister device: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrTokenNotFound
	}
	return nil
}

// DeregisterByToken removes a device token across all users (used when a push
// gateway reports the token as invalid).
func (s *Store) DeregisterByToken(ctx context.Context, tenantID, deviceToken string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM device_tokens WHERE tenant_id = ? AND device_token = ?`,
		tenantID, deviceToken)
	if err != nil {
		return fmt.Errorf("deregister by token: %w", err)
	}
	return nil
}

// ListDevices returns all device tokens for a user within a tenant.
func (s *Store) ListDevices(ctx context.Context, tenantID, userID string) ([]DeviceToken, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT token_id, tenant_id, user_id, platform, device_token, device_name, app_bundle_id, created_at, updated_at
		FROM device_tokens
		WHERE tenant_id = ? AND user_id = ?
		ORDER BY created_at`, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()

	var out []DeviceToken
	for rows.Next() {
		var d DeviceToken
		if err := rows.Scan(&d.TokenID, &d.TenantID, &d.UserID, &d.Platform,
			&d.DeviceToken, &d.DeviceName, &d.AppBundleID, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListDevicesByPlatform returns all device tokens for a user on a specific platform.
func (s *Store) ListDevicesByPlatform(ctx context.Context, tenantID, userID string, platform notifications.Platform) ([]DeviceToken, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT token_id, tenant_id, user_id, platform, device_token, device_name, app_bundle_id, created_at, updated_at
		FROM device_tokens
		WHERE tenant_id = ? AND user_id = ? AND platform = ?
		ORDER BY created_at`, tenantID, userID, platform)
	if err != nil {
		return nil, fmt.Errorf("list devices by platform: %w", err)
	}
	defer rows.Close()

	var out []DeviceToken
	for rows.Next() {
		var d DeviceToken
		if err := rows.Scan(&d.TokenID, &d.TenantID, &d.UserID, &d.Platform,
			&d.DeviceToken, &d.DeviceName, &d.AppBundleID, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ---------- Push Credential CRUD ----------

// UpsertCredential creates or updates push credentials for a tenant + platform.
func (s *Store) UpsertCredential(ctx context.Context, cred PushCredential) (PushCredential, error) {
	if cred.TenantID == "" {
		return PushCredential{}, ErrMissingTenantID
	}
	if !ValidPlatform(cred.Platform) {
		return PushCredential{}, ErrInvalidPlatform
	}

	now := time.Now().UTC()
	if cred.CredentialID == "" {
		cred.CredentialID = s.idGen()
	}
	cred.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO push_credentials (credential_id, tenant_id, platform, credentials, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, platform) DO UPDATE SET
			credentials = excluded.credentials,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at`,
		cred.CredentialID, cred.TenantID, cred.Platform, cred.Credentials,
		cred.Enabled, now, now)
	if err != nil {
		return PushCredential{}, fmt.Errorf("upsert credential: %w", err)
	}
	cred.CreatedAt = now
	return cred, nil
}

// GetCredential returns the push credential for a tenant + platform.
func (s *Store) GetCredential(ctx context.Context, tenantID string, platform notifications.Platform) (PushCredential, error) {
	var cred PushCredential
	err := s.db.QueryRowContext(ctx, `
		SELECT credential_id, tenant_id, platform, credentials, enabled, created_at, updated_at
		FROM push_credentials
		WHERE tenant_id = ? AND platform = ?`, tenantID, platform).
		Scan(&cred.CredentialID, &cred.TenantID, &cred.Platform,
			&cred.Credentials, &cred.Enabled, &cred.CreatedAt, &cred.UpdatedAt)
	if err != nil {
		return PushCredential{}, ErrCredentialNotFound
	}
	return cred, nil
}

// DeleteCredential removes push credentials for a tenant + platform.
func (s *Store) DeleteCredential(ctx context.Context, tenantID string, platform notifications.Platform) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM push_credentials WHERE tenant_id = ? AND platform = ?`,
		tenantID, platform)
	if err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCredentialNotFound
	}
	return nil
}

// ListCredentials returns all push credentials for a tenant.
func (s *Store) ListCredentials(ctx context.Context, tenantID string) ([]PushCredential, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT credential_id, tenant_id, platform, credentials, enabled, created_at, updated_at
		FROM push_credentials
		WHERE tenant_id = ?
		ORDER BY platform`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	defer rows.Close()

	var out []PushCredential
	for rows.Next() {
		var c PushCredential
		if err := rows.Scan(&c.CredentialID, &c.TenantID, &c.Platform,
			&c.Credentials, &c.Enabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan credential: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
