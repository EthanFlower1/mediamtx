package broker

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/connect"
	"github.com/bluenviron/mediamtx/internal/directory/cloudconnector"
	_ "modernc.org/sqlite"
)

// TestSignupConnectResolveFlow exercises the full end-to-end flow:
//
//  1. POST /api/v1/signup → receive tenant_id + api_key
//  2. Connect a cloudconnector.Connector using the API key as bearer token
//  3. Wait for registration in the registry
//  4. GET /connect/resolve?tenant_id=...&alias=... → verify ConnectionPlan
//  5. Disconnect → verify site removed from registry
func TestSignupConnectResolveFlow(t *testing.T) {
	// --- 1. In-memory SQLite store ---
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	// --- 2. Build authenticator, registry, broker, resolve handler ---
	auth := NewAuthenticator(store)
	registry := connect.NewRegistry()

	broker := connect.NewBroker(connect.BrokerConfig{
		Registry:     registry,
		Authenticate: auth.Authenticate,
		RelayURL:     "wss://relay.test.raikada.com",
		Logger:       slog.Default(),
	})

	mux := http.NewServeMux()
	mux.Handle("/api/v1/signup", SignupHandler(store))
	mux.Handle("/ws/directory", broker)
	mux.Handle("/connect/resolve", connect.ResolveHandler(connect.ResolveConfig{
		Registry:     registry,
		RelayBaseURL: "wss://relay.test.raikada.com",
		STUNServers:  []string{"stun:stun.l.google.com:19302"},
	}))

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// --- 3. Signup ---
	signupBody, _ := json.Marshal(SignupRequest{
		CompanyName: "Integration Test Corp",
		Email:       "integration@test.com",
	})

	resp, err := http.Post(srv.URL+"/api/v1/signup", "application/json", bytes.NewReader(signupBody))
	if err != nil {
		t.Fatalf("signup request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("signup status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var signupResp SignupResponse
	if err := json.NewDecoder(resp.Body).Decode(&signupResp); err != nil {
		t.Fatalf("decode signup response: %v", err)
	}

	if signupResp.TenantID == "" {
		t.Fatal("signup returned empty tenant_id")
	}
	if signupResp.APIKey == "" {
		t.Fatal("signup returned empty api_key")
	}
	if !strings.HasPrefix(signupResp.APIKey, "kvue_") {
		t.Fatalf("api_key should start with kvue_, got %q", signupResp.APIKey[:10])
	}

	tenantID := signupResp.TenantID
	apiKey := signupResp.APIKey

	// --- 4. Connect a cloudconnector using the API key ---
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/directory"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	siteAlias := "hq-office"
	siteID := "site-integration-001"

	connector := cloudconnector.New(cloudconnector.Config{
		URL:   wsURL,
		Token: apiKey,
		Site: cloudconnector.SiteInfo{
			ID:       siteID,
			Alias:    siteAlias,
			Version:  "1.0.0-test",
			LANCIDRs: []string{"10.0.0.0/24"},
			Capabilities: cloudconnector.Capabilities{
				Streams:  true,
				Playback: true,
				AI:       false,
			},
		},
		HeartbeatInterval: 30 * time.Second,
		Logger:            slog.Default(),
	})

	go connector.Run(ctx)

	// --- 5. Wait for registration ---
	var registered bool
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := registry.LookupByAlias(tenantID, siteAlias); ok {
			registered = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !registered {
		t.Fatal("connector did not register within timeout")
	}

	// --- 6. Resolve ---
	resolveURL := srv.URL + "/connect/resolve?tenant_id=" + tenantID + "&alias=" + siteAlias
	resolveResp, err := http.Get(resolveURL)
	if err != nil {
		t.Fatalf("resolve request: %v", err)
	}
	defer resolveResp.Body.Close()

	if resolveResp.StatusCode != http.StatusOK {
		t.Fatalf("resolve status = %d, want %d", resolveResp.StatusCode, http.StatusOK)
	}

	var plan connect.ConnectionPlan
	if err := json.NewDecoder(resolveResp.Body).Decode(&plan); err != nil {
		t.Fatalf("decode connection plan: %v", err)
	}

	// Verify plan fields.
	if plan.SiteID != siteID {
		t.Fatalf("plan.SiteID = %q, want %q", plan.SiteID, siteID)
	}
	if plan.SiteAlias != siteAlias {
		t.Fatalf("plan.SiteAlias = %q, want %q", plan.SiteAlias, siteAlias)
	}
	if plan.Status != "online" {
		t.Fatalf("plan.Status = %q, want %q", plan.Status, "online")
	}

	// Verify endpoints include LAN and relay.
	kinds := map[string]bool{}
	for _, ep := range plan.Endpoints {
		kinds[ep.Kind] = true
	}
	if !kinds["lan"] {
		t.Fatal("expected lan endpoint in connection plan")
	}
	if !kinds["relay"] {
		t.Fatal("expected relay endpoint in connection plan")
	}

	// Verify relay URL contains the site ID.
	for _, ep := range plan.Endpoints {
		if ep.Kind == "relay" {
			if !strings.Contains(ep.URL, siteID) {
				t.Fatalf("relay URL %q should contain site ID %q", ep.URL, siteID)
			}
		}
	}

	// Verify STUN servers are populated.
	if len(plan.STUNServers) == 0 {
		t.Fatal("expected STUN servers in connection plan")
	}

	// Verify LAN CIDR data round-tripped.
	if len(plan.LANCIDRs) == 0 {
		t.Fatal("expected LAN CIDRs in connection plan")
	}
	if plan.LANCIDRs[0] != "10.0.0.0/24" {
		t.Fatalf("plan.LANCIDRs[0] = %q, want %q", plan.LANCIDRs[0], "10.0.0.0/24")
	}

	// --- 7. Disconnect and verify removal ---
	cancel()

	var removed bool
	removeDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(removeDeadline) {
		if _, ok := registry.LookupByAlias(tenantID, siteAlias); !ok {
			removed = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !removed {
		t.Fatal("expected site removed from registry after disconnect")
	}

	// Resolve should now return 404.
	resolveResp2, err := http.Get(resolveURL)
	if err != nil {
		t.Fatalf("resolve after disconnect: %v", err)
	}
	defer resolveResp2.Body.Close()

	if resolveResp2.StatusCode != http.StatusNotFound {
		t.Fatalf("resolve after disconnect status = %d, want %d", resolveResp2.StatusCode, http.StatusNotFound)
	}
}
