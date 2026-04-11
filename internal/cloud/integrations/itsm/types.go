package itsm

import "time"

// ProviderType enumerates supported ITSM/alerting providers.
type ProviderType string

const (
	ProviderPagerDuty ProviderType = "pagerduty"
	ProviderOpsgenie  ProviderType = "opsgenie"
)

// Severity represents alert severity levels, aligned with PagerDuty Events API v2
// and Opsgenie priority levels.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityError    Severity = "error"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

// Alert represents an alert to be sent to an ITSM provider.
type Alert struct {
	// Summary is a short, human-readable summary of the alert (max 1024 chars).
	Summary string `json:"summary"`

	// Source identifies the component that triggered the alert.
	Source string `json:"source"`

	// Severity is the alert severity level.
	Severity Severity `json:"severity"`

	// DedupKey is an optional deduplication key. Alerts with the same dedup key
	// are grouped together. If empty, a new incident is always created.
	DedupKey string `json:"dedup_key,omitempty"`

	// Details contains arbitrary key-value pairs with additional context.
	Details map[string]string `json:"details,omitempty"`

	// Timestamp is when the alert condition was detected.
	Timestamp time.Time `json:"timestamp"`

	// Group is an optional logical grouping (e.g., camera name, zone).
	Group string `json:"group,omitempty"`

	// Class categorizes the alert (e.g., "camera_offline", "storage_full").
	Class string `json:"class,omitempty"`
}

// AlertResult holds the outcome of an alert delivery attempt.
type AlertResult struct {
	// ProviderType is the provider that handled the alert.
	ProviderType ProviderType `json:"provider_type"`

	// ExternalID is the ID assigned by the upstream provider (e.g., PagerDuty
	// dedup_key or Opsgenie alert ID).
	ExternalID string `json:"external_id"`

	// Status is "success" or "error".
	Status string `json:"status"`

	// Message is an optional human-readable status message from the provider.
	Message string `json:"message,omitempty"`

	// Timestamp records when the result was received.
	Timestamp time.Time `json:"timestamp"`
}

// ProviderConfig holds the connection parameters for an ITSM provider.
type ProviderConfig struct {
	// ConfigID is a unique identifier for this configuration.
	ConfigID string `json:"config_id"`

	// TenantID scopes the configuration to a tenant.
	TenantID string `json:"tenant_id"`

	// Provider identifies which ITSM platform this config targets.
	Provider ProviderType `json:"provider"`

	// APIKey is the integration/routing key (PagerDuty) or API key (Opsgenie).
	APIKey string `json:"api_key"`

	// Endpoint overrides the default API endpoint (useful for EU regions or testing).
	Endpoint string `json:"endpoint,omitempty"`

	// Enabled controls whether this provider is active.
	Enabled bool `json:"enabled"`

	// CreatedAt is when the config was first created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the config was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// RoutingRule determines which provider receives alerts matching specific criteria.
type RoutingRule struct {
	// RuleID is a unique identifier for this routing rule.
	RuleID string `json:"rule_id"`

	// TenantID scopes the rule to a tenant.
	TenantID string `json:"tenant_id"`

	// Name is a human-readable name for the rule.
	Name string `json:"name"`

	// ProviderConfigID references the target ProviderConfig.
	ProviderConfigID string `json:"provider_config_id"`

	// MinSeverity is the minimum severity that triggers this rule.
	// Alerts at or above this level are routed. Empty means all severities.
	MinSeverity Severity `json:"min_severity,omitempty"`

	// AlertClasses is an optional list of alert classes this rule applies to.
	// An empty list means the rule applies to all classes.
	AlertClasses []string `json:"alert_classes,omitempty"`

	// Priority is the evaluation order (lower = higher priority).
	Priority int `json:"priority"`

	// Enabled controls whether this rule is active.
	Enabled bool `json:"enabled"`

	// CreatedAt is when the rule was first created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the rule was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}
