package push

import (
	"errors"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

// Sentinel errors.
var (
	ErrMissingTenantID   = errors.New("push: tenant_id required")
	ErrMissingUserID     = errors.New("push: user_id required")
	ErrMissingToken      = errors.New("push: device_token required")
	ErrMissingPlatform   = errors.New("push: platform required")
	ErrTokenNotFound     = errors.New("push: device token not found")
	ErrCredentialNotFound = errors.New("push: push credential not found")
	ErrInvalidPlatform   = errors.New("push: invalid platform")
)

// ValidPlatform returns true for recognized push platforms.
func ValidPlatform(p notifications.Platform) bool {
	switch p {
	case notifications.PlatformFCM, notifications.PlatformAPNs, notifications.PlatformWebPush:
		return true
	}
	return false
}

// DeviceToken represents a registered push device for a user.
type DeviceToken struct {
	TokenID     string                 `json:"token_id"`
	TenantID    string                 `json:"tenant_id"`
	UserID      string                 `json:"user_id"`
	Platform    notifications.Platform `json:"platform"`
	DeviceToken string                 `json:"device_token"`
	DeviceName  string                 `json:"device_name"`
	AppBundleID string                 `json:"app_bundle_id"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// PushCredential holds the per-tenant platform credentials for a push service.
type PushCredential struct {
	CredentialID string                 `json:"credential_id"`
	TenantID     string                 `json:"tenant_id"`
	Platform     notifications.Platform `json:"platform"`
	Credentials  string                 `json:"credentials"` // JSON blob: service account, APNs key, VAPID keys
	Enabled      bool                   `json:"enabled"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// FCMCredentials is the JSON structure stored in PushCredential.Credentials
// for the FCM platform.
type FCMCredentials struct {
	ProjectID          string `json:"project_id"`
	ServiceAccountJSON string `json:"service_account_json"`
}

// APNsCredentials is the JSON structure stored in PushCredential.Credentials
// for the APNs platform.
type APNsCredentials struct {
	KeyID      string `json:"key_id"`
	TeamID     string `json:"team_id"`
	BundleID   string `json:"bundle_id"`
	PrivateKey string `json:"private_key"` // PEM-encoded .p8 key
	Production bool   `json:"production"`  // false = sandbox
}

// WebPushCredentials is the JSON structure stored in PushCredential.Credentials
// for the Web Push (VAPID) platform.
type WebPushCredentials struct {
	VAPIDPublicKey  string `json:"vapid_public_key"`
	VAPIDPrivateKey string `json:"vapid_private_key"`
	Subject         string `json:"subject"` // mailto: or https: URL
}

// Metrics tracks per-platform delivery statistics.
type Metrics struct {
	Platform     notifications.Platform `json:"platform"`
	TotalSent    int64                  `json:"total_sent"`
	TotalFailed  int64                  `json:"total_failed"`
	TotalRemoved int64                  `json:"total_removed"` // tokens removed due to invalidity
}
