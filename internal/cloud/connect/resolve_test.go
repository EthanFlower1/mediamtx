package connect

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveReturnsConnectionPlan(t *testing.T) {
	reg := NewRegistry()

	reg.Add(Session{
		SiteID:    "site-1",
		TenantID:  "tenant-1",
		SiteAlias: "warehouse",
		PublicIP:  "203.0.113.1",
		LANCIDRs:  []string{"192.168.1.0/24", "10.0.0.0/8"},
		Status:    StatusOnline,
	})

	handler := ResolveHandler(ResolveConfig{
		Registry:     reg,
		RelayBaseURL: "wss://relay.raikada.com",
		STUNServers:  []string{"stun:stun.l.google.com:19302"},
	})

	req := httptest.NewRequest(http.MethodGet, "/resolve?tenant_id=tenant-1&alias=warehouse", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var plan ConnectionPlan
	if err := json.NewDecoder(w.Body).Decode(&plan); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if plan.SiteID != "site-1" {
		t.Errorf("SiteID = %q, want %q", plan.SiteID, "site-1")
	}
	if plan.SiteAlias != "warehouse" {
		t.Errorf("SiteAlias = %q, want %q", plan.SiteAlias, "warehouse")
	}
	if plan.Status != StatusOnline {
		t.Errorf("Status = %q, want %q", plan.Status, StatusOnline)
	}
	if plan.PublicIP != "203.0.113.1" {
		t.Errorf("PublicIP = %q, want %q", plan.PublicIP, "203.0.113.1")
	}

	// Should have LAN endpoints (2) + direct (1) + relay (1) = 4 endpoints.
	if len(plan.Endpoints) != 4 {
		t.Fatalf("len(Endpoints) = %d, want 4", len(plan.Endpoints))
	}

	// Verify LAN endpoints.
	lanCount := 0
	for _, ep := range plan.Endpoints {
		if ep.Kind == "lan" {
			lanCount++
			if ep.Priority != 1 {
				t.Errorf("lan endpoint priority = %d, want 1", ep.Priority)
			}
			if ep.EstimatedLatencyMS != 5 {
				t.Errorf("lan endpoint latency = %d, want 5", ep.EstimatedLatencyMS)
			}
		}
	}
	if lanCount != 2 {
		t.Errorf("lan endpoint count = %d, want 2", lanCount)
	}

	// Verify direct endpoint.
	var directEP *PlanEndpoint
	for i, ep := range plan.Endpoints {
		if ep.Kind == "direct" {
			directEP = &plan.Endpoints[i]
			break
		}
	}
	if directEP == nil {
		t.Fatal("expected a direct endpoint")
	}
	if directEP.URL != "https://203.0.113.1:8889" {
		t.Errorf("direct URL = %q, want %q", directEP.URL, "https://203.0.113.1:8889")
	}
	if directEP.Priority != 2 {
		t.Errorf("direct priority = %d, want 2", directEP.Priority)
	}

	// Verify relay endpoint.
	var relayEP *PlanEndpoint
	for i, ep := range plan.Endpoints {
		if ep.Kind == "relay" {
			relayEP = &plan.Endpoints[i]
			break
		}
	}
	if relayEP == nil {
		t.Fatal("expected a relay endpoint")
	}
	if relayEP.URL != "wss://relay.raikada.com/session/site-1" {
		t.Errorf("relay URL = %q, want %q", relayEP.URL, "wss://relay.raikada.com/session/site-1")
	}
	if relayEP.Priority != 3 {
		t.Errorf("relay priority = %d, want 3", relayEP.Priority)
	}
	if relayEP.EstimatedLatencyMS != 80 {
		t.Errorf("relay latency = %d, want 80", relayEP.EstimatedLatencyMS)
	}

	// Verify STUN servers.
	if len(plan.STUNServers) != 1 || plan.STUNServers[0] != "stun:stun.l.google.com:19302" {
		t.Errorf("STUNServers = %v, want [stun:stun.l.google.com:19302]", plan.STUNServers)
	}

	// Verify LANCIDRs passed through.
	if len(plan.LANCIDRs) != 2 {
		t.Errorf("LANCIDRs count = %d, want 2", len(plan.LANCIDRs))
	}
}

func TestResolveOfflineSite(t *testing.T) {
	reg := NewRegistry()

	handler := ResolveHandler(ResolveConfig{
		Registry:     reg,
		RelayBaseURL: "wss://relay.raikada.com",
		STUNServers:  []string{"stun:stun.l.google.com:19302"},
	})

	req := httptest.NewRequest(http.MethodGet, "/resolve?tenant_id=tenant-1&alias=unknown-site", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestResolveMissingParams(t *testing.T) {
	reg := NewRegistry()

	handler := ResolveHandler(ResolveConfig{
		Registry:     reg,
		RelayBaseURL: "wss://relay.raikada.com",
	})

	tests := []struct {
		name string
		url  string
	}{
		{"missing both", "/resolve"},
		{"missing alias", "/resolve?tenant_id=t1"},
		{"missing tenant_id", "/resolve?alias=a1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestCidrToHost(t *testing.T) {
	tests := []struct {
		cidr string
		want string
	}{
		{"192.168.1.0/24", "192.168.1.0"},
		{"10.0.0.0/8", "10.0.0.0"},
		{"172.16.0.1/16", "172.16.0.1"},
		{"invalid", "invalid"},
	}

	for _, tt := range tests {
		got := cidrToHost(tt.cidr)
		if got != tt.want {
			t.Errorf("cidrToHost(%q) = %q, want %q", tt.cidr, got, tt.want)
		}
	}
}

func TestResolveNoPublicIPSkipsDirectEndpoint(t *testing.T) {
	reg := NewRegistry()

	reg.Add(Session{
		SiteID:    "site-nopub",
		TenantID:  "tenant-1",
		SiteAlias: "office",
		PublicIP:  "",
		LANCIDRs:  []string{"192.168.1.0/24"},
		Status:    StatusOnline,
	})

	handler := ResolveHandler(ResolveConfig{
		Registry:     reg,
		RelayBaseURL: "wss://relay.raikada.com",
		STUNServers:  []string{"stun:stun.l.google.com:19302"},
	})

	req := httptest.NewRequest(http.MethodGet, "/resolve?tenant_id=tenant-1&alias=office", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var plan ConnectionPlan
	json.NewDecoder(w.Body).Decode(&plan)

	for _, ep := range plan.Endpoints {
		if ep.Kind == "direct" {
			t.Error("expected no direct endpoint when PublicIP is empty")
		}
	}
}
