// Package suppression implements ML-based alert suppression for the
// notification subsystem (KAI-373).
//
// Three suppression strategies reduce alert fatigue:
//
//  1. Event clustering: groups related events by camera + type within a
//     configurable time window into a single summary notification.
//  2. Activity baseline: learns per-camera, per-hour-of-day expected
//     activity levels and suppresses during expected high-activity windows.
//  3. False positive detection: tracks events that are dismissed without
//     action and suppresses recurring patterns.
//
// Each camera has a sensitivity knob (0.0 = no suppression, 1.0 = aggressive).
// Suppressed alerts are recorded in the database and visible in the UI
// with a "(suppressed)" marker.
package suppression
