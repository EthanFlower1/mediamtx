# ONVIF Credential Rotation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `POST /cameras/:id/rotate-credentials` endpoint that validates new ONVIF credentials against the camera before saving them, with implicit rollback on failure.

**Architecture:** A new `RotateCredentials` handler on `CameraHandler` validates credentials via `onvif.ProbeDeviceFull()`, encrypts and saves on success, and triggers scheduler resubscription. The scheduler gets a new `ResubscribeCamera()` method to tear down and recreate event subscriptions with fresh credentials.

**Tech Stack:** Go, Gin, SQLite, AES-256-GCM encryption, ONVIF

---

### Task 1: Add `ResubscribeCamera` to Scheduler

**Files:**
- Modify: `internal/nvr/scheduler/scheduler.go`

- [ ] **Step 1: Add `ResubscribeCamera` method**

Add the following method after the existing `stopEventPipelineLocked` method (around line 1187):

```go
// ResubscribeCamera tears down any active event subscription for the given
// camera and starts a fresh one using the current credentials from the DB.
// This should be called after credential rotation so that ONVIF event
// subscriptions use the updated credentials.
func (s *Scheduler) ResubscribeCamera(cameraID string) {
	cam, err := s.db.GetCamera(cameraID)
	if err != nil {
		log.Printf("scheduler: resubscribe camera %s: %v", cameraID, err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Tear down existing subscription if any.
	s.stopEventPipelineLocked(cameraID)

	// Only resubscribe if the camera has an ONVIF endpoint.
	if cam.ONVIFEndpoint == "" {
		return
	}

	// Check if there are active event-mode rules for this camera.
	rules, err := s.db.ListRecordingRules(cameraID)
	if err != nil {
		log.Printf("scheduler: resubscribe camera %s: list rules: %v", cameraID, err)
		return
	}

	hasEventRule := false
	for _, r := range rules {
		if r.Mode == "events" && r.Enabled {
			hasEventRule = true
			break
		}
	}

	if hasEventRule {
		s.startEventPipelineLocked(cameraID, cam, rules)
	} else {
		// Even without event rules, start the motion alert subscription.
		s.startMotionAlertSubscription(cam)
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/scheduler/`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/scheduler/scheduler.go
git commit -m "feat(scheduler): add ResubscribeCamera for credential rotation"
```

---

### Task 2: Add `RotateCredentials` Handler and Route

**Files:**
- Modify: `internal/nvr/api/cameras.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Add the `rotateCredentialsRequest` type and `RotateCredentials` handler**

Add the following to `internal/nvr/api/cameras.go`, after the `encryptPassword` / `decryptPassword` helpers (around line 193):

```go
// rotateCredentialsRequest is the JSON body for credential rotation.
type rotateCredentialsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// RotateCredentials validates new ONVIF credentials against the camera and
// saves them only if the ONVIF probe succeeds. Old credentials remain
// untouched on failure.
//
//	POST /cameras/:id/rotate-credentials
func (h *CameraHandler) RotateCredentials(c *gin.Context) {
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

	var req rotateCredentialsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.Username == "" && req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one of username or password must be provided"})
		return
	}

	// Use new values where provided, fall back to existing for omitted fields.
	effectiveUsername := cam.ONVIFUsername
	if req.Username != "" {
		effectiveUsername = req.Username
	}
	effectivePassword := h.decryptPassword(cam.ONVIFPassword)
	if req.Password != "" {
		effectivePassword = req.Password
	}

	// Validate new credentials against the camera's ONVIF endpoint.
	_, err = onvif.ProbeDeviceFull(cam.ONVIFEndpoint, effectiveUsername, effectivePassword)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("new credentials failed ONVIF authentication: %v", err),
		})
		return
	}

	// Credentials validated — save to DB.
	cam.ONVIFUsername = effectiveUsername
	cam.ONVIFPassword = h.encryptPassword(effectivePassword)

	if err := h.DB.UpdateCamera(cam); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to save rotated credentials", err)
		return
	}

	// Audit log.
	if h.Audit != nil {
		h.Audit.logAction(c, "credential_rotation", "camera", cam.ID, fmt.Sprintf("Rotated credentials for camera %q", cam.Name))
	}

	// Resubscribe ONVIF events with new credentials.
	if h.Scheduler != nil {
		go h.Scheduler.ResubscribeCamera(cam.ID)
	}

	c.JSON(http.StatusOK, gin.H{
		"id":      cam.ID,
		"message": "credentials rotated successfully",
	})
}
```

- [ ] **Step 2: Register the route**

In `internal/nvr/api/router.go`, add the following line after the `protected.POST("/cameras/:id/refresh", cameraHandler.RefreshCapabilities)` line (line 200):

```go
	protected.POST("/cameras/:id/rotate-credentials", cameraHandler.RotateCredentials)
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/api/`
Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/cameras.go internal/nvr/api/router.go
git commit -m "feat(api): add POST /cameras/:id/rotate-credentials endpoint"
```

---

### Task 3: Add Unit Tests

**Files:**
- Modify: `internal/nvr/api/cameras_test.go`

- [ ] **Step 1: Write tests for the `RotateCredentials` handler**

Add the following tests to `internal/nvr/api/cameras_test.go`:

```go
func TestRotateCredentials_CameraNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, cleanup := setupCameraTest(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "nonexistent"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"username":"admin","password":"pass"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.RotateCredentials(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRotateCredentials_NoONVIFEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, cleanup := setupCameraTest(t)
	defer cleanup()

	// Create a camera without ONVIF endpoint.
	cam := &db.Camera{
		ID:           "cam-no-onvif",
		Name:         "No ONVIF",
		RTSPURL:      "rtsp://192.168.1.100/stream",
		MediaMTXPath: "cam-no-onvif",
	}
	require.NoError(t, handler.DB.CreateCamera(cam))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "cam-no-onvif"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"username":"admin","password":"pass"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.RotateCredentials(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "no ONVIF endpoint")
}

func TestRotateCredentials_EmptyCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, cleanup := setupCameraTest(t)
	defer cleanup()

	cam := &db.Camera{
		ID:            "cam-empty-creds",
		Name:          "Test Cam",
		ONVIFEndpoint: "http://192.168.1.100:80/onvif/device_service",
		MediaMTXPath:  "cam-empty-creds",
	}
	require.NoError(t, handler.DB.CreateCamera(cam))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "cam-empty-creds"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.RotateCredentials(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "at least one of username or password")
}

func TestRotateCredentials_ProbeFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, cleanup := setupCameraTest(t)
	defer cleanup()

	// Create a camera with an unreachable ONVIF endpoint — probe will fail.
	cam := &db.Camera{
		ID:            "cam-probe-fail",
		Name:          "Probe Fail",
		ONVIFEndpoint: "http://192.0.2.1:80/onvif/device_service", // RFC 5737 TEST-NET
		ONVIFUsername: "olduser",
		ONVIFPassword: "oldpass",
		MediaMTXPath:  "cam-probe-fail",
	}
	require.NoError(t, handler.DB.CreateCamera(cam))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "cam-probe-fail"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"username":"newuser","password":"newpass"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.RotateCredentials(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "failed ONVIF authentication")

	// Verify old credentials are unchanged.
	updated, err := handler.DB.GetCamera("cam-probe-fail")
	require.NoError(t, err)
	assert.Equal(t, "olduser", updated.ONVIFUsername)
	assert.Equal(t, "oldpass", updated.ONVIFPassword)
}
```

- [ ] **Step 2: Run the tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestRotateCredentials -v -timeout 30s`
Expected: All 4 tests pass. The `TestRotateCredentials_ProbeFailure` test may take a few seconds due to the network timeout on the unreachable address.

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/cameras_test.go
git commit -m "test(api): add unit tests for credential rotation endpoint"
```
