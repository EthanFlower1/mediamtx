package db

import (
	"database/sql"
	"errors"
	"time"
)

// DefaultGapTolerance is the default maximum gap between two consecutive
// detections of the same class+zone before a new detection event is started.
const DefaultGapTolerance = 30 * time.Second

// DetectionAggregator groups consecutive raw detections into detection events.
// After each detection is stored, call Aggregate to either extend the current
// event or start a new one.
type DetectionAggregator struct {
	DB           *DB
	GapTolerance time.Duration
}

// NewDetectionAggregator creates an aggregator with the given gap tolerance.
// If gapTolerance is zero, DefaultGapTolerance is used.
func NewDetectionAggregator(db *DB, gapTolerance time.Duration) *DetectionAggregator {
	if gapTolerance <= 0 {
		gapTolerance = DefaultGapTolerance
	}
	return &DetectionAggregator{
		DB:           db,
		GapTolerance: gapTolerance,
	}
}

// AggregateInput holds the information needed to aggregate a single detection
// into the detection_events table.
type AggregateInput struct {
	CameraID      string
	ZoneID        string  // may be empty
	Class         string
	Confidence    float64
	FrameTime     string  // RFC3339 / timeFormat
	ThumbnailPath string  // representative thumbnail (optional)
}

// Aggregate processes a single detection and either extends the most recent
// matching detection event or creates a new one. It returns the detection
// event that was created or updated.
func (a *DetectionAggregator) Aggregate(input AggregateInput) (*DetectionEvent, error) {
	frameTime, err := time.Parse(timeFormat, input.FrameTime)
	if err != nil {
		// Try RFC3339 as fallback.
		frameTime, err = time.Parse(time.RFC3339, input.FrameTime)
		if err != nil {
			return nil, err
		}
	}

	latest, err := a.DB.GetLatestDetectionEvent(input.CameraID, input.Class, input.ZoneID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// If there is a recent event whose end_time is within the gap tolerance
	// of this new detection, extend it.
	if latest != nil {
		endTime, parseErr := time.Parse(timeFormat, latest.EndTime)
		if parseErr != nil {
			endTime, parseErr = time.Parse(time.RFC3339, latest.EndTime)
		}
		if parseErr == nil && frameTime.Sub(endTime) <= a.GapTolerance {
			// Extend the existing event.
			latest.EndTime = frameTime.UTC().Format(timeFormat)
			latest.DetectionCount++
			if input.Confidence > latest.PeakConfidence {
				latest.PeakConfidence = input.Confidence
				// Update thumbnail when we find higher confidence.
				if input.ThumbnailPath != "" {
					latest.ThumbnailPath = input.ThumbnailPath
				}
			}
			if err := a.DB.UpdateDetectionEvent(latest); err != nil {
				return nil, err
			}
			return latest, nil
		}
	}

	// Start a new detection event.
	ev := &DetectionEvent{
		CameraID:       input.CameraID,
		ZoneID:         input.ZoneID,
		Class:          input.Class,
		StartTime:      frameTime.UTC().Format(timeFormat),
		EndTime:        frameTime.UTC().Format(timeFormat),
		PeakConfidence: input.Confidence,
		ThumbnailPath:  input.ThumbnailPath,
		DetectionCount: 1,
	}
	if err := a.DB.InsertDetectionEvent(ev); err != nil {
		return nil, err
	}
	return ev, nil
}
