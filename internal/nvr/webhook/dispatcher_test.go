package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

func setupTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })
	return database
}

func TestDispatchDetectionSuccess(t *testing.T) {
	database := setupTestDB(t)

	var received DetectionEvent
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NotEmpty(t, r.Header.Get("X-Webhook-ID"))
		err := json.NewDecoder(r.Body).Decode(&received)
		require.NoError(t, err)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create webhook config.
	wh := &db.WebhookConfig{
		ID:             "wh1",
		Name:           "Test Hook",
		URL:            server.URL,
		EventTypes:     "detection",
		ObjectClasses:  "",
		Enabled:        true,
		MaxRetries:     3,
		TimeoutSeconds: 10,
	}
	require.NoError(t, database.InsertWebhookConfig(wh))

	// Create and start dispatcher.
	d := New(database)
	d.Start()
	defer d.Stop()

	// Insert a detection.
	det := &db.Detection{
		Class:      "person",
		Confidence: 0.95,
		BoxX:       0.1,
		BoxY:       0.2,
		BoxW:       0.3,
		BoxH:       0.4,
	}

	d.OnDetection("cam1", "Front Door", det)

	// Wait for the webhook handler to finish writing `received`. Using a
	// done channel rather than time.Sleep establishes a happens-before
	// edge so the test goroutine's reads below are race-free.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for webhook delivery")
	}
	// Give the dispatcher a brief moment to record the delivery row after
	// the HTTP handler returns 200. This sleep only gates the DB read
	// below; `received` is already fully visible via the done channel.
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, "detection", received.EventType)
	assert.Equal(t, "cam1", received.CameraID)
	assert.Equal(t, "Front Door", received.CameraName)
	require.NotNil(t, received.Detection)
	assert.Equal(t, "person", received.Detection.Class)
	assert.InDelta(t, 0.95, received.Detection.Confidence, 0.01)

	// Check delivery record.
	deliveries, err := database.ListWebhookDeliveries("wh1", 10)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	assert.Equal(t, "success", deliveries[0].Status)
}

func TestDispatchClassFilter(t *testing.T) {
	database := setupTestDB(t)

	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Webhook only wants "person" detections.
	wh := &db.WebhookConfig{
		ID:            "wh1",
		Name:          "Person Only",
		URL:           server.URL,
		EventTypes:    "detection",
		ObjectClasses: "person",
		Enabled:       true,
		MaxRetries:    1,
	}
	require.NoError(t, database.InsertWebhookConfig(wh))

	d := New(database)
	d.Start()
	defer d.Stop()

	// Send a "car" detection - should not match.
	d.OnDetection("cam1", "Front Door", &db.Detection{Class: "car", Confidence: 0.8})
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&callCount))

	// Send a "person" detection - should match.
	d.OnDetection("cam1", "Front Door", &db.Detection{Class: "person", Confidence: 0.9})
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount))
}

func TestDispatchCameraFilter(t *testing.T) {
	database := setupTestDB(t)

	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Webhook only for cam1.
	wh := &db.WebhookConfig{
		ID:       "wh1",
		Name:     "Cam1 Only",
		URL:      server.URL,
		CameraID: "cam1",
		Enabled:  true,
	}
	require.NoError(t, database.InsertWebhookConfig(wh))

	d := New(database)
	d.Start()
	defer d.Stop()

	// Wrong camera.
	d.OnDetection("cam2", "Back Door", &db.Detection{Class: "person", Confidence: 0.9})
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&callCount))

	// Right camera.
	d.OnDetection("cam1", "Front Door", &db.Detection{Class: "person", Confidence: 0.9})
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount))
}

func TestDispatchRetryOnFailure(t *testing.T) {
	database := setupTestDB(t)

	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		if n <= 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	wh := &db.WebhookConfig{
		ID:         "wh1",
		Name:       "Retry Test",
		URL:        server.URL,
		Enabled:    true,
		MaxRetries: 3,
	}
	require.NoError(t, database.InsertWebhookConfig(wh))

	d := New(database)
	d.Start()
	defer d.Stop()

	d.OnDetection("cam1", "Front Door", &db.Detection{Class: "person", Confidence: 0.9})

	// Wait for retry loop (polls every 5s + backoff).
	time.Sleep(8 * time.Second)

	assert.GreaterOrEqual(t, atomic.LoadInt32(&callCount), int32(2))

	deliveries, err := database.ListWebhookDeliveries("wh1", 10)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	// Should eventually succeed after retry.
	assert.Equal(t, "success", deliveries[0].Status)
}

func TestMatchesFilter(t *testing.T) {
	tests := []struct {
		name   string
		config *db.WebhookConfig
		event  *DetectionEvent
		match  bool
	}{
		{
			name:   "no filters matches all",
			config: &db.WebhookConfig{},
			event:  &DetectionEvent{EventType: "detection", CameraID: "cam1"},
			match:  true,
		},
		{
			name:   "camera filter matches",
			config: &db.WebhookConfig{CameraID: "cam1"},
			event:  &DetectionEvent{EventType: "detection", CameraID: "cam1"},
			match:  true,
		},
		{
			name:   "camera filter rejects",
			config: &db.WebhookConfig{CameraID: "cam2"},
			event:  &DetectionEvent{EventType: "detection", CameraID: "cam1"},
			match:  false,
		},
		{
			name:   "event type filter matches",
			config: &db.WebhookConfig{EventTypes: "detection,motion"},
			event:  &DetectionEvent{EventType: "motion", CameraID: "cam1"},
			match:  true,
		},
		{
			name:   "event type filter rejects",
			config: &db.WebhookConfig{EventTypes: "motion"},
			event:  &DetectionEvent{EventType: "detection", CameraID: "cam1"},
			match:  false,
		},
		{
			name:   "object class filter matches",
			config: &db.WebhookConfig{ObjectClasses: "person,car"},
			event:  &DetectionEvent{EventType: "detection", CameraID: "cam1", Detection: &DetectionInfo{Class: "car", Confidence: 0.9}},
			match:  true,
		},
		{
			name:   "object class filter rejects",
			config: &db.WebhookConfig{ObjectClasses: "person"},
			event:  &DetectionEvent{EventType: "detection", CameraID: "cam1", Detection: &DetectionInfo{Class: "car", Confidence: 0.9}},
			match:  false,
		},
		{
			name:   "min confidence rejects",
			config: &db.WebhookConfig{MinConfidence: 0.8},
			event:  &DetectionEvent{EventType: "detection", CameraID: "cam1", Detection: &DetectionInfo{Class: "person", Confidence: 0.5}},
			match:  false,
		},
		{
			name:   "min confidence passes",
			config: &db.WebhookConfig{MinConfidence: 0.5},
			event:  &DetectionEvent{EventType: "detection", CameraID: "cam1", Detection: &DetectionInfo{Class: "person", Confidence: 0.8}},
			match:  true,
		},
	}

	d := &Dispatcher{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.match, d.matches(tt.config, tt.event))
		})
	}
}
