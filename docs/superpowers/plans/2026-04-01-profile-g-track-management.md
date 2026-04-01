# KAI-11: ONVIF Profile G Track Management — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add CreateTrack, DeleteTrack, and GetTrackConfiguration ONVIF operations to manage multi-track recordings per Profile G.

**Architecture:** Extends the existing `recording_control.go` ONVIF client and `api/recording_control.go` API handlers from KAI-10 with 3 new SOAP operations and 3 new REST endpoints. All code follows the identical patterns already established — same SOAP helpers, same envelope type, same API handler structure.

**Tech Stack:** Go (raw SOAP via `trc:` namespace), Gin HTTP framework

**Spec:** `docs/superpowers/specs/2026-04-01-onvif-profile-g-track-management-design.md`

---

## File Structure

### Modified files
| File | Responsibility |
|------|---------------|
| `internal/nvr/onvif/recording_control.go` | Add TrackConfiguration type, 3 XML response types, extend recordingControlBody, add 3 public SOAP functions |
| `internal/nvr/api/recording_control.go` | Add 3 API handler methods on CameraHandler |
| `internal/nvr/api/router.go` | Register 3 new routes |

---

### Task 1: Add ONVIF track SOAP types and GetTrackConfiguration

**Files:**
- Modify: `internal/nvr/onvif/recording_control.go`

- [ ] **Step 1: Add the TrackConfiguration JSON type**

Add after the `RecordingJobState` type (around line 50):

```go
// TrackConfiguration holds the configuration for a track within a recording.
type TrackConfiguration struct {
	TrackToken  string `json:"track_token"`
	TrackType   string `json:"track_type"`
	Description string `json:"description"`
}
```

- [ ] **Step 2: Add XML response types for track operations**

Add after `recordingJobSourceTokenXML` (around line 132):

```go
// --- Track SOAP XML response types ---

type createTrackResponse struct {
	TrackToken string `xml:"TrackToken"`
}

type deleteTrackResponse struct{}

type getTrackConfigurationResponse struct {
	TrackConfiguration trackConfigurationXML `xml:"TrackConfiguration"`
}

type trackConfigurationXML struct {
	TrackToken  string `xml:"token,attr"`
	TrackType   string `xml:"TrackType"`
	Description string `xml:"Description"`
}
```

- [ ] **Step 3: Extend recordingControlBody with track response fields**

Add three new fields to the `recordingControlBody` struct (after `GetRecordingJobStateResponse`, before `Fault`):

```go
	CreateTrackResponse             *createTrackResponse             `xml:"CreateTrackResponse"`
	DeleteTrackResponse             *deleteTrackResponse             `xml:"DeleteTrackResponse"`
	GetTrackConfigurationResponse   *getTrackConfigurationResponse   `xml:"GetTrackConfigurationResponse"`
```

- [ ] **Step 4: Add GetTrackConfiguration function**

Add at the end of the file:

```go
// GetTrackConfiguration returns the configuration for a specific track within a recording.
func GetTrackConfiguration(xaddr, username, password, recordingToken, trackToken string) (*TrackConfiguration, error) {
	controlURL, err := getRecordingControlURL(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("GetTrackConfiguration: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	reqBody := fmt.Sprintf(`<trc:GetTrackConfiguration>
      <trc:RecordingToken>%s</trc:RecordingToken>
      <trc:TrackToken>%s</trc:TrackToken>
    </trc:GetTrackConfiguration>`,
		xmlEscape(recordingToken),
		xmlEscape(trackToken))

	body, err := doRecordingControlSOAP(ctx, controlURL, username, password, reqBody)
	if err != nil {
		return nil, fmt.Errorf("GetTrackConfiguration: %w", err)
	}

	var env recordingControlEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("GetTrackConfiguration parse: %w", err)
	}
	if env.Body.Fault != nil {
		return nil, fmt.Errorf("GetTrackConfiguration SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetTrackConfigurationResponse == nil {
		return nil, fmt.Errorf("GetTrackConfiguration: empty response")
	}

	tc := env.Body.GetTrackConfigurationResponse.TrackConfiguration
	return &TrackConfiguration{
		TrackToken:  tc.TrackToken,
		TrackType:   tc.TrackType,
		Description: tc.Description,
	}, nil
}
```

- [ ] **Step 5: Build to verify compilation**

```bash
cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...
```

Expected: clean build, no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/onvif/recording_control.go
git commit -m "feat(nvr/onvif): add track types and GetTrackConfiguration (KAI-11)"
```

---

### Task 2: Add CreateTrack and DeleteTrack ONVIF functions

**Files:**
- Modify: `internal/nvr/onvif/recording_control.go`

- [ ] **Step 1: Add CreateTrack function**

Add at the end of the file:

```go
// CreateTrack adds a new track to a recording on the device's edge storage.
// TrackType must be "Video", "Audio", or "Metadata".
func CreateTrack(xaddr, username, password, recordingToken, trackType, description string) (string, error) {
	controlURL, err := getRecordingControlURL(xaddr, username, password)
	if err != nil {
		return "", fmt.Errorf("CreateTrack: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	reqBody := fmt.Sprintf(`<trc:CreateTrack>
      <trc:RecordingToken>%s</trc:RecordingToken>
      <trc:TrackConfiguration>
        <tt:TrackType>%s</tt:TrackType>
        <tt:Description>%s</tt:Description>
      </trc:TrackConfiguration>
    </trc:CreateTrack>`,
		xmlEscape(recordingToken),
		xmlEscape(trackType),
		xmlEscape(description))

	body, err := doRecordingControlSOAP(ctx, controlURL, username, password, reqBody)
	if err != nil {
		return "", fmt.Errorf("CreateTrack: %w", err)
	}

	var env recordingControlEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("CreateTrack parse: %w", err)
	}
	if env.Body.Fault != nil {
		return "", fmt.Errorf("CreateTrack SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.CreateTrackResponse == nil {
		return "", fmt.Errorf("CreateTrack: empty response")
	}

	token := strings.TrimSpace(env.Body.CreateTrackResponse.TrackToken)
	if token == "" {
		return "", fmt.Errorf("CreateTrack: empty track token in response")
	}
	return token, nil
}
```

- [ ] **Step 2: Add DeleteTrack function**

Add at the end of the file:

```go
// DeleteTrack removes a track from a recording on the device's edge storage.
func DeleteTrack(xaddr, username, password, recordingToken, trackToken string) error {
	controlURL, err := getRecordingControlURL(xaddr, username, password)
	if err != nil {
		return fmt.Errorf("DeleteTrack: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	reqBody := fmt.Sprintf(`<trc:DeleteTrack>
      <trc:RecordingToken>%s</trc:RecordingToken>
      <trc:TrackToken>%s</trc:TrackToken>
    </trc:DeleteTrack>`,
		xmlEscape(recordingToken),
		xmlEscape(trackToken))

	body, err := doRecordingControlSOAP(ctx, controlURL, username, password, reqBody)
	if err != nil {
		return fmt.Errorf("DeleteTrack: %w", err)
	}

	var env recordingControlEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("DeleteTrack parse: %w", err)
	}
	if env.Body.Fault != nil {
		return fmt.Errorf("DeleteTrack SOAP fault: %s", env.Body.Fault.Faultstring)
	}

	return nil
}
```

- [ ] **Step 3: Build to verify compilation**

```bash
cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...
```

Expected: clean build, no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/onvif/recording_control.go
git commit -m "feat(nvr/onvif): add CreateTrack and DeleteTrack (KAI-11)"
```

---

### Task 3: Add API handlers for track management

**Files:**
- Modify: `internal/nvr/api/recording_control.go`

- [ ] **Step 1: Add CreateEdgeTrack handler**

Add at the end of `internal/nvr/api/recording_control.go`:

```go
// CreateEdgeTrack adds a track to a recording on the camera's edge storage.
//
//	POST /cameras/:id/recording-control/recordings/:token/tracks
func (h *CameraHandler) CreateEdgeTrack(c *gin.Context) {
	id := c.Param("id")
	recordingToken := c.Param("token")

	var req struct {
		TrackType   string `json:"track_type"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if req.TrackType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "track_type is required (Video, Audio, or Metadata)"})
		return
	}

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for create track", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)
	trackToken, err := onvif.CreateTrack(
		cam.ONVIFEndpoint, cam.ONVIFUsername, password,
		recordingToken, req.TrackType, req.Description)
	if err != nil {
		nvrLogError("recording-control",
			fmt.Sprintf("failed to create track on camera %s recording %s", id, recordingToken), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to create track on device"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"track_token": trackToken})
}
```

- [ ] **Step 2: Add DeleteEdgeTrack handler**

Add at the end of `internal/nvr/api/recording_control.go`:

```go
// DeleteEdgeTrack removes a track from a recording on the camera's edge storage.
//
//	DELETE /cameras/:id/recording-control/recordings/:token/tracks/:trackToken
func (h *CameraHandler) DeleteEdgeTrack(c *gin.Context) {
	id := c.Param("id")
	recordingToken := c.Param("token")
	trackToken := c.Param("trackToken")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for delete track", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)
	err = onvif.DeleteTrack(
		cam.ONVIFEndpoint, cam.ONVIFUsername, password,
		recordingToken, trackToken)
	if err != nil {
		nvrLogError("recording-control",
			fmt.Sprintf("failed to delete track %s on camera %s", trackToken, id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to delete track on device"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "track deleted"})
}
```

- [ ] **Step 3: Add GetEdgeTrackConfig handler**

Add at the end of `internal/nvr/api/recording_control.go`:

```go
// GetEdgeTrackConfig returns the configuration for a track on the camera's edge storage.
//
//	GET /cameras/:id/recording-control/tracks/:trackToken/config?recording_token=X
func (h *CameraHandler) GetEdgeTrackConfig(c *gin.Context) {
	id := c.Param("id")
	trackToken := c.Param("trackToken")
	recordingToken := c.Query("recording_token")

	if recordingToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "recording_token query parameter is required"})
		return
	}

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for track config", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)
	config, err := onvif.GetTrackConfiguration(
		cam.ONVIFEndpoint, cam.ONVIFUsername, password,
		recordingToken, trackToken)
	if err != nil {
		nvrLogError("recording-control",
			fmt.Sprintf("failed to get track config for camera %s track %s", id, trackToken), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get track configuration from device"})
		return
	}

	c.JSON(http.StatusOK, config)
}
```

- [ ] **Step 4: Build to verify compilation**

```bash
cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...
```

Expected: clean build, no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/api/recording_control.go
git commit -m "feat(nvr/api): add track management API handlers (KAI-11)"
```

---

### Task 4: Register track routes

**Files:**
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Add track management routes**

In `internal/nvr/api/router.go`, add the following three routes immediately after the existing recording control routes (after line 259, after the `GetEdgeRecordingJobState` route):

```go
	// Track management (Profile G — manage tracks within recordings on device).
	protected.POST("/cameras/:id/recording-control/recordings/:token/tracks", cameraHandler.CreateEdgeTrack)
	protected.DELETE("/cameras/:id/recording-control/recordings/:token/tracks/:trackToken", cameraHandler.DeleteEdgeTrack)
	protected.GET("/cameras/:id/recording-control/tracks/:trackToken/config", cameraHandler.GetEdgeTrackConfig)
```

- [ ] **Step 2: Build to verify compilation**

```bash
cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...
```

Expected: clean build, no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/router.go
git commit -m "feat(nvr/api): register track management routes (KAI-11)"
```

---

### Task 5: Full build and test verification

- [ ] **Step 1: Run full NVR package build**

```bash
cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...
```

Expected: clean build, no errors.

- [ ] **Step 2: Run existing tests to verify no regressions**

```bash
cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/... -count=1 -timeout 120s
```

Expected: all existing tests pass.

- [ ] **Step 3: Run go vet**

```bash
cd /Users/ethanflower/personal_projects/mediamtx && go vet ./internal/nvr/...
```

Expected: no issues.
