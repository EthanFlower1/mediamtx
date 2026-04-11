// Package anomaly implements per-camera anomaly detection with time-of-day
// baselines. It learns normal activity patterns (object counts by hour-of-day)
// during a configurable learning phase and flags deviations that exceed a
// customer-tunable sensitivity threshold.
//
// This feature is BETA at v1. Edge-only, no cloud dependencies.
//
// KAI-288: Anomaly detection with per-camera baselines
package anomaly
