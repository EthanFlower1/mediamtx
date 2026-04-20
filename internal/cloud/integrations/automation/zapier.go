package automation

// ZapierApp is the top-level descriptor for the Zapier integration. It
// contains everything needed to generate a Zapier CLI app definition or to
// serve Zapier's REST hooks protocol.
type ZapierApp struct {
	// Identity
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`

	// Auth
	Auth ZapierAuth `json:"authentication"`

	// Triggers & Actions
	Triggers []ZapierTrigger `json:"triggers"`
	Actions  []ZapierAction  `json:"actions"`
}

// ZapierAuth describes the OAuth2 configuration Zapier needs.
type ZapierAuth struct {
	Type               string `json:"type"` // "oauth2"
	AuthorizeURL       string `json:"authorize_url"`
	AccessTokenURL     string `json:"access_token_url"`
	RefreshTokenURL    string `json:"refresh_token_url,omitempty"`
	Scope              string `json:"scope,omitempty"`
	ConnectionLabel    string `json:"connection_label,omitempty"`
	AutoRefresh        bool   `json:"auto_refresh"`
}

// ZapierTrigger is a single REST-hook trigger in the Zapier app.
type ZapierTrigger struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Description string  `json:"description"`
	Type        string  `json:"type"` // "hook"
	SubscribeURL   string `json:"subscribe_url"`
	UnsubscribeURL string `json:"unsubscribe_url"`
	PerformListURL string `json:"perform_list_url"`
	SampleData     any    `json:"sample"`
	OutputFields   []Field `json:"output_fields,omitempty"`
}

// ZapierAction is a single action in the Zapier app.
type ZapierAction struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Description string  `json:"description"`
	PerformURL  string  `json:"perform_url"`
	InputFields []Field `json:"input_fields"`
}

// DefaultZapierApp returns a fully populated ZapierApp using the shared
// triggers/actions and the supplied base URL for webhook endpoints.
func DefaultZapierApp(baseURL string) *ZapierApp {
	triggers := make([]ZapierTrigger, 0, len(SharedTriggers()))
	for _, t := range SharedTriggers() {
		triggers = append(triggers, ZapierTrigger{
			Key:            t.Key,
			Label:          t.Label,
			Description:    t.Description,
			Type:           "hook",
			SubscribeURL:   baseURL + "/api/v1/integrations/automation/webhooks/subscribe",
			UnsubscribeURL: baseURL + "/api/v1/integrations/automation/webhooks/unsubscribe",
			PerformListURL: baseURL + "/api/v1/integrations/automation/webhooks/sample/" + t.Key,
			SampleData:     t.SampleData,
		})
	}

	actions := make([]ZapierAction, 0, len(SharedActions()))
	for _, a := range SharedActions() {
		actions = append(actions, ZapierAction{
			Key:         a.Key,
			Label:       a.Label,
			Description: a.Description,
			PerformURL:  baseURL + "/api/v1/integrations/automation/actions/" + a.Key,
			InputFields: a.InputFields,
		})
	}

	return &ZapierApp{
		Name:        "Raikada",
		Version:     "1.0.0",
		Description: "Connect your NVR cameras and alerts to thousands of apps.",
		Auth: ZapierAuth{
			Type:            "oauth2",
			AuthorizeURL:    baseURL + "/oauth/authorize",
			AccessTokenURL:  baseURL + "/oauth/token",
			RefreshTokenURL: baseURL + "/oauth/token",
			Scope:           "cameras:read clips:write notifications:write",
			ConnectionLabel: "Raikada ({{bundle.authData.email}})",
			AutoRefresh:     true,
		},
		Triggers: triggers,
		Actions:  actions,
	}
}
