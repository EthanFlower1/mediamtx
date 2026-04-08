// Package middleware provides an HTTP/Connect-Go interceptor that
// auto-records audit entries for every authenticated request.
//
// Contract (see ../README.md):
//   - 2xx responses produce a ResultAllow entry
//   - 403 responses produce a ResultDeny entry
//   - 4xx (non-403) and 5xx responses do NOT auto-record; the handler must
//     call Recorder.Record explicitly with ResultError + an error_code
//
// The explicit-error rule exists because a generic "status code 500 means
// error" recording would lie about *what* failed. Handlers know the error
// code to attribute; middleware does not.
package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
)

// PrincipalExtractor pulls the authenticated principal off a request.
// Cloud handlers already populate a session context upstream of this
// middleware; the extractor is a small adapter that converts that into the
// fields the audit log needs.
//
// Returning ok=false means "unauthenticated request" and the middleware
// will skip audit recording (the auth middleware earlier in the chain
// should have already refused the request).
type PrincipalExtractor func(r *http.Request) (Principal, bool)

// Principal is the minimum identity surface the audit middleware needs.
type Principal struct {
	TenantID             string
	UserID               string
	Agent                audit.ActorAgent
	ImpersonatingUserID  *string
	ImpersonatedTenantID *string
}

// ActionResolver maps an HTTP request to a canonical action + resource
// pair. Typical implementation walks the Gin/Chi route table; a test-only
// resolver can just hard-code the mapping.
type ActionResolver func(r *http.Request) (action, resourceType, resourceID string)

// Config wires the middleware to the surrounding cloud stack.
type Config struct {
	Recorder  audit.Recorder
	Principal PrincipalExtractor
	Resolve   ActionResolver
	RequestID func(r *http.Request) string // optional; defaults to X-Request-ID header
	Clock     func() time.Time             // optional; defaults to time.Now
}

// New returns a standard http.Handler middleware.
func New(cfg Config) func(http.Handler) http.Handler {
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.RequestID == nil {
		cfg.RequestID = func(r *http.Request) string { return r.Header.Get("X-Request-Id") }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)

			// Only auto-record the two "clean" outcomes.
			var result audit.Result
			switch {
			case rw.status >= 200 && rw.status < 300:
				result = audit.ResultAllow
			case rw.status == http.StatusForbidden:
				result = audit.ResultDeny
			default:
				return
			}

			principal, ok := cfg.Principal(r)
			if !ok {
				return
			}
			action, resourceType, resourceID := cfg.Resolve(r)
			if action == "" || resourceType == "" {
				// Route was not mapped — refusing to emit a half-populated
				// entry is safer than writing noise to the log. The cloud
				// integration tests catch missing mappings.
				return
			}

			entry := audit.Entry{
				TenantID:             principal.TenantID,
				ActorUserID:          principal.UserID,
				ActorAgent:           principal.Agent,
				ImpersonatingUserID:  principal.ImpersonatingUserID,
				ImpersonatedTenantID: principal.ImpersonatedTenantID,
				Action:               action,
				ResourceType:         resourceType,
				ResourceID:           resourceID,
				Result:               result,
				IPAddress:            clientIP(r),
				UserAgent:            r.UserAgent(),
				RequestID:            cfg.RequestID(r),
				Timestamp:            cfg.Clock().UTC(),
			}
			// Audit failure should NOT overwrite the response the handler
			// already wrote — instead we detach the context so a slow
			// record call doesn't block the client. The caller's logger
			// (injected separately) picks up any error.
			go func(ctx context.Context, e audit.Entry) {
				_ = cfg.Recorder.Record(ctx, e)
			}(context.WithoutCancel(r.Context()), entry)
		})
	}
}

// statusRecorder captures the outgoing status so the middleware can decide
// whether to emit an allow or deny entry.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wrote {
		s.wrote = true
	}
	return s.ResponseWriter.Write(b)
}

// Status exposes the captured status for tests.
func (s *statusRecorder) Status() int { return s.status }

func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		return v
	}
	return r.RemoteAddr
}
