package kaivue

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// IntegrationService handles integration CRUD operations.
type IntegrationService struct {
	client *Client
}

// CreateIntegrationRequest is the input for creating an integration.
type CreateIntegrationRequest struct {
	Name             string            `json:"name"`
	Kind             IntegrationKind   `json:"kind"`
	Config           map[string]string `json:"config,omitempty"`
	SubscribedEvents []EventKind       `json:"subscribed_events,omitempty"`
	CameraIDs        []string          `json:"camera_ids,omitempty"`
}

// UpdateIntegrationRequest is the input for updating an integration.
type UpdateIntegrationRequest struct {
	ID               string            `json:"id"`
	Name             string            `json:"name,omitempty"`
	Enabled          *bool             `json:"enabled,omitempty"`
	Config           map[string]string `json:"config,omitempty"`
	SubscribedEvents []EventKind       `json:"subscribed_events,omitempty"`
	CameraIDs        []string          `json:"camera_ids,omitempty"`
	UpdateMask       []string          `json:"update_mask"`
}

// ListIntegrationsRequest is the input for listing integrations.
type ListIntegrationsRequest struct {
	KindFilter IntegrationKind `json:"kind_filter,omitempty"`
	PageSize   int             `json:"page_size,omitempty"`
	Cursor     string          `json:"cursor,omitempty"`
}

// ListIntegrationsResponse wraps a page of integrations.
type ListIntegrationsResponse struct {
	Integrations []Integration `json:"integrations"`
	NextCursor   string        `json:"next_cursor"`
}

// TestIntegrationResponse contains the result of a connectivity test.
type TestIntegrationResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	LatencyMs int64  `json:"latency_ms"`
}

// Create creates a new integration.
func (s *IntegrationService) Create(ctx context.Context, req *CreateIntegrationRequest) (*Integration, error) {
	body, err := s.client.post(ctx, "/v1/integrations", req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Integration Integration `json:"integration"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal integration: %w", err)
	}
	return &resp.Integration, nil
}

// Get retrieves an integration by ID.
func (s *IntegrationService) Get(ctx context.Context, id string) (*Integration, error) {
	body, err := s.client.get(ctx, "/v1/integrations/"+id, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Integration Integration `json:"integration"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal integration: %w", err)
	}
	return &resp.Integration, nil
}

// Update updates an integration.
func (s *IntegrationService) Update(ctx context.Context, req *UpdateIntegrationRequest) (*Integration, error) {
	body, err := s.client.patch(ctx, "/v1/integrations/"+req.ID, req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Integration Integration `json:"integration"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal integration: %w", err)
	}
	return &resp.Integration, nil
}

// Delete deletes an integration.
func (s *IntegrationService) Delete(ctx context.Context, id string) error {
	_, err := s.client.delete(ctx, "/v1/integrations/"+id, nil)
	return err
}

// List lists integrations with optional kind filter.
func (s *IntegrationService) List(ctx context.Context, req *ListIntegrationsRequest) (*ListIntegrationsResponse, error) {
	params := url.Values{}
	if req.PageSize > 0 {
		params.Set("page_size", strconv.Itoa(req.PageSize))
	} else {
		params.Set("page_size", "50")
	}
	if req.KindFilter != "" {
		params.Set("kind_filter", string(req.KindFilter))
	}
	if req.Cursor != "" {
		params.Set("cursor", req.Cursor)
	}

	body, err := s.client.get(ctx, "/v1/integrations", params)
	if err != nil {
		return nil, err
	}
	var resp ListIntegrationsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal integrations list: %w", err)
	}
	return &resp, nil
}

// Test tests an integration connectivity.
func (s *IntegrationService) Test(ctx context.Context, id string) (*TestIntegrationResponse, error) {
	body, err := s.client.post(ctx, "/v1/integrations/"+id+"/test", nil)
	if err != nil {
		return nil, err
	}
	var resp TestIntegrationResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal test response: %w", err)
	}
	return &resp, nil
}
