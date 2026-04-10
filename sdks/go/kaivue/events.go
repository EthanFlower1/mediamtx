package kaivue

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// EventService handles event operations.
type EventService struct {
	client *Client
}

// ListEventsRequest is the input for listing events.
type ListEventsRequest struct {
	CameraID      string      `json:"camera_id,omitempty"`
	Kinds         []EventKind `json:"kinds,omitempty"`
	StartTime     *time.Time  `json:"start_time,omitempty"`
	EndTime       *time.Time  `json:"end_time,omitempty"`
	MinConfidence float32     `json:"min_confidence,omitempty"`
	Query         string      `json:"query,omitempty"`
	PageSize      int         `json:"page_size,omitempty"`
	Cursor        string      `json:"cursor,omitempty"`
}

// ListEventsResponse wraps a page of events.
type ListEventsResponse struct {
	Events     []Event `json:"events"`
	NextCursor string  `json:"next_cursor"`
	TotalCount int     `json:"total_count"`
}

// AcknowledgeEventRequest is the input for acknowledging an event.
type AcknowledgeEventRequest struct {
	ID   string `json:"id"`
	Note string `json:"note,omitempty"`
}

// Get retrieves an event by ID.
func (s *EventService) Get(ctx context.Context, id string) (*Event, error) {
	body, err := s.client.get(ctx, "/v1/events/"+id, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Event Event `json:"event"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal event: %w", err)
	}
	return &resp.Event, nil
}

// List lists events with optional filters.
func (s *EventService) List(ctx context.Context, req *ListEventsRequest) (*ListEventsResponse, error) {
	params := url.Values{}
	if req.PageSize > 0 {
		params.Set("page_size", strconv.Itoa(req.PageSize))
	} else {
		params.Set("page_size", "50")
	}
	if req.CameraID != "" {
		params.Set("camera_id", req.CameraID)
	}
	if len(req.Kinds) > 0 {
		kinds := make([]string, len(req.Kinds))
		for i, k := range req.Kinds {
			kinds[i] = string(k)
		}
		params.Set("kinds", strings.Join(kinds, ","))
	}
	if req.StartTime != nil {
		params.Set("start_time", req.StartTime.Format(time.RFC3339))
	}
	if req.EndTime != nil {
		params.Set("end_time", req.EndTime.Format(time.RFC3339))
	}
	if req.MinConfidence > 0 {
		params.Set("min_confidence", fmt.Sprintf("%.2f", req.MinConfidence))
	}
	if req.Query != "" {
		params.Set("query", req.Query)
	}
	if req.Cursor != "" {
		params.Set("cursor", req.Cursor)
	}

	body, err := s.client.get(ctx, "/v1/events", params)
	if err != nil {
		return nil, err
	}
	var resp ListEventsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal events list: %w", err)
	}
	return &resp, nil
}

// Acknowledge acknowledges an event.
func (s *EventService) Acknowledge(ctx context.Context, req *AcknowledgeEventRequest) (*Event, error) {
	body, err := s.client.post(ctx, "/v1/events/"+req.ID+"/acknowledge", req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Event Event `json:"event"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal event: %w", err)
	}
	return &resp.Event, nil
}
