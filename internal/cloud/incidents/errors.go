package incidents

import "errors"

var (
	// ErrIncidentNotFound is returned when an incident does not exist.
	ErrIncidentNotFound = errors.New("incidents: incident not found")

	// ErrRunbookNotFound is returned when a runbook mapping does not exist.
	ErrRunbookNotFound = errors.New("incidents: runbook not found")

	// ErrPostMortemNotFound is returned when a post-mortem does not exist.
	ErrPostMortemNotFound = errors.New("incidents: post-mortem not found")

	// ErrOnCallNotFound is returned when an on-call schedule does not exist.
	ErrOnCallNotFound = errors.New("incidents: on-call schedule not found")

	// ErrPagerDutyAPI is returned when the PagerDuty API returns an error.
	ErrPagerDutyAPI = errors.New("incidents: pagerduty API error")
)
