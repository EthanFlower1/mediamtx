package api

import (
	"testing"
)

func TestValidateStreamRTSPURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid rtsp", "rtsp://192.168.1.100:554/stream1", false},
		{"valid rtsps", "rtsps://192.168.1.100:554/stream1", false},
		{"valid with credentials", "rtsp://admin:pass@192.168.1.100:554/cam/realmonitor", false},
		{"empty url", "", true},
		{"http url", "http://example.com/stream", true},
		{"no scheme", "192.168.1.100:554/stream", true},
		{"ftp url", "ftp://example.com/file", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStreamURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateStreamURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}
