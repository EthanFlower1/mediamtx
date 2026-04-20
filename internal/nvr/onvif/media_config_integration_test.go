package onvif

import (
	"os"
	"testing"
)

// These tests hit real ONVIF cameras on the local network.
// Skip in CI; run locally with:
//
//   ONVIF_TEST_CAMERA=http://192.168.1.218/onvif/device_service \
//   ONVIF_TEST_USER=admin \
//   ONVIF_TEST_PASS=Gsd4life. \
//   go test -v -run TestIntegration ./internal/nvr/onvif/

func getTestCamera(t *testing.T) (xaddr, user, pass string) {
	t.Helper()
	xaddr = os.Getenv("ONVIF_TEST_CAMERA")
	user = os.Getenv("ONVIF_TEST_USER")
	pass = os.Getenv("ONVIF_TEST_PASS")
	if xaddr == "" {
		t.Skip("ONVIF_TEST_CAMERA not set — skipping real camera test")
	}
	return
}

func TestIntegrationGetProfiles(t *testing.T) {
	xaddr, user, pass := getTestCamera(t)

	profiles, err := GetProfilesFull(xaddr, user, pass)
	if err != nil {
		t.Fatalf("GetProfilesFull: %v", err)
	}
	if len(profiles) == 0 {
		t.Fatal("expected at least one profile")
	}

	for _, p := range profiles {
		t.Logf("Profile: token=%q name=%q", p.Token, p.Name)
		if p.VideoEncoder != nil {
			t.Logf("  VideoEncoder: token=%q encoding=%s %dx%d fps=%d bitrate=%d",
				p.VideoEncoder.Token, p.VideoEncoder.Encoding,
				p.VideoEncoder.Width, p.VideoEncoder.Height,
				p.VideoEncoder.FrameRate, p.VideoEncoder.BitrateLimit)
		}
	}
}

func TestIntegrationGetVideoEncoderConfig(t *testing.T) {
	xaddr, user, pass := getTestCamera(t)

	// Get profiles to find encoder config tokens.
	profiles, err := GetProfilesFull(xaddr, user, pass)
	if err != nil {
		t.Fatalf("GetProfilesFull: %v", err)
	}

	for _, p := range profiles {
		if p.VideoEncoder == nil {
			continue
		}
		cfg, err := GetVideoEncoderConfig(xaddr, user, pass, p.VideoEncoder.Token)
		if err != nil {
			t.Errorf("GetVideoEncoderConfig(%q): %v", p.VideoEncoder.Token, err)
			continue
		}
		t.Logf("Config %q: encoding=%s %dx%d fps=%d bitrate=%d quality=%.1f interval=%d",
			cfg.Token, cfg.Encoding,
			cfg.Width, cfg.Height,
			cfg.FrameRate, cfg.BitrateLimit, cfg.Quality, cfg.EncodingInterval)
	}
}

func TestIntegrationGetVideoEncoderOptions(t *testing.T) {
	xaddr, user, pass := getTestCamera(t)

	profiles, err := GetProfilesFull(xaddr, user, pass)
	if err != nil {
		t.Fatalf("GetProfilesFull: %v", err)
	}

	for _, p := range profiles {
		if p.VideoEncoder == nil {
			continue
		}
		opts, err := GetVideoEncoderOpts(xaddr, user, pass, p.VideoEncoder.Token)
		if err != nil {
			t.Errorf("GetVideoEncoderOpts(%q): %v", p.VideoEncoder.Token, err)
			continue
		}
		t.Logf("Options for %q:", p.VideoEncoder.Token)
		t.Logf("  Encodings: %v", opts.Encodings)
		t.Logf("  Resolutions: %v", opts.Resolutions)
		t.Logf("  FrameRate: %d-%d", opts.FrameRateRange.Min, opts.FrameRateRange.Max)
		t.Logf("  Quality: %d-%d", opts.QualityRange.Min, opts.QualityRange.Max)
		t.Logf("  H264 Profiles: %v", opts.H264Profiles)
	}
}

func TestIntegrationSetFrameRate(t *testing.T) {
	xaddr, user, pass := getTestCamera(t)

	// Test each encoder config to find which ones actually accept changes.
	profiles, err := GetProfilesFull(xaddr, user, pass)
	if err != nil {
		t.Fatalf("GetProfilesFull: %v", err)
	}

	for _, p := range profiles {
		if p.VideoEncoder == nil {
			continue
		}
		t.Run(p.VideoEncoder.Token+"_"+p.Name, func(t *testing.T) {
			testSetFrameRateForToken(t, xaddr, user, pass, p.VideoEncoder.Token)
		})
	}
}

func testSetFrameRateForToken(t *testing.T, xaddr, user, pass, token string) {
	target, err := GetVideoEncoderConfig(xaddr, user, pass, token)
	if err != nil {
		t.Fatalf("GetVideoEncoderConfig: %v", err)
	}

	t.Logf("Original: token=%q encoding=%s fps=%d bitrate=%d",
		target.Token, target.Encoding, target.FrameRate, target.BitrateLimit)

	// Change frame rate.
	newFPS := 10
	if target.FrameRate == 10 {
		newFPS = 15
	}

	modified := *target
	modified.FrameRate = newFPS
	t.Logf("Setting fps to %d...", newFPS)

	if err := SetVideoEncoderConfig(xaddr, user, pass, &modified); err != nil {
		t.Fatalf("SetVideoEncoderConfig: %v", err)
	}

	// Read back.
	readBack, err := GetVideoEncoderConfig(xaddr, user, pass, target.Token)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}

	t.Logf("ReadBack: encoding=%s fps=%d bitrate=%d",
		readBack.Encoding, readBack.FrameRate, readBack.BitrateLimit)

	if readBack.FrameRate != newFPS {
		t.Errorf("FAIL: fps not updated: got %d, want %d", readBack.FrameRate, newFPS)
	}

	// Restore.
	t.Log("Restoring original...")
	if err := SetVideoEncoderConfig(xaddr, user, pass, target); err != nil {
		t.Errorf("restore: %v", err)
	}
}

func TestIntegrationSetEncoding(t *testing.T) {
	xaddr, user, pass := getTestCamera(t)

	profiles, err := GetProfilesFull(xaddr, user, pass)
	if err != nil {
		t.Fatalf("GetProfilesFull: %v", err)
	}

	// Find a profile that supports multiple encodings.
	var target *VideoEncoderConfig
	var targetToken string
	for _, p := range profiles {
		if p.VideoEncoder == nil {
			continue
		}
		opts, err := GetVideoEncoderOpts(xaddr, user, pass, p.VideoEncoder.Token)
		if err != nil || len(opts.Encodings) < 2 {
			continue
		}
		cfg, err := GetVideoEncoderConfig(xaddr, user, pass, p.VideoEncoder.Token)
		if err != nil {
			continue
		}
		target = cfg
		targetToken = p.VideoEncoder.Token
		break
	}
	if target == nil {
		t.Fatal("no video encoder configs found")
	}

	t.Logf("Current: token=%q encoding=%s %dx%d", targetToken, target.Encoding, target.Width, target.Height)

	// Check supported encodings.
	opts, err := GetVideoEncoderOpts(xaddr, user, pass, targetToken)
	if err != nil {
		t.Fatalf("GetVideoEncoderOpts: %v", err)
	}
	t.Logf("Supported encodings: %v", opts.Encodings)

	// Try to switch.
	var newEncoding string
	if target.Encoding == "H264" {
		for _, e := range opts.Encodings {
			if e == "JPEG" {
				newEncoding = "JPEG"
				break
			}
		}
	} else if target.Encoding == "JPEG" {
		for _, e := range opts.Encodings {
			if e == "H264" {
				newEncoding = "H264"
				break
			}
		}
	}

	if newEncoding == "" {
		t.Skip("camera doesn't support switching encoding for this profile")
	}

	// Use a resolution from the options for the new encoding.
	modified := *target
	modified.Encoding = newEncoding
	// Keep same resolution — let camera adjust if needed.

	t.Logf("Setting encoding to %s...", newEncoding)
	if err := SetVideoEncoderConfig(xaddr, user, pass, &modified); err != nil {
		t.Fatalf("SetVideoEncoderConfig to %s: %v", newEncoding, err)
	}

	// Read back.
	readBack, err := GetVideoEncoderConfig(xaddr, user, pass, targetToken)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}

	t.Logf("ReadBack: encoding=%s %dx%d fps=%d",
		readBack.Encoding, readBack.Width, readBack.Height, readBack.FrameRate)

	if readBack.Encoding != newEncoding {
		t.Errorf("FAIL: encoding not updated: got %s, want %s", readBack.Encoding, newEncoding)
	}

	// Restore.
	t.Log("Restoring original...")
	if err := SetVideoEncoderConfig(xaddr, user, pass, target); err != nil {
		t.Errorf("restore: %v", err)
	}
}
