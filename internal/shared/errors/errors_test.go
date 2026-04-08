package errors_test

import (
	"context"
	stderrors "errors"
	"encoding/json"
	"strings"
	"testing"

	errs "github.com/bluenviron/mediamtx/internal/shared/errors"
)

func TestNew(t *testing.T) {
	e := errs.New(errs.CodeAuthInvalidCredentials, "bad creds")
	if e.Code != errs.CodeAuthInvalidCredentials {
		t.Fatalf("code = %q", e.Code)
	}
	if e.Message != "bad creds" {
		t.Fatalf("message = %q", e.Message)
	}
	if !strings.Contains(e.Error(), "auth.invalid_credentials") {
		t.Fatalf("Error() missing code: %q", e.Error())
	}
}

func TestNewWithOptions(t *testing.T) {
	e := errs.New(
		errs.CodeStreamTokenExpired,
		"token expired",
		errs.WithCorrelationID("corr-123"),
		errs.WithSuggestion("Re-mint via /api/stream/token"),
	)
	if e.CorrelationID != "corr-123" {
		t.Fatalf("correlation = %q", e.CorrelationID)
	}
	if e.Suggestion == "" {
		t.Fatalf("suggestion not set")
	}
}

func TestWrapPropagatesCorrelationID(t *testing.T) {
	inner := errs.New(errs.CodeStreamCameraNotFound, "no cam",
		errs.WithCorrelationID("corr-xyz"))
	outer := errs.Wrap(inner, errs.CodeStreamRecorderOffline, "recorder gone")
	if outer.CorrelationID != "corr-xyz" {
		t.Fatalf("correlation not propagated: %q", outer.CorrelationID)
	}
	if !stderrors.Is(outer, inner) {
		t.Fatalf("errors.Is should match wrapped *Error by code")
	}
}

func TestWrapDoesNotOverrideExplicitCorrelationID(t *testing.T) {
	inner := errs.New(errs.CodeStreamCameraNotFound, "no cam",
		errs.WithCorrelationID("inner"))
	outer := errs.Wrap(inner, errs.CodeStreamRecorderOffline, "recorder gone",
		errs.WithCorrelationID("outer"))
	if outer.CorrelationID != "outer" {
		t.Fatalf("explicit correlation overwritten: %q", outer.CorrelationID)
	}
}

func TestUnwrap(t *testing.T) {
	cause := stderrors.New("network blew up")
	e := errs.Wrap(cause, errs.CodeNotificationChannelFailed, "send failed")
	if got := stderrors.Unwrap(e); got != cause {
		t.Fatalf("unwrap = %v, want cause", got)
	}
}

func TestErrorsIsByCode(t *testing.T) {
	a := errs.New(errs.CodeAuthExpiredToken, "a")
	b := errs.New(errs.CodeAuthExpiredToken, "different message")
	if !stderrors.Is(a, b) {
		t.Fatalf("two errors with same code should be Is-equal")
	}
	c := errs.New(errs.CodeAuthMissingToken, "c")
	if stderrors.Is(a, c) {
		t.Fatalf("different codes should NOT be Is-equal")
	}
}

func TestJSONRoundtrip(t *testing.T) {
	orig := errs.New(
		errs.CodeBillingCardDeclined,
		"Your card was declined.",
		errs.WithCorrelationID("01HXYZ"),
		errs.WithSuggestion("Try a different payment method."),
		errs.WithCause(stderrors.New("stripe: do_not_honor")),
	)
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Cause must NEVER appear on the wire.
	if strings.Contains(string(data), "stripe") || strings.Contains(string(data), "do_not_honor") {
		t.Fatalf("cause leaked into wire envelope: %s", data)
	}
	var got errs.Error
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Code != orig.Code || got.Message != orig.Message ||
		got.CorrelationID != orig.CorrelationID || got.Suggestion != orig.Suggestion {
		t.Fatalf("roundtrip mismatch:\n got=%+v\nwant=%+v", got, *orig)
	}
}

func TestJSONOmitsEmptyOptionalFields(t *testing.T) {
	e := errs.New(errs.CodePermissionDenied, "denied")
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	if strings.Contains(s, "correlation_id") {
		t.Fatalf("correlation_id should be omitted: %s", s)
	}
	if strings.Contains(s, "suggestion") {
		t.Fatalf("suggestion should be omitted: %s", s)
	}
}

func TestCorrelationIDContext(t *testing.T) {
	ctx := errs.ContextWithCorrelationID(context.Background(), "ctx-id")
	got, ok := errs.CorrelationIDFromContext(ctx)
	if !ok || got != "ctx-id" {
		t.Fatalf("ctx round-trip: got=%q ok=%v", got, ok)
	}
	e := errs.New(errs.CodeTenantNotFound, "no tenant",
		errs.WithCorrelationIDFromContext(ctx))
	if e.CorrelationID != "ctx-id" {
		t.Fatalf("correlation not pulled from ctx: %q", e.CorrelationID)
	}
}

func TestCorrelationIDFromContextMissing(t *testing.T) {
	if _, ok := errs.CorrelationIDFromContext(context.Background()); ok {
		t.Fatalf("expected no correlation id on bare context")
	}
}

func TestIsSecurityCritical(t *testing.T) {
	closed := []errs.Code{
		errs.CodeAuthInvalidCredentials,
		errs.CodeAuthExpiredToken,
		errs.CodeAuthMissingToken,
		errs.CodeAuthSSOFailed,
		errs.CodeAuthLocalLoginDisabled,
		errs.CodePermissionDenied,
		errs.CodePermissionInsufficientScope,
		errs.CodePermissionCrossTenantBlocked,
		errs.CodeTenantNotFound,
		errs.CodeTenantIsolationViolation,
		errs.CodeTenantQuotaExceeded,
	}
	for _, c := range closed {
		if !errs.IsSecurityCritical(c) {
			t.Errorf("expected %s to be security-critical", c)
		}
	}
	open := []errs.Code{
		errs.CodeStreamTokenExpired,
		errs.CodeStreamCameraNotFound,
		errs.CodeStreamRecorderOffline,
		errs.CodeBillingCardDeclined,
		errs.CodeBillingOverageLimit,
		errs.CodeNotificationChannelFailed,
		errs.CodeNotificationRateLimited,
	}
	for _, c := range open {
		if errs.IsSecurityCritical(c) {
			t.Errorf("expected %s to be fail-open (not security-critical)", c)
		}
	}
}

func TestCodeOfAndCorrelationIDOf(t *testing.T) {
	e := errs.New(errs.CodeStreamNonceReused, "replay",
		errs.WithCorrelationID("c-1"))
	wrapped := stderrors.New("outer: " + e.Error())
	_ = wrapped // not an Error, won't match

	if errs.CodeOf(e) != errs.CodeStreamNonceReused {
		t.Fatalf("CodeOf = %q", errs.CodeOf(e))
	}
	if errs.CorrelationIDOf(e) != "c-1" {
		t.Fatalf("CorrelationIDOf = %q", errs.CorrelationIDOf(e))
	}
	if errs.CodeOf(nil) != "" {
		t.Fatalf("CodeOf(nil) should be empty")
	}
}

// TestNoCodeReuse is the linter: it walks the registry and verifies that
// every Code string is unique. Retired entries still occupy their slot —
// reuse remains forbidden. This test fails the build if any developer
// accidentally publishes the same code twice.
func TestNoCodeReuse(t *testing.T) {
	seen := make(map[errs.Code]int, len(errs.Registry))
	for i, entry := range errs.Registry {
		if entry.Code == "" {
			t.Errorf("registry[%d]: empty code", i)
			continue
		}
		if prev, ok := seen[entry.Code]; ok {
			t.Errorf("error code %q reused: registry[%d] and registry[%d] (codes are append-only and never reused — mark old entry Retired:true and choose a new code)",
				entry.Code, prev, i)
		}
		seen[entry.Code] = i
	}
}

func TestRegistryWellFormed(t *testing.T) {
	for _, entry := range errs.Registry {
		s := string(entry.Code)
		if !strings.Contains(s, ".") {
			t.Errorf("code %q must be of the form <domain>.<reason>", s)
		}
		if entry.Description == "" {
			t.Errorf("code %q has no description", s)
		}
	}
}

func TestLookupCode(t *testing.T) {
	if _, ok := errs.LookupCode(errs.CodeAuthInvalidCredentials); !ok {
		t.Fatalf("expected CodeAuthInvalidCredentials in registry")
	}
	if _, ok := errs.LookupCode(errs.Code("nope.does_not_exist")); ok {
		t.Fatalf("unexpected hit on bogus code")
	}
}

// TestFailOpenRecordingUnderAuthOutage models the fail-open policy at a
// recorder admission point: if Zitadel returns auth errors, the recorder
// should classify the upstream error as security-critical (so the admin UI
// denies the user) but the recording loop itself must NOT abort. This test
// pins the classification side; the runtime behavior is enforced by the
// recorder package.
func TestFailOpenRecordingUnderAuthOutage(t *testing.T) {
	zitadelDown := errs.New(errs.CodeAuthSSOFailed, "Zitadel unreachable")
	if !zitadelDown.IsSecurityCritical() {
		t.Fatalf("auth.sso_failed must be security-critical (fail closed for users)")
	}
	// Simulate the recorder gate. The recording loop checks for stream.*
	// errors specifically; an auth.* error from a sidecar must NOT cause
	// the recording loop to drop frames.
	recordingShouldStop := errs.IsSecurityCritical(errs.CodeStreamCameraNotFound)
	if recordingShouldStop {
		t.Fatalf("stream.* errors must be fail-open; recording must continue")
	}
}
