package tenants

import "errors"

// Sentinel errors returned by Service. Callers should prefer errors.Is.
var (
	// ErrIntegratorExists is returned when the supplied display name (or
	// explicit ID) collides with an existing integrator row.
	ErrIntegratorExists = errors.New("tenants: integrator already exists")

	// ErrCustomerTenantExists mirrors ErrIntegratorExists for customer
	// tenants.
	ErrCustomerTenantExists = errors.New("tenants: customer tenant already exists")

	// ErrMaxDepthExceeded is returned by CreateSubReseller when creating
	// the child would push the integrator chain past MaxSubResellerDepth.
	ErrMaxDepthExceeded = errors.New("tenants: sub-reseller depth cap exceeded")

	// ErrParentIntegratorNotFound is returned by CreateSubReseller when
	// the parent id does not resolve inside the caller's region.
	ErrParentIntegratorNotFound = errors.New("tenants: parent integrator not found")

	// ErrMissingHomeIntegrator is returned when a customer tenant with
	// billing_mode=via_integrator is created without a HomeIntegratorID.
	ErrMissingHomeIntegrator = errors.New("tenants: via_integrator billing requires home_integrator_id")

	// ErrPermissionDenied is returned when the caller's subject does not
	// carry the required action on the platform tenant. Fail-closed.
	ErrPermissionDenied = errors.New("tenants: permission denied")

	// ErrInvalidSpec is returned for structurally invalid Create*Spec
	// inputs (empty display name, unknown billing mode, etc.).
	ErrInvalidSpec = errors.New("tenants: invalid spec")
)
