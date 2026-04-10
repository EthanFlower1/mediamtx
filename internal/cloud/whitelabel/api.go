package whitelabel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ConfigStore persists BrandConfig versions per tenant. Production will back
// this with Postgres (KAI-355 decides the exact table layout); the in-memory
// impl here is sufficient for round-trip tests and for the KAI-354 integrator
// portal to develop against.
type ConfigStore interface {
	Current(ctx context.Context, tenantID string) (*BrandConfig, error)
	Save(ctx context.Context, cfg *BrandConfig) (*BrandConfig, error)
	List(ctx context.Context, tenantID string) ([]*BrandConfig, error)
	GetVersion(ctx context.Context, tenantID string, version int) (*BrandConfig, error)
}

// ErrConfigNotFound is returned by ConfigStore when a tenant or version is
// missing.
var ErrConfigNotFound = errors.New("whitelabel: brand config not found")

// MemoryConfigStore is a simple map-backed implementation suitable for tests
// and the API round-trip acceptance criteria. Version assignment is monotonic
// per tenant.
type MemoryConfigStore struct {
	mu       sync.RWMutex
	versions map[string][]*BrandConfig // tenantID -> versions in ascending order
	nowFn    func() time.Time
}

// NewMemoryConfigStore returns an empty store.
func NewMemoryConfigStore() *MemoryConfigStore {
	return &MemoryConfigStore{
		versions: make(map[string][]*BrandConfig),
		nowFn:    time.Now,
	}
}

// Current returns the latest version for the tenant.
func (s *MemoryConfigStore) Current(_ context.Context, tenantID string) (*BrandConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := s.versions[tenantID]
	if len(v) == 0 {
		return nil, ErrConfigNotFound
	}
	return v[len(v)-1].Clone(), nil
}

// Save assigns the next monotonic version, stamps UpdatedAt, validates, then
// appends to the per-tenant history.
func (s *MemoryConfigStore) Save(_ context.Context, cfg *BrandConfig) (*BrandConfig, error) {
	if cfg == nil {
		return nil, errors.New("whitelabel: nil config")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := len(s.versions[cfg.TenantID]) + 1
	stored := cfg.Clone()
	stored.Version = next
	stored.UpdatedAt = s.nowFn().UTC()
	if err := stored.Validate(); err != nil {
		return nil, err
	}
	s.versions[cfg.TenantID] = append(s.versions[cfg.TenantID], stored)
	return stored.Clone(), nil
}

// List returns every version in ascending order.
func (s *MemoryConfigStore) List(_ context.Context, tenantID string) ([]*BrandConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := s.versions[tenantID]
	out := make([]*BrandConfig, 0, len(v))
	for _, c := range v {
		out = append(out, c.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// GetVersion returns a specific version or ErrConfigNotFound.
func (s *MemoryConfigStore) GetVersion(_ context.Context, tenantID string, version int) (*BrandConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.versions[tenantID] {
		if c.Version == version {
			return c.Clone(), nil
		}
	}
	return nil, ErrConfigNotFound
}

// API wires a ConfigStore + BrandAssetStore to the HTTP surface described in
// KAI-353. It intentionally uses net/http so it can be mounted by any router
// (the cloud stack uses gin; KAI-354 will wrap these handlers).
type API struct {
	Configs ConfigStore
	Assets  BrandAssetStore
}

// NewAPI constructs an API handler set. Either store may be nil in which case
// the affected endpoints will return 503.
func NewAPI(cfgs ConfigStore, assets BrandAssetStore) *API {
	return &API{Configs: cfgs, Assets: assets}
}

// Route describes a single method+path+handler tuple. The caller is expected
// to translate this into whatever router they use (gin, chi, stdlib).
type Route struct {
	Method  string
	Pattern string // e.g. /api/v1/integrators/{id}/brand
	Handler http.HandlerFunc
}

// Routes returns the full set of endpoints owned by KAI-353.
//
//   GET    /api/v1/integrators/{id}/brand
//   PUT    /api/v1/integrators/{id}/brand
//   POST   /api/v1/integrators/{id}/brand/assets/{kind}
//   GET    /api/v1/integrators/{id}/brand/versions
//   GET    /api/v1/integrators/{id}/brand/versions/{version}
func (a *API) Routes() []Route {
	return []Route{
		{http.MethodGet, "/api/v1/integrators/{id}/brand", a.handleGetCurrent},
		{http.MethodPut, "/api/v1/integrators/{id}/brand", a.handlePutCurrent},
		{http.MethodPost, "/api/v1/integrators/{id}/brand/assets/{kind}", a.handlePostAsset},
		{http.MethodGet, "/api/v1/integrators/{id}/brand/versions", a.handleListVersions},
		{http.MethodGet, "/api/v1/integrators/{id}/brand/versions/{version}", a.handleGetVersion},
	}
}

// ServeHTTP dispatches a request to the matching Route. Used in tests and as
// a convenience for callers that don't want to plug Routes() into a real
// router. Matching is intentionally simple — production should use gin or
// chi for correct path routing and middleware.
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for _, rt := range a.Routes() {
		if rt.Method != r.Method {
			continue
		}
		params, ok := matchPattern(rt.Pattern, r.URL.Path)
		if !ok {
			continue
		}
		ctx := context.WithValue(r.Context(), routeParamsKey{}, params)
		rt.Handler.ServeHTTP(w, r.WithContext(ctx))
		return
	}
	http.NotFound(w, r)
}

type routeParamsKey struct{}

func paramsFrom(r *http.Request) map[string]string {
	if v, ok := r.Context().Value(routeParamsKey{}).(map[string]string); ok {
		return v
	}
	return map[string]string{}
}

// matchPattern matches a `/a/{p}/b` style pattern against a request path,
// returning the captured parameters.
func matchPattern(pattern, path string) (map[string]string, bool) {
	pp := strings.Split(strings.Trim(pattern, "/"), "/")
	xp := strings.Split(strings.Trim(path, "/"), "/")
	if len(pp) != len(xp) {
		return nil, false
	}
	out := map[string]string{}
	for i, seg := range pp {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			out[strings.Trim(seg, "{}")] = xp[i]
			continue
		}
		if seg != xp[i] {
			return nil, false
		}
	}
	return out, true
}

func (a *API) handleGetCurrent(w http.ResponseWriter, r *http.Request) {
	if a.Configs == nil {
		writeError(w, http.StatusServiceUnavailable, "config store not configured")
		return
	}
	id := paramsFrom(r)["id"]
	cfg, err := a.Configs.Current(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (a *API) handlePutCurrent(w http.ResponseWriter, r *http.Request) {
	if a.Configs == nil {
		writeError(w, http.StatusServiceUnavailable, "config store not configured")
		return
	}
	id := paramsFrom(r)["id"]
	var cfg BrandConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode: %v", err))
		return
	}
	// TenantID in the body must match the path (defence against cross-tenant
	// writes from a compromised portal session).
	if cfg.TenantID != "" && cfg.TenantID != id {
		writeError(w, http.StatusBadRequest, "tenantId mismatch between path and body")
		return
	}
	cfg.TenantID = id
	// Version is ignored on PUT — the store assigns the next value.
	cfg.Version = 1
	if err := cfg.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	saved, err := a.Configs.Save(r.Context(), &cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (a *API) handlePostAsset(w http.ResponseWriter, r *http.Request) {
	if a.Assets == nil {
		writeError(w, http.StatusServiceUnavailable, "asset store not configured")
		return
	}
	params := paramsFrom(r)
	id := params["id"]
	kind := AssetKind(params["kind"])
	if !kind.Valid() {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid asset kind %q", kind))
		return
	}
	// Cap the request body to 8 MiB to bound memory usage; per-kind limits
	// in validateAsset still apply.
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	filename := r.Header.Get("X-Asset-Filename")
	if filename == "" {
		filename = string(kind)
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("read body: %v", err))
		return
	}
	ref, err := a.Assets.Put(r.Context(), id, kind, filename, strings.NewReader(string(body)))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, ref)
}

func (a *API) handleListVersions(w http.ResponseWriter, r *http.Request) {
	if a.Configs == nil {
		writeError(w, http.StatusServiceUnavailable, "config store not configured")
		return
	}
	id := paramsFrom(r)["id"]
	versions, err := a.Configs.List(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

func (a *API) handleGetVersion(w http.ResponseWriter, r *http.Request) {
	if a.Configs == nil {
		writeError(w, http.StatusServiceUnavailable, "config store not configured")
		return
	}
	params := paramsFrom(r)
	id := params["id"]
	v, err := strconv.Atoi(params["version"])
	if err != nil || v < 1 {
		writeError(w, http.StatusBadRequest, "version must be a positive integer")
		return
	}
	cfg, err := a.Configs.GetVersion(r.Context(), id, v)
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

type errorBody struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}
