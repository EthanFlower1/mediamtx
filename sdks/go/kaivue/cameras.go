package kaivue

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// CameraService handles camera CRUD operations.
type CameraService struct {
	client *Client
}

// CreateCameraRequest is the input for creating a camera.
type CreateCameraRequest struct {
	Name          string        `json:"name"`
	Description   string        `json:"description,omitempty"`
	IPAddress     string        `json:"ip_address"`
	RecorderID    string        `json:"recorder_id"`
	RecordingMode RecordingMode `json:"recording_mode,omitempty"`
	Labels        []string      `json:"labels,omitempty"`
	Username      string        `json:"username,omitempty"`
	Password      string        `json:"password,omitempty"`
}

// ListCamerasRequest is the input for listing cameras.
type ListCamerasRequest struct {
	Search      string      `json:"search,omitempty"`
	RecorderID  string      `json:"recorder_id,omitempty"`
	StateFilter CameraState `json:"state_filter,omitempty"`
	PageSize    int         `json:"page_size,omitempty"`
	Cursor      string      `json:"cursor,omitempty"`
}

// ListCamerasResponse wraps a page of cameras.
type ListCamerasResponse struct {
	Cameras    []Camera `json:"cameras"`
	NextCursor string   `json:"next_cursor"`
	TotalCount int      `json:"total_count"`
}

// UpdateCameraRequest is the input for updating a camera.
type UpdateCameraRequest struct {
	ID                string        `json:"id"`
	Name              string        `json:"name,omitempty"`
	Description       string        `json:"description,omitempty"`
	RecordingMode     RecordingMode `json:"recording_mode,omitempty"`
	Labels            []string      `json:"labels,omitempty"`
	AudioEnabled      *bool         `json:"audio_enabled,omitempty"`
	MotionSensitivity *int          `json:"motion_sensitivity,omitempty"`
	UpdateMask        []string      `json:"update_mask"`
}

// Create creates a new camera.
func (s *CameraService) Create(ctx context.Context, req *CreateCameraRequest) (*Camera, error) {
	body, err := s.client.post(ctx, "/v1/cameras", req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Camera Camera `json:"camera"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal camera: %w", err)
	}
	return &resp.Camera, nil
}

// Get retrieves a camera by ID.
func (s *CameraService) Get(ctx context.Context, id string) (*Camera, error) {
	body, err := s.client.get(ctx, "/v1/cameras/"+id, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Camera Camera `json:"camera"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal camera: %w", err)
	}
	return &resp.Camera, nil
}

// Update updates a camera.
func (s *CameraService) Update(ctx context.Context, req *UpdateCameraRequest) (*Camera, error) {
	body, err := s.client.patch(ctx, "/v1/cameras/"+req.ID, req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Camera Camera `json:"camera"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal camera: %w", err)
	}
	return &resp.Camera, nil
}

// Delete deletes a camera.
func (s *CameraService) Delete(ctx context.Context, id string, purgeRecordings bool) error {
	params := url.Values{}
	if purgeRecordings {
		params.Set("purge_recordings", "true")
	}
	_, err := s.client.delete(ctx, "/v1/cameras/"+id, params)
	return err
}

// List lists cameras with optional filters.
func (s *CameraService) List(ctx context.Context, req *ListCamerasRequest) (*ListCamerasResponse, error) {
	params := url.Values{}
	if req.PageSize > 0 {
		params.Set("page_size", strconv.Itoa(req.PageSize))
	} else {
		params.Set("page_size", "50")
	}
	if req.Search != "" {
		params.Set("search", req.Search)
	}
	if req.RecorderID != "" {
		params.Set("recorder_id", req.RecorderID)
	}
	if req.StateFilter != "" {
		params.Set("state_filter", string(req.StateFilter))
	}
	if req.Cursor != "" {
		params.Set("cursor", req.Cursor)
	}

	body, err := s.client.get(ctx, "/v1/cameras", params)
	if err != nil {
		return nil, err
	}
	var resp ListCamerasResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal cameras list: %w", err)
	}
	return &resp, nil
}

// ListAll auto-paginates through all cameras matching the filter.
func (s *CameraService) ListAll(ctx context.Context, req *ListCamerasRequest) ([]Camera, error) {
	var all []Camera
	cursor := req.Cursor
	for {
		r := *req
		r.Cursor = cursor
		resp, err := s.List(ctx, &r)
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Cameras...)
		if resp.NextCursor == "" || len(resp.Cameras) == 0 {
			break
		}
		cursor = resp.NextCursor
	}
	return all, nil
}
