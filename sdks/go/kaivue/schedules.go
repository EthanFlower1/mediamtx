package kaivue

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// ScheduleService handles schedule CRUD operations.
type ScheduleService struct {
	client *Client
}

// CreateScheduleRequest is the input for creating a schedule.
type CreateScheduleRequest struct {
	CameraID string          `json:"camera_id"`
	Name     string          `json:"name"`
	Timezone string          `json:"timezone"`
	Entries  []ScheduleEntry `json:"entries"`
}

// UpdateScheduleRequest is the input for updating a schedule.
type UpdateScheduleRequest struct {
	ID         string          `json:"id"`
	Name       string          `json:"name,omitempty"`
	Timezone   string          `json:"timezone,omitempty"`
	Entries    []ScheduleEntry `json:"entries,omitempty"`
	UpdateMask []string        `json:"update_mask"`
}

// ListSchedulesRequest is the input for listing schedules.
type ListSchedulesRequest struct {
	CameraID string `json:"camera_id,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
}

// ListSchedulesResponse wraps a page of schedules.
type ListSchedulesResponse struct {
	Schedules  []Schedule `json:"schedules"`
	NextCursor string     `json:"next_cursor"`
}

// Create creates a new schedule.
func (s *ScheduleService) Create(ctx context.Context, req *CreateScheduleRequest) (*Schedule, error) {
	body, err := s.client.post(ctx, "/v1/schedules", req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Schedule Schedule `json:"schedule"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal schedule: %w", err)
	}
	return &resp.Schedule, nil
}

// Get retrieves a schedule by ID.
func (s *ScheduleService) Get(ctx context.Context, id string) (*Schedule, error) {
	body, err := s.client.get(ctx, "/v1/schedules/"+id, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Schedule Schedule `json:"schedule"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal schedule: %w", err)
	}
	return &resp.Schedule, nil
}

// Update updates a schedule.
func (s *ScheduleService) Update(ctx context.Context, req *UpdateScheduleRequest) (*Schedule, error) {
	body, err := s.client.patch(ctx, "/v1/schedules/"+req.ID, req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Schedule Schedule `json:"schedule"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal schedule: %w", err)
	}
	return &resp.Schedule, nil
}

// Delete deletes a schedule.
func (s *ScheduleService) Delete(ctx context.Context, id string) error {
	_, err := s.client.delete(ctx, "/v1/schedules/"+id, nil)
	return err
}

// List lists schedules with optional camera filter.
func (s *ScheduleService) List(ctx context.Context, req *ListSchedulesRequest) (*ListSchedulesResponse, error) {
	params := url.Values{}
	if req.PageSize > 0 {
		params.Set("page_size", strconv.Itoa(req.PageSize))
	} else {
		params.Set("page_size", "50")
	}
	if req.CameraID != "" {
		params.Set("camera_id", req.CameraID)
	}
	if req.Cursor != "" {
		params.Set("cursor", req.Cursor)
	}

	body, err := s.client.get(ctx, "/v1/schedules", params)
	if err != nil {
		return nil, err
	}
	var resp ListSchedulesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal schedules list: %w", err)
	}
	return &resp, nil
}
