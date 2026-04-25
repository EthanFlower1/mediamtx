// Package cloudconnector defines the wire message types exchanged over the
// WSS control channel between an on-prem Directory and the cloud broker.
package cloudconnector

import (
	"encoding/json"
	"time"
)

// Message type constants.
const (
	MsgTypeRegister        = "register"         // on-prem → cloud
	MsgTypeHeartbeat       = "heartbeat"        // on-prem → cloud
	MsgTypeEvent           = "event"            // on-prem → cloud
	MsgTypeCommandResponse = "command_response" // on-prem → cloud
	MsgTypeCommand         = "command"          // cloud → on-prem
	MsgTypeRegistered      = "registered"       // cloud → on-prem (ack)
)

// Envelope is the top-level wire message. Exactly one payload field is non-nil
// based on Type.
type Envelope struct {
	Type            string                  `json:"type"`
	Register        *RegisterPayload        `json:"register,omitempty"`
	Registered      *RegisteredPayload      `json:"registered,omitempty"`
	Heartbeat       *HeartbeatPayload       `json:"heartbeat,omitempty"`
	Event           *EventPayload           `json:"event,omitempty"`
	Command         *CommandPayload         `json:"command,omitempty"`
	CommandResponse *CommandResponsePayload `json:"command_response,omitempty"`
}

// Capabilities describes what features the on-prem site supports.
type Capabilities struct {
	Streams  bool `json:"streams"`
	Playback bool `json:"playback"`
	AI       bool `json:"ai"`
}

// RegisterPayload is sent once on WSS connect to identify the site.
type RegisterPayload struct {
	SiteID       string       `json:"site_id"`
	SiteAlias    string       `json:"site_alias,omitempty"`
	Version      string       `json:"version"`
	PublicIP     string       `json:"public_ip,omitempty"`
	LANCIDRs     []string     `json:"lan_cidrs,omitempty"`
	Capabilities Capabilities `json:"capabilities"`
}

// RegisteredPayload is the cloud acknowledgment to a register message.
type RegisteredPayload struct {
	OK       bool   `json:"ok"`
	RelayURL string `json:"relay_url,omitempty"`
	Error    string `json:"error,omitempty"`
}

// HeartbeatPayload is sent periodically (every 30s) to report site health.
type HeartbeatPayload struct {
	SiteID        string    `json:"site_id"`
	Timestamp     time.Time `json:"timestamp"`
	UptimeSec     int64     `json:"uptime_sec"`
	CameraCount   int       `json:"camera_count"`
	RecorderCount int       `json:"recorder_count"`
	DiskUsedPct   float64   `json:"disk_used_pct"`
	PublicIP      string    `json:"public_ip,omitempty"`
}

// EventPayload carries alerts, AI detections, or camera status changes.
type EventPayload struct {
	Kind       string          `json:"kind"`
	CameraID   string          `json:"camera_id,omitempty"`
	RecorderID string          `json:"recorder_id,omitempty"`
	Timestamp  time.Time       `json:"timestamp"`
	Data       json.RawMessage `json:"data,omitempty"`
}

// CommandPayload carries a command from the cloud broker to an on-prem site.
type CommandPayload struct {
	ID   string          `json:"id"`
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data,omitempty"`
}

// CommandResponsePayload carries the on-prem response to a cloud command.
type CommandResponsePayload struct {
	ID      string          `json:"id"`
	Success bool            `json:"success"`
	Error   string          `json:"error,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}
