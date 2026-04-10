package federation

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1/kaivuev1connect"
)

// ServerConfig configures the federation mTLS server.
type ServerConfig struct {
	// ListenAddr is the address to listen on (e.g. ":9443").
	ListenAddr string

	// TLSCert is the server's TLS certificate chain (leaf + intermediates).
	// Typically obtained from ClusterCA.IssueDirectoryServingCert().
	TLSCert *tls.Certificate

	// ClientCAs is the CA pool used to verify peer certificates. In
	// federation mode this is the federation root pool; only peers whose
	// certs chain to a trusted federation root are accepted.
	ClientCAs *x509.CertPool

	// Handler is the FederationPeerServiceHandler to serve.
	Handler *Handler

	// Logger is the structured logger.
	Logger *slog.Logger
}

func (c *ServerConfig) validate() error {
	if c.ListenAddr == "" {
		return errors.New("federation: ListenAddr is required")
	}
	if c.TLSCert == nil {
		return errors.New("federation: TLSCert is required")
	}
	if c.ClientCAs == nil {
		return errors.New("federation: ClientCAs is required")
	}
	if c.Handler == nil {
		return errors.New("federation: Handler is required")
	}
	return nil
}

// Server is the mTLS-authenticated federation RPC server. It serves the
// FederationPeerService over Connect-Go with mandatory client certificate
// verification.
type Server struct {
	cfg    ServerConfig
	log    *slog.Logger
	server *http.Server
}

// NewServer constructs a Server. Call ListenAndServe to start.
func NewServer(cfg ServerConfig) (*Server, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	mux := http.NewServeMux()
	path, handler := kaivuev1connect.NewFederationPeerServiceHandler(cfg.Handler)
	mux.Handle(path, handler)

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{*cfg.TLSCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    cfg.ClientCAs,
		MinVersion:   tls.VersionTLS13,
	}

	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      mux,
		TLSConfig:    tlsCfg,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return &Server{
		cfg:    cfg,
		log:    logger.With(slog.String("component", "directory/federation/server")),
		server: srv,
	}, nil
}

// ListenAndServe starts the mTLS listener. It blocks until the server
// is shut down or an unrecoverable error occurs. Callers typically run
// this in a goroutine and call Shutdown when the Directory is stopping.
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("federation: listen %s: %w", s.cfg.ListenAddr, err)
	}

	tlsLn := tls.NewListener(ln, s.server.TLSConfig)
	s.log.Info("federation peer server listening",
		"addr", ln.Addr().String(),
		"tls", "mTLS-1.3",
	)

	if err := s.server.Serve(tlsLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("federation: serve: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the server with a 10-second deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	shutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	s.log.Info("federation peer server shutting down")
	return s.server.Shutdown(shutCtx)
}
