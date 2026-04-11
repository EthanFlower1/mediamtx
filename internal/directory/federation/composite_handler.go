package federation

import (
	"context"

	"connectrpc.com/connect"

	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
	"github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1/kaivuev1connect"
)

// Compile-time check that CompositeHandler satisfies the Connect-Go interface.
var _ kaivuev1connect.FederationPeerServiceHandler = (*CompositeHandler)(nil)

// CompositeHandler delegates federation RPCs to the appropriate sub-handler:
//   - Ping, GetJWKS, ListUsers, ListGroups, ListCameras → RPCHandler
//   - SearchRecordings, MintStreamURL → StreamingHandler
type CompositeHandler struct {
	rpc       *RPCHandler
	streaming *StreamingHandler
}

// NewCompositeHandler constructs a CompositeHandler from validated sub-handlers.
func NewCompositeHandler(rpcCfg RPCConfig, streamCfg StreamingHandlerConfig) (*CompositeHandler, error) {
	rpc, err := NewRPCHandler(rpcCfg)
	if err != nil {
		return nil, err
	}
	streaming, err := NewStreamingHandler(streamCfg)
	if err != nil {
		return nil, err
	}
	return &CompositeHandler{
		rpc:       rpc,
		streaming: streaming,
	}, nil
}

// Ping delegates to RPCHandler.
func (c *CompositeHandler) Ping(
	ctx context.Context,
	req *connect.Request[kaivuev1.PingRequest],
) (*connect.Response[kaivuev1.PingResponse], error) {
	return c.rpc.Ping(ctx, req)
}

// GetJWKS delegates to RPCHandler.
func (c *CompositeHandler) GetJWKS(
	ctx context.Context,
	req *connect.Request[kaivuev1.GetJWKSRequest],
) (*connect.Response[kaivuev1.GetJWKSResponse], error) {
	return c.rpc.GetJWKS(ctx, req)
}

// ListUsers delegates to RPCHandler.
func (c *CompositeHandler) ListUsers(
	ctx context.Context,
	req *connect.Request[kaivuev1.ListUsersRequest],
) (*connect.Response[kaivuev1.ListUsersResponse], error) {
	return c.rpc.ListUsers(ctx, req)
}

// ListGroups delegates to RPCHandler.
func (c *CompositeHandler) ListGroups(
	ctx context.Context,
	req *connect.Request[kaivuev1.ListGroupsRequest],
) (*connect.Response[kaivuev1.ListGroupsResponse], error) {
	return c.rpc.ListGroups(ctx, req)
}

// ListCameras delegates to RPCHandler.
func (c *CompositeHandler) ListCameras(
	ctx context.Context,
	req *connect.Request[kaivuev1.FederationPeerServiceListCamerasRequest],
) (*connect.Response[kaivuev1.FederationPeerServiceListCamerasResponse], error) {
	return c.rpc.ListCameras(ctx, req)
}

// SearchRecordings delegates to StreamingHandler.
func (c *CompositeHandler) SearchRecordings(
	ctx context.Context,
	req *connect.Request[kaivuev1.SearchRecordingsRequest],
	stream *connect.ServerStream[kaivuev1.SearchRecordingsResponse],
) error {
	return c.streaming.SearchRecordings(ctx, req, stream)
}

// MintStreamURL delegates to StreamingHandler.
func (c *CompositeHandler) MintStreamURL(
	ctx context.Context,
	req *connect.Request[kaivuev1.MintStreamURLRequest],
) (*connect.Response[kaivuev1.MintStreamURLResponse], error) {
	return c.streaming.MintStreamURL(ctx, req)
}
