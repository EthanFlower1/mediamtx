package onvif

import (
	"context"
	"errors"
	"fmt"
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
