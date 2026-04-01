package onvif

import (
	"context"
	"fmt"

	onvifgo "github.com/0x524a/onvif-go"
)

// PTZPreset represents a single preset exposed by the camera.
type PTZPreset struct {
	Token string `json:"token"`
	Name  string `json:"name"`
}

// PTZController wraps an ONVIF device and provides PTZ operations.
type PTZController struct {
	dev *onvifgo.Client
}

// NewPTZController connects to an ONVIF device at xaddr with the given
// credentials and returns a controller ready for PTZ operations.
func NewPTZController(xaddr, username, password string) (*PTZController, error) {
	var opts []onvifgo.ClientOption
	if username != "" {
		opts = append(opts, onvifgo.WithCredentials(username, password))
	}

	dev, err := onvifgo.NewClient(xaddr, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect to ONVIF device: %w", err)
	}

	ctx := context.Background()
	if err := dev.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("connect to ONVIF device: %w", err)
	}

	return &PTZController{dev: dev}, nil
}

// ContinuousMove starts a continuous PTZ movement. Speed values are in the
// range [-1.0, 1.0]. The camera keeps moving until Stop is called.
func (p *PTZController) ContinuousMove(profileToken string, panSpeed, tiltSpeed, zoomSpeed float64) error {
	velocity := &onvifgo.PTZSpeed{
		PanTilt: &onvifgo.Vector2D{
			X: panSpeed,
			Y: tiltSpeed,
		},
		Zoom: &onvifgo.Vector1D{
			X: zoomSpeed,
		},
	}

	ctx := context.Background()
	if err := p.dev.ContinuousMove(ctx, profileToken, velocity, nil); err != nil {
		return fmt.Errorf("continuous move: %w", err)
	}
	return nil
}

// Stop halts all PTZ movement on the given profile.
func (p *PTZController) Stop(profileToken string) error {
	ctx := context.Background()
	if err := p.dev.Stop(ctx, profileToken, true, true); err != nil {
		return fmt.Errorf("stop: %w", err)
	}
	return nil
}

// GetPresets retrieves the list of PTZ presets configured on the camera.
func (p *PTZController) GetPresets(profileToken string) ([]PTZPreset, error) {
	ctx := context.Background()
	rawPresets, err := p.dev.GetPresets(ctx, profileToken)
	if err != nil {
		return nil, fmt.Errorf("get presets: %w", err)
	}

	var presets []PTZPreset
	for _, preset := range rawPresets {
		if preset.Token != "" {
			presets = append(presets, PTZPreset{
				Token: preset.Token,
				Name:  preset.Name,
			})
		}
	}

	return presets, nil
}

// GotoPreset moves the camera to a previously saved preset position.
func (p *PTZController) GotoPreset(profileToken, presetToken string) error {
	ctx := context.Background()
	if err := p.dev.GotoPreset(ctx, profileToken, presetToken, nil); err != nil {
		return fmt.Errorf("goto preset: %w", err)
	}
	return nil
}

// GotoHome moves the camera to its home position.
func (p *PTZController) GotoHome(profileToken string) error {
	ctx := context.Background()
	if err := p.dev.GotoHomePosition(ctx, profileToken, nil); err != nil {
		return fmt.Errorf("goto home: %w", err)
	}
	return nil
}

// PTZNode represents a PTZ node and its capabilities.
type PTZNode struct {
	Token         string `json:"token"`
	Name          string `json:"name"`
	MaxPresets    int    `json:"max_presets"`
	HomeSupported bool   `json:"home_supported"`
}

// GetNodes retrieves PTZ node information from the device using PTZ configurations.
func (p *PTZController) GetNodes() ([]PTZNode, error) {
	ctx := context.Background()
	configs, err := p.dev.GetConfigurations(ctx)
	if err != nil {
		return nil, fmt.Errorf("get PTZ nodes: %w", err)
	}

	var nodes []PTZNode
	for _, cfg := range configs {
		if cfg.NodeToken != "" {
			nodes = append(nodes, PTZNode{
				Token:         cfg.NodeToken,
				Name:          cfg.Name,
				HomeSupported: true,
			})
		}
	}

	return nodes, nil
}

// AbsoluteMove moves the camera to an absolute position.
func (p *PTZController) AbsoluteMove(profileToken string, panPos, tiltPos, zoomPos float64) error {
	position := &onvifgo.PTZVector{
		PanTilt: &onvifgo.Vector2D{X: panPos, Y: tiltPos},
		Zoom:    &onvifgo.Vector1D{X: zoomPos},
	}
	ctx := context.Background()
	if err := p.dev.AbsoluteMove(ctx, profileToken, position, nil); err != nil {
		return fmt.Errorf("absolute move: %w", err)
	}
	return nil
}

// RelativeMove moves the camera by a relative offset.
func (p *PTZController) RelativeMove(profileToken string, panDelta, tiltDelta, zoomDelta float64) error {
	translation := &onvifgo.PTZVector{
		PanTilt: &onvifgo.Vector2D{X: panDelta, Y: tiltDelta},
		Zoom:    &onvifgo.Vector1D{X: zoomDelta},
	}
	ctx := context.Background()
	if err := p.dev.RelativeMove(ctx, profileToken, translation, nil); err != nil {
		return fmt.Errorf("relative move: %w", err)
	}
	return nil
}

// SetPreset saves the current position as a named preset. Returns the preset token.
func (p *PTZController) SetPreset(profileToken, presetName string) (string, error) {
	ctx := context.Background()
	token, err := p.dev.SetPreset(ctx, profileToken, presetName, "")
	if err != nil {
		return "", fmt.Errorf("set preset: %w", err)
	}
	return token, nil
}

// RemovePreset deletes a preset from the camera.
func (p *PTZController) RemovePreset(profileToken, presetToken string) error {
	ctx := context.Background()
	if err := p.dev.RemovePreset(ctx, profileToken, presetToken); err != nil {
		return fmt.Errorf("remove preset: %w", err)
	}
	return nil
}

// SetHomePosition saves the current position as the home position.
func (p *PTZController) SetHomePosition(profileToken string) error {
	ctx := context.Background()
	if err := p.dev.SetHomePosition(ctx, profileToken); err != nil {
		return fmt.Errorf("set home position: %w", err)
	}
	return nil
}

// PTZStatus holds the current PTZ position and movement state.
type PTZStatus struct {
	PanPosition  float64 `json:"pan_position"`
	TiltPosition float64 `json:"tilt_position"`
	ZoomPosition float64 `json:"zoom_position"`
	IsMoving     bool    `json:"is_moving"`
}

// GetStatus returns the current PTZ position and movement state.
func (p *PTZController) GetStatus(profileToken string) (*PTZStatus, error) {
	ctx := context.Background()
	status, err := p.dev.GetStatus(ctx, profileToken)
	if err != nil {
		return nil, fmt.Errorf("get PTZ status: %w", err)
	}
	result := &PTZStatus{}
	if status.Position != nil {
		if status.Position.PanTilt != nil {
			result.PanPosition = status.Position.PanTilt.X
			result.TiltPosition = status.Position.PanTilt.Y
		}
		if status.Position.Zoom != nil {
			result.ZoomPosition = status.Position.Zoom.X
		}
	}
	if status.MoveStatus != nil {
		result.IsMoving = status.MoveStatus.PanTilt != "IDLE" || status.MoveStatus.Zoom != "IDLE"
	}
	return result, nil
}
