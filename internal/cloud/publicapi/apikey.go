package publicapi

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// APIKeyHeader is the canonical header for API key authentication.
const APIKeyHeader = "X-API-Key"

// APIKeyPrefix is the conventional prefix for KaiVue API keys.
// Format: "kvue_" + 40 hex chars.
const APIKeyPrefix = "kvue_"

// ErrInvalidAPIKey is returned when an API key fails validation.
var ErrInvalidAPIKey = errors.New("invalid API key")

// ErrAPIKeyExpired is returned when an API key has passed its expiry.
var ErrAPIKeyExpired = errors.New("API key expired")

// ErrAPIKeyRevoked is returned when an API key has been revoked.
var ErrAPIKeyRevoked = errors.New("API key revoked")

// ErrAPIKeyNotFound is returned when the requested key ID does not exist.
var ErrAPIKeyNotFound = errors.New("API key not found")

// ErrAPIKeyAlreadyRevoked is returned when attempting to revoke an already-revoked key.
var ErrAPIKeyAlreadyRevoked = errors.New("API key already revoked")

// DefaultGracePeriod is the default window during which a rotated-from key
// remains valid after rotation.
const DefaultGracePeriod = 24 * time.Hour

// CreateAPIKeyRequest is the input for creating a new API key.
type CreateAPIKeyRequest struct {
	TenantID  string
	Name      string
	Scopes    []string
	ExpiresAt time.Time // zero = no expiry
	CreatedBy string
	Tier      TenantTier
}

// CreateAPIKeyResult is returned from Create. RawKey is the plaintext key —
// it is shown exactly once and never stored or retrievable again.
type CreateAPIKeyResult struct {
	RawKey string
	Key    *APIKey
}

// RotateAPIKeyRequest is the input for rotating an existing API key.
type RotateAPIKeyRequest struct {
	KeyID       string
	RotatedBy   string
	GracePeriod time.Duration // zero = DefaultGracePeriod
}

// RotateAPIKeyResult contains the new key and the grace expiry of the old key.
type RotateAPIKeyResult struct {
	RawKey          string
	NewKey          *APIKey
	OldKeyGraceEnd  time.Time
}

// ListAPIKeysFilter constrains a List call.
type ListAPIKeysFilter struct {
	TenantID   string
	IncludeRevoked bool
	Limit      int
	Cursor     string // key ID for keyset pagination
}

// APIKey represents a validated API key record.
type APIKey struct {
	// ID is the server-assigned unique identifier.
	ID string
	// KeyHash is the SHA-256 hash of the key (never the plaintext).
	KeyHash string
	// TenantID is the tenant this key belongs to.
	TenantID string
	// Tier is the rate-limit tier for this key's tenant.
	Tier TenantTier
	// Name is a human-readable label.
	Name string
	// Scopes limits which resources/actions this key can access.
	// Empty means full tenant access.
	Scopes []string
	// CreatedAt is when the key was created.
	CreatedAt time.Time
	// ExpiresAt is the key's expiry. Zero means no expiry.
	ExpiresAt time.Time
	// RevokedAt is set when the key is revoked. Zero means active.
	RevokedAt time.Time
	// LastUsedAt tracks the most recent API call.
	LastUsedAt time.Time
	// CreatedBy is the user who created the key.
	CreatedBy string
	// KeyPrefix is the first 8 characters of the key for display (e.g. "kvue_a1b").
	KeyPrefix string
	// RotatedFromID links to the predecessor key when created via rotation.
	RotatedFromID string
	// GraceExpiresAt is set on the OLD key after rotation; the old key remains
	// valid until this time to allow consumers to switch over.
	GraceExpiresAt time.Time
}

// IsExpired reports whether the key has passed its expiry time.
func (k *APIKey) IsExpired() bool {
	if k.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(k.ExpiresAt)
}

// IsRevoked reports whether the key has been revoked.
func (k *APIKey) IsRevoked() bool {
	return !k.RevokedAt.IsZero()
}

// IsActive reports whether the key can be used for authentication.
func (k *APIKey) IsActive() bool {
	return !k.IsExpired() && !k.IsRevoked()
}

// HasScope reports whether the key grants access to the given scope.
// An empty scope list means full access.
func (k *APIKey) HasScope(scope string) bool {
	if len(k.Scopes) == 0 {
		return true
	}
	for _, s := range k.Scopes {
		if s == scope {
			return true
		}
		// Wildcard: "cameras:*" matches "cameras:read", "cameras:write"
		if strings.HasSuffix(s, ":*") {
			prefix := strings.TrimSuffix(s, "*")
			if strings.HasPrefix(scope, prefix) {
				return true
			}
		}
	}
	return false
}

// APIKeyStore is the interface for API key persistence and lifecycle.
type APIKeyStore interface {
	// Create generates a new API key, stores its SHA-256 hash, and returns
	// the plaintext key exactly once. The plaintext is never stored.
	Create(ctx context.Context, req CreateAPIKeyRequest) (*CreateAPIKeyResult, error)

	// Get returns the key metadata by ID. Returns ErrAPIKeyNotFound if missing.
	Get(ctx context.Context, keyID string) (*APIKey, error)

	// List returns keys for a tenant, ordered by created_at desc.
	List(ctx context.Context, filter ListAPIKeysFilter) ([]*APIKey, error)

	// Rotate creates a new key to replace an existing one. The old key enters
	// a grace period (default 24h) during which both keys are valid. After the
	// grace period the old key is treated as expired.
	Rotate(ctx context.Context, req RotateAPIKeyRequest) (*RotateAPIKeyResult, error)

	// Revoke immediately marks a key as revoked. Returns ErrAPIKeyAlreadyRevoked
	// if already revoked, ErrAPIKeyNotFound if missing.
	Revoke(ctx context.Context, keyID string, revokedBy string) error

	// Validate looks up an API key by its raw value, verifies the hash,
	// and returns the key record. Returns ErrInvalidAPIKey if not found,
	// ErrAPIKeyExpired if expired, ErrAPIKeyRevoked if revoked.
	Validate(ctx context.Context, rawKey string) (*APIKey, error)

	// TouchLastUsed updates the last_used_at timestamp for the key.
	// Best-effort; errors are non-fatal.
	TouchLastUsed(ctx context.Context, keyID string) error

	// ListExpiring returns active keys whose expiry falls within the given
	// window. Used by rotation-reminder jobs.
	ListExpiring(ctx context.Context, tenantID string, within time.Duration) ([]*APIKey, error)
}

// APIKeyToClaims converts a validated API key into auth.Claims so the
// existing permission middleware can enforce access. The claims carry
// a synthetic user ID ("apikey:<key-id>") and the key's tenant.
func APIKeyToClaims(key *APIKey) *auth.Claims {
	groups := make([]auth.GroupID, len(key.Scopes))
	for i, s := range key.Scopes {
		groups[i] = auth.GroupID(s)
	}
	return &auth.Claims{
		UserID: auth.UserID("apikey:" + key.ID),
		TenantRef: auth.TenantRef{
			Type: auth.TenantTypeCustomer,
			ID:   key.TenantID,
		},
		Groups: groups,
	}
}
