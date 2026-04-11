package pdk

import "errors"

var (
	// ErrConfigNotFound is returned when no PDK integration config exists for the tenant.
	ErrConfigNotFound = errors.New("pdk: integration config not found")

	// ErrDoorNotFound is returned when a door lookup fails.
	ErrDoorNotFound = errors.New("pdk: door not found")

	// ErrEventNotFound is returned when an event lookup fails.
	ErrEventNotFound = errors.New("pdk: event not found")

	// ErrMappingNotFound is returned when a door-camera mapping lookup fails.
	ErrMappingNotFound = errors.New("pdk: door-camera mapping not found")

	// ErrInvalidWebhookSignature is returned when a webhook signature check fails.
	ErrInvalidWebhookSignature = errors.New("pdk: invalid webhook signature")

	// ErrAPIAuth is returned when PDK API authentication fails.
	ErrAPIAuth = errors.New("pdk: API authentication failed")

	// ErrAPIRequest is returned when a PDK API request fails.
	ErrAPIRequest = errors.New("pdk: API request failed")

	// ErrIntegrationDisabled is returned when the tenant's PDK integration is disabled.
	ErrIntegrationDisabled = errors.New("pdk: integration is disabled")
)
