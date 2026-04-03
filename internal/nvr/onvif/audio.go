package onvif

import (
	"context"
	"fmt"
)

// AudioCapabilities summarises the audio capabilities of an ONVIF camera.
type AudioCapabilities struct {
	HasBackchannel bool `json:"has_backchannel"`
	AudioSources   int  `json:"audio_sources"`
	AudioOutputs   int  `json:"audio_outputs"`
}

// AudioSourceInfo describes a physical audio input (microphone) on the device.
type AudioSourceInfo struct {
	Token    string `json:"token"`
	Channels int    `json:"channels"`
}

// AudioSourceConfig represents an audio source configuration that binds
// a physical audio source to a media profile.
type AudioSourceConfig struct {
	Token       string `json:"token"`
	Name        string `json:"name"`
	UseCount    int    `json:"use_count"`
	SourceToken string `json:"source_token"`
}

// AudioSourceConfigOptions describes the available options when configuring
// an audio source (e.g. which input tokens can be selected).
type AudioSourceConfigOptions struct {
	InputTokensAvailable []string `json:"input_tokens_available"`
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

// GetAudioSources returns all audio sources (microphones) on the device.
func GetAudioSources(xaddr, username, password string) ([]*AudioSourceInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	sources, err := client.Dev.GetAudioSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("get audio sources: %w", err)
	}

	result := make([]*AudioSourceInfo, len(sources))
	for i, s := range sources {
		result[i] = &AudioSourceInfo{
			Token:    s.Token,
			Channels: s.Channels,
		}
	}
	return result, nil
}
