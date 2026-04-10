package email

import (
	"errors"
	"time"
)

// Dialect selects SQL flavour. Production = Postgres; tests = SQLite.
type Dialect int

const (
	DialectPostgres Dialect = iota
	DialectSQLite
)

// Sentinel errors.
var (
	ErrMissingTenant     = errors.New("email: tenant_id required")
	ErrMissingDomain     = errors.New("email: domain required")
	ErrInvalidDomain     = errors.New("email: domain is not a valid FQDN")
	ErrUnknownDialect    = errors.New("email: unknown SQL dialect")
	ErrDomainNotFound    = errors.New("email: domain not found for tenant")
	ErrSelectorInvalid   = errors.New("email: selector must be s1 or s2")
	ErrKeyAlreadyExists  = errors.New("email: DKIM key already exists for this selector")
	ErrVerificationState = errors.New("email: invalid verification state")
)

// VerificationState tracks the DNS-side status of a sender domain.
type VerificationState string

const (
	// VerificationPending — domain created, DNS records computed,
	// integrator has not yet added them to their zone (or resolver
	// has not re-checked since creation).
	VerificationPending VerificationState = "pending"

	// VerificationPartial — at least one of SPF/DKIM/DMARC has
	// validated but not all three. Domain is NOT yet usable as a
	// sender.
	VerificationPartial VerificationState = "partial"

	// VerificationVerified — all three records validated. Domain is
	// usable as a sender.
	VerificationVerified VerificationState = "verified"

	// VerificationFailed — a previously-verified record has gone
	// missing on re-check, or a newly-added record contains wrong
	// values.
	VerificationFailed VerificationState = "failed"
)

// ValidVerificationState returns true if s is one of the known states.
func ValidVerificationState(s VerificationState) bool {
	switch s {
	case VerificationPending, VerificationPartial, VerificationVerified, VerificationFailed:
		return true
	}
	return false
}

// Selector is the DKIM selector label. Only "s1" and "s2" are allowed
// (48h-grace-period rotation pattern).
type Selector string

const (
	SelectorS1 Selector = "s1"
	SelectorS2 Selector = "s2"
)

// ValidSelector returns true for s1 or s2.
func ValidSelector(s Selector) bool {
	return s == SelectorS1 || s == SelectorS2
}

// Domain is a tenant's sender domain record.
type Domain struct {
	ID               string
	TenantID         string
	Domain           string
	SendGridSubuser  string
	ActiveSelector   Selector
	VerificationState VerificationState
	SPFVerifiedAt    *time.Time
	DKIMVerifiedAt   *time.Time
	DMARCVerifiedAt  *time.Time
	LastCheckedAt    *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// DKIMKey holds metadata + the public key for one (domain, selector)
// pair. The private key lives in the cryptostore (KAI-251); this
// struct only knows the cryptostore key id.
type DKIMKey struct {
	ID               string
	TenantID         string
	DomainID         string
	Selector         Selector
	PublicKeyPEM     string
	CryptostoreKeyID string
	KeySizeBits      int
	Status           string // "active" | "retired"
	RotatedAt        *time.Time
	CreatedAt        time.Time
}

// DNSRecords is the set of DNS records an integrator must add to
// their zone to complete verification. Returned from
// Service.ComputeDNSRecords and surfaced to the UI for copy-paste.
type DNSRecords struct {
	// SPF is a TXT record at the domain apex that includes the
	// Kaivue (or SendGrid parent) SPF include macro.
	SPF TXTRecord

	// DKIM is a TXT record at {selector}._domainkey.{domain} that
	// publishes the active DKIM public key.
	DKIM TXTRecord

	// DMARC is a TXT record at _dmarc.{domain} with a conservative
	// "quarantine" policy and a rua= aggregate report address.
	DMARC TXTRecord
}

// TXTRecord is a DNS TXT record the integrator must publish.
type TXTRecord struct {
	Name  string // FQDN, no trailing dot
	Type  string // always "TXT"
	Value string // already quoted / escaped for zone-file use
	TTL   int    // seconds
}
