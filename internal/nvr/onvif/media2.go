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
)

// --- Media2 SOAP response types ---

type media2Envelope struct {
	XMLName xml.Name   `xml:"Envelope"`
	Body    media2Body `xml:"Body"`
}

type media2Body struct {
	GetProfilesResponse                        *getProfiles2Response                 `xml:"GetProfilesResponse"`
	GetStreamUriResponse                       *getStreamUri2Response                `xml:"GetStreamUriResponse"`
	GetSnapshotUriResponse                     *getSnapshotUri2Response              `xml:"GetSnapshotUriResponse"`
	CreateProfileResponse                      *createProfile2Response               `xml:"CreateProfileResponse"`
	DeleteProfileResponse                      *deleteProfile2Response               `xml:"DeleteProfileResponse"`
	AddConfigurationResponse                   *addConfiguration2Response            `xml:"AddConfigurationResponse"`
	RemoveConfigurationResponse                *removeConfiguration2Response         `xml:"RemoveConfigurationResponse"`
	GetVideoSourceConfigurationsResponse       *getVideoSourceConfigs2Response       `xml:"GetVideoSourceConfigurationsResponse"`
	SetVideoSourceConfigurationResponse        *setVideoSourceConfig2Response        `xml:"SetVideoSourceConfigurationResponse"`
	GetVideoSourceConfigurationOptionsResponse *getVideoSourceConfigOptions2Response `xml:"GetVideoSourceConfigurationOptionsResponse"`
	GetAudioSourceConfigurationsResponse       *getAudioSourceConfigs2Response       `xml:"GetAudioSourceConfigurationsResponse"`
	SetAudioSourceConfigurationResponse        *setAudioSourceConfig2Response        `xml:"SetAudioSourceConfigurationResponse"`
	Fault                                      *media2Fault                          `xml:"Fault"`
}

type media2Fault struct {
	Faultstring string `xml:"faultstring"`
}

type getProfiles2Response struct {
	Profiles []media2Profile `xml:"Profiles"`
}

type media2Profile struct {
	Token          string               `xml:"token,attr"`
	Name           string               `xml:"Name"`
	Configurations media2Configurations `xml:"Configurations"`
}

type media2Configurations struct {
	VideoSource  *media2VideoSourceConfig  `xml:"VideoSource"`
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

// --- Media2 Profile CRUD response types ---

type createProfile2Response struct {
	Token string `xml:"Token"`
	Name  string `xml:"Name"`
}

type deleteProfile2Response struct {
	// Empty — success is indicated by HTTP 200 with no fault.
}

type addConfiguration2Response struct {
	// Empty — success is indicated by HTTP 200 with no fault.
}

type removeConfiguration2Response struct {
	// Empty — success is indicated by HTTP 200 with no fault.
}

// --- Media2 Video Source Configuration response types ---

type media2VideoSourceConfig struct {
	Token       string        `xml:"token,attr"`
	Name        string        `xml:"Name"`
	SourceToken string        `xml:"SourceToken"`
	Bounds      *media2Bounds `xml:"Bounds"`
}

type media2Bounds struct {
	X      int `xml:"x,attr"`
	Y      int `xml:"y,attr"`
	Width  int `xml:"width,attr"`
	Height int `xml:"height,attr"`
}

type getVideoSourceConfigs2Response struct {
	Configurations []media2VideoSourceConfig `xml:"Configurations"`
}

type setVideoSourceConfig2Response struct {
	// Empty — success indicated by no fault.
}

type getVideoSourceConfigOptions2Response struct {
	Options media2VideoSourceConfigOptions `xml:"Options"`
}

type media2VideoSourceConfigOptions struct {
	BoundsRange             *media2IntRectangleRange `xml:"BoundsRange"`
	MaximumNumberOfProfiles int                      `xml:"MaximumNumberOfProfiles"`
}

type media2IntRectangleRange struct {
	XRange      media2IntRange `xml:"XRange"`
	YRange      media2IntRange `xml:"YRange"`
	WidthRange  media2IntRange `xml:"WidthRange"`
	HeightRange media2IntRange `xml:"HeightRange"`
}

type media2IntRange struct {
	Min int `xml:"Min"`
	Max int `xml:"Max"`
}

// --- Media2 Audio Source Configuration response types ---

type media2AudioSourceConfig struct {
	Token       string `xml:"token,attr"`
	Name        string `xml:"Name"`
	SourceToken string `xml:"SourceToken"`
}

type getAudioSourceConfigs2Response struct {
	Configurations []media2AudioSourceConfig `xml:"Configurations"`
}

type setAudioSourceConfig2Response struct {
	// Empty — success indicated by no fault.
}

var validConfigTypes = map[string]bool{
	"VideoSource": true, "VideoEncoder": true,
	"AudioSource": true, "AudioEncoder": true,
	"PTZ": true, "Analytics": true, "Metadata": true,
}

// media2SOAP builds a SOAP envelope with the tr2 namespace for Media2 requests.
func media2SOAP(innerBody string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tr2="http://www.onvif.org/ver20/media/wsdl"
            xmlns:tt="http://www.onvif.org/ver10/schema">
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
		if p.Configurations.VideoSource != nil {
			mp.VideoSourceToken = p.Configurations.VideoSource.SourceToken
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

	// Fall back to Media1 via the new library.
	ctx := context.Background()
	rawProfiles, err := client.Dev.GetProfiles(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("media1 GetProfiles: %w", err)
	}

	var profiles []MediaProfile
	for _, p := range rawProfiles {
		mp := profileToMediaProfile(p)

		streamResp, sErr := client.Dev.GetStreamURI(ctx, p.Token)
		if sErr == nil && streamResp != nil {
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

	return profiles, false, nil
}

// CreateProfile2 creates a new media profile via Media2.
func CreateProfile2(client *Client, name string) (string, string, error) {
	reqBody := fmt.Sprintf(`<tr2:CreateProfile>
      <tr2:Name>%s</tr2:Name>
    </tr2:CreateProfile>`, name)

	body, err := doMedia2SOAP(client, reqBody)
	if err != nil {
		return "", "", fmt.Errorf("media2 CreateProfile: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return "", "", fmt.Errorf("media2 CreateProfile parse: %w", err)
	}
	if env.Body.Fault != nil {
		return "", "", fmt.Errorf("media2 CreateProfile SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.CreateProfileResponse == nil {
		return "", "", fmt.Errorf("media2 CreateProfile: empty response")
	}

	return env.Body.CreateProfileResponse.Token, env.Body.CreateProfileResponse.Name, nil
}

// DeleteProfile2 deletes a media profile via Media2.
func DeleteProfile2(client *Client, token string) error {
	reqBody := fmt.Sprintf(`<tr2:DeleteProfile>
      <tr2:Token>%s</tr2:Token>
    </tr2:DeleteProfile>`, token)

	body, err := doMedia2SOAP(client, reqBody)
	if err != nil {
		return fmt.Errorf("media2 DeleteProfile: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("media2 DeleteProfile parse: %w", err)
	}
	if env.Body.Fault != nil {
		return fmt.Errorf("media2 DeleteProfile SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.DeleteProfileResponse == nil {
		return fmt.Errorf("media2 DeleteProfile: empty response")
	}

	return nil
}

// AddConfiguration2 adds a configuration to a profile via Media2.
// configType is one of: VideoSource, VideoEncoder, AudioSource, AudioEncoder, PTZ, Analytics, Metadata.
func AddConfiguration2(client *Client, profileToken, configType, configToken string) error {
	if !validConfigTypes[configType] {
		return fmt.Errorf("media2 AddConfiguration: invalid configuration type %q", configType)
	}
	reqBody := fmt.Sprintf(`<tr2:AddConfiguration>
      <tr2:ProfileToken>%s</tr2:ProfileToken>
      <tr2:Configuration>
        <tr2:Type>%s</tr2:Type>
        <tr2:Token>%s</tr2:Token>
      </tr2:Configuration>
    </tr2:AddConfiguration>`, profileToken, configType, configToken)

	body, err := doMedia2SOAP(client, reqBody)
	if err != nil {
		return fmt.Errorf("media2 AddConfiguration: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("media2 AddConfiguration parse: %w", err)
	}
	if env.Body.Fault != nil {
		return fmt.Errorf("media2 AddConfiguration SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.AddConfigurationResponse == nil {
		return fmt.Errorf("media2 AddConfiguration: empty response")
	}

	return nil
}

// RemoveConfiguration2 removes a configuration from a profile via Media2.
func RemoveConfiguration2(client *Client, profileToken, configType, configToken string) error {
	if !validConfigTypes[configType] {
		return fmt.Errorf("media2 RemoveConfiguration: invalid configuration type %q", configType)
	}
	reqBody := fmt.Sprintf(`<tr2:RemoveConfiguration>
      <tr2:ProfileToken>%s</tr2:ProfileToken>
      <tr2:Configuration>
        <tr2:Type>%s</tr2:Type>
        <tr2:Token>%s</tr2:Token>
      </tr2:Configuration>
    </tr2:RemoveConfiguration>`, profileToken, configType, configToken)

	body, err := doMedia2SOAP(client, reqBody)
	if err != nil {
		return fmt.Errorf("media2 RemoveConfiguration: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("media2 RemoveConfiguration parse: %w", err)
	}
	if env.Body.Fault != nil {
		return fmt.Errorf("media2 RemoveConfiguration SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.RemoveConfigurationResponse == nil {
		return fmt.Errorf("media2 RemoveConfiguration: empty response")
	}

	return nil
}

// GetVideoSourceConfigurations2 retrieves all video source configurations via Media2.
func GetVideoSourceConfigurations2(client *Client) ([]VideoSourceConfig, error) {
	body, err := doMedia2SOAP(client, `<tr2:GetVideoSourceConfigurations/>`)
	if err != nil {
		return nil, fmt.Errorf("media2 GetVideoSourceConfigurations: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("media2 GetVideoSourceConfigurations parse: %w", err)
	}
	if env.Body.Fault != nil {
		return nil, fmt.Errorf("media2 GetVideoSourceConfigurations SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetVideoSourceConfigurationsResponse == nil {
		return nil, fmt.Errorf("media2 GetVideoSourceConfigurations: empty response")
	}

	var configs []VideoSourceConfig
	for _, c := range env.Body.GetVideoSourceConfigurationsResponse.Configurations {
		cfg := VideoSourceConfig{
			Token:       c.Token,
			Name:        c.Name,
			SourceToken: c.SourceToken,
		}
		if c.Bounds != nil {
			cfg.Bounds = &IntRectangle{
				X:      c.Bounds.X,
				Y:      c.Bounds.Y,
				Width:  c.Bounds.Width,
				Height: c.Bounds.Height,
			}
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}

// SetVideoSourceConfiguration2 updates a video source configuration via Media2.
func SetVideoSourceConfiguration2(client *Client, cfg *VideoSourceConfig) error {
	boundsXML := ""
	if cfg.Bounds != nil {
		boundsXML = fmt.Sprintf(`<tt:Bounds x="%d" y="%d" width="%d" height="%d"/>`,
			cfg.Bounds.X, cfg.Bounds.Y, cfg.Bounds.Width, cfg.Bounds.Height)
	}

	reqBody := fmt.Sprintf(`<tr2:SetVideoSourceConfiguration>
      <tr2:Configuration token="%s">
        <tt:Name>%s</tt:Name>
        <tt:SourceToken>%s</tt:SourceToken>
        %s
      </tr2:Configuration>
    </tr2:SetVideoSourceConfiguration>`, cfg.Token, cfg.Name, cfg.SourceToken, boundsXML)

	body, err := doMedia2SOAP(client, reqBody)
	if err != nil {
		return fmt.Errorf("media2 SetVideoSourceConfiguration: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("media2 SetVideoSourceConfiguration parse: %w", err)
	}
	if env.Body.Fault != nil {
		return fmt.Errorf("media2 SetVideoSourceConfiguration SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.SetVideoSourceConfigurationResponse == nil {
		return fmt.Errorf("media2 SetVideoSourceConfiguration: empty response")
	}

	return nil
}

// GetVideoSourceConfigurationOptions2 retrieves the available options for a video source configuration via Media2.
func GetVideoSourceConfigurationOptions2(client *Client, configToken, profileToken string) (*VideoSourceConfigOptions, error) {
	inner := "<tr2:GetVideoSourceConfigurationOptions>"
	if configToken != "" {
		inner += fmt.Sprintf("<tr2:ConfigurationToken>%s</tr2:ConfigurationToken>", configToken)
	}
	if profileToken != "" {
		inner += fmt.Sprintf("<tr2:ProfileToken>%s</tr2:ProfileToken>", profileToken)
	}
	inner += "</tr2:GetVideoSourceConfigurationOptions>"

	body, err := doMedia2SOAP(client, inner)
	if err != nil {
		return nil, fmt.Errorf("media2 GetVideoSourceConfigurationOptions: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("media2 GetVideoSourceConfigurationOptions parse: %w", err)
	}
	if env.Body.Fault != nil {
		return nil, fmt.Errorf("media2 GetVideoSourceConfigurationOptions SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetVideoSourceConfigurationOptionsResponse == nil {
		return nil, fmt.Errorf("media2 GetVideoSourceConfigurationOptions: empty response")
	}

	opts := &VideoSourceConfigOptions{
		MaximumNumberOfProfiles: env.Body.GetVideoSourceConfigurationOptionsResponse.Options.MaximumNumberOfProfiles,
	}
	if br := env.Body.GetVideoSourceConfigurationOptionsResponse.Options.BoundsRange; br != nil {
		opts.BoundsRange = &IntRectangleRange{
			XRange:      Range{Min: br.XRange.Min, Max: br.XRange.Max},
			YRange:      Range{Min: br.YRange.Min, Max: br.YRange.Max},
			WidthRange:  Range{Min: br.WidthRange.Min, Max: br.WidthRange.Max},
			HeightRange: Range{Min: br.HeightRange.Min, Max: br.HeightRange.Max},
		}
	}

	return opts, nil
}

// GetAudioSourceConfigurations2 retrieves all audio source configurations via Media2.
func GetAudioSourceConfigurations2(client *Client) ([]AudioSourceConfig, error) {
	body, err := doMedia2SOAP(client, `<tr2:GetAudioSourceConfigurations/>`)
	if err != nil {
		return nil, fmt.Errorf("media2 GetAudioSourceConfigurations: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("media2 GetAudioSourceConfigurations parse: %w", err)
	}
	if env.Body.Fault != nil {
		return nil, fmt.Errorf("media2 GetAudioSourceConfigurations SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetAudioSourceConfigurationsResponse == nil {
		return nil, fmt.Errorf("media2 GetAudioSourceConfigurations: empty response")
	}

	var configs []AudioSourceConfig
	for _, c := range env.Body.GetAudioSourceConfigurationsResponse.Configurations {
		configs = append(configs, AudioSourceConfig{
			Token:       c.Token,
			Name:        c.Name,
			SourceToken: c.SourceToken,
		})
	}
	return configs, nil
}

// SetAudioSourceConfiguration2 updates an audio source configuration via Media2.
func SetAudioSourceConfiguration2(client *Client, cfg *AudioSourceConfig) error {
	reqBody := fmt.Sprintf(`<tr2:SetAudioSourceConfiguration>
      <tr2:Configuration token="%s">
        <tt:Name>%s</tt:Name>
        <tt:SourceToken>%s</tt:SourceToken>
      </tr2:Configuration>
    </tr2:SetAudioSourceConfiguration>`, cfg.Token, cfg.Name, cfg.SourceToken)

	body, err := doMedia2SOAP(client, reqBody)
	if err != nil {
		return fmt.Errorf("media2 SetAudioSourceConfiguration: %w", err)
	}

	var env media2Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("media2 SetAudioSourceConfiguration parse: %w", err)
	}
	if env.Body.Fault != nil {
		return fmt.Errorf("media2 SetAudioSourceConfiguration SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.SetAudioSourceConfigurationResponse == nil {
		return fmt.Errorf("media2 SetAudioSourceConfiguration: empty response")
	}

	return nil
}
