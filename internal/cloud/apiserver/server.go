// TODO(KAI-310): replace with generated connectrpc code once buf is wired.
package apiserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	auditmw "github.com/bluenviron/mediamtx/internal/cloud/audit/middleware"
)

// Server is the cloud control-plane HTTP server. It owns an http.Server, a
// mux with every Connect-Go service mounted, and the metrics registry.
type Server struct {
	cfg      Config
	http     *http.Server
	mux      *http.ServeMux
	metrics  *metricsRegistry
	limiter  *rateLimiter
	probes   ReadinessProbes
	routes   map[string]RouteAuthorization
}

// New constructs a Server ready to Start(). It validates the config, applies
// defaults, builds the middleware stack, and mounts every service stub.
func New(cfg Config) (*Server, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	cfg.defaults()

	s := &Server{
		cfg:     cfg,
		mux:     http.NewServeMux(),
		metrics: newMetricsRegistry(),
		limiter: newRateLimiter(cfg.RateLimit),
		probes:  defaultReadinessProbes(cfg.DB, cfg.Identity, cfg.Enforcer),
		routes:  defaultRouteAuthorizations(),
	}

	// ------------------- Health + metrics -------------------
	//
	// Health endpoints live OUTSIDE the full middleware chain because
	// they must respond during an outage that has already taken down
	// the DB or the IdP. Region routing still applies (a probe on the
	// wrong region should tell the orchestrator to point elsewhere).
	s.mux.Handle("/healthz", regionMiddleware(cfg.Region, cfg.RegionRoutes)(livenessHandler()))
	// /readyz delegates through a holder so tests can swap probes
	// after New() without re-registering the mux pattern (http.ServeMux
	// forbids duplicate registration).
	s.mux.Handle("/readyz", regionMiddleware(cfg.Region, cfg.RegionRoutes)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readinessHandler(s.probes).ServeHTTP(w, r)
	})))
	s.mux.Handle("/metrics", metricsHandler(s.metrics))

	// ------------------- Connect-Go services ----------------
	//
	// Each method gets a separate pattern so http.ServeMux routes
	// exactly (no prefix fallthrough). The full middleware chain is
	// wrapped around each method handler.
	chainForConnect := s.buildConnectChain()
	for _, svc := range connectServices {
		for _, m := range svc.methods {
			path := ServicePath(svc.service, m)
			s.mux.Handle(path, chainForConnect(unimplementedHandler(svc.service, m)))
		}
	}

	s.http = &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s, nil
}

// buildConnectChain assembles the canonical middleware stack. The order
// matters and is covered by middleware_test.go: the topmost middleware is
// the outermost wrapper, which sees requests FIRST and responses LAST.
//
// Top-to-bottom = outside-to-inside:
//
//	 1. recovery           — catches panics from everything below
//	 2. metrics            — counts every terminal status
//	 3. request ID         — injects X-Request-Id into ctx + response
//	 4. tracing            — wraps handler with optional OTel hook
//	 5. region routing     — 307 to peer region if X-Kaivue-Region mismatches
//	 6. CORS               — browser preflight + origin allow-list
//	 7. tenant resolution  — best-effort tenant hint for rate limiter
//	 8. rate limiting      — per-tenant token bucket (pre-auth on purpose)
//	 9. auth               — verify bearer token via IdentityProvider
//	10. audit              — KAI-233 middleware: 2xx allow / 403 deny
//	11. permission         — Casbin enforce (a deny flows back through
//	                         audit as ResultDeny because audit wraps it)
//	12. (handler)
func (s *Server) buildConnectChain() Middleware {
	auditMW := auditmw.New(auditmw.Config{
		Recorder:  s.cfg.AuditRecorder,
		Principal: auditPrincipalExtractor,
		Resolve:   auditActionResolver(s.routes),
	})
	// Note on ordering: audit MUST wrap permission so that a 403 written
	// by the permission middleware is still observed by the audit
	// status-recorder on the way back up the stack. Put another way: the
	// audit middleware needs to be OUTSIDE permission, not inside it,
	// otherwise the 403 is written before audit's ServeHTTP returns.
	return chain(
		recoveryMiddleware(),
		metricsMiddleware(s.metrics),
		requestIDMiddleware(),
		tracingMiddleware(s.cfg.Tracer),
		regionMiddleware(s.cfg.Region, s.cfg.RegionRoutes),
		corsMiddleware(s.cfg.CORSAllowedOrigins),
		tenantMiddleware(),
		rateLimitMiddleware(s.limiter),
		authMiddleware(s.cfg.Identity),
		auditMW,
		permissionMiddleware(s.cfg.Enforcer, s.routes),
	)
}

// Start begins listening. It blocks until Shutdown is called or the
// underlying listener errors out. The passed context is NOT the one used
// for graceful shutdown — call Shutdown(ctx) explicitly.
func (s *Server) Start(ctx context.Context) error {
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", s.cfg.ListenAddr)
	if err != nil {
		return err
	}
	s.cfg.Logger.Info("apiserver: listening",
		"addr", s.cfg.ListenAddr,
		"region", s.cfg.Region,
	)
	if err := s.http.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown gracefully stops the server. It honours the parent ctx's
// deadline AND the configured ShutdownTimeout, whichever expires first.
func (s *Server) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.ShutdownTimeout)
	defer cancel()
	s.cfg.Logger.Info("apiserver: shutting down")
	return s.http.Shutdown(ctx)
}

// Handler exposes the fully-assembled mux. Tests use this to hit handlers
// via httptest.Server without standing up a real listener.
func (s *Server) Handler() http.Handler { return s.mux }

// River returns the async job queue handle. Returns nil if KAI-234 hasn't
// wired one in yet; handlers must nil-check before enqueuing.
func (s *Server) River() RiverClient { return s.cfg.River }

// SetReadinessProbes overrides the default probes. Used in tests to
// simulate a DB outage and assert /readyz returns 503. The holder closure
// wired in New() reads s.probes on every request so this swap is live.
func (s *Server) SetReadinessProbes(p ReadinessProbes) {
	s.probes = p
}
