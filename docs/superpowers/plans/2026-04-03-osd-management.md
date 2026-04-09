# KAI-25: ONVIF Profile T OSD Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement CRUD operations for ONVIF Profile T On-Screen Display (OSD) overlays — text and image — via the Media2 service, exposed through REST API endpoints.

**Architecture:** New `osd.go` in `internal/nvr/onvif/` with custom SOAP XML types and 5 public functions that reuse the existing `doMedia2SOAP()` helper from `media2.go`. Five new handler methods on `CameraHandler` in `cameras.go` wired via `router.go`.

**Tech Stack:** Go, ONVIF Media2 SOAP (tr2 namespace), Gin HTTP router, encoding/xml

---

## File Structure

| File                          | Action | Responsibility                                                                                         |
| ----------------------------- | ------ | ------------------------------------------------------------------------------------------------------ |
| `internal/nvr/onvif/osd.go`   | Create | OSD SOAP types, XML parsing, 5 public functions (GetOSDs, GetOSDOptions, CreateOSD, SetOSD, DeleteOSD) |
| `internal/nvr/api/cameras.go` | Modify | Add 5 handler methods for OSD endpoints                                                                |
| `internal/nvr/api/router.go`  | Modify | Register 5 OSD routes under protected group                                                            |

---

### Task 1: OSD SOAP Types and GetOSDs

**Files:**

- Create: `internal/nvr/onvif/osd.go`

This task creates the new file with all XML response types, the Go data types, the error sentinel, and the `GetOSDs` function.

- [ ] **Step 1: Create `osd.go` with types, error, and GetOSDs**

Create file `internal/nvr/onvif/osd.go`:

```go
package onvif

import (
	"encoding/xml"
	"fmt"
)

// ErrOSDNotSupported is returned when the camera does not advertise the Media2 service.
var ErrOSDNotSupported = fmt.Errorf("camera does not support OSD via Media2 service")

// --- Public types ---

// OSD represents an on-screen display overlay on a video source.
type OSD struct {
	Token            string      `json:"token"`
	VideoSourceToken string      `json:"video_source_token"`
	Type             string      `json:"type"`
	Position         OSDPosition `json:"position"`
	TextString       *OSDText    `json:"text_string,omitempty"`
	Image            *OSDImage   `json:"image,omitempty"`
}

// OSDPosition describes the placement of an OSD overlay.
type OSDPosition struct {
	Type string   `json:"type"`
	X    *float64 `json:"x,omitempty"`
	Y    *float64 `json:"y,omitempty"`
}

// OSDText describes the text content of a text OSD overlay.
type OSDText struct {
	IsPersistentText bool   `json:"is_persistent_text"`
	Type             string `json:"type"`
	PlainText        string `json:"plain_text,omitempty"`
	FontSize         *int   `json:"font_size,omitempty"`
	FontColor        string `json:"font_color,omitempty"`
	BackgroundColor  string `json:"background_color,omitempty"`
}

// OSDImage describes the image content of an image OSD overlay.
type OSDImage struct {
	ImagePath string `json:"image_path"`
}

// OSDOptions describes the OSD capabilities of a video source.
type OSDOptions struct {
	MaximumNumberOfOSDs MaxOSDs      `json:"maximum_number_of_osds"`
	Types               []string     `json:"types"`
	PositionOptions     []string     `json:"position_options"`
	TextOptions         *OSDTextOpts `json:"text_options,omitempty"`
}

// MaxOSDs describes the maximum number of each OSD type.
type MaxOSDs struct {
	Total       int `json:"total"`
	PlainText   int `json:"plain_text"`
	DateAndTime int `json:"date_and_time"`
	Image       int `json:"image"`
}

// OSDTextOpts describes the available text OSD options.
type OSDTextOpts struct {
	Types           []string `json:"types"`
	FontSizeRange   *Range   `json:"font_size_range,omitempty"`
	FontColors      []string `json:"font_colors,omitempty"`
	BackgroundColors []string `json:"background_colors,omitempty"`
}

// Range represents a numeric range with min and max.
type Range struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// OSDConfig is the input type for CreateOSD and SetOSD.
type OSDConfig struct {
	Token            string      `json:"token,omitempty"`
	VideoSourceToken string      `json:"video_source_token"`
	Type             string      `json:"type"`
	Position         OSDPosition `json:"position"`
	TextString       *OSDText    `json:"text_string,omitempty"`
	Image            *OSDImage   `json:"image,omitempty"`
}

// --- XML response types for OSD ---

type osdEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    osdBody  `xml:"Body"`
}

type osdBody struct {
	GetOSDsResponse       *getOSDsResponse       `xml:"GetOSDsResponse"`
	GetOSDOptionsResponse *getOSDOptionsResponse  `xml:"GetOSDOptionsResponse"`
	CreateOSDResponse     *createOSDResponse      `xml:"CreateOSDResponse"`
	SetOSDResponse        *setOSDResponse         `xml:"SetOSDResponse"`
	DeleteOSDResponse     *deleteOSDResponse      `xml:"DeleteOSDResponse"`
	Fault                 *osdFault               `xml:"Fault"`
}

type osdFault struct {
	Faultstring string `xml:"faultstring"`
}

type getOSDsResponse struct {
	OSDs []osdXML `xml:"OSDs"`
}

type osdXML struct {
	Token            string         `xml:"token,attr"`
	VideoSourceToken osdReference   `xml:"VideoSourceConfigurationToken"`
	Type             string         `xml:"Type"`
	Position         osdPositionXML `xml:"Position"`
	TextString       *osdTextXML    `xml:"TextString"`
	Image            *osdImageXML   `xml:"Image"`
}

type osdReference struct {
	Token string `xml:",chardata"`
}

type osdPositionXML struct {
	Type string      `xml:"Type"`
	Pos  *osdPosXML  `xml:"Pos"`
}

type osdPosXML struct {
	X float64 `xml:"x,attr"`
	Y float64 `xml:"y,attr"`
}

type osdTextXML struct {
	IsPersistentText bool          `xml:"IsPersistentText"`
	Type             string        `xml:"Type"`
	PlainText        string        `xml:"PlainText"`
	FontSize         *int          `xml:"FontSize"`
	FontColor        *osdColorXML  `xml:"FontColor>Color"`
	BackgroundColor  *osdColorXML  `xml:"BackgroundColor>Color"`
}

type osdColorXML struct {
	X float64 `xml:"X,attr"`
	Y float64 `xml:"Y,attr"`
	Z float64 `xml:"Z,attr"`
}

type osdImageXML struct {
	ImagePath string `xml:"ImagePath"`
}

type getOSDOptionsResponse struct {
	OSDOptions osdOptionsXML `xml:"OSDOptions"`
}

type osdOptionsXML struct {
	MaximumNumberOfOSDs osdMaxXML        `xml:"MaximumNumberOfOSDs"`
	Type                []string         `xml:"Type"`
	PositionOption      []string         `xml:"PositionOption"`
	TextOption          *osdTextOptXML   `xml:"TextOption"`
}

type osdMaxXML struct {
	Total       int `xml:"Total,attr"`
	PlainText   int `xml:"PlainText,attr"`
	Date        int `xml:"Date,attr"`
	Time        int `xml:"Time,attr"`
	DateAndTime int `xml:"DateAndTime,attr"`
	Image       int `xml:"Image,attr"`
}

type osdTextOptXML struct {
	Type             []string       `xml:"Type"`
	FontSizeRange    *osdRangeXML   `xml:"FontSizeRange"`
	FontColor        []osdColorXML  `xml:"FontColor>Color"`
	BackgroundColor  []osdColorXML  `xml:"BackgroundColor>Color"`
}

type osdRangeXML struct {
	Min int `xml:"Min"`
	Max int `xml:"Max"`
}

type createOSDResponse struct {
	OSDToken string `xml:"OSDToken"`
}

type setOSDResponse struct{}

type deleteOSDResponse struct{}

// --- Public functions ---

// GetOSDs retrieves all OSD configurations for a video source via the Media2 service.
// Pass an empty configToken to get all OSDs on the device.
func GetOSDs(xaddr, username, password, configToken string) ([]OSD, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("GetOSDs: %w", err)
	}
	if !client.HasService("media2") {
		return nil, ErrOSDNotSupported
	}

	var reqBody string
	if configToken != "" {
		reqBody = fmt.Sprintf(`<tr2:GetOSDs>
      <tr2:ConfigurationToken>%s</tr2:ConfigurationToken>
    </tr2:GetOSDs>`, xmlEscape(configToken))
	} else {
		reqBody = `<tr2:GetOSDs/>`
	}

	body, err := doMedia2SOAP(client, reqBody)
	if err != nil {
		return nil, fmt.Errorf("GetOSDs: %w", err)
	}

	var env osdEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("GetOSDs parse: %w", err)
	}
	if env.Body.Fault != nil {
		return nil, fmt.Errorf("GetOSDs SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetOSDsResponse == nil {
		return nil, fmt.Errorf("GetOSDs: empty response")
	}

	return convertOSDs(env.Body.GetOSDsResponse.OSDs), nil
}

// convertOSDs converts XML OSD representations to the public OSD type.
func convertOSDs(xmlOSDs []osdXML) []OSD {
	var osds []OSD
	for _, x := range xmlOSDs {
		osd := OSD{
			Token:            x.Token,
			VideoSourceToken: x.VideoSourceToken.Token,
			Type:             x.Type,
			Position: OSDPosition{
				Type: x.Position.Type,
			},
		}
		if x.Position.Pos != nil {
			xVal := x.Position.Pos.X
			yVal := x.Position.Pos.Y
			osd.Position.X = &xVal
			osd.Position.Y = &yVal
		}
		if x.TextString != nil {
			osd.TextString = &OSDText{
				IsPersistentText: x.TextString.IsPersistentText,
				Type:             x.TextString.Type,
				PlainText:        x.TextString.PlainText,
				FontSize:         x.TextString.FontSize,
			}
			if x.TextString.FontColor != nil {
				osd.TextString.FontColor = formatOSDColor(*x.TextString.FontColor)
			}
			if x.TextString.BackgroundColor != nil {
				osd.TextString.BackgroundColor = formatOSDColor(*x.TextString.BackgroundColor)
			}
		}
		if x.Image != nil {
			osd.Image = &OSDImage{
				ImagePath: x.Image.ImagePath,
			}
		}
		osds = append(osds, osd)
	}
	return osds
}

// formatOSDColor converts ONVIF YCbCr color values to a string representation.
func formatOSDColor(c osdColorXML) string {
	return fmt.Sprintf("%.0f,%.0f,%.0f", c.X, c.Y, c.Z)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd internal/nvr/onvif && go build ./...`
Expected: Clean build, no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/onvif/osd.go
git commit -m "feat(osd): add OSD types, XML parsing, and GetOSDs function"
```

---

### Task 2: GetOSDOptions, CreateOSD, SetOSD, DeleteOSD

**Files:**

- Modify: `internal/nvr/onvif/osd.go`

Add the remaining four public functions to `osd.go`.

- [ ] **Step 1: Add GetOSDOptions function**

Append to `internal/nvr/onvif/osd.go` after the `GetOSDs` function:

```go
// GetOSDOptions retrieves the available OSD configuration options for a video source.
func GetOSDOptions(xaddr, username, password, configToken string) (*OSDOptions, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("GetOSDOptions: %w", err)
	}
	if !client.HasService("media2") {
		return nil, ErrOSDNotSupported
	}

	reqBody := fmt.Sprintf(`<tr2:GetOSDOptions>
      <tr2:ConfigurationToken>%s</tr2:ConfigurationToken>
    </tr2:GetOSDOptions>`, xmlEscape(configToken))

	body, err := doMedia2SOAP(client, reqBody)
	if err != nil {
		return nil, fmt.Errorf("GetOSDOptions: %w", err)
	}

	var env osdEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("GetOSDOptions parse: %w", err)
	}
	if env.Body.Fault != nil {
		return nil, fmt.Errorf("GetOSDOptions SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetOSDOptionsResponse == nil {
		return nil, fmt.Errorf("GetOSDOptions: empty response")
	}

	return convertOSDOptions(env.Body.GetOSDOptionsResponse.OSDOptions), nil
}

// convertOSDOptions converts the XML options to the public OSDOptions type.
func convertOSDOptions(x osdOptionsXML) *OSDOptions {
	opts := &OSDOptions{
		MaximumNumberOfOSDs: MaxOSDs{
			Total:       x.MaximumNumberOfOSDs.Total,
			PlainText:   x.MaximumNumberOfOSDs.PlainText,
			DateAndTime: x.MaximumNumberOfOSDs.DateAndTime,
			Image:       x.MaximumNumberOfOSDs.Image,
		},
		Types:           x.Type,
		PositionOptions: x.PositionOption,
	}

	if x.TextOption != nil {
		textOpts := &OSDTextOpts{
			Types: x.TextOption.Type,
		}
		if x.TextOption.FontSizeRange != nil {
			textOpts.FontSizeRange = &Range{
				Min: x.TextOption.FontSizeRange.Min,
				Max: x.TextOption.FontSizeRange.Max,
			}
		}
		for _, c := range x.TextOption.FontColor {
			textOpts.FontColors = append(textOpts.FontColors, formatOSDColor(c))
		}
		for _, c := range x.TextOption.BackgroundColor {
			textOpts.BackgroundColors = append(textOpts.BackgroundColors, formatOSDColor(c))
		}
		opts.TextOptions = textOpts
	}

	return opts
}
```

- [ ] **Step 2: Add CreateOSD function**

Append to `internal/nvr/onvif/osd.go`:

```go
// CreateOSD creates a new OSD overlay on the device via the Media2 service.
// Returns the token assigned to the new OSD by the device.
func CreateOSD(xaddr, username, password string, cfg OSDConfig) (string, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return "", fmt.Errorf("CreateOSD: %w", err)
	}
	if !client.HasService("media2") {
		return "", ErrOSDNotSupported
	}

	osdXMLBody := buildOSDXML(cfg)
	reqBody := fmt.Sprintf(`<tr2:CreateOSD>
      <tr2:OSD>%s</tr2:OSD>
    </tr2:CreateOSD>`, osdXMLBody)

	body, err := doMedia2SOAP(client, reqBody)
	if err != nil {
		return "", fmt.Errorf("CreateOSD: %w", err)
	}

	var env osdEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("CreateOSD parse: %w", err)
	}
	if env.Body.Fault != nil {
		return "", fmt.Errorf("CreateOSD SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.CreateOSDResponse == nil {
		return "", fmt.Errorf("CreateOSD: empty response")
	}

	return env.Body.CreateOSDResponse.OSDToken, nil
}
```

- [ ] **Step 3: Add SetOSD function**

Append to `internal/nvr/onvif/osd.go`:

```go
// SetOSD updates an existing OSD overlay on the device via the Media2 service.
func SetOSD(xaddr, username, password string, cfg OSDConfig) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return fmt.Errorf("SetOSD: %w", err)
	}
	if !client.HasService("media2") {
		return ErrOSDNotSupported
	}

	osdXMLBody := buildOSDXML(cfg)
	tokenAttr := ""
	if cfg.Token != "" {
		tokenAttr = fmt.Sprintf(` token="%s"`, xmlEscape(cfg.Token))
	}
	reqBody := fmt.Sprintf(`<tr2:SetOSD>
      <tr2:OSD%s>%s</tr2:OSD>
    </tr2:SetOSD>`, tokenAttr, osdXMLBody)

	body, err := doMedia2SOAP(client, reqBody)
	if err != nil {
		return fmt.Errorf("SetOSD: %w", err)
	}

	var env osdEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("SetOSD parse: %w", err)
	}
	if env.Body.Fault != nil {
		return fmt.Errorf("SetOSD SOAP fault: %s", env.Body.Fault.Faultstring)
	}

	return nil
}
```

- [ ] **Step 4: Add DeleteOSD function**

Append to `internal/nvr/onvif/osd.go`:

```go
// DeleteOSD removes an OSD overlay from the device via the Media2 service.
func DeleteOSD(xaddr, username, password, osdToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return fmt.Errorf("DeleteOSD: %w", err)
	}
	if !client.HasService("media2") {
		return ErrOSDNotSupported
	}

	reqBody := fmt.Sprintf(`<tr2:DeleteOSD>
      <tr2:OSDToken>%s</tr2:OSDToken>
    </tr2:DeleteOSD>`, xmlEscape(osdToken))

	body, err := doMedia2SOAP(client, reqBody)
	if err != nil {
		return fmt.Errorf("DeleteOSD: %w", err)
	}

	var env osdEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("DeleteOSD parse: %w", err)
	}
	if env.Body.Fault != nil {
		return fmt.Errorf("DeleteOSD SOAP fault: %s", env.Body.Fault.Faultstring)
	}

	return nil
}
```

- [ ] **Step 5: Add the buildOSDXML helper**

Append to `internal/nvr/onvif/osd.go`:

```go
// buildOSDXML constructs the inner XML body for an OSD element used by CreateOSD and SetOSD.
func buildOSDXML(cfg OSDConfig) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`<tt:VideoSourceConfigurationToken>%s</tt:VideoSourceConfigurationToken>`, xmlEscape(cfg.VideoSourceToken)))
	sb.WriteString(fmt.Sprintf(`<tt:Type>%s</tt:Type>`, xmlEscape(cfg.Type)))

	// Position
	sb.WriteString(`<tt:Position>`)
	sb.WriteString(fmt.Sprintf(`<tt:Type>%s</tt:Type>`, xmlEscape(cfg.Position.Type)))
	if cfg.Position.X != nil && cfg.Position.Y != nil {
		sb.WriteString(fmt.Sprintf(`<tt:Pos x="%.6f" y="%.6f"/>`, *cfg.Position.X, *cfg.Position.Y))
	}
	sb.WriteString(`</tt:Position>`)

	// TextString
	if cfg.TextString != nil {
		sb.WriteString(`<tt:TextString>`)
		if cfg.TextString.IsPersistentText {
			sb.WriteString(`<tt:IsPersistentText>true</tt:IsPersistentText>`)
		} else {
			sb.WriteString(`<tt:IsPersistentText>false</tt:IsPersistentText>`)
		}
		sb.WriteString(fmt.Sprintf(`<tt:Type>%s</tt:Type>`, xmlEscape(cfg.TextString.Type)))
		if cfg.TextString.PlainText != "" {
			sb.WriteString(fmt.Sprintf(`<tt:PlainText>%s</tt:PlainText>`, xmlEscape(cfg.TextString.PlainText)))
		}
		if cfg.TextString.FontSize != nil {
			sb.WriteString(fmt.Sprintf(`<tt:FontSize>%d</tt:FontSize>`, *cfg.TextString.FontSize))
		}
		sb.WriteString(`</tt:TextString>`)
	}

	// Image
	if cfg.Image != nil {
		sb.WriteString(fmt.Sprintf(`<tt:Image><tt:ImagePath>%s</tt:ImagePath></tt:Image>`, xmlEscape(cfg.Image.ImagePath)))
	}

	return sb.String()
}
```

- [ ] **Step 6: Add `strings` import and `tt` namespace to SOAP envelope**

The `buildOSDXML` function uses `strings.Builder` — add `"strings"` to the import block in `osd.go`.

The OSD create/set requests use `tt:` namespace prefix for schema types. Update the `media2SOAP` function in `media2.go` to include the `tt` namespace. Edit `internal/nvr/onvif/media2.go`, replacing the existing `media2SOAP` function:

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

- [ ] **Step 7: Verify it compiles**

Run: `cd internal/nvr/onvif && go build ./...`
Expected: Clean build, no errors.

- [ ] **Step 8: Commit**

```bash
git add internal/nvr/onvif/osd.go internal/nvr/onvif/media2.go
git commit -m "feat(osd): add GetOSDOptions, CreateOSD, SetOSD, DeleteOSD functions"
```

---

### Task 3: API Handlers

**Files:**

- Modify: `internal/nvr/api/cameras.go` (append after analytics handlers, ~line 1600)

Add 5 handler methods to `CameraHandler`.

- [ ] **Step 1: Add GetOSDs handler**

Append to `internal/nvr/api/cameras.go`:

```go
// GetOSDs returns all OSD overlays for a camera.
//
//	GET /cameras/:id/osd
func (h *CameraHandler) GetOSDs(c *gin.Context) {
	id := c.Param("id")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for OSD", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	osds, err := onvif.GetOSDs(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), cam.ONVIFProfileToken)
	if err != nil {
		if errors.Is(err, onvif.ErrOSDNotSupported) {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "camera does not support OSD management"})
			return
		}
		nvrLogError("osd", fmt.Sprintf("failed to get OSDs for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get OSDs from device"})
		return
	}

	if osds == nil {
		osds = []onvif.OSD{}
	}

	c.JSON(http.StatusOK, gin.H{"osds": osds})
}
```

- [ ] **Step 2: Add GetOSDOptions handler**

Append to `internal/nvr/api/cameras.go`:

```go
// GetOSDOptions returns available OSD options for a camera.
//
//	GET /cameras/:id/osd/options
func (h *CameraHandler) GetOSDOptions(c *gin.Context) {
	id := c.Param("id")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for OSD options", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	options, err := onvif.GetOSDOptions(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), cam.ONVIFProfileToken)
	if err != nil {
		if errors.Is(err, onvif.ErrOSDNotSupported) {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "camera does not support OSD management"})
			return
		}
		nvrLogError("osd", fmt.Sprintf("failed to get OSD options for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get OSD options from device"})
		return
	}

	c.JSON(http.StatusOK, options)
}
```

- [ ] **Step 3: Add CreateOSD handler**

Append to `internal/nvr/api/cameras.go`:

```go
// CreateOSD creates a new OSD overlay on a camera.
//
//	POST /cameras/:id/osd
func (h *CameraHandler) CreateOSD(c *gin.Context) {
	id := c.Param("id")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for OSD creation", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	var cfg onvif.OSDConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if err := validateOSDConfig(cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token, err := onvif.CreateOSD(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), cfg)
	if err != nil {
		if errors.Is(err, onvif.ErrOSDNotSupported) {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "camera does not support OSD management"})
			return
		}
		nvrLogError("osd", fmt.Sprintf("failed to create OSD for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to create OSD on device"})
		return
	}

	nvrLogInfo("osd", fmt.Sprintf("Created OSD %q on camera %s", token, id))
	c.JSON(http.StatusCreated, gin.H{"token": token})
}
```

- [ ] **Step 4: Add SetOSD handler**

Append to `internal/nvr/api/cameras.go`:

```go
// SetOSD updates an existing OSD overlay on a camera.
//
//	PUT /cameras/:id/osd/:token
func (h *CameraHandler) SetOSD(c *gin.Context) {
	id := c.Param("id")
	osdToken := c.Param("token")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for OSD update", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	var cfg onvif.OSDConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Use URL param token if not in body.
	if cfg.Token == "" {
		cfg.Token = osdToken
	}

	if err := validateOSDConfig(cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := onvif.SetOSD(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), cfg); err != nil {
		if errors.Is(err, onvif.ErrOSDNotSupported) {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "camera does not support OSD management"})
			return
		}
		nvrLogError("osd", fmt.Sprintf("failed to update OSD %q for camera %s", osdToken, id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to update OSD on device"})
		return
	}

	nvrLogInfo("osd", fmt.Sprintf("Updated OSD %q on camera %s", osdToken, id))
	c.JSON(http.StatusOK, gin.H{"message": "OSD updated"})
}
```

- [ ] **Step 5: Add DeleteOSD handler**

Append to `internal/nvr/api/cameras.go`:

```go
// DeleteOSD removes an OSD overlay from a camera.
//
//	DELETE /cameras/:id/osd/:token
func (h *CameraHandler) DeleteOSD(c *gin.Context) {
	id := c.Param("id")
	osdToken := c.Param("token")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for OSD deletion", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	if err := onvif.DeleteOSD(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), osdToken); err != nil {
		if errors.Is(err, onvif.ErrOSDNotSupported) {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "camera does not support OSD management"})
			return
		}
		nvrLogError("osd", fmt.Sprintf("failed to delete OSD %q for camera %s", osdToken, id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to delete OSD from device"})
		return
	}

	nvrLogInfo("osd", fmt.Sprintf("Deleted OSD %q from camera %s", osdToken, id))
	c.JSON(http.StatusOK, gin.H{"message": "OSD deleted"})
}
```

- [ ] **Step 6: Add validateOSDConfig helper**

Append to `internal/nvr/api/cameras.go`:

```go
// validateOSDConfig validates an OSD configuration for create and set operations.
func validateOSDConfig(cfg onvif.OSDConfig) error {
	if cfg.Type != "Text" && cfg.Type != "Image" {
		return fmt.Errorf("type must be 'Text' or 'Image'")
	}

	validPositions := map[string]bool{
		"UpperLeft": true, "UpperRight": true,
		"LowerLeft": true, "LowerRight": true,
		"Custom": true,
	}
	if !validPositions[cfg.Position.Type] {
		return fmt.Errorf("position type must be one of: UpperLeft, UpperRight, LowerLeft, LowerRight, Custom")
	}

	if cfg.Position.Type == "Custom" && (cfg.Position.X == nil || cfg.Position.Y == nil) {
		return fmt.Errorf("custom position requires x and y coordinates")
	}

	if cfg.Type == "Text" && cfg.TextString == nil {
		return fmt.Errorf("text_string is required when type is 'Text'")
	}

	if cfg.Type == "Image" && (cfg.Image == nil || cfg.Image.ImagePath == "") {
		return fmt.Errorf("image with image_path is required when type is 'Image'")
	}

	if cfg.VideoSourceToken == "" {
		return fmt.Errorf("video_source_token is required")
	}

	return nil
}
```

- [ ] **Step 7: Verify it compiles**

Run: `go build ./internal/nvr/api/...`
Expected: Clean build, no errors.

- [ ] **Step 8: Commit**

```bash
git add internal/nvr/api/cameras.go
git commit -m "feat(osd): add OSD API handlers with validation"
```

---

### Task 4: Route Registration

**Files:**

- Modify: `internal/nvr/api/router.go` (~line 280, after analytics routes)

- [ ] **Step 1: Add OSD routes**

In `internal/nvr/api/router.go`, add the following block after the analytics routes (after line 280, after `protected.GET("/cameras/:id/analytics/modules", cameraHandler.GetAnalyticsModules)`):

```go
	// OSD (On-Screen Display) management.
	protected.GET("/cameras/:id/osd", cameraHandler.GetOSDs)
	protected.GET("/cameras/:id/osd/options", cameraHandler.GetOSDOptions)
	protected.POST("/cameras/:id/osd", cameraHandler.CreateOSD)
	protected.PUT("/cameras/:id/osd/:token", cameraHandler.SetOSD)
	protected.DELETE("/cameras/:id/osd/:token", cameraHandler.DeleteOSD)
```

- [ ] **Step 2: Verify the full project compiles**

Run: `go build ./...`
Expected: Clean build, no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/router.go
git commit -m "feat(osd): register OSD API routes"
```

---

### Task 5: Verify and Final Commit

- [ ] **Step 1: Run existing tests to ensure nothing is broken**

Run: `go test ./internal/nvr/... -count=1 -timeout 60s`
Expected: All existing tests pass.

- [ ] **Step 2: Run `go vet` for static analysis**

Run: `go vet ./internal/nvr/...`
Expected: No issues reported.

- [ ] **Step 3: Verify all three files are in expected state**

Confirm the following files exist and contain the expected code:

- `internal/nvr/onvif/osd.go` — should contain `GetOSDs`, `GetOSDOptions`, `CreateOSD`, `SetOSD`, `DeleteOSD`, `buildOSDXML`, `convertOSDs`, `convertOSDOptions`, `formatOSDColor`, `validateOSDConfig` (wait — `validateOSDConfig` is in cameras.go, not osd.go)
- `internal/nvr/api/cameras.go` — should contain `GetOSDs`, `GetOSDOptions`, `CreateOSD`, `SetOSD`, `DeleteOSD` handler methods and `validateOSDConfig`
- `internal/nvr/api/router.go` — should contain 5 OSD route registrations

Run: `grep -n "func.*OSD" internal/nvr/onvif/osd.go internal/nvr/api/cameras.go`
Expected: All 5 public ONVIF functions and 5 handlers plus the validator listed.

- [ ] **Step 4: Push branch and create PR**

```bash
git push -u origin feat/kai-25-osd-management
```

Then create PR to main with title "feat: KAI-25 ONVIF Profile T OSD management" covering the 3 changed files and 5 CRUD operations.
