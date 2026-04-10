package publicapi

import (
	"testing"
	"time"
)

func TestAPIKeyIsActive(t *testing.T) {
	tests := []struct {
		name   string
		key    APIKey
		active bool
	}{
		{
			name:   "fresh key",
			key:    APIKey{ID: "1", TenantID: "t1"},
			active: true,
		},
		{
			name:   "expired key",
			key:    APIKey{ID: "2", ExpiresAt: time.Now().Add(-1 * time.Hour)},
			active: false,
		},
		{
			name:   "revoked key",
			key:    APIKey{ID: "3", RevokedAt: time.Now().Add(-1 * time.Hour)},
			active: false,
		},
		{
			name:   "future expiry",
			key:    APIKey{ID: "4", ExpiresAt: time.Now().Add(24 * time.Hour)},
			active: true,
		},
		{
			name:   "no expiry (zero time)",
			key:    APIKey{ID: "5"},
			active: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.key.IsActive(); got != tt.active {
				t.Errorf("IsActive() = %v; want %v", got, tt.active)
			}
		})
	}
}

func TestAPIKeyHasScope(t *testing.T) {
	tests := []struct {
		name   string
		scopes []string
		check  string
		has    bool
	}{
		{
			name:   "empty scopes = full access",
			scopes: nil,
			check:  "cameras:read",
			has:    true,
		},
		{
			name:   "exact match",
			scopes: []string{"cameras:read", "events:read"},
			check:  "cameras:read",
			has:    true,
		},
		{
			name:   "no match",
			scopes: []string{"cameras:read"},
			check:  "cameras:create",
			has:    false,
		},
		{
			name:   "wildcard match",
			scopes: []string{"cameras:*"},
			check:  "cameras:create",
			has:    true,
		},
		{
			name:   "wildcard no cross-resource",
			scopes: []string{"cameras:*"},
			check:  "events:read",
			has:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := APIKey{Scopes: tt.scopes}
			if got := key.HasScope(tt.check); got != tt.has {
				t.Errorf("HasScope(%q) = %v; want %v", tt.check, got, tt.has)
			}
		})
	}
}

func TestAPIKeyToClaims(t *testing.T) {
	key := &APIKey{
		ID:       "key-123",
		TenantID: "acme",
		Scopes:   []string{"cameras:read"},
	}

	claims := APIKeyToClaims(key)
	if string(claims.UserID) != "apikey:key-123" {
		t.Errorf("UserID = %q; want apikey:key-123", claims.UserID)
	}
	if claims.TenantRef.ID != "acme" {
		t.Errorf("TenantRef.ID = %q; want acme", claims.TenantRef.ID)
	}
	if len(claims.Groups) != 1 || string(claims.Groups[0]) != "cameras:read" {
		t.Errorf("Groups = %v; want [cameras:read]", claims.Groups)
	}
}
