package federation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
	"github.com/bluenviron/mediamtx/internal/shared/permissions"
	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
	"github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1/kaivuev1connect"
)

// Ensure Handler implements the connect handler interface at compile time.
var _ kaivuev1connect.FederationPeerServiceHandler = (*Handler)(nil)

// RecordingEntry represents a single recording segment in the local index.
type RecordingEntry struct {
	TenantID    string
	RecorderID  string
	CameraID    string
	SegmentID   string
	StartTime   time.Time
	EndTime     time.Time
	Bytes       int64
	IsEventClip bool
	// AI event IDs associated with this segment.
	EventIDs []string
	// AI event kinds for filtering.
	EventKinds []kaivuev1.AIEventKind
}

// RecordingIndex is the seam for querying the local recording index.
// The real implementation is backed by the Directory's segment_index table.
type RecordingIndex interface {
	// Search returns recording entries matching the given filters.
	// An empty cameraIDs slice means "all cameras". Zero-value times mean
	// "unbounded" on that end. eventKinds filters by AI detection type.
	Search(ctx context.Context, tenantID string, cameraIDs []string,
		startTime, endTime time.Time,
		eventKinds []kaivuev1.AIEventKind,
		query string,
		pageSize int,
	) ([]RecordingEntry, error)
}

// CameraRegistry resolves camera ownership for access-control checks.
type CameraRegistry interface {
	// CameraTenantID returns the tenant ID that owns the given camera, or
	// an error if the camera is unknown.
	CameraTenantID(ctx context.Context, cameraID string) (string, error)
	// CameraRecorderBaseURL returns the base URL for the recorder serving
	// this camera (used to build the stream URL).
	CameraRecorderBaseURL(ctx context.Context, cameraID string) (baseURL string, recorderID string, err error)
}

// StreamSigner signs stream tokens. The real implementation uses the
// Directory's JWKS private key.
type StreamSigner interface {
	// SignStreamToken returns a signed JWT containing the given claims and
	// the URL the client should connect to.
	SignStreamToken(claims StreamTokenClaims) (signedToken string, err error)
}

// StreamTokenClaims carries the fields written into the stream JWT.
type StreamTokenClaims struct {
	UserID       string
	TenantID     string
	CameraID     string
	RecorderID   string
	Kind         uint32
	Protocol     kaivuev1.StreamProtocol
	Nonce        string
	PlaybackFrom *time.Time
	PlaybackTo   *time.Time
	ClientIP     string
	SessionID    string
	IssuedAt     time.Time
	ExpiresAt    time.Time
}

// PeerIdentity is extracted from the request context (mTLS + bearer token)
// by an interceptor upstream of this handler.
type PeerIdentity struct {
	PeerDirectoryID string
}

// PeerIdentityExtractor pulls the authenticated peer identity from context.
type PeerIdentityExtractor interface {
	Extract(ctx context.Context) (PeerIdentity, error)
}

// Handler implements the FederationPeerService RPCs.
type Handler struct {
	kaivuev1connect.UnimplementedFederationPeerServiceHandler

	enforcer *permissions.Enforcer
	index    RecordingIndex
	cameras  CameraRegistry
	signer   StreamSigner
	peerID   PeerIdentityExtractor
	baseURL  string // this Directory's external base URL
	maxTTL   time.Duration
	nowFunc  func() time.Time // injectable clock for tests
}

// HandlerConfig configures Handler construction.
type HandlerConfig struct {
	Enforcer *permissions.Enforcer
	Index    RecordingIndex
	Cameras  CameraRegistry
	Signer   StreamSigner
	PeerID   PeerIdentityExtractor
	BaseURL  string
	MaxTTL   time.Duration
	NowFunc  func() time.Time
}

const (
	defaultMaxTTL   = 5 * time.Minute
	defaultPageSize = 100
	maxPageSize     = 1000
)

// NewHandler creates a federation peer handler.
func NewHandler(cfg HandlerConfig) (*Handler, error) {
	if cfg.Enforcer == nil {
		return nil, fmt.Errorf("federation: enforcer is required")
	}
	if cfg.Index == nil {
		return nil, fmt.Errorf("federation: recording index is required")
	}
	if cfg.Cameras == nil {
		return nil, fmt.Errorf("federation: camera registry is required")
	}
	if cfg.Signer == nil {
		return nil, fmt.Errorf("federation: stream signer is required")
	}
	if cfg.PeerID == nil {
		return nil, fmt.Errorf("federation: peer identity extractor is required")
	}
	ttl := cfg.MaxTTL
	if ttl == 0 {
		ttl = defaultMaxTTL
	}
	nf := cfg.NowFunc
	if nf == nil {
		nf = time.Now
	}
	return &Handler{
		enforcer: cfg.Enforcer,
		index:    cfg.Index,
		cameras:  cfg.Cameras,
		signer:   cfg.Signer,
		peerID:   cfg.PeerID,
		baseURL:  cfg.BaseURL,
		maxTTL:   ttl,
		nowFunc:  nf,
	}, nil
}

// SearchRecordings streams recording hits filtered by time range, camera,
// event kinds, and keyword -- all scoped to the requesting peer's Casbin grants.
func (h *Handler) SearchRecordings(
	ctx context.Context,
	req *connect.Request[kaivuev1.SearchRecordingsRequest],
	stream *connect.ServerStream[kaivuev1.SearchRecordingsResponse],
) error {
	peer, err := h.peerID.Extract(ctx)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("federation: %w", err))
	}

	msg := req.Msg
	if msg.Tenant == nil || msg.Tenant.Id == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("federation: tenant is required"))
	}
	tenantID := msg.Tenant.Id
	tenantRef := auth.TenantRef{Type: tenantTypeFromProto(msg.Tenant.Type), ID: tenantID}

	// Check federation-level read access on the recordings resource.
	sub := permissions.NewFederationSubject(peer.PeerDirectoryID)
	obj := permissions.NewObjectAll(tenantRef, "recordings")
	allowed, err := h.enforcer.Enforce(ctx, sub, obj, permissions.ActionViewPlayback)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("federation: permission check: %w", err))
	}
	if !allowed {
		return connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("federation: peer %q lacks playback access on tenant %q", peer.PeerDirectoryID, tenantID))
	}

	// If camera IDs are specified, verify access to each one.
	if len(msg.CameraIds) > 0 {
		for _, camID := range msg.CameraIds {
			camObj := permissions.NewObject(tenantRef, "cameras", camID)
			ok, err := h.enforcer.Enforce(ctx, sub, camObj, permissions.ActionViewPlayback)
			if err != nil {
				return connect.NewError(connect.CodeInternal, fmt.Errorf("federation: camera permission check: %w", err))
			}
			if !ok {
				return connect.NewError(connect.CodePermissionDenied,
					fmt.Errorf("federation: peer %q lacks access to camera %q", peer.PeerDirectoryID, camID))
			}
		}
	}

	// Parse time range.
	var startTime, endTime time.Time
	if msg.StartTime != nil {
		startTime = msg.StartTime.AsTime()
	}
	if msg.EndTime != nil {
		endTime = msg.EndTime.AsTime()
	}

	pageSize := int(msg.PageSize)
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	entries, err := h.index.Search(ctx, tenantID, msg.CameraIds, startTime, endTime, msg.EventKinds, msg.Query, pageSize)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("federation: search: %w", err))
	}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return connect.NewError(connect.CodeCanceled, err)
		}

		hit := entryToHit(msg.Tenant, entry)
		if err := stream.Send(&kaivuev1.SearchRecordingsResponse{Hit: hit}); err != nil {
			return err
		}
	}

	return nil
}

// MintStreamURL verifies the peer's access to the camera, signs a stream
// token with this Directory's JWKS, and returns a URL the peer's client can
// connect to directly.
func (h *Handler) MintStreamURL(
	ctx context.Context,
	req *connect.Request[kaivuev1.MintStreamURLRequest],
) (*connect.Response[kaivuev1.MintStreamURLResponse], error) {
	peer, err := h.peerID.Extract(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("federation: %w", err))
	}

	msg := req.Msg
	if msg.CameraId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("federation: camera_id is required"))
	}

	// Resolve the camera's tenant.
	cameraTenantID, err := h.cameras.CameraTenantID(ctx, msg.CameraId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("federation: camera %q not found", msg.CameraId))
	}

	// Verify the peer has access to this camera.
	sub := permissions.NewFederationSubject(peer.PeerDirectoryID)
	tenantRef := auth.TenantRef{Type: auth.TenantTypeCustomer, ID: cameraTenantID}
	camObj := permissions.NewObject(tenantRef, "cameras", msg.CameraId)

	// Check live or playback depending on requested kind.
	action := actionForKind(msg.RequestedKind)
	allowed, err := h.enforcer.Enforce(ctx, sub, camObj, action)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("federation: permission check: %w", err))
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("federation: peer %q lacks %s access to camera %q", peer.PeerDirectoryID, action, msg.CameraId))
	}

	// Resolve the recorder serving this camera.
	recBaseURL, recorderID, err := h.cameras.CameraRecorderBaseURL(ctx, msg.CameraId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("federation: resolve recorder: %w", err))
	}

	now := h.nowFunc()

	// Compute TTL: honour the client's requested max, capped by server max.
	ttl := h.maxTTL
	if msg.MaxTtlSeconds > 0 {
		requested := time.Duration(msg.MaxTtlSeconds) * time.Second
		if requested < ttl {
			ttl = requested
		}
	}

	nonce, err := generateNonce()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("federation: nonce: %w", err))
	}

	claims := StreamTokenClaims{
		UserID:     fmt.Sprintf("federation:%s", peer.PeerDirectoryID),
		TenantID:   cameraTenantID,
		CameraID:   msg.CameraId,
		RecorderID: recorderID,
		Kind:       msg.RequestedKind,
		Protocol:   msg.PreferredProtocol,
		Nonce:      nonce,
		ClientIP:   msg.ClientIp,
		IssuedAt:   now,
		ExpiresAt:  now.Add(ttl),
	}

	if msg.PlaybackRange != nil {
		if msg.PlaybackRange.Start != nil {
			t := msg.PlaybackRange.Start.AsTime()
			claims.PlaybackFrom = &t
		}
		if msg.PlaybackRange.End != nil {
			t := msg.PlaybackRange.End.AsTime()
			claims.PlaybackTo = &t
		}
	}

	signedToken, err := h.signer.SignStreamToken(claims)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("federation: sign token: %w", err))
	}

	// Build the stream URL pointing at the recorder.
	streamURL := buildStreamURL(recBaseURL, msg.CameraId, msg.PreferredProtocol, signedToken)

	// Build proto claims for the response.
	protoClaims := claimsToProto(claims, tenantRef)

	return connect.NewResponse(&kaivuev1.MintStreamURLResponse{
		Url:         streamURL,
		Claims:      protoClaims,
		GrantedKind: msg.RequestedKind,
	}), nil
}

// --- helpers -----------------------------------------------------------------

func tenantTypeFromProto(pt kaivuev1.TenantType) auth.TenantType {
	switch pt {
	case kaivuev1.TenantType_TENANT_TYPE_INTEGRATOR:
		return auth.TenantTypeIntegrator
	case kaivuev1.TenantType_TENANT_TYPE_CUSTOMER:
		return auth.TenantTypeCustomer
	default:
		return auth.TenantTypeCustomer
	}
}

func entryToHit(tenant *kaivuev1.TenantRef, e RecordingEntry) *kaivuev1.RecordingHit {
	hit := &kaivuev1.RecordingHit{
		Tenant:           tenant,
		RecorderId:       e.RecorderID,
		CameraId:         e.CameraID,
		SegmentId:        e.SegmentID,
		Bytes:            e.Bytes,
		IsEventClip:      e.IsEventClip,
		MatchingEventIds: e.EventIDs,
	}
	if !e.StartTime.IsZero() {
		hit.StartTime = timestamppb.New(e.StartTime)
	}
	if !e.EndTime.IsZero() {
		hit.EndTime = timestamppb.New(e.EndTime)
	}
	return hit
}

func actionForKind(kind uint32) string {
	// If playback bit (0x2) is set, require playback permission.
	if kind&uint32(kaivuev1.StreamKindBit_STREAM_KIND_BIT_PLAYBACK) != 0 {
		return permissions.ActionViewPlayback
	}
	// Default to live view.
	return permissions.ActionViewLive
}

func generateNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func buildStreamURL(recBaseURL, cameraID string, protocol kaivuev1.StreamProtocol, token string) string {
	base := strings.TrimRight(recBaseURL, "/")
	switch protocol {
	case kaivuev1.StreamProtocol_STREAM_PROTOCOL_WEBRTC:
		return fmt.Sprintf("%s/webrtc/%s?token=%s", base, cameraID, token)
	case kaivuev1.StreamProtocol_STREAM_PROTOCOL_HLS:
		return fmt.Sprintf("%s/hls/%s/index.m3u8?token=%s", base, cameraID, token)
	case kaivuev1.StreamProtocol_STREAM_PROTOCOL_RTSP:
		// Strip any scheme from the base URL for RTSP.
		rtspHost := base
		if idx := strings.Index(rtspHost, "://"); idx >= 0 {
			rtspHost = rtspHost[idx+3:]
		}
		return fmt.Sprintf("rtsp://%s/%s?token=%s", rtspHost, cameraID, token)
	case kaivuev1.StreamProtocol_STREAM_PROTOCOL_MP4:
		return fmt.Sprintf("%s/playback/%s.mp4?token=%s", base, cameraID, token)
	case kaivuev1.StreamProtocol_STREAM_PROTOCOL_JPEG:
		return fmt.Sprintf("%s/snapshot/%s.jpg?token=%s", base, cameraID, token)
	default:
		return fmt.Sprintf("%s/stream/%s?token=%s", base, cameraID, token)
	}
}

func claimsToProto(c StreamTokenClaims, tenantRef auth.TenantRef) *kaivuev1.StreamClaims {
	sc := &kaivuev1.StreamClaims{
		UserId:     c.UserID,
		CameraId:   c.CameraID,
		RecorderId: c.RecorderID,
		Kind:       c.Kind,
		Protocol:   c.Protocol,
		Nonce:      c.Nonce,
		ClientIp:   c.ClientIP,
		SessionId:  c.SessionID,
		TenantRef: &kaivuev1.TenantRef{
			Type: kaivuev1.TenantType_TENANT_TYPE_CUSTOMER,
			Id:   tenantRef.ID,
		},
		IssuedAt:  timestamppb.New(c.IssuedAt),
		ExpiresAt: timestamppb.New(c.ExpiresAt),
	}

	if c.PlaybackFrom != nil && c.PlaybackTo != nil {
		sc.PlaybackRange = &kaivuev1.PlaybackRange{
			Start: timestamppb.New(*c.PlaybackFrom),
			End:   timestamppb.New(*c.PlaybackTo),
		}
	}

	return sc
}
