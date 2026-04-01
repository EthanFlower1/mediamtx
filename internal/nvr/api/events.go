package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/ai"
)

// DetectionData holds per-detection bounding box data serialised inside a
// detection_frame WebSocket event. Coordinates are normalised [0,1].
type DetectionData struct {
	Class      string  `json:"class"`
	Confidence float32 `json:"confidence"`
	TrackID    int     `json:"track_id"`
	X          float32 `json:"x"`
	Y          float32 `json:"y"`
	W          float32 `json:"w"`
	H          float32 `json:"h"`
}

// Event represents a system event that is broadcast to SSE clients.
type Event struct {
	Type       string          `json:"type"`    // "motion", "camera_offline", "camera_online", "recording_started", "recording_stopped", "recording_stalled", "recording_recovered", "recording_failed", "detection_frame"
	Camera     string          `json:"camera"`  // camera name
	Message    string          `json:"message"`
	Time       string          `json:"time"`
	Detections []DetectionData `json:"detections,omitempty"`
}

// EventBroadcaster fans out events to all connected SSE clients.
type EventBroadcaster struct {
	mu      sync.RWMutex
	clients map[chan Event]struct{}
}

// NewEventBroadcaster creates a new EventBroadcaster.
func NewEventBroadcaster() *EventBroadcaster {
	return &EventBroadcaster{
		clients: make(map[chan Event]struct{}),
	}
}

// Subscribe registers a new client and returns its event channel.
// The channel is buffered to avoid blocking the publisher if a client
// is slow to read.
func (b *EventBroadcaster) Subscribe() chan Event {
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a client channel and closes it.
func (b *EventBroadcaster) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	close(ch)
}

// Publish sends an event to all connected clients. If a client's buffer
// is full, the event is dropped for that client to avoid blocking.
func (b *EventBroadcaster) Publish(event Event) {
	if event.Time == "" {
		event.Time = time.Now().UTC().Format(time.RFC3339)
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- event:
		default:
			log.Printf("events: dropped %s event for slow client", event.Type)
		}
	}
}

// PublishDetectionFrame broadcasts per-frame bounding box data to all connected
// WebSocket clients so that Flutter analytics overlays can render live boxes.
// It accepts []ai.DetectionFrameData (defined in the ai package) and converts
// them to the JSON-serialisable DetectionData type.
func (b *EventBroadcaster) PublishDetectionFrame(camera string, detections []ai.DetectionFrameData) {
	dets := make([]DetectionData, len(detections))
	for i, d := range detections {
		dets[i] = DetectionData{
			Class:      d.Class,
			Confidence: d.Confidence,
			TrackID:    d.TrackID,
			X:          d.X,
			Y:          d.Y,
			W:          d.W,
			H:          d.H,
		}
	}
	b.Publish(Event{
		Type:       "detection_frame",
		Camera:     camera,
		Detections: dets,
		Time:       time.Now().UTC().Format(time.RFC3339),
	})
}

// PublishMotion publishes a motion-detected event for the given camera.
func (b *EventBroadcaster) PublishMotion(cameraName string) {
	b.Publish(Event{
		Type:    "motion",
		Camera:  cameraName,
		Message: fmt.Sprintf("Motion detected on %s", cameraName),
	})
}

// PublishAIDetection publishes an AI detection event with the specific object class.
func (b *EventBroadcaster) PublishAIDetection(cameraName, className string, confidence float32) {
	b.Publish(Event{
		Type:    "ai_detection",
		Camera:  cameraName,
		Message: fmt.Sprintf("%s detected on %s (%.0f%%)", className, cameraName, confidence*100),
	})
}

// PublishTampering publishes a tampering-detected event for the given camera.
func (b *EventBroadcaster) PublishTampering(cameraName string) {
	b.Publish(Event{
		Type:    "tampering",
		Camera:  cameraName,
		Message: fmt.Sprintf("Tampering detected on %s", cameraName),
	})
}

// PublishCameraOffline publishes a camera-offline event.
func (b *EventBroadcaster) PublishCameraOffline(cameraName string) {
	b.Publish(Event{
		Type:    "camera_offline",
		Camera:  cameraName,
		Message: fmt.Sprintf("Camera %s went offline", cameraName),
	})
}

// PublishCameraOnline publishes a camera-online event.
func (b *EventBroadcaster) PublishCameraOnline(cameraName string) {
	b.Publish(Event{
		Type:    "camera_online",
		Camera:  cameraName,
		Message: fmt.Sprintf("Camera %s is online", cameraName),
	})
}

// PublishRecordingStarted publishes a recording-started event.
func (b *EventBroadcaster) PublishRecordingStarted(cameraName string) {
	b.Publish(Event{
		Type:    "recording_started",
		Camera:  cameraName,
		Message: fmt.Sprintf("Recording started on %s", cameraName),
	})
}

// PublishRecordingStopped publishes a recording-stopped event.
func (b *EventBroadcaster) PublishRecordingStopped(cameraName string) {
	b.Publish(Event{
		Type:    "recording_stopped",
		Camera:  cameraName,
		Message: fmt.Sprintf("Recording stopped on %s", cameraName),
	})
}

// PublishRecordingStalled publishes a recording-stalled event.
func (b *EventBroadcaster) PublishRecordingStalled(cameraName string) {
	b.Publish(Event{
		Type:    "recording_stalled",
		Camera:  cameraName,
		Message: fmt.Sprintf("Recording stalled on %s", cameraName),
	})
}

// PublishRecordingRecovered publishes a recording-recovered event.
func (b *EventBroadcaster) PublishRecordingRecovered(cameraName string) {
	b.Publish(Event{
		Type:    "recording_recovered",
		Camera:  cameraName,
		Message: fmt.Sprintf("Recording recovered on %s", cameraName),
	})
}

// PublishRecordingFailed publishes a recording-failed event.
func (b *EventBroadcaster) PublishRecordingFailed(cameraName string) {
	b.Publish(Event{
		Type:    "recording_failed",
		Camera:  cameraName,
		Message: fmt.Sprintf("Recording failed on %s", cameraName),
	})
}

// StreamDetections serves an SSE (Server-Sent Events) stream of detection_frame
// events filtered to a single camera. The connection stays open until the client
// disconnects. Each event is a JSON-encoded line prefixed with "data: ".
//
// GET /cameras/:id/detections/stream?camera_name=X
func (b *EventBroadcaster) StreamDetections(c *gin.Context) {
	cameraName := c.Query("camera_name")
	if cameraName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_name is required"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // disable nginx buffering

	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	ctx := c.Request.Context()
	c.Stream(func(w io.Writer) bool {
		select {
		case <-ctx.Done():
			return false
		case event, ok := <-ch:
			if !ok {
				return false
			}
			if event.Type != "detection_frame" || event.Camera != cameraName {
				return true // skip non-matching events
			}
			data, err := json.Marshal(event)
			if err != nil {
				return true
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			return true
		}
	})
}
