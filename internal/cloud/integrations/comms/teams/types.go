package teams

// Config holds credentials and settings for a Teams connector.
type Config struct {
	WebhookURL string `json:"webhook_url"`
	TenantID   string `json:"tenant_id"` // Azure AD tenant
	AppID      string `json:"app_id,omitempty"`
	AppSecret  string `json:"app_secret,omitempty"`
	// BaseURL overrides the webhook URL for testing.
	BaseURL string `json:"base_url,omitempty"`
}

// AdaptiveCard is a minimal representation of a Microsoft Adaptive Card.
type AdaptiveCard struct {
	Type    string            `json:"type"`
	Version string           `json:"version"`
	Body    []CardElement     `json:"body"`
	Actions []CardAction      `json:"actions,omitempty"`
	Schema  string            `json:"$schema,omitempty"`
}

// CardElement is a single element in an Adaptive Card body.
type CardElement struct {
	Type    string        `json:"type"`
	Text    string        `json:"text,omitempty"`
	Size    string        `json:"size,omitempty"`
	Weight  string        `json:"weight,omitempty"`
	Color   string        `json:"color,omitempty"`
	Wrap    bool          `json:"wrap,omitempty"`
	Facts   []Fact        `json:"facts,omitempty"`
	Columns []Column      `json:"columns,omitempty"`
}

// Fact is a key-value pair in a FactSet.
type Fact struct {
	Title string `json:"title"`
	Value string `json:"value"`
}

// Column is a column in a ColumnSet.
type Column struct {
	Type  string        `json:"type"`
	Width string        `json:"width,omitempty"`
	Items []CardElement `json:"items,omitempty"`
}

// CardAction is an action button on an Adaptive Card.
type CardAction struct {
	Type  string            `json:"type"`
	Title string            `json:"title"`
	URL   string            `json:"url,omitempty"`
	Data  map[string]string `json:"data,omitempty"`
	Verb  string            `json:"verb,omitempty"`
}

// WebhookPayload wraps an Adaptive Card for the Teams Incoming Webhook.
type WebhookPayload struct {
	Type        string         `json:"type"`
	Attachments []Attachment   `json:"attachments"`
}

// Attachment wraps an Adaptive Card in the webhook payload format.
type Attachment struct {
	ContentType string       `json:"contentType"`
	ContentURL  *string      `json:"contentUrl"`
	Content     AdaptiveCard `json:"content"`
}

// InvokePayload represents a Teams card invoke callback when a user
// clicks an Action.Execute button.
type InvokePayload struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value struct {
		Action  string `json:"action"`
		AlertID string `json:"alert_id"`
	} `json:"value"`
	From struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"from"`
	ChannelID string `json:"channelId"`
}

// PostResponse is the HTTP response from the Teams webhook.
type PostResponse struct {
	StatusCode int
	Body       string
}
