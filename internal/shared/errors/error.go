package errors

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
)

// Error is the canonical error type returned across the Kaivue Recording
// Server. It carries a stable Code, a translatable customer-facing Message,
// a CorrelationID for log lookup, an optional Suggestion for the customer,
// and an optional wrapped Cause for server-side debugging.
//
// Error implements the standard error interface and integrates with
// errors.Is / errors.Unwrap so it composes cleanly with stdlib error
// handling. Its JSON marshaling is the public wire format documented in
// the package doc.
type Error struct {
	// Code is the stable, public error identifier. Required.
	Code Code
	// Message is the human-readable, translatable customer-facing string.
	// Callers are responsible for translation before construction; this
	// package does not own i18n.
	Message string
	// CorrelationID ties this error to server-side log lines. If empty,
	// the wire envelope omits the field.
	CorrelationID string
	// Suggestion is an optional next-step the customer can take.
	Suggestion string
	// Cause is the wrapped underlying error, if any. It is NOT serialized
	// onto the wire — it exists for server-side logging and errors.Is /
	// errors.As traversal only.
	Cause error
}

// Option configures an Error during New / Wrap.
type Option func(*Error)

// WithCorrelationID attaches an explicit correlation ID to the error.
func WithCorrelationID(id string) Option {
	return func(e *Error) { e.CorrelationID = id }
}

// WithCorrelationIDFromContext pulls the correlation ID out of ctx (if
// present) and attaches it to the error. This is the preferred form for
// handler / service code.
func WithCorrelationIDFromContext(ctx context.Context) Option {
	return func(e *Error) {
		if id, ok := CorrelationIDFromContext(ctx); ok {
			e.CorrelationID = id
		}
	}
}

// WithSuggestion attaches an optional actionable suggestion.
func WithSuggestion(s string) Option {
	return func(e *Error) { e.Suggestion = s }
}

// WithCause attaches a wrapped underlying error. Prefer Wrap() at call
// sites that already have an error in hand; this option exists for symmetry
// with the other options.
func WithCause(cause error) Option {
	return func(e *Error) { e.Cause = cause }
}

// New constructs a new Error with the given code and customer-visible
// message. Code MUST be present in the registry; in tests this is enforced
// by TestRegistryCoversAllConstants. At runtime an unregistered code is
// still permitted (so a hot-fix doesn't crash the process), but should be
// caught by CI.
func New(code Code, message string, opts ...Option) *Error {
	e := &Error{Code: code, Message: message}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Wrap constructs a new Error that wraps cause. If cause is itself an
// *Error and no explicit correlation ID is supplied via opts, the
// correlation ID is propagated from the wrapped error so log lookups stay
// consistent across layers.
func Wrap(cause error, code Code, message string, opts ...Option) *Error {
	e := &Error{Code: code, Message: message, Cause: cause}
	for _, opt := range opts {
		opt(e)
	}
	if e.CorrelationID == "" {
		var inner *Error
		if stderrors.As(cause, &inner) {
			e.CorrelationID = inner.CorrelationID
		}
	}
	return e
}

// Error implements the standard error interface. The format is intended
// for server-side logs, not the wire envelope.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the wrapped cause, supporting errors.Is / errors.As.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Is reports whether target is the same kind of error as e. Two *Error
// values match when their Code is equal — Code is the stable identity of
// the error. This makes idiomatic checks like
// `errors.Is(err, errors.New(errors.CodeAuthInvalidCredentials, ""))`
// work, but the preferred form is errors.Is(err, errors.ErrAuthInvalidCredentials)
// or a direct code comparison via CodeOf(err).
func (e *Error) Is(target error) bool {
	if e == nil || target == nil {
		return e == nil && target == nil
	}
	var t *Error
	if !stderrors.As(target, &t) {
		return false
	}
	return e.Code == t.Code
}

// IsSecurityCritical reports whether this error is in a fail-closed
// domain (auth / permission / tenant).
func (e *Error) IsSecurityCritical() bool {
	if e == nil {
		return false
	}
	return IsSecurityCritical(e.Code)
}

// wireEnvelope is the JSON shape sent to customers. Field names and tags
// are part of the public API.
type wireEnvelope struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	CorrelationID string `json:"correlation_id,omitempty"`
	Suggestion    string `json:"suggestion,omitempty"`
}

// MarshalJSON serializes the customer-facing wire envelope. The wrapped
// Cause is intentionally NOT included — it is for server-side logs only.
func (e *Error) MarshalJSON() ([]byte, error) {
	if e == nil {
		return []byte("null"), nil
	}
	return json.Marshal(wireEnvelope{
		Code:          string(e.Code),
		Message:       e.Message,
		CorrelationID: e.CorrelationID,
		Suggestion:    e.Suggestion,
	})
}

// UnmarshalJSON parses the wire envelope back into an Error. Round-trips
// against MarshalJSON.
func (e *Error) UnmarshalJSON(data []byte) error {
	var w wireEnvelope
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	e.Code = Code(w.Code)
	e.Message = w.Message
	e.CorrelationID = w.CorrelationID
	e.Suggestion = w.Suggestion
	return nil
}

// CodeOf returns the Code of the first *Error in err's chain, or "" if
// none is present.
func CodeOf(err error) Code {
	if err == nil {
		return ""
	}
	var e *Error
	if stderrors.As(err, &e) {
		return e.Code
	}
	return ""
}

// CorrelationIDOf returns the correlation ID of the first *Error in err's
// chain, or "" if none is present.
func CorrelationIDOf(err error) string {
	if err == nil {
		return ""
	}
	var e *Error
	if stderrors.As(err, &e) {
		return e.CorrelationID
	}
	return ""
}

// ----------------------------------------------------------------------------
// Correlation ID context plumbing
// ----------------------------------------------------------------------------

type correlationIDKey struct{}

// ContextWithCorrelationID returns a child context carrying the given
// correlation ID. Middleware should call this once per request.
func ContextWithCorrelationID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, correlationIDKey{}, id)
}

// CorrelationIDFromContext retrieves the correlation ID stored in ctx, if
// any. The boolean reports presence so callers can distinguish "no ID" from
// "empty string ID".
func CorrelationIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	v, ok := ctx.Value(correlationIDKey{}).(string)
	return v, ok
}
