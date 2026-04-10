// Package timeline assembles unified playback timelines from segment indexes
// across multiple Recorders. This enables the playback UI to show a continuous
// timeline even when a camera was recorded by different Recorders.
package timeline

import "time"

// Segment represents a contiguous recording range from a single Recorder.
type Segment struct {
	CameraID   string    `json:"camera_id"`
	RecorderID string    `json:"recorder_id"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
}

// MergedSegment is a segment in the unified timeline. It may span recordings
// from multiple Recorders (when overlapping), or a single Recorder.
type MergedSegment struct {
	CameraID    string    `json:"camera_id"`
	RecorderIDs []string  `json:"recorder_ids"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
}

// Gap represents a time range where no Recorder had footage for a camera.
type Gap struct {
	CameraID string    `json:"camera_id"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
}

// TimelineRequest specifies the cameras and time range to query.
type TimelineRequest struct {
	CameraIDs []string  `json:"camera_ids"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
}

// TimelineResponse contains the assembled timeline for the requested range.
type TimelineResponse struct {
	Segments []MergedSegment `json:"segments"`
	Gaps     []Gap           `json:"gaps"`
}

// SegmentStore is the interface for fetching raw segments. The real
// implementation comes from the Directory's segment_index table (KAI-144).
type SegmentStore interface {
	// QuerySegments returns all segments for the given cameras within [start, end].
	QuerySegments(cameraIDs []string, start, end time.Time) ([]Segment, error)
}
