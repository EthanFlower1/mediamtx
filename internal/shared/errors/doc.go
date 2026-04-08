// Package errors implements the Kaivue Recording Server error-handling
// framework: stable error codes, correlation IDs, and a documented
// fail-closed/fail-open policy.
//
// # Wire envelope
//
// Every error returned to a customer (over HTTP, Connect-Go, gRPC, or any
// other transport) is serialized as the JSON envelope produced by Error's
// MarshalJSON. The envelope is part of the public API contract:
//
//	{
//	  "code":           "auth.invalid_credentials",
//	  "message":        "The username or password is incorrect.",
//	  "correlation_id": "01HXYZ...",
//	  "suggestion":     "Check your credentials or reset your password."
//	}
//
// # Stable codes
//
// Error codes are defined in codes.go. They are stable, public, documented,
// and never reused. A code that is retired must be left in the registry as
// a tombstone (with a Retired:true flag) so the linter can refuse to reuse
// the string. The TestNoCodeReuse unit test enforces this at build time.
//
// # Fail-closed for security, fail-open for recording
//
// The framework codifies one of the most important runtime policies in the
// product:
//
//   - Auth, permission, and tenant errors are SECURITY-CRITICAL. The system
//     fails CLOSED — when in doubt, deny. A token validation outage MUST
//     reject the request rather than admit it. Use IsSecurityCritical(code)
//     to branch on this in middleware and policy decision points.
//
//   - Stream, recording, notification, and billing errors are NOT
//     security-critical. The system fails OPEN for recording specifically —
//     if Zitadel is down, recordings continue from cached camera state and
//     local segment storage. Customers will lose visibility, but they will
//     not lose footage. Recording is the product's core promise; auth
//     outages must never cause data loss.
//
// Concretely:
//
//	if errors.IsSecurityCritical(err.Code) {
//	    // Deny. Log. Page.
//	    return http.StatusForbidden
//	}
//	// Otherwise: serve from cache, queue for retry, keep recording.
//
// # Correlation IDs
//
// Every error carries a correlation ID that ties the customer-visible
// envelope to server-side log lines. Middleware should call
// WithCorrelationID(ctx, id) at request entry; downstream code uses
// CorrelationIDFromContext(ctx) when constructing errors so the same ID
// flows through logs, traces, and the wire envelope.
package errors
