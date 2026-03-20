package onvif

import (
	"context"
	"fmt"

	onviflib "github.com/use-go/onvif"
	onvifptz "github.com/use-go/onvif/ptz"
	sdkptz "github.com/use-go/onvif/sdk/ptz"
	"github.com/use-go/onvif/xsd"
	onviftypes "github.com/use-go/onvif/xsd/onvif"
)

// PTZPreset represents a single preset exposed by the camera.
type PTZPreset struct {
	Token string `json:"token"`
	Name  string `json:"name"`
}

// PTZController wraps an ONVIF device and provides PTZ operations.
type PTZController struct {
	dev *onviflib.Device
}

// NewPTZController connects to an ONVIF device at xaddr with the given
// credentials and returns a controller ready for PTZ operations.
func NewPTZController(xaddr, username, password string) (*PTZController, error) {
	host := xaddrToHost(xaddr)
	if host == "" {
		host = xaddr
	}

	dev, err := onviflib.NewDevice(onviflib.DeviceParams{
		Xaddr:    host,
		Username: username,
		Password: password,
	})
	if err != nil {
		return nil, fmt.Errorf("connect to ONVIF device: %w", err)
	}

	return &PTZController{dev: dev}, nil
}

// ContinuousMove starts a continuous PTZ movement. Speed values are in the
// range [-1.0, 1.0]. The camera keeps moving until Stop is called.
func (p *PTZController) ContinuousMove(profileToken string, panSpeed, tiltSpeed, zoomSpeed float64) error {
	req := onvifptz.ContinuousMove{
		ProfileToken: onviftypes.ReferenceToken(profileToken),
		Velocity: onviftypes.PTZSpeed{
			PanTilt: onviftypes.Vector2D{
				X: panSpeed,
				Y: tiltSpeed,
			},
			Zoom: onviftypes.Vector1D{
				X: zoomSpeed,
			},
		},
	}

	ctx := context.Background()
	_, err := sdkptz.Call_ContinuousMove(ctx, p.dev, req)
	if err != nil {
		return fmt.Errorf("continuous move: %w", err)
	}
	return nil
}

// Stop halts all PTZ movement on the given profile.
func (p *PTZController) Stop(profileToken string) error {
	req := onvifptz.Stop{
		ProfileToken: onviftypes.ReferenceToken(profileToken),
		PanTilt:      xsd.Boolean(true),
		Zoom:         xsd.Boolean(true),
	}

	ctx := context.Background()
	_, err := sdkptz.Call_Stop(ctx, p.dev, req)
	if err != nil {
		return fmt.Errorf("stop: %w", err)
	}
	return nil
}

// GetPresets retrieves the list of PTZ presets configured on the camera.
func (p *PTZController) GetPresets(profileToken string) ([]PTZPreset, error) {
	req := onvifptz.GetPresets{
		ProfileToken: onviftypes.ReferenceToken(profileToken),
	}

	ctx := context.Background()
	resp, err := sdkptz.Call_GetPresets(ctx, p.dev, req)
	if err != nil {
		return nil, fmt.Errorf("get presets: %w", err)
	}

	// The SDK returns a single PTZPreset struct; some cameras embed multiple
	// presets in a single response. We handle both the single-value and the
	// case where only one preset is returned.
	var presets []PTZPreset
	preset := resp.Preset
	if string(preset.Token) != "" {
		presets = append(presets, PTZPreset{
			Token: string(preset.Token),
			Name:  string(preset.Name),
		})
	}

	return presets, nil
}

// GotoPreset moves the camera to a previously saved preset position.
func (p *PTZController) GotoPreset(profileToken, presetToken string) error {
	req := onvifptz.GotoPreset{
		ProfileToken: onviftypes.ReferenceToken(profileToken),
		PresetToken:  onviftypes.ReferenceToken(presetToken),
	}

	ctx := context.Background()
	_, err := sdkptz.Call_GotoPreset(ctx, p.dev, req)
	if err != nil {
		return fmt.Errorf("goto preset: %w", err)
	}
	return nil
}

// GotoHome moves the camera to its home position.
func (p *PTZController) GotoHome(profileToken string) error {
	req := onvifptz.GotoHomePosition{
		ProfileToken: onviftypes.ReferenceToken(profileToken),
	}

	ctx := context.Background()
	_, err := sdkptz.Call_GotoHomePosition(ctx, p.dev, req)
	if err != nil {
		return fmt.Errorf("goto home: %w", err)
	}
	return nil
}
