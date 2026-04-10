// Package credentialvault manages integrator signing credentials (Apple
// Distribution certs, Google Play service accounts, APNs keys, etc.) used by
// the per-integrator mobile build pipeline (KAI-354). Every credential is
// scoped to a tenant_id for multi-tenant isolation.
package credentialvault

import (
	"context"
	"errors"
	"time"
)

// CredentialType identifies the kind of signing credential stored.
type CredentialType string

const (
	AppleDistributionCert    CredentialType = "apple_distribution_cert"
	AppleProvisioningProfile CredentialType = "apple_provisioning_profile"
	GooglePlayServiceAccount CredentialType = "google_play_service_account"
	APNsKey                  CredentialType = "apns_key"
	FCMServiceAccount        CredentialType = "fcm_service_account"
)

// ValidCredentialTypes enumerates all recognised types for input validation.
var ValidCredentialTypes = []CredentialType{
	AppleDistributionCert,
	AppleProvisioningProfile,
	GooglePlayServiceAccount,
	APNsKey,
	FCMServiceAccount,
}

// IsValid reports whether t is a known credential type.
func (t CredentialType) IsValid() bool {
	for _, v := range ValidCredentialTypes {
		if t == v {
			return true
		}
	}
	return false
}

// Credential is the stored metadata for a signing credential. The actual
// secret material is held in the backend (AWS Secrets Manager); Credential
// only carries the metadata envelope.
type Credential struct {
	TenantID    string
	Type        CredentialType
	Label       string    // human-readable label, e.g. "ACME iOS Distribution 2026"
	SecretID    string    // backend-specific identifier (Secrets Manager ARN)
	Version     string    // backend version/stage id
	CreatedAt   time.Time
	RotatedAt   time.Time
	ExpiresAt   *time.Time // nil if no expiry (e.g. service account keys)
}

// StoreRequest is the input for storing a new credential.
type StoreRequest struct {
	TenantID string
	Type     CredentialType
	Label    string
	Value    []byte     // raw secret material (cert PEM, JSON key, etc.)
	ExpiresAt *time.Time
}

// Validate checks that required fields are present.
func (r StoreRequest) Validate() error {
	if r.TenantID == "" {
		return errors.New("credentialvault: tenant_id is required")
	}
	if !r.Type.IsValid() {
		return errors.New("credentialvault: invalid credential type")
	}
	if len(r.Value) == 0 {
		return errors.New("credentialvault: value is required")
	}
	return nil
}

// RotateRequest is the input for rotating (replacing) an existing credential.
type RotateRequest struct {
	TenantID string
	Type     CredentialType
	NewValue []byte
	ExpiresAt *time.Time
}

// Validate checks that required fields are present.
func (r RotateRequest) Validate() error {
	if r.TenantID == "" {
		return errors.New("credentialvault: tenant_id is required")
	}
	if !r.Type.IsValid() {
		return errors.New("credentialvault: invalid credential type")
	}
	if len(r.NewValue) == 0 {
		return errors.New("credentialvault: new_value is required")
	}
	return nil
}

// VaultBackend abstracts the secret storage backend. The production
// implementation uses AWS Secrets Manager; tests use an in-memory fake.
type VaultBackend interface {
	// Store creates a new secret and returns its backend ID + version.
	Store(ctx context.Context, path string, value []byte) (secretID, version string, err error)

	// Get retrieves the current secret value by path.
	Get(ctx context.Context, path string) (value []byte, version string, err error)

	// Rotate replaces the secret value and returns the new version.
	Rotate(ctx context.Context, path string, newValue []byte) (version string, err error)

	// Delete removes the secret.
	Delete(ctx context.Context, path string) error

	// List returns all secret paths matching the prefix.
	List(ctx context.Context, prefix string) ([]string, error)
}

// AuditHook is called after every mutating operation for audit logging.
type AuditHook func(ctx context.Context, event AuditEvent)

// AuditEvent describes a credential vault mutation.
type AuditEvent struct {
	Action   string // "store", "rotate", "delete"
	TenantID string
	Type     CredentialType
	SecretID string
}
