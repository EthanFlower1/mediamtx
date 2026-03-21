package onvif

import (
	"context"

	onvifmedia "github.com/use-go/onvif/media"
	sdkmedia "github.com/use-go/onvif/sdk/media"
)

// AudioCapabilities summarises the audio capabilities of an ONVIF camera.
type AudioCapabilities struct {
	HasBackchannel bool `json:"has_backchannel"`
	AudioSources   int  `json:"audio_sources"`
	AudioOutputs   int  `json:"audio_outputs"`
}

// GetAudioCapabilities queries an ONVIF camera for its audio capabilities.
// A camera with audio outputs is considered to support backchannel (two-way) audio.
func GetAudioCapabilities(xaddr, username, password string) (*AudioCapabilities, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	caps := &AudioCapabilities{}
	ctx := context.Background()

	// Check audio outputs (backchannel = camera has audio output capability).
	outputResp, err := sdkmedia.Call_GetAudioOutputs(ctx, client.Dev, onvifmedia.GetAudioOutputs{})
	if err == nil && string(outputResp.AudioOutputs.Token) != "" {
		caps.HasBackchannel = true
		caps.AudioOutputs = 1
	}

	// Check audio sources (microphone input on the camera).
	sourceResp, err := sdkmedia.Call_GetAudioSources(ctx, client.Dev, onvifmedia.GetAudioSources{})
	if err == nil && sourceResp.AudioSources.Channels > 0 {
		caps.AudioSources = 1
	}

	return caps, nil
}
