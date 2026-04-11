package brivo

// IntegrationID is the stable identifier for this integration, used in the
// integrations registry, feature flags, and UI routing.
const IntegrationID = "brivo"

// IntegrationMeta returns the metadata that the integrations registry needs
// to surface Brivo as a supported integration in the UI and entitlement
// checks.
func IntegrationMeta() IntegrationInfo {
	return IntegrationInfo{
		ID:          IntegrationID,
		DisplayName: "Brivo",
		Category:    "access_control",
		Description: "Cloud access control integration: door events trigger video snapshots, " +
			"NVR motion events can trigger Brivo lockdowns.",
		LogoURL:       "/assets/integrations/brivo.svg",
		DocsURL:       "https://docs.kaivue.com/integrations/brivo",
		FeatureFlag:   "integrations",
		ConfigURL:     "/integrations/brivo",
		WebhookPath:   "/integrations/brivo/webhook",
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
