package onvif

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"

	onviflib "github.com/use-go/onvif"
	onvifmedia "github.com/use-go/onvif/media"
	sdkmedia "github.com/use-go/onvif/sdk/media"
	onviftypes "github.com/use-go/onvif/xsd/onvif"
)

// ErrNotImplemented is returned by stub methods that are not yet implemented.
var ErrNotImplemented = errors.New("not implemented")

// ProbeDevice connects to an ONVIF device with credentials and returns its media profiles
// including RTSP stream URIs.
func ProbeDevice(xaddr, username, password string) ([]MediaProfile, error) {
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
		return nil, fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	profilesResp, err := sdkmedia.Call_GetProfiles(ctx, dev, onvifmedia.GetProfiles{})
	if err != nil {
		return nil, fmt.Errorf("get profiles: %w", err)
	}

	var profiles []MediaProfile
	for _, p := range profilesResp.Profiles {
		mp := MediaProfile{
			Token:      string(p.Token),
			Name:       string(p.Name),
			VideoCodec: string(p.VideoEncoderConfiguration.Encoding),
			Width:      int(p.VideoEncoderConfiguration.Resolution.Width),
			Height:     int(p.VideoEncoderConfiguration.Resolution.Height),
		}

		streamResp, err := sdkmedia.Call_GetStreamUri(ctx, dev, onvifmedia.GetStreamUri{
			ProfileToken: p.Token,
			StreamSetup: onviftypes.StreamSetup{
				Stream:    "RTP-Unicast",
				Transport: onviftypes.Transport{Protocol: "RTSP"},
			},
		})
		if err == nil {
			uri := string(streamResp.MediaUri.Uri)
			// Inject credentials into the RTSP URL.
			if username != "" {
				if u, err := url.Parse(uri); err == nil {
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
	Profiles     []MediaProfile `json:"profiles"`
	SnapshotURI  string         `json:"snapshot_uri,omitempty"`
	Capabilities Capabilities   `json:"capabilities"`
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
		return result, nil // return what we have
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
			snapResp, err := sdkmedia.Call_GetSnapshotUri(ctx, client.Dev, onvifmedia.GetSnapshotUri{
				ProfileToken: onviftypes.ReferenceToken(result.Profiles[0].Token),
			})
			if err == nil {
				result.SnapshotURI = string(snapResp.MediaUri.Uri)
			}
		}
	}

	// Check audio backchannel capability.
	ctx := context.Background()
	outputResp, err := sdkmedia.Call_GetAudioOutputs(ctx, client.Dev, onvifmedia.GetAudioOutputs{})
	if err == nil && string(outputResp.AudioOutputs.Token) != "" {
		result.Capabilities.AudioBackchannel = true
	}

	return result, nil
}
