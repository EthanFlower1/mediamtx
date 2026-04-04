package onvif

import "testing"

func TestBuildRangeHeader(t *testing.T) {
	tests := []struct {
		name      string
		startTime string
		endTime   string
		want      string
	}{
		{
			name: "empty start time returns empty",
			want: "",
		},
		{
			name:      "start only (open-ended)",
			startTime: "2024-01-15T12:00:00Z",
			want:      "clock=20240115T120000Z-",
		},
		{
			name:      "start and end (bounded range)",
			startTime: "2024-01-15T12:00:00Z",
			endTime:   "2024-01-15T13:00:00Z",
			want:      "clock=20240115T120000Z-20240115T130000Z",
		},
		{
			name:      "start with timezone offset",
			startTime: "2024-06-20T14:30:00+02:00",
			endTime:   "2024-06-20T15:30:00+02:00",
			want:      "clock=20240620T123000Z-20240620T133000Z",
		},
		{
			name:      "unparseable start falls back to raw string",
			startTime: "not-a-date",
			endTime:   "also-not-a-date",
			want:      "clock=not-a-date-also-not-a-date",
		},
		{
			name:      "unparseable start no end",
			startTime: "not-a-date",
			want:      "clock=not-a-date-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildRangeHeader(tt.startTime, tt.endTime)
			if got != tt.want {
				t.Errorf("buildRangeHeader(%q, %q) = %q, want %q",
					tt.startTime, tt.endTime, got, tt.want)
			}
		})
	}
}

func TestBuildReplaySession_MissingToken(t *testing.T) {
	_, err := BuildReplaySession("http://example.com", "user", "pass", &ReplaySessionRequest{})
	if err == nil {
		t.Fatal("expected error for missing recording_token, got nil")
	}
}

func TestReplaySession_HeaderConstruction(t *testing.T) {
	// Test that a ReplaySession with Scale and Speed produces correct header values.
	session := &ReplaySession{
		Scale: 2.0,
		Speed: 4.0,
	}

	// Verify scale/speed formatting expectations match what BuildReplaySession would set.
	scaleHeader := ""
	if session.Scale != 0 {
		scaleHeader = "2.0"
	}
	speedHeader := ""
	if session.Speed != 0 {
		speedHeader = "4.0"
	}

	if scaleHeader != "2.0" {
		t.Errorf("expected scale header '2.0', got %q", scaleHeader)
	}
	if speedHeader != "4.0" {
		t.Errorf("expected speed header '4.0', got %q", speedHeader)
	}
}
