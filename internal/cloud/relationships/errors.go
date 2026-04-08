package relationships

import "errors"

// Sentinel errors. Callers use errors.Is to distinguish them.
var (
	// ErrNotFound is returned when the requested relationship does not exist.
	ErrNotFound = errors.New("relationships: not found")

	// ErrAlreadyExists is returned when trying to create a relationship that
	// already exists (regardless of its current status).
	ErrAlreadyExists = errors.New("relationships: relationship already exists")

	// ErrInvalidTransition is returned when the caller attempts a status
	// transition that the state machine does not permit.
	ErrInvalidTransition = errors.New("relationships: invalid state transition")

	// ErrPermissionDenied is returned when the caller lacks the required
	// Casbin action for the operation. Fail-closed.
	ErrPermissionDenied = errors.New("relationships: permission denied")

	// ErrInvalidSpec is returned when a CreateSpec or UpdateSpec is
	// structurally invalid (e.g. empty integrator ID, unknown role template).
	ErrInvalidSpec = errors.New("relationships: invalid spec")

	// ErrScopeEscalation is returned when a sub-reseller would gain a
	// permission the parent does not possess. Children narrow, never broaden.
	ErrScopeEscalation = errors.New("relationships: sub-reseller cannot escalate beyond parent scope")
)
