# KAI-20: ONVIF Analytics Events Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend ONVIF event handling to support Profile T analytics events (line crossing, intrusion, loitering, object counting), storing them in the database and exposing them via a unified API endpoint.

**Architecture:** Extend the existing event pipeline — add new `DetectedEventType` constants, widen `classifyTopic()` and `parseEvents()`, add a `metadata` column to `motion_events`, extend the scheduler dispatcher, add SSE publish methods, and create a new unified `GET /cameras/:id/events` API endpoint with type filtering.

**Tech Stack:** Go, SQLite, Gin HTTP router, ONVIF WS-BaseNotification XML

---

### Task 1: Add new event type constants and extend topic classification

**Files:**

- Modify: `internal/nvr/onvif/events.go:20-27` (constants) and `events.go:515-524` (classifyTopic)
- Test: `internal/nvr/onvif/events_test.go` (new file)

- [ ] **Step 1: Create test file with classifyTopic tests**

Create `internal/nvr/onvif/events_test.go`:

```go
package onvif

import "testing"

func TestClassifyTopic(t *testing.T) {
	tests := []struct {
		topic    string
		wantType DetectedEventType
		wantOK   bool
	}{
		// Existing: motion
		{"tns1:RuleEngine/CellMotionDetector/Motion", EventMotion, true},
		{"tns1:VideoSource/MotionAlarm", EventMotion, true},
		// Existing: tampering
		{"tns1:VideoSource/GlobalSceneChange/ImagingService", EventTampering, true},
		{"tns1:VideoSource/Tamper", EventTampering, true},
		// New: line crossing
		{"tns1:RuleEngine/LineCrossingDetector/Crossed", EventLineCrossing, true},
		{"tns1:RuleEngine/LineCounter/Crossed", EventLineCrossing, true},
		// New: intrusion / field detection
		{"tns1:RuleEngine/FieldDetector/ObjectsInside", EventIntrusion, true},
		{"tns1:RuleEngine/IntrusionDetection/Alert", EventIntrusion, true},
		{"tns1:RuleEngine/FieldDetection/ObjectsInside", EventIntrusion, true},
		// New: loitering
		{"tns1:RuleEngine/LoiteringDetector/Alert", EventLoitering, true},
		// New: object counting
		{"tns1:RuleEngine/ObjectCounting/Count", EventObjectCount, true},
		{"tns1:RuleEngine/CountAggregation/Counting", EventObjectCount, true},
		// Unknown topic
		{"tns1:Device/HardwareFailure", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		gotType, gotOK := classifyTopic(tt.topic)
		if gotType != tt.wantType || gotOK != tt.wantOK {
			t.Errorf("classifyTopic(%q) = (%q, %v), want (%q, %v)",
				tt.topic, gotType, gotOK, tt.wantType, tt.wantOK)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/onvif && go test -run TestClassifyTopic -v`
Expected: FAIL — `EventLineCrossing`, `EventIntrusion`, etc. are undefined.

- [ ] **Step 3: Add new event type constants**

In `internal/nvr/onvif/events.go`, replace the constants block (lines 20-27):

```go
// DetectedEventType identifies the kind of ONVIF event detected.
type DetectedEventType string

const (
	// EventMotion represents a motion detection event.
	EventMotion DetectedEventType = "motion"
	// EventTampering represents a camera tampering / global scene change event.
	EventTampering DetectedEventType = "tampering"
	// EventLineCrossing represents a virtual line crossing detection event.
	EventLineCrossing DetectedEventType = "line_crossing"
	// EventIntrusion represents a field/intrusion detection event.
	EventIntrusion DetectedEventType = "intrusion"
	// EventLoitering represents a loitering detection event.
	EventLoitering DetectedEventType = "loitering"
	// EventObjectCount represents an object counting event.
	EventObjectCount DetectedEventType = "object_count"
)
```

- [ ] **Step 4: Extend classifyTopic**

Replace the `classifyTopic` function in `events.go` (lines 515-524):

```go
// classifyTopic determines the DetectedEventType from a notification topic string.
// Returns the event type and true if the topic is recognized, or ("", false) otherwise.
func classifyTopic(topic string) (DetectedEventType, bool) {
	lower := strings.ToLower(topic)
	if strings.Contains(lower, "motion") || strings.Contains(lower, "cellmotion") {
		return EventMotion, true
	}
	if strings.Contains(lower, "globalscenechange") || strings.Contains(lower, "tamper") {
		return EventTampering, true
	}
	if strings.Contains(lower, "linecrossing") || strings.Contains(lower, "linecounter") {
		return EventLineCrossing, true
	}
	if strings.Contains(lower, "fielddetect") || strings.Contains(lower, "intrusiondetect") {
		return EventIntrusion, true
	}
	if strings.Contains(lower, "loitering") {
		return EventLoitering, true
	}
	if strings.Contains(lower, "objectcount") || strings.Contains(lower, "counting") {
		return EventObjectCount, true
	}
	return "", false
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd internal/nvr/onvif && go test -run TestClassifyTopic -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/onvif/events.go internal/nvr/onvif/events_test.go
git commit -m "feat(events): add Profile T analytics event type constants and topic classification"
```

---

### Task 2: Extend event parsing to extract metadata

**Files:**

- Modify: `internal/nvr/onvif/events.go:29-33` (DetectedEvent struct) and `events.go:528-562` (parseEvents)
- Test: `internal/nvr/onvif/events_test.go`

- [ ] **Step 1: Write tests for parseEvents with analytics event XML**

Append to `internal/nvr/onvif/events_test.go`:

```go
func TestParseEventsLineCrossing(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <wsnt:Notify xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
      <wsnt:NotificationMessage>
        <wsnt:Topic>tns1:RuleEngine/LineCrossingDetector/Crossed</wsnt:Topic>
        <wsnt:Message>
          <tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
            <tt:Data>
              <tt:SimpleItem Name="State" Value="true"/>
              <tt:SimpleItem Name="Direction" Value="LeftToRight"/>
            </tt:Data>
          </tt:Message>
        </wsnt:Message>
      </wsnt:NotificationMessage>
    </wsnt:Notify>
  </s:Body>
</s:Envelope>`

	events, err := parseEvents([]byte(xml))
	if err != nil {
		t.Fatalf("parseEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventLineCrossing {
		t.Errorf("expected type %q, got %q", EventLineCrossing, events[0].Type)
	}
	if !events[0].Active {
		t.Error("expected active=true")
	}
	if events[0].Metadata == nil || events[0].Metadata["direction"] != "LeftToRight" {
		t.Errorf("expected metadata direction=LeftToRight, got %v", events[0].Metadata)
	}
}

func TestParseEventsObjectCount(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <wsnt:Notify xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
      <wsnt:NotificationMessage>
        <wsnt:Topic>tns1:RuleEngine/ObjectCounting/Count</wsnt:Topic>
        <wsnt:Message>
          <tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
            <tt:Data>
              <tt:SimpleItem Name="Count" Value="5"/>
            </tt:Data>
          </tt:Message>
        </wsnt:Message>
      </wsnt:NotificationMessage>
    </wsnt:Notify>
  </s:Body>
</s:Envelope>`

	events, err := parseEvents([]byte(xml))
	if err != nil {
		t.Fatalf("parseEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventObjectCount {
		t.Errorf("expected type %q, got %q", EventObjectCount, events[0].Type)
	}
	if !events[0].Active {
		t.Error("expected active=true for count > 0")
	}
	if events[0].Metadata == nil || events[0].Metadata["count"] != "5" {
		t.Errorf("expected metadata count=5, got %v", events[0].Metadata)
	}
}

func TestParseEventsObjectCountZero(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <wsnt:Notify xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
      <wsnt:NotificationMessage>
        <wsnt:Topic>tns1:RuleEngine/ObjectCounting/Count</wsnt:Topic>
        <wsnt:Message>
          <tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
            <tt:Data>
              <tt:SimpleItem Name="Count" Value="0"/>
            </tt:Data>
          </tt:Message>
        </wsnt:Message>
      </wsnt:NotificationMessage>
    </wsnt:Notify>
  </s:Body>
</s:Envelope>`

	events, err := parseEvents([]byte(xml))
	if err != nil {
		t.Fatalf("parseEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Active {
		t.Error("expected active=false for count == 0")
	}
}

func TestParseEventsIntrusion(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <wsnt:Notify xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
      <wsnt:NotificationMessage>
        <wsnt:Topic>tns1:RuleEngine/FieldDetector/ObjectsInside</wsnt:Topic>
        <wsnt:Message>
          <tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
            <tt:Data>
              <tt:SimpleItem Name="State" Value="true"/>
            </tt:Data>
          </tt:Message>
        </wsnt:Message>
      </wsnt:NotificationMessage>
    </wsnt:Notify>
  </s:Body>
</s:Envelope>`

	events, err := parseEvents([]byte(xml))
	if err != nil {
		t.Fatalf("parseEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventIntrusion {
		t.Errorf("expected type %q, got %q", EventIntrusion, events[0].Type)
	}
	if !events[0].Active {
		t.Error("expected active=true")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd internal/nvr/onvif && go test -run "TestParseEvents(LineCrossing|ObjectCount|Intrusion)" -v`
Expected: FAIL — `DetectedEvent` has no `Metadata` field.

- [ ] **Step 3: Add Metadata field to DetectedEvent**

In `events.go`, update the `DetectedEvent` struct (lines 29-33):

```go
// DetectedEvent carries the type and active state of a single ONVIF event.
type DetectedEvent struct {
	Type     DetectedEventType
	Active   bool
	Metadata map[string]string // optional key-value metadata (e.g., direction, count)
}
```

- [ ] **Step 4: Rewrite parseEvents to handle all event types and extract metadata**

Replace the `parseEvents` function in `events.go` (lines 528-562):

```go
// parseEvents scans PullMessages or Notify responses for all recognized events.
// It returns ALL detected events (motion, tampering, analytics, etc.).
func parseEvents(body []byte) ([]DetectedEvent, error) {
	var env soapEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// Check both PullMessages and Notify message arrays.
	messages := env.Body.PullResponse.Messages
	if len(messages) == 0 {
		messages = env.Body.Notify.Messages
	}

	var detected []DetectedEvent
	for _, msg := range messages {
		eventType, ok := classifyTopic(msg.Topic.Value)
		if !ok {
			continue
		}

		items := msg.Message.InnerMessage.Data.SimpleItems

		switch eventType {
		case EventObjectCount:
			evt := DetectedEvent{Type: eventType, Metadata: make(map[string]string)}
			for _, item := range items {
				nameLower := strings.ToLower(item.Name)
				if nameLower == "count" || nameLower == "objectcount" {
					evt.Metadata["count"] = item.Value
					count := 0
					fmt.Sscanf(item.Value, "%d", &count)
					evt.Active = count > 0
				}
			}
			// If no count property found, treat as active (camera-dependent).
			if _, hasCount := evt.Metadata["count"]; !hasCount {
				evt.Active = true
			}
			detected = append(detected, evt)

		case EventLineCrossing:
			evt := DetectedEvent{Type: eventType, Metadata: make(map[string]string)}
			for _, item := range items {
				nameLower := strings.ToLower(item.Name)
				if nameLower == "state" || nameLower == "ismotion" {
					valueLower := strings.ToLower(strings.TrimSpace(item.Value))
					evt.Active = valueLower == "true" || valueLower == "1"
				}
				if nameLower == "direction" {
					evt.Metadata["direction"] = item.Value
				}
			}
			detected = append(detected, evt)

		default:
			// Motion, tampering, intrusion, loitering — standard state-based.
			for _, item := range items {
				nameLower := strings.ToLower(item.Name)
				if nameLower == "ismotion" || nameLower == "state" {
					valueLower := strings.ToLower(strings.TrimSpace(item.Value))
					active := valueLower == "true" || valueLower == "1"
					detected = append(detected, DetectedEvent{
						Type:   eventType,
						Active: active,
					})
					break
				}
			}
		}
	}

	return detected, nil
}
```

- [ ] **Step 5: Run all events tests to verify they pass**

Run: `cd internal/nvr/onvif && go test -run "TestClassifyTopic|TestParseEvents" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/onvif/events.go internal/nvr/onvif/events_test.go
git commit -m "feat(events): extend event parsing for analytics events with metadata extraction"
```

---

### Task 3: Database migration — add metadata column

**Files:**

- Modify: `internal/nvr/db/migrations.go:455` (add migration 31)
- Modify: `internal/nvr/db/motion_events.go:8-19` (MotionEvent struct)
- Test: `internal/nvr/db/db_test.go` (existing test validates migrations)

- [ ] **Step 1: Write test for metadata column**

Append to `internal/nvr/db/db_test.go` — but first, check what's already there. Add this test:

Create a new file `internal/nvr/db/motion_events_test.go`:

```go
package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInsertMotionEventWithMetadata(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(dbPath)
	require.NoError(t, err)
	defer d.Close()

	// Insert a camera first.
	cam := &Camera{Name: "TestCam", RTSPURL: "rtsp://test"}
	require.NoError(t, d.CreateCamera(cam))

	metadata := `{"direction":"LeftToRight"}`
	event := &MotionEvent{
		CameraID:  cam.ID,
		StartedAt: "2026-04-03T10:00:00.000Z",
		EventType: "line_crossing",
		Metadata:  &metadata,
	}
	err = d.InsertMotionEvent(event)
	require.NoError(t, err)
	require.NotZero(t, event.ID)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/db && go test -run TestInsertMotionEventWithMetadata -v`
Expected: FAIL — `MotionEvent` has no `Metadata` field.

- [ ] **Step 3: Add migration 31**

Append to the `migrations` slice in `internal/nvr/db/migrations.go` (before the closing `}`):

```go
	// Migration 31: Add metadata column for analytics event details (KAI-20).
	{
		version: 31,
		sql:     `ALTER TABLE motion_events ADD COLUMN metadata TEXT;`,
	},
```

- [ ] **Step 4: Add Metadata field to MotionEvent struct**

In `internal/nvr/db/motion_events.go`, update the struct (lines 8-19):

```go
// MotionEvent represents a motion detection event for a camera.
type MotionEvent struct {
	ID               int64   `json:"id"`
	CameraID         string  `json:"camera_id"`
	StartedAt        string  `json:"started_at"`
	EndedAt          *string `json:"ended_at"`
	ThumbnailPath    string  `json:"thumbnail_path,omitempty"`
	EventType        string  `json:"event_type"`
	ObjectClass      string  `json:"object_class"`
	Confidence       float64 `json:"confidence"`
	Embedding        []byte  `json:"-"`
	DetectionSummary string  `json:"detection_summary,omitempty"`
	Metadata         *string `json:"metadata,omitempty"`
}
```

- [ ] **Step 5: Update InsertMotionEvent to include metadata**

In `internal/nvr/db/motion_events.go`, replace `InsertMotionEvent` (lines 22-42):

```go
// InsertMotionEvent inserts a new motion event into the database.
func (d *DB) InsertMotionEvent(event *MotionEvent) error {
	if event.EventType == "" {
		event.EventType = "motion"
	}
	res, err := d.Exec(`
		INSERT INTO motion_events (camera_id, started_at, ended_at, thumbnail_path, event_type, object_class, confidence, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		event.CameraID, event.StartedAt, event.EndedAt, event.ThumbnailPath, event.EventType,
		event.ObjectClass, event.Confidence, event.Metadata,
	)
	if err != nil {
		return err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	event.ID = id
	return nil
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `cd internal/nvr/db && go test -run TestInsertMotionEventWithMetadata -v`
Expected: PASS

- [ ] **Step 7: Run existing DB tests to ensure no regressions**

Run: `cd internal/nvr/db && go test -v`
Expected: All existing tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/nvr/db/migrations.go internal/nvr/db/motion_events.go internal/nvr/db/motion_events_test.go
git commit -m "feat(db): add metadata column to motion_events for analytics event data"
```

---

### Task 4: Add QueryEvents with type filtering

**Files:**

- Modify: `internal/nvr/db/motion_events.go` (add QueryEvents method)
- Modify: `internal/nvr/db/motion_events.go` (update QueryMotionEvents and QueryMotionEventsByClass to include metadata in SELECT)
- Test: `internal/nvr/db/motion_events_test.go`

- [ ] **Step 1: Write test for QueryEvents**

Append to `internal/nvr/db/motion_events_test.go`:

```go
import (
	"time"
)

func TestQueryEvents(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(dbPath)
	require.NoError(t, err)
	defer d.Close()

	cam := &Camera{Name: "TestCam", RTSPURL: "rtsp://test"}
	require.NoError(t, d.CreateCamera(cam))

	// Insert events of different types.
	for _, et := range []string{"motion", "tampering", "line_crossing", "intrusion"} {
		require.NoError(t, d.InsertMotionEvent(&MotionEvent{
			CameraID:  cam.ID,
			StartedAt: "2026-04-03T10:00:00.000Z",
			EventType: et,
		}))
	}

	start, _ := time.Parse(time.RFC3339, "2026-04-03T00:00:00Z")
	end, _ := time.Parse(time.RFC3339, "2026-04-04T00:00:00Z")

	// Query all types.
	all, err := d.QueryEvents(cam.ID, start, end, nil)
	require.NoError(t, err)
	require.Len(t, all, 4)

	// Query single type.
	lc, err := d.QueryEvents(cam.ID, start, end, []string{"line_crossing"})
	require.NoError(t, err)
	require.Len(t, lc, 1)
	require.Equal(t, "line_crossing", lc[0].EventType)

	// Query multiple types.
	multi, err := d.QueryEvents(cam.ID, start, end, []string{"motion", "intrusion"})
	require.NoError(t, err)
	require.Len(t, multi, 2)
}
```

Note: The import block at the top of the file should be merged with the existing imports. The final file should have a single import block:

```go
import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/db && go test -run TestQueryEvents -v`
Expected: FAIL — `QueryEvents` method does not exist.

- [ ] **Step 3: Implement QueryEvents**

Append to `internal/nvr/db/motion_events.go`:

```go
// QueryEvents returns events for a camera that overlap the given time range,
// optionally filtered by event type. When eventTypes is nil or empty, all
// types are returned.
func (d *DB) QueryEvents(cameraID string, start, end time.Time, eventTypes []string) ([]*MotionEvent, error) {
	query := `
		SELECT id, camera_id, started_at, ended_at, COALESCE(thumbnail_path, ''),
		       COALESCE(event_type, 'motion'), COALESCE(object_class, ''),
		       COALESCE(confidence, 0), metadata
		FROM motion_events
		WHERE camera_id = ?
		  AND started_at < ?
		  AND (ended_at IS NULL OR ended_at > ?)`

	args := []interface{}{
		cameraID,
		end.UTC().Format(timeFormat),
		start.UTC().Format(timeFormat),
	}

	if len(eventTypes) > 0 {
		placeholders := make([]string, len(eventTypes))
		for i, et := range eventTypes {
			placeholders[i] = "?"
			args = append(args, et)
		}
		query += ` AND event_type IN (` + strings.Join(placeholders, ",") + `)`
	}

	query += ` ORDER BY started_at`

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*MotionEvent
	for rows.Next() {
		ev := &MotionEvent{}
		if err := rows.Scan(&ev.ID, &ev.CameraID, &ev.StartedAt, &ev.EndedAt,
			&ev.ThumbnailPath, &ev.EventType, &ev.ObjectClass, &ev.Confidence,
			&ev.Metadata); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}
```

Also add `"strings"` to the import block at the top of `motion_events.go`:

```go
import (
	"strings"
	"time"
)
```

- [ ] **Step 4: Update existing query methods to include metadata in SELECT**

In `QueryMotionEvents` (line 113), update the SELECT to include metadata:

```go
	rows, err := d.Query(`
		SELECT id, camera_id, started_at, ended_at, COALESCE(thumbnail_path, ''), COALESCE(event_type, 'motion'),
		       COALESCE(object_class, ''), COALESCE(confidence, 0), metadata
		FROM motion_events
		WHERE camera_id = ?
		  AND started_at < ?
		  AND (ended_at IS NULL OR ended_at > ?)
		ORDER BY started_at`,
```

And update the Scan call (line 132) to include `&ev.Metadata`:

```go
		if err := rows.Scan(&ev.ID, &ev.CameraID, &ev.StartedAt, &ev.EndedAt, &ev.ThumbnailPath, &ev.EventType,
			&ev.ObjectClass, &ev.Confidence, &ev.Metadata); err != nil {
```

Do the same for `QueryMotionEventsByClass` — update its SELECT (line 206) and Scan (line 235) to include `metadata` and `&ev.Metadata`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd internal/nvr/db && go test -v`
Expected: All tests PASS including the new ones.

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/db/motion_events.go internal/nvr/db/motion_events_test.go
git commit -m "feat(db): add QueryEvents method with event type filtering"
```

---

### Task 5: Add intensity query with event type filtering

**Files:**

- Modify: `internal/nvr/db/motion_events_intensity.go`

- [ ] **Step 1: Write test for GetMotionIntensityByType**

Create `internal/nvr/db/motion_events_intensity_test.go`:

```go
package db

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetMotionIntensityByType(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(dbPath)
	require.NoError(t, err)
	defer d.Close()

	cam := &Camera{Name: "TestCam", RTSPURL: "rtsp://test"}
	require.NoError(t, d.CreateCamera(cam))

	// Insert motion and line_crossing events.
	require.NoError(t, d.InsertMotionEvent(&MotionEvent{
		CameraID: cam.ID, StartedAt: "2026-04-03T10:00:00.000Z", EventType: "motion",
	}))
	require.NoError(t, d.InsertMotionEvent(&MotionEvent{
		CameraID: cam.ID, StartedAt: "2026-04-03T10:00:30.000Z", EventType: "line_crossing",
	}))

	start, _ := time.Parse(time.RFC3339, "2026-04-03T00:00:00Z")
	end, _ := time.Parse(time.RFC3339, "2026-04-04T00:00:00Z")

	// All types — should return 2 events total.
	all, err := d.GetMotionIntensityByType(cam.ID, start, end, 60, "")
	require.NoError(t, err)
	total := 0
	for _, b := range all {
		total += b.Count
	}
	require.Equal(t, 2, total)

	// Filtered to line_crossing only.
	lc, err := d.GetMotionIntensityByType(cam.ID, start, end, 60, "line_crossing")
	require.NoError(t, err)
	total = 0
	for _, b := range lc {
		total += b.Count
	}
	require.Equal(t, 1, total)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/db && go test -run TestGetMotionIntensityByType -v`
Expected: FAIL — `GetMotionIntensityByType` does not exist.

- [ ] **Step 3: Add GetMotionIntensityByType**

Append to `internal/nvr/db/motion_events_intensity.go`:

```go
// GetMotionIntensityByType returns event counts bucketed by the given interval,
// optionally filtered by event type. When eventType is empty, all types are included.
func (d *DB) GetMotionIntensityByType(cameraID string, start, end time.Time, bucketSeconds int, eventType string) ([]IntensityBucket, error) {
	query := `
        SELECT
            strftime('%s', started_at) / ? * ? as bucket_epoch,
            COUNT(*) as count
        FROM motion_events
        WHERE camera_id = ?
            AND started_at >= ?
            AND started_at < ?`

	args := []interface{}{
		bucketSeconds, bucketSeconds,
		cameraID,
		start.UTC().Format(timeFormat),
		end.UTC().Format(timeFormat),
	}

	if eventType != "" {
		query += ` AND event_type = ?`
		args = append(args, eventType)
	}

	query += `
        GROUP BY bucket_epoch
        ORDER BY bucket_epoch`

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var buckets []IntensityBucket
	for rows.Next() {
		var epochSec int64
		var count int
		if err := rows.Scan(&epochSec, &count); err != nil {
			return nil, err
		}
		buckets = append(buckets, IntensityBucket{
			BucketStart: time.Unix(epochSec, 0).UTC(),
			Count:       count,
		})
	}
	return buckets, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd internal/nvr/db && go test -run TestGetMotionIntensityByType -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/db/motion_events_intensity.go internal/nvr/db/motion_events_intensity_test.go
git commit -m "feat(db): add GetMotionIntensityByType for filtered intensity queries"
```

---

### Task 6: Add SSE publish methods for new event types

**Files:**

- Modify: `internal/nvr/api/events.go` (add publish methods)
- Modify: `internal/nvr/scheduler/scheduler.go:27-37` (EventPublisher interface)

- [ ] **Step 1: Add publish methods to EventBroadcaster**

Append after `PublishTampering` in `internal/nvr/api/events.go` (after line 137):

```go
// PublishLineCrossing publishes a line-crossing detection event.
func (b *EventBroadcaster) PublishLineCrossing(cameraName string) {
	b.Publish(Event{
		Type:    "line_crossing",
		Camera:  cameraName,
		Message: fmt.Sprintf("Line crossing detected on %s", cameraName),
	})
}

// PublishIntrusion publishes a field/intrusion detection event.
func (b *EventBroadcaster) PublishIntrusion(cameraName string) {
	b.Publish(Event{
		Type:    "intrusion",
		Camera:  cameraName,
		Message: fmt.Sprintf("Intrusion detected on %s", cameraName),
	})
}

// PublishLoitering publishes a loitering detection event.
func (b *EventBroadcaster) PublishLoitering(cameraName string) {
	b.Publish(Event{
		Type:    "loitering",
		Camera:  cameraName,
		Message: fmt.Sprintf("Loitering detected on %s", cameraName),
	})
}

// PublishObjectCount publishes an object counting event.
func (b *EventBroadcaster) PublishObjectCount(cameraName string, count int) {
	b.Publish(Event{
		Type:    "object_count",
		Camera:  cameraName,
		Message: fmt.Sprintf("Object count on %s: %d", cameraName, count),
	})
}
```

- [ ] **Step 2: Update the Event struct's type comment**

In `internal/nvr/api/events.go`, update the Event struct's Type field comment (line 31):

```go
	Type       string          `json:"type"`    // "motion", "tampering", "line_crossing", "intrusion", "loitering", "object_count", "camera_offline", "camera_online", "recording_started", "recording_stopped", "recording_stalled", "recording_recovered", "recording_failed", "detection_frame"
```

- [ ] **Step 3: Update EventPublisher interface in scheduler**

In `internal/nvr/scheduler/scheduler.go`, update the `EventPublisher` interface (lines 27-37):

```go
// EventPublisher is an interface for publishing system events from the scheduler.
// This avoids a circular import with the api package.
type EventPublisher interface {
	PublishMotion(cameraName string)
	PublishTampering(cameraName string)
	PublishLineCrossing(cameraName string)
	PublishIntrusion(cameraName string)
	PublishLoitering(cameraName string)
	PublishObjectCount(cameraName string, count int)
	PublishCameraOffline(cameraName string)
	PublishCameraOnline(cameraName string)
	PublishRecordingStarted(cameraName string)
	PublishRecordingStopped(cameraName string)
	PublishRecordingStalled(cameraName string)
	PublishRecordingRecovered(cameraName string)
	PublishRecordingFailed(cameraName string)
}
```

- [ ] **Step 4: Verify compilation**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...`
Expected: Build succeeds.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/api/events.go internal/nvr/scheduler/scheduler.go
git commit -m "feat(events): add SSE publish methods for analytics event types"
```

---

### Task 7: Extend scheduler event dispatcher for new event types

**Files:**

- Modify: `internal/nvr/scheduler/scheduler.go` (both eventCallback functions)

- [ ] **Step 1: Update the first eventCallback (recording-controlling subscription)**

In `scheduler.go`, the eventCallback starting around line 1002. After the `case onvif.EventTampering:` block (ending at line 1043), add new cases before the closing `}` of the switch:

```go
		case onvif.EventLineCrossing, onvif.EventIntrusion, onvif.EventLoitering, onvif.EventObjectCount:
			// Analytics events: drive recording like motion and persist to DB.
			s.mu.Lock()
			for sk, msm := range s.motionSMs {
				if sk == cam.ID || strings.HasPrefix(sk, cam.ID+":") {
					msm.OnMotion(active)
				}
			}
			s.mu.Unlock()
			now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			evtType := string(eventType)
			if active {
				s.StartMotionTimer(camID, cam.MotionTimeoutSeconds)
				if !s.db.HasOpenMotionEvent(camID) {
					_ = s.db.InsertMotionEvent(&db.MotionEvent{
						CameraID:  camID,
						StartedAt: now,
						EventType: evtType,
					})
				}
				if s.eventPub != nil {
					switch eventType {
					case onvif.EventLineCrossing:
						s.eventPub.PublishLineCrossing(cam.Name)
					case onvif.EventIntrusion:
						s.eventPub.PublishIntrusion(cam.Name)
					case onvif.EventLoitering:
						s.eventPub.PublishLoitering(cam.Name)
					case onvif.EventObjectCount:
						s.eventPub.PublishObjectCount(cam.Name, 0)
					}
				}
			} else {
				s.CancelMotionTimer(camID)
				_ = s.db.EndMotionEvent(camID, now)
			}
```

- [ ] **Step 2: Update the second eventCallback (motion-alert-only subscription)**

In `scheduler.go`, the second eventCallback starting around line 1098. After the `case onvif.EventTampering:` block (ending at line 1160), add the same pattern:

```go
		case onvif.EventLineCrossing, onvif.EventIntrusion, onvif.EventLoitering, onvif.EventObjectCount:
			s.mu.Lock()
			for sk, msm := range s.motionSMs {
				if sk == cam.ID || strings.HasPrefix(sk, cam.ID+":") {
					msm.OnMotion(active)
				}
			}
			s.mu.Unlock()
			now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			evtType := string(eventType)
			if active {
				s.StartMotionTimer(cam.ID, cam.MotionTimeoutSeconds)
				if !s.db.HasOpenMotionEvent(cam.ID) {
					_ = s.db.InsertMotionEvent(&db.MotionEvent{
						CameraID:  cam.ID,
						StartedAt: now,
						EventType: evtType,
					})
				}
				if s.eventPub != nil {
					switch eventType {
					case onvif.EventLineCrossing:
						s.eventPub.PublishLineCrossing(cam.Name)
					case onvif.EventIntrusion:
						s.eventPub.PublishIntrusion(cam.Name)
					case onvif.EventLoitering:
						s.eventPub.PublishLoitering(cam.Name)
					case onvif.EventObjectCount:
						s.eventPub.PublishObjectCount(cam.Name, 0)
					}
				}
			} else {
				s.CancelMotionTimer(cam.ID)
				_ = s.db.EndMotionEvent(cam.ID, now)
			}
```

- [ ] **Step 3: Verify compilation**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...`
Expected: Build succeeds.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/scheduler/scheduler.go
git commit -m "feat(scheduler): dispatch analytics events through motion state machine and DB"
```

---

### Task 8: Add unified events API endpoint

**Files:**

- Modify: `internal/nvr/api/recordings.go` (add Events handler + update Intensity)
- Modify: `internal/nvr/api/router.go` (add route)
- Test: `internal/nvr/api/recordings_test.go`

- [ ] **Step 1: Write test for the Events endpoint**

Append to `internal/nvr/api/recordings_test.go`:

```go
func TestEventsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, _, cleanup := setupRecordingTest(t)
	defer cleanup()

	// Create test camera.
	cam := &db.Camera{Name: "EventCam", RTSPURL: "rtsp://test"}
	if err := database.CreateCamera(cam); err != nil {
		t.Fatalf("create camera: %v", err)
	}

	// Insert events of different types.
	for _, et := range []string{"motion", "line_crossing", "intrusion"} {
		if err := database.InsertMotionEvent(&db.MotionEvent{
			CameraID:  cam.ID,
			StartedAt: "2026-04-03T10:00:00.000Z",
			EventType: et,
		}); err != nil {
			t.Fatalf("insert event: %v", err)
		}
	}

	// Test: query all types.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?date=2026-04-03", nil)
	c.Params = gin.Params{{Key: "id", Value: cam.ID}}
	c.Set("camera_permissions", "*")

	handler.Events(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var events []db.MotionEvent
	if err := json.Unmarshal(w.Body.Bytes(), &events); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Test: filter by type.
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?date=2026-04-03&type=line_crossing", nil)
	c.Params = gin.Params{{Key: "id", Value: cam.ID}}
	c.Set("camera_permissions", "*")

	handler.Events(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if err := json.Unmarshal(w.Body.Bytes(), &events); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "line_crossing" {
		t.Errorf("expected line_crossing, got %s", events[0].EventType)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/api && go test -run TestEventsEndpoint -v`
Expected: FAIL — `Events` method does not exist on `RecordingHandler`.

- [ ] **Step 3: Add Events handler**

Append to `internal/nvr/api/recordings.go`:

```go
// Events returns events for a camera on a given date, optionally filtered by type.
// Path param: id (camera ID). Query params: date (YYYY-MM-DD), type (comma-separated event types, optional).
func (h *RecordingHandler) Events(c *gin.Context) {
	cameraID := c.Param("id")

	if !hasCameraPermission(c, cameraID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
		return
	}

	dateStr := c.Query("date")
	date, err := time.ParseInLocation("2006-01-02", dateStr, time.Now().Location())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date, expected YYYY-MM-DD"})
		return
	}

	start := date
	end := date.Add(24 * time.Hour)

	var eventTypes []string
	if typeParam := c.Query("type"); typeParam != "" {
		eventTypes = strings.Split(typeParam, ",")
	}

	events, err := h.DB.QueryEvents(cameraID, start, end, eventTypes)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query events", err)
		return
	}

	if events == nil {
		events = []*db.MotionEvent{}
	}

	c.JSON(http.StatusOK, events)
}
```

- [ ] **Step 4: Update Intensity handler to support event_type filter**

In `internal/nvr/api/recordings.go`, update the `Intensity` method. After the `bucketSeconds` parsing (around line 234), add:

```go
	eventType := c.Query("event_type")
```

And replace the `h.DB.GetMotionIntensity(...)` call with:

```go
	var buckets []db.IntensityBucket
	if eventType != "" {
		buckets, err = h.DB.GetMotionIntensityByType(cameraID, start, end, bucketSeconds, eventType)
	} else {
		buckets, err = h.DB.GetMotionIntensity(cameraID, start, end, bucketSeconds)
	}
```

- [ ] **Step 5: Add route**

In `internal/nvr/api/router.go`, after the existing motion-events line (line 306):

```go
	protected.GET("/cameras/:id/events", recordingHandler.Events)
```

Add this line after `protected.GET("/cameras/:id/motion-events", recordingHandler.MotionEvents)`.

- [ ] **Step 6: Run test to verify it passes**

Run: `cd internal/nvr/api && go test -run TestEventsEndpoint -v`
Expected: PASS

- [ ] **Step 7: Run all API tests to verify no regressions**

Run: `cd internal/nvr/api && go test -v`
Expected: All tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/nvr/api/recordings.go internal/nvr/api/router.go internal/nvr/api/recordings_test.go
git commit -m "feat(api): add unified GET /cameras/:id/events endpoint with type filtering"
```

---

### Task 9: Full build and integration verification

**Files:** None (verification only)

- [ ] **Step 1: Run full build**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./...`
Expected: Build succeeds with no errors.

- [ ] **Step 2: Run all NVR tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/...`
Expected: All tests PASS.

- [ ] **Step 3: Run the sample config test**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test -run TestSampleConfFile -v ./...`
Expected: PASS (we didn't touch mediamtx.yml).

- [ ] **Step 4: Commit any fixups if needed, then final verification**

Run: `go vet ./internal/nvr/...`
Expected: No issues.
