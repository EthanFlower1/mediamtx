# KAI-17: Complete ONVIF Media Profile Management — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Media2 profile CRUD, video/audio source configuration operations, and expose all via NVR API endpoints.

**Architecture:** Extend the existing `doMedia2SOAP` pattern in `media2.go` with new SOAP request/response types for profile management, video source configs, and audio source configs. Add public wrapper functions in `media_config.go` that create a client and delegate. Wire up 9 new Gin handlers in `cameras.go` with routes in `router.go`.

**Tech Stack:** Go, ONVIF Media2 SOAP (tr2 namespace), Gin HTTP framework, encoding/xml

---

## File Structure

| File                                 | Action | Responsibility                                                                                                                                                                                                                                                                             |
| ------------------------------------ | ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `internal/nvr/onvif/media2.go`       | Modify | Add SOAP XML types and low-level Media2 functions (CreateProfile2, DeleteProfile2, AddConfiguration2, RemoveConfiguration2, GetVideoSourceConfigurations2, SetVideoSourceConfiguration2, GetVideoSourceConfigurationOptions2, GetAudioSourceConfigurations2, SetAudioSourceConfiguration2) |
| `internal/nvr/onvif/media_config.go` | Modify | Add new data types (VideoSourceConfig, AudioSourceConfig, etc.) and public wrapper functions                                                                                                                                                                                               |
| `internal/nvr/onvif/media2_test.go`  | Create | XML parsing unit tests for new SOAP response types                                                                                                                                                                                                                                         |
| `internal/nvr/api/cameras.go`        | Modify | Add 9 new handler methods on CameraHandler                                                                                                                                                                                                                                                 |
| `internal/nvr/api/router.go`         | Modify | Register 9 new routes under `/cameras/:id/media2/`                                                                                                                                                                                                                                         |

---

### Task 1: Add Media2 Profile CRUD SOAP Types and Functions

**Files:**

- Modify: `internal/nvr/onvif/media2.go:16-27` (extend media2Body struct)
- Modify: `internal/nvr/onvif/media2.go` (append new types + functions)

- [ ] **Step 1: Add SOAP response types for CreateProfile and DeleteProfile**

Append these types after the existing `getSnapshotUri2Response` type (line 66) in `media2.go`:

```go
// --- Media2 Profile CRUD response types ---

type createProfile2Response struct {
	Token string `xml:"Token"`
	Name  string `xml:"Name"`
}

type deleteProfile2Response struct {
	// Empty — success is indicated by HTTP 200 with no fault.
}

type addConfiguration2Response struct {
	// Empty — success is indicated by HTTP 200 with no fault.
}

type removeConfiguration2Response struct {
	// Empty — success is indicated by HTTP 200 with no fault.
}
```

Add these fields to the `media2Body` struct (after `GetSnapshotUriResponse` on line 25):

```go
	CreateProfileResponse       *createProfile2Response       `xml:"CreateProfileResponse"`
	DeleteProfileResponse       *deleteProfile2Response       `xml:"DeleteProfileResponse"`
	AddConfigurationResponse    *addConfiguration2Response    `xml:"AddConfigurationResponse"`
	RemoveConfigurationResponse *removeConfiguration2Response `xml:"RemoveConfigurationResponse"`
```

- [ ] **Step 2: Implement CreateProfile2**

Append to `media2.go`:

```go
// CreateProfile2 creates a new media profile via Media2.
func CreateProfile2(client *Client, name string) (string, string, error) {
	reqBody := fmt.Sprintf(`<tr2:CreateProfile>
      <tr2:Name>%s</tr2:Name>
    </tr2:CreateProfile>`, name)

	body, err := doMedia2SOAP(client, reqBody)
	if err != nil {
		return "", "", fmt.Errorf("media2 CreateProfile: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return "", "", fmt.Errorf("media2 CreateProfile parse: %w", err)
	}
	if env.Body.Fault != nil {
		return "", "", fmt.Errorf("media2 CreateProfile SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.CreateProfileResponse == nil {
		return "", "", fmt.Errorf("media2 CreateProfile: empty response")
	}

	return env.Body.CreateProfileResponse.Token, env.Body.CreateProfileResponse.Name, nil
}
```

- [ ] **Step 3: Implement DeleteProfile2**

Append to `media2.go`:

```go
// DeleteProfile2 deletes a media profile via Media2.
func DeleteProfile2(client *Client, token string) error {
	reqBody := fmt.Sprintf(`<tr2:DeleteProfile>
      <tr2:Token>%s</tr2:Token>
    </tr2:DeleteProfile>`, token)

	body, err := doMedia2SOAP(client, reqBody)
	if err != nil {
		return fmt.Errorf("media2 DeleteProfile: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("media2 DeleteProfile parse: %w", err)
	}
	if env.Body.Fault != nil {
		return fmt.Errorf("media2 DeleteProfile SOAP fault: %s", env.Body.Fault.Faultstring)
	}

	return nil
}
```

- [ ] **Step 4: Implement AddConfiguration2**

Append to `media2.go`:

```go
// AddConfiguration2 adds a configuration to a profile via Media2.
// configType is one of: VideoSource, VideoEncoder, AudioSource, AudioEncoder, PTZ, Analytics, Metadata.
func AddConfiguration2(client *Client, profileToken, configType, configToken string) error {
	reqBody := fmt.Sprintf(`<tr2:AddConfiguration>
      <tr2:ProfileToken>%s</tr2:ProfileToken>
      <tr2:Configuration>
        <tr2:Type>%s</tr2:Type>
        <tr2:Token>%s</tr2:Token>
      </tr2:Configuration>
    </tr2:AddConfiguration>`, profileToken, configType, configToken)

	body, err := doMedia2SOAP(client, reqBody)
	if err != nil {
		return fmt.Errorf("media2 AddConfiguration: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("media2 AddConfiguration parse: %w", err)
	}
	if env.Body.Fault != nil {
		return fmt.Errorf("media2 AddConfiguration SOAP fault: %s", env.Body.Fault.Faultstring)
	}

	return nil
}
```

- [ ] **Step 5: Implement RemoveConfiguration2**

Append to `media2.go`:

```go
// RemoveConfiguration2 removes a configuration from a profile via Media2.
func RemoveConfiguration2(client *Client, profileToken, configType, configToken string) error {
	reqBody := fmt.Sprintf(`<tr2:RemoveConfiguration>
      <tr2:ProfileToken>%s</tr2:ProfileToken>
      <tr2:Configuration>
        <tr2:Type>%s</tr2:Type>
        <tr2:Token>%s</tr2:Token>
      </tr2:Configuration>
    </tr2:RemoveConfiguration>`, profileToken, configType, configToken)

	body, err := doMedia2SOAP(client, reqBody)
	if err != nil {
		return fmt.Errorf("media2 RemoveConfiguration: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("media2 RemoveConfiguration parse: %w", err)
	}
	if env.Body.Fault != nil {
		return fmt.Errorf("media2 RemoveConfiguration SOAP fault: %s", env.Body.Fault.Faultstring)
	}

	return nil
}
```

- [ ] **Step 6: Verify compilation**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-17 && go build ./internal/nvr/onvif/...`
Expected: No errors

- [ ] **Step 7: Commit**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-17
git add internal/nvr/onvif/media2.go
git commit -m "feat(onvif): add Media2 profile CRUD SOAP operations"
```

---

### Task 2: Add Media2 Video Source Configuration SOAP Types and Functions

**Files:**

- Modify: `internal/nvr/onvif/media2.go` (extend media2Body, add types + functions)
- Modify: `internal/nvr/onvif/media_config.go` (add VideoSourceConfig and related types)

- [ ] **Step 1: Add data types in media_config.go**

Append after the `Range` struct (line 73) in `media_config.go`:

```go
// VideoSourceConfig holds a video source configuration from the device.
type VideoSourceConfig struct {
	Token       string        `json:"token"`
	Name        string        `json:"name"`
	SourceToken string        `json:"source_token"`
	Bounds      *IntRectangle `json:"bounds,omitempty"`
}

// IntRectangle represents a rectangle with position and dimensions.
type IntRectangle struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// VideoSourceConfigOptions holds the available options for a video source configuration.
type VideoSourceConfigOptions struct {
	BoundsRange             *IntRectangleRange `json:"bounds_range,omitempty"`
	MaximumNumberOfProfiles int                `json:"maximum_number_of_profiles,omitempty"`
}

// IntRectangleRange represents min/max ranges for each rectangle dimension.
type IntRectangleRange struct {
	XRange      Range `json:"x_range"`
	YRange      Range `json:"y_range"`
	WidthRange  Range `json:"width_range"`
	HeightRange Range `json:"height_range"`
}

// AudioSourceConfig holds an audio source configuration from the device.
type AudioSourceConfig struct {
	Token       string `json:"token"`
	Name        string `json:"name"`
	SourceToken string `json:"source_token"`
}
```

- [ ] **Step 2: Add SOAP response types for video source configs in media2.go**

Append after the `removeConfiguration2Response` type:

```go
// --- Media2 Video Source Configuration response types ---

type media2VideoSourceConfig struct {
	Token       string          `xml:"token,attr"`
	Name        string          `xml:"Name"`
	SourceToken string          `xml:"SourceToken"`
	Bounds      *media2Bounds   `xml:"Bounds"`
}

type media2Bounds struct {
	X      int `xml:"x,attr"`
	Y      int `xml:"y,attr"`
	Width  int `xml:"width,attr"`
	Height int `xml:"height,attr"`
}

type getVideoSourceConfigs2Response struct {
	Configurations []media2VideoSourceConfig `xml:"Configurations"`
}

type setVideoSourceConfig2Response struct {
	// Empty — success indicated by no fault.
}

type getVideoSourceConfigOptions2Response struct {
	Options media2VideoSourceConfigOptions `xml:"Options"`
}

type media2VideoSourceConfigOptions struct {
	BoundsRange             *media2IntRectangleRange `xml:"BoundsRange"`
	MaximumNumberOfProfiles int                      `xml:"MaximumNumberOfProfiles"`
}

type media2IntRectangleRange struct {
	XRange      media2IntRange `xml:"XRange"`
	YRange      media2IntRange `xml:"YRange"`
	WidthRange  media2IntRange `xml:"WidthRange"`
	HeightRange media2IntRange `xml:"HeightRange"`
}

type media2IntRange struct {
	Min int `xml:"Min"`
	Max int `xml:"Max"`
}
```

Add these fields to the `media2Body` struct:

```go
	GetVideoSourceConfigurationsResponse       *getVideoSourceConfigs2Response        `xml:"GetVideoSourceConfigurationsResponse"`
	SetVideoSourceConfigurationResponse        *setVideoSourceConfig2Response         `xml:"SetVideoSourceConfigurationResponse"`
	GetVideoSourceConfigurationOptionsResponse *getVideoSourceConfigOptions2Response  `xml:"GetVideoSourceConfigurationOptionsResponse"`
```

- [ ] **Step 3: Implement GetVideoSourceConfigurations2**

Append to `media2.go`:

```go
// GetVideoSourceConfigurations2 retrieves all video source configurations via Media2.
func GetVideoSourceConfigurations2(client *Client) ([]VideoSourceConfig, error) {
	body, err := doMedia2SOAP(client, `<tr2:GetVideoSourceConfigurations/>`)
	if err != nil {
		return nil, fmt.Errorf("media2 GetVideoSourceConfigurations: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("media2 GetVideoSourceConfigurations parse: %w", err)
	}
	if env.Body.Fault != nil {
		return nil, fmt.Errorf("media2 GetVideoSourceConfigurations SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetVideoSourceConfigurationsResponse == nil {
		return nil, fmt.Errorf("media2 GetVideoSourceConfigurations: empty response")
	}

	var configs []VideoSourceConfig
	for _, c := range env.Body.GetVideoSourceConfigurationsResponse.Configurations {
		cfg := VideoSourceConfig{
			Token:       c.Token,
			Name:        c.Name,
			SourceToken: c.SourceToken,
		}
		if c.Bounds != nil {
			cfg.Bounds = &IntRectangle{
				X:      c.Bounds.X,
				Y:      c.Bounds.Y,
				Width:  c.Bounds.Width,
				Height: c.Bounds.Height,
			}
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}
```

- [ ] **Step 4: Implement SetVideoSourceConfiguration2**

Append to `media2.go`:

```go
// SetVideoSourceConfiguration2 updates a video source configuration via Media2.
func SetVideoSourceConfiguration2(client *Client, cfg *VideoSourceConfig) error {
	boundsXML := ""
	if cfg.Bounds != nil {
		boundsXML = fmt.Sprintf(`<tt:Bounds x="%d" y="%d" width="%d" height="%d"/>`,
			cfg.Bounds.X, cfg.Bounds.Y, cfg.Bounds.Width, cfg.Bounds.Height)
	}

	reqBody := fmt.Sprintf(`<tr2:SetVideoSourceConfiguration>
      <tr2:Configuration token="%s">
        <tt:Name>%s</tt:Name>
        <tt:SourceToken>%s</tt:SourceToken>
        %s
      </tr2:Configuration>
    </tr2:SetVideoSourceConfiguration>`, cfg.Token, cfg.Name, cfg.SourceToken, boundsXML)

	body, err := doMedia2SOAP(client, reqBody)
	if err != nil {
		return fmt.Errorf("media2 SetVideoSourceConfiguration: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("media2 SetVideoSourceConfiguration parse: %w", err)
	}
	if env.Body.Fault != nil {
		return fmt.Errorf("media2 SetVideoSourceConfiguration SOAP fault: %s", env.Body.Fault.Faultstring)
	}

	return nil
}
```

Note: This function uses the `tt:` namespace prefix for child elements inside the Configuration. Update the `media2SOAP` function to include the `tt` namespace. Modify the `media2SOAP` function (line 70-79) to:

```go
func media2SOAP(innerBody string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tr2="http://www.onvif.org/ver20/media/wsdl"
            xmlns:tt="http://www.onvif.org/ver10/schema">
  <s:Header></s:Header>
  <s:Body>
    %s
  </s:Body>
</s:Envelope>`, innerBody)
}
```

- [ ] **Step 5: Implement GetVideoSourceConfigurationOptions2**

Append to `media2.go`:

```go
// GetVideoSourceConfigurationOptions2 retrieves the available options for a video source configuration via Media2.
func GetVideoSourceConfigurationOptions2(client *Client, configToken, profileToken string) (*VideoSourceConfigOptions, error) {
	inner := "<tr2:GetVideoSourceConfigurationOptions>"
	if configToken != "" {
		inner += fmt.Sprintf("<tr2:ConfigurationToken>%s</tr2:ConfigurationToken>", configToken)
	}
	if profileToken != "" {
		inner += fmt.Sprintf("<tr2:ProfileToken>%s</tr2:ProfileToken>", profileToken)
	}
	inner += "</tr2:GetVideoSourceConfigurationOptions>"

	body, err := doMedia2SOAP(client, inner)
	if err != nil {
		return nil, fmt.Errorf("media2 GetVideoSourceConfigurationOptions: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("media2 GetVideoSourceConfigurationOptions parse: %w", err)
	}
	if env.Body.Fault != nil {
		return nil, fmt.Errorf("media2 GetVideoSourceConfigurationOptions SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetVideoSourceConfigurationOptionsResponse == nil {
		return nil, fmt.Errorf("media2 GetVideoSourceConfigurationOptions: empty response")
	}

	opts := &VideoSourceConfigOptions{
		MaximumNumberOfProfiles: env.Body.GetVideoSourceConfigurationOptionsResponse.Options.MaximumNumberOfProfiles,
	}
	if br := env.Body.GetVideoSourceConfigurationOptionsResponse.Options.BoundsRange; br != nil {
		opts.BoundsRange = &IntRectangleRange{
			XRange:      Range{Min: br.XRange.Min, Max: br.XRange.Max},
			YRange:      Range{Min: br.YRange.Min, Max: br.YRange.Max},
			WidthRange:  Range{Min: br.WidthRange.Min, Max: br.WidthRange.Max},
			HeightRange: Range{Min: br.HeightRange.Min, Max: br.HeightRange.Max},
		}
	}

	return opts, nil
}
```

- [ ] **Step 6: Verify compilation**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-17 && go build ./internal/nvr/onvif/...`
Expected: No errors

- [ ] **Step 7: Commit**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-17
git add internal/nvr/onvif/media2.go internal/nvr/onvif/media_config.go
git commit -m "feat(onvif): add Media2 video source configuration operations"
```

---

### Task 3: Add Media2 Audio Source Configuration SOAP Types and Functions

**Files:**

- Modify: `internal/nvr/onvif/media2.go` (extend media2Body, add types + functions)

- [ ] **Step 1: Add SOAP response types for audio source configs in media2.go**

Append after the video source config types:

```go
// --- Media2 Audio Source Configuration response types ---

type media2AudioSourceConfig struct {
	Token       string `xml:"token,attr"`
	Name        string `xml:"Name"`
	SourceToken string `xml:"SourceToken"`
}

type getAudioSourceConfigs2Response struct {
	Configurations []media2AudioSourceConfig `xml:"Configurations"`
}

type setAudioSourceConfig2Response struct {
	// Empty — success indicated by no fault.
}
```

Add these fields to the `media2Body` struct:

```go
	GetAudioSourceConfigurationsResponse *getAudioSourceConfigs2Response `xml:"GetAudioSourceConfigurationsResponse"`
	SetAudioSourceConfigurationResponse  *setAudioSourceConfig2Response  `xml:"SetAudioSourceConfigurationResponse"`
```

- [ ] **Step 2: Implement GetAudioSourceConfigurations2**

Append to `media2.go`:

```go
// GetAudioSourceConfigurations2 retrieves all audio source configurations via Media2.
func GetAudioSourceConfigurations2(client *Client) ([]AudioSourceConfig, error) {
	body, err := doMedia2SOAP(client, `<tr2:GetAudioSourceConfigurations/>`)
	if err != nil {
		return nil, fmt.Errorf("media2 GetAudioSourceConfigurations: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("media2 GetAudioSourceConfigurations parse: %w", err)
	}
	if env.Body.Fault != nil {
		return nil, fmt.Errorf("media2 GetAudioSourceConfigurations SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetAudioSourceConfigurationsResponse == nil {
		return nil, fmt.Errorf("media2 GetAudioSourceConfigurations: empty response")
	}

	var configs []AudioSourceConfig
	for _, c := range env.Body.GetAudioSourceConfigurationsResponse.Configurations {
		configs = append(configs, AudioSourceConfig{
			Token:       c.Token,
			Name:        c.Name,
			SourceToken: c.SourceToken,
		})
	}
	return configs, nil
}
```

- [ ] **Step 3: Implement SetAudioSourceConfiguration2**

Append to `media2.go`:

```go
// SetAudioSourceConfiguration2 updates an audio source configuration via Media2.
func SetAudioSourceConfiguration2(client *Client, cfg *AudioSourceConfig) error {
	reqBody := fmt.Sprintf(`<tr2:SetAudioSourceConfiguration>
      <tr2:Configuration token="%s">
        <tt:Name>%s</tt:Name>
        <tt:SourceToken>%s</tt:SourceToken>
      </tr2:Configuration>
    </tr2:SetAudioSourceConfiguration>`, cfg.Token, cfg.Name, cfg.SourceToken)

	body, err := doMedia2SOAP(client, reqBody)
	if err != nil {
		return fmt.Errorf("media2 SetAudioSourceConfiguration: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("media2 SetAudioSourceConfiguration parse: %w", err)
	}
	if env.Body.Fault != nil {
		return fmt.Errorf("media2 SetAudioSourceConfiguration SOAP fault: %s", env.Body.Fault.Faultstring)
	}

	return nil
}
```

- [ ] **Step 4: Verify compilation**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-17 && go build ./internal/nvr/onvif/...`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-17
git add internal/nvr/onvif/media2.go
git commit -m "feat(onvif): add Media2 audio source configuration operations"
```

---

### Task 4: Add Public Wrapper Functions in media_config.go

**Files:**

- Modify: `internal/nvr/onvif/media_config.go` (append wrapper functions)

These wrappers follow the existing pattern: create a Client from xaddr/username/password, call the Media2 function, return the result. They are the public API that the HTTP handlers call.

- [ ] **Step 1: Add profile CRUD wrappers**

Append to `media_config.go`:

```go
// --- Media2 public wrappers ---

// CreateMedia2Profile creates a new media profile via Media2.
func CreateMedia2Profile(xaddr, username, password, name string) (*ProfileInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	token, retName, err := CreateProfile2(client, name)
	if err != nil {
		return nil, err
	}
	return &ProfileInfo{Token: token, Name: retName}, nil
}

// DeleteMedia2Profile deletes a media profile via Media2.
func DeleteMedia2Profile(xaddr, username, password, token string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}
	return DeleteProfile2(client, token)
}

// AddMedia2Configuration adds a configuration to a profile via Media2.
func AddMedia2Configuration(xaddr, username, password, profileToken, configType, configToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}
	return AddConfiguration2(client, profileToken, configType, configToken)
}

// RemoveMedia2Configuration removes a configuration from a profile via Media2.
func RemoveMedia2Configuration(xaddr, username, password, profileToken, configType, configToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}
	return RemoveConfiguration2(client, profileToken, configType, configToken)
}
```

- [ ] **Step 2: Add video source configuration wrappers**

Append to `media_config.go`:

```go
// GetVideoSourceConfigs returns all video source configurations via Media2.
func GetVideoSourceConfigs(xaddr, username, password string) ([]VideoSourceConfig, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}
	return GetVideoSourceConfigurations2(client)
}

// SetVideoSourceConfig updates a video source configuration via Media2.
func SetVideoSourceConfig(xaddr, username, password string, cfg *VideoSourceConfig) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}
	return SetVideoSourceConfiguration2(client, cfg)
}

// GetVideoSourceConfigOpts returns the available options for a video source configuration via Media2.
func GetVideoSourceConfigOpts(xaddr, username, password, configToken, profileToken string) (*VideoSourceConfigOptions, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}
	return GetVideoSourceConfigurationOptions2(client, configToken, profileToken)
}
```

- [ ] **Step 3: Add audio source configuration wrappers**

Append to `media_config.go`:

```go
// GetAudioSourceConfigs returns all audio source configurations via Media2.
func GetAudioSourceConfigs(xaddr, username, password string) ([]AudioSourceConfig, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}
	return GetAudioSourceConfigurations2(client)
}

// SetAudioSourceConfig updates an audio source configuration via Media2.
func SetAudioSourceConfig(xaddr, username, password string, cfg *AudioSourceConfig) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}
	return SetAudioSourceConfiguration2(client, cfg)
}
```

- [ ] **Step 4: Verify compilation**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-17 && go build ./internal/nvr/onvif/...`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-17
git add internal/nvr/onvif/media_config.go
git commit -m "feat(onvif): add public wrappers for Media2 operations"
```

---

### Task 5: Add API Handlers for Media2 Profile Operations

**Files:**

- Modify: `internal/nvr/api/cameras.go` (append 4 new handler methods after line 2189)

All handlers follow the existing pattern: get camera from DB, check ONVIF endpoint, decrypt password, call onvif function, return JSON.

- [ ] **Step 1: Add CreateMedia2Profile handler**

Append after the `GetVideoEncoderOptions` handler (after line 2189) in `cameras.go`:

```go
// CreateMedia2Profile creates a new ONVIF media profile via Media2.
//
//	POST /cameras/:id/media2/profiles
func (h *CameraHandler) CreateMedia2Profile(c *gin.Context) {
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
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	profile, err := onvif.CreateMedia2Profile(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), req.Name)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to create profile via Media2"})
		return
	}
	c.JSON(http.StatusCreated, profile)
}
```

- [ ] **Step 2: Add DeleteMedia2Profile handler**

```go
// DeleteMedia2Profile deletes an ONVIF media profile via Media2.
//
//	DELETE /cameras/:id/media2/profiles/:token
func (h *CameraHandler) DeleteMedia2Profile(c *gin.Context) {
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
	if err := onvif.DeleteMedia2Profile(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), token); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to delete profile via Media2"})
		return
	}
	c.Status(http.StatusNoContent)
}
```

- [ ] **Step 3: Add AddMedia2Configuration handler**

```go
// AddMedia2Configuration adds a configuration to an ONVIF media profile via Media2.
//
//	POST /cameras/:id/media2/profiles/:token/configurations
func (h *CameraHandler) AddMedia2Configuration(c *gin.Context) {
	id := c.Param("id")
	profileToken := c.Param("token")
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
		Type  string `json:"type" binding:"required"`
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type and token are required"})
		return
	}
	if err := onvif.AddMedia2Configuration(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), profileToken, req.Type, req.Token); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to add configuration to profile"})
		return
	}
	c.Status(http.StatusNoContent)
}
```

- [ ] **Step 4: Add RemoveMedia2Configuration handler**

```go
// RemoveMedia2Configuration removes a configuration from an ONVIF media profile via Media2.
//
//	DELETE /cameras/:id/media2/profiles/:token/configurations
func (h *CameraHandler) RemoveMedia2Configuration(c *gin.Context) {
	id := c.Param("id")
	profileToken := c.Param("token")
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
		Type  string `json:"type" binding:"required"`
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type and token are required"})
		return
	}
	if err := onvif.RemoveMedia2Configuration(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), profileToken, req.Type, req.Token); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to remove configuration from profile"})
		return
	}
	c.Status(http.StatusNoContent)
}
```

- [ ] **Step 5: Verify compilation**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-17 && go build ./internal/nvr/api/...`
Expected: Errors about unregistered routes (expected — routes added in Task 7)

- [ ] **Step 6: Commit**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-17
git add internal/nvr/api/cameras.go
git commit -m "feat(api): add Media2 profile CRUD handlers"
```

---

### Task 6: Add API Handlers for Video/Audio Source Configurations

**Files:**

- Modify: `internal/nvr/api/cameras.go` (append 5 new handler methods)

- [ ] **Step 1: Add GetVideoSourceConfigs handler**

Append to `cameras.go`:

```go
// GetVideoSourceConfigs returns all video source configurations for a camera via Media2.
//
//	GET /cameras/:id/media2/video-source-configs
func (h *CameraHandler) GetVideoSourceConfigs(c *gin.Context) {
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
	configs, err := onvif.GetVideoSourceConfigs(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword))
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get video source configurations"})
		return
	}
	c.JSON(http.StatusOK, configs)
}
```

- [ ] **Step 2: Add SetVideoSourceConfig handler**

```go
// SetVideoSourceConfig updates a video source configuration on a camera via Media2.
//
//	PUT /cameras/:id/media2/video-source-configs/:token
func (h *CameraHandler) SetVideoSourceConfig(c *gin.Context) {
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
	var cfg onvif.VideoSourceConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cfg.Token = token
	if err := onvif.SetVideoSourceConfig(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), &cfg); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to update video source configuration"})
		return
	}
	c.JSON(http.StatusOK, cfg)
}
```

- [ ] **Step 3: Add GetVideoSourceConfigOptions handler**

```go
// GetVideoSourceConfigOptions returns the options for a video source configuration via Media2.
//
//	GET /cameras/:id/media2/video-source-configs/:token/options
func (h *CameraHandler) GetVideoSourceConfigOptions(c *gin.Context) {
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
	profileToken := c.Query("profile_token")
	opts, err := onvif.GetVideoSourceConfigOpts(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), token, profileToken)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get video source configuration options"})
		return
	}
	c.JSON(http.StatusOK, opts)
}
```

- [ ] **Step 4: Add GetAudioSourceConfigs handler**

```go
// GetAudioSourceConfigs returns all audio source configurations for a camera via Media2.
//
//	GET /cameras/:id/media2/audio-source-configs
func (h *CameraHandler) GetAudioSourceConfigs(c *gin.Context) {
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
	configs, err := onvif.GetAudioSourceConfigs(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword))
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get audio source configurations"})
		return
	}
	c.JSON(http.StatusOK, configs)
}
```

- [ ] **Step 5: Add SetAudioSourceConfig handler**

```go
// SetAudioSourceConfig updates an audio source configuration on a camera via Media2.
//
//	PUT /cameras/:id/media2/audio-source-configs/:token
func (h *CameraHandler) SetAudioSourceConfig(c *gin.Context) {
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
	var cfg onvif.AudioSourceConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cfg.Token = token
	if err := onvif.SetAudioSourceConfig(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), &cfg); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to update audio source configuration"})
		return
	}
	c.JSON(http.StatusOK, cfg)
}
```

- [ ] **Step 6: Commit**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-17
git add internal/nvr/api/cameras.go
git commit -m "feat(api): add video/audio source configuration handlers"
```

---

### Task 7: Register New API Routes

**Files:**

- Modify: `internal/nvr/api/router.go:220-221` (add routes after existing media configuration block)

- [ ] **Step 1: Add all 9 Media2 routes**

Insert after line 220 (`protected.GET("/cameras/:id/media/video-encoder/:token/options", ...)`) and before line 222 (`// Device info.`):

```go

	// Media2 configuration.
	protected.POST("/cameras/:id/media2/profiles", cameraHandler.CreateMedia2Profile)
	protected.DELETE("/cameras/:id/media2/profiles/:token", cameraHandler.DeleteMedia2Profile)
	protected.POST("/cameras/:id/media2/profiles/:token/configurations", cameraHandler.AddMedia2Configuration)
	protected.DELETE("/cameras/:id/media2/profiles/:token/configurations", cameraHandler.RemoveMedia2Configuration)
	protected.GET("/cameras/:id/media2/video-source-configs", cameraHandler.GetVideoSourceConfigs)
	protected.PUT("/cameras/:id/media2/video-source-configs/:token", cameraHandler.SetVideoSourceConfig)
	protected.GET("/cameras/:id/media2/video-source-configs/:token/options", cameraHandler.GetVideoSourceConfigOptions)
	protected.GET("/cameras/:id/media2/audio-source-configs", cameraHandler.GetAudioSourceConfigs)
	protected.PUT("/cameras/:id/media2/audio-source-configs/:token", cameraHandler.SetAudioSourceConfig)
```

- [ ] **Step 2: Verify full build**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-17 && go build ./...`
Expected: No errors (ignore any unrelated warnings)

- [ ] **Step 3: Run existing tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-17 && go test ./internal/nvr/onvif/... -v -count=1 -run TestDiscoveryInitialState`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-17
git add internal/nvr/api/router.go
git commit -m "feat(api): register Media2 profile and source config routes"
```

---

### Task 8: Add XML Parsing Unit Tests

**Files:**

- Create: `internal/nvr/onvif/media2_test.go`

These tests verify that the SOAP XML response types parse correctly without needing a live ONVIF device.

- [ ] **Step 1: Write parsing tests**

Create `internal/nvr/onvif/media2_test.go`:

```go
package onvif

import (
	"encoding/xml"
	"testing"
)

func TestParseCreateProfileResponse(t *testing.T) {
	raw := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <CreateProfileResponse xmlns="http://www.onvif.org/ver20/media/wsdl">
      <Token>profile_tok_1</Token>
      <Name>TestProfile</Name>
    </CreateProfileResponse>
  </s:Body>
</s:Envelope>`

	var env media2Envelope
	if err := xml.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Body.Fault != nil {
		t.Fatalf("unexpected fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.CreateProfileResponse == nil {
		t.Fatal("expected CreateProfileResponse, got nil")
	}
	if env.Body.CreateProfileResponse.Token != "profile_tok_1" {
		t.Errorf("expected token 'profile_tok_1', got %q", env.Body.CreateProfileResponse.Token)
	}
	if env.Body.CreateProfileResponse.Name != "TestProfile" {
		t.Errorf("expected name 'TestProfile', got %q", env.Body.CreateProfileResponse.Name)
	}
}

func TestParseGetVideoSourceConfigurationsResponse(t *testing.T) {
	raw := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetVideoSourceConfigurationsResponse xmlns="http://www.onvif.org/ver20/media/wsdl">
      <Configurations token="vsc_1">
        <Name>VideoSrc1</Name>
        <SourceToken>src_001</SourceToken>
        <Bounds x="0" y="0" width="1920" height="1080"/>
      </Configurations>
      <Configurations token="vsc_2">
        <Name>VideoSrc2</Name>
        <SourceToken>src_002</SourceToken>
      </Configurations>
    </GetVideoSourceConfigurationsResponse>
  </s:Body>
</s:Envelope>`

	var env media2Envelope
	if err := xml.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Body.Fault != nil {
		t.Fatalf("unexpected fault: %s", env.Body.Fault.Faultstring)
	}
	resp := env.Body.GetVideoSourceConfigurationsResponse
	if resp == nil {
		t.Fatal("expected GetVideoSourceConfigurationsResponse, got nil")
	}
	if len(resp.Configurations) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(resp.Configurations))
	}

	c := resp.Configurations[0]
	if c.Token != "vsc_1" || c.Name != "VideoSrc1" || c.SourceToken != "src_001" {
		t.Errorf("unexpected first config: %+v", c)
	}
	if c.Bounds == nil {
		t.Fatal("expected bounds on first config")
	}
	if c.Bounds.Width != 1920 || c.Bounds.Height != 1080 {
		t.Errorf("unexpected bounds: %+v", c.Bounds)
	}

	c2 := resp.Configurations[1]
	if c2.Token != "vsc_2" {
		t.Errorf("unexpected second config token: %q", c2.Token)
	}
	if c2.Bounds != nil {
		t.Errorf("expected nil bounds on second config, got %+v", c2.Bounds)
	}
}

func TestParseGetVideoSourceConfigurationOptionsResponse(t *testing.T) {
	raw := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetVideoSourceConfigurationOptionsResponse xmlns="http://www.onvif.org/ver20/media/wsdl">
      <Options>
        <BoundsRange>
          <XRange><Min>0</Min><Max>0</Max></XRange>
          <YRange><Min>0</Min><Max>0</Max></YRange>
          <WidthRange><Min>1</Min><Max>1920</Max></WidthRange>
          <HeightRange><Min>1</Min><Max>1080</Max></HeightRange>
        </BoundsRange>
        <MaximumNumberOfProfiles>6</MaximumNumberOfProfiles>
      </Options>
    </GetVideoSourceConfigurationOptionsResponse>
  </s:Body>
</s:Envelope>`

	var env media2Envelope
	if err := xml.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	resp := env.Body.GetVideoSourceConfigurationOptionsResponse
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.Options.MaximumNumberOfProfiles != 6 {
		t.Errorf("expected 6 max profiles, got %d", resp.Options.MaximumNumberOfProfiles)
	}
	if resp.Options.BoundsRange == nil {
		t.Fatal("expected bounds range")
	}
	if resp.Options.BoundsRange.WidthRange.Max != 1920 {
		t.Errorf("expected max width 1920, got %d", resp.Options.BoundsRange.WidthRange.Max)
	}
}

func TestParseGetAudioSourceConfigurationsResponse(t *testing.T) {
	raw := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetAudioSourceConfigurationsResponse xmlns="http://www.onvif.org/ver20/media/wsdl">
      <Configurations token="asc_1">
        <Name>AudioInput1</Name>
        <SourceToken>audio_src_001</SourceToken>
      </Configurations>
    </GetAudioSourceConfigurationsResponse>
  </s:Body>
</s:Envelope>`

	var env media2Envelope
	if err := xml.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	resp := env.Body.GetAudioSourceConfigurationsResponse
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if len(resp.Configurations) != 1 {
		t.Fatalf("expected 1 config, got %d", len(resp.Configurations))
	}
	c := resp.Configurations[0]
	if c.Token != "asc_1" || c.Name != "AudioInput1" || c.SourceToken != "audio_src_001" {
		t.Errorf("unexpected config: %+v", c)
	}
}

func TestParseSOAPFault(t *testing.T) {
	raw := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <s:Fault>
      <faultstring>Action not supported</faultstring>
    </s:Fault>
  </s:Body>
</s:Envelope>`

	var env media2Envelope
	if err := xml.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Body.Fault == nil {
		t.Fatal("expected fault, got nil")
	}
	if env.Body.Fault.Faultstring != "Action not supported" {
		t.Errorf("unexpected faultstring: %q", env.Body.Fault.Faultstring)
	}
}
```

- [ ] **Step 2: Run the tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-17 && go test ./internal/nvr/onvif/... -v -count=1 -run "TestParse"`
Expected: All 5 tests PASS

- [ ] **Step 3: Commit**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-17
git add internal/nvr/onvif/media2_test.go
git commit -m "test: add XML parsing tests for Media2 SOAP response types"
```

---

### Task 9: Final Build Verification and Push

**Files:** None (verification only)

- [ ] **Step 1: Full build**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-17 && go build ./...`
Expected: No errors

- [ ] **Step 2: Run all onvif tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-17 && go test ./internal/nvr/onvif/... -v -count=1 -run "TestParse|TestDiscoveryInitialState"`
Expected: All tests PASS

- [ ] **Step 3: Push branch**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/kai-17
git push -u origin feat/kai-17-media-profile-management
```

- [ ] **Step 4: Create PR**

```bash
gh pr create --title "feat: complete ONVIF Media2 profile management" --body "$(cat <<'EOF'
## Summary
- Add Media2 profile CRUD operations (CreateProfile, DeleteProfile, AddConfiguration, RemoveConfiguration)
- Add Media2 video source configuration operations (Get, Set, GetOptions)
- Add Media2 audio source configuration operations (Get, Set)
- Expose all 9 operations via new `/media2/` API endpoints
- Add XML parsing unit tests for all new SOAP response types

## Test plan
- [ ] XML parsing tests pass (`go test ./internal/nvr/onvif/... -run TestParse`)
- [ ] Full build passes (`go build ./...`)
- [ ] Manual test: create/delete Media2 profile on real ONVIF camera
- [ ] Manual test: get/set video source configuration on real camera
- [ ] Manual test: get/set audio source configuration on real camera

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```
