package openpath

// IntegrationID is the stable identifier for this integration, used in the
// integrations registry, feature flags, and UI routing.
const IntegrationID = "openpath"

// IntegrationMeta returns the metadata that the integrations registry needs
// to surface OpenPath / Avigilon Alta as a supported integration in the UI
// and entitlement checks.
func IntegrationMeta() IntegrationInfo {
	return IntegrationInfo{
		ID:          IntegrationID,
		DisplayName: "OpenPath / Avigilon Alta",
		Category:    "access_control",
		Description: "Cloud access control integration: door events trigger video correlation, " +
			"NVR security events can trigger Alta door lockdowns.",
		LogoURL:       "/assets/integrations/openpath.svg",
		DocsURL:       "https://docs.kaivue.com/integrations/openpath",
		FeatureFlag:   "integrations",
		ConfigURL:     "/integrations/openpath",
		WebhookPath:   "/integrations/openpath/webhook",
		Bidirectional: true,
	}
}

// IntegrationInfo describes a third-party integration for the registry.
type IntegrationInfo struct {
	ID            string `json:"id"`
	DisplayName   string `json:"display_name"`
	Category      string `json:"category"`
	Description   string `json:"description"`
	LogoURL       string `json:"logo_url"`
	DocsURL       string `json:"docs_url"`
	FeatureFlag   string `json:"feature_flag"`
	ConfigURL     string `json:"config_url"`
	WebhookPath   string `json:"webhook_path"`
	Bidirectional bool   `json:"bidirectional"`
}
