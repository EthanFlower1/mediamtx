package openpath

import (
	"errors"
	"time"
)

// -----------------------------------------------------------------------
// Configuration
// -----------------------------------------------------------------------

// Config holds the per-tenant OpenPath / Avigilon Alta integration settings.
type Config struct {
	// TenantID is the owning tenant. Required.
	TenantID string `json:"tenant_id"`

	// OrgID is the Alta organisation identifier.
	OrgID string `json:"org_id"`

	// ClientID is the OAuth2 client_credentials client ID.
	ClientID string `json:"client_id"`

	// ClientSecret is the OAuth2 client_credentials client secret.
	// Stored encrypted at rest via credentialvault; this field carries the
	// plaintext only in-memory during token exchange.
	ClientSecret string `json:"client_secret"`

	// BaseURL is the Alta API base URL. Defaults to DefaultBaseURL.
	BaseURL string `json:"base_url,omitempty"`

	// WebhookSecret is the HMAC-SHA256 secret used to verify inbound
	// webhook payloads from Alta.
	WebhookSecret string `json:"webhook_secret"`

	// DoorCameraMappings maps Alta door IDs to NVR camera path names.
	// Multiple cameras can observe a single door.
	DoorCameraMappings []DoorCameraMapping `json:"door_camera_mappings"`

	// Enabled controls whether event ingestion is active for this tenant.
	Enabled bool `json:"enabled"`
}

// DefaultBaseURL is the production Alta API endpoint.
const DefaultBaseURL = "https://api.openpath.com"

// Validate checks that required fields are present.
func (c Config) Validate() error {
	if c.TenantID == "" {
		return errors.New("openpath: tenant_id is required")
	}
	if c.OrgID == "" {
		return errors.New("openpath: org_id is required")
	}
	if c.ClientID == "" {
		return errors.New("openpath: client_id is required")
	}
	if c.ClientSecret == "" {
		return errors.New("openpath: client_secret is required")
	}
	if c.WebhookSecret == "" {
		return errors.New("openpath: webhook_secret is required")
	}
	return nil
}

// EffectiveBaseURL returns BaseURL if set, otherwise DefaultBaseURL.
func (c Config) EffectiveBaseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultBaseURL
}

// DoorCameraMapping links an Alta door to one or more NVR camera paths.
type DoorCameraMapping struct {
	DoorID      string   `json:"door_id"`
	DoorName    string   `json:"door_name"`
	CameraPaths []string `json:"camera_paths"`
}

// -----------------------------------------------------------------------
// Door events (inbound from Alta)
// -----------------------------------------------------------------------

// DoorEventType discriminates the kind of access event.
type DoorEventType string

const (
	DoorEventUnlock     DoorEventType = "unlock"
	DoorEventDenied     DoorEventType = "denied"
	DoorEventForcedOpen DoorEventType = "forced_open"
	DoorEventHeldOpen   DoorEventType = "held_open"
	DoorEventLockdown   DoorEventType = "lockdown"
)

// DoorEvent is the normalised representation of an Alta webhook payload.
type DoorEvent struct {
	// ID is the Alta event ID (idempotency key).
	ID string `json:"id"`

	// TenantID is injected by the webhook handler from the URL path.
	TenantID string `json:"tenant_id"`

	// OrgID is the Alta organisation that originated the event.
	OrgID string `json:"org_id"`

	// DoorID is the Alta door identifier.
	DoorID string `json:"door_id"`

	// DoorName is the human-readable door label.
	DoorName string `json:"door_name"`

	// Type is the event classification.
	Type DoorEventType `json:"type"`

	// UserName is the credentialed user, empty for anonymous/forced events.
	UserName string `json:"user_name,omitempty"`

	// Timestamp is the event timestamp from Alta, UTC.
	Timestamp time.Time `json:"timestamp"`

	// Raw is the original JSON payload for audit/debug.
	Raw []byte `json:"-"`
}

// -----------------------------------------------------------------------
// Video correlation result
// -----------------------------------------------------------------------

// CorrelatedClip ties a door event to one or more camera recording segments.
type CorrelatedClip struct {
	DoorEventID string    `json:"door_event_id"`
	CameraPath  string    `json:"camera_path"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
}

// -----------------------------------------------------------------------
// OAuth token
// -----------------------------------------------------------------------

// Token is the in-memory representation of an Alta OAuth2 access token.
type Token struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Valid reports whether the token is non-empty and not yet expired.
// A 30-second buffer is applied to avoid using tokens that are about to expire.
func (t Token) Valid() bool {
	return t.AccessToken != "" && time.Now().Before(t.ExpiresAt.Add(-30*time.Second))
}

// -----------------------------------------------------------------------
// Bidirectional: NVR → Alta actions
// -----------------------------------------------------------------------

// LockdownRequest is sent to Alta to trigger a door lockdown in response
// to an NVR-detected security event (e.g. motion in a restricted zone).
type LockdownRequest struct {
	TenantID string `json:"tenant_id"`
	OrgID    string `json:"org_id"`
	DoorID   string `json:"door_id"`
	Reason   string `json:"reason"`
}
