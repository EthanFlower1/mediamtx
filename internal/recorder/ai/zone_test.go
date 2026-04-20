package ai

import (
	"testing"
)

func TestValidateZone_MinPoints(t *testing.T) {
	z := &DetectionZone{
		Name:   "test",
		Points: []Point{{0, 0}, {1, 0}},
	}
	err := ValidateZone(z)
	if err == nil {
		t.Fatal("expected error for polygon with < 3 points")
	}
	if err.Error() != "zone polygon must have at least 3 points, got 2" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateZone_EmptyName(t *testing.T) {
	z := &DetectionZone{
		Name:   "",
		Points: []Point{{0, 0}, {1, 0}, {1, 1}},
	}
	err := ValidateZone(z)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestValidateZone_OutOfRange(t *testing.T) {
	z := &DetectionZone{
		Name:   "test",
		Points: []Point{{0, 0}, {1.5, 0}, {1, 1}},
	}
	err := ValidateZone(z)
	if err == nil {
		t.Fatal("expected error for out-of-range coordinates")
	}
}

func TestValidateZone_SelfIntersecting(t *testing.T) {
	// Bowtie shape: edges (0,0)-(1,1) and (1,0)-(0,1) cross.
	z := &DetectionZone{
		Name:   "bowtie",
		Points: []Point{{0, 0}, {1, 1}, {1, 0}, {0, 1}},
	}
	err := ValidateZone(z)
	if err == nil {
		t.Fatal("expected error for self-intersecting polygon")
	}
}

func TestValidateZone_ValidTriangle(t *testing.T) {
	z := &DetectionZone{
		Name:   "triangle",
		Points: []Point{{0.1, 0.1}, {0.9, 0.1}, {0.5, 0.9}},
	}
	if err := ValidateZone(z); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateZone_ValidSquare(t *testing.T) {
	z := &DetectionZone{
		Name:   "square",
		Points: []Point{{0, 0}, {1, 0}, {1, 1}, {0, 1}},
	}
	if err := ValidateZone(z); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPointInZone(t *testing.T) {
	z := &DetectionZone{
		Name:    "square",
		Points:  []Point{{0.2, 0.2}, {0.8, 0.2}, {0.8, 0.8}, {0.2, 0.8}},
		Enabled: true,
	}

	tests := []struct {
		name   string
		x, y   float64
		expect bool
	}{
		{"centre", 0.5, 0.5, true},
		{"outside", 0.1, 0.1, false},
		{"corner edge", 0.2, 0.2, true},
		{"far outside", 0.95, 0.95, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PointInZone(z, tt.x, tt.y)
			if got != tt.expect {
				t.Errorf("PointInZone(%f, %f) = %v, want %v", tt.x, tt.y, got, tt.expect)
			}
		})
	}
}

func TestDetectionInZone_ClassFilter(t *testing.T) {
	z := &DetectionZone{
		Name:        "driveway",
		Points:      []Point{{0, 0}, {1, 0}, {1, 1}, {0, 1}},
		ClassFilter: []string{"person", "car"},
		Enabled:     true,
	}

	box := BoundingBox{X: 0.4, Y: 0.4, W: 0.2, H: 0.2}

	if !DetectionInZone(z, "person", box) {
		t.Error("expected person to match class filter")
	}
	if !DetectionInZone(z, "car", box) {
		t.Error("expected car to match class filter")
	}
	if DetectionInZone(z, "dog", box) {
		t.Error("expected dog to NOT match class filter")
	}
}

func TestDetectionInZone_Disabled(t *testing.T) {
	z := &DetectionZone{
		Name:    "disabled",
		Points:  []Point{{0, 0}, {1, 0}, {1, 1}, {0, 1}},
		Enabled: false,
	}
	box := BoundingBox{X: 0.4, Y: 0.4, W: 0.2, H: 0.2}
	if DetectionInZone(z, "person", box) {
		t.Error("expected disabled zone to never match")
	}
}

func TestMatchingZones_Overlapping(t *testing.T) {
	zones := []DetectionZone{
		{
			Name:    "zone-a",
			Points:  []Point{{0, 0}, {0.6, 0}, {0.6, 0.6}, {0, 0.6}},
			Enabled: true,
		},
		{
			Name:    "zone-b",
			Points:  []Point{{0.4, 0.4}, {1, 0.4}, {1, 1}, {0.4, 1}},
			Enabled: true,
		},
		{
			Name:    "zone-c",
			Points:  []Point{{0.8, 0.8}, {1, 0.8}, {1, 1}, {0.8, 1}},
			Enabled: true,
		},
	}

	// Point (0.5, 0.5) is in zone-a and zone-b but not zone-c.
	box := BoundingBox{X: 0.4, Y: 0.4, W: 0.2, H: 0.2} // centre at (0.5, 0.5)
	matched := MatchingZones(zones, "person", box)
	if len(matched) != 2 {
		t.Fatalf("expected 2 matching zones, got %d", len(matched))
	}
	names := map[string]bool{}
	for _, m := range matched {
		names[m.Name] = true
	}
	if !names["zone-a"] || !names["zone-b"] {
		t.Errorf("expected zone-a and zone-b, got %v", names)
	}
}

func TestMatchingZones_ClassFilterPerZone(t *testing.T) {
	zones := []DetectionZone{
		{
			Name:        "person-only",
			Points:      []Point{{0, 0}, {1, 0}, {1, 1}, {0, 1}},
			ClassFilter: []string{"person"},
			Enabled:     true,
		},
		{
			Name:        "car-only",
			Points:      []Point{{0, 0}, {1, 0}, {1, 1}, {0, 1}},
			ClassFilter: []string{"car"},
			Enabled:     true,
		},
	}

	box := BoundingBox{X: 0.4, Y: 0.4, W: 0.2, H: 0.2}

	matched := MatchingZones(zones, "person", box)
	if len(matched) != 1 || matched[0].Name != "person-only" {
		t.Errorf("expected only person-only zone, got %v", matched)
	}

	matched = MatchingZones(zones, "car", box)
	if len(matched) != 1 || matched[0].Name != "car-only" {
		t.Errorf("expected only car-only zone, got %v", matched)
	}
}
