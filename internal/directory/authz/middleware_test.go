package authz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test doubles ---

type fakeAuthorizer struct {
	allowed bool
	err     error
	// recorded captures the last call for assertions.
	recorded struct {
		subject SubjectRef
		object  ObjectRef
		action  string
	}
}

func (f *fakeAuthorizer) Enforce(_ context.Context, subject SubjectRef, object ObjectRef, action string) (bool, error) {
	f.recorded.subject = subject
	f.recorded.object = object
	f.recorded.action = action
	return f.allowed, f.err
}

type ctxKey struct{}

func testClaimsExtractor(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(ctxKey{}).(Claims)
	return c, ok
}

func withClaims(r *http.Request, c Claims) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxKey{}, c))
}

func testParamExtractor(r *http.Request, name string) string {
	return r.Header.Get("X-Test-Param-" + name)
}

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
})

// --- Middleware tests ---

func TestMiddleware_AllowedPassesThrough(t *testing.T) {
	az := &fakeAuthorizer{allowed: true}
	mw := Middleware(az, testClaimsExtractor, testParamExtractor, Route{
		Action:       "recorder.pair",
		ResourceType: "recorders",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/pairing/tokens", nil)
	req = withClaims(req, Claims{UserID: "u1", TenantID: "t1"})
	rec := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "user:u1@t1", az.recorded.subject.String())
	assert.Equal(t, "t1/recorders/*", az.recorded.object.String())
	assert.Equal(t, "recorder.pair", az.recorded.action)
}

func TestMiddleware_DeniedReturns403(t *testing.T) {
	az := &fakeAuthorizer{allowed: false}
	mw := Middleware(az, testClaimsExtractor, testParamExtractor, Route{
		Action:       "cameras.delete",
		ResourceType: "cameras",
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cameras/cam-1", nil)
	req = withClaims(req, Claims{UserID: "u2", TenantID: "t2"})
	rec := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "FORBIDDEN", body["code"])
	assert.Contains(t, body["message"], "insufficient permissions")
}

func TestMiddleware_MissingClaimsReturns401(t *testing.T) {
	az := &fakeAuthorizer{allowed: true}
	mw := Middleware(az, testClaimsExtractor, nil, Route{
		Action:       "view.live",
		ResourceType: "cameras",
	})

	// No claims in context.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/streams", nil)
	rec := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "UNAUTHENTICATED", body["code"])
}

func TestMiddleware_EnforcerErrorReturns403(t *testing.T) {
	az := &fakeAuthorizer{allowed: false, err: fmt.Errorf("db timeout")}
	mw := Middleware(az, testClaimsExtractor, nil, Route{
		Action:       "audit.read",
		ResourceType: "audit",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil)
	req = withClaims(req, Claims{UserID: "u3", TenantID: "t3"})
	rec := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Contains(t, body["message"], "authorization check failed")
}

func TestMiddleware_ResourceIDFromParam(t *testing.T) {
	az := &fakeAuthorizer{allowed: true}
	mw := Middleware(az, testClaimsExtractor, testParamExtractor, Route{
		Action:          "cameras.edit",
		ResourceType:    "cameras",
		ResourceIDParam: "camera_id",
	})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/cameras/cam-42", nil)
	req = withClaims(req, Claims{UserID: "u4", TenantID: "t4"})
	req.Header.Set("X-Test-Param-camera_id", "cam-42")
	rec := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "t4/cameras/cam-42", az.recorded.object.String())
}

func TestMiddleware_EmptyResourceIDFallsToWildcard(t *testing.T) {
	az := &fakeAuthorizer{allowed: true}
	mw := Middleware(az, testClaimsExtractor, testParamExtractor, Route{
		Action:          "cameras.edit",
		ResourceType:    "cameras",
		ResourceIDParam: "camera_id",
	})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/cameras/", nil)
	req = withClaims(req, Claims{UserID: "u5", TenantID: "t5"})
	// No param header — falls to wildcard.
	rec := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "t5/cameras/*", az.recorded.object.String())
}

// --- RequireAction tests ---

func TestRequireAction_Allowed(t *testing.T) {
	az := &fakeAuthorizer{allowed: true}
	mw := RequireAction(az, testClaimsExtractor, "system.health", "system")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req = withClaims(req, Claims{UserID: "u6", TenantID: "t6"})
	rec := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "system.health", az.recorded.action)
	assert.Equal(t, "t6/system/*", az.recorded.object.String())
}

// --- RouteTable tests ---

func TestRouteTable_Lookup(t *testing.T) {
	table := RouteTable{
		{Method: "POST", PathPrefix: "/api/v1/pairing/tokens", Route: Route{Action: "recorder.pair", ResourceType: "recorders"}},
		{Method: "GET", PathPrefix: "/api/v1/pairing/pending", Route: Route{Action: "recorder.pair", ResourceType: "recorders"}},
		{Method: "POST", PathPrefix: "/api/v1/pairing/pending", Route: Route{Action: "recorder.pair", ResourceType: "recorders"}},
		{Method: "GET", PathPrefix: "/api/v1/cameras", Route: Route{Action: "view.thumbnails", ResourceType: "cameras"}},
	}

	tests := []struct {
		method string
		path   string
		found  bool
		action string
	}{
		{"POST", "/api/v1/pairing/tokens", true, "recorder.pair"},
		{"GET", "/api/v1/pairing/pending", true, "recorder.pair"},
		{"GET", "/api/v1/cameras", true, "view.thumbnails"},
		{"GET", "/api/v1/cameras/cam-1", true, "view.thumbnails"}, // prefix match
		{"DELETE", "/api/v1/unknown", false, ""},
		{"PUT", "/api/v1/pairing/tokens", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			route, ok := table.Lookup(tt.method, tt.path)
			assert.Equal(t, tt.found, ok)
			if ok {
				assert.Equal(t, tt.action, route.Action)
			}
		})
	}
}

func TestTableMiddleware_MatchedRoute(t *testing.T) {
	az := &fakeAuthorizer{allowed: true}
	table := RouteTable{
		{Method: "POST", PathPrefix: "/api/v1/pairing/tokens", Route: Route{Action: "recorder.pair", ResourceType: "recorders"}},
	}
	mw := TableMiddleware(az, testClaimsExtractor, testParamExtractor, table, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/pairing/tokens", nil)
	req = withClaims(req, Claims{UserID: "u7", TenantID: "t7"})
	rec := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "recorder.pair", az.recorded.action)
}

func TestTableMiddleware_NoMatchDenies(t *testing.T) {
	az := &fakeAuthorizer{allowed: true}
	table := RouteTable{
		{Method: "POST", PathPrefix: "/api/v1/pairing/tokens", Route: Route{Action: "recorder.pair", ResourceType: "recorders"}},
	}
	mw := TableMiddleware(az, testClaimsExtractor, testParamExtractor, table, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
	req = withClaims(req, Claims{UserID: "u8", TenantID: "t8"})
	rec := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Contains(t, body["message"], "no authorization rule")
}

// --- SubjectRef/ObjectRef String tests ---

func TestSubjectRef_String(t *testing.T) {
	tests := []struct {
		ref    SubjectRef
		expect string
	}{
		{SubjectRef{Kind: "user", ID: "u1", Tenant: "t1"}, "user:u1@t1"},
		{SubjectRef{Kind: "integrator", ID: "i1", Tenant: "t2"}, "integrator:i1@t2"},
		{SubjectRef{Kind: "federation", ID: "peer-1"}, "federation:peer-1"},
		{SubjectRef{Kind: "unknown"}, ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expect, tt.ref.String())
	}
}

func TestObjectRef_String(t *testing.T) {
	tests := []struct {
		ref    ObjectRef
		expect string
	}{
		{ObjectRef{Tenant: "t1", ResourceType: "cameras", ResourceID: "cam-1"}, "t1/cameras/cam-1"},
		{ObjectRef{Tenant: "t1", ResourceType: "cameras", ResourceID: "*"}, "t1/cameras/*"},
		{ObjectRef{Tenant: "t1", ResourceType: "cameras"}, "t1/cameras/*"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expect, tt.ref.String())
	}
}
