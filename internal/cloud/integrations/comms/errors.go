package comms

import "errors"

var (
	// ErrNotConfigured is returned when an integration has not been set up.
	ErrNotConfigured = errors.New("comms: integration not configured")

	// ErrInvalidToken is returned when an OAuth token is missing or expired.
	ErrInvalidToken = errors.New("comms: invalid or expired token")

	// ErrChannelNotFound is returned when the target channel cannot be resolved.
	ErrChannelNotFound = errors.New("comms: channel not found")

	// ErrRuleNotFound is returned when no routing rule matches.
	ErrRuleNotFound = errors.New("comms: routing rule not found")

	// ErrUnsupportedAction is returned for unknown action types.
	ErrUnsupportedAction = errors.New("comms: unsupported action type")

	// ErrRateLimited is returned when the platform rejects due to rate limits.
	ErrRateLimited = errors.New("comms: rate limited by platform")

	// ErrSignatureInvalid is returned when a webhook signature check fails.
	ErrSignatureInvalid = errors.New("comms: request signature invalid")
)
