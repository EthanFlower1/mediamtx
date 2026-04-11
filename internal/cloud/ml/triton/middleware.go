package triton

import (
	"context"
	"net/http"
)

// TenantHeader is the HTTP/gRPC metadata key for per-tenant routing.
const TenantHeader = "x-kaivue-tenant-id"

// tenantContextKey is the context key for storing the tenant ID.
type tenantContextKey struct{}

// TenantFromContext extracts the tenant ID stored by TenantRoutingMiddleware.
// Returns an empty string if no tenant is present.
func TenantFromContext(ctx context.Context) string {
	v, _ := ctx.Value(tenantContextKey{}).(string)
	return v
}

// TenantRoutingMiddleware extracts the tenant ID from the incoming request
// and propagates it to the Triton inference context. It returns 400 if the
// tenant header is missing.
func TenantRoutingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.Header.Get(TenantHeader)
		if tenantID == "" {
			http.Error(w, "missing required header: "+TenantHeader, http.StatusBadRequest)
			return
		}
		// Store tenant in context and propagate to downstream handlers.
		// The Istio/Envoy sidecar will use this for subset routing.
		ctx := context.WithValue(r.Context(), tenantContextKey{}, tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ModelRoutingMiddleware validates the model name from the URL path and
// ensures it corresponds to a loaded model before forwarding to Triton.
type ModelRoutingMiddleware struct {
	// AllowedModels is the set of model names that are permitted.
	AllowedModels map[string]bool
}

// NewModelRoutingMiddleware creates a middleware that restricts inference
// to the given set of model names.
func NewModelRoutingMiddleware(models []string) *ModelRoutingMiddleware {
	allowed := make(map[string]bool, len(models))
	for _, m := range models {
		allowed[m] = true
	}
	return &ModelRoutingMiddleware{AllowedModels: allowed}
}

// Wrap returns an http.Handler that validates the model name.
func (m *ModelRoutingMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelName := r.URL.Query().Get("model")
		if modelName == "" {
			modelName = r.Header.Get("x-kaivue-model")
		}
		if modelName != "" && !m.AllowedModels[modelName] {
			http.Error(w, "model not found: "+modelName, http.StatusNotFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}
