package ai

import (
	"testing"
)

func TestNewClassThresholds_Defaults(t *testing.T) {
	ct := NewClassThresholds("", 0.5)
	if ct == nil {
		t.Fatal("expected non-nil ClassThresholds")
	}
	// Person should use default of 0.5.
	if v, ok := ct.perClass["person"]; !ok || v != 0.5 {
		t.Errorf("person threshold = %v, want 0.5", v)
	}
	// Cat should use default of 0.3.
	if v, ok := ct.perClass["cat"]; !ok || v != 0.3 {
		t.Errorf("cat threshold = %v, want 0.3", v)
	}
}

func TestNewClassThresholds_Overrides(t *testing.T) {
	ct := NewClassThresholds(`{"person":0.8,"custom_class":0.6}`, 0.5)
	// Person overridden to 0.8.
	if v := ct.perClass["person"]; v != 0.8 {
		t.Errorf("person threshold = %v, want 0.8", v)
	}
	// Custom class added.
	if v := ct.perClass["custom_class"]; v != 0.6 {
		t.Errorf("custom_class threshold = %v, want 0.6", v)
	}
	// Cat unchanged at default 0.3.
	if v := ct.perClass["cat"]; v != 0.3 {
		t.Errorf("cat threshold = %v, want 0.3", v)
	}
}

func TestNewClassThresholds_InvalidJSON(t *testing.T) {
	ct := NewClassThresholds("{bad json", 0.5)
	// Should fall back to defaults without panic.
	if v := ct.perClass["person"]; v != 0.5 {
		t.Errorf("person threshold = %v, want 0.5 (default)", v)
	}
}

func TestFilterDetections(t *testing.T) {
	ct := NewClassThresholds(`{"person":0.6,"car":0.3}`, 0.5)

	dets := []Detection{
		{Class: "person", Confidence: 0.7},  // above 0.6 -> keep
		{Class: "person", Confidence: 0.5},  // below 0.6 -> filter
		{Class: "car", Confidence: 0.35},     // above 0.3 -> keep
		{Class: "car", Confidence: 0.2},      // below 0.3 -> filter
		{Class: "unknown", Confidence: 0.6},  // above global 0.5 -> keep
		{Class: "unknown", Confidence: 0.4},  // below global 0.5 -> filter
	}

	filtered := ct.FilterDetections(dets)
	if len(filtered) != 3 {
		t.Fatalf("expected 3 detections after filtering, got %d", len(filtered))
	}

	expected := []struct {
		class string
		conf  float32
	}{
		{"person", 0.7},
		{"car", 0.35},
		{"unknown", 0.6},
	}
	for i, e := range expected {
		if filtered[i].Class != e.class || filtered[i].Confidence != e.conf {
			t.Errorf("filtered[%d] = {%s, %.2f}, want {%s, %.2f}",
				i, filtered[i].Class, filtered[i].Confidence, e.class, e.conf)
		}
	}
}

func TestFilterDetections_Empty(t *testing.T) {
	ct := NewClassThresholds("", 0.5)
	result := ct.FilterDetections(nil)
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
	result = ct.FilterDetections([]Detection{})
	if len(result) != 0 {
		t.Errorf("expected empty for empty input, got %v", result)
	}
}
