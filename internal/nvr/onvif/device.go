package onvif

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"

	onvifgo "github.com/EthanFlower1/onvif-go"
)

// ErrNotImplemented is returned by stub methods that are not yet implemented.
var ErrNotImplemented = errors.New("not implemented")

// ProbeDevice connects to an ONVIF device with credentials and returns its media profiles
// including RTSP stream URIs.
func ProbeDevice(xaddr, username, password string) ([]MediaProfile, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	rawProfiles, err := client.Dev.GetProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("get profiles: %w", err)
	}

	var profiles []MediaProfile
	for _, p := range rawProfiles {
		mp := profileToMediaProfile(p)

		streamResp, err := client.Dev.GetStreamURI(ctx, p.Token)
		if err == nil && streamResp != nil {
			uri := streamResp.URI
			if uri != "" && strings.HasPrefix(uri, "rtsp://") && username != "" {
				if u, parseErr := url.Parse(uri); parseErr == nil {
					u.User = url.UserPassword(username, password)
					uri = u.String()
				}
			}
			mp.StreamURI = uri
		}

		profiles = append(profiles, mp)
	}

	if len(profiles) == 0 {
		return nil, fmt.Errorf("no media profiles found")
	}

	return profiles, nil
}

// ProbeResult holds profiles, snapshot URI, and capability flags returned
// by a full device probe.
type ProbeResult struct {
	Profiles             []MediaProfile      `json:"profiles"`
	SnapshotURI          string              `json:"snapshot_uri,omitempty"`
	Capabilities         Capabilities        `json:"capabilities"`
	SupportedEventTopics []DetectedEventType `json:"supported_event_topics,omitempty"`
}

// ProbeDeviceFull connects to an ONVIF device and returns its media profiles,
// snapshot URI, and capability flags in a single call. It tries Media2 first
// and falls back to Media1 automatically.
func ProbeDeviceFull(xaddr, username, password string) (*ProbeResult, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("connect to ONVIF device: %w", err)
	}

	result := &ProbeResult{
		Capabilities: client.GetCapabilities(),
	}

	// Get profiles + stream URIs using Media2-first auto-detection.
	profiles, usedMedia2, err := GetProfilesAuto(xaddr, username, password)
	if err != nil {
		log.Printf("onvif probe [%s]: GetProfilesAuto failed: %v", xaddr, err)
		return result, nil
	}
	if len(profiles) == 0 {
		log.Printf("onvif probe [%s]: WARNING: 0 profiles returned (camera may not support GetProfiles or auth failed)", xaddr)
	}
	result.Profiles = profiles

	if usedMedia2 {
		log.Printf("onvif probe [%s]: used Media2 service", xaddr)
	}

	// Get snapshot URI for the first profile.
	if len(result.Profiles) > 0 {
		// Try Media2 snapshot URI first if supported.
		if client.HasService("media2") {
			snapURI, err := GetSnapshotUri2(client, result.Profiles[0].Token)
			if err == nil && snapURI != "" {
				result.SnapshotURI = snapURI
			}
		}
		// Fall back to Media1 snapshot URI.
		if result.SnapshotURI == "" {
			ctx := context.Background()
			snapResp, err := client.Dev.GetSnapshotURI(ctx, result.Profiles[0].Token)
			if err == nil && snapResp != nil {
				result.SnapshotURI = snapResp.URI
			}
		}
	}

	// Check audio backchannel capability.
	ctx := context.Background()
	outputs, err := client.Dev.GetAudioOutputs(ctx)
	if err == nil && len(outputs) > 0 && outputs[0].Token != "" {
		result.Capabilities.AudioBackchannel = true
	}

	// Discover supported event topics via GetEventProperties.
	if result.Capabilities.Events {
		eventURL := client.ServiceURL("events")
		if eventURL == "" {
			eventURL = client.ServiceURL("event")
		}
		if eventURL != "" {
			topics, err := GetEventPropertiesFromURL(ctx, eventURL, username, password)
			if err != nil {
				log.Printf("onvif probe [%s]: GetEventProperties failed: %v", xaddr, err)
			} else {
				result.SupportedEventTopics = topics
			}
		}
	}

	return result, nil
}

// profileToMediaProfile converts a library Profile to our MediaProfile type.
func profileToMediaProfile(p *onvifgo.Profile) MediaProfile {
	mp := MediaProfile{
		Token: p.Token,
		Name:  p.Name,
	}
	if p.VideoEncoderConfiguration != nil {
		mp.VideoCodec = p.VideoEncoderConfiguration.Encoding
		if p.VideoEncoderConfiguration.Resolution != nil {
			mp.Width = p.VideoEncoderConfiguration.Resolution.Width
			mp.Height = p.VideoEncoderConfiguration.Resolution.Height
		}
	}
	if p.AudioEncoderConfiguration != nil {
		mp.AudioCodec = p.AudioEncoderConfiguration.Encoding
	}
	return mp
}
