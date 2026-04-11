// Package integrations provides a central metadata registry for all
// first-party integration packages. It intentionally does NOT import the
// sub-packages (to avoid circular dependencies). Actual service
// instantiation happens at the application bootstrap level.
package integrations

import "sort"

// IntegrationInfo describes a first-party integration.
type IntegrationInfo struct {
	ID          string   // unique identifier (e.g., "brivo", "openpath")
	DisplayName string   // human-readable name
	Category    string   // "access_control", "alarm_panel", "itsm", "comms", "automation"
	Description string
	Features    []string // what it supports (e.g., "oauth", "webhook", "bidirectional")
}

// Registry holds all registered integrations.
type Registry struct {
	integrations map[string]IntegrationInfo
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		integrations: make(map[string]IntegrationInfo),
	}
}

// Register adds an integration to the registry. If an integration with the
// same ID already exists it is silently overwritten.
func (r *Registry) Register(info IntegrationInfo) {
	r.integrations[info.ID] = info
}

// Get returns the IntegrationInfo for the given ID and a boolean indicating
// whether it was found.
func (r *Registry) Get(id string) (IntegrationInfo, bool) {
	info, ok := r.integrations[id]
	return info, ok
}

// List returns all registered integrations sorted alphabetically by ID.
func (r *Registry) List() []IntegrationInfo {
	out := make([]IntegrationInfo, 0, len(r.integrations))
	for _, info := range r.integrations {
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ListByCategory returns all integrations matching the given category,
// sorted alphabetically by ID.
func (r *Registry) ListByCategory(category string) []IntegrationInfo {
	var out []IntegrationInfo
	for _, info := range r.integrations {
		if info.Category == category {
			out = append(out, info)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// DefaultRegistry returns a registry pre-populated with all 9 first-party
// integrations.
func DefaultRegistry() *Registry {
	r := NewRegistry()

	// Access control integrations
	r.Register(IntegrationInfo{
		ID:          "brivo",
		DisplayName: "Brivo",
		Category:    "access_control",
		Description: "Cloud access control with OAuth2, webhook door events, and bidirectional NVR-to-Brivo event sync.",
		Features:    []string{"oauth", "webhook", "bidirectional"},
	})
	r.Register(IntegrationInfo{
		ID:          "openpath",
		DisplayName: "OpenPath / Avigilon Alta",
		Category:    "access_control",
		Description: "Alta cloud access control with door-event-to-camera correlation and lockdown triggers.",
		Features:    []string{"oauth", "webhook", "bidirectional"},
	})
	r.Register(IntegrationInfo{
		ID:          "pdk",
		DisplayName: "ProdataKey (PDK)",
		Category:    "access_control",
		Description: "PDK cloud access control with client-credentials OAuth and door event ingestion.",
		Features:    []string{"oauth", "webhook"},
	})

	// Alarm panel integrations
	r.Register(IntegrationInfo{
		ID:          "bosch",
		DisplayName: "Bosch B/G-Series",
		Category:    "alarm_panel",
		Description: "Bosch B/G-Series alarm panel with zone-to-camera mapping and camera action dispatch.",
		Features:    []string{"webhook", "bidirectional"},
	})
	r.Register(IntegrationInfo{
		ID:          "dmp",
		DisplayName: "DMP XR-Series",
		Category:    "alarm_panel",
		Description: "DMP XR-Series alarm panel with SIA protocol receiver and zone-to-camera mapping.",
		Features:    []string{"sia_protocol", "zone_mapping"},
	})

	// ITSM / alerting
	r.Register(IntegrationInfo{
		ID:          "pagerduty_opsgenie",
		DisplayName: "PagerDuty + Opsgenie",
		Category:    "itsm",
		Description: "Critical alert escalation routing via PagerDuty Events API v2 and Opsgenie.",
		Features:    []string{"alerting", "escalation"},
	})

	// Communications
	r.Register(IntegrationInfo{
		ID:          "slack_teams",
		DisplayName: "Slack + Microsoft Teams",
		Category:    "comms",
		Description: "Alert cards to Slack and Teams channels with interactive actions and deep-links to clip viewer.",
		Features:    []string{"oauth", "webhook", "interactive_actions"},
	})

	// Automation
	r.Register(IntegrationInfo{
		ID:          "zapier",
		DisplayName: "Zapier",
		Category:    "automation",
		Description: "Zapier integration app with triggers and actions for camera events and alerts.",
		Features:    []string{"oauth", "webhook", "triggers", "actions"},
	})
	r.Register(IntegrationInfo{
		ID:          "make_n8n",
		DisplayName: "Make + n8n",
		Category:    "automation",
		Description: "Make (Integromat) app and n8n community node for workflow automation.",
		Features:    []string{"oauth", "webhook", "triggers", "actions"},
	})

	return r
}
