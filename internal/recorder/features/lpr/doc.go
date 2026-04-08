// Package lpr implements license plate recognition (LPR/ANPR) as an
// edge-first AI feature for the Kaivue NVR.
//
// Architecture
//
// The package is structured around three concerns:
//
//  1. Plate detection — a YOLO-style localization model produces bounding boxes
//     around candidate plate regions. Implemented via the inference.Inferencer
//     abstraction (KAI-278); never calls ONNX Runtime directly.
//
//  2. Plate reading — a character recognition model (CRNN) reads the character
//     sequence from the cropped plate region. Also via inference.Inferencer.
//
//  3. Regional format validation — a lookup table of compiled regexes covers
//     ~20 common regional formats (US per-state, EU ISO country, UK, AU).
//     Reads that fail all patterns are still stored with region="unknown".
//
// Triggering
//
// The Pipeline subscribes to object-detection events from KAI-281's
// objectdetection.DetectionEventSink. When a vehicle class is detected,
// the Pipeline crops the relevant frame region and runs plate detection +
// reading. This avoids running LPR on every frame.
//
// Watchlists
//
// Per-tenant allow/deny/alert watchlists live in the watchlist sub-package.
// Hot-path matching uses an in-memory bloom filter to avoid DB round-trips
// for the common "not in any watchlist" case.
//
// Read Logging
//
// Every read is stored via an LPREventPublisher interface. The production
// implementation (wiring to KAI-254's DirectoryIngest.PublishAIEvents) is
// stubbed behind that interface with a clear TODO. A logging stub is
// provided for tests and dev.
//
// EU AI Act
//
// LPR is NOT classified as high-risk under the EU AI Act (that designation
// applies to face recognition — KAI-282). However all reads and watchlist
// entries respect the per-watchlist retention_days field (default 90 days).
//
// Package boundary
//
// This package MUST NOT import internal/directory/ or internal/recorder/.
// It MAY import internal/shared/* and internal/recorder/features/objectdetection
// (for the Detection type used by the vehicle event subscription).
package lpr
