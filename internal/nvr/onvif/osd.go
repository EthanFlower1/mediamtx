package onvif

import (
	"encoding/xml"
	"fmt"
	"strings"
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
	Types            []string `json:"types"`
	FontSizeRange    *Range   `json:"font_size_range,omitempty"`
	FontColors       []string `json:"font_colors,omitempty"`
	BackgroundColors []string `json:"background_colors,omitempty"`
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
	GetOSDsResponse       *getOSDsResponse      `xml:"GetOSDsResponse"`
	GetOSDOptionsResponse *getOSDOptionsResponse `xml:"GetOSDOptionsResponse"`
	CreateOSDResponse     *createOSDResponse     `xml:"CreateOSDResponse"`
	SetOSDResponse        *setOSDResponse        `xml:"SetOSDResponse"`
	DeleteOSDResponse     *deleteOSDResponse     `xml:"DeleteOSDResponse"`
	Fault                 *osdFault              `xml:"Fault"`
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
	Type string     `xml:"Type"`
	Pos  *osdPosXML `xml:"Pos"`
}

type osdPosXML struct {
	X float64 `xml:"x,attr"`
	Y float64 `xml:"y,attr"`
}

type osdTextXML struct {
	IsPersistentText bool         `xml:"IsPersistentText"`
	Type             string       `xml:"Type"`
	PlainText        string       `xml:"PlainText"`
	FontSize         *int         `xml:"FontSize"`
	FontColor        *osdColorXML `xml:"FontColor>Color"`
	BackgroundColor  *osdColorXML `xml:"BackgroundColor>Color"`
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
	MaximumNumberOfOSDs osdMaxXML      `xml:"MaximumNumberOfOSDs"`
	Type                []string       `xml:"Type"`
	PositionOption      []string       `xml:"PositionOption"`
	TextOption          *osdTextOptXML `xml:"TextOption"`
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
	Type            []string      `xml:"Type"`
	FontSizeRange   *osdRangeXML  `xml:"FontSizeRange"`
	FontColor       []osdColorXML `xml:"FontColor>Color"`
	BackgroundColor []osdColorXML `xml:"BackgroundColor>Color"`
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
