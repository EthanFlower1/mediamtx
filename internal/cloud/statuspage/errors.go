package statuspage

import "errors"

var (
	ErrCheckNotFound    = errors.New("statuspage: health check not found")
	ErrIncidentNotFound = errors.New("statuspage: incident not found")
)
