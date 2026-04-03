# KAI-110: Event Service Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add GetEventProperties support and four new event topics (DigitalInput, SignalLoss, HardwareFailure, Relay) to the ONVIF event service.

**Architecture:** Extend the existing event subscriber with a GetEventProperties SOAP call that discovers supported topics during probe. Add four new DetectedEventType constants and extend classifyTopic() to recognize them. Store supported topics as JSON in a new camera column. Handle new events in the scheduler using the same pattern as tampering (insert/end in DB, broadcast via SSE).

**Tech Stack:** Go, SOAP/XML, SQLite, SSE

---

## File Structure

| File | Responsibility |
|------|---------------|
| `internal/nvr/onvif/events.go` | Event types, classifyTopic(), GetEventProperties SOAP, XML parsing |
| `internal/nvr/onvif/events_test.go` | Unit tests for classifyTopic() and GetEventProperties XML parsing |
| `internal/nvr/onvif/device.go` | Call GetEventProperties during ProbeDeviceFull, add topics to ProbeResult |
| `internal/nvr/db/migrations.go` | Migration 31: add supported_event_topics column |
| `internal/nvr/db/cameras.go` | Add SupportedEventTopics field to Camera struct, update all queries |
| `internal/nvr/api/cameras.go` | Save supported_event_topics during probe/refresh |
| `internal/nvr/api/events.go` | SSE publish methods for new event types |
| `internal/nvr/scheduler/scheduler.go` | Handle new event types in both event callbacks |

---

### Task 1: Add New Event Type Constants and Extend classifyTopic

**Files:**
- Modify: `internal/nvr/onvif/events.go:20-27` (DetectedEventType constants)
- Modify: `internal/nvr/onvif/events.go:515-524` (classifyTopic function)
- Create: `internal/nvr/onvif/events_test.go`

- [ ] **Step 1: Write tests for classifyTopic with all event types**

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
		{"tns1:VideoAnalytics/Motion", EventMotion, true},
		// Existing: tampering
		{"tns1:RuleEngine/TamperDetector/Tamper", EventTampering, true},
		{"tns1:VideoSource/GlobalSceneChange/ImagingService", EventTampering, true},
		// New: digital input
		{"tns1:Device/Trigger/DigitalInput", EventDigitalInput, true},
		{"tns1:Device/IO/Digital_Input", EventDigitalInput, true},
		{"tns1:Device/Trigger/LogicalState", EventDigitalInput, true},
		// New: signal loss
		{"tns1:VideoSource/SignalLoss", EventSignalLoss, true},
		{"tns1:MediaControl/VideoLoss", EventSignalLoss, true},
		// New: hardware failure
		{"tns1:Device/HardwareFailure/StorageFailure", EventHardwareFailure, true},
		{"tns1:Monitoring/ProcessorUsage", EventHardwareFailure, true},
		// New: relay
		{"tns1:Device/Trigger/Relay", EventRelay, true},
		{"tns1:Device/IO/RelayOutput", EventRelay, true},
		{"tns1:Device/Trigger/DigitalOutput", EventRelay, true},
		// Unknown
		{"tns1:SomeUnknown/Topic", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			gotType, gotOK := classifyTopic(tt.topic)
			if gotOK != tt.wantOK {
				t.Errorf("classifyTopic(%q) ok = %v, want %v", tt.topic, gotOK, tt.wantOK)
			}
			if gotType != tt.wantType {
				t.Errorf("classifyTopic(%q) type = %q, want %q", tt.topic, gotType, tt.wantType)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-110 && go test ./internal/nvr/onvif/ -run TestClassifyTopic -v`

Expected: FAIL — `EventDigitalInput`, `EventSignalLoss`, `EventHardwareFailure`, `EventRelay` are undefined.

- [ ] **Step 3: Add new event type constants**

In `internal/nvr/onvif/events.go`, replace the constants block (lines 22-27) with:

```go
const (
	// EventMotion represents a motion detection event.
	EventMotion DetectedEventType = "motion"
	// EventTampering represents a camera tampering / global scene change event.
	EventTampering DetectedEventType = "tampering"
	// EventDigitalInput represents a digital input trigger event.
	EventDigitalInput DetectedEventType = "digital_input"
	// EventSignalLoss represents a video signal loss event.
	EventSignalLoss DetectedEventType = "signal_loss"
	// EventHardwareFailure represents a hardware failure event.
	EventHardwareFailure DetectedEventType = "hardware_failure"
	// EventRelay represents a relay output event.
	EventRelay DetectedEventType = "relay"
)
```

- [ ] **Step 4: Extend classifyTopic**

In `internal/nvr/onvif/events.go`, replace the `classifyTopic` function (lines 515-524) with:

```go
func classifyTopic(topic string) (DetectedEventType, bool) {
	lower := strings.ToLower(topic)
	if strings.Contains(lower, "motion") || strings.Contains(lower, "cellmotion") {
		return EventMotion, true
	}
	if strings.Contains(lower, "globalscenechange") || strings.Contains(lower, "tamper") {
		return EventTampering, true
	}
	if strings.Contains(lower, "digitalinput") || strings.Contains(lower, "digital_input") || strings.Contains(lower, "logicalstate") {
		return EventDigitalInput, true
	}
	if strings.Contains(lower, "signalloss") || strings.Contains(lower, "videoloss") {
		return EventSignalLoss, true
	}
	if strings.Contains(lower, "hardwarefailure") || strings.Contains(lower, "processorusage") {
		return EventHardwareFailure, true
	}
	if strings.Contains(lower, "relayoutput") || strings.Contains(lower, "digitaloutput") {
		// Check relay-specific patterns before the generic "relay" to avoid false matches.
		return EventRelay, true
	}
	if strings.Contains(lower, "relay") {
		return EventRelay, true
	}
	return "", false
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-110 && go test ./internal/nvr/onvif/ -run TestClassifyTopic -v`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-110
git add internal/nvr/onvif/events.go internal/nvr/onvif/events_test.go
git commit -m "feat(events): add DigitalInput, SignalLoss, HardwareFailure, Relay event types"
```

---

### Task 2: Add parseEvents Test for New Event Types

**Files:**
- Modify: `internal/nvr/onvif/events_test.go`

- [ ] **Step 1: Write test for parseEvents handling new event types**

Append to `internal/nvr/onvif/events_test.go`:

```go
func TestParseEvents_AllTypes(t *testing.T) {
	// SOAP Notify body with one message per event type.
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
  <s:Body>
    <wsnt:Notify>
      <wsnt:NotificationMessage>
        <wsnt:Topic>tns1:RuleEngine/CellMotionDetector/Motion</wsnt:Topic>
        <wsnt:Message>
          <tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
            <tt:Data>
              <tt:SimpleItem Name="IsMotion" Value="true"/>
            </tt:Data>
          </tt:Message>
        </wsnt:Message>
      </wsnt:NotificationMessage>
      <wsnt:NotificationMessage>
        <wsnt:Topic>tns1:Device/Trigger/DigitalInput</wsnt:Topic>
        <wsnt:Message>
          <tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
            <tt:Data>
              <tt:SimpleItem Name="State" Value="true"/>
            </tt:Data>
          </tt:Message>
        </wsnt:Message>
      </wsnt:NotificationMessage>
      <wsnt:NotificationMessage>
        <wsnt:Topic>tns1:VideoSource/SignalLoss</wsnt:Topic>
        <wsnt:Message>
          <tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
            <tt:Data>
              <tt:SimpleItem Name="State" Value="false"/>
            </tt:Data>
          </tt:Message>
        </wsnt:Message>
      </wsnt:NotificationMessage>
      <wsnt:NotificationMessage>
        <wsnt:Topic>tns1:Device/HardwareFailure/StorageFailure</wsnt:Topic>
        <wsnt:Message>
          <tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
            <tt:Data>
              <tt:SimpleItem Name="State" Value="true"/>
            </tt:Data>
          </tt:Message>
        </wsnt:Message>
      </wsnt:NotificationMessage>
      <wsnt:NotificationMessage>
        <wsnt:Topic>tns1:Device/Trigger/Relay</wsnt:Topic>
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
</s:Envelope>`)

	events, err := parseEvents(body)
	if err != nil {
		t.Fatalf("parseEvents error: %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("got %d events, want 5", len(events))
	}

	expected := []DetectedEvent{
		{Type: EventMotion, Active: true},
		{Type: EventDigitalInput, Active: true},
		{Type: EventSignalLoss, Active: false},
		{Type: EventHardwareFailure, Active: true},
		{Type: EventRelay, Active: true},
	}
	for i, want := range expected {
		if events[i].Type != want.Type {
			t.Errorf("event[%d] type = %q, want %q", i, events[i].Type, want.Type)
		}
		if events[i].Active != want.Active {
			t.Errorf("event[%d] active = %v, want %v", i, events[i].Active, want.Active)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-110 && go test ./internal/nvr/onvif/ -run TestParseEvents_AllTypes -v`

Expected: PASS — parseEvents already handles any classified topic, so the new types should work with no additional changes.

- [ ] **Step 3: Commit**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-110
git add internal/nvr/onvif/events_test.go
git commit -m "test(events): add parseEvents test covering all event types"
```

---

### Task 3: Implement GetEventProperties SOAP Method

**Files:**
- Modify: `internal/nvr/onvif/events.go` (add GetEventProperties method and XML types)
- Modify: `internal/nvr/onvif/events_test.go` (add parsing test)

- [ ] **Step 1: Write test for parseEventProperties**

Append to `internal/nvr/onvif/events_test.go`:

```go
func TestParseEventProperties(t *testing.T) {
	// Simulated GetEventPropertiesResponse with a TopicSet containing
	// motion, tampering, digital input, and relay topics.
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <tev:GetEventPropertiesResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
      <tev:TopicNamespaceLocation>http://www.onvif.org/onvif/ver10/topics/topicns.xml</tev:TopicNamespaceLocation>
      <tev:TopicSet>
        <tns1:RuleEngine xmlns:tns1="http://www.onvif.org/ver10/topics">
          <CellMotionDetector>
            <Motion wstop:topic="true" xmlns:wstop="http://docs.oasis-open.org/wsn/t-1"/>
          </CellMotionDetector>
        </tns1:RuleEngine>
        <tns1:VideoSource xmlns:tns1="http://www.onvif.org/ver10/topics">
          <GlobalSceneChange>
            <ImagingService wstop:topic="true" xmlns:wstop="http://docs.oasis-open.org/wsn/t-1"/>
          </GlobalSceneChange>
          <SignalLoss wstop:topic="true" xmlns:wstop="http://docs.oasis-open.org/wsn/t-1"/>
        </tns1:VideoSource>
        <tns1:Device xmlns:tns1="http://www.onvif.org/ver10/topics">
          <Trigger>
            <DigitalInput wstop:topic="true" xmlns:wstop="http://docs.oasis-open.org/wsn/t-1"/>
            <Relay wstop:topic="true" xmlns:wstop="http://docs.oasis-open.org/wsn/t-1"/>
          </Trigger>
        </tns1:Device>
      </tev:TopicSet>
    </tev:GetEventPropertiesResponse>
  </s:Body>
</s:Envelope>`)

	topics, err := parseEventProperties(body)
	if err != nil {
		t.Fatalf("parseEventProperties error: %v", err)
	}

	// Should find: motion, tampering (globalscenechange), signal_loss, digital_input, relay
	want := map[DetectedEventType]bool{
		EventMotion:       true,
		EventTampering:    true,
		EventSignalLoss:   true,
		EventDigitalInput: true,
		EventRelay:        true,
	}
	got := make(map[DetectedEventType]bool)
	for _, t := range topics {
		got[t] = true
	}
	for wantType := range want {
		if !got[wantType] {
			t.Errorf("missing expected topic: %s", wantType)
		}
	}
	// Should not have duplicates.
	if len(topics) != len(got) {
		t.Errorf("got %d topics but %d unique — duplicates present", len(topics), len(got))
	}
}

func TestParseEventProperties_Empty(t *testing.T) {
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <tev:GetEventPropertiesResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
      <tev:TopicNamespaceLocation>http://www.onvif.org/onvif/ver10/topics/topicns.xml</tev:TopicNamespaceLocation>
      <tev:TopicSet/>
    </tev:GetEventPropertiesResponse>
  </s:Body>
</s:Envelope>`)

	topics, err := parseEventProperties(body)
	if err != nil {
		t.Fatalf("parseEventProperties error: %v", err)
	}
	if len(topics) != 0 {
		t.Errorf("expected 0 topics, got %d", len(topics))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-110 && go test ./internal/nvr/onvif/ -run TestParseEventProperties -v`

Expected: FAIL — `parseEventProperties` is undefined.

- [ ] **Step 3: Implement GetEventProperties and parseEventProperties**

Add to the end of `internal/nvr/onvif/events.go` (before the `truncate` function):

```go
// GetEventProperties sends the GetEventProperties SOAP request and returns
// the list of event types the camera supports, as determined by its TopicSet.
func (es *EventSubscriber) GetEventProperties(ctx context.Context) ([]DetectedEventType, error) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
  <s:Header></s:Header>
  <s:Body>
    <tev:GetEventProperties/>
  </s:Body>
</s:Envelope>`

	respBody, err := es.doSOAP(ctx, es.eventServiceURL, body)
	if err != nil {
		return nil, fmt.Errorf("GetEventProperties: %w", err)
	}

	return parseEventProperties(respBody)
}

// GetEventPropertiesFromURL sends GetEventProperties to an arbitrary event
// service URL with the given credentials. This is used during probe when no
// EventSubscriber exists yet.
func GetEventPropertiesFromURL(ctx context.Context, eventServiceURL, username, password string) ([]DetectedEventType, error) {
	es := &EventSubscriber{
		eventServiceURL: eventServiceURL,
		username:        username,
		password:        password,
		client:          &http.Client{Timeout: 10 * time.Second},
	}
	return es.GetEventProperties(ctx)
}

// parseEventProperties extracts supported event topics from a
// GetEventPropertiesResponse. It walks the TopicSet XML tree, building
// slash-separated paths from element names, and classifies each path.
func parseEventProperties(body []byte) ([]DetectedEventType, error) {
	// The TopicSet is a nested XML tree where element names form the topic path.
	// We need to walk it generically since the structure varies per camera.
	// Strategy: find the TopicSet element, then recursively walk all children
	// building paths, and classify each leaf.

	type genericElement struct {
		XMLName  xml.Name
		Children []genericElement `xml:",any"`
	}

	type topicSetEnvelope struct {
		XMLName xml.Name `xml:"Envelope"`
		Body    struct {
			Response struct {
				TopicSet genericElement `xml:"TopicSet"`
			} `xml:"GetEventPropertiesResponse"`
		} `xml:"Body"`
	}

	var env topicSetEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse GetEventPropertiesResponse: %w", err)
	}

	seen := make(map[DetectedEventType]bool)
	var topics []DetectedEventType

	var walk func(el genericElement, path string)
	walk = func(el genericElement, path string) {
		current := path
		if el.XMLName.Local != "" && el.XMLName.Local != "TopicSet" {
			if current != "" {
				current += "/"
			}
			current += el.XMLName.Local
		}

		// Try to classify the current path.
		if current != "" {
			if evtType, ok := classifyTopic(current); ok && !seen[evtType] {
				seen[evtType] = true
				topics = append(topics, evtType)
			}
		}

		for _, child := range el.Children {
			walk(child, current)
		}
	}

	walk(env.Body.Response.TopicSet, "")
	return topics, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-110 && go test ./internal/nvr/onvif/ -run TestParseEventProperties -v`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-110
git add internal/nvr/onvif/events.go internal/nvr/onvif/events_test.go
git commit -m "feat(events): implement GetEventProperties SOAP call and topic parser"
```

---

### Task 4: Database Migration and Camera Model Update

**Files:**
- Modify: `internal/nvr/db/migrations.go` (add migration 31)
- Modify: `internal/nvr/db/cameras.go` (add SupportedEventTopics field, update all queries)

- [ ] **Step 1: Add migration 31**

In `internal/nvr/db/migrations.go`, append to the `migrations` slice (after the version 30 entry, before the closing `}`):

```go
	// Migration 31: Supported event topics from GetEventProperties (KAI-110).
	{
		version: 31,
		sql:     `ALTER TABLE cameras ADD COLUMN supported_event_topics TEXT DEFAULT '';`,
	},
```

- [ ] **Step 2: Add SupportedEventTopics field to Camera struct**

In `internal/nvr/db/cameras.go`, add after the `QuotaCriticalPercent` field (line 50):

```go
	SupportedEventTopics    string `json:"supported_event_topics"`
```

- [ ] **Step 3: Update CreateCamera query**

In `internal/nvr/db/cameras.go`, in `CreateCamera`, update the INSERT statement to include `supported_event_topics` in both the column list and VALUES. Add it after `quota_critical_percent`:

Column list addition: `supported_event_topics,`
VALUES addition: one more `?`
Args addition: `cam.SupportedEventTopics,`

The full INSERT column list becomes:
```
INSERT INTO cameras (id, name, onvif_endpoint, onvif_username, onvif_password,
    onvif_profile_token, rtsp_url, ptz_capable, mediamtx_path, status, tags,
    retention_days, event_retention_days, detection_retention_days,
    supports_ptz, supports_imaging, supports_events,
    supports_relay, supports_audio_backchannel, snapshot_uri,
    supports_media2, supports_analytics, supports_edge_recording,
    motion_timeout_seconds, sub_stream_url, ai_enabled, audio_transcode,
    storage_path, quota_bytes, quota_warning_percent, quota_critical_percent,
    supported_event_topics,
    created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
```

And add `cam.SupportedEventTopics,` to the args before `cam.CreatedAt`.

- [ ] **Step 4: Update GetCamera, GetCameraByPath, ListCameras SELECT queries**

Add `supported_event_topics` to the SELECT column list in all three functions, and add `&cam.SupportedEventTopics` to the corresponding Scan calls. Place it after `quota_critical_percent` in both SELECT and Scan.

For `GetCamera` (the SELECT at line 105), add `supported_event_topics` after `quota_warning_percent, quota_critical_percent` in the SELECT, and add `&cam.SupportedEventTopics,` after `&cam.QuotaCriticalPercent,` in the Scan.

Apply the same change to `GetCameraByPath` and `ListCameras`.

- [ ] **Step 5: Update UpdateCamera query**

In the UPDATE statement in `UpdateCamera`, add `supported_event_topics = ?,` to the SET clause (after `quota_critical_percent = ?,`), and add `cam.SupportedEventTopics,` to the args (after `cam.QuotaCriticalPercent,`).

- [ ] **Step 6: Verify compilation**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-110 && go build ./internal/nvr/...`

Expected: Build succeeds.

- [ ] **Step 7: Commit**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-110
git add internal/nvr/db/migrations.go internal/nvr/db/cameras.go
git commit -m "feat(db): add supported_event_topics column to cameras table"
```

---

### Task 5: Integrate GetEventProperties into ProbeDeviceFull

**Files:**
- Modify: `internal/nvr/onvif/device.go` (add SupportedEventTopics to ProbeResult, call GetEventProperties)
- Modify: `internal/nvr/api/cameras.go` (save supported_event_topics on refresh)

- [ ] **Step 1: Add SupportedEventTopics to ProbeResult**

In `internal/nvr/onvif/device.go`, add a field to `ProbeResult` (after `Capabilities`):

```go
type ProbeResult struct {
	Profiles             []MediaProfile    `json:"profiles"`
	SnapshotURI          string            `json:"snapshot_uri,omitempty"`
	Capabilities         Capabilities      `json:"capabilities"`
	SupportedEventTopics []DetectedEventType `json:"supported_event_topics,omitempty"`
}
```

- [ ] **Step 2: Call GetEventProperties in ProbeDeviceFull**

In `internal/nvr/onvif/device.go`, in `ProbeDeviceFull`, after the audio backchannel check (after line 118), add:

```go
	// Discover supported event topics via GetEventProperties.
	if result.Capabilities.Events {
		eventURL := client.ServiceURL("events")
		if eventURL == "" {
			eventURL = client.ServiceURL("event")
		}
		if eventURL != "" {
			topics, err := GetEventPropertiesFromURL(ctx, eventURL, username, password)
			if err != nil {
				log.Printf("onvif probe [%s]: GetEventProperties failed: %v", xaddr, err)
			} else {
				result.SupportedEventTopics = topics
			}
		}
	}
```

- [ ] **Step 3: Save supported_event_topics during RefreshCapabilities**

In `internal/nvr/api/cameras.go`, in `RefreshCapabilities` (around line 782, after setting `cam.SupportsEdgeRecording`), add:

```go
	// Store supported event topics as JSON array.
	if len(result.SupportedEventTopics) > 0 {
		topicStrings := make([]string, len(result.SupportedEventTopics))
		for i, t := range result.SupportedEventTopics {
			topicStrings[i] = `"` + string(t) + `"`
		}
		cam.SupportedEventTopics = "[" + strings.Join(topicStrings, ",") + "]"
	}
```

Make sure `strings` is imported in this file (it likely already is — verify).

- [ ] **Step 4: Return supported_event_topics from Probe endpoint**

In `internal/nvr/api/cameras.go`, in the `Probe` handler (around line 738), add `supported_event_topics` to the response:

```go
	c.JSON(http.StatusOK, gin.H{
		"profiles":               result.Profiles,
		"capabilities":           result.Capabilities,
		"snapshot_uri":           result.SnapshotURI,
		"supported_event_topics": result.SupportedEventTopics,
	})
```

- [ ] **Step 5: Verify compilation**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-110 && go build ./internal/nvr/...`

Expected: Build succeeds.

- [ ] **Step 6: Commit**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-110
git add internal/nvr/onvif/device.go internal/nvr/api/cameras.go
git commit -m "feat(probe): call GetEventProperties during probe and store supported topics"
```

---

### Task 6: Add SSE Publish Methods for New Event Types

**Files:**
- Modify: `internal/nvr/api/events.go` (add four publish methods)

- [ ] **Step 1: Add publish methods**

In `internal/nvr/api/events.go`, add after `PublishTampering` (after line 137):

```go
// PublishDigitalInput publishes a digital-input event for the given camera.
func (b *EventBroadcaster) PublishDigitalInput(cameraName string, active bool) {
	action := "triggered"
	if !active {
		action = "cleared"
	}
	b.Publish(Event{
		Type:    "digital_input",
		Camera:  cameraName,
		Message: fmt.Sprintf("Digital input %s on %s", action, cameraName),
	})
}

// PublishSignalLoss publishes a signal-loss event for the given camera.
func (b *EventBroadcaster) PublishSignalLoss(cameraName string, active bool) {
	action := "detected"
	if !active {
		action = "recovered"
	}
	b.Publish(Event{
		Type:    "signal_loss",
		Camera:  cameraName,
		Message: fmt.Sprintf("Signal loss %s on %s", action, cameraName),
	})
}

// PublishHardwareFailure publishes a hardware-failure event for the given camera.
func (b *EventBroadcaster) PublishHardwareFailure(cameraName string, active bool) {
	action := "detected"
	if !active {
		action = "cleared"
	}
	b.Publish(Event{
		Type:    "hardware_failure",
		Camera:  cameraName,
		Message: fmt.Sprintf("Hardware failure %s on %s", action, cameraName),
	})
}

// PublishRelay publishes a relay-output event for the given camera.
func (b *EventBroadcaster) PublishRelay(cameraName string, active bool) {
	action := "activated"
	if !active {
		action = "deactivated"
	}
	b.Publish(Event{
		Type:    "relay",
		Camera:  cameraName,
		Message: fmt.Sprintf("Relay %s on %s", action, cameraName),
	})
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-110 && go build ./internal/nvr/...`

Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-110
git add internal/nvr/api/events.go
git commit -m "feat(events): add SSE publish methods for new event types"
```

---

### Task 7: Handle New Event Types in Scheduler Callbacks

**Files:**
- Modify: `internal/nvr/scheduler/scheduler.go` (extend both event callbacks)

- [ ] **Step 1: Extend startEventPipelineLocked callback**

In `internal/nvr/scheduler/scheduler.go`, in `startEventPipelineLocked`, after the `case onvif.EventTampering:` block (line 1043), add cases for the new event types. The full switch in the `eventCallback` should become:

```go
		case onvif.EventDigitalInput, onvif.EventSignalLoss, onvif.EventHardwareFailure, onvif.EventRelay:
			now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			if active {
				_ = s.db.InsertMotionEvent(&db.MotionEvent{
					CameraID:  camID,
					StartedAt: now,
					EventType: string(eventType),
				})
				if s.eventPub != nil {
					switch eventType {
					case onvif.EventDigitalInput:
						s.eventPub.PublishDigitalInput(cam.Name, true)
					case onvif.EventSignalLoss:
						s.eventPub.PublishSignalLoss(cam.Name, true)
					case onvif.EventHardwareFailure:
						s.eventPub.PublishHardwareFailure(cam.Name, true)
					case onvif.EventRelay:
						s.eventPub.PublishRelay(cam.Name, true)
					}
				}
			} else {
				_ = s.db.EndMotionEvent(camID, now)
				if s.eventPub != nil {
					switch eventType {
					case onvif.EventDigitalInput:
						s.eventPub.PublishDigitalInput(cam.Name, false)
					case onvif.EventSignalLoss:
						s.eventPub.PublishSignalLoss(cam.Name, false)
					case onvif.EventHardwareFailure:
						s.eventPub.PublishHardwareFailure(cam.Name, false)
					case onvif.EventRelay:
						s.eventPub.PublishRelay(cam.Name, false)
					}
				}
			}
```

- [ ] **Step 2: Extend startMotionAlertSubscription callback**

In `internal/nvr/scheduler/scheduler.go`, in `startMotionAlertSubscription`, after the `case onvif.EventTampering:` block (line 1160), add the same case block as in Step 1, but using `cam.ID` and `cam.Name` instead of `camID` and `cam.Name` (they're the same in this function):

```go
		case onvif.EventDigitalInput, onvif.EventSignalLoss, onvif.EventHardwareFailure, onvif.EventRelay:
			now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			if active {
				_ = s.db.InsertMotionEvent(&db.MotionEvent{
					CameraID:  cam.ID,
					StartedAt: now,
					EventType: string(eventType),
				})
				if s.eventPub != nil {
					switch eventType {
					case onvif.EventDigitalInput:
						s.eventPub.PublishDigitalInput(cam.Name, true)
					case onvif.EventSignalLoss:
						s.eventPub.PublishSignalLoss(cam.Name, true)
					case onvif.EventHardwareFailure:
						s.eventPub.PublishHardwareFailure(cam.Name, true)
					case onvif.EventRelay:
						s.eventPub.PublishRelay(cam.Name, true)
					}
				}
			} else {
				_ = s.db.EndMotionEvent(cam.ID, now)
				if s.eventPub != nil {
					switch eventType {
					case onvif.EventDigitalInput:
						s.eventPub.PublishDigitalInput(cam.Name, false)
					case onvif.EventSignalLoss:
						s.eventPub.PublishSignalLoss(cam.Name, false)
					case onvif.EventHardwareFailure:
						s.eventPub.PublishHardwareFailure(cam.Name, false)
					case onvif.EventRelay:
						s.eventPub.PublishRelay(cam.Name, false)
					}
				}
			}
```

- [ ] **Step 3: Verify compilation**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-110 && go build ./internal/nvr/...`

Expected: Build succeeds.

- [ ] **Step 4: Run all tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-110 && go test ./internal/nvr/onvif/ -v`

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-110
git add internal/nvr/scheduler/scheduler.go
git commit -m "feat(scheduler): handle DigitalInput, SignalLoss, HardwareFailure, Relay events"
```

---

### Task 8: Push and Create PR

- [ ] **Step 1: Run full build**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-110 && go build ./...`

Expected: Build succeeds.

- [ ] **Step 2: Push branch**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-110
git push -u origin feat/kai-110-event-service-completion
```

- [ ] **Step 3: Create PR**

```bash
gh pr create --title "feat: complete event service with GetEventProperties and new topics" --body "$(cat <<'EOF'
## Summary
- Implement GetEventProperties SOAP call (mandatory for Profile S and T) to discover camera-supported event topics during probe
- Add four new event topics: DigitalInput, SignalLoss, HardwareFailure, Relay
- Store supported topics as JSON in new `supported_event_topics` camera column
- Handle all new events in scheduler (DB insert/end + SSE broadcast)

## Test plan
- [ ] Run `go test ./internal/nvr/onvif/ -v` — classifyTopic and parseEventProperties tests pass
- [ ] Run `go build ./...` — full build succeeds
- [ ] Probe a camera with ONVIF events and verify supported_event_topics is populated
- [ ] Verify new event types appear in SSE stream when triggered

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```
