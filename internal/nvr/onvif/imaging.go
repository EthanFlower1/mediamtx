package onvif

import (
	"context"
	"fmt"

	onvifgo "github.com/EthanFlower1/onvif-go"
)

// ImagingSettings represents the adjustable image settings of a camera.
type ImagingSettings struct {
	Brightness float64 `json:"brightness"`
	Contrast   float64 `json:"contrast"`
	Saturation float64 `json:"saturation"`
	Sharpness  float64 `json:"sharpness"`
}

// ErrImagingNotSupported is returned when the camera doesn't expose an imaging service.
var ErrImagingNotSupported = fmt.Errorf("camera does not support ONVIF imaging service")

// GetImagingSettings retrieves the current imaging settings from an ONVIF camera.
func GetImagingSettings(xaddr, username, password, videoSourceToken string) (*ImagingSettings, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("connect to ONVIF device: %w", err)
	}

	// Check if camera supports imaging service.
	if !client.HasService("imaging") {
		return nil, ErrImagingNotSupported
	}

	ctx := context.Background()

	// If no video source token provided, discover it from the camera.
	if videoSourceToken == "" || videoSourceToken == "000" {
		token, err := getVideoSourceToken(ctx, client.Dev)
		if err != nil {
			return nil, fmt.Errorf("get video source token: %w", err)
		}
		videoSourceToken = token
	}

	raw, err := client.Dev.GetImagingSettings(ctx, videoSourceToken)
	if err != nil {
		return nil, fmt.Errorf("get imaging settings: %w", err)
	}

	return &ImagingSettings{
		Brightness: derefFloat(raw.Brightness),
		Contrast:   derefFloat(raw.Contrast),
		Saturation: derefFloat(raw.ColorSaturation),
		Sharpness:  derefFloat(raw.Sharpness),
	}, nil
}

// SetImagingSettings applies imaging settings to an ONVIF camera.
func SetImagingSettings(xaddr, username, password, videoSourceToken string, settings *ImagingSettings) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return fmt.Errorf("connect to ONVIF device: %w", err)
	}

	ctx := context.Background()

	// If no video source token provided, discover it from the camera.
	if videoSourceToken == "" || videoSourceToken == "000" {
		token, err := getVideoSourceToken(ctx, client.Dev)
		if err != nil {
			return fmt.Errorf("get video source token: %w", err)
		}
		videoSourceToken = token
	}

	raw := &onvifgo.ImagingSettings{
		Brightness:    &settings.Brightness,
		Contrast:      &settings.Contrast,
		ColorSaturation: &settings.Saturation,
		Sharpness:     &settings.Sharpness,
	}

	if err := client.Dev.SetImagingSettings(ctx, videoSourceToken, raw, true); err != nil {
		return fmt.Errorf("set imaging settings: %w", err)
	}

	return nil
}

// getVideoSourceToken discovers the first video source token from the camera.
func getVideoSourceToken(ctx context.Context, dev *onvifgo.Client) (string, error) {
	sources, err := dev.GetVideoSources(ctx)
	if err != nil {
		return "", fmt.Errorf("get video sources: %w", err)
	}
	if len(sources) == 0 {
		return "", fmt.Errorf("no video sources found")
	}
	return sources[0].Token, nil
}

// derefFloat safely dereferences a *float64, returning 0 if nil.
func derefFloat(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}
