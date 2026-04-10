// TODO(KAI-310): replace with generated connectrpc code once buf is wired.
//
// Package apiserver hosts the cloud control-plane HTTP server that fronts
// every Connect-Go service defined in internal/shared/proto/v1. The package
// owns the HTTP mux, middleware stack, health + metrics endpoints, and the
// region-scoped routing seam (architectural seam #9).
//
// This file: configuration structs consumed by New().
package apiserver

import (
	"errors"
	"log/slog"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	"github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/shared/permissions"
	"github.com/bluenviron/mediamtx/internal/cloud/streams"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// RiverClient is the minimal surface the apiserver needs from KAI-234's River
// queue client. Using a tiny local interface keeps the package from importing
// riverqueue directly while KAI-234 is still in flight.
//
// TODO(KAI-234): widen this interface when job-enqueuing handlers land.
type RiverClient interface{}

// streamsIssuer is the minimal surface the apiserver needs from
// streamclaims.Issuer to serve the JWKS endpoint. A local interface avoids
// the circular import path that would arise if we accepted *streamclaims.Issuer
// directly (streams imports streamclaims; apiserver imports streams).
//
// *streamclaims.Issuer satisfies this interface automatically.
type streamsIssuer interface {
	PublicKeySet() ([]byte, error)
}

// TracerHook is an optional OpenTelemetry tracing hook. The concrete
// otel.Tracer is avoided to keep this package dependency-free until KAI
// provides a shared tracing setup; the hook is invoked per request and
// returns a finish func callers run on request completion.
type TracerHook func(method, path string) (finish func(status int))

// RateLimitConfig describes a per-tenant token bucket.
type RateLimitConfig struct {
	// RequestsPerSecond is the steady-state fill rate. Zero disables rate
	// limiting entirely.
	RequestsPerSecond float64
	// Burst is the maximum bucket capacity; a fresh tenant starts full.
	Burst int
}

// RegionRoute is one entry in the canonical region → base URL table used by
// the region-scoped router to emit 307 redirects.
type RegionRoute struct {
	Region  string
	BaseURL string // e.g. "https://api-us-west-2.kaivue.io"
}

// Config bundles every dependency and tunable the server needs. Zero values
// of optional fields are always safe; required fields are validated in New.
type Config struct {
	// --- Required --------------------------------------------------------

	// ListenAddr is the TCP listen address, e.g. ":8443".
	ListenAddr string

	// Region is the region this instance serves. Requests carrying a
	// different value in the X-Kaivue-Region header are 307-redirected to
	// the canonical URL for that region (seam #9).
	Region string

	// DB is the cloud database handle (KAI-218). Readiness probes call
	// Ping against it.
	DB *db.DB

	// Identity is the tenant-aware identity provider (KAI-222 interface,
	// KAI-223 Zitadel adapter). The apiserver takes the interface so it
	// can be swapped for the fake in tests.
	Identity auth.IdentityProvider

	// Enforcer is the Casbin authorization engine (KAI-225).
	Enforcer *permissions.Enforcer

	// AuditRecorder is the audit-log sink wrapped by KAI-233's middleware.
	AuditRecorder audit.Recorder

	// --- Optional --------------------------------------------------------

	// Logger is the structured logger; nil defaults to slog.Default().
	Logger *slog.Logger

	// Tracer is an optional OpenTelemetry hook. Nil means no-op.
	Tracer TracerHook

	// CORSAllowedOrigins is the exact origin allow-list. Empty means CORS
	// is disabled (no Access-Control-* headers are emitted).
	CORSAllowedOrigins []string

	// RateLimit configures the in-memory per-tenant token bucket. Zero
	// RequestsPerSecond disables the limiter.
	RateLimit RateLimitConfig

	// RegionRoutes is the canonical region table used for 307 redirects.
	// If a request arrives with a region header that doesn't match this
	// server's Region, the matching row's BaseURL is used. A missing row
	// yields 421 Misdirected Request.
	RegionRoutes []RegionRoute

	// ShutdownTimeout caps the graceful shutdown window. Zero defaults to
	// 15s; the server's Shutdown refuses to block longer than this.
	ShutdownTimeout time.Duration

	// River is the async job-queue handle (KAI-234). Handlers that need
	// to enqueue work pull it off the Server via Server.River().
	River RiverClient

	// StreamsService is the KAI-255 stream URL minting handler.
	// When non-nil it is mounted at POST /api/v1/streams/request and
	// /.well-known/jwks.json. If nil, those routes are unregistered.
	StreamsService *streams.Service

	// StreamsIssuer is the streamclaims.Issuer used to serve
	// /.well-known/jwks.json. Required when StreamsService is non-nil.
	// Kept separate so the JWKS endpoint can be mounted without full
	// service wiring in environments where only key distribution matters.
	StreamsIssuer streamsIssuer

	// MetricsListenAddr is the TCP address for the dedicated admin metrics
	// listener (KAI-422). Defaults to ":9090". The /metrics endpoint on
	// this listener serves the shared Prometheus registry and is NOT
	// exposed on the main API port. Set to "" to disable.
	MetricsListenAddr string

	// BuildInfo is stamped into the kaivue_build_info gauge at startup.
	// Zero value is safe (version/commit will be empty strings).
	BuildInfo BuildInfoConfig
}

// BuildInfoConfig carries the build metadata for kaivue_build_info (KAI-422).
type BuildInfoConfig struct {
	Version   string
	Commit    string
	GoVersion string
}

func (c *Config) validate() error {
	if c.ListenAddr == "" {
		return errors.New("apiserver: ListenAddr is required")
	}
	if c.Region == "" {
		return errors.New("apiserver: Region is required")
	}
	if c.DB == nil {
		return errors.New("apiserver: DB is required")
	}
	if c.Identity == nil {
		return errors.New("apiserver: Identity is required")
	}
	if c.Enforcer == nil {
		return errors.New("apiserver: Enforcer is required")
	}
	if c.AuditRecorder == nil {
		return errors.New("apiserver: AuditRecorder is required")
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
	if c.MetricsListenAddr == "" {
		c.MetricsListenAddr = ":9090"
	}
}
