package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	onvifmedia "github.com/use-go/onvif/media"
	sdkmedia "github.com/use-go/onvif/sdk/media"
	onviftypes "github.com/use-go/onvif/xsd/onvif"
)

// --- Media2 SOAP response types ---

type media2Envelope struct {
	XMLName xml.Name   `xml:"Envelope"`
	Body    media2Body `xml:"Body"`
}

type media2Body struct {
	GetProfilesResponse    *getProfiles2Response    `xml:"GetProfilesResponse"`
	GetStreamUriResponse   *getStreamUri2Response   `xml:"GetStreamUriResponse"`
	GetSnapshotUriResponse *getSnapshotUri2Response `xml:"GetSnapshotUriResponse"`
	Fault                  *media2Fault             `xml:"Fault"`
}

type media2Fault struct {
	Faultstring string `xml:"faultstring"`
}

type getProfiles2Response struct {
	Profiles []media2Profile `xml:"Profiles"`
}

type media2Profile struct {
	Token         string                    `xml:"token,attr"`
	Name          string                    `xml:"Name"`
	Configurations media2Configurations     `xml:"Configurations"`
}

type media2Configurations struct {
	VideoEncoder *media2VideoEncoderConfig `xml:"VideoEncoder"`
	AudioEncoder *media2AudioEncoderConfig `xml:"AudioEncoder"`
}

type media2VideoEncoderConfig struct {
	Encoding   string           `xml:"Encoding"`
	Resolution media2Resolution `xml:"Resolution"`
}

type media2Resolution struct {
	Width  int `xml:"Width"`
	Height int `xml:"Height"`
}

type media2AudioEncoderConfig struct {
	Encoding string `xml:"Encoding"`
}

type getStreamUri2Response struct {
	Uri string `xml:"Uri"`
}

type getSnapshotUri2Response struct {
	Uri string `xml:"Uri"`
}

// media2SOAP builds a SOAP envelope with the tr2 namespace for Media2 requests.
func media2SOAP(innerBody string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tr2="http://www.onvif.org/ver20/media/wsdl">
  <s:Header></s:Header>
  <s:Body>
    %s
  </s:Body>
</s:Envelope>`, innerBody)
}

// doMedia2SOAP sends a SOAP request to the Media2 service endpoint.
func doMedia2SOAP(client *Client, body string) ([]byte, error) {
	media2URL := client.ServiceURL("media2")
	if media2URL == "" {
		return nil, fmt.Errorf("device does not support Media2 service")
	}

	soapBody := media2SOAP(body)

	// Inject WS-Security if credentials are present.
	if client.Username != "" {
		soapBody = injectWSSecurity(soapBody, client.Username, client.Password)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, media2URL, strings.NewReader(soapBody))
	if err != nil {
		return nil, fmt.Errorf("create media2 request: %w", err)
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("media2 http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("media2 read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("media2 SOAP fault (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	return respBody, nil
}

// --- Public functions ---

// GetProfiles2 queries the Media2 service for profiles and converts them
// to the shared MediaProfile type.
func GetProfiles2(client *Client) ([]MediaProfile, error) {
	body, err := doMedia2SOAP(client, `<tr2:GetProfiles/>`)
	if err != nil {
		return nil, fmt.Errorf("media2 GetProfiles: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("media2 GetProfiles parse: %w", err)
	}
	if env.Body.Fault != nil {
		return nil, fmt.Errorf("media2 GetProfiles SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetProfilesResponse == nil {
		return nil, fmt.Errorf("media2 GetProfiles: empty response")
	}

	var profiles []MediaProfile
	for _, p := range env.Body.GetProfilesResponse.Profiles {
		mp := MediaProfile{
			Token: p.Token,
			Name:  p.Name,
		}
		if p.Configurations.VideoEncoder != nil {
			mp.VideoCodec = p.Configurations.VideoEncoder.Encoding
			mp.Width = p.Configurations.VideoEncoder.Resolution.Width
			mp.Height = p.Configurations.VideoEncoder.Resolution.Height
		}
		if p.Configurations.AudioEncoder != nil {
			mp.AudioCodec = p.Configurations.AudioEncoder.Encoding
		}
		profiles = append(profiles, mp)
	}

	return profiles, nil
}

// GetStreamUri2 retrieves the RTSP stream URI for a profile via Media2.
func GetStreamUri2(client *Client, profileToken string) (string, error) {
	reqBody := fmt.Sprintf(`<tr2:GetStreamUri>
      <tr2:Protocol>RtspUnicast</tr2:Protocol>
      <tr2:ProfileToken>%s</tr2:ProfileToken>
    </tr2:GetStreamUri>`, profileToken)

	body, err := doMedia2SOAP(client, reqBody)
	if err != nil {
		return "", fmt.Errorf("media2 GetStreamUri: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("media2 GetStreamUri parse: %w", err)
	}
	if env.Body.Fault != nil {
		return "", fmt.Errorf("media2 GetStreamUri SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetStreamUriResponse == nil {
		return "", fmt.Errorf("media2 GetStreamUri: empty response")
	}

	return strings.TrimSpace(env.Body.GetStreamUriResponse.Uri), nil
}

// GetSnapshotUri2 retrieves the snapshot URI for a profile via Media2.
func GetSnapshotUri2(client *Client, profileToken string) (string, error) {
	reqBody := fmt.Sprintf(`<tr2:GetSnapshotUri>
      <tr2:ProfileToken>%s</tr2:ProfileToken>
    </tr2:GetSnapshotUri>`, profileToken)

	body, err := doMedia2SOAP(client, reqBody)
	if err != nil {
		return "", fmt.Errorf("media2 GetSnapshotUri: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("media2 GetSnapshotUri parse: %w", err)
	}
	if env.Body.Fault != nil {
		return "", fmt.Errorf("media2 GetSnapshotUri SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetSnapshotUriResponse == nil {
		return "", fmt.Errorf("media2 GetSnapshotUri: empty response")
	}

	return strings.TrimSpace(env.Body.GetSnapshotUriResponse.Uri), nil
}

// GetProfilesAuto tries Media2 first, then falls back to Media1.
// Returns (profiles, usedMedia2, error).
func GetProfilesAuto(xaddr, username, password string) ([]MediaProfile, bool, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, false, fmt.Errorf("connect to ONVIF device: %w", err)
	}

	// Try Media2 first.
	if client.HasService("media2") {
		profiles, err := GetProfiles2(client)
		if err == nil && len(profiles) > 0 {
			// Enrich with stream URIs via Media2.
			for i := range profiles {
				uri, sErr := GetStreamUri2(client, profiles[i].Token)
				if sErr == nil {
					if uri != "" && strings.HasPrefix(uri, "rtsp://") && username != "" {
						if u, parseErr := url.Parse(uri); parseErr == nil {
							u.User = url.UserPassword(username, password)
							uri = u.String()
						}
					}
					profiles[i].StreamURI = uri
				}
			}
			log.Printf("onvif media2 [%s]: got %d profiles via Media2", xaddr, len(profiles))
			return profiles, true, nil
		}
		log.Printf("onvif media2 [%s]: Media2 GetProfiles failed (%v), falling back to Media1", xaddr, err)
	}

	// Fall back to Media1.
	ctx := context.Background()
	profilesResp, err := sdkmedia.Call_GetProfiles(ctx, client.Dev, onvifmedia.GetProfiles{})
	if err != nil {
		return nil, false, fmt.Errorf("media1 GetProfiles: %w", err)
	}

	var profiles []MediaProfile
	for _, p := range profilesResp.Profiles {
		mp := MediaProfile{
			Token:      string(p.Token),
			Name:       string(p.Name),
			VideoCodec: string(p.VideoEncoderConfiguration.Encoding),
			AudioCodec: string(p.AudioEncoderConfiguration.Encoding),
			Width:      int(p.VideoEncoderConfiguration.Resolution.Width),
			Height:     int(p.VideoEncoderConfiguration.Resolution.Height),
		}

		streamResp, sErr := sdkmedia.Call_GetStreamUri(ctx, client.Dev, onvifmedia.GetStreamUri{
			ProfileToken: p.Token,
			StreamSetup: onviftypes.StreamSetup{
				Stream:    "RTP-Unicast",
				Transport: onviftypes.Transport{Protocol: "RTSP"},
			},
		})
		if sErr == nil {
			uri := string(streamResp.MediaUri.Uri)
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

	return profiles, false, nil
}
