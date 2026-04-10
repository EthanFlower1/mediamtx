package notifications

import "errors"

var (
	ErrChannelNotFound    = errors.New("notifications: channel not found")
	ErrPreferenceNotFound = errors.New("notifications: preference not found")
	ErrInvalidChannelType = errors.New("notifications: invalid channel type")
)
