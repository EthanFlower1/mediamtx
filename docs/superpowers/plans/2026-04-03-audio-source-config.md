# KAI-113: Audio Source Configuration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add audio source enumeration and configuration to the NVR ONVIF layer and expose via REST API.

**Architecture:** Extend `internal/nvr/onvif/audio.go` with 8 new functions wrapping onvif-go library calls (Media2 for reads when available, Media1 for writes). Add 8 corresponding Gin handler methods on `CameraHandler` and register routes under `/cameras/:id/audio/`.

**Tech Stack:** Go, onvif-go library (Media1 + Media2), Gin HTTP router

---

### File Structure

- **Modify:** `internal/nvr/onvif/audio.go` — Add types (`AudioSourceInfo`, `AudioSourceConfig`, `AudioSourceConfigOptions`) and 8 functions
- **Modify:** `internal/nvr/api/cameras.go` — Add 8 handler methods on `CameraHandler`
- **Modify:** `internal/nvr/api/router.go` — Register 8 new routes under the audio section

---

### Task 1: Create worktree

**Files:**
- None (git operation only)

- [ ] **Step 1: Create the worktree and branch**

```bash
git worktree add .worktrees/kai-113 -b feat/kai-113-audio-source-config
```

- [ ] **Step 2: Verify worktree**

```bash
cd .worktrees/kai-113 && git branch --show-current
```

Expected: `feat/kai-113-audio-source-config`

All remaining tasks execute inside `.worktrees/kai-113`.

---

### Task 2: Add audio source types and GetAudioSources

**Files:**
- Modify: `internal/nvr/onvif/audio.go`

- [ ] **Step 1: Add the new types after `AudioCapabilities`**

Add after the `AudioCapabilities` struct (after line 12):

```go
// AudioSourceInfo describes a physical audio input (microphone) on the device.
type AudioSourceInfo struct {
	Token    string `json:"token"`
	Channels int    `json:"channels"`
}

// AudioSourceConfig represents an audio source configuration that binds
// a physical audio source to a media profile.
type AudioSourceConfig struct {
	Token       string `json:"token"`
	Name        string `json:"name"`
	UseCount    int    `json:"use_count"`
	SourceToken string `json:"source_token"`
}

// AudioSourceConfigOptions describes the available options when configuring
// an audio source (e.g. which input tokens can be selected).
type AudioSourceConfigOptions struct {
	InputTokensAvailable []string `json:"input_tokens_available"`
}
```

- [ ] **Step 2: Add GetAudioSources function**

Add after `GetAudioCapabilities`:

```go
// GetAudioSources returns all audio sources (microphones) on the device.
func GetAudioSources(xaddr, username, password string) ([]*AudioSourceInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	sources, err := client.Dev.GetAudioSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("get audio sources: %w", err)
	}

	result := make([]*AudioSourceInfo, len(sources))
	for i, s := range sources {
		result[i] = &AudioSourceInfo{
			Token:    s.Token,
			Channels: s.Channels,
		}
	}
	return result, nil
}
```

- [ ] **Step 3: Add `"fmt"` to the import block** (if not already present)

The existing `audio.go` only imports `"context"`. Add `"fmt"`:

```go
import (
	"context"
	"fmt"
)
```

- [ ] **Step 4: Verify it compiles**

```bash
cd .worktrees/kai-113 && go build ./internal/nvr/onvif/...
```

Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/onvif/audio.go
git commit -m "feat(audio): add audio source types and GetAudioSources"
```

---

### Task 3: Add GetAudioSourceConfigurations with Media2 fallback

**Files:**
- Modify: `internal/nvr/onvif/audio.go`

- [ ] **Step 1: Add the helper to convert library configs to local type**

Add after `GetAudioSources`:

```go
func convertAudioSourceConfigs(configs []*onvifgo.AudioSourceConfiguration) []*AudioSourceConfig {
	result := make([]*AudioSourceConfig, len(configs))
	for i, cfg := range configs {
		result[i] = &AudioSourceConfig{
			Token:       cfg.Token,
			Name:        cfg.Name,
			UseCount:    cfg.UseCount,
			SourceToken: cfg.SourceToken,
		}
	}
	return result
}
```

- [ ] **Step 2: Add GetAudioSourceConfigurations with Media2-first logic**

```go
// GetAudioSourceConfigurations returns all audio source configurations.
// It tries Media2 first and falls back to Media1.
func GetAudioSourceConfigurations(xaddr, username, password string) ([]*AudioSourceConfig, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()

	if client.HasService("media2") {
		configs, err := client.Dev.GetMedia2AudioSourceConfigurations(ctx, nil, nil)
		if err == nil {
			return convertAudioSourceConfigs(configs), nil
		}
	}

	configs, err := client.Dev.GetAudioSourceConfigurations(ctx)
	if err != nil {
		return nil, fmt.Errorf("get audio source configurations: %w", err)
	}
	return convertAudioSourceConfigs(configs), nil
}
```

- [ ] **Step 3: Add the onvif-go import alias**

Update imports to include the library:

```go
import (
	"context"
	"fmt"

	onvifgo "github.com/EthanFlower1/onvif-go"
)
```

- [ ] **Step 4: Verify it compiles**

```bash
cd .worktrees/kai-113 && go build ./internal/nvr/onvif/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/onvif/audio.go
git commit -m "feat(audio): add GetAudioSourceConfigurations with Media2 fallback"
```

---

### Task 4: Add remaining ONVIF audio source functions

**Files:**
- Modify: `internal/nvr/onvif/audio.go`

- [ ] **Step 1: Add GetAudioSourceConfiguration (single config by token)**

```go
// GetAudioSourceConfiguration returns a specific audio source configuration.
func GetAudioSourceConfiguration(xaddr, username, password, configToken string) (*AudioSourceConfig, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	cfg, err := client.Dev.GetAudioSourceConfiguration(ctx, configToken)
	if err != nil {
		return nil, fmt.Errorf("get audio source configuration: %w", err)
	}
	return &AudioSourceConfig{
		Token:       cfg.Token,
		Name:        cfg.Name,
		UseCount:    cfg.UseCount,
		SourceToken: cfg.SourceToken,
	}, nil
}
```

- [ ] **Step 2: Add SetAudioSourceConfiguration**

```go
// SetAudioSourceConfiguration updates an audio source configuration on the device.
func SetAudioSourceConfiguration(xaddr, username, password string, cfg *AudioSourceConfig) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	asc := &onvifgo.AudioSourceConfiguration{
		Token:       cfg.Token,
		Name:        cfg.Name,
		SourceToken: cfg.SourceToken,
	}

	ctx := context.Background()
	if err := client.Dev.SetAudioSourceConfiguration(ctx, asc, true); err != nil {
		return fmt.Errorf("set audio source configuration: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: Add GetAudioSourceConfigOptions**

```go
// GetAudioSourceConfigOptions returns the available options for an audio source configuration.
func GetAudioSourceConfigOptions(xaddr, username, password, configToken, profileToken string) (*AudioSourceConfigOptions, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	opts, err := client.Dev.GetAudioSourceConfigurationOptions(ctx, configToken, profileToken)
	if err != nil {
		return nil, fmt.Errorf("get audio source config options: %w", err)
	}
	return &AudioSourceConfigOptions{
		InputTokensAvailable: opts.InputTokensAvailable,
	}, nil
}
```

- [ ] **Step 4: Add GetCompatibleAudioSourceConfigs**

```go
// GetCompatibleAudioSourceConfigs returns audio source configurations compatible with a profile.
func GetCompatibleAudioSourceConfigs(xaddr, username, password, profileToken string) ([]*AudioSourceConfig, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	configs, err := client.Dev.GetCompatibleAudioSourceConfigurations(ctx, profileToken)
	if err != nil {
		return nil, fmt.Errorf("get compatible audio source configurations: %w", err)
	}
	return convertAudioSourceConfigs(configs), nil
}
```

- [ ] **Step 5: Add AddAudioSourceToProfile**

```go
// AddAudioSourceToProfile adds an audio source configuration to a media profile.
func AddAudioSourceToProfile(xaddr, username, password, profileToken, configToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.AddAudioSourceConfiguration(ctx, profileToken, configToken); err != nil {
		return fmt.Errorf("add audio source to profile: %w", err)
	}
	return nil
}
```

- [ ] **Step 6: Add RemoveAudioSourceFromProfile**

```go
// RemoveAudioSourceFromProfile removes the audio source configuration from a media profile.
func RemoveAudioSourceFromProfile(xaddr, username, password, profileToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.RemoveAudioSourceConfiguration(ctx, profileToken); err != nil {
		return fmt.Errorf("remove audio source from profile: %w", err)
	}
	return nil
}
```

- [ ] **Step 7: Verify it compiles**

```bash
cd .worktrees/kai-113 && go build ./internal/nvr/onvif/...
```

- [ ] **Step 8: Commit**

```bash
git add internal/nvr/onvif/audio.go
git commit -m "feat(audio): add Set, Options, Compatible, Add, Remove audio source functions"
```

---

### Task 5: Add API handlers for read-only audio source endpoints

**Files:**
- Modify: `internal/nvr/api/cameras.go`

All handlers go after the existing `AudioCapabilities` method (after line 1377 in cameras.go). They follow the exact same pattern as `AudioCapabilities`: parse ID, get camera from DB, check ONVIF endpoint, call onvif function, return JSON.

- [ ] **Step 1: Add AudioSources handler**

```go
// AudioSources returns all audio sources (microphones) on the camera.
//
//	GET /cameras/:id/audio/sources
func (h *CameraHandler) AudioSources(c *gin.Context) {
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

	sources, err := onvif.GetAudioSources(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword))
	if err != nil {
		nvrLogError("audio", fmt.Sprintf("failed to get audio sources for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get audio sources from device"})
		return
	}

	c.JSON(http.StatusOK, sources)
}
```

- [ ] **Step 2: Add AudioSourceConfigs handler**

```go
// AudioSourceConfigs returns all audio source configurations from the camera.
//
//	GET /cameras/:id/audio/source-configs
func (h *CameraHandler) AudioSourceConfigs(c *gin.Context) {
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

	configs, err := onvif.GetAudioSourceConfigurations(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword))
	if err != nil {
		nvrLogError("audio", fmt.Sprintf("failed to get audio source configs for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get audio source configurations from device"})
		return
	}

	c.JSON(http.StatusOK, configs)
}
```

- [ ] **Step 3: Add GetAudioSourceConfig handler**

```go
// GetAudioSourceConfig returns a specific audio source configuration.
//
//	GET /cameras/:id/audio/source-configs/:token
func (h *CameraHandler) GetAudioSourceConfig(c *gin.Context) {
	id := c.Param("id")
	token := c.Param("token")

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

	cfg, err := onvif.GetAudioSourceConfiguration(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), token)
	if err != nil {
		nvrLogError("audio", fmt.Sprintf("failed to get audio source config %s for camera %s", token, id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get audio source configuration from device"})
		return
	}

	c.JSON(http.StatusOK, cfg)
}
```

- [ ] **Step 4: Add AudioSourceConfigOptions handler**

```go
// AudioSourceConfigOptions returns the available options for an audio source configuration.
//
//	GET /cameras/:id/audio/source-configs/:token/options
func (h *CameraHandler) AudioSourceConfigOptions(c *gin.Context) {
	id := c.Param("id")
	token := c.Param("token")

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

	opts, err := onvif.GetAudioSourceConfigOptions(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), token, "")
	if err != nil {
		nvrLogError("audio", fmt.Sprintf("failed to get audio source config options for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get audio source configuration options from device"})
		return
	}

	c.JSON(http.StatusOK, opts)
}
```

- [ ] **Step 5: Add CompatibleAudioSourceConfigs handler**

```go
// CompatibleAudioSourceConfigs returns audio source configurations compatible with a profile.
//
//	GET /cameras/:id/audio/source-configs/compatible/:profileToken
func (h *CameraHandler) CompatibleAudioSourceConfigs(c *gin.Context) {
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

	configs, err := onvif.GetCompatibleAudioSourceConfigs(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), profileToken)
	if err != nil {
		nvrLogError("audio", fmt.Sprintf("failed to get compatible audio source configs for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get compatible audio source configurations from device"})
		return
	}

	c.JSON(http.StatusOK, configs)
}
```

- [ ] **Step 6: Verify it compiles**

```bash
cd .worktrees/kai-113 && go build ./internal/nvr/api/...
```

- [ ] **Step 7: Commit**

```bash
git add internal/nvr/api/cameras.go
git commit -m "feat(audio): add read-only audio source API handlers"
```

---

### Task 6: Add API handlers for mutation audio source endpoints

**Files:**
- Modify: `internal/nvr/api/cameras.go`

- [ ] **Step 1: Add UpdateAudioSourceConfig handler**

```go
// UpdateAudioSourceConfig updates an audio source configuration on the camera.
//
//	PUT /cameras/:id/audio/source-configs/:token
func (h *CameraHandler) UpdateAudioSourceConfig(c *gin.Context) {
	id := c.Param("id")
	token := c.Param("token")

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

	var req onvif.AudioSourceConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	req.Token = token

	if err := onvif.SetAudioSourceConfiguration(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), &req); err != nil {
		nvrLogError("audio", fmt.Sprintf("failed to set audio source config %s for camera %s", token, id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to update audio source configuration on device"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
```

- [ ] **Step 2: Add AddAudioSourceToProfile handler**

```go
// AddAudioSourceToProfile adds an audio source configuration to a media profile.
//
//	POST /cameras/:id/audio/source-configs/add
func (h *CameraHandler) AddAudioSourceToProfile(c *gin.Context) {
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
		ProfileToken string `json:"profile_token"`
		ConfigToken  string `json:"config_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ProfileToken == "" || req.ConfigToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "profile_token and config_token are required"})
		return
	}

	if err := onvif.AddAudioSourceToProfile(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), req.ProfileToken, req.ConfigToken); err != nil {
		nvrLogError("audio", fmt.Sprintf("failed to add audio source to profile for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to add audio source configuration to profile"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
```

- [ ] **Step 3: Add RemoveAudioSourceFromProfile handler**

```go
// RemoveAudioSourceFromProfile removes the audio source configuration from a media profile.
//
//	POST /cameras/:id/audio/source-configs/remove
func (h *CameraHandler) RemoveAudioSourceFromProfile(c *gin.Context) {
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
		ProfileToken string `json:"profile_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ProfileToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "profile_token is required"})
		return
	}

	if err := onvif.RemoveAudioSourceFromProfile(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), req.ProfileToken); err != nil {
		nvrLogError("audio", fmt.Sprintf("failed to remove audio source from profile for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to remove audio source configuration from profile"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
```

- [ ] **Step 4: Verify it compiles**

```bash
cd .worktrees/kai-113 && go build ./internal/nvr/api/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/api/cameras.go
git commit -m "feat(audio): add mutation audio source API handlers"
```

---

### Task 7: Register routes

**Files:**
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Add routes after the existing audio capabilities line**

In `router.go`, find the section (around line 245-247):

```go
	// Audio capabilities.
	protected.GET("/cameras/:id/audio/capabilities", cameraHandler.AudioCapabilities)
```

Replace it with:

```go
	// Audio.
	protected.GET("/cameras/:id/audio/capabilities", cameraHandler.AudioCapabilities)
	protected.GET("/cameras/:id/audio/sources", cameraHandler.AudioSources)
	protected.GET("/cameras/:id/audio/source-configs", cameraHandler.AudioSourceConfigs)
	protected.GET("/cameras/:id/audio/source-configs/compatible/:profileToken", cameraHandler.CompatibleAudioSourceConfigs)
	protected.GET("/cameras/:id/audio/source-configs/:token", cameraHandler.GetAudioSourceConfig)
	protected.PUT("/cameras/:id/audio/source-configs/:token", cameraHandler.UpdateAudioSourceConfig)
	protected.GET("/cameras/:id/audio/source-configs/:token/options", cameraHandler.AudioSourceConfigOptions)
	protected.POST("/cameras/:id/audio/source-configs/add", cameraHandler.AddAudioSourceToProfile)
	protected.POST("/cameras/:id/audio/source-configs/remove", cameraHandler.RemoveAudioSourceFromProfile)
```

Note: The `compatible/:profileToken` route is registered **before** the `:token` routes so Gin doesn't match "compatible" as a token value.

- [ ] **Step 2: Verify it compiles**

```bash
cd .worktrees/kai-113 && go build ./...
```

- [ ] **Step 3: Run existing tests**

```bash
cd .worktrees/kai-113 && go test ./internal/nvr/... 2>&1 | tail -20
```

Expected: all existing tests pass

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/router.go
git commit -m "feat(audio): register audio source configuration API routes"
```

---

### Task 8: Push branch and create PR

**Files:**
- None (git operations only)

- [ ] **Step 1: Push the branch**

```bash
cd .worktrees/kai-113 && git push -u origin feat/kai-113-audio-source-config
```

- [ ] **Step 2: Create the PR**

```bash
cd .worktrees/kai-113 && gh pr create --title "feat: add audio source configuration and enumeration (KAI-113)" --body "$(cat <<'EOF'
## Summary
- Add audio source enumeration and configuration to the ONVIF layer (`internal/nvr/onvif/audio.go`)
- Support Media2 for reads (with Media1 fallback) and Media1 for writes
- Expose 8 new API endpoints under `/cameras/:id/audio/`
- Operations: list sources, list/get/set source configs, get options, compatible configs, add/remove from profile

## Test plan
- [ ] Verify `GET /audio/sources` returns microphone list from ONVIF camera
- [ ] Verify `GET /audio/source-configs` returns configurations (uses Media2 when available)
- [ ] Verify `GET /audio/source-configs/:token` returns specific configuration
- [ ] Verify `PUT /audio/source-configs/:token` updates configuration on device
- [ ] Verify `GET /audio/source-configs/:token/options` returns available input tokens
- [ ] Verify `GET /audio/source-configs/compatible/:profileToken` returns compatible configs
- [ ] Verify `POST /audio/source-configs/add` binds audio source to profile
- [ ] Verify `POST /audio/source-configs/remove` unbinds audio source from profile
- [ ] Verify all endpoints return 404 for unknown camera, 400 for missing ONVIF endpoint

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```
