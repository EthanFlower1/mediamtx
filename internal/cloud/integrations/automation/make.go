package automation

// MakeApp describes a Make (Integromat) integration. Make uses "modules"
// (triggers, actions, searches) grouped into a single app with a connection
// definition and optional webhook receivers.
type MakeApp struct {
	Name        string          `json:"name"`
	Label       string          `json:"label"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	BaseURL     string          `json:"base_url"`
	Connection  MakeConnection  `json:"connection"`
	Webhooks    []MakeWebhook   `json:"webhooks"`
	Modules     []MakeModule    `json:"modules"`
}

// MakeConnection is the OAuth2 connection block for Make.
type MakeConnection struct {
	Type            string `json:"type"` // "oauth2"
	AuthorizeURL    string `json:"authorize_url"`
	TokenURL        string `json:"token_url"`
	Scope           string `json:"scope,omitempty"`
	InvalidateURL   string `json:"invalidate_url,omitempty"`
}

// MakeWebhook defines an instant trigger (webhook) in Make.
type MakeWebhook struct {
	Name       string `json:"name"`
	Label      string `json:"label"`
	Type       string `json:"type"` // "web"
	AttachURL  string `json:"attach_url"`
	DetachURL  string `json:"detach_url"`
	Parameters []Field `json:"parameters,omitempty"`
}

// MakeModuleType is the kind of module (trigger, action, etc.).
type MakeModuleType string

const (
	MakeModuleInstantTrigger MakeModuleType = "instant_trigger"
	MakeModuleAction         MakeModuleType = "action"
)

// MakeModule represents a single module in the Make app definition.
type MakeModule struct {
	Name        string         `json:"name"`
	Label       string         `json:"label"`
	Description string         `json:"description"`
	ModuleType  MakeModuleType `json:"module_type"`
	Webhook     string         `json:"webhook,omitempty"`   // references MakeWebhook.Name
	URL         string         `json:"url,omitempty"`       // for action modules
	Method      string         `json:"method,omitempty"`    // for action modules
	Parameters  []Field        `json:"parameters,omitempty"`
	Output      []Field        `json:"output,omitempty"`
}

// DefaultMakeApp builds the Make app definition from shared triggers/actions.
func DefaultMakeApp(baseURL string) *MakeApp {
	webhooks := make([]MakeWebhook, 0, len(SharedTriggers()))
	modules := make([]MakeModule, 0, len(SharedTriggers())+len(SharedActions()))

	for _, t := range SharedTriggers() {
		whName := "watch_" + t.Key
		webhooks = append(webhooks, MakeWebhook{
			Name:      whName,
			Label:     t.Label,
			Type:      "web",
			AttachURL: baseURL + "/api/v1/integrations/automation/webhooks/subscribe",
			DetachURL: baseURL + "/api/v1/integrations/automation/webhooks/unsubscribe",
		})
		modules = append(modules, MakeModule{
			Name:        whName,
			Label:       "Watch " + t.Label,
			Description: t.Description,
			ModuleType:  MakeModuleInstantTrigger,
			Webhook:     whName,
		})
	}

	for _, a := range SharedActions() {
		modules = append(modules, MakeModule{
			Name:        a.Key,
			Label:       a.Label,
			Description: a.Description,
			ModuleType:  MakeModuleAction,
			URL:         baseURL + "/api/v1/integrations/automation/actions/" + a.Key,
			Method:      "POST",
			Parameters:  a.InputFields,
		})
	}

	return &MakeApp{
		Name:        "raikada",
		Label:       "Raikada",
		Version:     "1.0.0",
		Description: "Connect your NVR cameras and alerts to Make scenarios.",
		BaseURL:     baseURL,
		Connection: MakeConnection{
			Type:         "oauth2",
			AuthorizeURL: baseURL + "/oauth/authorize",
			TokenURL:     baseURL + "/oauth/token",
			Scope:        "cameras:read clips:write notifications:write",
		},
		Webhooks: webhooks,
		Modules:  modules,
	}
}
