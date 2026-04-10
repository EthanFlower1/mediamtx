package authwebhook

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AuthRequest is the JSON body MediaMTX sends when authMethod is "http".
// Field names match MediaMTX's internal/auth/manager.go authenticateHTTP.
type AuthRequest struct {
	IP       string `json:"ip"`
	User     string `json:"user"`
	Password string `json:"password"`
	Token    string `json:"token"`
	Action   string `json:"action"`   // "publish", "read", "playback", "api", "metrics", "pprof"
	Path     string `json:"path"`     // e.g. "cam_abc123"
	Protocol string `json:"protocol"` // "rtsp", "webrtc", "hls", "srt"
	ID       string `json:"id"`       // session UUID (nullable in upstream, string here)
	Query    string `json:"query"`    // raw query string from the viewer URL
}

// TokenVerifier validates a viewer JWT and returns the verified claims.
// This is the seam to the shared TokenVerifier (KAI-129) or any test double.
type TokenVerifier interface {
	// Verify validates the token string and returns the subject and tenant_id.
	// Returns an error if the token is invalid, expired, or revoked.
	Verify(ctx context.Context, token string) (subject, tenantID string, err error)
}

// PathResolver maps a MediaMTX path name (e.g. "cam_abc123") to a
// camera ID and tenant ID. This is the seam to the Recorder's camera
// cache (KAI-250 / state.Store).
type PathResolver interface {
	// ResolvePath returns the camera ID and tenant ID for the given
	// MediaMTX path. Returns ("", "", nil) if the path is not a managed
	// camera (fall through to MediaMTX's own auth for system paths).
	// Returns an error only on store failures.
	ResolvePath(ctx context.Context, path string) (cameraID, tenantID string, err error)
}

// ServerConfig configures the auth webhook server.
type ServerConfig struct {
	// ListenAddr is the loopback address to bind. Default: "127.0.0.1:0"
	// (ephemeral port). The caller reads the actual address from Addr().
	ListenAddr string

	// Verifier validates viewer tokens.
	Verifier TokenVerifier

	// Resolver maps paths to cameras/tenants.
	Resolver PathResolver

	// Log is the structured logger.
	Log *slog.Logger

	// AllowedActions controls which MediaMTX actions are checked. Actions
	// not in this set are auto-allowed (e.g. "publish" from the RTSP
	// source is always the Recorder itself).
	// Default: {"read", "playback"}
	AllowedActions map[string]struct{}
}

// Server is the auth webhook HTTP server.
type Server struct {
	cfg      ServerConfig
	listener net.Listener
	srv      *http.Server

	mu      sync.Mutex
	running bool
}

// New creates a Server but does not start it. Call Start() to begin
// accepting connections.
func New(cfg ServerConfig) (*Server, error) {
	if cfg.Verifier == nil {
		return nil, fmt.Errorf("authwebhook: Verifier is required")
	}
	if cfg.Resolver == nil {
		return nil, fmt.Errorf("authwebhook: Resolver is required")
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:0"
	}
	if cfg.AllowedActions == nil {
		cfg.AllowedActions = map[string]struct{}{
			"read":     {},
			"playback": {},
		}
	}

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return nil, fmt.Errorf("authwebhook: listen: %w", err)
	}

	s := &Server{
		cfg:      cfg,
		listener: ln,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleAuth)

	s.srv = &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	return s, nil
}

// Addr returns the listener address. Useful when binding to :0.
func (s *Server) Addr() string {
	return s.listener.Addr().String()
}

// Start begins serving. Non-blocking — runs in a background goroutine.
func (s *Server) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return
	}
	s.running = true
	go func() {
		if err := s.srv.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			s.cfg.Log.Error("authwebhook: serve error", slog.String("error", err.Error()))
		}
	}()
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	return s.srv.Shutdown(ctx)
}

// handleAuth is the single HTTP handler that MediaMTX calls.
func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Auto-allow actions we don't check (e.g. "publish" from the RTSP source).
	if _, check := s.cfg.AllowedActions[req.Action]; !check {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Extract the viewer token. MediaMTX sends it in the "token" field
	// (from query param or RTSP DESCRIBE header) or in "password" (RTSP
	// basic auth where user is empty and password carries the JWT).
	token := req.Token
	if token == "" {
		token = req.Password
	}
	if token == "" {
		// Try query string: ?token=...
		token = extractQueryParam(req.Query, "token")
	}
	if token == "" {
		s.cfg.Log.DebugContext(ctx, "authwebhook: no token provided",
			slog.String("path", req.Path),
			slog.String("ip", req.IP))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Verify the token.
	subject, tokenTenantID, err := s.cfg.Verifier.Verify(ctx, token)
	if err != nil {
		s.cfg.Log.DebugContext(ctx, "authwebhook: token verification failed",
			slog.String("path", req.Path),
			slog.String("ip", req.IP),
			slog.String("error", err.Error()))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Resolve the path to a camera + tenant.
	cameraID, pathTenantID, err := s.cfg.Resolver.ResolvePath(ctx, req.Path)
	if err != nil {
		s.cfg.Log.ErrorContext(ctx, "authwebhook: path resolution error",
			slog.String("path", req.Path),
			slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// If the path is not a managed camera, allow through (system paths,
	// healthcheck, etc.).
	if cameraID == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Tenant isolation: the viewer's tenant must match the camera's tenant.
	if tokenTenantID != pathTenantID {
		s.cfg.Log.WarnContext(ctx, "authwebhook: tenant mismatch",
			slog.String("path", req.Path),
			slog.String("camera_id", cameraID),
			slog.String("token_tenant", tokenTenantID),
			slog.String("path_tenant", pathTenantID),
			slog.String("subject", subject),
			slog.String("ip", req.IP))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	s.cfg.Log.DebugContext(ctx, "authwebhook: authorized",
		slog.String("subject", subject),
		slog.String("camera_id", cameraID),
		slog.String("tenant_id", tokenTenantID),
		slog.String("action", req.Action),
		slog.String("protocol", req.Protocol))

	w.WriteHeader(http.StatusOK)
}

// extractQueryParam extracts a single query parameter value from a raw
// query string without allocating a full url.Values map.
func extractQueryParam(query, key string) string {
	for query != "" {
		var part string
		if idx := strings.IndexByte(query, '&'); idx >= 0 {
			part, query = query[:idx], query[idx+1:]
		} else {
			part, query = query, ""
		}
		if eqIdx := strings.IndexByte(part, '='); eqIdx >= 0 {
			if part[:eqIdx] == key {
				return part[eqIdx+1:]
			}
		}
	}
	return ""
}
