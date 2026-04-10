package streams

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	"github.com/bluenviron/mediamtx/internal/shared/permissions"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
	"github.com/bluenviron/mediamtx/internal/shared/streamclaims"
)

// CameraRegistry is the stub interface for the camera/recorder registry
// (KAI-249). The real implementation will live in internal/cloud/cameras once
// that package lands. Using an interface here means we compile and test without
// that dependency.
//
// TODO(KAI-249): replace this stub with the real registry type once
// internal/cloud/cameras is on main.
type CameraRegistry interface {
	// GetCamera returns the camera record scoped to the given tenant. Returns
	// ErrCameraNotFound when no camera with that ID exists in the tenant.
	// MUST return ErrCameraNotFound (not a different sentinel) so the service
	// can map it to a 404 without leaking cross-tenant existence.
	GetCamera(ctx context.Context, tenantID, cameraID string) (Camera, error)
}

// ErrCameraNotFound is returned by CameraRegistry.GetCamera when the camera
// does not exist within the queried tenant.
var ErrCameraNotFound = errors.New("streams: camera not found")

// Config bundles the dependencies of the Service. All fields are required.
type Config struct {
	// Issuer is the streamclaims.Issuer loaded from the cloud signing key.
	// It is shared across requests and safe for concurrent use.
	Issuer *streamclaims.Issuer

	// Router selects the set of endpoint choices for each request.
	Router *Router

	// CameraRegistry looks up camera + recorder metadata.
	// TODO(KAI-249): real implementation replaces the stub.
	CameraRegistry CameraRegistry

	// Enforcer is the Casbin authorization engine (KAI-225). The service
	// performs an additional fine-grained check on the stream kind on top of
	// the route-level check already performed by the apiserver middleware.
	Enforcer *permissions.Enforcer

	// AuditRecorder is the KAI-233 audit sink. Every mint attempt (including
	// denied and failed ones) writes a record here.
	AuditRecorder audit.Recorder

	// DirectoryID is the stable identifier for this cloud Directory instance.
	// Embedded in every StreamClaims JWT so Recorders can verify the issuer.
	DirectoryID string

	// Logger is the structured logger; nil defaults to slog.Default().
	Logger *slog.Logger
}

func (c *Config) validate() error {
	switch {
	case c.Issuer == nil:
		return errors.New("streams: Config.Issuer is required")
	case c.Router == nil:
		return errors.New("streams: Config.Router is required")
	case c.CameraRegistry == nil:
		return errors.New("streams: Config.CameraRegistry is required")
	case c.Enforcer == nil:
		return errors.New("streams: Config.Enforcer is required")
	case c.AuditRecorder == nil:
		return errors.New("streams: Config.AuditRecorder is required")
	case c.DirectoryID == "":
		return errors.New("streams: Config.DirectoryID is required")
	}
	return nil
}

// Service is the stream URL minting handler. It implements http.Handler and
// is mounted at POST /api/v1/streams/request by the apiserver.
//
// Every call to ServeHTTP:
//  1. Authenticates (via already-attached claims from apiserver auth middleware)
//  2. Authorises the stream kind via Casbin
//  3. Looks up the camera + recorder (tenant-scoped)
//  4. Selects endpoint routes via Router
//  5. Mints a StreamClaims JWT (RS256, cloud signing key) per endpoint
//  6. Writes an audit entry (allow or deny)
//  7. Increments Prometheus metrics
//
// Fail-closed: any error in token construction returns 500, never a partial
// response. The architectural seam: signing happens here; verification at the
// Recorder/Gateway.
type Service struct {
	cfg     Config
	log     *slog.Logger
	metrics *serviceMetrics
}

// serviceMetrics holds the Prometheus-style atomic counters for this service.
// Using plain atomics keeps the package dependency-free from
// prometheus/client_golang until KAI-421 wires the shared metrics registry.
type serviceMetrics struct {
	requestTotal    atomic.Uint64
	requestOK       atomic.Uint64
	requestDenied   atomic.Uint64
	requestError    atomic.Uint64
	endpointsLAN    atomic.Uint64
	endpointsPublic atomic.Uint64
	endpointsRelay  atomic.Uint64
	// ttlBuckets approximates a histogram: index i counts tokens whose TTL
	// falls in [(i)*30s, (i+1)*30s). Index 9 is the ≥270s bucket.
	ttlBuckets [10]atomic.Uint64
}

func (m *serviceMetrics) recordTTL(d time.Duration) {
	secs := int(d.Seconds())
	idx := secs / 30
	if idx >= len(m.ttlBuckets) {
		idx = len(m.ttlBuckets) - 1
	}
	m.ttlBuckets[idx].Add(1)
}

// NewService constructs and validates a Service.
func NewService(cfg Config) (*Service, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Service{cfg: cfg, log: log, metrics: &serviceMetrics{}}, nil
}

// MintRequest is the JSON body for POST /api/v1/streams/request.
type MintRequest struct {
	CameraID      string         `json:"camera_id"`
	Kind          string         `json:"kind"`     // "live" | "playback" | "snapshot" | "audio_talkback"
	Protocol      string         `json:"protocol"` // "webrtc" | "ll-hls" | "hls" | "mjpeg" | "rtsp-tls"
	PlaybackRange *PlaybackRange `json:"playback_range,omitempty"`
}

// PlaybackRange is the JSON representation of a closed time interval.
type PlaybackRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// MintResponse is the JSON body returned on success.
type MintResponse struct {
	StreamID   string             `json:"stream_id"`
	TTLSeconds int                `json:"ttl_seconds"`
	Endpoints  []EndpointResponse `json:"endpoints"`
}

// EndpointResponse is one element of MintResponse.Endpoints.
type EndpointResponse struct {
	Kind               EndpointKind `json:"kind"`
	URL                string       `json:"url"`   // base URL + ?token=<jwt>
	Token              string       `json:"token"` // raw JWT, for clients that prefer header auth
	EstimatedLatencyMS int          `json:"estimated_latency_ms"`
}

// ServeHTTP implements http.Handler for POST /api/v1/streams/request.
// It is mounted by the apiserver after the full middleware chain
// (auth + permission + audit + rate-limit) has already run.
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.metrics.requestTotal.Add(1)

	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// --- 1. Extract verified claims from context (set by auth middleware) ---
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		// Should never reach here — the auth middleware already enforced this.
		s.metrics.requestError.Add(1)
		writeJSONError(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	// --- 2. Parse and validate request body ---
	var req MintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.metrics.requestError.Add(1)
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.CameraID == "" {
		s.metrics.requestError.Add(1)
		writeJSONError(w, http.StatusBadRequest, "camera_id is required")
		return
	}

	kind, err := parseStreamKind(req.Kind)
	if err != nil {
		s.metrics.requestError.Add(1)
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	proto, err := parseProtocol(req.Protocol)
	if err != nil {
		s.metrics.requestError.Add(1)
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Playback range required for playback kind.
	if kind.Has(streamclaims.StreamKindPlayback) && req.PlaybackRange == nil {
		s.metrics.requestError.Add(1)
		writeJSONError(w, http.StatusBadRequest, "playback_range is required for kind=playback")
		return
	}
	if !kind.Has(streamclaims.StreamKindPlayback) && req.PlaybackRange != nil {
		s.metrics.requestError.Add(1)
		writeJSONError(w, http.StatusBadRequest, "playback_range must be omitted for non-playback kinds")
		return
	}

	// --- 3. Casbin permission check for stream kind ---
	//
	// The apiserver middleware already enforced "streams.mint" at the route
	// level. We additionally check the kind-specific action so that a user
	// with "streams.mint" but not "view.live" cannot mint a live token.
	tenantRef := claims.TenantRef
	subj := permissions.SubjectFromClaims(*claims)

	// Audio talkback requires both view.live AND audio.talkback.
	if kind.Has(streamclaims.StreamKindAudioTalkback) {
		obj := permissions.NewObjectAll(tenantRef, "streams")
		allowed, enforceErr := s.cfg.Enforcer.Enforce(r.Context(), subj, obj, permissions.ActionAudioTalkback)
		if enforceErr != nil {
			s.metrics.requestError.Add(1)
			s.recordAudit(r.Context(), claims, req.CameraID, audit.ResultError, "audio.talkback enforce error")
			writeJSONError(w, http.StatusInternalServerError, "authorization error")
			return
		}
		if !allowed {
			s.metrics.requestDenied.Add(1)
			s.recordAudit(r.Context(), claims, req.CameraID, audit.ResultDeny, "audio.talkback denied")
			writeJSONError(w, http.StatusForbidden, "forbidden: audio.talkback permission required")
			return
		}
	}

	kindAction, actionErr := kindToAction(kind)
	if actionErr != nil {
		s.metrics.requestError.Add(1)
		writeJSONError(w, http.StatusBadRequest, actionErr.Error())
		return
	}

	{
		obj := permissions.NewObjectAll(tenantRef, "streams")
		allowed, enforceErr := s.cfg.Enforcer.Enforce(r.Context(), subj, obj, kindAction)
		if enforceErr != nil {
			s.metrics.requestError.Add(1)
			s.recordAudit(r.Context(), claims, req.CameraID, audit.ResultError, "kind enforce error")
			writeJSONError(w, http.StatusInternalServerError, "authorization error")
			return
		}
		if !allowed {
			s.metrics.requestDenied.Add(1)
			s.recordAudit(r.Context(), claims, req.CameraID, audit.ResultDeny, "kind denied: "+kindAction)
			writeJSONError(w, http.StatusForbidden, "forbidden: "+kindAction+" permission required")
			return
		}
	}

	// --- 4. Camera registry lookup (tenant-scoped — seam #1) ---
	//
	// TODO(KAI-249): real lookup goes through internal/cloud/cameras once
	// that package is on main. The stub CameraRegistry is injected via Config.
	cam, camErr := s.cfg.CameraRegistry.GetCamera(r.Context(), tenantRef.ID, req.CameraID)
	if errors.Is(camErr, ErrCameraNotFound) {
		// Return 404 regardless of whether the camera exists in another
		// tenant — don't leak cross-tenant existence.
		s.metrics.requestError.Add(1)
		s.recordAudit(r.Context(), claims, req.CameraID, audit.ResultError, "camera not found")
		writeJSONError(w, http.StatusNotFound, "camera not found")
		return
	}
	if camErr != nil {
		s.metrics.requestError.Add(1)
		s.recordAudit(r.Context(), claims, req.CameraID, audit.ResultError, "camera lookup error")
		s.log.Error("camera registry lookup failed",
			slog.String("camera_id", req.CameraID),
			slog.String("tenant_id", tenantRef.ID),
			slog.Any("error", camErr),
		)
		writeJSONError(w, http.StatusInternalServerError, "failed to resolve camera")
		return
	}

	// --- 5. Choose routing endpoints ---
	clientInfo := ClientInfo{SourceIP: parseSourceIP(r)}
	endpointChoices := s.cfg.Router.ChooseEndpoints(clientInfo, cam)

	// --- 6. Mint one JWT per endpoint (fail-closed: all or nothing) ---
	expiresAt := time.Now().UTC().Add(streamclaims.MaxTTL)

	var pbRange *streamclaims.TimeRange
	if req.PlaybackRange != nil {
		pbRange = &streamclaims.TimeRange{
			Start: req.PlaybackRange.Start,
			End:   req.PlaybackRange.End,
		}
	}

	// Generate a unique stream ID that groups the per-endpoint tokens.
	streamID, idErr := streamclaims.GenerateNonce()
	if idErr != nil {
		s.metrics.requestError.Add(1)
		s.recordAudit(r.Context(), claims, req.CameraID, audit.ResultError, "stream id generation failed")
		writeJSONError(w, http.StatusInternalServerError, "failed to generate stream id")
		return
	}

	endpoints := make([]EndpointResponse, 0, len(endpointChoices))
	for _, choice := range endpointChoices {
		nonce, nonceErr := streamclaims.GenerateNonce()
		if nonceErr != nil {
			s.metrics.requestError.Add(1)
			s.recordAudit(r.Context(), claims, req.CameraID, audit.ResultError, "nonce generation failed")
			s.log.Error("nonce generation failed",
				slog.String("camera_id", req.CameraID),
				slog.Any("error", nonceErr),
			)
			writeJSONError(w, http.StatusInternalServerError, "failed to generate token nonce")
			return
		}

		sc := streamclaims.StreamClaims{
			UserID:        claims.UserID,
			TenantRef:     tenantRef,
			CameraID:      cam.ID,
			RecorderID:    auth.RecorderID(cam.RecorderID),
			DirectoryID:   s.cfg.DirectoryID,
			Kind:          kind,
			Protocol:      proto,
			PlaybackRange: pbRange,
			ExpiresAt:     expiresAt,
			Nonce:         nonce,
		}

		signed, signErr := s.cfg.Issuer.Sign(sc)
		if signErr != nil {
			// Fail-closed: any signing failure is a hard 500. Never return
			// a partial response with some endpoints but not others.
			s.metrics.requestError.Add(1)
			s.recordAudit(r.Context(), claims, req.CameraID, audit.ResultError, "token signing failed")
			s.log.Error("stream token signing failed",
				slog.String("camera_id", req.CameraID),
				slog.String("endpoint_kind", string(choice.Kind)),
				slog.Any("error", signErr),
			)
			writeJSONError(w, http.StatusInternalServerError, "failed to mint stream token")
			return
		}

		endpointURL := fmt.Sprintf("%s?token=%s", choice.BaseURL, signed)
		endpoints = append(endpoints, EndpointResponse{
			Kind:               choice.Kind,
			URL:                endpointURL,
			Token:              signed,
			EstimatedLatencyMS: choice.EstimatedLatencyMS,
		})

		// Increment per-endpoint-kind metrics.
		switch choice.Kind {
		case EndpointKindLANDirect:
			s.metrics.endpointsLAN.Add(1)
		case EndpointKindSelfHostedPublic:
			s.metrics.endpointsPublic.Add(1)
		case EndpointKindManagedRelay:
			s.metrics.endpointsRelay.Add(1)
		}
	}

	s.metrics.requestOK.Add(1)
	s.metrics.recordTTL(streamclaims.MaxTTL)

	s.recordAudit(r.Context(), claims, req.CameraID, audit.ResultAllow, "")

	s.log.Info("stream URL minted",
		slog.String("stream_id", streamID),
		slog.String("camera_id", req.CameraID),
		slog.String("tenant_id", tenantRef.ID),
		slog.String("user_id", string(claims.UserID)),
		slog.Int("endpoints", len(endpoints)),
	)

	resp := MintResponse{
		StreamID:   streamID,
		TTLSeconds: int(streamclaims.MaxTTL.Seconds()),
		Endpoints:  endpoints,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// MetricsText emits the service's Prometheus exposition-format metrics. It is
// called by the apiserver's /metrics handler extension point (KAI-421 will
// wire a proper registry).
func (s *Service) MetricsText(w http.ResponseWriter) {
	m := s.metrics
	write := func(name, help, typ string, val uint64) {
		_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n%s %d\n",
			name, help, name, typ, name, val)
	}
	writeLabeled := func(name string, val uint64) {
		_, _ = fmt.Fprintf(w, "%s %d\n", name, val)
	}
	write("kaivue_streams_request_total", "Total stream URL mint requests.", "counter",
		m.requestTotal.Load())
	writeLabeled(`kaivue_streams_request_result_total{result="ok"}`, m.requestOK.Load())
	writeLabeled(`kaivue_streams_request_result_total{result="denied"}`, m.requestDenied.Load())
	writeLabeled(`kaivue_streams_request_result_total{result="error"}`, m.requestError.Load())
	write(`kaivue_streams_endpoints_returned_total{kind="lan_direct"}`,
		"LAN-direct endpoints minted.", "counter", m.endpointsLAN.Load())
	write(`kaivue_streams_endpoints_returned_total{kind="self_hosted_public"}`,
		"Self-hosted public endpoints minted.", "counter", m.endpointsPublic.Load())
	write(`kaivue_streams_endpoints_returned_total{kind="managed_relay"}`,
		"Managed-relay endpoints minted.", "counter", m.endpointsRelay.Load())
}

// --- helpers ----------------------------------------------------------------

// claimsFromContext extracts the verified auth.Claims placed by the apiserver
// auth middleware. Duplicated from apiserver/context.go to avoid a circular
// import — the apiserver package imports us, so we cannot import it back.
//
// The context key is the same package-private value used by apiserver; it is
// the responsibility of apiserver/context.go to keep that key stable.
//
// TODO(KAI-310): when Connect-Go interceptors replace the hand-rolled
// middleware chain, switch to a shared claims context helper.
func claimsFromContext(ctx context.Context) (*auth.Claims, bool) {
	// We piggyback on the apiserver's context key by importing the public
	// accessor. The apiserver package exports ClaimsFromContext for exactly
	// this purpose; we import that function here to avoid duplicating the
	// context-key logic.
	//
	// Import cycle guard: apiserver imports cloud/streams (to mount us), so
	// we cannot import apiserver. Instead, we accept claims via a
	// constructor-injected extractor or the context key value directly.
	//
	// Practical solution: require the caller (the apiserver) to embed the
	// claims in a well-known key accessible via the auth package. Until
	// KAI-310 arrives, the apiserver injects claims via its own context key;
	// we work around the cycle by using a wrapper registered at construction.
	//
	// For now: callers must wrap this handler with injectClaimsExtractor
	// (see Handler() method below) or pass claims through the adapter.
	c, ok := ctx.Value(claimsCtxKey{}).(*auth.Claims)
	return c, ok && c != nil
}

// claimsCtxKey is a private key type so packages outside this one cannot
// accidentally collide. The apiserver populates this via ClaimsAdapter before
// delegating to ServeHTTP.
type claimsCtxKey struct{}

// ClaimsAdapter wraps h so that the auth.Claims from the apiserver's context
// key are re-injected under this package's own key. This breaks the import
// cycle (streams cannot import apiserver, but apiserver CAN import streams
// and can call this adapter with the claims it already has on the context).
//
// Usage in apiserver:
//
//	s.mux.Handle(path, claimsChain(streams.ClaimsAdapter(svc)))
//
// where claimsChain is the existing Connect middleware chain.
func ClaimsAdapter(upstream *auth.Claims, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), claimsCtxKey{}, upstream)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Handler returns an http.Handler that wraps ServeHTTP with a claims injection
// shim that reads from the apiserver context key. It is the function the
// apiserver mounts at POST /api/v1/streams/request:
//
//	mux.Handle("/api/v1/streams/request", svc.Handler(apiserver.ClaimsFromContext))
func (s *Service) Handler(extractClaims func(*http.Request) (*auth.Claims, bool)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r)
		if !ok {
			writeJSONError(w, http.StatusUnauthorized, "missing authentication")
			return
		}
		ctx := context.WithValue(r.Context(), claimsCtxKey{}, claims)
		s.ServeHTTP(w, r.WithContext(ctx))
	})
}

// recordAudit writes one audit entry. Errors are logged but not surfaced —
// a best-effort audit log failure must not block the primary response.
func (s *Service) recordAudit(
	ctx context.Context,
	claims *auth.Claims,
	cameraID string,
	result audit.Result,
	errCode string,
) {
	entry := audit.Entry{
		TenantID:     claims.TenantRef.ID,
		ActorUserID:  string(claims.UserID),
		ActorAgent:   audit.AgentCloud,
		Action:       "streams.mint",
		ResourceType: "streams",
		ResourceID:   cameraID,
		Result:       result,
		Timestamp:    time.Now().UTC(),
	}
	if errCode != "" && result == audit.ResultError {
		entry.ErrorCode = &errCode
	}
	if recErr := s.cfg.AuditRecorder.Record(ctx, entry); recErr != nil {
		s.log.Warn("audit record failed",
			slog.String("camera_id", cameraID),
			slog.Any("error", recErr),
		)
	}
}

// parseSourceIP extracts the effective client IP from an HTTP request,
// honouring X-Forwarded-For and X-Real-IP as set by a trusted proxy layer.
// Falls back to r.RemoteAddr.
func parseSourceIP(r *http.Request) net.IP {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For may be "client, proxy1, proxy2" — leftmost is client.
		parts := strings.Split(xff, ",")
		ip := net.ParseIP(strings.TrimSpace(parts[0]))
		if ip != nil {
			return ip
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		ip := net.ParseIP(strings.TrimSpace(xri))
		if ip != nil {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return net.ParseIP(r.RemoteAddr)
	}
	return net.ParseIP(host)
}

// parseStreamKind converts the request string into a StreamKind bitfield.
func parseStreamKind(s string) (streamclaims.StreamKind, error) {
	switch strings.ToLower(s) {
	case "live":
		return streamclaims.StreamKindLive, nil
	case "playback":
		return streamclaims.StreamKindPlayback, nil
	case "snapshot":
		return streamclaims.StreamKindSnapshot, nil
	case "audio_talkback":
		// Audio talkback requires live too (per spec §9.1).
		return streamclaims.StreamKindLive | streamclaims.StreamKindAudioTalkback, nil
	default:
		return 0, fmt.Errorf("invalid kind %q: must be live|playback|snapshot|audio_talkback", s)
	}
}

// parseProtocol converts the request string into a Protocol.
func parseProtocol(s string) (streamclaims.Protocol, error) {
	switch strings.ToLower(s) {
	case "webrtc":
		return streamclaims.ProtocolWebRTC, nil
	case "ll-hls":
		return streamclaims.ProtocolLLHLS, nil
	case "hls":
		return streamclaims.ProtocolHLS, nil
	case "mjpeg":
		return streamclaims.ProtocolMJPEG, nil
	case "rtsp-tls":
		return streamclaims.ProtocolRTSPTLS, nil
	default:
		return "", fmt.Errorf("invalid protocol %q: must be webrtc|ll-hls|hls|mjpeg|rtsp-tls", s)
	}
}

// kindToAction returns the Casbin action string for the primary stream kind.
// Audio talkback requires an additional check performed before this call.
func kindToAction(kind streamclaims.StreamKind) (string, error) {
	switch {
	case kind.Has(streamclaims.StreamKindLive):
		return permissions.ActionViewLive, nil
	case kind.Has(streamclaims.StreamKindPlayback):
		return permissions.ActionViewPlayback, nil
	case kind.Has(streamclaims.StreamKindSnapshot):
		return permissions.ActionViewSnapshot, nil
	default:
		return "", fmt.Errorf("cannot determine action for stream kind %d", uint32(kind))
	}
}

// writeJSONError writes a minimal JSON error envelope. It follows the same
// shape as the apiserver's errorEnvelope so clients need only one parser.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": message})
}
