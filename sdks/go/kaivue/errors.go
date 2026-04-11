package kaivue

import (
	"encoding/json"
	"fmt"
)

// APIError represents an error returned by the KaiVue API.
type APIError struct {
	StatusCode  int          `json:"-"`
	Message     string       `json:"message"`
	RequestID   string       `json:"request_id,omitempty"`
	FieldErrors []FieldError `json:"field_errors,omitempty"`
}

// FieldError is a single field-level validation error.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	s := fmt.Sprintf("[%d] %s", e.StatusCode, e.Message)
	if e.RequestID != "" {
		s += fmt.Sprintf(" (request_id=%s)", e.RequestID)
	}
	return s
}

// IsNotFound returns true if the error is a 404.
func IsNotFound(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == 404
	}
	return false
}

// IsAuthError returns true if the error is a 401 or 403.
func IsAuthError(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == 401 || apiErr.StatusCode == 403
	}
	return false
}

// IsValidationError returns true if the error is a 400 or 422.
func IsValidationError(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == 400 || apiErr.StatusCode == 422
	}
	return false
}

// IsRateLimited returns true if the error is a 429.
func IsRateLimited(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == 429
	}
	return false
}

func parseAPIError(statusCode int, body []byte, requestID string) *APIError {
	apiErr := &APIError{
		StatusCode: statusCode,
		RequestID:  requestID,
	}
	if err := json.Unmarshal(body, apiErr); err != nil {
		apiErr.Message = string(body)
	}
	if apiErr.RequestID == "" {
		apiErr.RequestID = requestID
	}
	return apiErr
}
