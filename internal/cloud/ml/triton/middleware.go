package triton

import (
	"net/http"
)

// TenantHeader is the HTTP/gRPC metadata key for per-tenant routing.
const TenantHeader = "x-kaivue-tenant-id"

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
		// Propagate tenant to downstream via context header passthrough.
		// The Istio/Envoy sidecar will use this for subset routing.
		next.ServeHTTP(w, r)
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
