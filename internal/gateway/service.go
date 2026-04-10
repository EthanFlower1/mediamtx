package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/logging"
	"github.com/bluenviron/mediamtx/internal/shared/streamclaims"
)

// Config configures a Gateway [Service].
//
// Listen is the address the Gateway HTTP front-end binds to. In v1 this
// is typically the loopback address inside the same process as the
// Directory; an outer reverse proxy terminates TLS on the public edge.
//
// MediaMTXBaseURL is the loopback URL of the co-resident MediaMTX
// sidecar. The Gateway never proxies bytes itself: it 302-redirects
// authenticated clients (or rewrites the URL on the way through) to a
// path inside this MediaMTX, which in turn pulls from the upstream
// Recorder over the mesh via its `source:` config.
//
// Verifier validates incoming stream JWTs. Use streamclaims.NewVerifier
// in production; tests can substitute the fake.
//
// Resolver maps verified claims to a concrete upstream RecorderEndpoint.
//
// Nonce, if non-nil, enforces single-use semantics on the JWT nonce
// claim. KAI-257 supplies the production impl. When nil the Service
// runs in "no replay protection" mode and logs a WARN at startup —
// only acceptable in unit tests.
type Config struct {
	Listen          string
	MediaMTXBaseURL string
	Verifier        StreamVerifier
	Resolver        RecorderResolver
	Nonce           NonceChecker
	Logger          *slog.Logger
}

// Validate checks that all required fields are set. Called by NewService.
func (c *Config) Validate() error {
	switch {
	case c == nil:
		return errors.New("gateway: nil config")
	case c.Listen == "":
		return errors.New("gateway: Config.Listen must be set")
	case c.MediaMTXBaseURL == "":
		return errors.New("gateway: Config.MediaMTXBaseURL must be set")
	case c.Verifier == nil:
		return errors.New("gateway: Config.Verifier must be set")
	case c.Resolver == nil:
		return errors.New("gateway: Config.Resolver must be set")
	}
	if _, err := url.Parse(c.MediaMTXBaseURL); err != nil {
		return fmt.Errorf("gateway: invalid MediaMTXBaseURL: %w", err)
	}
	return nil
}

// Service is the Gateway role's top-level orchestrator. It owns:
//   - the HTTP front-end that verifies stream JWTs and mints upstream URLs
//   - the dynamic MediaMTX sidecar config (rendered from the Resolver)
//
// The zero value is not usable; construct via [NewService].
type Service struct {
	cfg     Config
	log     *slog.Logger
	mediaMTX *url.URL

	mu        sync.RWMutex
	endpoints map[string]RecorderEndpoint // keyed by RecorderID
}

// NewService constructs a Service. It validates cfg and pre-renders the
// initial endpoint table from cfg.Resolver.ListRecorders.
func NewService(ctx context.Context, cfg Config) (*Service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	log = logging.WithComponent(log, "gateway")

	parsed, _ := url.Parse(cfg.MediaMTXBaseURL)
	s := &Service{
		cfg:       cfg,
		log:       log,
		mediaMTX:  parsed,
		endpoints: make(map[string]RecorderEndpoint),
	}

	if cfg.Nonce == nil {
		log.Warn("gateway running without nonce replay protection — KAI-257 not wired")
	}

	if err := s.refreshEndpoints(ctx); err != nil {
		return nil, fmt.Errorf("gateway: initial endpoint refresh: %w", err)
	}
	return s, nil
}

// refreshEndpoints rebuilds the in-memory endpoint table from the
// Resolver. Safe to call repeatedly; takes the write lock.
func (s *Service) refreshEndpoints(ctx context.Context) error {
	eps, err := s.cfg.Resolver.ListRecorders(ctx)
	if err != nil {
		return err
	}
	next := make(map[string]RecorderEndpoint, len(eps))
	for _, e := range eps {
		next[string(e.RecorderID)] = e
	}
	s.mu.Lock()
	s.endpoints = next
	s.mu.Unlock()
	s.log.Info("gateway endpoint table refreshed", slog.Int("count", len(next)))
	return nil
}

// Endpoints returns a snapshot of the current endpoint table. Used by
// the MediaMTX path renderer (RenderMediaMTXPaths) and by tests.
func (s *Service) Endpoints() []RecorderEndpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RecorderEndpoint, 0, len(s.endpoints))
	for _, e := range s.endpoints {
		out = append(out, e)
	}
	return out
}

// Handler returns the http.Handler that fronts the Gateway. Mount it
// under any prefix; the handler is path-agnostic and reads only the
// Authorization header (or ?token= query param).
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/stream", s.handleStream)
	return mux
}

func (s *Service) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleStream is the auth-gated entry point. It:
//
//  1. Extracts the stream JWT from Authorization: Bearer or ?token=
//  2. Verifies it via cfg.Verifier
//  3. Consumes the nonce via cfg.Nonce (if configured)
//  4. Resolves the upstream RecorderEndpoint via cfg.Resolver
//  5. Mints an upstream MediaMTX URL and 307-redirects the client
//
// On any failure it returns the appropriate status code and a JSON
// error body. It NEVER leaks which check failed beyond a generic
// "unauthorized" message — the structured log carries the detail.
func (s *Service) handleStream(w http.ResponseWriter, r *http.Request) {
	tok := extractToken(r)
	if tok == "" {
		s.fail(w, r, http.StatusUnauthorized, "missing token", nil)
		return
	}

	claims, err := s.cfg.Verifier.Verify(tok)
	if err != nil {
		s.fail(w, r, http.StatusUnauthorized, "invalid token", err)
		return
	}

	if s.cfg.Nonce != nil {
		if err := s.cfg.Nonce.CheckAndConsume(r.Context(), claims.Nonce, claims.ExpiresAt.Unix()); err != nil {
			if errors.Is(err, ErrReplay) {
				s.fail(w, r, http.StatusUnauthorized, "replay", err)
				return
			}
			s.fail(w, r, http.StatusInternalServerError, "nonce check failed", err)
			return
		}
	}

	ep, err := s.cfg.Resolver.Resolve(r.Context(), claims)
	switch {
	case errors.Is(err, ErrCameraNotFound), errors.Is(err, ErrRecorderNotFound):
		s.fail(w, r, http.StatusNotFound, "not found", err)
		return
	case err != nil:
		s.fail(w, r, http.StatusBadGateway, "resolver error", err)
		return
	}

	upstream, err := s.MintUpstreamURL(claims, *ep)
	if err != nil {
		s.fail(w, r, http.StatusInternalServerError, "mint url failed", err)
		return
	}

	s.log.Info("gateway stream redirect",
		slog.String("camera", claims.CameraID),
		slog.String("recorder", string(claims.RecorderID)),
		slog.String("proto", string(claims.Protocol)),
		slog.Time("expires_at", claims.ExpiresAt),
	)
	http.Redirect(w, r, upstream, http.StatusTemporaryRedirect)
}

// MintUpstreamURL returns the local MediaMTX URL the client should be
// redirected to for the given verified claims and resolved endpoint.
//
// The upstream MediaMTX has been configured with a `paths:` entry for
// each Recorder endpoint that uses `source: <scheme>://<recorder-host>...`,
// so calling .../<recorder-id>/<path-name> on the local sidecar causes
// it to pull from the upstream Recorder over the mesh.
//
// Exposed (rather than inlined into handleStream) so the routing layer
// in KAI-258 and the integration tests can use the same minting logic.
func (s *Service) MintUpstreamURL(claims *streamclaims.StreamClaims, ep RecorderEndpoint) (string, error) {
	if claims == nil {
		return "", errors.New("gateway: nil claims")
	}
	base := *s.mediaMTX
	// Path layout: /<recorder-id>/<camera-id>[/playback?start=...&end=...]
	pathName := ep.PathName
	if pathName == "" {
		pathName = claims.CameraID
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/" + string(ep.RecorderID) + "/" + pathName

	q := base.Query()
	if claims.Kind.Has(streamclaims.StreamKindPlayback) && claims.PlaybackRange != nil {
		q.Set("start", claims.PlaybackRange.Start.UTC().Format(time.RFC3339Nano))
		q.Set("end", claims.PlaybackRange.End.UTC().Format(time.RFC3339Nano))
	}
	q.Set("proto", string(claims.Protocol))
	base.RawQuery = q.Encode()
	return base.String(), nil
}

// RenderMediaMTXPaths returns a YAML-shaped map of MediaMTX path
// definitions that the sidecar config writer (KAI-259 territory) can
// marshal into the running MediaMTX instance's `paths:` block.
//
// The shape is intentionally untyped (map[string]any) so this package
// does not have to import the mediamtx config types — keeping the
// gateway -> conf coupling at zero. The returned map keys are the
// MediaMTX path names ("<recorder-id>/<camera-id>" or, for whole-Recorder
// passthrough, "<recorder-id>/all"); the values carry `source:` URLs
// pointing at the resolved Recorder over the mesh.
//
// In v1 the gateway only emits a wildcard catch-all per Recorder so a
// new camera does not require a sidecar reload. Per-camera entries can
// be added later when the resolver gains a per-camera enumeration API.
func (s *Service) RenderMediaMTXPaths() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]any, len(s.endpoints))
	for _, ep := range s.endpoints {
		key := string(ep.RecorderID) + "/~^.*$"
		scheme := ep.Scheme
		if scheme == "" {
			scheme = "rtsp"
		}
		out[key] = map[string]any{
			"source":         fmt.Sprintf("%s://%s:%d/$G1", scheme, ep.Host, ep.MediaPort),
			"sourceOnDemand": true,
		}
	}
	return out
}

// extractToken pulls a stream JWT from either the Authorization
// header (Bearer scheme) or a ?token= query parameter.  The query
// fallback exists because <video> elements cannot set custom headers.
func extractToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		const prefix = "Bearer "
		if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
			return strings.TrimSpace(h[len(prefix):])
		}
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

// fail writes a uniform error response and logs the underlying cause
// at WARN. The public body never includes the wrapped err — that goes
// to the structured log only.
func (s *Service) fail(w http.ResponseWriter, r *http.Request, code int, public string, cause error) {
	s.log.Warn("gateway request rejected",
		slog.Int("status", code),
		slog.String("reason", public),
		slog.String("path", r.URL.Path),
		slog.Any("err", cause),
	)
	http.Error(w, public, code)
}
