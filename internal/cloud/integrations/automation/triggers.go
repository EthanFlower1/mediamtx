// Package automation provides shared trigger/action definitions and platform
// adapters for Zapier, Make (Integromat), and n8n workflow-automation services.
package automation

import "time"

// TriggerType identifies the kind of event that fires a trigger.
type TriggerType string

const (
	TriggerCameraEvent TriggerType = "camera_event"
	TriggerAlert       TriggerType = "alert"
)

// ActionType identifies an operation the platform can invoke.
type ActionType string

const (
	ActionCreateClip       ActionType = "create_clip"
	ActionSendNotification ActionType = "send_notification"
)

// TriggerPayload is the canonical envelope sent to every subscriber when a
// trigger fires, regardless of platform.
type TriggerPayload struct {
	TriggerType TriggerType `json:"trigger_type"`
	CameraID    string      `json:"camera_id,omitempty"`
	CameraName  string      `json:"camera_name,omitempty"`
	EventType   string      `json:"event_type,omitempty"`
	AlertLevel  string      `json:"alert_level,omitempty"`
	Message     string      `json:"message,omitempty"`
	Timestamp   time.Time   `json:"timestamp"`
	Metadata    Metadata    `json:"metadata,omitempty"`
}

// ActionRequest is the canonical inbound payload for an action invocation.
type ActionRequest struct {
	ActionType ActionType `json:"action_type"`
	CameraID   string     `json:"camera_id,omitempty"`
	StartTime  time.Time  `json:"start_time,omitempty"`
	EndTime    time.Time  `json:"end_time,omitempty"`
	Title      string     `json:"title,omitempty"`
	Message    string     `json:"message,omitempty"`
	Recipients []string   `json:"recipients,omitempty"`
	Metadata   Metadata   `json:"metadata,omitempty"`
}

// ActionResponse is returned to the platform after an action completes.
type ActionResponse struct {
	Success bool   `json:"success"`
	ID      string `json:"id,omitempty"`
	Message string `json:"message,omitempty"`
}

// Metadata carries arbitrary key-value pairs for extensibility.
type Metadata map[string]any

// TriggerDefinition describes a trigger that automation platforms can
// subscribe to.
type TriggerDefinition struct {
	Key         string      `json:"key"`
	Label       string      `json:"label"`
	Description string      `json:"description"`
	Type        TriggerType `json:"type"`
	SampleData  any         `json:"sample_data,omitempty"`
}

// ActionDefinition describes an action that automation platforms can invoke.
type ActionDefinition struct {
	Key         string     `json:"key"`
	Label       string     `json:"label"`
	Description string     `json:"description"`
	Type        ActionType `json:"type"`
	InputFields []Field    `json:"input_fields"`
}

// Field describes a single input or output field for an action.
type Field struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Type     string `json:"type"` // string, datetime, array
	Required bool   `json:"required"`
	HelpText string `json:"help_text,omitempty"`
}

// SharedTriggers returns the canonical set of trigger definitions used by all
// three automation platforms.
func SharedTriggers() []TriggerDefinition {
	return []TriggerDefinition{
		{
			Key:         "camera_event",
			Label:       "Camera Event",
			Description: "Triggers when a camera event occurs (motion, disconnect, etc.)",
			Type:        TriggerCameraEvent,
			SampleData: TriggerPayload{
				TriggerType: TriggerCameraEvent,
				CameraID:    "cam-001",
				CameraName:  "Front Door",
				EventType:   "motion_detected",
				Timestamp:   time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
			},
		},
		{
			Key:         "alert",
			Label:       "Alert",
			Description: "Triggers when a system alert is raised.",
			Type:        TriggerAlert,
			SampleData: TriggerPayload{
				TriggerType: TriggerAlert,
				AlertLevel:  "warning",
				Message:     "Camera offline: Back Yard",
				Timestamp:   time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
			},
		},
	}
}

// SharedActions returns the canonical set of action definitions.
func SharedActions() []ActionDefinition {
	return []ActionDefinition{
		{
			Key:         "create_clip",
			Label:       "Create Clip",
			Description: "Creates a video clip from a camera for the given time range.",
			Type:        ActionCreateClip,
			InputFields: []Field{
				{Key: "camera_id", Label: "Camera ID", Type: "string", Required: true},
				{Key: "start_time", Label: "Start Time", Type: "datetime", Required: true},
				{Key: "end_time", Label: "End Time", Type: "datetime", Required: true},
				{Key: "title", Label: "Clip Title", Type: "string", Required: false, HelpText: "Optional label for the clip."},
			},
		},
		{
			Key:         "send_notification",
			Label:       "Send Notification",
			Description: "Sends a push/email notification to the specified recipients.",
			Type:        ActionSendNotification,
			InputFields: []Field{
				{Key: "message", Label: "Message", Type: "string", Required: true},
				{Key: "recipients", Label: "Recipients", Type: "array", Required: true, HelpText: "List of user IDs or email addresses."},
			},
		},
	}
}
