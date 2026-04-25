package audio

import (
	"testing"
)

func TestAllEventTypes(t *testing.T) {
	types := AllEventTypes()
	if len(types) != 4 {
		t.Fatalf("expected 4 event types, got %d", len(types))
	}

	expected := map[EventType]bool{
		EventGunshot:      true,
		EventGlassBreak:   true,
		EventRaisedVoices: true,
		EventSirenHorn:    true,
	}
	for _, et := range types {
		if !expected[et] {
			t.Errorf("unexpected event type: %s", et)
		}
	}
}

func TestConfig_ConfidenceFor(t *testing.T) {
	cfg := Config{
		CameraID: "cam1",
		Enabled:  true,
		ConfidenceThresholds: map[EventType]float32{
			EventGunshot: 0.80,
		},
	}

	if got := cfg.ConfidenceFor(EventGunshot); got != 0.80 {
		t.Errorf("expected 0.80 for gunshot, got %f", got)
	}
	if got := cfg.ConfidenceFor(EventGlassBreak); got != DefaultConfidenceThreshold {
		t.Errorf("expected default %f for glass_break, got %f", DefaultConfidenceThreshold, got)
	}
}

func TestConfig_IsEventEnabled(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		event   EventType
		enabled bool
	}{
		{
			name:    "disabled camera",
			config:  Config{CameraID: "c1", Enabled: false},
			event:   EventGunshot,
			enabled: false,
		},
		{
			name:    "enabled all events",
			config:  Config{CameraID: "c1", Enabled: true},
			event:   EventGunshot,
			enabled: true,
		},
		{
			name: "specific events only",
			config: Config{
				CameraID:      "c1",
				Enabled:       true,
				EnabledEvents: []EventType{EventGunshot, EventSirenHorn},
			},
			event:   EventGlassBreak,
			enabled: false,
		},
		{
			name: "specific event present",
			config: Config{
				CameraID:      "c1",
				Enabled:       true,
				EnabledEvents: []EventType{EventGunshot, EventSirenHorn},
			},
			event:   EventSirenHorn,
			enabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsEventEnabled(tt.event); got != tt.enabled {
				t.Errorf("IsEventEnabled(%s) = %v, want %v", tt.event, got, tt.enabled)
			}
		})
	}
}
