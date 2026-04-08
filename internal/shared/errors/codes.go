package errors

import "strings"

// Code is a stable, public, dotted error identifier of the form
// "<domain>.<reason>". Codes are part of the product's public API and MUST
// NOT be reused once published. To retire a code, set Retired:true on its
// registry entry — never delete the line and never reassign the string.
type Code string

// Domain returns the leading "<domain>" segment of the code.
func (c Code) Domain() string {
	s := string(c)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return s[:i]
	}
	return s
}

// String implements fmt.Stringer.
func (c Code) String() string { return string(c) }

// CodeInfo is the registry entry for a single error code.
type CodeInfo struct {
	// Code is the stable dotted identifier (e.g. "auth.invalid_credentials").
	Code Code
	// Description is a one-line, developer-facing explanation of when this
	// code is emitted. Customer-facing copy lives on the Error.Message
	// field at construction time and is translatable.
	Description string
	// Retired marks a code that has been removed from active use. Retired
	// codes remain in the registry forever as tombstones to guarantee
	// non-reuse. The linter still rejects reuse of the string.
	Retired bool
}

// ----------------------------------------------------------------------------
// Auth domain — fail closed.
// ----------------------------------------------------------------------------

const (
	CodeAuthInvalidCredentials  Code = "auth.invalid_credentials"
	CodeAuthExpiredToken        Code = "auth.expired_token"
	CodeAuthMissingToken        Code = "auth.missing_token"
	CodeAuthSSOFailed           Code = "auth.sso_failed"
	CodeAuthLocalLoginDisabled  Code = "auth.local_login_disabled"
)

// ----------------------------------------------------------------------------
// Permission domain — fail closed.
// ----------------------------------------------------------------------------

const (
	CodePermissionDenied             Code = "permission.denied"
	CodePermissionInsufficientScope  Code = "permission.insufficient_scope"
	CodePermissionCrossTenantBlocked Code = "permission.cross_tenant_blocked"
)

// ----------------------------------------------------------------------------
// Stream domain — fail open (keep recording).
// ----------------------------------------------------------------------------

const (
	CodeStreamTokenExpired    Code = "stream.token_expired"
	CodeStreamNonceReused     Code = "stream.nonce_reused"
	CodeStreamCameraNotFound  Code = "stream.camera_not_found"
	CodeStreamRecorderOffline Code = "stream.recorder_offline"
)

// ----------------------------------------------------------------------------
// Tenant domain — fail closed.
// ----------------------------------------------------------------------------

const (
	CodeTenantNotFound          Code = "tenant.not_found"
	CodeTenantIsolationViolation Code = "tenant.isolation_violation"
	CodeTenantQuotaExceeded     Code = "tenant.quota_exceeded"
)

// ----------------------------------------------------------------------------
// Billing domain — fail open (never block recording on a billing blip).
// ----------------------------------------------------------------------------

const (
	CodeBillingCardDeclined         Code = "billing.card_declined"
	CodeBillingPlanDowngradeBlocked Code = "billing.plan_downgrade_blocked"
	CodeBillingOverageLimit         Code = "billing.overage_limit"
)

// ----------------------------------------------------------------------------
// Notification domain — fail open.
// ----------------------------------------------------------------------------

const (
	CodeNotificationChannelFailed Code = "notification.channel_failed"
	CodeNotificationRateLimited   Code = "notification.rate_limited"
)

// securityCriticalDomains lists the domains for which the system fails
// CLOSED. Adding a domain here is a security-sensitive change and should be
// reviewed by security-compliance.
var securityCriticalDomains = map[string]struct{}{
	"auth":       {},
	"permission": {},
	"tenant":     {},
}

// IsSecurityCriticalDomain reports whether errors in the given domain must
// fail closed (deny by default).
func IsSecurityCriticalDomain(domain string) bool {
	_, ok := securityCriticalDomains[domain]
	return ok
}

// IsSecurityCritical reports whether the given code is in a fail-closed
// domain. Use this at policy decision points — middleware, recorder gates,
// admission controllers — to choose between deny-by-default and
// serve-from-cache behavior.
func IsSecurityCritical(c Code) bool {
	return IsSecurityCriticalDomain(c.Domain())
}

// Registry is the canonical, ordered list of every error code the product
// has ever published. Entries are append-only. To retire a code, set
// Retired:true on its entry; do NOT delete the line and do NOT reuse the
// string for a new code. The TestNoCodeReuse unit test enforces this.
var Registry = []CodeInfo{
	// auth.*
	{Code: CodeAuthInvalidCredentials, Description: "Username or password did not match a known principal."},
	{Code: CodeAuthExpiredToken, Description: "The presented token is past its expiration."},
	{Code: CodeAuthMissingToken, Description: "No bearer token was supplied on a request that requires one."},
	{Code: CodeAuthSSOFailed, Description: "The upstream SSO/IdP rejected the assertion or was unreachable."},
	{Code: CodeAuthLocalLoginDisabled, Description: "Local username/password login is disabled for this tenant; SSO is required."},

	// permission.*
	{Code: CodePermissionDenied, Description: "The principal lacks the required role for this operation."},
	{Code: CodePermissionInsufficientScope, Description: "The token is valid but lacks a scope required for this operation."},
	{Code: CodePermissionCrossTenantBlocked, Description: "Request attempted to access a resource owned by a different tenant."},

	// stream.*
	{Code: CodeStreamTokenExpired, Description: "Short-lived stream token is past its expiration; client should re-mint."},
	{Code: CodeStreamNonceReused, Description: "A stream nonce was presented twice; possible replay attack."},
	{Code: CodeStreamCameraNotFound, Description: "Requested camera ID is not in the directory."},
	{Code: CodeStreamRecorderOffline, Description: "The recorder responsible for this camera is currently unreachable."},

	// tenant.*
	{Code: CodeTenantNotFound, Description: "Requested tenant ID does not exist."},
	{Code: CodeTenantIsolationViolation, Description: "Operation crossed a tenant isolation boundary; rejected."},
	{Code: CodeTenantQuotaExceeded, Description: "Tenant has exceeded a hard quota (cameras, storage, bandwidth, ...)."},

	// billing.*
	{Code: CodeBillingCardDeclined, Description: "Payment processor declined the card on file."},
	{Code: CodeBillingPlanDowngradeBlocked, Description: "Plan downgrade blocked because current usage exceeds the target plan."},
	{Code: CodeBillingOverageLimit, Description: "Tenant has reached its configured overage spending limit."},

	// notification.*
	{Code: CodeNotificationChannelFailed, Description: "An outbound notification channel (email, SMS, push, webhook) failed to deliver."},
	{Code: CodeNotificationRateLimited, Description: "Notification was suppressed by the per-tenant rate limiter."},
}

// LookupCode returns the registry entry for a given code, or false if the
// code is not registered. Useful for the linter and for runtime sanity
// checks in tests.
func LookupCode(c Code) (CodeInfo, bool) {
	for _, e := range Registry {
		if e.Code == c {
			return e, true
		}
	}
	return CodeInfo{}, false
}
