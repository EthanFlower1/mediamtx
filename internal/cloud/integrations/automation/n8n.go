package automation

// N8NNodeType describes a single n8n community node. n8n nodes are
// TypeScript-based in practice; this Go struct is used to serve a JSON
// description that a thin TS wrapper can consume, and to drive the webhook
// receiver on the server side.
type N8NNodeType struct {
	Name            string           `json:"name"`
	DisplayName     string           `json:"displayName"`
	Description     string           `json:"description"`
	Version         int              `json:"version"`
	Group           []string         `json:"group"`
	Defaults        N8NNodeDefaults  `json:"defaults"`
	Credentials     []N8NCredential  `json:"credentials"`
	Triggers        []N8NTrigger     `json:"triggers"`
	Actions         []N8NAction      `json:"actions"`
	WebhookPath     string           `json:"webhookPath"`
}

// N8NNodeDefaults holds default display values.
type N8NNodeDefaults struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// N8NCredential describes the credential block for authentication.
type N8NCredential struct {
	Name     string     `json:"name"`
	Required bool       `json:"required"`
	Type     string     `json:"type"` // "oAuth2Api"
	Props    []N8NProp  `json:"properties"`
}

// N8NProp is a single property/field in an n8n node.
type N8NProp struct {
	DisplayName string `json:"displayName"`
	Name        string `json:"name"`
	Type        string `json:"type"` // "string", "dateTime", "options"
	Default     any    `json:"default,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
}

// N8NTrigger is a webhook-based trigger inside the node.
type N8NTrigger struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Type        string `json:"type"` // "webhook"
}

// N8NAction is an executable action inside the node.
type N8NAction struct {
	Name        string    `json:"name"`
	DisplayName string    `json:"displayName"`
	Description string    `json:"description"`
	InputFields []N8NProp `json:"inputFields"`
}

// DefaultN8NNode builds the n8n node definition from the shared
// triggers/actions.
func DefaultN8NNode(baseURL string) *N8NNodeType {
	triggers := make([]N8NTrigger, 0, len(SharedTriggers()))
	for _, t := range SharedTriggers() {
		triggers = append(triggers, N8NTrigger{
			Name:        t.Key,
			DisplayName: t.Label,
			Description: t.Description,
			Type:        "webhook",
		})
	}

	actions := make([]N8NAction, 0, len(SharedActions()))
	for _, a := range SharedActions() {
		props := make([]N8NProp, 0, len(a.InputFields))
		for _, f := range a.InputFields {
			props = append(props, N8NProp{
				DisplayName: f.Label,
				Name:        f.Key,
				Type:        fieldTypeToN8N(f.Type),
				Required:    f.Required,
				Description: f.HelpText,
			})
		}
		actions = append(actions, N8NAction{
			Name:        a.Key,
			DisplayName: a.Label,
			Description: a.Description,
			InputFields: props,
		})
	}

	return &N8NNodeType{
		Name:        "MediaMtxNvr",
		DisplayName: "MediaMTX NVR",
		Description: "Interact with MediaMTX NVR cameras, clips, and alerts.",
		Version:     1,
		Group:       []string{"transform"},
		Defaults: N8NNodeDefaults{
			Name:  "MediaMTX NVR",
			Color: "#1A82e2",
		},
		Credentials: []N8NCredential{
			{
				Name:     "mediaMtxNvrOAuth2",
				Required: true,
				Type:     "oAuth2Api",
				Props: []N8NProp{
					{DisplayName: "Authorization URL", Name: "authUrl", Type: "string", Default: baseURL + "/oauth/authorize"},
					{DisplayName: "Token URL", Name: "accessTokenUrl", Type: "string", Default: baseURL + "/oauth/token"},
					{DisplayName: "Scope", Name: "scope", Type: "string", Default: "cameras:read clips:write notifications:write"},
				},
			},
		},
		Triggers:    triggers,
		Actions:     actions,
		WebhookPath: "/api/v1/integrations/automation/webhooks/n8n",
	}
}

func fieldTypeToN8N(ft string) string {
	switch ft {
	case "datetime":
		return "dateTime"
	case "array":
		return "string" // n8n typically uses comma-separated or expression
	default:
		return "string"
	}
}
