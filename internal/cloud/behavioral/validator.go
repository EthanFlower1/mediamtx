package behavioral

import (
	"encoding/json"
	"fmt"
)

// ValidateParams runs per-detector parameter validation against the raw JSON
// params string. Returns ErrInvalidParams (wrapped) on any violation.
//
// Required fields per detector:
//
//	loitering:     roi_polygon ([][2]float64, ≥3 points), threshold_seconds (float64 > 0)
//	line_crossing: line_start ([2]float64), line_end ([2]float64)
//	roi:           roi_polygon ([][2]float64, ≥3 points)
//	crowd_density: max_count (int > 0), roi_polygon (optional [][2]float64)
//	tailgating:    threshold_seconds (float64 > 0)
//	fall_detection: (no required params, but params must be valid JSON object)
func ValidateParams(dt DetectorType, paramsJSON string) error {
	if !dt.IsValid() {
		return fmt.Errorf("%w: unknown detector type %q", ErrInvalidParams, dt)
	}

	if paramsJSON == "" {
		paramsJSON = "{}"
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(paramsJSON), &raw); err != nil {
		return fmt.Errorf("%w: params is not valid JSON: %w", ErrInvalidParams, err)
	}

	switch dt {
	case DetectorLoitering:
		return validateLoitering(raw)
	case DetectorLineCrossing:
		return validateLineCrossing(raw)
	case DetectorROI:
		return validateROI(raw)
	case DetectorCrowdDensity:
		return validateCrowdDensity(raw)
	case DetectorTailgating:
		return validateTailgating(raw)
	case DetectorFallDetection:
		// No required params; any extra fields are allowed.
		return nil
	}
	// Unreachable after IsValid() guard above.
	return nil
}

// -----------------------------------------------------------------------
// per-detector validators
// -----------------------------------------------------------------------

func validateLoitering(p map[string]any) error {
	if err := requirePolygon(p, "roi_polygon", 3); err != nil {
		return err
	}
	return requirePositiveFloat(p, "threshold_seconds")
}

func validateLineCrossing(p map[string]any) error {
	if err := requirePoint(p, "line_start"); err != nil {
		return err
	}
	return requirePoint(p, "line_end")
}

func validateROI(p map[string]any) error {
	return requirePolygon(p, "roi_polygon", 3)
}

func validateCrowdDensity(p map[string]any) error {
	return requirePositiveInt(p, "max_count")
}

func validateTailgating(p map[string]any) error {
	return requirePositiveFloat(p, "threshold_seconds")
}

// -----------------------------------------------------------------------
// primitive validators
// -----------------------------------------------------------------------

// requirePositiveFloat asserts that key maps to a JSON number > 0.
func requirePositiveFloat(p map[string]any, key string) error {
	v, ok := p[key]
	if !ok {
		return fmt.Errorf("%w: %q is required", ErrInvalidParams, key)
	}
	n, ok := v.(float64)
	if !ok {
		return fmt.Errorf("%w: %q must be a number", ErrInvalidParams, key)
	}
	if n <= 0 {
		return fmt.Errorf("%w: %q must be > 0", ErrInvalidParams, key)
	}
	return nil
}

// requirePositiveInt asserts that key maps to a JSON number ≥ 1.
func requirePositiveInt(p map[string]any, key string) error {
	v, ok := p[key]
	if !ok {
		return fmt.Errorf("%w: %q is required", ErrInvalidParams, key)
	}
	n, ok := v.(float64) // JSON numbers always deserialise as float64.
	if !ok {
		return fmt.Errorf("%w: %q must be a number", ErrInvalidParams, key)
	}
	if n < 1 {
		return fmt.Errorf("%w: %q must be >= 1", ErrInvalidParams, key)
	}
	return nil
}

// requirePoint asserts that key maps to a [x, y] two-element JSON array.
func requirePoint(p map[string]any, key string) error {
	v, exists := p[key]
	if !exists {
		return fmt.Errorf("%w: %q is required", ErrInvalidParams, key)
	}
	arr, isArr := v.([]any)
	if !isArr || len(arr) != 2 {
		return fmt.Errorf("%w: %q must be a [x, y] pair", ErrInvalidParams, key)
	}
	for i, c := range arr {
		if _, isNum := c.(float64); !isNum {
			return fmt.Errorf("%w: %q[%d] must be a number", ErrInvalidParams, key, i)
		}
	}
	return nil
}

// requirePolygon asserts that key maps to a JSON array of at least minPoints [x, y] pairs.
func requirePolygon(p map[string]any, key string, minPoints int) error {
	v, exists := p[key]
	if !exists {
		return fmt.Errorf("%w: %q is required", ErrInvalidParams, key)
	}
	arr, isArr := v.([]any)
	if !isArr {
		return fmt.Errorf("%w: %q must be an array of [x, y] pairs", ErrInvalidParams, key)
	}
	if len(arr) < minPoints {
		return fmt.Errorf("%w: %q requires at least %d points, got %d",
			ErrInvalidParams, key, minPoints, len(arr))
	}
	for i, pt := range arr {
		pair, isPair := pt.([]any)
		if !isPair || len(pair) != 2 {
			return fmt.Errorf("%w: %q[%d] must be a [x, y] pair", ErrInvalidParams, key, i)
		}
		for j, c := range pair {
			if _, isNum := c.(float64); !isNum {
				return fmt.Errorf("%w: %q[%d][%d] must be a number", ErrInvalidParams, key, i, j)
			}
		}
	}
	return nil
}
