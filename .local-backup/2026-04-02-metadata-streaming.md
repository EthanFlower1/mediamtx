# KAI-18: ONVIF Profile T Metadata Streaming — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable metadata configuration management on ONVIF cameras and subscribe to metadata streams via RTSP/RTP, routing analytics detections into the existing event processing pipeline.

**Architecture:** Metadata configuration CRUD wraps the onvif-go library methods (same pattern as `media_config.go`). Metadata stream subscription uses gortsplib as an RTSP client to connect to the camera's metadata stream URI, receives RTP packets containing XML analytics data, parses them with the existing `ParseMetadataFrame`, and feeds detections into both the `EventCallback` pipeline (motion/tampering events) and the AI pipeline's `ONVIFSrc`. API endpoints follow the established camera handler pattern.

**Tech Stack:** Go, gortsplib/v5 (already in go.mod), onvif-go, Gin

---

## File Structure

### New files

| File | Responsibility |
|------|---------------|
| `internal/nvr/onvif/metadata_config.go` | Metadata configuration CRUD: Get/Set/Add/Remove via onvif-go library |
| `internal/nvr/onvif/metadata_stream.go` | RTSP-based metadata stream subscriber using gortsplib |
| `internal/nvr/onvif/metadata_config_test.go` | Unit tests for metadata config wrappers |
| `internal/nvr/onvif/metadata_stream_test.go` | Unit tests for metadata stream subscriber |

### Modified files

| File | Change |
|------|--------|
| `internal/nvr/onvif/metadata.go` | Add `EventMetadata` type for scene-change/motion events extracted from metadata frames |
| `internal/nvr/api/cameras.go` | Add metadata configuration handler methods |
| `internal/nvr/api/router.go` | Register metadata configuration routes |
| `internal/nvr/scheduler/scheduler.go` | Start/stop metadata stream subscribers alongside event subscribers |

---

### Task 1: Metadata Configuration CRUD

**Files:**
- Create: `internal/nvr/onvif/metadata_config.go`
- Create: `internal/nvr/onvif/metadata_config_test.go`

- [ ] **Step 1: Write failing tests for GetMetadataConfigurations and GetMetadataConfiguration**

Create `internal/nvr/onvif/metadata_config_test.go`:

```go
package onvif

import (
	"testing"
)

func TestMetadataConfigInfoFields(t *testing.T) {
	cfg := MetadataConfigInfo{
		Token:     "MetadataToken1",
		Name:      "Metadata Config",
		UseCount:  2,
		Analytics: true,
	}
	if cfg.Token != "MetadataToken1" {
		t.Errorf("expected token MetadataToken1, got %s", cfg.Token)
	}
	if !cfg.Analytics {
		t.Error("expected Analytics to be true")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/nvr/onvif/ -run TestMetadataConfigInfoFields -v`
Expected: FAIL — `MetadataConfigInfo` undefined

- [ ] **Step 3: Implement metadata_config.go with types and CRUD functions**

Create `internal/nvr/onvif/metadata_config.go`:

```go
package onvif

import (
	"context"
	"fmt"

	onvifgo "github.com/EthanFlower1/onvif-go"
)

// MetadataConfigInfo holds metadata configuration details from the device.
type MetadataConfigInfo struct {
	Token          string `json:"token"`
	Name           string `json:"name"`
	UseCount       int    `json:"use_count"`
	Analytics      bool   `json:"analytics"`
	SessionTimeout string `json:"session_timeout,omitempty"`
}

// GetMetadataConfigurations returns all metadata configurations from the device.
func GetMetadataConfigurations(xaddr, username, password string) ([]*MetadataConfigInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	configs, err := client.Dev.GetMetadataConfigurations(ctx)
	if err != nil {
		return nil, fmt.Errorf("get metadata configurations: %w", err)
	}

	result := make([]*MetadataConfigInfo, len(configs))
	for i, cfg := range configs {
		result[i] = convertMetadataConfig(cfg)
	}
	return result, nil
}

// GetMetadataConfiguration returns a single metadata configuration by token.
func GetMetadataConfiguration(xaddr, username, password, configToken string) (*MetadataConfigInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	cfg, err := client.Dev.GetMetadataConfiguration(ctx, configToken)
	if err != nil {
		return nil, fmt.Errorf("get metadata configuration: %w", err)
	}

	return convertMetadataConfig(cfg), nil
}

// SetMetadataConfiguration updates a metadata configuration on the device.
func SetMetadataConfiguration(xaddr, username, password string, cfg *MetadataConfigInfo) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	mc := &onvifgo.MetadataConfiguration{
		Token:     cfg.Token,
		Name:      cfg.Name,
		UseCount:  cfg.UseCount,
		Analytics: cfg.Analytics,
	}

	ctx := context.Background()
	if err := client.Dev.SetMetadataConfiguration(ctx, mc, true); err != nil {
		return fmt.Errorf("set metadata configuration: %w", err)
	}
	return nil
}

// AddMetadataToProfile adds a metadata configuration to a media profile.
func AddMetadataToProfile(xaddr, username, password, profileToken, configToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.AddMetadataConfiguration(ctx, profileToken, configToken); err != nil {
		return fmt.Errorf("add metadata to profile: %w", err)
	}
	return nil
}

// RemoveMetadataFromProfile removes the metadata configuration from a media profile.
func RemoveMetadataFromProfile(xaddr, username, password, profileToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.RemoveMetadataConfiguration(ctx, profileToken); err != nil {
		return fmt.Errorf("remove metadata from profile: %w", err)
	}
	return nil
}

func convertMetadataConfig(cfg *onvifgo.MetadataConfiguration) *MetadataConfigInfo {
	info := &MetadataConfigInfo{
		Token:     cfg.Token,
		Name:      cfg.Name,
		UseCount:  cfg.UseCount,
		Analytics: cfg.Analytics,
	}
	if cfg.SessionTimeout > 0 {
		info.SessionTimeout = cfg.SessionTimeout.String()
	}
	return info
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/nvr/onvif/ -run TestMetadataConfigInfoFields -v`
Expected: PASS

- [ ] **Step 5: Build to verify compilation**

Run: `go build ./internal/nvr/onvif/...`
Expected: clean build

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/onvif/metadata_config.go internal/nvr/onvif/metadata_config_test.go
git commit -m "feat(nvr): add ONVIF metadata configuration CRUD wrappers"
```

---

### Task 2: Metadata Stream Parsing Enhancements

**Files:**
- Modify: `internal/nvr/onvif/metadata.go`

- [ ] **Step 1: Add EventMetadata type and classifyMetadataEvents function**

The existing `ParseMetadataFrame` handles object detections. Add support for extracting motion/tampering events from metadata frames, which appear as `<tt:Event>` elements in the metadata stream alongside `<tt:VideoAnalytics>`.

Edit `internal/nvr/onvif/metadata.go` to add after the existing types:

```go
// EventMetadata represents a motion or scene-change event extracted from
// an ONVIF metadata stream frame.
type EventMetadata struct {
	Topic  string // e.g. "Motion", "GlobalSceneChange"
	Source string // source token (video source)
	Active bool
}

// MetadataStreamFull parses both analytics frames and event notifications
// from a metadata stream XML chunk.
type MetadataStreamFull struct {
	XMLName xml.Name        `xml:"MetadataStream"`
	Frames  []MetadataFrame `xml:"VideoAnalytics>Frame"`
	Events  []metadataEvent `xml:"Event>NotificationMessage"`
}

type metadataEvent struct {
	Topic   string          `xml:"Topic"`
	Message metadataMessage `xml:"Message>Message"`
}

type metadataMessage struct {
	Source metadataItemSet `xml:"Source"`
	Data   metadataItemSet `xml:"Data"`
}

type metadataItemSet struct {
	SimpleItems []metadataSimpleItem `xml:"SimpleItem"`
}

type metadataSimpleItem struct {
	Name  string `xml:"Name,attr"`
	Value string `xml:"Value,attr"`
}

// ParseMetadataStreamFull parses a metadata stream XML chunk and returns
// both analytics frames and event notifications.
func ParseMetadataStreamFull(data []byte) (*MetadataFrame, []EventMetadata, error) {
	var stream MetadataStreamFull
	if err := xml.Unmarshal(data, &stream); err != nil {
		// Fall back to single-frame parsing.
		frame, err := ParseMetadataFrame(data)
		return frame, nil, err
	}

	var frame *MetadataFrame
	if len(stream.Frames) > 0 {
		frame = &stream.Frames[0]
	}

	var events []EventMetadata
	for _, evt := range stream.Events {
		em := EventMetadata{Topic: evt.Topic}
		for _, item := range evt.Message.Source.SimpleItems {
			if item.Name == "VideoSourceConfigurationToken" || item.Name == "Source" {
				em.Source = item.Value
			}
		}
		for _, item := range evt.Message.Data.SimpleItems {
			lower := strings.ToLower(item.Name)
			if lower == "ismotion" || lower == "state" {
				em.Active = strings.ToLower(item.Value) == "true" || item.Value == "1"
			}
		}
		events = append(events, em)
	}

	return frame, events, nil
}
```

Add `"strings"` to the imports.

- [ ] **Step 2: Build to verify compilation**

Run: `go build ./internal/nvr/onvif/...`
Expected: clean build

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/onvif/metadata.go
git commit -m "feat(nvr): add full metadata stream parsing with event extraction"
```

---

### Task 3: RTSP Metadata Stream Subscriber

**Files:**
- Create: `internal/nvr/onvif/metadata_stream.go`
- Create: `internal/nvr/onvif/metadata_stream_test.go`

- [ ] **Step 1: Write failing test for MetadataStreamSubscriber**

Create `internal/nvr/onvif/metadata_stream_test.go`:

```go
package onvif

import (
	"testing"
)

func TestMetadataStreamSubscriberRequiresStreamURI(t *testing.T) {
	_, err := NewMetadataStreamSubscriber("", nil, nil)
	if err == nil {
		t.Fatal("expected error for empty stream URI")
	}
}

func TestMetadataStreamSubscriberRequiresCallback(t *testing.T) {
	_, err := NewMetadataStreamSubscriber("rtsp://camera/metadata", nil, nil)
	if err == nil {
		t.Fatal("expected error for nil callbacks")
	}
}

func TestMetadataStreamSubscriberCreation(t *testing.T) {
	eventCb := func(eventType DetectedEventType, active bool) {}
	frameCb := func(frame *MetadataFrame) {}
	sub, err := NewMetadataStreamSubscriber("rtsp://camera/metadata", eventCb, frameCb)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sub == nil {
		t.Fatal("expected non-nil subscriber")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/nvr/onvif/ -run TestMetadataStream -v`
Expected: FAIL — `NewMetadataStreamSubscriber` undefined

- [ ] **Step 3: Implement MetadataStreamSubscriber**

Create `internal/nvr/onvif/metadata_stream.go`:

```go
package onvif

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"
)

// MetadataFrameCallback is invoked when a metadata analytics frame is received.
type MetadataFrameCallback func(frame *MetadataFrame)

// MetadataStreamSubscriber connects to an ONVIF camera's RTSP metadata
// stream and routes parsed analytics data into event and detection callbacks.
type MetadataStreamSubscriber struct {
	streamURI string
	eventCb   EventCallback
	frameCb   MetadataFrameCallback
	cancel    context.CancelFunc
}

// NewMetadataStreamSubscriber creates a subscriber for the given RTSP metadata
// stream URI. eventCb receives motion/tampering events. frameCb receives
// analytics object detection frames (may be nil if not needed).
func NewMetadataStreamSubscriber(
	streamURI string,
	eventCb EventCallback,
	frameCb MetadataFrameCallback,
) (*MetadataStreamSubscriber, error) {
	if streamURI == "" {
		return nil, fmt.Errorf("metadata stream: stream URI is required")
	}
	if eventCb == nil && frameCb == nil {
		return nil, fmt.Errorf("metadata stream: at least one callback is required")
	}

	return &MetadataStreamSubscriber{
		streamURI: streamURI,
		eventCb:   eventCb,
		frameCb:   frameCb,
	}, nil
}

// Start connects to the RTSP metadata stream and blocks until ctx is cancelled.
// It reconnects with exponential backoff on errors.
func (ms *MetadataStreamSubscriber) Start(ctx context.Context) {
	ctx, ms.cancel = context.WithCancel(ctx)

	backoff := 5 * time.Second
	maxBackoff := 5 * time.Minute

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := ms.connectAndRead(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("metadata stream [%s]: %v, retrying in %v", ms.streamURI, err, backoff)
		} else {
			backoff = 5 * time.Second
		}

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}

		backoff = backoff * 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// Stop cancels the metadata stream subscription.
func (ms *MetadataStreamSubscriber) Stop() {
	if ms.cancel != nil {
		ms.cancel()
	}
}

// connectAndRead performs a single RTSP session: DESCRIBE, find metadata
// track, SETUP, PLAY, and read RTP packets until error or cancellation.
func (ms *MetadataStreamSubscriber) connectAndRead(ctx context.Context) error {
	u, err := base.ParseURL(ms.streamURI)
	if err != nil {
		return fmt.Errorf("parse stream URI: %w", err)
	}

	c := &gortsplib.Client{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	err = c.Start(u.Scheme, u.Host)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer c.Close()

	desc, _, err := c.Describe(u)
	if err != nil {
		return fmt.Errorf("describe: %w", err)
	}

	// Find the metadata media track. ONVIF cameras expose metadata as
	// application/metadata or in a track with type "application".
	var metadataMedia *description.Media
	for _, m := range desc.Medias {
		if m.IsBackChannel {
			continue
		}
		mediaType := strings.ToLower(string(m.Type))
		if mediaType == "application" || mediaType == "metadata" {
			metadataMedia = m
			break
		}
		// Some cameras use a generic format with "application" in the codec.
		for _, f := range m.Formats {
			if gen, ok := f.(*format.Generic); ok {
				clockRate := gen.ClockRate()
				// Metadata streams typically use 90000Hz clock rate.
				if clockRate == 90000 || clockRate == 0 {
					codec := strings.ToLower(gen.Codec())
					if strings.Contains(codec, "metadata") || strings.Contains(codec, "application") || strings.Contains(codec, "vnd.onvif") {
						metadataMedia = m
						break
					}
				}
			}
		}
		if metadataMedia != nil {
			break
		}
	}

	if metadataMedia == nil {
		return fmt.Errorf("no metadata track found in RTSP session description")
	}

	_, err = c.Setup(desc.BaseURL, metadataMedia, 0, 0)
	if err != nil {
		return fmt.Errorf("setup metadata track: %w", err)
	}

	// Buffer for accumulating XML fragments across RTP packets.
	var xmlBuf []byte

	c.OnPacketRTPAny(func(medi *description.Media, forma format.Format, pkt *rtp.Packet) {
		if medi != metadataMedia {
			return
		}

		// Append payload to buffer. Metadata XML may span multiple RTP packets.
		xmlBuf = append(xmlBuf, pkt.Payload...)

		// Try to parse a complete XML document. If it fails, keep accumulating.
		// Limit buffer size to prevent unbounded growth.
		if len(xmlBuf) > 256*1024 {
			xmlBuf = xmlBuf[len(xmlBuf)-128*1024:]
		}

		frame, events, parseErr := ParseMetadataStreamFull(xmlBuf)
		if parseErr != nil {
			return // incomplete XML, keep accumulating
		}

		// Successful parse — reset buffer.
		xmlBuf = nil

		// Route analytics frame detections.
		if frame != nil && ms.frameCb != nil {
			ms.frameCb(frame)
		}

		// Route events to the event callback.
		if ms.eventCb != nil {
			for _, evt := range events {
				evtType, ok := classifyMetadataEventTopic(evt.Topic)
				if ok {
					ms.eventCb(evtType, evt.Active)
				}
			}
		}
	})

	_, err = c.Play(nil)
	if err != nil {
		return fmt.Errorf("play: %w", err)
	}

	// Block until context is cancelled or connection drops.
	select {
	case <-ctx.Done():
		return nil
	case err := <-c.Wait():
		return err
	}
}

// classifyMetadataEventTopic maps a metadata stream event topic to a
// DetectedEventType. Returns ("", false) if unrecognized.
func classifyMetadataEventTopic(topic string) (DetectedEventType, bool) {
	lower := strings.ToLower(topic)
	if strings.Contains(lower, "motion") || strings.Contains(lower, "cellmotion") {
		return EventMotion, true
	}
	if strings.Contains(lower, "globalscenechange") || strings.Contains(lower, "tamper") {
		return EventTampering, true
	}
	return "", false
}

// GetMetadataStreamURI retrieves the RTSP stream URI for a profile that has
// metadata configured. It tries Media2 first, then falls back to Media1.
func GetMetadataStreamURI(xaddr, username, password, profileToken string) (string, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return "", err
	}

	// Try Media2 first.
	if client.HasService("media2") {
		uri, err := GetStreamUri2(client, profileToken)
		if err == nil && uri != "" {
			return uri, nil
		}
	}

	// Fall back to Media1.
	ctx := context.Background()
	resp, err := client.Dev.GetStreamURI(ctx, profileToken)
	if err != nil {
		return "", fmt.Errorf("get stream URI: %w", err)
	}
	if resp == nil || resp.URI == "" {
		return "", fmt.Errorf("empty stream URI for profile %s", profileToken)
	}
	return resp.URI, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/nvr/onvif/ -run TestMetadataStream -v`
Expected: PASS

- [ ] **Step 5: Build to verify compilation**

Run: `go build ./internal/nvr/onvif/...`
Expected: clean build

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/onvif/metadata_stream.go internal/nvr/onvif/metadata_stream_test.go
git commit -m "feat(nvr): add RTSP-based metadata stream subscriber using gortsplib"
```

---

### Task 4: API Endpoints for Metadata Configuration

**Files:**
- Modify: `internal/nvr/api/cameras.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Add metadata configuration handlers to cameras.go**

Append to `internal/nvr/api/cameras.go`:

```go
// --- Metadata Configuration API ---

// GetMetadataConfigurations returns all metadata configurations for a camera.
//
//	GET /cameras/:id/metadata/configurations
func (h *CameraHandler) GetMetadataConfigurations(c *gin.Context) {
	id := c.Param("id")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for metadata configurations", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	configs, err := onvif.GetMetadataConfigurations(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword))
	if err != nil {
		nvrLogError("metadata", fmt.Sprintf("failed to get metadata configurations for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get metadata configurations from device"})
		return
	}

	if configs == nil {
		configs = []*onvif.MetadataConfigInfo{}
	}

	c.JSON(http.StatusOK, gin.H{"configurations": configs})
}

// GetMetadataConfiguration returns a single metadata configuration by token.
//
//	GET /cameras/:id/metadata/configurations/:token
func (h *CameraHandler) GetMetadataConfiguration(c *gin.Context) {
	id := c.Param("id")
	configToken := c.Param("token")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for metadata configuration", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	cfg, err := onvif.GetMetadataConfiguration(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), configToken)
	if err != nil {
		nvrLogError("metadata", fmt.Sprintf("failed to get metadata configuration %s for camera %s", configToken, id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get metadata configuration from device"})
		return
	}

	c.JSON(http.StatusOK, cfg)
}

// SetMetadataConfiguration updates a metadata configuration on the device.
//
//	PUT /cameras/:id/metadata/configurations/:token
func (h *CameraHandler) SetMetadataConfiguration(c *gin.Context) {
	id := c.Param("id")
	configToken := c.Param("token")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for metadata configuration update", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	var cfg onvif.MetadataConfigInfo
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cfg.Token = configToken

	if err := onvif.SetMetadataConfiguration(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), &cfg); err != nil {
		nvrLogError("metadata", fmt.Sprintf("failed to set metadata configuration %s for camera %s", configToken, id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to update metadata configuration on device"})
		return
	}

	nvrLogInfo("metadata", fmt.Sprintf("Updated metadata configuration %s on camera %s", configToken, id))
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// AddMetadataToProfile adds a metadata configuration to a profile.
//
//	POST /cameras/:id/metadata/profile
func (h *CameraHandler) AddMetadataToProfile(c *gin.Context) {
	id := c.Param("id")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	var req struct {
		ProfileToken string `json:"profile_token" binding:"required"`
		ConfigToken  string `json:"config_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "profile_token and config_token are required"})
		return
	}

	if err := onvif.AddMetadataToProfile(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), req.ProfileToken, req.ConfigToken); err != nil {
		nvrLogError("metadata", fmt.Sprintf("failed to add metadata to profile for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to add metadata configuration to profile"})
		return
	}

	nvrLogInfo("metadata", fmt.Sprintf("Added metadata config %s to profile %s on camera %s", req.ConfigToken, req.ProfileToken, id))
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// RemoveMetadataFromProfile removes the metadata configuration from a profile.
//
//	DELETE /cameras/:id/metadata/profile/:profileToken
func (h *CameraHandler) RemoveMetadataFromProfile(c *gin.Context) {
	id := c.Param("id")
	profileToken := c.Param("profileToken")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	if err := onvif.RemoveMetadataFromProfile(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), profileToken); err != nil {
		nvrLogError("metadata", fmt.Sprintf("failed to remove metadata from profile %s for camera %s", profileToken, id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to remove metadata configuration from profile"})
		return
	}

	nvrLogInfo("metadata", fmt.Sprintf("Removed metadata config from profile %s on camera %s", profileToken, id))
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
```

- [ ] **Step 2: Register metadata routes in router.go**

In `internal/nvr/api/router.go`, add after the analytics routes block (after line 280):

```go
	// Metadata configuration (Profile T).
	protected.GET("/cameras/:id/metadata/configurations", cameraHandler.GetMetadataConfigurations)
	protected.GET("/cameras/:id/metadata/configurations/:token", cameraHandler.GetMetadataConfiguration)
	protected.PUT("/cameras/:id/metadata/configurations/:token", cameraHandler.SetMetadataConfiguration)
	protected.POST("/cameras/:id/metadata/profile", cameraHandler.AddMetadataToProfile)
	protected.DELETE("/cameras/:id/metadata/profile/:profileToken", cameraHandler.RemoveMetadataFromProfile)
```

- [ ] **Step 3: Build to verify compilation**

Run: `go build ./internal/nvr/...`
Expected: clean build

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/cameras.go internal/nvr/api/router.go
git commit -m "feat(nvr): add metadata configuration REST API endpoints"
```

---

### Task 5: Integrate Metadata Stream into Scheduler

**Files:**
- Modify: `internal/nvr/scheduler/scheduler.go`

- [ ] **Step 1: Add metadataSubs map to Scheduler struct**

In `scheduler.go`, add a `metadataSubs` field to the Scheduler struct alongside `eventSubs`:

```go
metadataSubs map[string]*onvif.MetadataStreamSubscriber // camera ID -> metadata subscriber
```

Initialize it in the constructor alongside `eventSubs`:

```go
metadataSubs: make(map[string]*onvif.MetadataStreamSubscriber),
```

Add cleanup in `Close()` alongside event subscriber cleanup:

```go
for _, sub := range s.metadataSubs {
	sub.Stop()
}
```

- [ ] **Step 2: Add startMetadataStreamLocked and stopMetadataStreamLocked**

Add at the end of the file, after `stopEventPipelineLocked`:

```go
// startMetadataStreamLocked creates and starts a MetadataStreamSubscriber
// for the given camera. It looks up the metadata stream URI from the camera's
// ONVIF profile and connects via RTSP. Must be called with s.mu held.
func (s *Scheduler) startMetadataStreamLocked(camID string, cam *db.Camera) {
	if cam.ONVIFEndpoint == "" || cam.ONVIFProfileToken == "" {
		return
	}

	// Check if already running.
	if _, ok := s.metadataSubs[camID]; ok {
		return
	}

	// Get the metadata stream URI for the camera's profile.
	streamURI, err := onvif.GetMetadataStreamURI(
		cam.ONVIFEndpoint,
		cam.ONVIFUsername,
		s.decryptPassword(cam.ONVIFPassword),
		cam.ONVIFProfileToken,
	)
	if err != nil {
		log.Printf("scheduler: metadata stream URI for camera %s: %v (metadata streaming disabled)", camID, err)
		return
	}

	// Reuse the existing event callback from the event subscriber.
	eventCb := func(eventType onvif.DetectedEventType, active bool) {
		s.mu.Lock()
		// Only process if the event subscriber's callback would process this.
		// Dispatch to MotionSMs for motion events.
		if eventType == onvif.EventMotion {
			for sk, msm := range s.motionSMs {
				if sk == camID || strings.HasPrefix(sk, camID+":") {
					msm.OnMotion(active)
				}
			}
		}
		s.mu.Unlock()

		switch eventType {
		case onvif.EventMotion:
			now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			if active {
				s.StartMotionTimer(camID, cam.MotionTimeoutSeconds)
				if !s.db.HasOpenMotionEvent(camID) {
					_ = s.db.InsertMotionEvent(&db.MotionEvent{
						CameraID:  camID,
						StartedAt: now,
					})
					if s.eventPub != nil {
						s.eventPub.PublishMotion(cam.Name)
					}
				}
			} else {
				s.CancelMotionTimer(camID)
				_ = s.db.EndMotionEvent(camID, now)
			}
		case onvif.EventTampering:
			now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			if active {
				_ = s.db.InsertMotionEvent(&db.MotionEvent{
					CameraID:  camID,
					StartedAt: now,
					EventType: "tampering",
				})
				if s.eventPub != nil {
					s.eventPub.PublishTampering(cam.Name)
				}
			} else {
				_ = s.db.EndMotionEvent(camID, now)
			}
		}
	}

	// Frame callback logs analytics detections at debug level.
	frameCb := func(frame *onvif.MetadataFrame) {
		if len(frame.Objects) > 0 {
			log.Printf("scheduler: metadata stream [%s]: %d analytics objects at %s",
				cam.Name, len(frame.Objects), frame.UtcTime)
		}
	}

	sub, err := onvif.NewMetadataStreamSubscriber(streamURI, eventCb, frameCb)
	if err != nil {
		log.Printf("scheduler: create metadata subscriber for camera %s: %v", camID, err)
		return
	}

	s.metadataSubs[camID] = sub
	go sub.Start(context.Background())
	log.Printf("scheduler: metadata stream started for camera %s at %s", camID, streamURI)
}

// stopMetadataStreamLocked stops and removes the MetadataStreamSubscriber
// for the given camera. Must be called with s.mu held.
func (s *Scheduler) stopMetadataStreamLocked(camID string) {
	if sub, ok := s.metadataSubs[camID]; ok {
		sub.Stop()
		delete(s.metadataSubs, camID)
		log.Printf("scheduler: metadata stream stopped for camera %s", camID)
	}
}
```

- [ ] **Step 3: Call startMetadataStreamLocked from startEventPipelineLocked**

In `startEventPipelineLocked`, after the event subscriber is successfully started and registered (after `s.eventSubs[camID] = sub`), add:

```go
	// Start metadata stream subscriber if the camera's profile supports it.
	s.startMetadataStreamLocked(camID, cam)
```

- [ ] **Step 4: Call stopMetadataStreamLocked from stopEventPipelineLocked**

In `stopEventPipelineLocked`, alongside the event subscriber Stop call, add:

```go
	s.stopMetadataStreamLocked(camID)
```

- [ ] **Step 5: Build to verify compilation**

Run: `go build ./internal/nvr/...`
Expected: clean build

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/scheduler/scheduler.go
git commit -m "feat(nvr): integrate metadata stream subscriber into scheduler event pipeline"
```

---

### Task 6: Run Tests and Final Verification

**Files:**
- Various

- [ ] **Step 1: Run all ONVIF package tests**

Run: `go test ./internal/nvr/onvif/ -v -count=1`
Expected: all tests pass

- [ ] **Step 2: Run full NVR test suite**

Run: `go test ./internal/nvr/... -count=1 -timeout=120s`
Expected: all tests pass

- [ ] **Step 3: Build entire project**

Run: `go build ./...`
Expected: clean build (ignore test-only build errors if any)

- [ ] **Step 4: Verify no import cycles or unused imports**

Run: `go vet ./internal/nvr/...`
Expected: clean

- [ ] **Step 5: Final commit if any fixups needed**

```bash
git add -A
git commit -m "chore: fix any test/build issues from metadata streaming integration"
```

(Skip if no changes needed.)
