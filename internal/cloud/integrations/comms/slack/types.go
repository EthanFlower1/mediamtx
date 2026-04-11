package slack

import "time"

// Config holds credentials and settings for a Slack app installation.
type Config struct {
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	SigningSecret string `json:"signing_secret"`
	BotToken      string `json:"bot_token"`
	TeamID        string `json:"team_id"`
	TeamName      string `json:"team_name"`
	RedirectURL   string `json:"redirect_url"`
	// BaseURL overrides the Slack API base for testing.
	BaseURL string `json:"base_url,omitempty"`
}

// OAuthResponse is the token response from Slack's oauth.v2.access endpoint.
type OAuthResponse struct {
	OK          bool   `json:"ok"`
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	BotUserID   string `json:"bot_user_id"`
	Team        struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"team"`
	Error string `json:"error,omitempty"`
}

// BlockKitMessage is a minimal representation of a Slack Block Kit message.
type BlockKitMessage struct {
	Channel string  `json:"channel"`
	Text    string  `json:"text"` // fallback text
	Blocks  []Block `json:"blocks"`
}

// Block is a single Block Kit block.
type Block struct {
	Type     string    `json:"type"`
	Text     *TextObj  `json:"text,omitempty"`
	BlockID  string    `json:"block_id,omitempty"`
	Elements []Element `json:"elements,omitempty"`
	Fields   []TextObj `json:"fields,omitempty"`
}

// TextObj is a Block Kit text object.
type TextObj struct {
	Type string `json:"type"` // "plain_text" or "mrkdwn"
	Text string `json:"text"`
}

// Element is a Block Kit interactive element (button, etc).
type Element struct {
	Type     string   `json:"type"`
	Text     *TextObj `json:"text,omitempty"`
	ActionID string   `json:"action_id,omitempty"`
	URL      string   `json:"url,omitempty"`
	Value    string   `json:"value,omitempty"`
	Style    string   `json:"style,omitempty"`
}

// PostMessageResponse is the Slack API response for chat.postMessage.
type PostMessageResponse struct {
	OK        bool   `json:"ok"`
	Channel   string `json:"channel"`
	Timestamp string `json:"ts"`
	Error     string `json:"error,omitempty"`
}

// InteractionPayload is the top-level payload Slack sends when a user
// interacts with a Block Kit element.
type InteractionPayload struct {
	Type    string `json:"type"`
	User    User   `json:"user"`
	Channel struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"channel"`
	Actions []InteractionAction `json:"actions"`
	Team    struct {
		ID string `json:"id"`
	} `json:"team"`
	TriggerID string `json:"trigger_id"`
}

// InteractionAction is a single action within an interaction payload.
type InteractionAction struct {
	ActionID string `json:"action_id"`
	Value    string `json:"value"`
	Type     string `json:"type"`
}

// User is a Slack user reference.
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// SlashCommand represents a parsed Slack slash command request.
type SlashCommand struct {
	Command     string `json:"command"`
	Text        string `json:"text"`
	UserID      string `json:"user_id"`
	UserName    string `json:"user_name"`
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	TeamID      string `json:"team_id"`
	TriggerID   string `json:"trigger_id"`
	Timestamp   time.Time
}
