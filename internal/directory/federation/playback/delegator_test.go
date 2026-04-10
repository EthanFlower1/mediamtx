package playback

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
	"github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1/kaivuev1connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// --- test doubles ---

// fakeCatalog implements CatalogResolver for tests.
type fakeCatalog struct {
	entries map[string]CatalogEntry
}

func (f *fakeCatalog) ResolveCamera(_ context.Context, cameraID string) (CatalogEntry, error) {
	e, ok := f.entries[cameraID]
	if !ok {
		return CatalogEntry{}, errors.New("not found")
	}
	return e, nil
}

// fakePeerFactory implements PeerClientFactory for tests.
type fakePeerFactory struct {
	clients map[string]*fakePeerClient
}

func (f *fakePeerFactory) ClientForPeer(_ context.Context, peerID string) (kaivuev1connect.FederationPeerServiceClient, error) {
	c, ok := f.clients[peerID]
	if !ok {
		return nil, errors.New("unknown peer")
	}
	return c, nil
}

// fakePeerClient implements the subset of FederationPeerServiceClient we need.
type fakePeerClient struct {
	kaivuev1connect.UnimplementedFederationPeerServiceHandler
	mintResp *kaivuev1.MintStreamURLResponse
	mintErr  error
}

func (f *fakePeerClient) Ping(_ context.Context, _ *connect.Request[kaivuev1.PingRequest]) (*connect.Response[kaivuev1.PingResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

func (f *fakePeerClient) GetJWKS(_ context.Context, _ *connect.Request[kaivuev1.GetJWKSRequest]) (*connect.Response[kaivuev1.GetJWKSResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

func (f *fakePeerClient) ListUsers(_ context.Context, _ *connect.Request[kaivuev1.ListUsersRequest]) (*connect.Response[kaivuev1.ListUsersResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

func (f *fakePeerClient) ListGroups(_ context.Context, _ *connect.Request[kaivuev1.ListGroupsRequest]) (*connect.Response[kaivuev1.ListGroupsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

func (f *fakePeerClient) ListCameras(_ context.Context, _ *connect.Request[kaivuev1.FederationPeerServiceListCamerasRequest]) (*connect.Response[kaivuev1.FederationPeerServiceListCamerasResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

func (f *fakePeerClient) SearchRecordings(_ context.Context, _ *connect.Request[kaivuev1.SearchRecordingsRequest]) (*connect.ServerStreamForClient[kaivuev1.SearchRecordingsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

func (f *fakePeerClient) MintStreamURL(_ context.Context, _ *connect.Request[kaivuev1.MintStreamURLRequest]) (*connect.Response[kaivuev1.MintStreamURLResponse], error) {
	if f.mintErr != nil {
		return nil, f.mintErr
	}
	return connect.NewResponse(f.mintResp), nil
}

var discardLog = slog.New(slog.NewTextHandler(discard{}, nil))

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// --- Delegator tests ---

func TestDelegate_Success(t *testing.T) {
	catalog := &fakeCatalog{
		entries: map[string]CatalogEntry{
			"cam-remote-1": {CameraID: "cam-remote-1", PeerID: "peer-b", RecorderID: "rec-b-1"},
		},
	}
	expiresAt := time.Now().Add(5 * time.Minute)
	peerClient := &fakePeerClient{
		mintResp: &kaivuev1.MintStreamURLResponse{
			Url: "https://rec-b-1.peer-b.example.com/webrtc/cam-remote-1?token=signed-by-peer-b",
			Claims: &kaivuev1.StreamClaims{
				CameraId:   "cam-remote-1",
				RecorderId: "rec-b-1",
				Kind:       1, // LIVE
				ExpiresAt:  timestamppb.New(expiresAt),
			},
			GrantedKind: 1,
		},
	}
	factory := &fakePeerFactory{
		clients: map[string]*fakePeerClient{"peer-b": peerClient},
	}

	d := NewDelegator(catalog, factory, discardLog)

	resp, err := d.Delegate(context.Background(), DelegateRequest{
		CameraID:          "cam-remote-1",
		RequestedKind:     1,
		PreferredProtocol: kaivuev1.StreamProtocol_STREAM_PROTOCOL_WEBRTC,
		UserID:            "user-1",
	})

	require.NoError(t, err)
	assert.Equal(t, "https://rec-b-1.peer-b.example.com/webrtc/cam-remote-1?token=signed-by-peer-b", resp.URL)
	assert.Equal(t, uint32(1), resp.GrantedKind)
	assert.Equal(t, "peer-b", resp.PeerID)
	assert.Equal(t, "cam-remote-1", resp.Claims.CameraId)
}

func TestDelegate_CameraNotInCatalog(t *testing.T) {
	catalog := &fakeCatalog{entries: map[string]CatalogEntry{}}
	factory := &fakePeerFactory{clients: map[string]*fakePeerClient{}}

	d := NewDelegator(catalog, factory, discardLog)

	_, err := d.Delegate(context.Background(), DelegateRequest{
		CameraID:      "nonexistent",
		RequestedKind: 1,
		UserID:        "user-1",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCameraNotFound), "expected ErrCameraNotFound, got: %v", err)
}

func TestDelegate_PeerUnreachable_UnknownPeer(t *testing.T) {
	catalog := &fakeCatalog{
		entries: map[string]CatalogEntry{
			"cam-1": {CameraID: "cam-1", PeerID: "peer-gone", RecorderID: "rec-1"},
		},
	}
	// No client for peer-gone in the factory.
	factory := &fakePeerFactory{clients: map[string]*fakePeerClient{}}

	d := NewDelegator(catalog, factory, discardLog)

	_, err := d.Delegate(context.Background(), DelegateRequest{
		CameraID:      "cam-1",
		RequestedKind: 1,
		UserID:        "user-1",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPeerUnreachable), "expected ErrPeerUnreachable, got: %v", err)
}

func TestDelegate_PeerUnreachable_RPCUnavailable(t *testing.T) {
	catalog := &fakeCatalog{
		entries: map[string]CatalogEntry{
			"cam-1": {CameraID: "cam-1", PeerID: "peer-b", RecorderID: "rec-1"},
		},
	}
	peerClient := &fakePeerClient{
		mintErr: connect.NewError(connect.CodeUnavailable, errors.New("connection refused")),
	}
	factory := &fakePeerFactory{
		clients: map[string]*fakePeerClient{"peer-b": peerClient},
	}

	d := NewDelegator(catalog, factory, discardLog)

	_, err := d.Delegate(context.Background(), DelegateRequest{
		CameraID:      "cam-1",
		RequestedKind: 1,
		UserID:        "user-1",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPeerUnreachable), "expected ErrPeerUnreachable, got: %v", err)
}

func TestDelegate_PermissionDenied(t *testing.T) {
	catalog := &fakeCatalog{
		entries: map[string]CatalogEntry{
			"cam-1": {CameraID: "cam-1", PeerID: "peer-b", RecorderID: "rec-1"},
		},
	}
	peerClient := &fakePeerClient{
		mintErr: connect.NewError(connect.CodePermissionDenied, errors.New("user not authorized for this camera")),
	}
	factory := &fakePeerFactory{
		clients: map[string]*fakePeerClient{"peer-b": peerClient},
	}

	d := NewDelegator(catalog, factory, discardLog)

	_, err := d.Delegate(context.Background(), DelegateRequest{
		CameraID:      "cam-1",
		RequestedKind: 1,
		UserID:        "user-1",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPermissionDenied), "expected ErrPermissionDenied, got: %v", err)
}

func TestDelegate_PeerSaysCameraNotFound(t *testing.T) {
	catalog := &fakeCatalog{
		entries: map[string]CatalogEntry{
			"cam-stale": {CameraID: "cam-stale", PeerID: "peer-b", RecorderID: "rec-1"},
		},
	}
	peerClient := &fakePeerClient{
		mintErr: connect.NewError(connect.CodeNotFound, errors.New("camera cam-stale not found")),
	}
	factory := &fakePeerFactory{
		clients: map[string]*fakePeerClient{"peer-b": peerClient},
	}

	d := NewDelegator(catalog, factory, discardLog)

	_, err := d.Delegate(context.Background(), DelegateRequest{
		CameraID:      "cam-stale",
		RequestedKind: 1,
		UserID:        "user-1",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCameraNotFound), "expected ErrCameraNotFound, got: %v", err)
}

func TestDelegate_EmptyCameraID(t *testing.T) {
	d := NewDelegator(&fakeCatalog{}, &fakePeerFactory{}, discardLog)

	_, err := d.Delegate(context.Background(), DelegateRequest{
		CameraID:      "",
		RequestedKind: 1,
		UserID:        "user-1",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "camera_id is required")
}

func TestDelegate_PeerReturnsEmptyURL(t *testing.T) {
	catalog := &fakeCatalog{
		entries: map[string]CatalogEntry{
			"cam-1": {CameraID: "cam-1", PeerID: "peer-b", RecorderID: "rec-1"},
		},
	}
	peerClient := &fakePeerClient{
		mintResp: &kaivuev1.MintStreamURLResponse{
			Url:         "",
			GrantedKind: 1,
		},
	}
	factory := &fakePeerFactory{
		clients: map[string]*fakePeerClient{"peer-b": peerClient},
	}

	d := NewDelegator(catalog, factory, discardLog)

	_, err := d.Delegate(context.Background(), DelegateRequest{
		CameraID:      "cam-1",
		RequestedKind: 1,
		UserID:        "user-1",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPeerInternal), "expected ErrPeerInternal, got: %v", err)
}

func TestDelegate_PeerInternalError(t *testing.T) {
	catalog := &fakeCatalog{
		entries: map[string]CatalogEntry{
			"cam-1": {CameraID: "cam-1", PeerID: "peer-b", RecorderID: "rec-1"},
		},
	}
	peerClient := &fakePeerClient{
		mintErr: connect.NewError(connect.CodeInternal, errors.New("database crash")),
	}
	factory := &fakePeerFactory{
		clients: map[string]*fakePeerClient{"peer-b": peerClient},
	}

	d := NewDelegator(catalog, factory, discardLog)

	_, err := d.Delegate(context.Background(), DelegateRequest{
		CameraID:      "cam-1",
		RequestedKind: 1,
		UserID:        "user-1",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPeerInternal), "expected ErrPeerInternal, got: %v", err)
}

func TestDelegate_DeadlineExceeded(t *testing.T) {
	catalog := &fakeCatalog{
		entries: map[string]CatalogEntry{
			"cam-1": {CameraID: "cam-1", PeerID: "peer-b", RecorderID: "rec-1"},
		},
	}
	peerClient := &fakePeerClient{
		mintErr: connect.NewError(connect.CodeDeadlineExceeded, errors.New("timeout")),
	}
	factory := &fakePeerFactory{
		clients: map[string]*fakePeerClient{"peer-b": peerClient},
	}

	d := NewDelegator(catalog, factory, discardLog, WithTimeout(100*time.Millisecond))

	_, err := d.Delegate(context.Background(), DelegateRequest{
		CameraID:      "cam-1",
		RequestedKind: 1,
		UserID:        "user-1",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPeerUnreachable), "expected ErrPeerUnreachable for deadline, got: %v", err)
}

func TestDelegate_NetworkError(t *testing.T) {
	catalog := &fakeCatalog{
		entries: map[string]CatalogEntry{
			"cam-1": {CameraID: "cam-1", PeerID: "peer-b", RecorderID: "rec-1"},
		},
	}
	peerClient := &fakePeerClient{
		mintErr: errors.New("dial tcp: connection refused"),
	}
	factory := &fakePeerFactory{
		clients: map[string]*fakePeerClient{"peer-b": peerClient},
	}

	d := NewDelegator(catalog, factory, discardLog)

	_, err := d.Delegate(context.Background(), DelegateRequest{
		CameraID:      "cam-1",
		RequestedKind: 1,
		UserID:        "user-1",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPeerUnreachable), "expected ErrPeerUnreachable for network error, got: %v", err)
}

func TestDelegate_WithPlaybackRange(t *testing.T) {
	catalog := &fakeCatalog{
		entries: map[string]CatalogEntry{
			"cam-1": {CameraID: "cam-1", PeerID: "peer-b", RecorderID: "rec-1"},
		},
	}
	expiresAt := time.Now().Add(5 * time.Minute)
	peerClient := &fakePeerClient{
		mintResp: &kaivuev1.MintStreamURLResponse{
			Url: "https://rec-1.peer-b.example.com/mp4/cam-1?token=signed",
			Claims: &kaivuev1.StreamClaims{
				CameraId:   "cam-1",
				RecorderId: "rec-1",
				Kind:       2, // PLAYBACK
				ExpiresAt:  timestamppb.New(expiresAt),
			},
			GrantedKind: 2,
		},
	}
	factory := &fakePeerFactory{
		clients: map[string]*fakePeerClient{"peer-b": peerClient},
	}

	start := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 10, 1, 0, 0, 0, time.UTC)

	d := NewDelegator(catalog, factory, discardLog)

	resp, err := d.Delegate(context.Background(), DelegateRequest{
		CameraID:          "cam-1",
		RequestedKind:     2, // PLAYBACK
		PreferredProtocol: kaivuev1.StreamProtocol_STREAM_PROTOCOL_MP4,
		PlaybackRange: &kaivuev1.PlaybackRange{
			Start: timestamppb.New(start),
			End:   timestamppb.New(end),
		},
		UserID: "user-1",
	})

	require.NoError(t, err)
	assert.Equal(t, uint32(2), resp.GrantedKind)
	assert.Contains(t, resp.URL, "mp4/cam-1")
}

func TestDelegate_UnauthenticatedTreatedAsPermissionDenied(t *testing.T) {
	catalog := &fakeCatalog{
		entries: map[string]CatalogEntry{
			"cam-1": {CameraID: "cam-1", PeerID: "peer-b", RecorderID: "rec-1"},
		},
	}
	peerClient := &fakePeerClient{
		mintErr: connect.NewError(connect.CodeUnauthenticated, errors.New("invalid token")),
	}
	factory := &fakePeerFactory{
		clients: map[string]*fakePeerClient{"peer-b": peerClient},
	}

	d := NewDelegator(catalog, factory, discardLog)

	_, err := d.Delegate(context.Background(), DelegateRequest{
		CameraID:      "cam-1",
		RequestedKind: 1,
		UserID:        "user-1",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPermissionDenied), "expected ErrPermissionDenied, got: %v", err)
}
