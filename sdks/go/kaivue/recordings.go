package kaivue

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// RecordingService handles recording operations.
type RecordingService struct {
	client *Client
}

// ListRecordingsRequest is the input for listing recordings.
type ListRecordingsRequest struct {
	CameraID       string     `json:"camera_id,omitempty"`
	StartTime      *time.Time `json:"start_time,omitempty"`
	EndTime        *time.Time `json:"end_time,omitempty"`
	EventClipsOnly bool       `json:"event_clips_only,omitempty"`
	PageSize       int        `json:"page_size,omitempty"`
	Cursor         string     `json:"cursor,omitempty"`
}

// ListRecordingsResponse wraps a page of recordings.
type ListRecordingsResponse struct {
	Recordings []Recording `json:"recordings"`
	NextCursor string      `json:"next_cursor"`
	TotalCount int         `json:"total_count"`
}

// ExportRecordingRequest is the input for exporting a recording clip.
type ExportRecordingRequest struct {
	CameraID  string    `json:"camera_id"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Format    string    `json:"format,omitempty"`
}

// ExportRecordingResponse contains the download URL.
type ExportRecordingResponse struct {
	DownloadURL string     `json:"download_url"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// Get retrieves a recording by ID.
func (s *RecordingService) Get(ctx context.Context, id string) (*Recording, error) {
	body, err := s.client.get(ctx, "/v1/recordings/"+id, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Recording Recording `json:"recording"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal recording: %w", err)
	}
	return &resp.Recording, nil
}

// List lists recordings with optional filters.
func (s *RecordingService) List(ctx context.Context, req *ListRecordingsRequest) (*ListRecordingsResponse, error) {
	params := url.Values{}
	if req.PageSize > 0 {
		params.Set("page_size", strconv.Itoa(req.PageSize))
	} else {
		params.Set("page_size", "50")
	}
	if req.CameraID != "" {
		params.Set("camera_id", req.CameraID)
	}
	if req.StartTime != nil {
		params.Set("start_time", req.StartTime.Format(time.RFC3339))
	}
	if req.EndTime != nil {
		params.Set("end_time", req.EndTime.Format(time.RFC3339))
	}
	if req.EventClipsOnly {
		params.Set("event_clips_only", "true")
	}
	if req.Cursor != "" {
		params.Set("cursor", req.Cursor)
	}

	body, err := s.client.get(ctx, "/v1/recordings", params)
	if err != nil {
		return nil, err
	}
	var resp ListRecordingsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal recordings list: %w", err)
	}
	return &resp, nil
}

// Delete deletes a recording by ID.
func (s *RecordingService) Delete(ctx context.Context, id string) error {
	_, err := s.client.delete(ctx, "/v1/recordings/"+id, nil)
	return err
}

// Export exports a recording clip and returns a download URL.
func (s *RecordingService) Export(ctx context.Context, req *ExportRecordingRequest) (*ExportRecordingResponse, error) {
	body, err := s.client.post(ctx, "/v1/recordings/export", req)
	if err != nil {
		return nil, err
	}
	var resp ExportRecordingResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal export response: %w", err)
	}
	return &resp, nil
}
