package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// MulticastConfig holds a camera's multicast streaming settings.
type MulticastConfig struct {
	Address   string `json:"address"`
	Port      int    `json:"port"`
	TTL       int    `json:"ttl"`
	AutoStart bool   `json:"auto_start"`
}

// ValidateMulticastAddress checks that addr is a valid IPv4 multicast address
// in the range 224.0.0.0–239.255.255.255.
func ValidateMulticastAddress(addr string) error {
	if addr == "" {
		return fmt.Errorf("multicast address is empty")
	}
	ip := net.ParseIP(addr)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", addr)
	}
	ip = ip.To4()
	if ip == nil {
		return fmt.Errorf("not an IPv4 address: %s", addr)
	}
	if !ip.IsMulticast() {
		return fmt.Errorf("address %s is not in the multicast range (224.0.0.0–239.255.255.255)", addr)
	}
	return nil
}

// --- SOAP response types for multicast config ---

type multicastConfigEnvelope struct {
	XMLName xml.Name            `xml:"Envelope"`
	Body    multicastConfigBody `xml:"Body"`
}

type multicastConfigBody struct {
	GetVideoEncoderConfigurationResponse *vecResponse `xml:"GetVideoEncoderConfigurationResponse"`
	Fault                                *soapFault   `xml:"Fault"`
}

type soapFault struct {
	Faultstring string `xml:"faultstring"`
}

type vecResponse struct {
	Configuration vecConfiguration `xml:"Configuration"`
}

type vecConfiguration struct {
	Token     string          `xml:"token,attr"`
	Name      string          `xml:"Name"`
	Multicast *multicastBlock `xml:"Multicast"`
}

type multicastBlock struct {
	Address   multicastAddress `xml:"Address"`
	Port      int              `xml:"Port"`
	TTL       int              `xml:"TTL"`
	AutoStart bool             `xml:"AutoStart"`
}

type multicastAddress struct {
	Type        string `xml:"Type"`
	IPv4Address string `xml:"IPv4Address"`
}

// parseMulticastConfigResponse parses a GetVideoEncoderConfiguration SOAP
// response and extracts the multicast settings.
func parseMulticastConfigResponse(data []byte) (*MulticastConfig, error) {
	var env multicastConfigEnvelope
	if err := xml.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("parse multicast config: %w", err)
	}
	if env.Body.Fault != nil {
		return nil, fmt.Errorf("SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	resp := env.Body.GetVideoEncoderConfigurationResponse
	if resp == nil {
		return nil, fmt.Errorf("empty GetVideoEncoderConfigurationResponse")
	}
	if resp.Configuration.Multicast == nil {
		return nil, fmt.Errorf("camera does not report multicast configuration")
	}
	mc := resp.Configuration.Multicast
	return &MulticastConfig{
		Address:   mc.Address.IPv4Address,
		Port:      mc.Port,
		TTL:       mc.TTL,
		AutoStart: mc.AutoStart,
	}, nil
}

// --- SOAP response types for Media1 GetStreamUri ---

type media1StreamUriEnvelope struct {
	XMLName xml.Name            `xml:"Envelope"`
	Body    media1StreamUriBody `xml:"Body"`
}

type media1StreamUriBody struct {
	GetStreamUriResponse *media1StreamUriResponse `xml:"GetStreamUriResponse"`
	Fault                *soapFault               `xml:"Fault"`
}

type media1StreamUriResponse struct {
	MediaUri media1MediaUri `xml:"MediaUri"`
}

type media1MediaUri struct {
	Uri string `xml:"Uri"`
}

// parseMedia1StreamUriResponse parses a Media1 GetStreamUri SOAP response.
func parseMedia1StreamUriResponse(data []byte) (string, error) {
	var env media1StreamUriEnvelope
	if err := xml.Unmarshal(data, &env); err != nil {
		return "", fmt.Errorf("parse stream URI: %w", err)
	}
	if env.Body.Fault != nil {
		return "", fmt.Errorf("SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetStreamUriResponse == nil {
		return "", fmt.Errorf("empty GetStreamUriResponse")
	}
	return strings.TrimSpace(env.Body.GetStreamUriResponse.MediaUri.Uri), nil
}

// --- Media1 SOAP helper ---

func media1SOAP(innerBody string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:trt="http://www.onvif.org/ver10/media/wsdl"
            xmlns:tt="http://www.onvif.org/ver10/schema">
  <s:Header></s:Header>
  <s:Body>
    %s
  </s:Body>
</s:Envelope>`, innerBody)
}

func doMedia1SOAP(client *Client, body string) ([]byte, error) {
	mediaURL := client.ServiceURL("media")
	if mediaURL == "" {
		return nil, fmt.Errorf("device does not support Media service")
	}

	soapBody := media1SOAP(body)
	if client.Username != "" {
		soapBody = injectWSSecurity(soapBody, client.Username, client.Password)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mediaURL, strings.NewReader(soapBody))
	if err != nil {
		return nil, fmt.Errorf("create media1 request: %w", err)
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("media1 http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("media1 read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("media1 SOAP fault (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	return respBody, nil
}

// --- Public functions ---

// GetMulticastConfig retrieves the camera's current multicast settings
// from the video encoder configuration for the given profile token.
func GetMulticastConfig(xaddr, username, password, profileToken string) (*MulticastConfig, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("GetMulticastConfig: connect: %w", err)
	}

	// Get the video encoder configuration token from the profile.
	ctx := context.Background()
	profiles, err := client.Dev.GetProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetMulticastConfig: get profiles: %w", err)
	}

	var encoderToken string
	for _, p := range profiles {
		if p.Token == profileToken && p.VideoEncoderConfiguration != nil {
			encoderToken = p.VideoEncoderConfiguration.Token
			break
		}
	}
	if encoderToken == "" {
		return nil, fmt.Errorf("GetMulticastConfig: no video encoder found for profile %s", profileToken)
	}

	// Fetch the full encoder config via SOAP to get multicast block.
	reqBody := fmt.Sprintf(`<trt:GetVideoEncoderConfiguration>
      <trt:ConfigurationToken>%s</trt:ConfigurationToken>
    </trt:GetVideoEncoderConfiguration>`, xmlEscape(encoderToken))

	data, err := doMedia1SOAP(client, reqBody)
	if err != nil {
		return nil, fmt.Errorf("GetMulticastConfig: %w", err)
	}

	return parseMulticastConfigResponse(data)
}

// SetMulticastConfig updates the camera's multicast address, port, and TTL
// on the video encoder configuration for the given profile.
func SetMulticastConfig(xaddr, username, password, profileToken string, cfg *MulticastConfig) error {
	if err := ValidateMulticastAddress(cfg.Address); err != nil {
		return fmt.Errorf("SetMulticastConfig: %w", err)
	}

	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return fmt.Errorf("SetMulticastConfig: connect: %w", err)
	}

	// Look up the encoder token from the profile.
	ctx := context.Background()
	profiles, err := client.Dev.GetProfiles(ctx)
	if err != nil {
		return fmt.Errorf("SetMulticastConfig: get profiles: %w", err)
	}

	var encoderToken, encoderName, encoding string
	var width, height int
	var quality float64
	for _, p := range profiles {
		if p.Token == profileToken && p.VideoEncoderConfiguration != nil {
			vec := p.VideoEncoderConfiguration
			encoderToken = vec.Token
			encoderName = vec.Name
			encoding = vec.Encoding
			quality = vec.Quality
			if vec.Resolution != nil {
				width = vec.Resolution.Width
				height = vec.Resolution.Height
			}
			break
		}
	}
	if encoderToken == "" {
		return fmt.Errorf("SetMulticastConfig: no video encoder found for profile %s", profileToken)
	}

	reqBody := fmt.Sprintf(`<trt:SetVideoEncoderConfiguration>
      <trt:Configuration token="%s">
        <tt:Name>%s</tt:Name>
        <tt:Encoding>%s</tt:Encoding>
        <tt:Resolution>
          <tt:Width>%d</tt:Width>
          <tt:Height>%d</tt:Height>
        </tt:Resolution>
        <tt:Quality>%.1f</tt:Quality>
        <tt:Multicast>
          <tt:Address>
            <tt:Type>IPv4</tt:Type>
            <tt:IPv4Address>%s</tt:IPv4Address>
          </tt:Address>
          <tt:Port>%d</tt:Port>
          <tt:TTL>%d</tt:TTL>
          <tt:AutoStart>false</tt:AutoStart>
        </tt:Multicast>
      </trt:Configuration>
      <trt:ForcePersistence>true</trt:ForcePersistence>
    </trt:SetVideoEncoderConfiguration>`,
		xmlEscape(encoderToken), xmlEscape(encoderName), xmlEscape(encoding),
		width, height, quality,
		xmlEscape(cfg.Address), cfg.Port, cfg.TTL)

	data, err := doMedia1SOAP(client, reqBody)
	if err != nil {
		return fmt.Errorf("SetMulticastConfig: %w", err)
	}

	// Check for SOAP fault in response.
	type faultEnvelope struct {
		XMLName xml.Name `xml:"Envelope"`
		Body    struct {
			Fault *soapFault `xml:"Fault"`
		} `xml:"Body"`
	}
	var env faultEnvelope
	if err := xml.Unmarshal(data, &env); err == nil && env.Body.Fault != nil {
		return fmt.Errorf("SetMulticastConfig: SOAP fault: %s", env.Body.Fault.Faultstring)
	}

	return nil
}

// GetStreamUriMulticast retrieves the multicast stream URI for a profile.
// It tries Media2 first, then falls back to Media1.
func GetStreamUriMulticast(xaddr, username, password, profileToken string) (string, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return "", fmt.Errorf("GetStreamUriMulticast: connect: %w", err)
	}

	// Try Media2 first.
	if client.HasService("media2") {
		reqBody := fmt.Sprintf(`<tr2:GetStreamUri>
          <tr2:Protocol>RtspMulticast</tr2:Protocol>
          <tr2:ProfileToken>%s</tr2:ProfileToken>
        </tr2:GetStreamUri>`, xmlEscape(profileToken))

		data, err := doMedia2SOAP(client, reqBody)
		if err == nil {
			var env media2Envelope
			if xmlErr := xml.Unmarshal(data, &env); xmlErr == nil &&
				env.Body.Fault == nil &&
				env.Body.GetStreamUriResponse != nil {
				uri := strings.TrimSpace(env.Body.GetStreamUriResponse.Uri)
				if uri != "" {
					return uri, nil
				}
			}
		}
		// Fall through to Media1 on failure.
	}

	// Media1 fallback: use StreamSetup with RTP-Multicast.
	reqBody := fmt.Sprintf(`<trt:GetStreamUri>
      <trt:StreamSetup>
        <tt:Stream>RTP-Multicast</tt:Stream>
        <tt:Transport>
          <tt:Protocol>UDP</tt:Protocol>
        </tt:Transport>
      </trt:StreamSetup>
      <trt:ProfileToken>%s</trt:ProfileToken>
    </trt:GetStreamUri>`, xmlEscape(profileToken))

	data, err := doMedia1SOAP(client, reqBody)
	if err != nil {
		return "", fmt.Errorf("GetStreamUriMulticast: %w", err)
	}

	return parseMedia1StreamUriResponse(data)
}
