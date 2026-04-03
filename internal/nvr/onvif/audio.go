package onvif

import (
	"context"
)

// AudioCapabilities summarises the audio capabilities of an ONVIF camera.
type AudioCapabilities struct {
	HasBackchannel   bool   `json:"has_backchannel"`
	AudioSources     int    `json:"audio_sources"`
	AudioOutputs     int    `json:"audio_outputs"`
	BackchannelCodec string `json:"backchannel_codec,omitempty"`
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
	outputs, err := client.Dev.GetAudioOutputs(ctx)
	if err == nil && len(outputs) > 0 && outputs[0].Token != "" {
		caps.HasBackchannel = true
		caps.AudioOutputs = 1
	}

	// Check audio sources (microphone input on the camera).
	sources, err := client.Dev.GetAudioSources(ctx)
	if err == nil && len(sources) > 0 && sources[0].Channels > 0 {
		caps.AudioSources = 1
	}

	return caps, nil
}
