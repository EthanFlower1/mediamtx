package onvif

import (
	"testing"
	"time"
)

func TestBuildReplaySession_ForwardPlayback(t *testing.T) {
	uri := "rtsp://192.168.1.100/recording/token123"
	token := "token123"
	start := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)

	session, err := BuildReplaySession(uri, token, start, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if session.ReplayURI != uri {
		t.Errorf("ReplayURI = %q, want %q", session.ReplayURI, uri)
	}
	if session.RecordingToken != token {
		t.Errorf("RecordingToken = %q, want %q", session.RecordingToken, token)
	}
	if session.Scale != 1 {
		t.Errorf("Scale = %f, want 1", session.Scale)
	}
	if session.Reverse {
		t.Error("Reverse should be false for positive scale")
	}

	wantRange := "clock=20260115T103000.000Z-"
	if session.RTSPHeaders["Range"] != wantRange {
		t.Errorf("Range header = %q, want %q", session.RTSPHeaders["Range"], wantRange)
	}
	if session.RTSPHeaders["Scale"] != "1" {
		t.Errorf("Scale header = %q, want %q", session.RTSPHeaders["Scale"], "1")
	}
}

func TestBuildReplaySession_ReversePlayback(t *testing.T) {
	uri := "rtsp://192.168.1.100/recording/token123"
	token := "token123"
	start := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name      string
		scale     float64
		wantScale string
	}{
		{"reverse 1x", -1, "-1"},
		{"reverse 2x", -2, "-2"},
		{"reverse 4x", -4, "-4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, err := BuildReplaySession(uri, token, start, tt.scale)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !session.Reverse {
				t.Error("Reverse should be true for negative scale")
			}
			if session.Scale != tt.scale {
				t.Errorf("Scale = %f, want %f", session.Scale, tt.scale)
			}
			if session.RTSPHeaders["Scale"] != tt.wantScale {
				t.Errorf("Scale header = %q, want %q", session.RTSPHeaders["Scale"], tt.wantScale)
			}

			wantRange := "clock=20260115T103000.000Z-"
			if session.RTSPHeaders["Range"] != wantRange {
				t.Errorf("Range header = %q, want %q", session.RTSPHeaders["Range"], wantRange)
			}
		})
	}
}

func TestBuildReplaySession_FastForward(t *testing.T) {
	uri := "rtsp://192.168.1.100/recording/token123"
	token := "token123"
	start := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name      string
		scale     float64
		wantScale string
	}{
		{"2x forward", 2, "2"},
		{"4x forward", 4, "4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, err := BuildReplaySession(uri, token, start, tt.scale)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if session.Reverse {
				t.Error("Reverse should be false for positive scale")
			}
			if session.RTSPHeaders["Scale"] != tt.wantScale {
				t.Errorf("Scale header = %q, want %q", session.RTSPHeaders["Scale"], tt.wantScale)
			}
		})
	}
}

func TestBuildReplaySession_InvalidScale(t *testing.T) {
	uri := "rtsp://192.168.1.100/recording/token123"
	token := "token123"
	start := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)

	invalidScales := []float64{0, 0.5, -0.5, 3, -3, 8, -8, 10}
	for _, s := range invalidScales {
		_, err := BuildReplaySession(uri, token, start, s)
		if err == nil {
			t.Errorf("expected error for scale %.1f, got nil", s)
		}
	}
}

func TestBuildReplaySession_MissingFields(t *testing.T) {
	uri := "rtsp://192.168.1.100/recording/token123"
	token := "token123"
	start := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name  string
		uri   string
		token string
		start time.Time
	}{
		{"empty URI", "", token, start},
		{"empty token", uri, "", start},
		{"zero time", uri, token, time.Time{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildReplaySession(tt.uri, tt.token, tt.start, 1)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestIsAllowedScale(t *testing.T) {
	allowed := []float64{-4, -2, -1, 1, 2, 4}
	for _, s := range allowed {
		if !isAllowedScale(s) {
			t.Errorf("isAllowedScale(%f) = false, want true", s)
		}
	}

	notAllowed := []float64{0, 0.5, -0.5, 3, -3, 100}
	for _, s := range notAllowed {
		if isAllowedScale(s) {
			t.Errorf("isAllowedScale(%f) = true, want false", s)
		}
	}
}

func TestFormatScale(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{1, "1"},
		{-1, "-1"},
		{2, "2"},
		{-2, "-2"},
		{4, "4"},
		{-4, "-4"},
		{1.5, "1.5"},
		{-2.5, "-2.5"},
	}

	for _, tt := range tests {
		got := formatScale(tt.input)
		if got != tt.want {
			t.Errorf("formatScale(%f) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
