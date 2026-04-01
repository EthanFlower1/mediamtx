// internal/nvr/ai/merge_test.go
package ai

import "testing"

func TestMergeDetectionsNoDuplicates(t *testing.T) {
	yolo := []Detection{
		{Class: "person", Confidence: 0.9, Box: BoundingBox{0.1, 0.1, 0.3, 0.5}, Source: SourceYOLO},
	}
	onvif := []Detection{
		{Class: "car", Confidence: 0.8, Box: BoundingBox{0.6, 0.6, 0.2, 0.2}, Source: SourceONVIF},
	}
	merged := MergeDetections(yolo, onvif)
	if len(merged) != 2 {
		t.Fatalf("expected 2 detections, got %d", len(merged))
	}
}

func TestMergeDetectionsDedup(t *testing.T) {
	box := BoundingBox{0.1, 0.1, 0.3, 0.5}
	yolo := []Detection{
		{Class: "person", Confidence: 0.9, Box: box, Source: SourceYOLO},
	}
	onvif := []Detection{
		{Class: "person", Confidence: 0.85, Box: BoundingBox{0.11, 0.11, 0.29, 0.49}, Source: SourceONVIF},
	}
	merged := MergeDetections(yolo, onvif)
	if len(merged) != 1 {
		t.Fatalf("expected 1 detection (deduped), got %d", len(merged))
	}
	if merged[0].Source != SourceYOLO {
		t.Error("expected YOLO detection to be kept")
	}
}

func TestMergeDetectionsDifferentClass(t *testing.T) {
	box := BoundingBox{0.1, 0.1, 0.3, 0.5}
	yolo := []Detection{
		{Class: "person", Confidence: 0.9, Box: box, Source: SourceYOLO},
	}
	onvif := []Detection{
		{Class: "line_crossing", Confidence: 1.0, Box: box, Source: SourceONVIF},
	}
	merged := MergeDetections(yolo, onvif)
	if len(merged) != 2 {
		t.Fatalf("expected 2 detections (different class), got %d", len(merged))
	}
}

func TestMergeEmpty(t *testing.T) {
	yolo := []Detection{{Class: "person", Confidence: 0.9, Box: BoundingBox{0.1, 0.1, 0.3, 0.5}}}
	if got := MergeDetections(yolo, nil); len(got) != 1 {
		t.Errorf("yolo + nil = %d, want 1", len(got))
	}
	if got := MergeDetections(nil, yolo); len(got) != 1 {
		t.Errorf("nil + yolo = %d, want 1", len(got))
	}
}
