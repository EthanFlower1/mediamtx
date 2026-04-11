package whitelabel

import "time"

// StatusPageConfig is the per-integrator configuration for a white-labeled
// status page. It controls subdomain routing, visual branding, and which
// service components are exposed to the integrator's customers.
type StatusPageConfig struct {
	IntegratorID   string    `json:"integrator_id"`
	Subdomain      string    `json:"subdomain"`
	CustomDomain   string    `json:"custom_domain,omitempty"`
	PageTitle      string    `json:"page_title"`
	LogoURL        string    `json:"logo_url,omitempty"`
	FaviconURL     string    `json:"favicon_url,omitempty"`
	PrimaryColor   string    `json:"primary_color"`
	SecondaryColor string    `json:"secondary_color"`
	AccentColor    string    `json:"accent_color"`
	HeaderBgColor  string    `json:"header_bg_color"`
	FooterText     string    `json:"footer_text,omitempty"`
	CustomCSS      string    `json:"custom_css,omitempty"`
	ComponentIDs   []string  `json:"component_ids"`
	SupportURL     string    `json:"support_url,omitempty"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Subscriber is an email subscriber for integrator status page notifications.
type Subscriber struct {
	SubscriberID  string    `json:"subscriber_id"`
	IntegratorID  string    `json:"integrator_id"`
	Email         string    `json:"email"`
	Confirmed     bool      `json:"confirmed"`
	ConfirmToken  string    `json:"confirm_token,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// PublicStatusPage is the rendered view a customer sees when visiting the
// integrator's status subdomain.
type PublicStatusPage struct {
	Config          StatusPageConfig        `json:"config"`
	OverallStatus   string                  `json:"overall_status"`
	Components      []ComponentStatus       `json:"components"`
	ActiveIncidents []IncidentView          `json:"active_incidents"`
}

// ComponentStatus is a single service component visible on the public page.
type ComponentStatus struct {
	CheckID     string     `json:"check_id"`
	ServiceName string     `json:"service_name"`
	DisplayName string     `json:"display_name"`
	Status      string     `json:"status"`
	LastChecked *time.Time `json:"last_checked,omitempty"`
}

// IncidentView is a public-safe view of an incident for the status page.
type IncidentView struct {
	IncidentID       string         `json:"incident_id"`
	Title            string         `json:"title"`
	Severity         string         `json:"severity"`
	Status           string         `json:"status"`
	AffectedServices string         `json:"affected_services"`
	StartedAt        time.Time      `json:"started_at"`
	ResolvedAt       *time.Time     `json:"resolved_at,omitempty"`
	Updates          []UpdateView   `json:"updates,omitempty"`
}

// UpdateView is a public-safe incident update.
type UpdateView struct {
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
