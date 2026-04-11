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

// APIKeyStore is the interface for API key persistence. KAI-400 will
// provide the concrete implementation; this is the contract.
type APIKeyStore interface {
	// Validate looks up an API key by its raw value, verifies the hash,
	// and returns the key record. Returns ErrInvalidAPIKey if not found.
	Validate(ctx context.Context, rawKey string) (*APIKey, error)

	// TouchLastUsed updates the last_used_at timestamp for the key.
	// Best-effort; errors are non-fatal.
	TouchLastUsed(ctx context.Context, keyID string) error
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
