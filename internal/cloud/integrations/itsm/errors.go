package itsm

import "errors"

var (
	// ErrInvalidConfig is returned when a provider configuration is incomplete or invalid.
	ErrInvalidConfig = errors.New("itsm: invalid configuration")

	// ErrProviderNotFound is returned when a referenced provider does not exist.
	ErrProviderNotFound = errors.New("itsm: provider not found")

	// ErrRuleNotFound is returned when a routing rule cannot be located.
	ErrRuleNotFound = errors.New("itsm: routing rule not found")

	// ErrAlertFailed is returned when an alert delivery attempt fails.
	ErrAlertFailed = errors.New("itsm: alert delivery failed")

	// ErrRateLimited is returned when the upstream API rate-limits the request.
	ErrRateLimited = errors.New("itsm: rate limited by provider")

	// ErrUnsupportedProvider is returned for unknown provider types.
	ErrUnsupportedProvider = errors.New("itsm: unsupported provider")
)
