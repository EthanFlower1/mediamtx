// Command cloudbroker runs a minimal cloud broker for testing the
// QuickConnect-style remote access flow. It accepts Directory WSS
// connections, resolves site aliases, and relays traffic.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/broker"
	"github.com/bluenviron/mediamtx/internal/cloud/connect"
	"github.com/bluenviron/mediamtx/internal/cloud/frpserver"
	"github.com/bluenviron/mediamtx/internal/cloud/portalui"
	"github.com/bluenviron/mediamtx/internal/cloud/relay"
	_ "modernc.org/sqlite"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	token := flag.String("token", "test-token", "accepted bearer token for directory auth")
	tenantID := flag.String("tenant", "test-tenant", "tenant ID assigned to authenticated directories")
	dbPath := flag.String("db", "broker.db", "SQLite database path for tenant/key storage")
	frpPort := flag.Int("frp-port", 7000, "frp control port")
	frpHTTPPort := flag.Int("frp-http-port", 7080, "frp vhost HTTP port for subdomain routing")
	subdomainHost := flag.String("subdomain-host", "raikada.com", "base domain for frp subdomains")
	cookieDomain := flag.String("cookie-domain", "", "domain for browser session cookies (e.g. .raikada.com); empty for dev")
	secureCookies := flag.Bool("secure-cookies", false, "set Secure flag on session cookies (must be true in prod)")
	flag.Parse()

	sessionCfg := broker.SessionConfig{
		CookieDomain:  *cookieDomain,
		SecureCookies: *secureCookies,
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Open tenant/key SQLite database.
	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Error("cloudbroker: failed to open database", "path", *dbPath, "error", err)
		os.Exit(1)
	}
	defer db.Close()

	store, err := broker.NewStore(db)
	if err != nil {
		log.Error("cloudbroker: failed to initialise store", "error", err)
		os.Exit(1)
	}

	auth := broker.NewAuthenticator(store)

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

	connectBroker := connect.NewBroker(connect.BrokerConfig{
		Registry: registry,
		Authenticate: func(t string) (string, bool) {
			// Per-tenant API key authentication.
			if tid, ok := auth.Authenticate(t); ok {
				return tid, true
			}
			// Legacy fallback for backwards compatibility.
			if acceptedToken != "" && t == acceptedToken {
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

	mux.Handle("/ws/directory", connectBroker)
	mux.Handle("/connect/resolve", resolveHandler)
	mux.Handle("/relay/", relayHandler)
	mux.Handle("/stream/", streamProxy)

	// Tenant signup + account management APIs.
	mux.Handle("/api/v1/signup", broker.SignupHandler(store))
	mux.Handle("/api/v1/account", broker.AccountHandler(store))
	mux.Handle("/api/v1/account/keys", broker.ListKeysHandler(store))

	// Browser session auth (cookie-based) for the cloud portal.
	mux.Handle("/api/v1/login", broker.LoginHandler(store, sessionCfg))
	mux.Handle("/api/v1/logout", broker.LogoutHandler(store, sessionCfg))
	mux.Handle("/api/v1/session", broker.SessionHandler(store))
	mux.Handle("/api/v1/session/refresh", broker.RefreshHandler(store, sessionCfg))

	// Sweep expired sessions every hour.
	go func() {
		for range time.Tick(time.Hour) {
			if err := store.CleanupExpiredSessions(); err != nil {
				log.Error("cloudbroker: cleanup sessions", "error", err)
			}
		}
	}()

	// Cloud portal SPA — must be the last route registered since it's a catch-all.
	// All explicit /api/v1/*, /ws/*, /connect/*, /relay/*, /stream/* routes above
	// take precedence due to ServeMux's longest-prefix-match semantics.
	mux.Handle("/", portalui.Handler(""))

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
