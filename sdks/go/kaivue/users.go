package kaivue

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// UserService handles user CRUD operations.
type UserService struct {
	client *Client
}

// CreateUserRequest is the input for creating a user.
type CreateUserRequest struct {
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	Password    string   `json:"password"`
	DisplayName string   `json:"display_name,omitempty"`
	Groups      []string `json:"groups,omitempty"`
}

// UpdateUserRequest is the input for updating a user.
type UpdateUserRequest struct {
	ID          string   `json:"id"`
	Email       string   `json:"email,omitempty"`
	DisplayName string   `json:"display_name,omitempty"`
	Groups      []string `json:"groups,omitempty"`
	Disabled    *bool    `json:"disabled,omitempty"`
	UpdateMask  []string `json:"update_mask"`
}

// ListUsersRequest is the input for listing users.
type ListUsersRequest struct {
	Search   string `json:"search,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
}

// ListUsersResponse wraps a page of users.
type ListUsersResponse struct {
	Users      []User `json:"users"`
	NextCursor string `json:"next_cursor"`
	TotalCount int    `json:"total_count"`
}

// Create creates a new user.
func (s *UserService) Create(ctx context.Context, req *CreateUserRequest) (*User, error) {
	body, err := s.client.post(ctx, "/v1/users", req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		User User `json:"user"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal user: %w", err)
	}
	return &resp.User, nil
}

// Get retrieves a user by ID.
func (s *UserService) Get(ctx context.Context, id string) (*User, error) {
	body, err := s.client.get(ctx, "/v1/users/"+id, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		User User `json:"user"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal user: %w", err)
	}
	return &resp.User, nil
}

// Update updates a user.
func (s *UserService) Update(ctx context.Context, req *UpdateUserRequest) (*User, error) {
	body, err := s.client.patch(ctx, "/v1/users/"+req.ID, req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		User User `json:"user"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal user: %w", err)
	}
	return &resp.User, nil
}

// Delete deletes a user.
func (s *UserService) Delete(ctx context.Context, id string) error {
	_, err := s.client.delete(ctx, "/v1/users/"+id, nil)
	return err
}

// List lists users with optional search.
func (s *UserService) List(ctx context.Context, req *ListUsersRequest) (*ListUsersResponse, error) {
	params := url.Values{}
	if req.PageSize > 0 {
		params.Set("page_size", strconv.Itoa(req.PageSize))
	} else {
		params.Set("page_size", "50")
	}
	if req.Search != "" {
		params.Set("search", req.Search)
	}
	if req.Cursor != "" {
		params.Set("cursor", req.Cursor)
	}

	body, err := s.client.get(ctx, "/v1/users", params)
	if err != nil {
		return nil, err
	}
	var resp ListUsersResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kaivue: unmarshal users list: %w", err)
	}
	return &resp, nil
}
