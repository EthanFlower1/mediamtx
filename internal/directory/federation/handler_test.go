package federation_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/directory/federation"
	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
	"github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1/kaivuev1connect"
)

// --- test doubles ---

type staticJWKSProvider struct {
	json   string
	maxAge int32
	err    error
}

func (p *staticJWKSProvider) JWKSJSON(_ context.Context) (string, int32, error) {
	return p.json, p.maxAge, p.err
}

const testVersion = "1.0.0-test"

const testJWKS = `{
  "keys": [
    {
      "kty": "OKP",
      "crv": "Ed25519",
      "kid": "test-key-1",
      "x": "11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo"
    }
  ]
}`

func newTestHandler(t *testing.T, provider federation.RPCJWKSProvider) *federation.RPCHandler {
	t.Helper()
	h, err := federation.NewRPCHandler(federation.RPCConfig{
		ServerVersion: testVersion,
		JWKSProvider:  provider,
	})
	require.NoError(t, err)
	return h
}

func newTestClient(t *testing.T, handler *federation.RPCHandler) kaivuev1connect.FederationPeerServiceClient {
	t.Helper()
	_, h := kaivuev1connect.NewFederationPeerServiceHandler(handler)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return kaivuev1connect.NewFederationPeerServiceClient(http.DefaultClient, ts.URL)
}

// --- NewHandler validation ---

func TestNewRPCHandler_MissingVersion(t *testing.T) {
	_, err := federation.NewRPCHandler(federation.RPCConfig{
		JWKSProvider: &staticJWKSProvider{json: testJWKS, maxAge: 300},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ServerVersion")
}

func TestNewRPCHandler_MissingJWKSProvider(t *testing.T) {
	_, err := federation.NewRPCHandler(federation.RPCConfig{
		ServerVersion: "1.0.0",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "JWKSProvider")
}

// --- Ping RPC ---

func TestPing_EchoesNonce(t *testing.T) {
	h := newTestHandler(t, &staticJWKSProvider{json: testJWKS, maxAge: 300})
	client := newTestClient(t, h)

	resp, err := client.Ping(context.Background(), connect.NewRequest(&kaivuev1.PingRequest{
		Nonce: "test-nonce-42",
	}))
	require.NoError(t, err)
	assert.Equal(t, "test-nonce-42", resp.Msg.GetNonce())
}

func TestPing_ReturnsServerVersion(t *testing.T) {
	h := newTestHandler(t, &staticJWKSProvider{json: testJWKS, maxAge: 300})
	client := newTestClient(t, h)

	resp, err := client.Ping(context.Background(), connect.NewRequest(&kaivuev1.PingRequest{}))
	require.NoError(t, err)
	assert.Equal(t, testVersion, resp.Msg.GetServerVersion())
}

func TestPing_ReturnsServerTime(t *testing.T) {
	h := newTestHandler(t, &staticJWKSProvider{json: testJWKS, maxAge: 300})
	client := newTestClient(t, h)

	resp, err := client.Ping(context.Background(), connect.NewRequest(&kaivuev1.PingRequest{}))
	require.NoError(t, err)
	assert.NotNil(t, resp.Msg.GetServerTime())
	assert.False(t, resp.Msg.GetServerTime().AsTime().IsZero())
}

func TestPing_EmptyNonce(t *testing.T) {
	h := newTestHandler(t, &staticJWKSProvider{json: testJWKS, maxAge: 300})
	client := newTestClient(t, h)

	resp, err := client.Ping(context.Background(), connect.NewRequest(&kaivuev1.PingRequest{}))
	require.NoError(t, err)
	assert.Equal(t, "", resp.Msg.GetNonce())
	assert.NotEmpty(t, resp.Msg.GetServerVersion())
}

// --- GetJWKS RPC ---

func TestGetJWKS_ReturnsValidJWKS(t *testing.T) {
	provider := &staticJWKSProvider{json: testJWKS, maxAge: 300}
	h := newTestHandler(t, provider)
	client := newTestClient(t, h)

	resp, err := client.GetJWKS(context.Background(), connect.NewRequest(&kaivuev1.GetJWKSRequest{}))
	require.NoError(t, err)
	assert.Equal(t, testJWKS, resp.Msg.GetJwksJson())
	assert.Equal(t, int32(300), resp.Msg.GetMaxAgeSeconds())
}

func TestGetJWKS_ReturnsMaxAge(t *testing.T) {
	provider := &staticJWKSProvider{json: testJWKS, maxAge: 600}
	h := newTestHandler(t, provider)
	client := newTestClient(t, h)

	resp, err := client.GetJWKS(context.Background(), connect.NewRequest(&kaivuev1.GetJWKSRequest{}))
	require.NoError(t, err)
	assert.Equal(t, int32(600), resp.Msg.GetMaxAgeSeconds())
}

func TestGetJWKS_ProviderError(t *testing.T) {
	provider := &staticJWKSProvider{err: errors.New("key store unavailable")}
	h := newTestHandler(t, provider)
	client := newTestClient(t, h)

	_, err := client.GetJWKS(context.Background(), connect.NewRequest(&kaivuev1.GetJWKSRequest{}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeInternal, connectErr.Code())
}

func TestGetJWKS_EmptyJWKSReturnsError(t *testing.T) {
	provider := &staticJWKSProvider{json: "", maxAge: 300}
	h := newTestHandler(t, provider)
	client := newTestClient(t, h)

	_, err := client.GetJWKS(context.Background(), connect.NewRequest(&kaivuev1.GetJWKSRequest{}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeInternal, connectErr.Code())
}

// --- Catalog RPCs without peer ID return Unauthenticated ---

func TestListUsers_NoPeerID_ReturnsUnauthenticated(t *testing.T) {
	h := newTestHandler(t, &staticJWKSProvider{json: testJWKS, maxAge: 300})
	client := newTestClient(t, h)

	_, err := client.ListUsers(context.Background(), connect.NewRequest(&kaivuev1.ListUsersRequest{}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestListGroups_NoPeerID_ReturnsUnauthenticated(t *testing.T) {
	h := newTestHandler(t, &staticJWKSProvider{json: testJWKS, maxAge: 300})
	client := newTestClient(t, h)

	_, err := client.ListGroups(context.Background(), connect.NewRequest(&kaivuev1.ListGroupsRequest{}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestListCameras_NoPeerID_ReturnsUnauthenticated(t *testing.T) {
	h := newTestHandler(t, &staticJWKSProvider{json: testJWKS, maxAge: 300})
	client := newTestClient(t, h)

	_, err := client.ListCameras(context.Background(), connect.NewRequest(
		&kaivuev1.FederationPeerServiceListCamerasRequest{},
	))
	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

// --- Unimplemented RPCs ---

func TestSearchRecordings_Unimplemented(t *testing.T) {
	h := newTestHandler(t, &staticJWKSProvider{json: testJWKS, maxAge: 300})
	client := newTestClient(t, h)

	stream, err := client.SearchRecordings(context.Background(), connect.NewRequest(
		&kaivuev1.SearchRecordingsRequest{},
	))
	if err != nil {
		var connectErr *connect.Error
		require.True(t, errors.As(err, &connectErr))
		assert.Equal(t, connect.CodeUnimplemented, connectErr.Code())
		return
	}
	// For server-streaming, the error may come on first Receive.
	ok := stream.Receive()
	assert.False(t, ok)
	err = stream.Err()
	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeUnimplemented, connectErr.Code())
}

func TestMintStreamURL_Unimplemented(t *testing.T) {
	h := newTestHandler(t, &staticJWKSProvider{json: testJWKS, maxAge: 300})
	client := newTestClient(t, h)

	_, err := client.MintStreamURL(context.Background(), connect.NewRequest(
		&kaivuev1.MintStreamURLRequest{},
	))
	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeUnimplemented, connectErr.Code())
}
