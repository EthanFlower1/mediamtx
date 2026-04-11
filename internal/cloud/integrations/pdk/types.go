package pdk

import "time"

// EventType classifies PDK door events.
type EventType string

const (
	EventDoorUnlock    EventType = "door.unlock"
	EventDoorLock      EventType = "door.lock"
	EventAccessGranted EventType = "access.granted"
	EventAccessDenied  EventType = "access.denied"
	EventDoorForcedOpen EventType = "door.forced_open"
	EventDoorHeldOpen  EventType = "door.held_open"
)

// ConnectionStatus represents the state of a PDK integration.
type ConnectionStatus string

const (
	StatusConnected    ConnectionStatus = "connected"
	StatusDisconnected ConnectionStatus = "disconnected"
	StatusError        ConnectionStatus = "error"
)

// IntegrationConfig stores per-tenant PDK API credentials and settings.
type IntegrationConfig struct {
	ConfigID     string           `json:"config_id"`
	TenantID     string           `json:"tenant_id"`
	APIEndpoint  string           `json:"api_endpoint"`
	ClientID     string           `json:"client_id"`
	ClientSecret string           `json:"client_secret,omitempty"`
	PanelID      string           `json:"panel_id"`
	WebhookSecret string          `json:"webhook_secret,omitempty"`
	Enabled      bool             `json:"enabled"`
	Status       ConnectionStatus `json:"status"`
	LastSyncAt   *time.Time       `json:"last_sync_at,omitempty"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

// Door represents a PDK-managed door / access point.
type Door struct {
	DoorID      string    `json:"door_id"`
	TenantID    string    `json:"tenant_id"`
	PDKDoorID   string    `json:"pdk_door_id"`
	Name        string    `json:"name"`
	Location    string    `json:"location"`
	IsLocked    bool      `json:"is_locked"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// DoorEvent represents a single access control event from PDK.
type DoorEvent struct {
	EventID     string    `json:"event_id"`
	TenantID    string    `json:"tenant_id"`
	DoorID      string    `json:"door_id"`
	PDKEventID  string    `json:"pdk_event_id"`
	EventType   EventType `json:"event_type"`
	PersonName  string    `json:"person_name"`
	Credential  string    `json:"credential"`
	OccurredAt  time.Time `json:"occurred_at"`
	RawPayload  string    `json:"raw_payload"`
	CreatedAt   time.Time `json:"created_at"`
}

// DoorCameraMapping links a PDK door to one or more NVR camera paths,
// enabling automatic video correlation when door events arrive.
type DoorCameraMapping struct {
	MappingID  string    `json:"mapping_id"`
	TenantID   string    `json:"tenant_id"`
	DoorID     string    `json:"door_id"`
	CameraPath string    `json:"camera_path"`
	PreBuffer  int       `json:"pre_buffer_sec"`
	PostBuffer int       `json:"post_buffer_sec"`
	CreatedAt  time.Time `json:"created_at"`
}

// VideoCorrelation ties a door event to a specific video clip.
type VideoCorrelation struct {
	CorrelationID string    `json:"correlation_id"`
	TenantID      string    `json:"tenant_id"`
	EventID       string    `json:"event_id"`
	CameraPath    string    `json:"camera_path"`
	ClipStart     time.Time `json:"clip_start"`
	ClipEnd       time.Time `json:"clip_end"`
	CreatedAt     time.Time `json:"created_at"`
}

// WebhookPayload is the inbound payload structure from PDK's webhook system.
type WebhookPayload struct {
	EventID    string    `json:"event_id"`
	PanelID    string    `json:"panel_id"`
	DoorID     string    `json:"door_id"`
	EventType  string    `json:"event_type"`
	PersonName string    `json:"person_name"`
	Credential string    `json:"credential"`
	Timestamp  time.Time `json:"timestamp"`
	Raw        string    `json:"-"`
}

// TokenResponse represents an OAuth token from the PDK API.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// PDKDoor represents a door object from the PDK API.
type PDKDoor struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Location string `json:"location"`
	IsLocked bool   `json:"is_locked"`
}

// PDKEvent represents an event from the PDK API event stream.
type PDKEvent struct {
	ID         string    `json:"id"`
	DoorID     string    `json:"door_id"`
	EventType  string    `json:"event_type"`
	PersonName string    `json:"person_name"`
	Credential string    `json:"credential"`
	Timestamp  time.Time `json:"timestamp"`
}
