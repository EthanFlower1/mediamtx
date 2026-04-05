package ai

import (
	"encoding/json"
	"log"
)

// DefaultConfidenceThresholds maps common object classes to their minimum
// confidence scores. Detections below these thresholds are filtered out
// before storage and notification. Classes not present in the map use the
// pipeline's global ConfidenceThresh value as a fallback.
var DefaultConfidenceThresholds = map[string]float32{
	"person":     0.5,
	"car":        0.4,
	"truck":      0.4,
	"bus":        0.4,
	"motorcycle": 0.4,
	"bicycle":    0.4,
	"boat":       0.4,
	"cat":        0.3,
	"dog":        0.3,
	"horse":      0.3,
	"sheep":      0.3,
	"cow":        0.3,
	"elephant":   0.3,
	"bear":       0.3,
	"zebra":      0.3,
	"giraffe":    0.3,
}

// ClassThresholds holds parsed per-class confidence thresholds and a global
// fallback value. It is constructed once at pipeline creation time and used
// to filter detections in the hot path without repeated JSON parsing.
type ClassThresholds struct {
	perClass    map[string]float32
	globalFallback float32
}

// NewClassThresholds merges DefaultConfidenceThresholds with per-camera
// overrides provided as a JSON string (e.g. {"person":0.6,"car":0.35}).
// globalThresh is used for classes that appear in neither the defaults nor
// the overrides.
func NewClassThresholds(jsonStr string, globalThresh float32) *ClassThresholds {
	merged := make(map[string]float32, len(DefaultConfidenceThresholds))
	for k, v := range DefaultConfidenceThresholds {
		merged[k] = v
	}
	if jsonStr != "" {
		var overrides map[string]float64
		if err := json.Unmarshal([]byte(jsonStr), &overrides); err != nil {
			log.Printf("[ai] invalid confidence_thresholds JSON, using defaults: %v", err)
		} else {
			for k, v := range overrides {
				merged[k] = float32(v)
			}
		}
	}
	return &ClassThresholds{
		perClass:       merged,
		globalFallback: globalThresh,
	}
}

// FilterDetections removes detections that fall below the per-class confidence
// thresholds. Classes not in the per-class map use the global fallback.
func (ct *ClassThresholds) FilterDetections(dets []Detection) []Detection {
	if len(dets) == 0 {
		return dets
	}
	filtered := make([]Detection, 0, len(dets))
	for _, d := range dets {
		minConf := ct.globalFallback
		if t, ok := ct.perClass[d.Class]; ok {
			minConf = t
		}
		if d.Confidence >= minConf {
			filtered = append(filtered, d)
		}
	}
	return filtered
}
