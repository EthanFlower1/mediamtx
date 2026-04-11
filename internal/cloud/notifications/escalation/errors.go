package escalation

import "errors"

var (
	// ErrChainNotFound is returned when an escalation chain does not exist.
	ErrChainNotFound = errors.New("escalation: chain not found")

	// ErrAlertNotFound is returned when an alert escalation record does not exist.
	ErrAlertNotFound = errors.New("escalation: alert not found")

	// ErrAlreadyAcknowledged is returned when an alert has already been acknowledged.
	ErrAlreadyAcknowledged = errors.New("escalation: alert already acknowledged")

	// ErrChainNoSteps is returned when starting an escalation on a chain with no steps.
	ErrChainNoSteps = errors.New("escalation: chain has no steps")

	// ErrInvalidChain is returned when chain validation fails.
	ErrInvalidChain = errors.New("escalation: invalid chain configuration")
)
