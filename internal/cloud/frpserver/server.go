// Package frpserver wraps frp's server library to provide reverse-tunnel
// ingress for recorder-to-cloud connectivity.
package frpserver

import (
	"context"
	"fmt"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/server"
)

// Config holds the parameters needed to start an embedded frp server.
type Config struct {
	BindAddr      string // listen address, e.g. "0.0.0.0"
	BindPort      int    // control port, e.g. 7000
	VhostHTTPPort int    // vhost HTTP port, e.g. 7080
	SubDomainHost string // e.g. "raikada.com"
	Token         string // shared secret for client auth
}

// Server wraps an frp server.Service so callers don't need to import frp
// directly.
type Server struct {
	svc    *server.Service
	cancel context.CancelFunc
	done   chan struct{}
}

// New creates a new frp server from the given config. It does not start
// listening; call Run for that.
func New(cfg Config) (*Server, error) {
	scfg := &v1.ServerConfig{
		BindAddr:      cfg.BindAddr,
		BindPort:      cfg.BindPort,
		VhostHTTPPort: cfg.VhostHTTPPort,
		SubDomainHost: cfg.SubDomainHost,
		Auth: v1.AuthServerConfig{
			Method: v1.AuthMethodToken,
			Token:  cfg.Token,
		},
	}

	if err := scfg.Complete(); err != nil {
		return nil, fmt.Errorf("frp config complete: %w", err)
	}

	svc, err := server.NewService(scfg)
	if err != nil {
		return nil, fmt.Errorf("frp new service: %w", err)
	}

	return &Server{
		svc:  svc,
		done: make(chan struct{}),
	}, nil
}

// Run starts the frp server in the background and returns immediately.
// The server runs until Close is called.
func (s *Server) Run() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go func() {
		defer close(s.done)
		s.svc.Run(ctx)
	}()
}

// Close stops the frp server and releases all resources.
// Note: due to an upstream bug in the frp mux library, svc.Run may not
// return promptly after Close; resources are freed regardless.
func (s *Server) Close() error {
	err := s.svc.Close()
	if s.cancel != nil {
		s.cancel()
	}
	return err
}
