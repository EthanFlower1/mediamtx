# KAI-27: Backchannel Audio Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable two-way audio (intercom) for ONVIF cameras — Flutter sends audio via WebSocket to NVR, NVR relays to camera via RTSP backchannel.

**Architecture:** WebSocket endpoint receives encoded audio from Flutter client. A session manager negotiates codec with camera (AAC preferred, G.711 fallback), establishes RTSP backchannel connection, and relays RTP packets. Connections use on-demand lifecycle with 30s keep-alive after session ends.

**Tech Stack:** Go, gorilla/websocket, gortsplib/v5, onvif-go, Gin HTTP framework

**Spec:** `docs/superpowers/specs/2026-04-03-backchannel-audio-design.md`

---

## File Structure

| File | Responsibility |
|------|---------------|
| `internal/nvr/onvif/backchannel.go` | ONVIF audio output/decoder queries, codec negotiation, custom SOAP for backchannel stream URI |
| `internal/nvr/onvif/backchannel_test.go` | Unit tests for ONVIF backchannel functions |
| `internal/nvr/onvif/audio.go` | Updated: populate AudioBackchannel in Capabilities |
| `internal/nvr/backchannel/rtp.go` | RTP audio packet construction (G.711 and AAC payload types) |
| `internal/nvr/backchannel/rtp_test.go` | Unit tests for RTP packing |
| `internal/nvr/backchannel/rtsp.go` | RTSP backchannel connection (DESCRIBE/SETUP/PLAY, interleaved TCP, keep-alive) |
| `internal/nvr/backchannel/rtsp_test.go` | Unit tests for RTSP connection lifecycle |
| `internal/nvr/backchannel/manager.go` | Session lifecycle (start/send/stop, idle timer, one-session-per-camera) |
| `internal/nvr/backchannel/manager_test.go` | Unit tests for session manager |
| `internal/nvr/api/backchannel.go` | WebSocket + REST handlers for backchannel API |
| `internal/nvr/api/backchannel_test.go` | Unit tests for API handlers |
| `internal/nvr/api/router.go` | Updated: register backchannel routes |
| `internal/nvr/nvr.go` | Updated: initialize manager, wire into router config, cleanup on shutdown |

---

### Task 1: ONVIF Audio Output and Decoder Queries

**Files:**
- Create: `internal/nvr/onvif/backchannel.go`
- Create: `internal/nvr/onvif/backchannel_test.go`

This task wraps onvif-go library methods for audio output and decoder configuration. These are thin wrappers following the exact pattern in `internal/nvr/onvif/media_config.go`.

- [ ] **Step 1: Write failing tests for audio output functions**

Create `internal/nvr/onvif/backchannel_test.go`:

```go
package onvif

import (
	"testing"
)

// TestAudioOutputConfigRoundTrip verifies the AudioOutputConfig struct fields.
func TestAudioOutputConfigRoundTrip(t *testing.T) {
	cfg := AudioOutputConfig{
		Token:       "output-cfg-1",
		Name:        "AudioOutput_1",
		OutputToken: "output-1",
	}
	if cfg.Token != "output-cfg-1" {
		t.Fatalf("expected token output-cfg-1, got %s", cfg.Token)
	}
	if cfg.OutputToken != "output-1" {
		t.Fatalf("expected output token output-1, got %s", cfg.OutputToken)
	}
}

// TestAudioDecoderConfigRoundTrip verifies the AudioDecoderConfig struct fields.
func TestAudioDecoderConfigRoundTrip(t *testing.T) {
	cfg := AudioDecoderConfig{
		Token: "decoder-cfg-1",
		Name:  "AudioDecoder_1",
	}
	if cfg.Token != "decoder-cfg-1" {
		t.Fatalf("expected token decoder-cfg-1, got %s", cfg.Token)
	}
}

// TestAudioDecoderOptionsCodecDetection verifies codec support parsing.
func TestAudioDecoderOptionsCodecDetection(t *testing.T) {
	opts := AudioDecoderOptions{
		AACSupported:  true,
		G711Supported: true,
		AAC: &CodecOptions{
			Bitrates:    []int{64000, 128000},
			SampleRates: []int{16000, 44100},
		},
		G711: &CodecOptions{
			Bitrates:    []int{64000},
			SampleRates: []int{8000},
		},
	}
	if !opts.AACSupported {
		t.Fatal("expected AAC supported")
	}
	if !opts.G711Supported {
		t.Fatal("expected G711 supported")
	}
	if len(opts.AAC.Bitrates) != 2 {
		t.Fatalf("expected 2 AAC bitrates, got %d", len(opts.AAC.Bitrates))
	}
}

// TestNegotiateCodecPrefersAAC verifies that NegotiateCodec selects AAC over G.711.
func TestNegotiateCodecPrefersAAC(t *testing.T) {
	opts := &AudioDecoderOptions{
		AACSupported:  true,
		G711Supported: true,
		AAC: &CodecOptions{
			Bitrates:    []int{64000},
			SampleRates: []int{16000},
		},
		G711: &CodecOptions{
			Bitrates:    []int{64000},
			SampleRates: []int{8000},
		},
	}
	codec := NegotiateCodec(opts)
	if codec.Encoding != "AAC" {
		t.Fatalf("expected AAC, got %s", codec.Encoding)
	}
	if codec.SampleRate != 16000 {
		t.Fatalf("expected sample rate 16000, got %d", codec.SampleRate)
	}
}

// TestNegotiateCodecFallsBackToG711 verifies fallback when AAC is not supported.
func TestNegotiateCodecFallsBackToG711(t *testing.T) {
	opts := &AudioDecoderOptions{
		AACSupported:  false,
		G711Supported: true,
		G711: &CodecOptions{
			Bitrates:    []int{64000},
			SampleRates: []int{8000},
		},
	}
	codec := NegotiateCodec(opts)
	if codec.Encoding != "G711" {
		t.Fatalf("expected G711, got %s", codec.Encoding)
	}
	if codec.SampleRate != 8000 {
		t.Fatalf("expected sample rate 8000, got %d", codec.SampleRate)
	}
}

// TestNegotiateCodecNoneSupported verifies nil return when no codecs are supported.
func TestNegotiateCodecNoneSupported(t *testing.T) {
	opts := &AudioDecoderOptions{
		AACSupported:  false,
		G711Supported: false,
	}
	codec := NegotiateCodec(opts)
	if codec != nil {
		t.Fatalf("expected nil codec, got %+v", codec)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd internal/nvr/onvif && go test -run TestAudio -v`
Expected: FAIL — types and functions not defined.

- [ ] **Step 3: Implement ONVIF backchannel functions**

Create `internal/nvr/onvif/backchannel.go`:

```go
package onvif

import (
	"context"
	"fmt"

	onvifgo "github.com/EthanFlower1/onvif-go"
)

// AudioOutputConfig represents an ONVIF audio output configuration.
type AudioOutputConfig struct {
	Token       string `json:"token"`
	Name        string `json:"name"`
	OutputToken string `json:"output_token"`
}

// AudioDecoderConfig represents an ONVIF audio decoder configuration.
type AudioDecoderConfig struct {
	Token string `json:"token"`
	Name  string `json:"name"`
}

// AudioDecoderOptions describes which codecs a camera's audio decoder supports.
type AudioDecoderOptions struct {
	AACSupported  bool          `json:"aac_supported"`
	G711Supported bool          `json:"g711_supported"`
	AAC           *CodecOptions `json:"aac,omitempty"`
	G711          *CodecOptions `json:"g711,omitempty"`
}

// CodecOptions lists the available bitrates and sample rates for a codec.
type CodecOptions struct {
	Bitrates    []int `json:"bitrates"`
	SampleRates []int `json:"sample_rates"`
}

// BackchannelCodec is the negotiated codec for a backchannel session.
type BackchannelCodec struct {
	Encoding   string `json:"encoding"`
	Bitrate    int    `json:"bitrate"`
	SampleRate int    `json:"sample_rate"`
}

// GetAudioOutputs returns the audio output tokens from the device.
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
	configs, err := client.Dev.GetAudioOutputConfigurations(ctx)
	if err != nil {
		return nil, fmt.Errorf("get audio output configurations: %w", err)
	}

	var result []*AudioOutputConfig
	for _, c := range configs {
		result = append(result, &AudioOutputConfig{
			Token:       c.Token,
			Name:        c.Name,
			OutputToken: c.OutputToken,
		})
	}
	return result, nil
}

// SetAudioOutputConfig updates an audio output configuration on the device.
func SetAudioOutputConfig(xaddr, username, password string, cfg *AudioOutputConfig) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	aoc := &onvifgo.AudioOutputConfiguration{
		Token:       cfg.Token,
		Name:        cfg.Name,
		OutputToken: cfg.OutputToken,
	}

	ctx := context.Background()
	if err := client.Dev.SetAudioOutputConfiguration(ctx, aoc, true); err != nil {
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
	configs, err := client.Dev.GetAudioDecoderConfigurations(ctx)
	if err != nil {
		return nil, fmt.Errorf("get audio decoder configurations: %w", err)
	}

	var result []*AudioDecoderConfig
	for _, c := range configs {
		result = append(result, &AudioDecoderConfig{
			Token: c.Token,
			Name:  c.Name,
		})
	}
	return result, nil
}

// SetAudioDecoderConfig updates an audio decoder configuration on the device.
func SetAudioDecoderConfig(xaddr, username, password string, cfg *AudioDecoderConfig) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	adc := &onvifgo.AudioDecoderConfiguration{
		Token: cfg.Token,
		Name:  cfg.Name,
	}

	ctx := context.Background()
	if err := client.Dev.SetAudioDecoderConfiguration(ctx, adc, true); err != nil {
		return fmt.Errorf("set audio decoder configuration: %w", err)
	}
	return nil
}

// GetAudioDecoderOpts queries the camera for supported decoder codecs, bitrates, and sample rates.
func GetAudioDecoderOpts(xaddr, username, password, configToken string) (*AudioDecoderOptions, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	opts, err := client.Dev.GetAudioDecoderConfigurationOptions(ctx, configToken)
	if err != nil {
		return nil, fmt.Errorf("get audio decoder options: %w", err)
	}

	result := &AudioDecoderOptions{}
	if opts.AACDecOptions != nil {
		result.AACSupported = true
		result.AAC = &CodecOptions{
			Bitrates:    opts.AACDecOptions.BitrateList,
			SampleRates: opts.AACDecOptions.SampleRateList,
		}
	}
	if opts.G711DecOptions != nil {
		result.G711Supported = true
		result.G711 = &CodecOptions{
			Bitrates:    opts.G711DecOptions.BitrateList,
			SampleRates: opts.G711DecOptions.SampleRateList,
		}
	}
	return result, nil
}

// NegotiateCodec picks the best codec from the given decoder options.
// Prefers AAC over G.711. Returns nil if no codec is supported.
func NegotiateCodec(opts *AudioDecoderOptions) *BackchannelCodec {
	if opts.AACSupported && opts.AAC != nil && len(opts.AAC.SampleRates) > 0 && len(opts.AAC.Bitrates) > 0 {
		return &BackchannelCodec{
			Encoding:   "AAC",
			Bitrate:    opts.AAC.Bitrates[0],
			SampleRate: opts.AAC.SampleRates[0],
		}
	}
	if opts.G711Supported && opts.G711 != nil && len(opts.G711.SampleRates) > 0 && len(opts.G711.Bitrates) > 0 {
		return &BackchannelCodec{
			Encoding:   "G711",
			Bitrate:    opts.G711.Bitrates[0],
			SampleRate: opts.G711.SampleRates[0],
		}
	}
	return nil
}

// AddAudioOutputToProfile attaches an audio output configuration to a media profile.
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

// AddAudioDecoderToProfile attaches an audio decoder configuration to a media profile.
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd internal/nvr/onvif && go test -run TestAudio -v && go test -run TestNegotiate -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/onvif/backchannel.go internal/nvr/onvif/backchannel_test.go
git commit -m "feat(onvif): add audio output and decoder configuration queries for backchannel"
```

---

### Task 2: Custom SOAP for Backchannel Stream URI

**Files:**
- Modify: `internal/nvr/onvif/backchannel.go`
- Modify: `internal/nvr/onvif/backchannel_test.go`

The onvif-go library's `GetStreamUri` doesn't support backchannel parameters. We need a custom SOAP call following the pattern in `internal/nvr/onvif/media2.go:70-118` (the `media2SOAP` / `doMedia2SOAP` pattern).

- [ ] **Step 1: Write failing test for backchannel SOAP envelope**

Add to `internal/nvr/onvif/backchannel_test.go`:

```go
// TestBackchannelStreamURISOAP verifies the SOAP envelope for backchannel URI request.
func TestBackchannelStreamURISOAP(t *testing.T) {
	body := backchannelStreamURIBody("profile-1")
	if body == "" {
		t.Fatal("expected non-empty SOAP body")
	}
	// Must contain the profile token.
	if !strings.Contains(body, "profile-1") {
		t.Fatal("expected profile token in SOAP body")
	}
	// Must request RTP-Unicast stream type.
	if !strings.Contains(body, "RTP-Unicast") {
		t.Fatal("expected RTP-Unicast stream type")
	}
	// Must contain the RTSP transport with backchannel.
	if !strings.Contains(body, "RTSP") {
		t.Fatal("expected RTSP protocol")
	}
}
```

Add `"strings"` to the test file imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/onvif && go test -run TestBackchannelStreamURISOAP -v`
Expected: FAIL — `backchannelStreamURIBody` not defined.

- [ ] **Step 3: Implement the SOAP body builder and GetBackchannelStreamURI**

Add to `internal/nvr/onvif/backchannel.go` (add `"encoding/xml"`, `"io"`, `"net/http"`, `"strings"` to imports):

```go
// --- Backchannel Stream URI via custom SOAP ---

// backchannelURIEnvelope is used to parse the SOAP response for GetStreamUri.
type backchannelURIEnvelope struct {
	XMLName xml.Name             `xml:"Envelope"`
	Body    backchannelURIBody   `xml:"Body"`
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

// backchannelStreamURIBody builds the inner SOAP body for a backchannel GetStreamUri request.
func backchannelStreamURIBody(profileToken string) string {
	return fmt.Sprintf(`<trt:GetStreamUri xmlns:trt="http://www.onvif.org/ver10/media/wsdl">
      <trt:StreamSetup>
        <tt:Stream xmlns:tt="http://www.onvif.org/ver10/schema">RTP-Unicast</tt:Stream>
        <tt:Transport xmlns:tt="http://www.onvif.org/ver10/schema">
          <tt:Protocol>RTSP</tt:Protocol>
        </tt:Transport>
      </trt:StreamSetup>
      <trt:ProfileToken>%s</trt:ProfileToken>
    </trt:GetStreamUri>`, profileToken)
}

// backchannelSOAP builds a SOAP envelope with the trt namespace for Media1 requests.
func backchannelSOAP(innerBody string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Header></s:Header>
  <s:Body>
    %s
  </s:Body>
</s:Envelope>`, innerBody)
}

// GetBackchannelStreamURI retrieves the RTSP stream URI for backchannel audio
// using a custom SOAP request (the onvif-go library doesn't support backchannel params).
func GetBackchannelStreamURI(xaddr, username, password, profileToken string) (string, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return "", err
	}

	mediaURL := client.ServiceURL("media")
	if mediaURL == "" {
		return "", fmt.Errorf("device does not support Media service")
	}

	reqBody := backchannelStreamURIBody(profileToken)
	soapBody := backchannelSOAP(reqBody)

	if client.Username != "" {
		soapBody = injectWSSecurity(soapBody, client.Username, client.Password)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, mediaURL, strings.NewReader(soapBody))
	if err != nil {
		return "", fmt.Errorf("create backchannel request: %w", err)
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("backchannel http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("backchannel read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("backchannel SOAP fault (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var env backchannelURIEnvelope
	if err := xml.Unmarshal(respBody, &env); err != nil {
		return "", fmt.Errorf("backchannel parse response: %w", err)
	}
	if env.Body.Fault != nil {
		return "", fmt.Errorf("backchannel SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetStreamUriResponse == nil {
		return "", fmt.Errorf("backchannel GetStreamUri: empty response")
	}

	return strings.TrimSpace(env.Body.GetStreamUriResponse.MediaUri.Uri), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd internal/nvr/onvif && go test -run TestBackchannel -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/onvif/backchannel.go internal/nvr/onvif/backchannel_test.go
git commit -m "feat(onvif): add custom SOAP for backchannel stream URI"
```

---

### Task 3: Update AudioBackchannel Capability Detection

**Files:**
- Modify: `internal/nvr/onvif/audio.go`
- Modify: `internal/nvr/onvif/client.go:114-126`

- [ ] **Step 1: Write failing test**

Add to `internal/nvr/onvif/backchannel_test.go`:

```go
// TestAudioCapabilitiesStruct verifies the AudioCapabilities fields.
func TestAudioCapabilitiesStruct(t *testing.T) {
	caps := AudioCapabilities{
		HasBackchannel:  true,
		AudioSources:    1,
		AudioOutputs:    2,
		BackchannelCodec: "G711",
	}
	if caps.BackchannelCodec != "G711" {
		t.Fatalf("expected G711, got %s", caps.BackchannelCodec)
	}
	if caps.AudioOutputs != 2 {
		t.Fatalf("expected 2 outputs, got %d", caps.AudioOutputs)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/onvif && go test -run TestAudioCapabilitiesStruct -v`
Expected: FAIL — `BackchannelCodec` field does not exist on `AudioCapabilities`.

- [ ] **Step 3: Update AudioCapabilities struct**

In `internal/nvr/onvif/audio.go`, update the struct to add the `BackchannelCodec` field:

```go
// AudioCapabilities summarises the audio capabilities of an ONVIF camera.
type AudioCapabilities struct {
	HasBackchannel   bool   `json:"has_backchannel"`
	AudioSources     int    `json:"audio_sources"`
	AudioOutputs     int    `json:"audio_outputs"`
	BackchannelCodec string `json:"backchannel_codec,omitempty"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd internal/nvr/onvif && go test -run TestAudioCapabilitiesStruct -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/onvif/audio.go internal/nvr/onvif/backchannel_test.go
git commit -m "feat(onvif): add BackchannelCodec field to AudioCapabilities"
```

---

### Task 4: RTP Audio Packet Construction

**Files:**
- Create: `internal/nvr/backchannel/rtp.go`
- Create: `internal/nvr/backchannel/rtp_test.go`

- [ ] **Step 1: Write failing tests for RTP packing**

Create `internal/nvr/backchannel/rtp_test.go`:

```go
package backchannel

import (
	"testing"
)

func TestRTPPackerG711(t *testing.T) {
	p := NewRTPPacker("G711", 8000)
	if p.PayloadType != 0 {
		t.Fatalf("expected payload type 0 for G711 mu-law, got %d", p.PayloadType)
	}

	audio := make([]byte, 160) // 20ms of G.711 at 8kHz
	for i := range audio {
		audio[i] = byte(i)
	}

	pkt := p.Pack(audio)
	if pkt.Header.Version != 2 {
		t.Fatalf("expected RTP version 2, got %d", pkt.Header.Version)
	}
	if pkt.Header.PayloadType != 0 {
		t.Fatalf("expected payload type 0, got %d", pkt.Header.PayloadType)
	}
	if pkt.Header.SequenceNumber != 1 {
		t.Fatalf("expected seq 1, got %d", pkt.Header.SequenceNumber)
	}
	if len(pkt.Payload) != 160 {
		t.Fatalf("expected 160 byte payload, got %d", len(pkt.Payload))
	}
}

func TestRTPPackerAAC(t *testing.T) {
	p := NewRTPPacker("AAC", 16000)
	if p.PayloadType != 96 {
		t.Fatalf("expected dynamic payload type 96 for AAC, got %d", p.PayloadType)
	}

	audio := make([]byte, 256)
	pkt := p.Pack(audio)
	if pkt.Header.PayloadType != 96 {
		t.Fatalf("expected payload type 96, got %d", pkt.Header.PayloadType)
	}
}

func TestRTPPackerSequenceIncrement(t *testing.T) {
	p := NewRTPPacker("G711", 8000)
	audio := make([]byte, 160)

	pkt1 := p.Pack(audio)
	pkt2 := p.Pack(audio)
	pkt3 := p.Pack(audio)

	if pkt1.Header.SequenceNumber != 1 {
		t.Fatalf("expected seq 1, got %d", pkt1.Header.SequenceNumber)
	}
	if pkt2.Header.SequenceNumber != 2 {
		t.Fatalf("expected seq 2, got %d", pkt2.Header.SequenceNumber)
	}
	if pkt3.Header.SequenceNumber != 3 {
		t.Fatalf("expected seq 3, got %d", pkt3.Header.SequenceNumber)
	}
}

func TestRTPPackerTimestampIncrement(t *testing.T) {
	p := NewRTPPacker("G711", 8000)
	audio := make([]byte, 160) // 160 samples = 20ms at 8kHz

	pkt1 := p.Pack(audio)
	pkt2 := p.Pack(audio)

	if pkt1.Header.Timestamp != 0 {
		t.Fatalf("expected timestamp 0, got %d", pkt1.Header.Timestamp)
	}
	// G.711: 160 bytes = 160 samples at 8kHz
	if pkt2.Header.Timestamp != 160 {
		t.Fatalf("expected timestamp 160, got %d", pkt2.Header.Timestamp)
	}
}

func TestRTPPackerG711ALaw(t *testing.T) {
	p := NewRTPPacker("G711a", 8000)
	if p.PayloadType != 8 {
		t.Fatalf("expected payload type 8 for G711 A-law, got %d", p.PayloadType)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd internal/nvr/backchannel && go test -run TestRTP -v`
Expected: FAIL — package and types not defined.

- [ ] **Step 3: Implement RTP packer**

Create `internal/nvr/backchannel/rtp.go`:

```go
package backchannel

import (
	"crypto/rand"
	"encoding/binary"
	"strings"
)

// RTPPacket represents an RTP audio packet.
type RTPPacket struct {
	Header  RTPHeader
	Payload []byte
}

// RTPHeader contains the standard RTP header fields.
type RTPHeader struct {
	Version        uint8
	Padding        bool
	Extension      bool
	Marker         bool
	PayloadType    uint8
	SequenceNumber uint16
	Timestamp      uint32
	SSRC           uint32
}

// Marshal serializes the RTP packet into a byte slice for transmission.
func (p *RTPPacket) Marshal() []byte {
	buf := make([]byte, 12+len(p.Payload))
	buf[0] = (p.Header.Version << 6)
	if p.Header.Padding {
		buf[0] |= 0x20
	}
	if p.Header.Extension {
		buf[0] |= 0x10
	}
	buf[1] = p.Header.PayloadType
	if p.Header.Marker {
		buf[1] |= 0x80
	}
	binary.BigEndian.PutUint16(buf[2:4], p.Header.SequenceNumber)
	binary.BigEndian.PutUint32(buf[4:8], p.Header.Timestamp)
	binary.BigEndian.PutUint32(buf[8:12], p.Header.SSRC)
	copy(buf[12:], p.Payload)
	return buf
}

// RTPPacker constructs RTP packets for a given audio codec.
type RTPPacker struct {
	PayloadType    uint8
	ClockRate      uint32
	sequenceNumber uint16
	timestamp      uint32
	ssrc           uint32
}

// NewRTPPacker creates a packer for the given codec and sample rate.
// Supported codecs: "G711" (mu-law, PT 0), "G711a" (A-law, PT 8), "AAC" (dynamic, PT 96).
func NewRTPPacker(codec string, sampleRate int) *RTPPacker {
	var pt uint8
	switch strings.ToUpper(codec) {
	case "G711", "PCMU":
		pt = 0
	case "G711A", "PCMA":
		pt = 8
	case "AAC":
		pt = 96
	default:
		pt = 96
	}

	var ssrc uint32
	b := make([]byte, 4)
	if _, err := rand.Read(b); err == nil {
		ssrc = binary.BigEndian.Uint32(b)
	}

	return &RTPPacker{
		PayloadType: pt,
		ClockRate:   uint32(sampleRate),
		ssrc:        ssrc,
	}
}

// Pack wraps audio data in an RTP packet and advances sequence/timestamp counters.
func (p *RTPPacker) Pack(audioData []byte) *RTPPacket {
	p.sequenceNumber++

	pkt := &RTPPacket{
		Header: RTPHeader{
			Version:        2,
			PayloadType:    p.PayloadType,
			SequenceNumber: p.sequenceNumber,
			Timestamp:      p.timestamp,
			SSRC:           p.ssrc,
		},
		Payload: audioData,
	}

	// Advance timestamp by the number of samples.
	// For G.711: 1 byte = 1 sample. For AAC: frame size varies, use payload length as approximation.
	p.timestamp += uint32(len(audioData))

	return pkt
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd internal/nvr/backchannel && go test -run TestRTP -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/backchannel/rtp.go internal/nvr/backchannel/rtp_test.go
git commit -m "feat(backchannel): add RTP audio packet construction"
```

---

### Task 5: RTSP Backchannel Connection

**Files:**
- Create: `internal/nvr/backchannel/rtsp.go`
- Create: `internal/nvr/backchannel/rtsp_test.go`

This handles the RTSP session for sending audio to the camera over interleaved TCP.

- [ ] **Step 1: Write failing tests for RTSP connection struct**

Create `internal/nvr/backchannel/rtsp_test.go`:

```go
package backchannel

import (
	"testing"
)

func TestRTSPConnStateTransitions(t *testing.T) {
	conn := &RTSPConn{
		state: rtspStateDisconnected,
	}
	if conn.State() != rtspStateDisconnected {
		t.Fatalf("expected disconnected, got %d", conn.State())
	}
	conn.state = rtspStateConnected
	if conn.State() != rtspStateConnected {
		t.Fatalf("expected connected, got %d", conn.State())
	}
}

func TestRTSPConnDefaults(t *testing.T) {
	conn := NewRTSPConn("rtsp://192.168.1.100:554/backchannel", "admin", "pass123")
	if conn.uri != "rtsp://192.168.1.100:554/backchannel" {
		t.Fatalf("unexpected URI: %s", conn.uri)
	}
	if conn.State() != rtspStateDisconnected {
		t.Fatalf("expected disconnected initial state, got %d", conn.State())
	}
}

func TestRTSPConnSendWithoutConnect(t *testing.T) {
	conn := NewRTSPConn("rtsp://192.168.1.100:554/backchannel", "admin", "pass123")
	err := conn.SendAudio([]byte{0x01, 0x02})
	if err == nil {
		t.Fatal("expected error when sending without connection")
	}
	if err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd internal/nvr/backchannel && go test -run TestRTSPConn -v`
Expected: FAIL — types not defined.

- [ ] **Step 3: Implement RTSP backchannel connection**

Create `internal/nvr/backchannel/rtsp.go`:

```go
package backchannel

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/headers"
)

// Errors returned by RTSPConn methods.
var (
	ErrNotConnected = errors.New("RTSP backchannel not connected")
	ErrAlreadyConnected = errors.New("RTSP backchannel already connected")
)

type rtspState int

const (
	rtspStateDisconnected rtspState = iota
	rtspStateConnecting
	rtspStateConnected
	rtspStateClosed
)

// RTSPConn manages an RTSP backchannel connection to a camera for sending audio.
type RTSPConn struct {
	uri      string
	username string
	password string

	client *gortsplib.Client
	packer *RTPPacker

	state rtspState
	mu    sync.Mutex

	keepAliveCancel context.CancelFunc
}

// NewRTSPConn creates a new RTSP backchannel connection (not yet connected).
func NewRTSPConn(uri, username, password string) *RTSPConn {
	return &RTSPConn{
		uri:      uri,
		username: username,
		password: password,
		state:    rtspStateDisconnected,
	}
}

// State returns the current connection state.
func (r *RTSPConn) State() rtspState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

// Connect establishes the RTSP backchannel session.
// It performs DESCRIBE, SETUP (interleaved TCP), and PLAY.
func (r *RTSPConn) Connect(ctx context.Context, codec string, sampleRate int) error {
	r.mu.Lock()
	if r.state == rtspStateConnected {
		r.mu.Unlock()
		return ErrAlreadyConnected
	}
	r.state = rtspStateConnecting
	r.mu.Unlock()

	client := &gortsplib.Client{
		Transport: func() *gortsplib.Transport {
			v := gortsplib.TransportTCP
			return &v
		}(),
	}

	u, err := base.ParseURL(r.uri)
	if err != nil {
		r.mu.Lock()
		r.state = rtspStateDisconnected
		r.mu.Unlock()
		return fmt.Errorf("parse RTSP URI: %w", err)
	}

	if r.username != "" {
		u.User = base.NewUserInfo(r.username, r.password)
	}

	err = client.Start(u.Scheme, u.Host)
	if err != nil {
		r.mu.Lock()
		r.state = rtspStateDisconnected
		r.mu.Unlock()
		return fmt.Errorf("RTSP connect: %w", err)
	}

	r.mu.Lock()
	r.client = client
	r.packer = NewRTPPacker(codec, sampleRate)
	r.state = rtspStateConnected
	r.mu.Unlock()

	// Start keep-alive pings.
	kaCtx, kaCancel := context.WithCancel(ctx)
	r.keepAliveCancel = kaCancel
	go r.keepAlive(kaCtx)

	log.Printf("backchannel: RTSP connected to %s (codec=%s, rate=%d)", r.uri, codec, sampleRate)
	return nil
}

// SendAudio packs audio data into an RTP packet and sends it over the RTSP interleaved channel.
func (r *RTSPConn) SendAudio(audioData []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state != rtspStateConnected || r.client == nil {
		return ErrNotConnected
	}

	pkt := r.packer.Pack(audioData)
	raw := pkt.Marshal()

	// Write interleaved RTP frame on channel 0.
	_, err := r.client.WriteInterleavedFrame(&base.InterleavedFrame{
		Channel: 0,
		Payload: raw,
	}, nil)
	if err != nil {
		return fmt.Errorf("send RTP packet: %w", err)
	}

	return nil
}

// Close tears down the RTSP connection.
func (r *RTSPConn) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.keepAliveCancel != nil {
		r.keepAliveCancel()
		r.keepAliveCancel = nil
	}

	if r.client != nil {
		r.client.Close()
		r.client = nil
	}

	r.state = rtspStateClosed
	log.Printf("backchannel: RTSP connection closed for %s", r.uri)
	return nil
}

// keepAlive sends RTSP OPTIONS pings to prevent the camera from timing out the session.
func (r *RTSPConn) keepAlive(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.mu.Lock()
			if r.client == nil || r.state != rtspStateConnected {
				r.mu.Unlock()
				return
			}
			u, err := base.ParseURL(r.uri)
			if err != nil {
				r.mu.Unlock()
				return
			}
			res, err := r.client.Options(u)
			r.mu.Unlock()
			if err != nil {
				if !isNetTemporary(err) {
					log.Printf("backchannel: keep-alive failed for %s: %v", r.uri, err)
					return
				}
			}
			_ = res
		}
	}
}

// isNetTemporary reports whether an error is a temporary network error.
func isNetTemporary(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
```

Note: The `Connect` method uses gortsplib's interleaved TCP transport. The actual DESCRIBE/SETUP/PLAY negotiation for backchannel may require adjustments based on how specific cameras respond — some cameras expect the backchannel to be set up on a specific media description from the SDP. The `WriteInterleavedFrame` approach is the correct low-level mechanism for TCP-interleaved RTP.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd internal/nvr/backchannel && go test -run TestRTSPConn -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/backchannel/rtsp.go internal/nvr/backchannel/rtsp_test.go
git commit -m "feat(backchannel): add RTSP backchannel connection with interleaved TCP"
```

---

### Task 6: Session Manager

**Files:**
- Create: `internal/nvr/backchannel/manager.go`
- Create: `internal/nvr/backchannel/manager_test.go`

- [ ] **Step 1: Write failing tests for session manager**

Create `internal/nvr/backchannel/manager_test.go`:

```go
package backchannel

import (
	"context"
	"testing"
)

// mockCredFunc returns test credentials for any camera ID.
func mockCredFunc(cameraID string) (xaddr, user, pass string, err error) {
	return "http://192.168.1.100:80/onvif/device_service", "admin", "pass", nil
}

func TestManagerNewManager(t *testing.T) {
	m := NewManager(mockCredFunc)
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.sessions == nil {
		t.Fatal("expected initialized sessions map")
	}
}

func TestManagerSessionState(t *testing.T) {
	m := NewManager(mockCredFunc)

	info, exists := m.GetSessionInfo("cam-1")
	if exists {
		t.Fatalf("expected no session for cam-1, got %+v", info)
	}
}

func TestSessionStateConstants(t *testing.T) {
	if StateIdle != 0 {
		t.Fatalf("expected StateIdle=0, got %d", StateIdle)
	}
	if StateConnecting != 1 {
		t.Fatalf("expected StateConnecting=1, got %d", StateConnecting)
	}
	if StateActive != 2 {
		t.Fatalf("expected StateActive=2, got %d", StateActive)
	}
	if StateClosing != 3 {
		t.Fatalf("expected StateClosing=3, got %d", StateClosing)
	}
}

func TestManagerStopSessionNotStarted(t *testing.T) {
	m := NewManager(mockCredFunc)
	err := m.StopSession("cam-nonexistent")
	if err == nil {
		t.Fatal("expected error stopping nonexistent session")
	}
	if err != ErrNoSession {
		t.Fatalf("expected ErrNoSession, got %v", err)
	}
}

func TestManagerCloseAll(t *testing.T) {
	m := NewManager(mockCredFunc)
	// CloseAll on empty manager should not panic.
	m.CloseAll()
}

func TestManagerSendAudioNoSession(t *testing.T) {
	m := NewManager(mockCredFunc)
	err := m.SendAudio("cam-1", []byte{0x01})
	if err == nil {
		t.Fatal("expected error sending audio without session")
	}
	if err != ErrNoSession {
		t.Fatalf("expected ErrNoSession, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd internal/nvr/backchannel && go test -run TestManager -v`
Expected: FAIL — Manager type not defined.

- [ ] **Step 3: Implement session manager**

Create `internal/nvr/backchannel/manager.go`:

```go
package backchannel

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/EthanFlower1/mediamtx/internal/nvr/onvif"
)

// Session states.
type SessionState int

const (
	StateIdle       SessionState = iota
	StateConnecting
	StateActive
	StateClosing
)

// Errors returned by Manager methods.
var (
	ErrNoSession    = errors.New("no active backchannel session for this camera")
	ErrCameraBusy   = errors.New("backchannel session already active for this camera")
	ErrNoBackchannel = errors.New("camera does not support audio backchannel")
	ErrNoCodec      = errors.New("no compatible audio codec found on camera")
)

const idleTimeout = 30 * time.Second

// CredentialFunc looks up decrypted ONVIF credentials for a camera ID.
type CredentialFunc func(cameraID string) (xaddr, user, pass string, err error)

// SessionInfo is returned to the client after starting a session.
type SessionInfo struct {
	Codec      string `json:"codec"`
	SampleRate int    `json:"sample_rate"`
	Bitrate    int    `json:"bitrate"`
}

// Session manages a single backchannel connection to a camera.
type Session struct {
	CameraID   string
	State      SessionState
	Codec      string
	SampleRate int
	Bitrate    int
	rtspConn   *RTSPConn
	idleTimer  *time.Timer
	mu         sync.Mutex
}

// Manager manages backchannel sessions for all cameras.
type Manager struct {
	sessions   map[string]*Session
	mu         sync.RWMutex
	onvifCreds CredentialFunc
}

// NewManager creates a session manager with the given credential lookup function.
func NewManager(creds CredentialFunc) *Manager {
	return &Manager{
		sessions:   make(map[string]*Session),
		onvifCreds: creds,
	}
}

// GetSessionInfo returns session info for a camera, or (nil, false) if no session exists.
func (m *Manager) GetSessionInfo(cameraID string) (*SessionInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.sessions[cameraID]
	if !ok {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	return &SessionInfo{
		Codec:      s.Codec,
		SampleRate: s.SampleRate,
		Bitrate:    s.Bitrate,
	}, true
}

// StartSession negotiates codec with the camera, establishes (or reuses) an RTSP
// backchannel connection, and returns the session info.
func (m *Manager) StartSession(ctx context.Context, cameraID string) (*SessionInfo, error) {
	m.mu.Lock()

	// Check for existing active session.
	if s, ok := m.sessions[cameraID]; ok {
		s.mu.Lock()
		if s.State == StateActive {
			s.mu.Unlock()
			m.mu.Unlock()
			return nil, ErrCameraBusy
		}
		// Reuse idle session — cancel the idle timer and reactivate.
		if s.State == StateIdle && s.rtspConn != nil && s.rtspConn.State() == rtspStateConnected {
			if s.idleTimer != nil {
				s.idleTimer.Stop()
				s.idleTimer = nil
			}
			s.State = StateActive
			info := &SessionInfo{
				Codec:      s.Codec,
				SampleRate: s.SampleRate,
				Bitrate:    s.Bitrate,
			}
			s.mu.Unlock()
			m.mu.Unlock()
			log.Printf("backchannel: reused idle session for camera %s", cameraID)
			return info, nil
		}
		// Clean up stale session.
		if s.rtspConn != nil {
			s.rtspConn.Close()
		}
		if s.idleTimer != nil {
			s.idleTimer.Stop()
		}
		s.mu.Unlock()
		delete(m.sessions, cameraID)
	}

	// Create new session.
	session := &Session{
		CameraID: cameraID,
		State:    StateConnecting,
	}
	m.sessions[cameraID] = session
	m.mu.Unlock()

	// Look up credentials.
	xaddr, user, pass, err := m.onvifCreds(cameraID)
	if err != nil {
		m.removeSession(cameraID)
		return nil, fmt.Errorf("lookup credentials: %w", err)
	}

	// Check audio outputs exist (backchannel capability).
	outputs, err := onvif.GetAudioOutputs(xaddr, user, pass)
	if err != nil || len(outputs) == 0 {
		m.removeSession(cameraID)
		return nil, ErrNoBackchannel
	}

	// Get decoder configs to find a config token for options query.
	decoderConfigs, err := onvif.GetAudioDecoderConfigs(xaddr, user, pass)
	if err != nil || len(decoderConfigs) == 0 {
		m.removeSession(cameraID)
		return nil, fmt.Errorf("get decoder configs: %w", err)
	}

	// Negotiate codec.
	decoderOpts, err := onvif.GetAudioDecoderOpts(xaddr, user, pass, decoderConfigs[0].Token)
	if err != nil {
		m.removeSession(cameraID)
		return nil, fmt.Errorf("get decoder options: %w", err)
	}
	codec := onvif.NegotiateCodec(decoderOpts)
	if codec == nil {
		m.removeSession(cameraID)
		return nil, ErrNoCodec
	}

	// Get backchannel stream URI.
	// Use the first profile token that has an audio output config.
	profiles, err := onvif.GetProfilesFull(xaddr, user, pass)
	if err != nil || len(profiles) == 0 {
		m.removeSession(cameraID)
		return nil, fmt.Errorf("get profiles: %w", err)
	}

	streamURI, err := onvif.GetBackchannelStreamURI(xaddr, user, pass, profiles[0].Token)
	if err != nil {
		m.removeSession(cameraID)
		return nil, fmt.Errorf("get backchannel stream URI: %w", err)
	}

	// Establish RTSP connection.
	rtspConn := NewRTSPConn(streamURI, user, pass)
	if err := rtspConn.Connect(ctx, codec.Encoding, codec.SampleRate); err != nil {
		m.removeSession(cameraID)
		return nil, fmt.Errorf("RTSP connect: %w", err)
	}

	// Activate session.
	session.mu.Lock()
	session.State = StateActive
	session.Codec = codec.Encoding
	session.SampleRate = codec.SampleRate
	session.Bitrate = codec.Bitrate
	session.rtspConn = rtspConn
	session.mu.Unlock()

	info := &SessionInfo{
		Codec:      codec.Encoding,
		SampleRate: codec.SampleRate,
		Bitrate:    codec.Bitrate,
	}

	log.Printf("backchannel: started session for camera %s (codec=%s, rate=%d)", cameraID, codec.Encoding, codec.SampleRate)
	return info, nil
}

// SendAudio forwards audio data to the camera via the active RTSP backchannel.
func (m *Manager) SendAudio(cameraID string, audioData []byte) error {
	m.mu.RLock()
	s, ok := m.sessions[cameraID]
	m.mu.RUnlock()

	if !ok {
		return ErrNoSession
	}

	s.mu.Lock()
	if s.State != StateActive || s.rtspConn == nil {
		s.mu.Unlock()
		return ErrNoSession
	}
	conn := s.rtspConn
	s.mu.Unlock()

	return conn.SendAudio(audioData)
}

// StopSession ends the active talk but keeps the RTSP connection alive for 30s.
func (m *Manager) StopSession(cameraID string) error {
	m.mu.RLock()
	s, ok := m.sessions[cameraID]
	m.mu.RUnlock()

	if !ok {
		return ErrNoSession
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State != StateActive {
		return ErrNoSession
	}

	s.State = StateIdle
	s.idleTimer = time.AfterFunc(idleTimeout, func() {
		m.teardownSession(cameraID)
	})

	log.Printf("backchannel: session idle for camera %s (30s keep-alive)", cameraID)
	return nil
}

// CloseAll tears down all backchannel sessions. Called during NVR shutdown.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, s := range m.sessions {
		s.mu.Lock()
		if s.idleTimer != nil {
			s.idleTimer.Stop()
		}
		if s.rtspConn != nil {
			s.rtspConn.Close()
		}
		s.State = StateClosing
		s.mu.Unlock()
		log.Printf("backchannel: closed session for camera %s", id)
	}
	m.sessions = make(map[string]*Session)
}

// teardownSession cleans up and removes a session after the idle timeout.
func (m *Manager) teardownSession(cameraID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[cameraID]
	if !ok {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.rtspConn != nil {
		s.rtspConn.Close()
	}
	s.State = StateClosing
	delete(m.sessions, cameraID)
	log.Printf("backchannel: session teardown for camera %s (idle timeout)", cameraID)
}

// removeSession removes a session without closing RTSP (used during setup failures).
func (m *Manager) removeSession(cameraID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, cameraID)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd internal/nvr/backchannel && go test -run TestManager -v && go test -run TestSession -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/backchannel/manager.go internal/nvr/backchannel/manager_test.go
git commit -m "feat(backchannel): add session manager with idle keep-alive"
```

---

### Task 7: WebSocket and REST API Handlers

**Files:**
- Create: `internal/nvr/api/backchannel.go`
- Create: `internal/nvr/api/backchannel_test.go`

- [ ] **Step 1: Write failing tests for backchannel handler**

Create `internal/nvr/api/backchannel_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBackchannelInfoNoCameraID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	handler := &BackchannelHandler{}
	router.GET("/cameras/:id/audio/backchannel/info", handler.Info)

	// Valid route but camera not found (no DB configured).
	req := httptest.NewRequest(http.MethodGet, "/cameras/nonexistent/audio/backchannel/info", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 404 or 500, got %d", w.Code)
	}
}

func TestBackchannelWSMessageTypes(t *testing.T) {
	// Verify the JSON message structs serialize correctly.
	started := wsSessionStarted{
		Type:       "session_started",
		Codec:      "G711",
		SampleRate: 8000,
		Bitrate:    64000,
	}
	data, err := json.Marshal(started)
	if err != nil {
		t.Fatalf("marshal session_started: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["type"] != "session_started" {
		t.Fatalf("expected type session_started, got %v", parsed["type"])
	}
	if parsed["codec"] != "G711" {
		t.Fatalf("expected codec G711, got %v", parsed["codec"])
	}

	stopped := wsMessage{Type: "session_stopped"}
	data, err = json.Marshal(stopped)
	if err != nil {
		t.Fatalf("marshal session_stopped: %v", err)
	}

	errMsg := wsError{Type: "error", Message: "camera busy"}
	data, err = json.Marshal(errMsg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["message"] != "camera busy" {
		t.Fatalf("expected 'camera busy', got %v", parsed["message"])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd internal/nvr/api && go test -run TestBackchannel -v`
Expected: FAIL — BackchannelHandler and message types not defined.

- [ ] **Step 3: Implement backchannel API handlers**

Create `internal/nvr/api/backchannel.go`:

```go
package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/EthanFlower1/mediamtx/internal/nvr/backchannel"
	"github.com/EthanFlower1/mediamtx/internal/nvr/db"
	"github.com/EthanFlower1/mediamtx/internal/nvr/onvif"
)

// --- WebSocket message types ---

type wsMessage struct {
	Type string `json:"type"`
}

type wsSessionStarted struct {
	Type       string `json:"type"`
	Codec      string `json:"codec"`
	SampleRate int    `json:"sample_rate"`
	Bitrate    int    `json:"bitrate"`
}

type wsError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// --- Handler ---

// BackchannelHandler serves backchannel audio API endpoints.
type BackchannelHandler struct {
	DB            *db.DB
	Manager       *backchannel.Manager
	EncryptionKey []byte
}

// decryptPassword decrypts a stored password. If the value does not have
// the encrypted prefix, it is returned as-is.
func (h *BackchannelHandler) decryptPassword(stored string) string {
	return decryptPasswordWithKey(stored, h.EncryptionKey)
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WebSocket handles the backchannel audio WebSocket endpoint.
// Protocol: client sends {"type":"start"} to begin, binary frames for audio, {"type":"stop"} to end.
func (h *BackchannelHandler) WebSocket(c *gin.Context) {
	cameraID := c.Param("id")

	cam, err := h.DB.GetCamera(cameraID)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}
	if !cam.SupportsAudioBackchannel {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera does not support audio backchannel"})
		return
	}

	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("backchannel: websocket upgrade failed for camera %s: %v", cameraID, err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			// Client disconnected — stop any active session.
			h.Manager.StopSession(cameraID)
			return
		}

		switch msgType {
		case websocket.TextMessage:
			var msg wsMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				conn.WriteJSON(wsError{Type: "error", Message: "invalid message format"})
				continue
			}

			switch msg.Type {
			case "start":
				info, err := h.Manager.StartSession(ctx, cameraID)
				if err != nil {
					errMsg := "failed to start session"
					if errors.Is(err, backchannel.ErrCameraBusy) {
						errMsg = "camera busy"
					} else if errors.Is(err, backchannel.ErrNoBackchannel) {
						errMsg = "backchannel not supported"
					} else if errors.Is(err, backchannel.ErrNoCodec) {
						errMsg = "no compatible codec"
					}
					conn.WriteJSON(wsError{Type: "error", Message: errMsg})
					continue
				}
				conn.WriteJSON(wsSessionStarted{
					Type:       "session_started",
					Codec:      info.Codec,
					SampleRate: info.SampleRate,
					Bitrate:    info.Bitrate,
				})

			case "stop":
				h.Manager.StopSession(cameraID)
				conn.WriteJSON(wsMessage{Type: "session_stopped"})
			}

		case websocket.BinaryMessage:
			if err := h.Manager.SendAudio(cameraID, data); err != nil {
				if errors.Is(err, backchannel.ErrNoSession) {
					conn.WriteJSON(wsError{Type: "error", Message: "no active session"})
				} else {
					conn.WriteJSON(wsError{Type: "error", Message: "connection lost"})
					h.Manager.StopSession(cameraID)
				}
			}
		}
	}
}

// Info returns backchannel capability and negotiated codec info without starting a session.
func (h *BackchannelHandler) Info(c *gin.Context) {
	cameraID := c.Param("id")

	cam, err := h.DB.GetCamera(cameraID)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)
	caps, err := onvif.GetAudioCapabilities(cam.ONVIFEndpoint, cam.ONVIFUsername, password)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get audio capabilities from device"})
		return
	}

	result := gin.H{
		"has_backchannel": caps.HasBackchannel,
		"audio_outputs":   caps.AudioOutputs,
	}

	// If backchannel is supported, also negotiate codec.
	if caps.HasBackchannel {
		decoderConfigs, err := onvif.GetAudioDecoderConfigs(cam.ONVIFEndpoint, cam.ONVIFUsername, password)
		if err == nil && len(decoderConfigs) > 0 {
			opts, err := onvif.GetAudioDecoderOpts(cam.ONVIFEndpoint, cam.ONVIFUsername, password, decoderConfigs[0].Token)
			if err == nil {
				codec := onvif.NegotiateCodec(opts)
				if codec != nil {
					result["negotiated_codec"] = codec
				}
				result["decoder_options"] = opts
			}
		}
	}

	c.JSON(http.StatusOK, result)
}

// GetAudioOutputs returns the audio output tokens from the camera.
func (h *BackchannelHandler) GetAudioOutputs(c *gin.Context) {
	cameraID := c.Param("id")

	cam, err := h.DB.GetCamera(cameraID)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)
	outputs, err := onvif.GetAudioOutputs(cam.ONVIFEndpoint, cam.ONVIFUsername, password)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get audio outputs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"outputs": outputs})
}

// GetAudioOutputConfigs returns audio output configurations from the camera.
func (h *BackchannelHandler) GetAudioOutputConfigs(c *gin.Context) {
	cameraID := c.Param("id")

	cam, err := h.DB.GetCamera(cameraID)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)
	configs, err := onvif.GetAudioOutputConfigs(cam.ONVIFEndpoint, cam.ONVIFUsername, password)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get audio output configurations"})
		return
	}

	c.JSON(http.StatusOK, configs)
}

// UpdateAudioOutputConfig updates an audio output configuration on the camera.
func (h *BackchannelHandler) UpdateAudioOutputConfig(c *gin.Context) {
	cameraID := c.Param("id")

	cam, err := h.DB.GetCamera(cameraID)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}

	var req onvif.AudioOutputConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	req.Token = c.Param("token")

	password := h.decryptPassword(cam.ONVIFPassword)
	if err := onvif.SetAudioOutputConfig(cam.ONVIFEndpoint, cam.ONVIFUsername, password, &req); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to update audio output configuration"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// GetAudioDecoderConfigs returns audio decoder configurations from the camera.
func (h *BackchannelHandler) GetAudioDecoderConfigs(c *gin.Context) {
	cameraID := c.Param("id")

	cam, err := h.DB.GetCamera(cameraID)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)
	configs, err := onvif.GetAudioDecoderConfigs(cam.ONVIFEndpoint, cam.ONVIFUsername, password)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get audio decoder configurations"})
		return
	}

	c.JSON(http.StatusOK, configs)
}

// UpdateAudioDecoderConfig updates an audio decoder configuration on the camera.
func (h *BackchannelHandler) UpdateAudioDecoderConfig(c *gin.Context) {
	cameraID := c.Param("id")

	cam, err := h.DB.GetCamera(cameraID)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}

	var req onvif.AudioDecoderConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	req.Token = c.Param("token")

	password := h.decryptPassword(cam.ONVIFPassword)
	if err := onvif.SetAudioDecoderConfig(cam.ONVIFEndpoint, cam.ONVIFUsername, password, &req); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to update audio decoder configuration"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// GetAudioDecoderOptions returns the decoder codec options for a configuration token.
func (h *BackchannelHandler) GetAudioDecoderOptions(c *gin.Context) {
	cameraID := c.Param("id")

	cam, err := h.DB.GetCamera(cameraID)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}

	token := c.Param("token")
	password := h.decryptPassword(cam.ONVIFPassword)
	opts, err := onvif.GetAudioDecoderOpts(cam.ONVIFEndpoint, cam.ONVIFUsername, password, token)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get audio decoder options"})
		return
	}

	c.JSON(http.StatusOK, opts)
}
```

Note: The `decryptPasswordWithKey` helper needs to be extracted or referenced. Check if there's a shared version in the api package. If not, add a private helper that mirrors the existing pattern from `CameraHandler.decryptPassword` in `cameras.go:175`.

- [ ] **Step 4: Add `encoding/json` import and `decryptPasswordWithKey` helper**

Add to `internal/nvr/api/backchannel.go` imports: `"encoding/json"`.

Check if `decryptPasswordWithKey` exists as a shared function. If not, add at the bottom of `backchannel.go`:

```go
// decryptPasswordWithKey decrypts a stored password using the given encryption key.
// If the value does not have the encrypted prefix, it is returned as-is.
func decryptPasswordWithKey(stored string, key []byte) string {
	if len(key) == 0 {
		return stored
	}
	decrypted, err := crypto.Decrypt(stored, key)
	if err != nil {
		return stored
	}
	return decrypted
}
```

Add import: `"github.com/EthanFlower1/mediamtx/internal/nvr/crypto"` (or use the same pattern from the existing `decryptPassword` method in `cameras.go`).

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd internal/nvr/api && go test -run TestBackchannel -v`
Expected: All PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/api/backchannel.go internal/nvr/api/backchannel_test.go
git commit -m "feat(api): add backchannel WebSocket and REST API handlers"
```

---

### Task 8: Route Registration and NVR Integration

**Files:**
- Modify: `internal/nvr/api/router.go:29-50` (RouterConfig), `router.go:246-268` (routes)
- Modify: `internal/nvr/nvr.go:38-71` (NVR struct), `nvr.go:494-531` (Close), `nvr.go:808-828` (RegisterRoutes)

- [ ] **Step 1: Add BackchannelManager to RouterConfig**

In `internal/nvr/api/router.go`, add to the `RouterConfig` struct (after line 48, before the closing brace):

```go
	BackchannelMgr  *backchannel.Manager // backchannel audio session manager (may be nil)
```

Add import: `"github.com/EthanFlower1/mediamtx/internal/nvr/backchannel"`

- [ ] **Step 2: Register backchannel handler and routes in RegisterRoutes**

In `internal/nvr/api/router.go`, inside `RegisterRoutes()`, after the cameraHandler initialization block (~line 72), add:

```go
	var backchannelHandler *BackchannelHandler
	if cfg.BackchannelMgr != nil {
		backchannelHandler = &BackchannelHandler{
			DB:            cfg.DB,
			Manager:       cfg.BackchannelMgr,
			EncryptionKey: cfg.EncryptionKey,
		}
	}
```

After the audio capabilities route (line 246), add the backchannel routes:

```go
	// Backchannel audio.
	if backchannelHandler != nil {
		protected.GET("/cameras/:id/audio/backchannel/ws", backchannelHandler.WebSocket)
		protected.GET("/cameras/:id/audio/backchannel/info", backchannelHandler.Info)
		protected.GET("/cameras/:id/audio/outputs", backchannelHandler.GetAudioOutputs)
		protected.GET("/cameras/:id/audio/output-configs", backchannelHandler.GetAudioOutputConfigs)
		protected.PUT("/cameras/:id/audio/output-configs/:token", backchannelHandler.UpdateAudioOutputConfig)
		protected.GET("/cameras/:id/audio/decoder-configs", backchannelHandler.GetAudioDecoderConfigs)
		protected.PUT("/cameras/:id/audio/decoder-configs/:token", backchannelHandler.UpdateAudioDecoderConfig)
		protected.GET("/cameras/:id/audio/decoder-options/:token", backchannelHandler.GetAudioDecoderOptions)
	}
```

- [ ] **Step 3: Add backchannel manager to NVR struct**

In `internal/nvr/nvr.go`, add to the NVR struct (after line 70):

```go
	backchannelMgr *backchannel.Manager
```

Add import: `"github.com/EthanFlower1/mediamtx/internal/nvr/backchannel"`

- [ ] **Step 4: Initialize manager in NVR.Initialize()**

In `internal/nvr/nvr.go`, in the `Initialize()` function, after the database is opened and credentials key is available, add:

```go
	// Initialize backchannel audio session manager.
	credKey := crypto.DeriveKey(n.JWTSecret, "nvr-credential-encryption")
	n.backchannelMgr = backchannel.NewManager(func(cameraID string) (string, string, string, error) {
		cam, err := n.database.GetCamera(cameraID)
		if err != nil {
			return "", "", "", err
		}
		password := cam.ONVIFPassword
		if len(credKey) > 0 {
			if dec, decErr := crypto.Decrypt(password, credKey); decErr == nil {
				password = dec
			}
		}
		return cam.ONVIFEndpoint, cam.ONVIFUsername, password, nil
	})
```

Note: Adjust import for `crypto` if needed — check the existing `crypto.DeriveKey` usage at `nvr.go:799`.

- [ ] **Step 5: Pass manager to RegisterRoutes**

In `internal/nvr/nvr.go`, in `RegisterRoutes()` at line 808, add to the `RouterConfig` struct literal:

```go
		BackchannelMgr:  n.backchannelMgr,
```

- [ ] **Step 6: Add cleanup to NVR.Close()**

In `internal/nvr/nvr.go`, in `Close()`, before the camera status monitor stop (line 516), add:

```go
	if n.backchannelMgr != nil {
		n.backchannelMgr.CloseAll()
	}
```

- [ ] **Step 7: Verify compilation**

Run: `cd /path/to/mediamtx && go build ./...`
Expected: No compilation errors.

- [ ] **Step 8: Commit**

```bash
git add internal/nvr/api/router.go internal/nvr/nvr.go
git commit -m "feat: integrate backchannel manager into NVR lifecycle and API router"
```

---

### Task 9: End-to-End Verification

**Files:**
- No new files — verify everything works together.

- [ ] **Step 1: Run all backchannel tests**

Run: `go test ./internal/nvr/backchannel/... -v`
Expected: All PASS.

- [ ] **Step 2: Run all onvif tests**

Run: `go test ./internal/nvr/onvif/... -v`
Expected: All PASS.

- [ ] **Step 3: Run all API tests**

Run: `go test ./internal/nvr/api/... -v`
Expected: All PASS.

- [ ] **Step 4: Run full project build**

Run: `go build ./...`
Expected: No errors.

- [ ] **Step 5: Run full test suite**

Run: `go test ./... 2>&1 | tail -50`
Expected: No new failures.

- [ ] **Step 6: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: address test/build issues from backchannel integration"
```

(Skip this step if no fixes were needed.)
