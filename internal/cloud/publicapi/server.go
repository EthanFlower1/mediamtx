package publicapi

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// Config bundles every dependency the public API server needs.
type Config struct {
	// ListenAddr is the TCP listen address, e.g. ":8444".
	ListenAddr string

	// Identity is the OAuth identity provider for bearer token auth.
	Identity auth.IdentityProvider

	// APIKeyStore is the API key persistence layer. Nil disables API key auth.
	// KAI-400 provides the concrete implementation.
	APIKeyStore APIKeyStore

	// TierResolver looks up a tenant's subscription tier.
	// Nil defaults all tenants to TierFree.
	TierResolver func(tenantID string) TenantTier

	// Logger is the structured logger; nil defaults to slog.Default().
	Logger *slog.Logger

	// ShutdownTimeout caps the graceful shutdown window.
	ShutdownTimeout time.Duration

	// CORSAllowedOrigins is the exact origin allow-list.
	CORSAllowedOrigins []string
}

func (c *Config) validate() error {
	if c.ListenAddr == "" {
		return errors.New("publicapi: ListenAddr is required")
	}
	if c.Identity == nil {
		return errors.New("publicapi: Identity is required")
	}
	return nil
}

func (c *Config) defaults() {
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.ShutdownTimeout == 0 {
		c.ShutdownTimeout = 15 * time.Second
	}
}

// Server is the public API HTTP server. It owns the mux, middleware chain,
// and all public service stubs.
type Server struct {
	cfg     Config
	http    *http.Server
	mux     *http.ServeMux
	limiter *TieredRateLimiter
	routes  map[string]RouteAuth
}

// New constructs a Server ready to Start().
func New(cfg Config) (*Server, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	cfg.defaults()

	limiter := NewTieredRateLimiter()
	limiter.TierResolver = cfg.TierResolver

	s := &Server{
		cfg:     cfg,
		mux:     http.NewServeMux(),
		limiter: limiter,
		routes:  PublicRouteAuthorizations(),
	}

	// Health endpoint (outside auth chain).
	s.mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","version":"v1"}`))
	})

	// OpenAPI spec endpoint (outside auth chain).
	s.mux.HandleFunc("/api/v1/openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(OpenAPISpec()))
	})

	// Build the middleware chain for public API routes.
	publicChain := s.buildPublicChain()

	// Mount Connect-Go service stubs.
	for _, svc := range PublicServices {
		for _, m := range svc.methods {
			path := PublicServicePath(svc.service, m)
			s.mux.Handle(path, publicChain(unimplementedPublicHandler(svc.service, m)))
		}
	}

	// Mount REST gateway stubs for downstream integration convenience.
	// These map REST verbs to the Connect paths and return the same
	// error format. Downstream tickets will wire real implementations.
	s.mountRESTGateway(publicChain)

	s.http = &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return s, nil
}

// buildPublicChain assembles the public API middleware stack.
//
// Order (outside to inside):
//  1. API version header
//  2. CORS
//  3. API key auth (tries X-API-Key first)
//  4. OAuth bearer auth (fallback)
//  5. Tiered rate limiting (after auth so we know the tenant)
//  6. Scope enforcement (API key scopes)
//  7. (handler)
func (s *Server) buildPublicChain() Middleware {
	return chain(
		APIVersionMiddleware(),
		publicCORSMiddleware(s.cfg.CORSAllowedOrigins),
		APIKeyAuthMiddleware(s.cfg.APIKeyStore),
		OAuthAuthMiddleware(s.cfg.Identity),
		TieredRateLimitMiddleware(s.limiter),
		ScopeEnforcementMiddleware(s.routes),
	)
}

// publicCORSMiddleware is the CORS middleware for public API endpoints.
func publicCORSMiddleware(origins []string) Middleware {
	if len(origins) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	allowed := make(map[string]struct{}, len(origins))
	for _, o := range origins {
		allowed[o] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if _, ok := allowed[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers",
					"Authorization, Content-Type, X-API-Key, X-Request-Id")
				w.Header().Set("Access-Control-Allow-Methods",
					"GET, POST, PUT, PATCH, DELETE, OPTIONS")
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

// mountRESTGateway registers REST-style routes that map to the Connect
// service methods. This allows integration consumers to use standard
// REST conventions while the wire format remains Connect-Go compatible.
func (s *Server) mountRESTGateway(publicChain Middleware) {
	resources := []struct {
		name    string
		methods map[string]string // HTTP method -> description
	}{
		{"cameras", map[string]string{"GET": "list", "POST": "create"}},
		{"users", map[string]string{"GET": "list", "POST": "create"}},
		{"recordings", map[string]string{"GET": "list"}},
		{"events", map[string]string{"GET": "list"}},
		{"schedules", map[string]string{"GET": "list", "POST": "create"}},
		{"retention-policies", map[string]string{"GET": "list", "POST": "create"}},
		{"integrations", map[string]string{"GET": "list", "POST": "create"}},
	}

	for _, res := range resources {
		res := res // capture
		s.mux.Handle(PublicRESTPath(res.name),
			publicChain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writePublicError(w, http.StatusNotImplemented, "unimplemented",
					"REST gateway for /api/v1/"+res.name+" not yet implemented")
			})))
	}
}

// Start begins listening on the configured address.
func (s *Server) Start(ctx context.Context) error {
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", s.cfg.ListenAddr)
	if err != nil {
		return err
	}
	s.cfg.Logger.Info("publicapi: listening", "addr", s.cfg.ListenAddr)
	if err := s.http.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.ShutdownTimeout)
	defer cancel()
	s.cfg.Logger.Info("publicapi: shutting down")
	return s.http.Shutdown(ctx)
}

// Handler exposes the mux for testing.
func (s *Server) Handler() http.Handler { return s.mux }

// Routes returns the route authorization table for testing.
func (s *Server) Routes() map[string]RouteAuth { return s.routes }
