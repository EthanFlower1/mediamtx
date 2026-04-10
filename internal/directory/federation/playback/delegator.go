package playback

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
	"github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1/kaivuev1connect"
)

// Sentinel errors returned by the Delegator.
var (
	// ErrCameraNotFound is returned when the camera_id is not present in any
	// peer's catalog.
	ErrCameraNotFound = errors.New("playback: camera not found in federated catalog")

	// ErrPeerUnreachable is returned when the owning peer cannot be contacted.
	ErrPeerUnreachable = errors.New("playback: owning peer is unreachable")

	// ErrPermissionDenied is returned when the peer refuses to mint a URL for
	// the requesting user.
	ErrPermissionDenied = errors.New("playback: permission denied by owning peer")

	// ErrPeerInternal is returned when the peer returns an unexpected error.
	ErrPeerInternal = errors.New("playback: peer returned an internal error")
)

// CatalogEntry represents a camera record in the federated catalog cache.
// The catalog is populated by periodic ListCameras sync from each peer.
type CatalogEntry struct {
	CameraID   string
	PeerID     string
	RecorderID string
}

// CatalogResolver looks up a camera_id in the local federated catalog cache
// and returns the peer that owns it. Implementations are expected to be
// backed by an in-memory or SQLite cache that is periodically refreshed.
type CatalogResolver interface {
	// ResolveCamera returns the catalog entry for the given camera ID, or an
	// error if the camera is not known to any federated peer.
	ResolveCamera(ctx context.Context, cameraID string) (CatalogEntry, error)
}

// PeerClientFactory creates a FederationPeerServiceClient for a given peer ID.
// The factory is responsible for looking up the peer's base URL, configuring
// mTLS, and attaching the integrator bearer token.
type PeerClientFactory interface {
	// ClientForPeer returns a Connect client configured to talk to the given
	// peer's FederationPeerService. Returns an error if the peer is unknown
	// or its endpoint cannot be resolved.
	ClientForPeer(ctx context.Context, peerID string) (kaivuev1connect.FederationPeerServiceClient, error)
}

// DelegateRequest is the input to Delegate.
type DelegateRequest struct {
	CameraID          string
	RequestedKind     uint32
	PreferredProtocol kaivuev1.StreamProtocol
	PlaybackRange     *kaivuev1.PlaybackRange
	ClientIP          string
	MaxTTLSeconds     int32
	// UserID is the authenticated user making the request. Passed through to
	// the peer so it can enforce permissions.
	UserID string
}

// DelegateResponse is the output of a successful delegation.
type DelegateResponse struct {
	// URL is the fully-formed, signed stream URL pointing at the peer's edge.
	URL string
	// Claims is the decoded claim metadata from the peer's response.
	Claims *kaivuev1.StreamClaims
	// GrantedKind is the actual kind bits the peer granted.
	GrantedKind uint32
	// PeerID identifies which peer minted the URL.
	PeerID string
}

// Delegator orchestrates cross-site playback URL minting.
type Delegator struct {
	catalog CatalogResolver
	peers   PeerClientFactory
	log     *slog.Logger
	timeout time.Duration
}

// Option configures optional Delegator behavior.
type Option func(*Delegator)

// WithTimeout sets the per-peer RPC timeout. Defaults to 10 seconds.
func WithTimeout(d time.Duration) Option {
	return func(del *Delegator) { del.timeout = d }
}

// NewDelegator creates a PlaybackDelegator.
func NewDelegator(catalog CatalogResolver, peers PeerClientFactory, log *slog.Logger, opts ...Option) *Delegator {
	d := &Delegator{
		catalog: catalog,
		peers:   peers,
		log:     log,
		timeout: 10 * time.Second,
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Delegate resolves the camera's owning peer, calls MintStreamURL on it,
// and returns the signed URL. The caller (HTTP handler) can return this
// directly to the client.
func (d *Delegator) Delegate(ctx context.Context, req DelegateRequest) (*DelegateResponse, error) {
	if req.CameraID == "" {
		return nil, fmt.Errorf("playback: camera_id is required")
	}

	// Step 1: resolve camera -> owning peer from catalog cache.
	entry, err := d.catalog.ResolveCamera(ctx, req.CameraID)
	if err != nil {
		d.log.WarnContext(ctx, "camera not found in federated catalog",
			slog.String("camera_id", req.CameraID),
			slog.Any("error", err))
		return nil, ErrCameraNotFound
	}

	d.log.DebugContext(ctx, "resolved camera to peer",
		slog.String("camera_id", req.CameraID),
		slog.String("peer_id", entry.PeerID))

	// Step 2: get a client for the owning peer.
	client, err := d.peers.ClientForPeer(ctx, entry.PeerID)
	if err != nil {
		d.log.ErrorContext(ctx, "failed to create client for peer",
			slog.String("peer_id", entry.PeerID),
			slog.Any("error", err))
		return nil, fmt.Errorf("%w: %s", ErrPeerUnreachable, entry.PeerID)
	}

	// Step 3: call MintStreamURL on the peer with a timeout.
	rpcCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	mintReq := &kaivuev1.MintStreamURLRequest{
		CameraId:          req.CameraID,
		RequestedKind:     req.RequestedKind,
		PreferredProtocol: req.PreferredProtocol,
		PlaybackRange:     req.PlaybackRange,
		ClientIp:          req.ClientIP,
		MaxTtlSeconds:     req.MaxTTLSeconds,
	}

	resp, err := client.MintStreamURL(rpcCtx, connect.NewRequest(mintReq))
	if err != nil {
		return nil, d.classifyError(ctx, entry.PeerID, err)
	}

	msg := resp.Msg
	if msg.Url == "" {
		d.log.ErrorContext(ctx, "peer returned empty URL",
			slog.String("peer_id", entry.PeerID),
			slog.String("camera_id", req.CameraID))
		return nil, fmt.Errorf("%w: empty URL in response", ErrPeerInternal)
	}

	d.log.InfoContext(ctx, "cross-site stream URL minted",
		slog.String("camera_id", req.CameraID),
		slog.String("peer_id", entry.PeerID),
		slog.Uint64("granted_kind", uint64(msg.GrantedKind)))

	return &DelegateResponse{
		URL:         msg.Url,
		Claims:      msg.Claims,
		GrantedKind: msg.GrantedKind,
		PeerID:      entry.PeerID,
	}, nil
}

// classifyError maps a Connect RPC error to one of the package's sentinel
// errors so callers can decide the HTTP status code.
func (d *Delegator) classifyError(ctx context.Context, peerID string, err error) error {
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		switch connectErr.Code() {
		case connect.CodePermissionDenied, connect.CodeUnauthenticated:
			d.log.WarnContext(ctx, "peer denied permission",
				slog.String("peer_id", peerID),
				slog.String("code", connectErr.Code().String()),
				slog.String("message", connectErr.Message()))
			return fmt.Errorf("%w: %s", ErrPermissionDenied, connectErr.Message())
		case connect.CodeNotFound:
			d.log.WarnContext(ctx, "peer says camera not found",
				slog.String("peer_id", peerID),
				slog.String("message", connectErr.Message()))
			return fmt.Errorf("%w: peer reports camera not found", ErrCameraNotFound)
		case connect.CodeUnavailable, connect.CodeDeadlineExceeded:
			d.log.ErrorContext(ctx, "peer unreachable or timed out",
				slog.String("peer_id", peerID),
				slog.String("code", connectErr.Code().String()),
				slog.Any("error", err))
			return fmt.Errorf("%w: %s", ErrPeerUnreachable, peerID)
		default:
			d.log.ErrorContext(ctx, "peer returned unexpected error",
				slog.String("peer_id", peerID),
				slog.String("code", connectErr.Code().String()),
				slog.Any("error", err))
			return fmt.Errorf("%w: %s", ErrPeerInternal, connectErr.Message())
		}
	}

	// Non-Connect error (network failure, DNS timeout, etc.)
	d.log.ErrorContext(ctx, "RPC to peer failed",
		slog.String("peer_id", peerID),
		slog.Any("error", err))
	return fmt.Errorf("%w: %s", ErrPeerUnreachable, err)
}
