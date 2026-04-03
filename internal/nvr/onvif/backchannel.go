package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"

	onvifgo "github.com/EthanFlower1/onvif-go"
)

// --- Backchannel stream URI SOAP response types ---

type backchannelURIEnvelope struct {
	XMLName xml.Name            `xml:"Envelope"`
	Body    backchannelURIBody  `xml:"Body"`
}

type backchannelURIBody struct {
	GetStreamUriResponse *backchannelURIResponse `xml:"GetStreamUriResponse"`
	Fault                *soapFault              `xml:"Fault"`
}

type backchannelURIResponse struct {
	MediaUri struct {
		Uri string `xml:"Uri"`
	} `xml:"MediaUri"`
}

type soapFault struct {
	Faultstring string `xml:"faultstring"`
}

// backchannelStreamURIBody builds the inner SOAP XML for a Media1 GetStreamUri
// request using RTP-Unicast/RTSP transport.
func backchannelStreamURIBody(profileToken string) string {
	return fmt.Sprintf(`<trt:GetStreamUri xmlns:trt="http://www.onvif.org/ver10/media/wsdl"
                         xmlns:tt="http://www.onvif.org/ver10/schema">
      <trt:StreamSetup>
        <tt:Stream>RTP-Unicast</tt:Stream>
        <tt:Transport>
          <tt:Protocol>RTSP</tt:Protocol>
        </tt:Transport>
      </trt:StreamSetup>
      <trt:ProfileToken>%s</trt:ProfileToken>
    </trt:GetStreamUri>`, profileToken)
}

// backchannelSOAP wraps an inner body in a SOAP 1.2 envelope without Media2 namespaces.
func backchannelSOAP(innerBody string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Header></s:Header>
  <s:Body>
    %s
  </s:Body>
</s:Envelope>`, innerBody)
}

// GetBackchannelStreamURI retrieves the RTSP backchannel stream URI from the
// device's Media1 service using a custom SOAP call.
func GetBackchannelStreamURI(xaddr, username, password, profileToken string) (string, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return "", fmt.Errorf("backchannel stream uri: create client: %w", err)
	}

	mediaURL := client.ServiceURL("media")
	if mediaURL == "" {
		return "", fmt.Errorf("backchannel stream uri: device does not support Media service")
	}

	soapBody := backchannelSOAP(backchannelStreamURIBody(profileToken))

	if client.Username != "" {
		soapBody = injectWSSecurity(soapBody, client.Username, client.Password)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, mediaURL, strings.NewReader(soapBody))
	if err != nil {
		return "", fmt.Errorf("backchannel stream uri: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("backchannel stream uri: http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("backchannel stream uri: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("backchannel stream uri: SOAP fault (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var env backchannelURIEnvelope
	if err := xml.Unmarshal(respBody, &env); err != nil {
		return "", fmt.Errorf("backchannel stream uri: parse response: %w", err)
	}
	if env.Body.Fault != nil {
		return "", fmt.Errorf("backchannel stream uri: SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetStreamUriResponse == nil {
		return "", fmt.Errorf("backchannel stream uri: empty response")
	}

	return strings.TrimSpace(env.Body.GetStreamUriResponse.MediaUri.Uri), nil
}

// AudioOutputConfig holds the configuration for a device audio output.
type AudioOutputConfig struct {
	Token       string `json:"token"`
	Name        string `json:"name"`
	OutputToken string `json:"output_token"`
}

// AudioDecoderConfig holds the configuration for an audio decoder.
type AudioDecoderConfig struct {
	Token string `json:"token"`
	Name  string `json:"name"`
}

// AudioDecoderOptions describes the codecs a device can decode for backchannel audio.
type AudioDecoderOptions struct {
	AACSupported  bool         `json:"aac_supported"`
	G711Supported bool         `json:"g711_supported"`
	AAC           *CodecOptions `json:"aac,omitempty"`
	G711          *CodecOptions `json:"g711,omitempty"`
}

// CodecOptions holds the bitrate and sample-rate choices for a single codec.
type CodecOptions struct {
	Bitrates    []int `json:"bitrates"`
	SampleRates []int `json:"sample_rates"`
}

// BackchannelCodec is the negotiated codec to use for a backchannel session.
type BackchannelCodec struct {
	Encoding   string `json:"encoding"`
	Bitrate    int    `json:"bitrate"`
	SampleRate int    `json:"sample_rate"`
}

// GetAudioOutputs returns the token for every audio output on the device.
func GetAudioOutputs(xaddr, username, password string) ([]string, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	outputs, err := client.Dev.GetAudioOutputs(ctx)
	if err != nil {
		return nil, fmt.Errorf("get audio outputs: %w", err)
	}

	var tokens []string
	for _, o := range outputs {
		tokens = append(tokens, o.Token)
	}
	return tokens, nil
}

// GetAudioOutputConfigs returns all audio output configurations from the device.
func GetAudioOutputConfigs(xaddr, username, password string) ([]*AudioOutputConfig, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	cfgs, err := client.Dev.GetAudioOutputConfigurations(ctx)
	if err != nil {
		return nil, fmt.Errorf("get audio output configurations: %w", err)
	}

	var result []*AudioOutputConfig
	for _, c := range cfgs {
		result = append(result, &AudioOutputConfig{
			Token:       c.Token,
			Name:        c.Name,
			OutputToken: c.OutputToken,
		})
	}
	return result, nil
}

// SetAudioOutputConfig pushes an audio output configuration to the device.
func SetAudioOutputConfig(xaddr, username, password string, cfg *AudioOutputConfig) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	oc := &onvifgo.AudioOutputConfiguration{
		Token:       cfg.Token,
		Name:        cfg.Name,
		OutputToken: cfg.OutputToken,
	}

	ctx := context.Background()
	if err := client.Dev.SetAudioOutputConfiguration(ctx, oc, true); err != nil {
		return fmt.Errorf("set audio output configuration: %w", err)
	}
	return nil
}

// GetAudioDecoderConfigs returns all audio decoder configurations from the device.
func GetAudioDecoderConfigs(xaddr, username, password string) ([]*AudioDecoderConfig, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	cfgs, err := client.Dev.GetAudioDecoderConfigurations(ctx)
	if err != nil {
		return nil, fmt.Errorf("get audio decoder configurations: %w", err)
	}

	var result []*AudioDecoderConfig
	for _, c := range cfgs {
		result = append(result, &AudioDecoderConfig{
			Token: c.Token,
			Name:  c.Name,
		})
	}
	return result, nil
}

// SetAudioDecoderConfig pushes an audio decoder configuration to the device.
func SetAudioDecoderConfig(xaddr, username, password string, cfg *AudioDecoderConfig) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	dc := &onvifgo.AudioDecoderConfiguration{
		Token: cfg.Token,
		Name:  cfg.Name,
	}

	ctx := context.Background()
	if err := client.Dev.SetAudioDecoderConfiguration(ctx, dc, true); err != nil {
		return fmt.Errorf("set audio decoder configuration: %w", err)
	}
	return nil
}

// GetAudioDecoderOpts returns the decoder options for a given configuration token.
func GetAudioDecoderOpts(xaddr, username, password, token string) (*AudioDecoderOptions, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	raw, err := client.Dev.GetAudioDecoderConfigurationOptions(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("get audio decoder options: %w", err)
	}

	opts := &AudioDecoderOptions{}
	if raw.AACDecOptions != nil {
		opts.AACSupported = true
		bitrates := make([]int, len(raw.AACDecOptions.BitrateList))
		copy(bitrates, raw.AACDecOptions.BitrateList)
		sampleRates := make([]int, len(raw.AACDecOptions.SampleRateList))
		copy(sampleRates, raw.AACDecOptions.SampleRateList)
		opts.AAC = &CodecOptions{
			Bitrates:    bitrates,
			SampleRates: sampleRates,
		}
	}
	if raw.G711DecOptions != nil {
		opts.G711Supported = true
		bitrates := make([]int, len(raw.G711DecOptions.BitrateList))
		copy(bitrates, raw.G711DecOptions.BitrateList)
		sampleRates := make([]int, len(raw.G711DecOptions.SampleRateList))
		copy(sampleRates, raw.G711DecOptions.SampleRateList)
		opts.G711 = &CodecOptions{
			Bitrates:    bitrates,
			SampleRates: sampleRates,
		}
	}
	return opts, nil
}

// NegotiateCodec picks the best codec from the device options.
// AAC is preferred over G.711. Returns nil when neither codec is supported.
func NegotiateCodec(opts *AudioDecoderOptions) *BackchannelCodec {
	if opts == nil {
		return nil
	}

	if opts.AACSupported && opts.AAC != nil && len(opts.AAC.Bitrates) > 0 && len(opts.AAC.SampleRates) > 0 {
		return &BackchannelCodec{
			Encoding:   "AAC",
			Bitrate:    opts.AAC.Bitrates[0],
			SampleRate: opts.AAC.SampleRates[0],
		}
	}

	if opts.G711Supported && opts.G711 != nil && len(opts.G711.Bitrates) > 0 && len(opts.G711.SampleRates) > 0 {
		return &BackchannelCodec{
			Encoding:   "G711",
			Bitrate:    opts.G711.Bitrates[0],
			SampleRate: opts.G711.SampleRates[0],
		}
	}

	return nil
}

// AddAudioOutputToProfile adds an audio output configuration to a media profile.
func AddAudioOutputToProfile(xaddr, username, password, profileToken, configToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.AddAudioOutputConfiguration(ctx, profileToken, configToken); err != nil {
		return fmt.Errorf("add audio output to profile: %w", err)
	}
	return nil
}

// AddAudioDecoderToProfile adds an audio decoder configuration to a media profile.
func AddAudioDecoderToProfile(xaddr, username, password, profileToken, configToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.AddAudioDecoderConfiguration(ctx, profileToken, configToken); err != nil {
		return fmt.Errorf("add audio decoder to profile: %w", err)
	}
	return nil
}
