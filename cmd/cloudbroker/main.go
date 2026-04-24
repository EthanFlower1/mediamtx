// Command cloudbroker runs a minimal cloud broker for testing the
// QuickConnect-style remote access flow. It accepts Directory WSS
// connections, resolves site aliases, and relays traffic.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/connect"
	"github.com/bluenviron/mediamtx/internal/cloud/frpserver"
	"github.com/bluenviron/mediamtx/internal/cloud/relay"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	token := flag.String("token", "test-token", "accepted bearer token for directory auth")
	tenantID := flag.String("tenant", "test-tenant", "tenant ID assigned to authenticated directories")
	frpPort := flag.Int("frp-port", 7000, "frp control port")
	frpHTTPPort := flag.Int("frp-http-port", 7080, "frp vhost HTTP port for subdomain routing")
	subdomainHost := flag.String("subdomain-host", "raikada.com", "base domain for frp subdomains")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	registry := connect.NewRegistry()
	sessions := relay.NewSessionManager()

	// Cleanup expired relay sessions every 30s.
	go func() {
		for range time.Tick(30 * time.Second) {
			sessions.Cleanup()
		}
	}()

	acceptedToken := *token
	acceptedTenant := *tenantID

	broker := connect.NewBroker(connect.BrokerConfig{
		Registry: registry,
		Authenticate: func(t string) (string, bool) {
			if t == acceptedToken {
				return acceptedTenant, true
			}
			return "", false
		},
		RelayURL: fmt.Sprintf("ws://localhost%s/relay", *addr),
		Logger:   log.With(slog.String("component", "broker")),
	})

	resolveHandler := connect.ResolveHandler(connect.ResolveConfig{
		Registry:     registry,
		RelayBaseURL: fmt.Sprintf("ws://localhost%s/relay", *addr),
		STUNServers:  []string{"stun:stun.l.google.com:19302"},
	})

	relayHandler := relay.NewHandler(relay.HandlerConfig{
		Sessions: sessions,
		Logger:   log.With(slog.String("component", "relay")),
	})

	mux := http.NewServeMux()

	// Health check.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","component":"cloudbroker"}`)
	})

	// List connected sites (for debugging / e2e assertions).
	mux.HandleFunc("/debug/sites", func(w http.ResponseWriter, r *http.Request) {
		sites := registry.ListByTenant(acceptedTenant)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"count":%d,"sites":[`, len(sites))
		for i, s := range sites {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"site_id":%q,"alias":%q,"status":%q,"cameras":%d}`,
				s.SiteID, s.SiteAlias, s.Status, s.CameraCount)
		}
		fmt.Fprint(w, "]}")
	})

	// Stream proxy — HLS through the WSS tunnel.
	streamProxy := connect.StreamProxyHandler(connect.StreamProxyConfig{
		Registry: registry,
		TenantID: acceptedTenant,
		Logger:   log.With(slog.String("component", "stream-proxy")),
	})

	mux.Handle("/ws/directory", broker)
	mux.Handle("/connect/resolve", resolveHandler)
	mux.Handle("/relay/", relayHandler)
	mux.Handle("/stream/", streamProxy)

	// Start the embedded frp server for reverse tunneling.
	frpSrv, err := frpserver.New(frpserver.Config{
		BindAddr:      "0.0.0.0",
		BindPort:      *frpPort,
		VhostHTTPPort: *frpHTTPPort,
		SubDomainHost: *subdomainHost,
		Token:         *token,
	})
	if err != nil {
		log.Error("cloudbroker: failed to create frp server", "error", err)
		os.Exit(1)
	}
	frpSrv.Run()
	log.Info("cloudbroker: frp server started", "control-port", *frpPort, "http-port", *frpHTTPPort, "subdomain-host", *subdomainHost)

	log.Info("cloudbroker: starting", "addr", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Error("cloudbroker: fatal", "error", err)
		os.Exit(1)
	}
}
