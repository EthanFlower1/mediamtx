package kaivue

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// RetentionService handles retention policy CRUD operations.
type RetentionService struct {
	client *Client
}

// CreateRetentionPolicyRequest is the input for creating a retention policy.
type CreateRetentionPolicyRequest struct {
	Name               string `json:"name"`
	Description        string `json:"description,omitempty"`
	RetentionDays      int    `json:"retention_days"`
	MaxBytes           int64  `json:"max_bytes,omitempty"`
	EventRetentionDays int    `json:"event_retention_days,omitempty"`
}

// UpdateRetentionPolicyRequest is the input for updating a retention policy.
type UpdateRetentionPolicyRequest struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name,omitempty"`
	Description        string   `json:"description,omitempty"`
	RetentionDays      *int     `json:"retention_days,omitempty"`
	MaxBytes           *int64   `json:"max_bytes,omitempty"`
	EventRetentionDays *int     `json:"event_retention_days,omitempty"`
	UpdateMask         []string `json:"update_mask"`
}

// ListRetentionPoliciesRequest is the input for listing retention policies.
type ListRetentionPoliciesRequest struct {
	PageSize int    `json:"page_size,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
}

// ListRetentionPoliciesResponse wraps a page of retention policies.
type ListRetentionPoliciesResponse struct {
	Policies   []RetentionPolicy `json:"policies"`
	NextCursor string            `json:"next_cursor"`
}

// ApplyRetentionPolicyRequest applies a policy to cameras.
type ApplyRetentionPolicyRequest struct {
	PolicyID  string   `json:"policy_id"`
	CameraIDs []string `json:"camera_ids"`
}

// Create creates a new retention policy.
func (s *RetentionService) Create(ctx context.Context, req *CreateRetentionPolicyRequest) (*RetentionPolicy, error) {
	body, err := s.client.post(ctx, "/v1/retention-policies", req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Policy RetentionPolicy `json:"policy"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal retention policy: %w", err)
	}
	return &resp.Policy, nil
}

// Get retrieves a retention policy by ID.
func (s *RetentionService) Get(ctx context.Context, id string) (*RetentionPolicy, error) {
	body, err := s.client.get(ctx, "/v1/retention-policies/"+id, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Policy RetentionPolicy `json:"policy"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal retention policy: %w", err)
	}
	return &resp.Policy, nil
}

// Update updates a retention policy.
func (s *RetentionService) Update(ctx context.Context, req *UpdateRetentionPolicyRequest) (*RetentionPolicy, error) {
	body, err := s.client.patch(ctx, "/v1/retention-policies/"+req.ID, req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Policy RetentionPolicy `json:"policy"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal retention policy: %w", err)
	}
	return &resp.Policy, nil
}

// Delete deletes a retention policy.
func (s *RetentionService) Delete(ctx context.Context, id string) error {
	_, err := s.client.delete(ctx, "/v1/retention-policies/"+id, nil)
	return err
}

// List lists retention policies.
func (s *RetentionService) List(ctx context.Context, req *ListRetentionPoliciesRequest) (*ListRetentionPoliciesResponse, error) {
	params := url.Values{}
	if req.PageSize > 0 {
		params.Set("page_size", strconv.Itoa(req.PageSize))
	} else {
		params.Set("page_size", "50")
	}
	if req.Cursor != "" {
		params.Set("cursor", req.Cursor)
	}

	body, err := s.client.get(ctx, "/v1/retention-policies", params)
	if err != nil {
		return nil, err
	}
	var resp ListRetentionPoliciesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal retention policies list: %w", err)
	}
	return &resp, nil
}

// Apply applies a retention policy to one or more cameras.
func (s *RetentionService) Apply(ctx context.Context, req *ApplyRetentionPolicyRequest) (*RetentionPolicy, error) {
	body, err := s.client.post(ctx, "/v1/retention-policies/"+req.PolicyID+"/apply", req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Policy RetentionPolicy `json:"policy"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal retention policy: %w", err)
	}
	return &resp.Policy, nil
}
