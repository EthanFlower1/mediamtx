# KAI-21: ONVIF Multicast Streaming Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add opt-in per-camera multicast streaming via ONVIF, so the NVR can receive multicast streams instead of unicast when enabled.

**Architecture:** New ONVIF multicast functions in `internal/nvr/onvif/multicast.go` negotiate multicast transport with cameras. Four new DB columns on the `cameras` table store multicast config. Two new API endpoints (GET/PUT) on `/cameras/:id/multicast` expose configuration. When enabled, the existing YAML source-writing logic switches to the multicast URI.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), Gin HTTP router, ONVIF SOAP, MediaMTX YAML config

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `internal/nvr/onvif/multicast.go` | ONVIF SOAP functions: GetStreamUriMulticast, GetMulticastConfig, SetMulticastConfig |
| Create | `internal/nvr/onvif/multicast_test.go` | Unit tests for multicast validation and SOAP parsing |
| Modify | `internal/nvr/db/migrations.go` | Migration 31: add multicast columns to cameras table |
| Modify | `internal/nvr/db/cameras.go` | Add multicast fields to Camera struct and update scan/update queries |
| Modify | `internal/nvr/api/cameras.go` | Add GetMulticast and UpdateMulticast handlers |
| Modify | `internal/nvr/api/router.go` | Register GET/PUT `/cameras/:id/multicast` routes |

---

### Task 1: ONVIF Multicast SOAP Functions

**Files:**
- Create: `internal/nvr/onvif/multicast.go`
- Create: `internal/nvr/onvif/multicast_test.go`

- [ ] **Step 1: Write the test for ValidateMulticastAddress**

```go
// internal/nvr/onvif/multicast_test.go
package onvif

import "testing"

func TestValidateMulticastAddress(t *testing.T) {
	tests := []struct {
		addr    string
		wantErr bool
	}{
		{"239.1.1.10", false},
		{"224.0.0.1", false},
		{"239.255.255.255", false},
		{"192.168.1.1", true},    // not multicast
		{"223.255.255.255", true}, // just below range
		{"240.0.0.0", true},      // just above range
		{"", true},               // empty
		{"not-an-ip", true},      // invalid
		{"256.1.1.1", true},      // invalid octet
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			err := ValidateMulticastAddress(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMulticastAddress(%q) error = %v, wantErr %v", tt.addr, err, tt.wantErr)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/onvif && go test -run TestValidateMulticastAddress -v`
Expected: FAIL — `ValidateMulticastAddress` not defined

- [ ] **Step 3: Write the test for ParseMulticastConfigResponse**

```go
// append to internal/nvr/onvif/multicast_test.go

func TestParseMulticastConfigResponse(t *testing.T) {
	xmlResp := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:trt="http://www.onvif.org/ver10/media/wsdl"
            xmlns:tt="http://www.onvif.org/ver10/schema">
  <s:Body>
    <trt:GetVideoEncoderConfigurationResponse>
      <trt:Configuration token="encoder_1">
        <tt:Name>Main Encoder</tt:Name>
        <tt:Multicast>
          <tt:Address>
            <tt:Type>IPv4</tt:Type>
            <tt:IPv4Address>239.1.1.10</tt:IPv4Address>
          </tt:Address>
          <tt:Port>5004</tt:Port>
          <tt:TTL>5</tt:TTL>
          <tt:AutoStart>false</tt:AutoStart>
        </tt:Multicast>
      </trt:Configuration>
    </trt:GetVideoEncoderConfigurationResponse>
  </s:Body>
</s:Envelope>`)

	cfg, err := parseMulticastConfigResponse(xmlResp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Address != "239.1.1.10" {
		t.Errorf("address = %q, want %q", cfg.Address, "239.1.1.10")
	}
	if cfg.Port != 5004 {
		t.Errorf("port = %d, want %d", cfg.Port, 5004)
	}
	if cfg.TTL != 5 {
		t.Errorf("ttl = %d, want %d", cfg.TTL, 5)
	}
}

func TestParseMulticastConfigResponse_Fault(t *testing.T) {
	xmlResp := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <s:Fault>
      <faultstring>not supported</faultstring>
    </s:Fault>
  </s:Body>
</s:Envelope>`)

	_, err := parseMulticastConfigResponse(xmlResp)
	if err == nil {
		t.Fatal("expected error for SOAP fault, got nil")
	}
}
```

- [ ] **Step 4: Write the test for ParseMulticastStreamUriResponse**

```go
// append to internal/nvr/onvif/multicast_test.go

func TestParseMulticastStreamUriResponse(t *testing.T) {
	xmlResp := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:trt="http://www.onvif.org/ver10/media/wsdl"
            xmlns:tt="http://www.onvif.org/ver10/schema">
  <s:Body>
    <trt:GetStreamUriResponse>
      <trt:MediaUri>
        <tt:Uri>rtp://239.1.1.10:5004</tt:Uri>
      </trt:MediaUri>
    </trt:GetStreamUriResponse>
  </s:Body>
</s:Envelope>`)

	uri, err := parseMedia1StreamUriResponse(xmlResp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uri != "rtp://239.1.1.10:5004" {
		t.Errorf("uri = %q, want %q", uri, "rtp://239.1.1.10:5004")
	}
}
```

- [ ] **Step 5: Run all tests to verify they fail**

Run: `cd internal/nvr/onvif && go test -run "TestValidateMulticast|TestParseMulticast|TestParseMedia1Stream" -v`
Expected: FAIL — functions not defined

- [ ] **Step 6: Implement multicast.go**

```go
// internal/nvr/onvif/multicast.go
package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// MulticastConfig holds a camera's multicast streaming settings.
type MulticastConfig struct {
	Address   string `json:"address"`
	Port      int    `json:"port"`
	TTL       int    `json:"ttl"`
	AutoStart bool   `json:"auto_start"`
}

// ValidateMulticastAddress checks that addr is a valid IPv4 multicast address
// in the range 224.0.0.0–239.255.255.255.
func ValidateMulticastAddress(addr string) error {
	if addr == "" {
		return fmt.Errorf("multicast address is empty")
	}
	ip := net.ParseIP(addr)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", addr)
	}
	ip = ip.To4()
	if ip == nil {
		return fmt.Errorf("not an IPv4 address: %s", addr)
	}
	if !ip.IsMulticast() {
		return fmt.Errorf("address %s is not in the multicast range (224.0.0.0–239.255.255.255)", addr)
	}
	return nil
}

// --- SOAP response types for multicast config ---

type multicastConfigEnvelope struct {
	XMLName xml.Name             `xml:"Envelope"`
	Body    multicastConfigBody  `xml:"Body"`
}

type multicastConfigBody struct {
	GetVideoEncoderConfigurationResponse *vecResponse   `xml:"GetVideoEncoderConfigurationResponse"`
	Fault                                *soapFault     `xml:"Fault"`
}

type soapFault struct {
	Faultstring string `xml:"faultstring"`
}

type vecResponse struct {
	Configuration vecConfiguration `xml:"Configuration"`
}

type vecConfiguration struct {
	Token     string          `xml:"token,attr"`
	Name      string          `xml:"Name"`
	Multicast *multicastBlock `xml:"Multicast"`
}

type multicastBlock struct {
	Address   multicastAddress `xml:"Address"`
	Port      int              `xml:"Port"`
	TTL       int              `xml:"TTL"`
	AutoStart bool             `xml:"AutoStart"`
}

type multicastAddress struct {
	Type        string `xml:"Type"`
	IPv4Address string `xml:"IPv4Address"`
}

// parseMulticastConfigResponse parses a GetVideoEncoderConfiguration SOAP
// response and extracts the multicast settings.
func parseMulticastConfigResponse(data []byte) (*MulticastConfig, error) {
	var env multicastConfigEnvelope
	if err := xml.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("parse multicast config: %w", err)
	}
	if env.Body.Fault != nil {
		return nil, fmt.Errorf("SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	resp := env.Body.GetVideoEncoderConfigurationResponse
	if resp == nil {
		return nil, fmt.Errorf("empty GetVideoEncoderConfigurationResponse")
	}
	if resp.Configuration.Multicast == nil {
		return nil, fmt.Errorf("camera does not report multicast configuration")
	}
	mc := resp.Configuration.Multicast
	return &MulticastConfig{
		Address:   mc.Address.IPv4Address,
		Port:      mc.Port,
		TTL:       mc.TTL,
		AutoStart: mc.AutoStart,
	}, nil
}

// --- SOAP response types for Media1 GetStreamUri ---

type media1StreamUriEnvelope struct {
	XMLName xml.Name              `xml:"Envelope"`
	Body    media1StreamUriBody   `xml:"Body"`
}

type media1StreamUriBody struct {
	GetStreamUriResponse *media1StreamUriResponse `xml:"GetStreamUriResponse"`
	Fault                *soapFault               `xml:"Fault"`
}

type media1StreamUriResponse struct {
	MediaUri media1MediaUri `xml:"MediaUri"`
}

type media1MediaUri struct {
	Uri string `xml:"Uri"`
}

// parseMedia1StreamUriResponse parses a Media1 GetStreamUri SOAP response.
func parseMedia1StreamUriResponse(data []byte) (string, error) {
	var env media1StreamUriEnvelope
	if err := xml.Unmarshal(data, &env); err != nil {
		return "", fmt.Errorf("parse stream URI: %w", err)
	}
	if env.Body.Fault != nil {
		return "", fmt.Errorf("SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetStreamUriResponse == nil {
		return "", fmt.Errorf("empty GetStreamUriResponse")
	}
	return strings.TrimSpace(env.Body.GetStreamUriResponse.MediaUri.Uri), nil
}

// --- Media1 SOAP helper ---

func media1SOAP(innerBody string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:trt="http://www.onvif.org/ver10/media/wsdl"
            xmlns:tt="http://www.onvif.org/ver10/schema">
  <s:Header></s:Header>
  <s:Body>
    %s
  </s:Body>
</s:Envelope>`, innerBody)
}

func doMedia1SOAP(client *Client, body string) ([]byte, error) {
	mediaURL := client.ServiceURL("media")
	if mediaURL == "" {
		return nil, fmt.Errorf("device does not support Media service")
	}

	soapBody := media1SOAP(body)
	if client.Username != "" {
		soapBody = injectWSSecurity(soapBody, client.Username, client.Password)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mediaURL, strings.NewReader(soapBody))
	if err != nil {
		return nil, fmt.Errorf("create media1 request: %w", err)
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("media1 http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("media1 read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("media1 SOAP fault (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	return respBody, nil
}

// --- Public functions ---

// GetMulticastConfig retrieves the camera's current multicast settings
// from the video encoder configuration for the given profile token.
func GetMulticastConfig(xaddr, username, password, profileToken string) (*MulticastConfig, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("GetMulticastConfig: connect: %w", err)
	}

	// Get the video encoder configuration token from the profile.
	ctx := context.Background()
	profiles, err := client.Dev.GetProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetMulticastConfig: get profiles: %w", err)
	}

	var encoderToken string
	for _, p := range profiles {
		if p.Token == profileToken && p.VideoEncoderConfiguration != nil {
			encoderToken = p.VideoEncoderConfiguration.Token
			break
		}
	}
	if encoderToken == "" {
		return nil, fmt.Errorf("GetMulticastConfig: no video encoder found for profile %s", profileToken)
	}

	// Fetch the full encoder config via SOAP to get multicast block.
	reqBody := fmt.Sprintf(`<trt:GetVideoEncoderConfiguration>
      <trt:ConfigurationToken>%s</trt:ConfigurationToken>
    </trt:GetVideoEncoderConfiguration>`, xmlEscape(encoderToken))

	data, err := doMedia1SOAP(client, reqBody)
	if err != nil {
		return nil, fmt.Errorf("GetMulticastConfig: %w", err)
	}

	return parseMulticastConfigResponse(data)
}

// SetMulticastConfig updates the camera's multicast address, port, and TTL
// on the video encoder configuration for the given profile.
func SetMulticastConfig(xaddr, username, password, profileToken string, cfg *MulticastConfig) error {
	if err := ValidateMulticastAddress(cfg.Address); err != nil {
		return fmt.Errorf("SetMulticastConfig: %w", err)
	}

	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return fmt.Errorf("SetMulticastConfig: connect: %w", err)
	}

	// Look up the encoder token from the profile.
	ctx := context.Background()
	profiles, err := client.Dev.GetProfiles(ctx)
	if err != nil {
		return fmt.Errorf("SetMulticastConfig: get profiles: %w", err)
	}

	var encoderToken, encoderName, encoding string
	var width, height int
	var quality float64
	for _, p := range profiles {
		if p.Token == profileToken && p.VideoEncoderConfiguration != nil {
			vec := p.VideoEncoderConfiguration
			encoderToken = vec.Token
			encoderName = vec.Name
			encoding = vec.Encoding
			quality = vec.Quality
			if vec.Resolution != nil {
				width = vec.Resolution.Width
				height = vec.Resolution.Height
			}
			break
		}
	}
	if encoderToken == "" {
		return fmt.Errorf("SetMulticastConfig: no video encoder found for profile %s", profileToken)
	}

	reqBody := fmt.Sprintf(`<trt:SetVideoEncoderConfiguration>
      <trt:Configuration token="%s">
        <tt:Name>%s</tt:Name>
        <tt:Encoding>%s</tt:Encoding>
        <tt:Resolution>
          <tt:Width>%d</tt:Width>
          <tt:Height>%d</tt:Height>
        </tt:Resolution>
        <tt:Quality>%.1f</tt:Quality>
        <tt:Multicast>
          <tt:Address>
            <tt:Type>IPv4</tt:Type>
            <tt:IPv4Address>%s</tt:IPv4Address>
          </tt:Address>
          <tt:Port>%d</tt:Port>
          <tt:TTL>%d</tt:TTL>
          <tt:AutoStart>false</tt:AutoStart>
        </tt:Multicast>
      </trt:Configuration>
      <trt:ForcePersistence>true</trt:ForcePersistence>
    </trt:SetVideoEncoderConfiguration>`,
		xmlEscape(encoderToken), xmlEscape(encoderName), xmlEscape(encoding),
		width, height, quality,
		xmlEscape(cfg.Address), cfg.Port, cfg.TTL)

	data, err := doMedia1SOAP(client, reqBody)
	if err != nil {
		return fmt.Errorf("SetMulticastConfig: %w", err)
	}

	// Check for SOAP fault in response.
	type faultEnvelope struct {
		XMLName xml.Name `xml:"Envelope"`
		Body    struct {
			Fault *soapFault `xml:"Fault"`
		} `xml:"Body"`
	}
	var env faultEnvelope
	if err := xml.Unmarshal(data, &env); err == nil && env.Body.Fault != nil {
		return fmt.Errorf("SetMulticastConfig: SOAP fault: %s", env.Body.Fault.Faultstring)
	}

	return nil
}

// GetStreamUriMulticast retrieves the multicast stream URI for a profile.
// It tries Media2 first, then falls back to Media1.
func GetStreamUriMulticast(xaddr, username, password, profileToken string) (string, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return "", fmt.Errorf("GetStreamUriMulticast: connect: %w", err)
	}

	// Try Media2 first.
	if client.HasService("media2") {
		reqBody := fmt.Sprintf(`<tr2:GetStreamUri>
          <tr2:Protocol>RtspMulticast</tr2:Protocol>
          <tr2:ProfileToken>%s</tr2:ProfileToken>
        </tr2:GetStreamUri>`, xmlEscape(profileToken))

		data, err := doMedia2SOAP(client, reqBody)
		if err == nil {
			var env media2Envelope
			if xmlErr := xml.Unmarshal(data, &env); xmlErr == nil &&
				env.Body.Fault == nil &&
				env.Body.GetStreamUriResponse != nil {
				uri := strings.TrimSpace(env.Body.GetStreamUriResponse.Uri)
				if uri != "" {
					return uri, nil
				}
			}
		}
		// Fall through to Media1 on failure.
	}

	// Media1 fallback: use StreamSetup with RTP-Multicast.
	reqBody := fmt.Sprintf(`<trt:GetStreamUri>
      <trt:StreamSetup>
        <tt:Stream>RTP-Multicast</tt:Stream>
        <tt:Transport>
          <tt:Protocol>UDP</tt:Protocol>
        </tt:Transport>
      </trt:StreamSetup>
      <trt:ProfileToken>%s</trt:ProfileToken>
    </trt:GetStreamUri>`, xmlEscape(profileToken))

	data, err := doMedia1SOAP(client, reqBody)
	if err != nil {
		return "", fmt.Errorf("GetStreamUriMulticast: %w", err)
	}

	return parseMedia1StreamUriResponse(data)
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd internal/nvr/onvif && go test -run "TestValidateMulticast|TestParseMulticast|TestParseMedia1Stream" -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/nvr/onvif/multicast.go internal/nvr/onvif/multicast_test.go
git commit -m "feat(onvif): add multicast SOAP functions and validation"
```

---

### Task 2: Database Migration and Camera Model

**Files:**
- Modify: `internal/nvr/db/migrations.go` (after line 454, append migration 31)
- Modify: `internal/nvr/db/cameras.go` (Camera struct, scan, update queries)

- [ ] **Step 1: Add migration 31**

Append to the `migrations` slice in `internal/nvr/db/migrations.go`, before the closing `}`:

```go
	{
		version: 31,
		sql: `
		ALTER TABLE cameras ADD COLUMN multicast_enabled INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE cameras ADD COLUMN multicast_address TEXT NOT NULL DEFAULT '';
		ALTER TABLE cameras ADD COLUMN multicast_port INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE cameras ADD COLUMN multicast_ttl INTEGER NOT NULL DEFAULT 5;
		`,
	},
```

- [ ] **Step 2: Add multicast fields to Camera struct**

In `internal/nvr/db/cameras.go`, add these fields to the `Camera` struct after `QuotaCriticalPercent`:

```go
	MulticastEnabled  bool   `json:"multicast_enabled"`
	MulticastAddress  string `json:"multicast_address"`
	MulticastPort     int    `json:"multicast_port"`
	MulticastTTL      int    `json:"multicast_ttl"`
```

- [ ] **Step 3: Update the scan query in GetCamera**

Find the `GetCamera` function's `QueryRow` and `Scan` call. Add the four new columns to the SELECT list and Scan arguments:

Add to SELECT: `, multicast_enabled, multicast_address, multicast_port, multicast_ttl`

Add to Scan: `, &cam.MulticastEnabled, &cam.MulticastAddress, &cam.MulticastPort, &cam.MulticastTTL`

- [ ] **Step 4: Update the scan in ListCameras**

Same changes as Step 3, applied to the `ListCameras` function's SELECT and Scan.

- [ ] **Step 5: Update UpdateCamera to persist multicast fields**

In the `UpdateCamera` function, add the four multicast columns to the UPDATE SET clause and parameter list:

Add to SET: `, multicast_enabled = ?, multicast_address = ?, multicast_port = ?, multicast_ttl = ?`

Add to args: `, cam.MulticastEnabled, cam.MulticastAddress, cam.MulticastPort, cam.MulticastTTL`

- [ ] **Step 6: Add UpdateCameraMulticast convenience method**

Add a new method to `internal/nvr/db/cameras.go`:

```go
// UpdateCameraMulticast updates only the multicast configuration fields.
func (d *DB) UpdateCameraMulticast(id string, enabled bool, address string, port, ttl int) error {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := d.Exec(`
		UPDATE cameras
		SET multicast_enabled = ?, multicast_address = ?, multicast_port = ?, multicast_ttl = ?, updated_at = ?
		WHERE id = ?`,
		enabled, address, port, ttl, now, id)
	if err != nil {
		return fmt.Errorf("update multicast config: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 7: Verify the project compiles**

Run: `go build ./...`
Expected: PASS (no compilation errors)

- [ ] **Step 8: Commit**

```bash
git add internal/nvr/db/migrations.go internal/nvr/db/cameras.go
git commit -m "feat(db): add multicast config columns to cameras table"
```

---

### Task 3: API Endpoints — GetMulticast and UpdateMulticast

**Files:**
- Modify: `internal/nvr/api/cameras.go` (add handler methods)
- Modify: `internal/nvr/api/router.go` (register routes)

- [ ] **Step 1: Add the multicast request/response types**

Add to `internal/nvr/api/cameras.go` (near the other request types):

```go
// multicastResponse is the JSON response for GET /cameras/:id/multicast.
type multicastResponse struct {
	Supported bool   `json:"supported"`
	Enabled   bool   `json:"enabled"`
	Address   string `json:"address"`
	Port      int    `json:"port"`
	TTL       int    `json:"ttl"`
}

// multicastRequest is the JSON body for PUT /cameras/:id/multicast.
type multicastRequest struct {
	Enabled bool   `json:"enabled"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	TTL     int    `json:"ttl"`
}
```

- [ ] **Step 2: Implement GetMulticast handler**

```go
// GetMulticast returns the multicast configuration for a camera and probes
// the device to check whether it supports multicast streaming.
func (h *CameraHandler) GetMulticast(c *gin.Context) {
	id := c.Param("id")
	cam, err := h.DB.GetCamera(id)
	if err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "fetch camera", err)
		return
	}

	resp := multicastResponse{
		Enabled: cam.MulticastEnabled,
		Address: cam.MulticastAddress,
		Port:    cam.MulticastPort,
		TTL:     cam.MulticastTTL,
	}

	// Probe device for multicast support if ONVIF is configured.
	if cam.ONVIFEndpoint != "" && cam.ONVIFProfileToken != "" {
		password := h.decryptPassword(cam.ONVIFPassword)
		cfg, err := onvif.GetMulticastConfig(cam.ONVIFEndpoint, cam.ONVIFUsername, password, cam.ONVIFProfileToken)
		if err == nil && cfg != nil {
			resp.Supported = true
			// If the camera has never been configured locally, show the camera's defaults.
			if resp.Address == "" {
				resp.Address = cfg.Address
				resp.Port = cfg.Port
				resp.TTL = cfg.TTL
			}
		}
	}

	c.JSON(http.StatusOK, resp)
}
```

- [ ] **Step 3: Implement UpdateMulticast handler**

```go
// UpdateMulticast enables or disables multicast streaming for a camera.
// When enabling, it configures the camera via ONVIF, retrieves the multicast
// stream URI, and updates the MediaMTX source. When disabling, it reverts
// to the unicast stream URI.
func (h *CameraHandler) UpdateMulticast(c *gin.Context) {
	id := c.Param("id")
	cam, err := h.DB.GetCamera(id)
	if err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "fetch camera", err)
		return
	}

	var req multicastRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if req.Enabled {
		// Validate prerequisites.
		if cam.ONVIFEndpoint == "" || cam.ONVIFProfileToken == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "camera must have ONVIF endpoint and profile token configured"})
			return
		}
		if err := onvif.ValidateMulticastAddress(req.Address); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.Port < 1024 || req.Port > 65535 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "port must be between 1024 and 65535"})
			return
		}
		if req.TTL < 1 || req.TTL > 255 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "TTL must be between 1 and 255"})
			return
		}

		password := h.decryptPassword(cam.ONVIFPassword)

		// Check camera supports multicast.
		_, err := onvif.GetMulticastConfig(cam.ONVIFEndpoint, cam.ONVIFUsername, password, cam.ONVIFProfileToken)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "camera does not support multicast: " + err.Error()})
			return
		}

		// Configure multicast on the camera.
		mcCfg := &onvif.MulticastConfig{
			Address: req.Address,
			Port:    req.Port,
			TTL:     req.TTL,
		}
		if err := onvif.SetMulticastConfig(cam.ONVIFEndpoint, cam.ONVIFUsername, password, cam.ONVIFProfileToken, mcCfg); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to configure multicast on camera: " + err.Error()})
			return
		}

		// Get the multicast stream URI.
		multicastURI, err := onvif.GetStreamUriMulticast(cam.ONVIFEndpoint, cam.ONVIFUsername, password, cam.ONVIFProfileToken)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to get multicast stream URI: " + err.Error()})
			return
		}

		// Update MediaMTX source to multicast URI.
		stablePath := cam.MediaMTXPath
		yamlConfig := map[string]interface{}{
			"source": multicastURI,
		}
		if err := h.YAMLWriter.AddPath(stablePath, yamlConfig); err != nil {
			apiError(c, http.StatusInternalServerError, "failed to update stream config", err)
			return
		}

		nvrLogInfo("cameras", fmt.Sprintf("Enabled multicast for camera %q: addr=%s port=%d ttl=%d uri=%s", cam.Name, req.Address, req.Port, req.TTL, multicastURI))
	} else {
		// Disable: revert to unicast.
		stablePath := cam.MediaMTXPath
		yamlConfig := map[string]interface{}{
			"source": cam.RTSPURL,
		}
		if err := h.YAMLWriter.AddPath(stablePath, yamlConfig); err != nil {
			apiError(c, http.StatusInternalServerError, "failed to revert stream config", err)
			return
		}

		nvrLogInfo("cameras", fmt.Sprintf("Disabled multicast for camera %q, reverted to unicast", cam.Name))
	}

	// Persist multicast settings in DB.
	if err := h.DB.UpdateCameraMulticast(id, req.Enabled, req.Address, req.Port, req.TTL); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to save multicast config", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"enabled": req.Enabled,
		"address": req.Address,
		"port":    req.Port,
		"ttl":     req.TTL,
	})
}
```

- [ ] **Step 4: Register routes in router.go**

In `internal/nvr/api/router.go`, add these two lines in the `// Media configuration` section (after line 220):

```go
	// Multicast streaming.
	protected.GET("/cameras/:id/multicast", cameraHandler.GetMulticast)
	protected.PUT("/cameras/:id/multicast", cameraHandler.UpdateMulticast)
```

- [ ] **Step 5: Verify the project compiles**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/api/cameras.go internal/nvr/api/router.go
git commit -m "feat(api): add GET/PUT multicast endpoints for per-camera multicast config"
```

---

### Task 4: Stream Source Switching on Refresh

**Files:**
- Modify: `internal/nvr/api/cameras.go` (RefreshCapabilities handler, around line 826)

When a camera is refreshed and multicast is enabled, the stream source should use the multicast URI instead of the unicast URI.

- [ ] **Step 1: Update RefreshCapabilities to respect multicast**

In `internal/nvr/api/cameras.go`, in the `RefreshCapabilities` handler, after the streams are recreated (after line 826), add multicast-aware source selection:

Find this block (around line 828):
```go
	nvrLogInfo("cameras", fmt.Sprintf("Refreshed capabilities for camera %q: %d profiles found, streams recreated", cam.Name, len(result.Profiles)))
```

Insert before it:

```go
	// If multicast is enabled, update the YAML source to the multicast URI.
	// Otherwise the refresh would leave the source pointing at whatever was
	// last set (which might be stale after a profile change).
	if cam.MulticastEnabled && cam.ONVIFProfileToken != "" {
		password := h.decryptPassword(cam.ONVIFPassword)
		multicastURI, err := onvif.GetStreamUriMulticast(cam.ONVIFEndpoint, cam.ONVIFUsername, password, cam.ONVIFProfileToken)
		if err != nil {
			nvrLogWarn("cameras", fmt.Sprintf("multicast enabled but failed to get multicast URI for camera %q: %v", cam.Name, err))
		} else {
			stablePath := cam.MediaMTXPath
			yamlConfig := map[string]interface{}{
				"source": multicastURI,
			}
			if err := h.YAMLWriter.AddPath(stablePath, yamlConfig); err != nil {
				nvrLogWarn("cameras", fmt.Sprintf("failed to update multicast source for camera %q: %v", cam.Name, err))
			} else {
				nvrLogInfo("cameras", fmt.Sprintf("Refreshed multicast source for camera %q: %s", cam.Name, multicastURI))
			}
		}
	}
```

- [ ] **Step 2: Verify the project compiles**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/cameras.go
git commit -m "feat: refresh multicast stream URI when camera capabilities are refreshed"
```

---

### Task 5: Final Verification

- [ ] **Step 1: Run all ONVIF tests**

Run: `cd internal/nvr/onvif && go test ./... -v`
Expected: PASS

- [ ] **Step 2: Run full project build**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 3: Run full test suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: PASS (or pre-existing failures only)

- [ ] **Step 4: Commit any remaining fixes**

If any test fixes were needed, commit them:
```bash
git add -A
git commit -m "fix: address test issues from multicast implementation"
```
