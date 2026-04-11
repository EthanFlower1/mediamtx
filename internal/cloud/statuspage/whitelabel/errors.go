package whitelabel

import "errors"

var (
	// ErrConfigNotFound is returned when no status page config exists for the
	// integrator or subdomain.
	ErrConfigNotFound = errors.New("whitelabel-status: config not found")

	// ErrSubdomainTaken is returned when the requested subdomain is already
	// registered to another integrator.
	ErrSubdomainTaken = errors.New("whitelabel-status: subdomain already taken")

	// ErrSubscriberExists is returned when the email is already subscribed.
	ErrSubscriberExists = errors.New("whitelabel-status: subscriber already exists")

	// ErrSubscriberNotFound is returned when the subscriber does not exist.
	ErrSubscriberNotFound = errors.New("whitelabel-status: subscriber not found")

	// ErrInvalidSubdomain is returned for empty or malformed subdomains.
	ErrInvalidSubdomain = errors.New("whitelabel-status: invalid subdomain")

	// ErrInvalidEmail is returned for malformed email addresses.
	ErrInvalidEmail = errors.New("whitelabel-status: invalid email")

	// ErrPageDisabled is returned when the status page is disabled.
	ErrPageDisabled = errors.New("whitelabel-status: status page disabled")

	// ErrInvalidToken is returned for bad confirmation tokens.
	ErrInvalidToken = errors.New("whitelabel-status: invalid confirmation token")
)
