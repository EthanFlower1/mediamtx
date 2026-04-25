package connect

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/directory/cloudconnector"
)

func TestEndToEndConnectAndResolve(t *testing.T) {
	// 1. Set up registry and broker.
	reg := NewRegistry()
	broker := NewBroker(BrokerConfig{
		Registry: reg,
		Authenticate: func(token string) (string, bool) {
			if token == "site-token" {
				return "tenant-1", true
			}
			return "", false
		},
		RelayURL: "wss://relay.test.com",
		Logger:   slog.Default(),
	})

	// 2. Set up resolve handler.
	mux := http.NewServeMux()
	mux.Handle("/ws/directory", broker)
	mux.Handle("/connect/resolve", ResolveHandler(ResolveConfig{
		Registry:     reg,
		RelayBaseURL: "wss://relay.test.com",
		STUNServers:  []string{"stun:stun.test.com:19302"},
	}))

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// 3. Connect an on-prem Directory via the connector.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/directory"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	connector := cloudconnector.New(cloudconnector.Config{
		URL:   wsURL,
		Token: "site-token",
		Site: cloudconnector.SiteInfo{
			ID:       "site-abc",
			Alias:    "my-home",
			LANCIDRs: []string{"192.168.1.0/24"},
			Capabilities: cloudconnector.Capabilities{
				Streams:  true,
				Playback: true,
			},
		},
		HeartbeatInterval: 30 * time.Second,
		Logger:            slog.Default(),
	})

	go connector.Run(ctx)

	// 4. Wait for registration to complete.
	time.Sleep(500 * time.Millisecond)

	// 5. Resolve the site alias.
	resolveURL := srv.URL + "/connect/resolve?tenant_id=tenant-1&alias=my-home"
	resp, err := http.Get(resolveURL)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var plan ConnectionPlan
	if err := json.NewDecoder(resp.Body).Decode(&plan); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Verify the plan.
	if plan.SiteID != "site-abc" {
		t.Fatalf("site_id = %q, want %q", plan.SiteID, "site-abc")
	}
	if plan.Status != StatusOnline {
		t.Fatalf("status = %q, want %q", plan.Status, StatusOnline)
	}

	// Check endpoint kinds.
	kinds := map[string]bool{}
	for _, ep := range plan.Endpoints {
		kinds[ep.Kind] = true
	}
	if !kinds["lan"] {
		t.Fatal("expected lan endpoint")
	}
	if !kinds["relay"] {
		t.Fatal("expected relay endpoint")
	}

	// Check STUN servers.
	if len(plan.STUNServers) == 0 {
		t.Fatal("expected STUN servers")
	}

	// 6. Verify disconnect removes from registry.
	cancel()
	time.Sleep(500 * time.Millisecond)

	_, ok := reg.LookupByAlias("tenant-1", "my-home")
	if ok {
		t.Fatal("expected site removed from registry after disconnect")
	}
}
