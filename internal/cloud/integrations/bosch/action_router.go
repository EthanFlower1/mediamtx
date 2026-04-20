package bosch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// CameraActionDispatcher is the interface that the ActionRouter uses to
// trigger camera operations. Implementations bridge to the recorder
// control plane or NVR subsystem.
type CameraActionDispatcher interface {
	// StartRecording begins recording on the specified camera for the given
	// duration. A duration of 0 means record until explicitly stopped.
	StartRecording(ctx context.Context, tenantID, cameraID string, durationSec int) error

	// RecallPTZPreset moves a camera to a named PTZ preset position.
	RecallPTZPreset(ctx context.Context, tenantID, cameraID, presetName string) error

	// TakeSnapshot captures a still image from the camera.
	TakeSnapshot(ctx context.Context, tenantID, cameraID string) error
}

// ActionRouter receives AlarmEvents, matches them against configured
// zone-to-camera mappings, and dispatches the corresponding camera actions.
type ActionRouter struct {
	dispatcher CameraActionDispatcher
	httpClient *http.Client

	mu       sync.RWMutex
	mappings map[string][]ZoneCameraMapping // key: panelID

	// Stats
	actionsDispatched int64
	actionsFailed     int64
}

// NewActionRouter creates an action router with the given camera dispatcher.
func NewActionRouter(dispatcher CameraActionDispatcher) *ActionRouter {
	return &ActionRouter{
		dispatcher: dispatcher,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		mappings:   make(map[string][]ZoneCameraMapping),
	}
}

// SetMappings replaces the zone-to-camera mappings for a panel.
func (r *ActionRouter) SetMappings(panelID string, mappings []ZoneCameraMapping) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mappings[panelID] = mappings
}

// RemoveMappings removes all mappings for a panel.
func (r *ActionRouter) RemoveMappings(panelID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.mappings, panelID)
}

// HandleEvent is the EventHandler callback registered with the EventIngester.
// It finds matching zone mappings and executes the configured actions.
func (r *ActionRouter) HandleEvent(event *AlarmEvent) {
	r.mu.RLock()
	panelMappings, ok := r.mappings[event.PanelID]
	r.mu.RUnlock()

	if !ok {
		return
	}

	for _, mapping := range panelMappings {
		if !mapping.Enabled {
			continue
		}
		if mapping.ZoneNumber != event.ZoneNumber {
			continue
		}

		for _, cameraID := range mapping.CameraIDs {
			for _, action := range mapping.Actions {
				go r.executeAction(event, cameraID, action)
			}
		}
	}
}

func (r *ActionRouter) executeAction(event *AlarmEvent, cameraID string, action Action) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var err error
	switch action.Type {
	case ActionRecord:
		duration := action.Duration
		if duration == 0 {
			duration = 60 // default 60 seconds
		}
		err = r.dispatcher.StartRecording(ctx, event.TenantID, cameraID, duration)

	case ActionPTZPreset:
		if action.PTZPreset == "" {
			log.Printf("[bosch] [action] PTZ preset action with empty preset for camera %s", cameraID)
			return
		}
		err = r.dispatcher.RecallPTZPreset(ctx, event.TenantID, cameraID, action.PTZPreset)

	case ActionSnapshot:
		err = r.dispatcher.TakeSnapshot(ctx, event.TenantID, cameraID)

	case ActionWebhook:
		err = r.sendWebhook(ctx, action.WebhookURL, event, cameraID)

	default:
		log.Printf("[bosch] [action] unknown action type %q for camera %s", action.Type, cameraID)
		return
	}

	r.mu.Lock()
	if err != nil {
		r.actionsFailed++
		r.mu.Unlock()
		log.Printf("[bosch] [action] %s failed for camera %s: %v", action.Type, cameraID, err)
	} else {
		r.actionsDispatched++
		r.mu.Unlock()
		log.Printf("[bosch] [action] %s executed for camera %s (zone %d, event %s)",
			action.Type, cameraID, event.ZoneNumber, event.EventType)
	}
}

// webhookPayload is the JSON body sent to webhook endpoints on alarm events.
type webhookPayload struct {
	Source     string      `json:"source"`
	PanelID    string     `json:"panel_id"`
	EventType  EventType  `json:"event_type"`
	ZoneNumber int        `json:"zone_number"`
	AreaNumber int        `json:"area_number"`
	Priority   int        `json:"priority"`
	Message    string     `json:"message"`
	CameraID   string     `json:"camera_id"`
	Timestamp  time.Time  `json:"timestamp"`
}

func (r *ActionRouter) sendWebhook(ctx context.Context, url string, event *AlarmEvent, cameraID string) error {
	payload := webhookPayload{
		Source:     "bosch_panel",
		PanelID:   event.PanelID,
		EventType: event.EventType,
		ZoneNumber: event.ZoneNumber,
		AreaNumber: event.AreaNumber,
		Priority:  event.Priority,
		Message:   event.Message,
		CameraID:  cameraID,
		Timestamp: event.Timestamp,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Raikada-BoschIntegration/1.0")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// Stats returns action dispatch statistics.
func (r *ActionRouter) Stats() (dispatched, failed int64) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.actionsDispatched, r.actionsFailed
}
