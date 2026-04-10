// Package preferences implements per-user, per-camera notification
// preferences with quiet-hours suppression and severity thresholds
// (KAI-371).
//
// Each preference entry scopes to a (tenant, user, camera, event_type)
// tuple where camera and event_type may be empty strings meaning "all".
// Resolution follows most-specific-wins: camera+event > camera-only >
// event-only > default.
package preferences
