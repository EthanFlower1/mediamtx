package onvif

import (
	"context"
	"fmt"

	onviflib "github.com/use-go/onvif"
	onvifimaging "github.com/use-go/onvif/Imaging"
	"github.com/use-go/onvif/sdk"
	"github.com/use-go/onvif/xsd"
	onviftypes "github.com/use-go/onvif/xsd/onvif"
)

// ImagingSettings represents the adjustable image settings of a camera.
type ImagingSettings struct {
	Brightness float64 `json:"brightness"`
	Contrast   float64 `json:"contrast"`
	Saturation float64 `json:"saturation"`
	Sharpness  float64 `json:"sharpness"`
}

// getImagingSettingsResponse is the SOAP envelope for GetImagingSettings.
type getImagingSettingsResponse struct {
	ImagingSettings onviftypes.ImagingSettings20 `xml:"ImagingSettings"`
}

// getVideoSourceToken connects to the camera and finds the first video source token.
// The imaging service needs a video source token, not a media profile token.
func getVideoSourceToken(dev *onviflib.Device) (string, error) {
	type getVideoSourcesResp struct {
		VideoSources []struct {
			Token string `xml:"token,attr"`
		} `xml:"VideoSources"`
	}
	type envelope struct {
		Body struct {
			GetVideoSourcesResponse getVideoSourcesResp
		}
	}

	httpReply, err := dev.CallMethod(struct {
		XMLName string `xml:"trt:GetVideoSources"`
	}{})
	if err != nil {
		return "", err
	}

	var reply envelope
	if err := sdk.ReadAndParse(context.Background(), httpReply, &reply, "GetVideoSources"); err != nil {
		return "", err
	}

	if len(reply.Body.GetVideoSourcesResponse.VideoSources) > 0 {
		return reply.Body.GetVideoSourcesResponse.VideoSources[0].Token, nil
	}
	return "", fmt.Errorf("no video sources found")
}

// connectDevice creates an ONVIF device connection from an endpoint URL.
// It delegates to NewClient so the connection logic is centralised.
func connectDevice(xaddr, username, password string) (*onviflib.Device, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}
	return client.Dev, nil
}

// ErrImagingNotSupported is returned when the camera doesn't expose an imaging service.
var ErrImagingNotSupported = fmt.Errorf("camera does not support ONVIF imaging service")

// GetImagingSettings retrieves the current imaging settings from an ONVIF camera.
func GetImagingSettings(xaddr, username, password, videoSourceToken string) (*ImagingSettings, error) {
	dev, err := connectDevice(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("connect to ONVIF device: %w", err)
	}

	// Check if camera supports imaging service.
	services := dev.GetServices()
	if _, ok := services["imaging"]; !ok {
		return nil, ErrImagingNotSupported
	}

	// If no video source token provided, discover it from the camera.
	if videoSourceToken == "" || videoSourceToken == "000" {
		token, err := getVideoSourceToken(dev)
		if err != nil {
			return nil, fmt.Errorf("get video source token: %w", err)
		}
		videoSourceToken = token
	}

	req := onvifimaging.GetImagingSettings{
		VideoSourceToken: onviftypes.ReferenceToken(videoSourceToken),
	}

	type Envelope struct {
		Header struct{}
		Body   struct {
			GetImagingSettingsResponse getImagingSettingsResponse
		}
	}

	var reply Envelope
	ctx := context.Background()
	httpReply, err := dev.CallMethod(req)
	if err != nil {
		return nil, fmt.Errorf("call GetImagingSettings: %w", err)
	}

	if err := sdk.ReadAndParse(ctx, httpReply, &reply, "GetImagingSettings"); err != nil {
		return nil, fmt.Errorf("parse GetImagingSettings: %w", err)
	}

	settings := reply.Body.GetImagingSettingsResponse.ImagingSettings
	return &ImagingSettings{
		Brightness: settings.Brightness,
		Contrast:   settings.Contrast,
		Saturation: settings.ColorSaturation,
		Sharpness:  settings.Sharpness,
	}, nil
}

// SetImagingSettings applies imaging settings to an ONVIF camera.
func SetImagingSettings(xaddr, username, password, videoSourceToken string, settings *ImagingSettings) error {
	dev, err := connectDevice(xaddr, username, password)
	if err != nil {
		return fmt.Errorf("connect to ONVIF device: %w", err)
	}

	// If no video source token provided, discover it from the camera.
	if videoSourceToken == "" || videoSourceToken == "000" {
		token, err := getVideoSourceToken(dev)
		if err != nil {
			return fmt.Errorf("get video source token: %w", err)
		}
		videoSourceToken = token
	}

	req := onvifimaging.SetImagingSettings{
		VideoSourceToken: onviftypes.ReferenceToken(videoSourceToken),
		ImagingSettings: onviftypes.ImagingSettings20{
			Brightness:    settings.Brightness,
			Contrast:      settings.Contrast,
			ColorSaturation: settings.Saturation,
			Sharpness:     settings.Sharpness,
		},
		ForcePersistence: xsd.Boolean(true),
	}

	ctx := context.Background()

	type Envelope struct {
		Header struct{}
		Body   struct {
			SetImagingSettingsResponse struct{} // empty response on success
		}
	}

	var reply Envelope
	httpReply, err := dev.CallMethod(req)
	if err != nil {
		return fmt.Errorf("call SetImagingSettings: %w", err)
	}

	if err := sdk.ReadAndParse(ctx, httpReply, &reply, "SetImagingSettings"); err != nil {
		return fmt.Errorf("parse SetImagingSettings: %w", err)
	}

	return nil
}
