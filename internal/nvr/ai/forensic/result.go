package forensic

import "time"

// Result represents a single forensic search result — a time-bounded segment
// from a specific camera that matched the query.
type Result struct {
	// ID is a unique identifier for deduplication (detection or event ID).
	ID string `json:"id"`

	// CameraID is the source camera.
	CameraID string `json:"camera_id"`

	// CameraName is the human-readable camera name.
	CameraName string `json:"camera_name"`

	// Timestamp is the frame or event time.
	Timestamp time.Time `json:"timestamp"`

	// MatchedClasses lists which object classes were detected.
	MatchedClasses []string `json:"matched_classes,omitempty"`

	// MatchedPlate contains the plate text if an LPR clause matched.
	MatchedPlate string `json:"matched_plate,omitempty"`

	// Score is the composite relevance score (0-1).
	Score float64 `json:"score"`

	// CLIPSimilarity is the CLIP cosine similarity if a clip clause was used.
	CLIPSimilarity float64 `json:"clip_similarity,omitempty"`

	// Confidence is the detection confidence.
	Confidence float64 `json:"confidence"`

	// ThumbnailPath is the path to the event/detection thumbnail.
	ThumbnailPath string `json:"thumbnail_path,omitempty"`

	// SnippetStart and SnippetEnd define a short video clip around the match.
	SnippetStart *time.Time `json:"snippet_start,omitempty"`
	SnippetEnd   *time.Time `json:"snippet_end,omitempty"`
}

// ResultSet is the response from a forensic search execution.
type ResultSet struct {
	// Query is the human-readable form of the executed query.
	Query string `json:"query"`

	// TotalMatches is the total number of matches before limit/offset.
	TotalMatches int `json:"total_matches"`

	// Results is the page of results.
	Results []Result `json:"results"`

	// ExecutionTimeMs is how long the search took.
	ExecutionTimeMs int64 `json:"execution_time_ms"`
}
