package onvif

import (
	"strings"
	"testing"
)

func TestAudioOutputConfigRoundTrip(t *testing.T) {
	cfg := AudioOutputConfig{
		Token:       "AudioOutputToken1",
		Name:        "Audio Output 1",
		OutputToken: "AudioOut_1",
	}

	if cfg.Token != "AudioOutputToken1" {
		t.Errorf("expected Token %q, got %q", "AudioOutputToken1", cfg.Token)
	}
	if cfg.Name != "Audio Output 1" {
		t.Errorf("expected Name %q, got %q", "Audio Output 1", cfg.Name)
	}
	if cfg.OutputToken != "AudioOut_1" {
		t.Errorf("expected OutputToken %q, got %q", "AudioOut_1", cfg.OutputToken)
	}
}

func TestAudioDecoderConfigRoundTrip(t *testing.T) {
	cfg := AudioDecoderConfig{
		Token: "AudioDecoderToken1",
		Name:  "Audio Decoder 1",
	}

	if cfg.Token != "AudioDecoderToken1" {
		t.Errorf("expected Token %q, got %q", "AudioDecoderToken1", cfg.Token)
	}
	if cfg.Name != "Audio Decoder 1" {
		t.Errorf("expected Name %q, got %q", "Audio Decoder 1", cfg.Name)
	}
}

func TestAudioDecoderOptionsCodecDetection(t *testing.T) {
	opts := AudioDecoderOptions{
		AACSupported:  true,
		G711Supported: true,
		AAC: &CodecOptions{
			Bitrates:    []int{32000, 64000, 128000},
			SampleRates: []int{8000, 16000, 44100},
		},
		G711: &CodecOptions{
			Bitrates:    []int{64000},
			SampleRates: []int{8000},
		},
	}

	if !opts.AACSupported {
		t.Error("expected AACSupported to be true")
	}
	if !opts.G711Supported {
		t.Error("expected G711Supported to be true")
	}
	if opts.AAC == nil {
		t.Fatal("expected AAC options to be non-nil")
	}
	if len(opts.AAC.Bitrates) != 3 {
		t.Errorf("expected 3 AAC bitrates, got %d", len(opts.AAC.Bitrates))
	}
	if len(opts.AAC.SampleRates) != 3 {
		t.Errorf("expected 3 AAC sample rates, got %d", len(opts.AAC.SampleRates))
	}
	if opts.G711 == nil {
		t.Fatal("expected G711 options to be non-nil")
	}
	if len(opts.G711.Bitrates) != 1 {
		t.Errorf("expected 1 G711 bitrate, got %d", len(opts.G711.Bitrates))
	}
}

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
	if codec == nil {
		t.Fatal("expected non-nil codec, got nil")
	}
	if codec.Encoding != "AAC" {
		t.Errorf("expected encoding %q, got %q", "AAC", codec.Encoding)
	}
	if codec.Bitrate != 64000 {
		t.Errorf("expected bitrate 64000, got %d", codec.Bitrate)
	}
	if codec.SampleRate != 16000 {
		t.Errorf("expected sample rate 16000, got %d", codec.SampleRate)
	}
}

func TestNegotiateCodecFallsBackToG711(t *testing.T) {
	opts := &AudioDecoderOptions{
		AACSupported:  false,
		G711Supported: true,
		AAC:           nil,
		G711: &CodecOptions{
			Bitrates:    []int{64000},
			SampleRates: []int{8000},
		},
	}

	codec := NegotiateCodec(opts)
	if codec == nil {
		t.Fatal("expected non-nil codec, got nil")
	}
	if codec.Encoding != "G711" {
		t.Errorf("expected encoding %q, got %q", "G711", codec.Encoding)
	}
	if codec.Bitrate != 64000 {
		t.Errorf("expected bitrate 64000, got %d", codec.Bitrate)
	}
	if codec.SampleRate != 8000 {
		t.Errorf("expected sample rate 8000, got %d", codec.SampleRate)
	}
}

func TestNegotiateCodecNoneSupported(t *testing.T) {
	opts := &AudioDecoderOptions{
		AACSupported:  false,
		G711Supported: false,
		AAC:           nil,
		G711:          nil,
	}

	codec := NegotiateCodec(opts)
	if codec != nil {
		t.Errorf("expected nil codec when no codecs supported, got %+v", codec)
	}
}

func TestAudioCapabilitiesStruct(t *testing.T) {
	caps := AudioCapabilities{
		HasBackchannel:   true,
		AudioSources:     1,
		AudioOutputs:     2,
		BackchannelCodec: "G711",
	}
	if caps.BackchannelCodec != "G711" {
		t.Fatalf("expected G711, got %s", caps.BackchannelCodec)
	}
	if caps.AudioOutputs != 2 {
		t.Fatalf("expected 2 outputs, got %d", caps.AudioOutputs)
	}
}

func TestBackchannelStreamURISOAP(t *testing.T) {
	profileToken := "Profile_1"
	innerBody := backchannelStreamURIBody(profileToken)
	envelope := backchannelSOAP(innerBody)

	if !strings.Contains(envelope, profileToken) {
		t.Errorf("SOAP body missing profile token %q", profileToken)
	}
	if !strings.Contains(envelope, "RTP-Unicast") {
		t.Error("SOAP body missing transport type \"RTP-Unicast\"")
	}
	if !strings.Contains(envelope, "RTSP") {
		t.Error("SOAP body missing protocol \"RTSP\"")
	}
}
