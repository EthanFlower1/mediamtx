package brivo

import (
	"context"
	"errors"
	"time"
)

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	// ErrNotConnected is returned when an operation requires a Brivo
	// connection but the tenant has not completed OAuth yet.
	ErrNotConnected = errors.New("brivo: tenant not connected")

	// ErrTokenExpired is returned when the stored token pair has expired
	// and automatic refresh also failed.
	ErrTokenExpired = errors.New("brivo: token expired and refresh failed")

	// ErrInvalidState is returned when the OAuth callback state parameter
	// does not match the stored PKCE session.
	ErrInvalidState = errors.New("brivo: invalid oauth state")

	// ErrDoorNotFound is returned when a door ID referenced in an event
	// does not exist in the tenant's Brivo site.
	ErrDoorNotFound = errors.New("brivo: door not found")

	// ErrCameraNotMapped is returned when no camera is mapped to the
	// door that triggered an event.
	ErrCameraNotMapped = errors.New("brivo: no camera mapped to door")

	// ErrWebhookSignatureInvalid is returned when the HMAC signature on
	// an inbound Brivo webhook does not verify.
	ErrWebhookSignatureInvalid = errors.New("brivo: webhook signature invalid")
)

// ---------------------------------------------------------------------------
// OAuth / Token types
// ---------------------------------------------------------------------------

// OAuthConfig holds the Brivo OAuth application credentials. These are
// provisioned once per KaiVue deployment, not per tenant.
type OAuthConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	// AuthURL is the Brivo authorization endpoint.
	// Default: https://auth.brivo.com/oauth/authorize
	AuthURL string `json:"auth_url"`
	// TokenURL is the Brivo token endpoint.
	// Default: https://auth.brivo.com/oauth/token
	TokenURL string `json:"token_url"`
	// RedirectURL is the KaiVue callback URL registered with Brivo.
	RedirectURL string `json:"redirect_url"`
	// WebhookSecret is the HMAC key Brivo uses to sign webhook payloads.
	WebhookSecret string `json:"webhook_secret"`
}

// TokenPair holds an OAuth access + refresh token pair along with expiry
// metadata. Stored encrypted in the credential vault.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope"`
}

// IsExpired reports whether the access token has expired (with a 60-second
// safety margin so we refresh before the exact deadline).
func (t TokenPair) IsExpired() bool {
	return time.Now().After(t.ExpiresAt.Add(-60 * time.Second))
}

// PKCESession stores the in-flight PKCE parameters for an OAuth flow that
// has not yet completed. Keyed by (tenant_id, state).
type PKCESession struct {
	TenantID     string    `json:"tenant_id"`
	State        string    `json:"state"`
	CodeVerifier string    `json:"code_verifier"`
	CreatedAt    time.Time `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Connection / mapping types
// ---------------------------------------------------------------------------

// Connection represents a tenant's active Brivo integration link.
type Connection struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	BrivoSiteID string     `json:"brivo_site_id"`
	SiteName    string     `json:"site_name"`
	Status      ConnStatus `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	// LastSyncAt is the timestamp of the last successful event sync.
	LastSyncAt *time.Time `json:"last_sync_at,omitempty"`
}

// ConnStatus enumerates connection health states.
type ConnStatus string

const (
	ConnStatusActive       ConnStatus = "active"
	ConnStatusDisconnected ConnStatus = "disconnected"
	ConnStatusError        ConnStatus = "error"
	ConnStatusPending      ConnStatus = "pending"
)

// DoorCameraMapping links a Brivo door to a KaiVue camera for event
// correlation. A single door may map to multiple cameras (e.g. interior
// and exterior views).
type DoorCameraMapping struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id"`
	BrivoDoorID string `json:"brivo_door_id"`
	DoorName    string `json:"door_name"`
	CameraID    string `json:"camera_id"`
	CameraName  string `json:"camera_name"`
}

// ---------------------------------------------------------------------------
// Event types
// ---------------------------------------------------------------------------

// DoorEvent represents an inbound event from Brivo's webhook.
type DoorEvent struct {
	EventID     string        `json:"event_id"`
	TenantID    string        `json:"tenant_id"`
	BrivoSiteID string        `json:"brivo_site_id"`
	BrivoDoorID string        `json:"brivo_door_id"`
	DoorName    string        `json:"door_name"`
	EventType   DoorEventType `json:"event_type"`
	UserName    string        `json:"user_name,omitempty"`
	Credential  string        `json:"credential,omitempty"`
	OccurredAt  time.Time     `json:"occurred_at"`
	ReceivedAt  time.Time     `json:"received_at"`
	// SnapshotURLs are populated after camera correlation completes.
	SnapshotURLs []string `json:"snapshot_urls,omitempty"`
}

// DoorEventType enumerates Brivo access-control event kinds.
type DoorEventType string

const (
	DoorEventUnlock      DoorEventType = "door_unlock"
	DoorEventLock        DoorEventType = "door_lock"
	DoorEventForcedEntry DoorEventType = "forced_entry"
	DoorEventHeldOpen    DoorEventType = "held_open"
	DoorEventAccessDeny  DoorEventType = "access_denied"
)

// NVREvent is an outbound event from KaiVue to Brivo (bidirectional flow).
// For example, a motion alert near a door can trigger a Brivo lockdown.
type NVREvent struct {
	EventType  string `json:"event_type"`
	TenantID   string `json:"tenant_id"`
	CameraID   string `json:"camera_id"`
	CameraName string `json:"camera_name"`
	DoorID     string `json:"door_id"`
	Action     string `json:"action"` // "lock", "unlock", "alert"
	Reason     string `json:"reason"`
}

// ---------------------------------------------------------------------------
// Brivo API response types (subset needed for config UI)
// ---------------------------------------------------------------------------

// BrivoSite is a site returned by the Brivo API.
type BrivoSite struct {
	ID   string `json:"id"`
	Name string `json:"siteName"`
}

// BrivoDoor is a door (access point) returned by the Brivo API.
type BrivoDoor struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	SiteID   string `json:"siteId"`
	SiteName string `json:"siteName"`
}

// ---------------------------------------------------------------------------
// Interfaces
// ---------------------------------------------------------------------------

// TokenStore persists OAuth token pairs per tenant. The production
// implementation wraps the credential vault with Brivo-specific key paths.
type TokenStore interface {
	// StoreToken persists or replaces the token pair for a tenant.
	StoreToken(ctx context.Context, tenantID string, token TokenPair) error
	// GetToken retrieves the current token pair. Returns ErrNotConnected
	// if no token is stored.
	GetToken(ctx context.Context, tenantID string) (TokenPair, error)
	// DeleteToken removes the stored token (disconnect flow).
	DeleteToken(ctx context.Context, tenantID string) error
}

// ConnectionStore persists Brivo connection metadata.
type ConnectionStore interface {
	Upsert(ctx context.Context, conn Connection) error
	Get(ctx context.Context, tenantID string) (*Connection, error)
	Delete(ctx context.Context, tenantID string) error
	UpdateSyncTime(ctx context.Context, tenantID string, t time.Time) error
}

// MappingStore persists door-to-camera mappings.
type MappingStore interface {
	Set(ctx context.Context, m DoorCameraMapping) error
	ListByTenant(ctx context.Context, tenantID string) ([]DoorCameraMapping, error)
	ListByDoor(ctx context.Context, tenantID, brivoDooorID string) ([]DoorCameraMapping, error)
	Delete(ctx context.Context, tenantID, mappingID string) error
}

// EventLog records processed door events for audit and replay.
type EventLog interface {
	Append(ctx context.Context, event DoorEvent) error
	ListByTenant(ctx context.Context, tenantID string, from, to time.Time, limit int) ([]DoorEvent, error)
}

// SnapshotService captures a still frame from a camera at a given timestamp.
type SnapshotService interface {
	Capture(ctx context.Context, tenantID, cameraID string, ts time.Time) (url string, err error)
}

// BrivoAPIClient abstracts HTTP calls to the Brivo REST API so the service
// can be tested without network access.
type BrivoAPIClient interface {
	// ExchangeCode trades an authorization code + PKCE verifier for tokens.
	ExchangeCode(ctx context.Context, code, codeVerifier string) (TokenPair, error)
	// RefreshToken uses a refresh token to obtain a new access token.
	RefreshToken(ctx context.Context, refreshToken string) (TokenPair, error)
	// ListSites returns sites visible to the authenticated user.
	ListSites(ctx context.Context, accessToken string) ([]BrivoSite, error)
	// ListDoors returns doors (access points) for a site.
	ListDoors(ctx context.Context, accessToken string, siteID string) ([]BrivoDoor, error)
	// SendEvent pushes an NVR event to the Brivo API (bidirectional flow).
	SendEvent(ctx context.Context, accessToken string, event NVREvent) error
}

// AuditHook is called after every significant integration action for the
// cloud audit log.
type AuditHook func(ctx context.Context, event AuditEvent)

// AuditEvent describes a Brivo integration action.
type AuditEvent struct {
	Action   string `json:"action"` // "connect", "disconnect", "event_received", "event_sent", "token_refresh", "test"
	TenantID string `json:"tenant_id"`
	Detail   string `json:"detail,omitempty"`
}
