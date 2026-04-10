// Package authz provides HTTP authorization middleware for the Directory API.
// It bridges the shared permissions Enforcer (KAI-145) with the Directory's
// net/http handlers by extracting verified claims from the request context
// and performing a fail-closed Casbin check before the handler runs.
package authz

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
)

// Authorizer is the single authorization interface the middleware depends on.
// It is satisfied by *permissions.Enforcer after KAI-145 merges the package
// into internal/shared/permissions.
type Authorizer interface {
	// Enforce checks whether subject may perform action on object.
	// Returns (true, nil) on allow, (false, nil) on explicit deny,
	// (false, err) on error. Callers MUST treat any non-true result as deny.
	Enforce(ctx context.Context, subject SubjectRef, object ObjectRef, action string) (bool, error)
}

// SubjectRef identifies an actor. Mirrors permissions.SubjectRef but is
// defined locally so this package compiles without the shared/permissions
// import on branches that don't have it yet.
type SubjectRef struct {
	Kind   string // "user", "integrator", "federation"
	ID     string
	Tenant string // tenant ID
}

func (s SubjectRef) String() string {
	switch s.Kind {
	case "user":
		return "user:" + s.ID + "@" + s.Tenant
	case "integrator":
		return "integrator:" + s.ID + "@" + s.Tenant
	case "federation":
		return "federation:" + s.ID
	default:
		return ""
	}
}

func (s SubjectRef) Validate() error { return nil }

// ObjectRef identifies a tenant-scoped resource. Mirrors permissions.ObjectRef.
type ObjectRef struct {
	Tenant       string
	ResourceType string
	ResourceID   string
}

func (o ObjectRef) String() string {
	if o.ResourceID == "" || o.ResourceID == "*" {
		return o.Tenant + "/" + o.ResourceType + "/*"
	}
	return o.Tenant + "/" + o.ResourceType + "/" + o.ResourceID
}

func (o ObjectRef) Validate() error { return nil }

// Claims is the minimal set of verified token claims the middleware needs.
// Satisfied by auth.Claims (or any struct with these fields).
type Claims struct {
	UserID   string
	TenantID string
}

// ClaimsExtractor pulls verified claims from the request context. The authn
// middleware (KAI-129 TokenVerifier) is expected to have put them there.
type ClaimsExtractor func(ctx context.Context) (Claims, bool)

// Route maps an HTTP method+path pattern to a required action and resource type.
type Route struct {
	// Action is the canonical permission action (e.g. "recorder.pair").
	Action string
	// ResourceType is the Casbin object resource type (e.g. "recorders").
	ResourceType string
	// ResourceIDParam is the name of the path parameter that carries the
	// specific resource ID. If empty, the middleware uses "*" (type-level check).
	ResourceIDParam string
}

// PathParamExtractor pulls a named path parameter from the request. The caller
// provides this so the middleware doesn't couple to a specific router.
type PathParamExtractor func(r *http.Request, name string) string

// Middleware returns an http.Handler middleware that performs Casbin authorization.
// It is fail-closed: if claims are missing, the authorizer errors, or the
// policy says deny, the request is rejected with 403.
//
// Usage:
//
//	mux.Handle("POST /api/v1/pairing/tokens",
//	    authz.Middleware(enforcer, extractClaims, extractParam, authz.Route{
//	        Action:       "recorder.pair",
//	        ResourceType: "recorders",
//	    })(generateHandler))
func Middleware(
	authz Authorizer,
	claims ClaimsExtractor,
	params PathParamExtractor,
	route Route,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, ok := claims(r.Context())
			if !ok {
				writeJSON(w, http.StatusUnauthorized, "UNAUTHENTICATED", "missing or invalid credentials")
				return
			}

			subject := SubjectRef{Kind: "user", ID: c.UserID, Tenant: c.TenantID}

			resourceID := "*"
			if route.ResourceIDParam != "" && params != nil {
				if id := params(r, route.ResourceIDParam); id != "" {
					resourceID = id
				}
			}
			object := ObjectRef{
				Tenant:       c.TenantID,
				ResourceType: route.ResourceType,
				ResourceID:   resourceID,
			}

			allowed, err := authz.Enforce(r.Context(), subject, object, route.Action)
			if err != nil {
				slog.Error("authz: enforcer error",
					"subject", subject.String(),
					"object", object.String(),
					"action", route.Action,
					"error", err,
				)
				writeJSON(w, http.StatusForbidden, "FORBIDDEN", "authorization check failed")
				return
			}
			if !allowed {
				writeJSON(w, http.StatusForbidden, "FORBIDDEN", "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAction is a convenience wrapper that builds a Middleware for a
// single action on a type-level resource (no specific resource ID).
func RequireAction(authz Authorizer, claims ClaimsExtractor, action, resourceType string) func(http.Handler) http.Handler {
	return Middleware(authz, claims, nil, Route{
		Action:       action,
		ResourceType: resourceType,
	})
}

// RouteTable is an ordered set of (method, path-prefix) → Route mappings.
// It allows a single middleware instance to cover a multiplexed handler.
type RouteTable []RouteEntry

// RouteEntry pairs an HTTP method + path prefix with a Route spec.
type RouteEntry struct {
	Method      string // "GET", "POST", etc. Empty means any method.
	PathPrefix  string // e.g. "/api/v1/pairing/tokens"
	Route       Route
}

// Lookup finds the first matching entry for the given method and path.
func (rt RouteTable) Lookup(method, path string) (Route, bool) {
	for _, e := range rt {
		if e.Method != "" && e.Method != method {
			continue
		}
		if strings.HasPrefix(path, e.PathPrefix) {
			return e.Route, true
		}
	}
	return Route{}, false
}

// TableMiddleware returns a middleware that uses a RouteTable for dynamic
// route→action resolution. Requests that don't match any entry are rejected.
func TableMiddleware(
	authz Authorizer,
	claims ClaimsExtractor,
	params PathParamExtractor,
	table RouteTable,
	log *slog.Logger,
) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route, ok := table.Lookup(r.Method, r.URL.Path)
			if !ok {
				// No route matched — fail-closed.
				log.Warn("authz: no route matched, denying",
					"method", r.Method,
					"path", r.URL.Path,
				)
				writeJSON(w, http.StatusForbidden, "FORBIDDEN", "no authorization rule for this endpoint")
				return
			}

			// Delegate to the per-route middleware logic.
			Middleware(authz, claims, params, route)(next).ServeHTTP(w, r)
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"code":"` + code + `","message":"` + message + `"}`))
}
