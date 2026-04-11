package federation_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/directory/federation"
	"github.com/bluenviron/mediamtx/internal/shared/permissions"
	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
	"github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1/kaivuev1connect"
)

// --- composite test stubs (external package, so we redeclare minimal stubs) --

type compositeJWKS struct{}

func (*compositeJWKS) JWKSJSON(_ context.Context) (string, int32, error) {
	return testJWKS, 300, nil
}

type compositePeerExtractor struct{ id string }

func (p *compositePeerExtractor) Extract(_ context.Context) (federation.PeerIdentity, error) {
	return federation.PeerIdentity{PeerDirectoryID: p.id}, nil
}

type compositeRecordingIndex struct{ entries []federation.RecordingEntry }

func (i *compositeRecordingIndex) Search(
	_ context.Context, _ string, _ []string,
	_, _ time.Time, _ []kaivuev1.AIEventKind, _ string, _ int,
) ([]federation.RecordingEntry, error) {
	return i.entries, nil
}

type compositeCameraRegistry struct{}

func (*compositeCameraRegistry) CameraTenantID(_ context.Context, id string) (string, error) {
	if id == "cam-c1" {
		return "tenant-c1", nil
	}
	return "", fmt.Errorf("unknown camera %q", id)
}

func (*compositeCameraRegistry) CameraRecorderBaseURL(_ context.Context, id string) (string, string, error) {
	if id == "cam-c1" {
		return "https://rec.example.com", "rec-c1", nil
	}
	return "", "", fmt.Errorf("unknown camera %q", id)
}

type compositeSigner struct{}

func (*compositeSigner) SignStreamToken(_ federation.StreamTokenClaims) (string, error) {
	return "composite-test-token", nil
}

const compositePeerID = "composite-peer-1"

func compositeStreamEnforcer(t *testing.T) *permissions.Enforcer {
	t.Helper()
	sub := fmt.Sprintf("federation:%s", compositePeerID)
	store := permissions.NewInMemoryStore()
	// Grant playback on all recordings for tenant-c1
	require.NoError(t, store.AddPolicy(permissions.PolicyRule{
		Sub: sub, Obj: "tenant-c1/recordings/*", Act: permissions.ActionViewPlayback, Eft: "allow",
	}))
	// Grant live on camera cam-c1
	require.NoError(t, store.AddPolicy(permissions.PolicyRule{
		Sub: sub, Obj: "tenant-c1/cameras/cam-c1", Act: permissions.ActionViewLive, Eft: "allow",
	}))
	e, err := permissions.NewEnforcer(store, nil)
	require.NoError(t, err)
	return e
}

func newCompositeTestClient(t *testing.T, h *federation.CompositeHandler) kaivuev1connect.FederationPeerServiceClient {
	t.Helper()
	path, handler := kaivuev1connect.NewFederationPeerServiceHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return kaivuev1connect.NewFederationPeerServiceClient(srv.Client(), srv.URL)
}

func newCompositeHandler(t *testing.T) *federation.CompositeHandler {
	t.Helper()
	enforcer := compositeStreamEnforcer(t)
	h, err := federation.NewCompositeHandler(
		federation.RPCConfig{
			ServerVersion: testVersion,
			JWKSProvider:  &compositeJWKS{},
		},
		federation.StreamingHandlerConfig{
			Enforcer: enforcer,
			Index:    &compositeRecordingIndex{},
			Cameras:  &compositeCameraRegistry{},
			Signer:   &compositeSigner{},
			PeerID:   &compositePeerExtractor{id: compositePeerID},
			BaseURL:  "https://directory.example.com",
			NowFunc:  func() time.Time { return time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC) },
		},
	)
	require.NoError(t, err)
	return h
}

// --- Tests -------------------------------------------------------------------

func TestCompositeHandler_Ping_DelegatesToRPC(t *testing.T) {
	h := newCompositeHandler(t)
	client := newCompositeTestClient(t, h)

	resp, err := client.Ping(context.Background(), connect.NewRequest(&kaivuev1.PingRequest{
		Nonce: "composite-nonce",
	}))
	require.NoError(t, err)
	assert.Equal(t, "composite-nonce", resp.Msg.GetNonce())
	assert.Equal(t, testVersion, resp.Msg.GetServerVersion())
}

func TestCompositeHandler_SearchRecordings_DelegatesToStreaming(t *testing.T) {
	h := newCompositeHandler(t)
	client := newCompositeTestClient(t, h)

	stream, err := client.SearchRecordings(context.Background(), connect.NewRequest(
		&kaivuev1.SearchRecordingsRequest{
			Tenant: &kaivuev1.TenantRef{
				Type: kaivuev1.TenantType_TENANT_TYPE_CUSTOMER,
				Id:   "tenant-c1",
			},
		},
	))
	require.NoError(t, err)

	// The stub index returns no entries, so the stream should end cleanly
	// (no CodeUnimplemented error -- that would mean the handler was not wired).
	ok := stream.Receive()
	assert.False(t, ok, "expected no results from empty index")
	// The key assertion: err should be nil (clean EOF), NOT CodeUnimplemented.
	assert.NoError(t, stream.Err())
}

func TestCompositeHandler_MintStreamURL_DelegatesToStreaming(t *testing.T) {
	h := newCompositeHandler(t)
	client := newCompositeTestClient(t, h)

	resp, err := client.MintStreamURL(context.Background(), connect.NewRequest(
		&kaivuev1.MintStreamURLRequest{
			CameraId:    "cam-c1",
			RequestedKind: uint32(kaivuev1.StreamKindBit_STREAM_KIND_BIT_LIVE),
		},
	))
	// The key assertion: NOT CodeUnimplemented.
	require.NoError(t, err)
	assert.Contains(t, resp.Msg.GetUrl(), "composite-test-token")
	assert.Contains(t, resp.Msg.GetUrl(), "cam-c1")
}

func TestCompositeHandler_MintStreamURL_NotUnimplemented(t *testing.T) {
	// Verify the old RPCHandler alone would return Unimplemented for MintStreamURL,
	// but composite does not.
	rpc := newTestHandler(t, &staticJWKSProvider{json: testJWKS, maxAge: 300})
	rpcClient := newTestClient(t, rpc)

	_, err := rpcClient.MintStreamURL(context.Background(), connect.NewRequest(
		&kaivuev1.MintStreamURLRequest{},
	))
	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeUnimplemented, connectErr.Code(),
		"bare RPCHandler should still return Unimplemented for MintStreamURL")
}
