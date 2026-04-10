package federation

import (
	"context"
	"errors"
	"log/slog"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
	"github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1/kaivuev1connect"
)

// JWKSProvider returns the current JWKS JSON document for this Directory
// instance. Implementations typically serialize the signing key set from
// the auth subsystem.
type JWKSProvider interface {
	// JWKSJSON returns the raw JWKS JSON and a cache lifetime hint in seconds.
	JWKSJSON(ctx context.Context) (jwksJSON string, maxAgeSeconds int32, err error)
}

// Config configures the federation peer service handler.
type Config struct {
	// ServerVersion is the software version string returned by Ping.
	ServerVersion string

	// JWKSProvider returns this Directory's JWKS document.
	JWKSProvider JWKSProvider

	// Logger is the structured logger. Nil defaults to slog.Default().
	Logger *slog.Logger
}

func (c *Config) validate() error {
	if c.ServerVersion == "" {
		return errors.New("federation: ServerVersion is required")
	}
	if c.JWKSProvider == nil {
		return errors.New("federation: JWKSProvider is required")
	}
	return nil
}

// Handler implements the FederationPeerServiceHandler interface from
// Connect-Go generated code. Ping and GetJWKS are fully implemented;
// all other RPCs embed UnimplementedFederationPeerServiceHandler to
// return CodeUnimplemented, ready for KAI-465 and KAI-466 to fill in.
type Handler struct {
	kaivuev1connect.UnimplementedFederationPeerServiceHandler

	cfg Config
	log *slog.Logger
}

// NewHandler constructs a validated Handler. Returns an error if any
// required Config field is missing.
func NewHandler(cfg Config) (*Handler, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		cfg: cfg,
		log: logger.With(slog.String("component", "directory/federation")),
	}, nil
}

// Ping is a liveness probe that echoes the nonce and returns the server
// time and version. It is the cheapest RPC in the federation service and
// is intended for health checks and RTT measurement.
func (h *Handler) Ping(
	ctx context.Context,
	req *connect.Request[kaivuev1.PingRequest],
) (*connect.Response[kaivuev1.PingResponse], error) {
	h.log.DebugContext(ctx, "Ping",
		"nonce", req.Msg.GetNonce(),
		"peer", req.Peer().Addr,
	)

	resp := &kaivuev1.PingResponse{
		Nonce:         req.Msg.GetNonce(),
		ServerTime:    timestamppb.Now(),
		ServerVersion: h.cfg.ServerVersion,
	}
	return connect.NewResponse(resp), nil
}

// GetJWKS returns this Directory's JSON Web Key Set so the peer can
// verify tokens signed by this instance. The max_age_seconds field is
// a cache-lifetime hint; callers SHOULD honor it before re-fetching.
func (h *Handler) GetJWKS(
	ctx context.Context,
	req *connect.Request[kaivuev1.GetJWKSRequest],
) (*connect.Response[kaivuev1.GetJWKSResponse], error) {
	h.log.DebugContext(ctx, "GetJWKS",
		"peer", req.Peer().Addr,
	)

	jwksJSON, maxAge, err := h.cfg.JWKSProvider.JWKSJSON(ctx)
	if err != nil {
		h.log.ErrorContext(ctx, "GetJWKS: JWKS provider failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to retrieve JWKS"))
	}
	if jwksJSON == "" {
		return nil, connect.NewError(connect.CodeInternal, errors.New("JWKS is empty"))
	}

	resp := &kaivuev1.GetJWKSResponse{
		JwksJson:      jwksJSON,
		MaxAgeSeconds: maxAge,
	}
	return connect.NewResponse(resp), nil
}
