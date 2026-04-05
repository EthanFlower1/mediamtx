package ai

import (
	"fmt"
	"math"
)

// Point represents a 2D point with normalized [0,1] coordinates.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// DetectionZone defines a polygonal region on a camera's field of view.
// Detections are checked against all zones for a camera; a detection
// can match multiple overlapping zones.
type DetectionZone struct {
	ID          string   `json:"id"`
	CameraID    string   `json:"camera_id"`
	Name        string   `json:"name"`
	Points      []Point  `json:"points"`
	ClassFilter []string `json:"class_filter"` // empty = all classes
	Enabled     bool     `json:"enabled"`
}

// ValidateZone checks that a DetectionZone has a valid polygon:
//   - At least 3 points
//   - All coordinates in [0, 1]
//   - Non-empty name
//   - No self-intersecting edges
//
// Returns a descriptive error if validation fails, nil otherwise.
func ValidateZone(z *DetectionZone) error {
	if z.Name == "" {
		return fmt.Errorf("zone name must not be empty")
	}
	if len(z.Points) < 3 {
		return fmt.Errorf("zone polygon must have at least 3 points, got %d", len(z.Points))
	}
	for i, p := range z.Points {
		if p.X < 0 || p.X > 1 || p.Y < 0 || p.Y > 1 {
			return fmt.Errorf("point %d has out-of-range coordinates (%.4f, %.4f); values must be in [0, 1]", i, p.X, p.Y)
		}
	}
	if selfIntersects(z.Points) {
		return fmt.Errorf("zone polygon must not have self-intersecting edges")
	}
	return nil
}

// PointInZone tests whether a normalized (x, y) coordinate falls inside the
// zone's polygon using the ray-casting algorithm.
func PointInZone(z *DetectionZone, x, y float64) bool {
	return pointInPolygon(z.Points, x, y)
}

// DetectionInZone tests whether the centre of a bounding box falls inside the zone.
// If the zone has a non-empty ClassFilter, the detection's class must also
// appear in that list.
func DetectionInZone(z *DetectionZone, className string, box BoundingBox) bool {
	if !z.Enabled {
		return false
	}
	if len(z.ClassFilter) > 0 {
		found := false
		for _, c := range z.ClassFilter {
			if c == className {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	cx := float64(box.X + box.W/2)
	cy := float64(box.Y + box.H/2)
	return pointInPolygon(z.Points, cx, cy)
}

// MatchingZones returns every zone from the given list that contains the
// detection. This enables overlapping-zone support: a single detection can
// trigger events in multiple zones.
func MatchingZones(zones []DetectionZone, className string, box BoundingBox) []DetectionZone {
	var matched []DetectionZone
	for i := range zones {
		if DetectionInZone(&zones[i], className, box) {
			matched = append(matched, zones[i])
		}
	}
	return matched
}

// pointInPolygon implements the ray-casting (even-odd rule) algorithm.
func pointInPolygon(poly []Point, x, y float64) bool {
	n := len(poly)
	if n < 3 {
		return false
	}
	inside := false
	j := n - 1
	for i := 0; i < n; i++ {
		yi, yj := poly[i].Y, poly[j].Y
		xi, xj := poly[i].X, poly[j].X
		if (yi > y) != (yj > y) {
			slope := (x - xi) * (yj - yi) - (xj - xi) * (y - yi)
			if slope == 0 {
				return true // on edge
			}
			if (slope < 0) != (yj < yi) {
				inside = !inside
			}
		}
		j = i
	}
	return inside
}

// selfIntersects checks whether any pair of non-adjacent polygon edges cross.
func selfIntersects(pts []Point) bool {
	n := len(pts)
	if n < 4 {
		return false // triangle can never self-intersect
	}
	for i := 0; i < n; i++ {
		a, b := pts[i], pts[(i+1)%n]
		for j := i + 2; j < n; j++ {
			if i == 0 && j == n-1 {
				continue // first and last edge share a vertex
			}
			c, d := pts[j], pts[(j+1)%n]
			if segmentsIntersect(a, b, c, d) {
				return true
			}
		}
	}
	return false
}

// segmentsIntersect returns true if line segment (a,b) properly crosses (c,d).
func segmentsIntersect(a, b, c, d Point) bool {
	d1 := cross(c, d, a)
	d2 := cross(c, d, b)
	d3 := cross(a, b, c)
	d4 := cross(a, b, d)

	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) &&
		((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}

	// Check collinear cases.
	if d1 == 0 && onSegment(c, d, a) {
		return true
	}
	if d2 == 0 && onSegment(c, d, b) {
		return true
	}
	if d3 == 0 && onSegment(a, b, c) {
		return true
	}
	if d4 == 0 && onSegment(a, b, d) {
		return true
	}
	return false
}

// cross returns the cross product of vectors (b-a) and (c-a).
func cross(a, b, c Point) float64 {
	return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
}

// onSegment checks if point p lies on segment (a,b), given that a,b,p are collinear.
func onSegment(a, b, p Point) bool {
	return math.Min(a.X, b.X) <= p.X && p.X <= math.Max(a.X, b.X) &&
		math.Min(a.Y, b.Y) <= p.Y && p.Y <= math.Max(a.Y, b.Y)
}
