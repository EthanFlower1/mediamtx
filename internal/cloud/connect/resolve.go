package connect

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ResolveConfig configures the resolve endpoint.
type ResolveConfig struct {
	Registry     *Registry
	RelayBaseURL string   // e.g. "wss://relay.raikada.com"
	STUNServers  []string // e.g. ["stun:stun.l.google.com:19302"]
}

// ConnectionPlan describes how a client should connect to an on-prem site.
type ConnectionPlan struct {
	SiteID      string         `json:"site_id"`
	SiteAlias   string         `json:"site_alias"`
	Status      string         `json:"status"`
	PublicIP    string         `json:"public_ip,omitempty"`
	LANCIDRs    []string       `json:"lan_cidrs,omitempty"`
	Endpoints   []PlanEndpoint `json:"endpoints"`
	STUNServers []string       `json:"stun_servers,omitempty"`
}

// PlanEndpoint is a single connectivity option within a ConnectionPlan.
type PlanEndpoint struct {
	Kind               string `json:"kind"` // "lan", "direct", "relay"
	URL                string `json:"url"`
	Priority           int    `json:"priority"`
	EstimatedLatencyMS int    `json:"estimated_latency_ms"`
}

// ResolveHandler returns an http.Handler that resolves a tenant+alias pair
// into a ConnectionPlan describing how to reach the on-prem site.
func ResolveHandler(cfg ResolveConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.URL.Query().Get("tenant_id")
		alias := r.URL.Query().Get("alias")

		if tenantID == "" || alias == "" {
			http.Error(w, `{"error":"tenant_id and alias are required"}`, http.StatusBadRequest)
			return
		}

		session, ok := cfg.Registry.LookupByAlias(tenantID, alias)
		if !ok {
			http.Error(w, `{"error":"site not found"}`, http.StatusNotFound)
			return
		}

		plan := buildConnectionPlan(cfg, session)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(plan)
	})
}

// buildConnectionPlan creates a ConnectionPlan from a registry session.
func buildConnectionPlan(cfg ResolveConfig, s Session) ConnectionPlan {
	var endpoints []PlanEndpoint

	// LAN endpoints: one per CIDR.
	for _, cidr := range s.LANCIDRs {
		host := cidrToHost(cidr)
		endpoints = append(endpoints, PlanEndpoint{
			Kind:               "lan",
			URL:                fmt.Sprintf("https://%s:8889", host),
			Priority:           1,
			EstimatedLatencyMS: 5,
		})
	}

	// Direct endpoint: from PublicIP.
	if s.PublicIP != "" {
		endpoints = append(endpoints, PlanEndpoint{
			Kind:               "direct",
			URL:                fmt.Sprintf("https://%s:8889", s.PublicIP),
			Priority:           2,
			EstimatedLatencyMS: 30,
		})
	}

	// Relay endpoint.
	endpoints = append(endpoints, PlanEndpoint{
		Kind:               "relay",
		URL:                fmt.Sprintf("%s/session/%s", cfg.RelayBaseURL, s.SiteID),
		Priority:           3,
		EstimatedLatencyMS: 80,
	})

	return ConnectionPlan{
		SiteID:      s.SiteID,
		SiteAlias:   s.SiteAlias,
		Status:      s.Status,
		PublicIP:    s.PublicIP,
		LANCIDRs:    s.LANCIDRs,
		Endpoints:   endpoints,
		STUNServers: cfg.STUNServers,
	}
}

// cidrToHost extracts the network address from a CIDR string, stripping the
// /prefix portion. If the string does not contain a slash, it is returned as-is.
func cidrToHost(cidr string) string {
	idx := strings.IndexByte(cidr, '/')
	if idx < 0 {
		return cidr
	}
	return cidr[:idx]
}
