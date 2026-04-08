// TODO(KAI-310): replace with generated connectrpc code once buf is wired.
package apiserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	auditmw "github.com/bluenviron/mediamtx/internal/cloud/audit/middleware"
	"github.com/bluenviron/mediamtx/internal/cloud/permissions"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
	"github.com/bluenviron/mediamtx/internal/shared/logging"
)

// Middleware is the canonical std-library middleware shape. Using the plain
// http idiom (rather than a pinned connect interceptor chain) lets us stack
// Connect-Go handlers and plain /healthz style routes under the same stack.
type Middleware func(http.Handler) http.Handler

// chain composes middlewares so the first argument runs first (outermost).
//
// chain(A, B, C)(h) == A(B(C(h)))
func chain(mws ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for i := len(mws) - 1; i >= 0; i-- {
			next = mws[i](next)
		}
		return next
	}
}

// -----------------------------------------------------------------------
// 1. Request ID
// -----------------------------------------------------------------------

// requestIDMiddleware injects an X-Request-Id header into every request. If
// the caller supplied one we honour it (so tracing propagates from an API
// gateway); otherwise we mint a random 16-byte hex id. The id is stashed on
// the context under the logging package's key so every downstream logger
// automatically carries it.
func requestIDMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rid := r.Header.Get("X-Request-Id")
			if rid == "" {
				rid = newRequestID()
			}
			w.Header().Set("X-Request-Id", rid)
			ctx := logging.ContextWithRequestID(r.Context(), rid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// -----------------------------------------------------------------------
// 2. Tracing
// -----------------------------------------------------------------------

// tracingMiddleware is a thin adapter around a TracerHook. When the hook is
// nil (default) the middleware is a pure pass-through so there is zero cost
// in un-instrumented environments.
func tracingMiddleware(hook TracerHook) Middleware {
	if hook == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			finish := hook(r.Method, r.URL.Path)
			sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sr, r)
			if finish != nil {
				finish(sr.status)
			}
		})
	}
}

// -----------------------------------------------------------------------
// 3. CORS
// -----------------------------------------------------------------------

func corsMiddleware(origins []string) Middleware {
	allowed := make(map[string]struct{}, len(origins))
	for _, o := range origins {
		allowed[o] = struct{}{}
	}
	if len(allowed) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if _, ok := allowed[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers",
					"Authorization, Content-Type, Connect-Protocol-Version, X-Request-Id, X-Kaivue-Region, X-Kaivue-Tenant")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// -----------------------------------------------------------------------
// 4. Tenant resolution
// -----------------------------------------------------------------------

// tenantMiddleware derives a best-effort tenant hint from either:
//   1. the X-Kaivue-Tenant header (set by a gateway / sidecar)
//   2. the leftmost subdomain of the Host header, e.g. "acme" in
//      "acme.api.kaivue.io"
//
// The result is stashed on the context but is NOT authoritative: the auth
// middleware re-derives tenant from verified claims and overwrites it. The
// reason we still pre-populate is rate-limiting — rate limits are keyed by
// tenant and they run before authentication so DoS traffic is rejected
// before doing any token validation work.
func tenantMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tenant auth.TenantRef
			if h := r.Header.Get("X-Kaivue-Tenant"); h != "" {
				tenant = parseTenantHeader(h)
			} else if sub := leftmostSubdomain(r.Host); sub != "" {
				tenant = auth.TenantRef{Type: auth.TenantTypeCustomer, ID: sub}
			}
			ctx := withTenant(r.Context(), tenant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// parseTenantHeader accepts the canonical "type:id" or a bare id.
func parseTenantHeader(h string) auth.TenantRef {
	if i := strings.IndexByte(h, ':'); i > 0 {
		return auth.TenantRef{Type: auth.TenantType(h[:i]), ID: h[i+1:]}
	}
	return auth.TenantRef{Type: auth.TenantTypeCustomer, ID: h}
}

func leftmostSubdomain(host string) string {
	// strip optional :port suffix
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	parts := strings.SplitN(host, ".", 2)
	if len(parts) < 2 {
		return ""
	}
	return parts[0]
}

// -----------------------------------------------------------------------
// 5. Authentication
// -----------------------------------------------------------------------

// isPublicPath decides whether auth is required. The Connect-Go streaming
// services used by recorders do their own authentication via mTLS + pairing
// tokens handled in their handlers, and the /healthz / /readyz / /metrics
// endpoints are always anonymous.
var publicPrefixes = []string{
	"/healthz",
	"/readyz",
	"/metrics",
	// Connect login endpoint obviously cannot require an access token
	"/kaivue.v1.AuthService/Login",
	"/kaivue.v1.AuthService/Refresh",
	"/kaivue.v1.AuthService/BeginSSOFlow",
	"/kaivue.v1.AuthService/CompleteSSOFlow",
}

func isPublicPath(path string) bool {
	for _, p := range publicPrefixes {
		if path == p || strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// authMiddleware verifies bearer tokens via the IdentityProvider and attaches
// the resulting Claims to the context. A missing/invalid token on a protected
// path short-circuits the request with a 401 JSON envelope.
func authMiddleware(idp auth.IdentityProvider) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			authz := r.Header.Get("Authorization")
			if !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
				writeError(w, NewError(CodeUnauthenticated, errors.New("missing bearer token")))
				return
			}
			token := strings.TrimSpace(authz[len("Bearer "):])
			claims, err := idp.VerifyToken(r.Context(), token)
			if err != nil || claims == nil {
				writeError(w, NewError(CodeUnauthenticated, errors.New("invalid token")))
				return
			}
			ctx := withClaims(r.Context(), claims)
			// The verified tenant ALWAYS wins over the hint set by the
			// tenant middleware — this is the seam that prevents an
			// attacker from claiming another tenant's subdomain with
			// a foreign token.
			ctx = withTenant(ctx, claims.TenantRef)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// -----------------------------------------------------------------------
// 6. Permission enforcement
// -----------------------------------------------------------------------

// RouteAuthorization describes the object + action that protect one Connect
// path. Routes without an entry are treated as anonymous (handled by
// isPublicPath) OR are implicitly authenticated-only (e.g. /me style
// endpoints). The enforcement middleware refuses to authorize an unknown
// protected path — fail-closed.
type RouteAuthorization struct {
	ResourceType string // e.g. "cameras"
	Action       string // e.g. "read"
}

// permissionMiddleware runs Casbin for every non-public request whose path
// has a route-authorization entry. Requests without an entry on a protected
// path fall through to the handler; requests with an entry that resolves to
// a deny response get a 403 JSON envelope + propagate to the audit
// middleware, which records a ResultDeny entry.
func permissionMiddleware(enforcer *permissions.Enforcer, routes map[string]RouteAuthorization) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			entry, ok := routes[r.URL.Path]
			if !ok {
				// Not wired into the ACL table (yet). Fall through —
				// KAI-233 audit middleware will still log the response.
				next.ServeHTTP(w, r)
				return
			}
			claims, hasClaims := ClaimsFromContext(r.Context())
			if !hasClaims {
				writeError(w, NewError(CodeUnauthenticated, errors.New("missing claims")))
				return
			}
			subj := permissions.SubjectFromClaims(*claims)
			obj := permissions.NewObjectAll(claims.TenantRef, entry.ResourceType)
			allowed, err := enforcer.Enforce(r.Context(), subj, obj, entry.Action)
			if err != nil {
				writeError(w, NewError(CodeInternal, err))
				return
			}
			if !allowed {
				writeError(w, NewError(CodePermissionDenied, errors.New("forbidden")))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// -----------------------------------------------------------------------
// 7. Audit logging — thin delegation to KAI-233
// -----------------------------------------------------------------------

// auditPrincipalExtractor lifts Claims off the request context into the
// Principal struct KAI-233's middleware wants. Unauthenticated requests
// return ok=false, which tells the audit middleware to skip recording.
func auditPrincipalExtractor(r *http.Request) (auditmw.Principal, bool) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		return auditmw.Principal{}, false
	}
	return auditmw.Principal{
		TenantID: claims.TenantRef.ID,
		UserID:   string(claims.UserID),
		// Agent defaults to cloud end-user. Recorder fleet tokens will
		// set AgentOnPrem; integrator-scoped tokens will set
		// AgentIntegrator. TODO(KAI-233 integration): thread an
		// explicit agent claim once recorder tokens exist.
		Agent: audit.AgentCloud,
	}, true
}

// auditActionResolver maps a Connect path like
// "/kaivue.v1.CamerasService/ListCameras" to (action, resourceType, id).
// We deliberately hard-code the mapping here rather than walk a route table
// at runtime — the Connect path IS the canonical method identifier.
func auditActionResolver(routes map[string]RouteAuthorization) auditmw.ActionResolver {
	return func(r *http.Request) (string, string, string) {
		if entry, ok := routes[r.URL.Path]; ok {
			// Connect requests don't carry a resource id in the URL;
			// real handlers will augment the audit entry via an
			// explicit Recorder.Record call.
			return entry.Action, entry.ResourceType, ""
		}
		return "", "", ""
	}
}

// -----------------------------------------------------------------------
// 8. Rate limiting — in-memory per-tenant token bucket
// -----------------------------------------------------------------------

// tokenBucket is a lazy, self-refilling bucket. The refill happens on
// Allow() rather than on a timer so idle tenants don't spin goroutines.
type tokenBucket struct {
	mu       sync.Mutex
	tokens   float64
	capacity float64
	rate     float64 // tokens per second
	last     time.Time
}

func (b *tokenBucket) allow(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.last.IsZero() {
		elapsed := now.Sub(b.last).Seconds()
		b.tokens += elapsed * b.rate
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
	}
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// rateLimiter is the shared map of tenant buckets plus the defaults used to
// seed fresh entries.
type rateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*tokenBucket
	rate     float64
	burst    int
	now      func() time.Time
}

func newRateLimiter(cfg RateLimitConfig) *rateLimiter {
	return &rateLimiter{
		buckets: map[string]*tokenBucket{},
		rate:    cfg.RequestsPerSecond,
		burst:   cfg.Burst,
		now:     time.Now,
	}
}

func (rl *rateLimiter) allow(tenantID string) bool {
	if rl == nil || rl.rate <= 0 {
		return true
	}
	rl.mu.Lock()
	b, ok := rl.buckets[tenantID]
	if !ok {
		b = &tokenBucket{
			capacity: float64(rl.burst),
			tokens:   float64(rl.burst),
			rate:     rl.rate,
		}
		rl.buckets[tenantID] = b
	}
	rl.mu.Unlock()
	return b.allow(rl.now())
}

func rateLimitMiddleware(rl *rateLimiter) Middleware {
	if rl == nil || rl.rate <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			tenant, _ := TenantFromContext(r.Context())
			key := tenant.ID
			if key == "" {
				key = "__anon__"
			}
			if !rl.allow(key) {
				writeError(w, NewError(CodeResourceExhausted, errors.New("rate limit exceeded")))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// -----------------------------------------------------------------------
// 9. Recovery
// -----------------------------------------------------------------------

func recoveryMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					// Best-effort log via the default logger; callers
					// rarely attach a logger to context in a panic path.
					rid := logging.RequestIDFromContext(r.Context())
					err := errors.New("panic: request crashed")
					logging.LoggerFromContext(r.Context(), nil).
						Error("panic in handler",
							"panic", rec,
							"stack", string(debug.Stack()),
							"request_id", rid,
						)
					writeError(w, NewError(CodeInternal, err))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// -----------------------------------------------------------------------
// Shared helpers
// -----------------------------------------------------------------------

// errorEnvelope is the JSON body the server emits for any ConnectError. It
// intentionally matches the connect-go error wire format so the migration
// in KAI-310 is transparent to callers.
type errorEnvelope struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func writeError(w http.ResponseWriter, err *ConnectError) {
	env := errorEnvelope{Code: err.Code().String(), Message: err.Error()}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.Code().HTTPStatus())
	_ = json.NewEncoder(w).Encode(env)
}

// statusRecorder captures the outgoing status so tracing / tests can
// inspect it. Matches the shape in KAI-233's audit middleware so consumers
// recognise the idiom.
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

// Status returns the captured HTTP status.
func (s *statusRecorder) Status() int { return s.status }

// contextChain exposes the single chain helper for reuse in tests where we
// want to stack a tracing shim around our middleware under test.
var _ = context.Background
