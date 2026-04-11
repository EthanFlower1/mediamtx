package slack

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/integrations/comms"
)

const (
	defaultBaseURL = "https://slack.com/api"
	oauthEndpoint  = "/oauth.v2.access"
	postMsgEndpoint = "/chat.postMessage"
)

// Client implements comms.Sender for Slack.
type Client struct {
	cfg    Config
	http   *http.Client
	base   string
}

// NewClient creates a Slack client from the given config.
func NewClient(cfg Config) (*Client, error) {
	if cfg.BotToken == "" && cfg.ClientID == "" {
		return nil, comms.ErrNotConfigured
	}
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 15 * time.Second},
		base: base,
	}, nil
}

// Platform returns comms.PlatformSlack.
func (c *Client) Platform() comms.Platform {
	return comms.PlatformSlack
}

// ExchangeCode exchanges an OAuth authorization code for an access token.
func (c *Client) ExchangeCode(ctx context.Context, code string) (*OAuthResponse, error) {
	data := url.Values{
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
		"code":          {code},
		"redirect_uri":  {c.cfg.RedirectURL},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+oauthEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("slack oauth: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("slack oauth: %w", err)
	}
	defer resp.Body.Close()

	var oauthResp OAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&oauthResp); err != nil {
		return nil, fmt.Errorf("slack oauth decode: %w", err)
	}
	if !oauthResp.OK {
		return nil, fmt.Errorf("slack oauth error: %s", oauthResp.Error)
	}
	return &oauthResp, nil
}

// PostAlert posts an alert as a Block Kit message to a Slack channel.
func (c *Client) PostAlert(ctx context.Context, channelRef string, alert comms.Alert) (comms.PostResult, error) {
	msg := buildAlertMessage(channelRef, alert)

	body, err := json.Marshal(msg)
	if err != nil {
		return comms.PostResult{Platform: comms.PlatformSlack, ChannelRef: channelRef},
			fmt.Errorf("slack marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+postMsgEndpoint, bytes.NewReader(body))
	if err != nil {
		return comms.PostResult{Platform: comms.PlatformSlack, ChannelRef: channelRef}, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+c.cfg.BotToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return comms.PostResult{Platform: comms.PlatformSlack, ChannelRef: channelRef},
			fmt.Errorf("slack post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return comms.PostResult{Platform: comms.PlatformSlack, ChannelRef: channelRef},
			comms.ErrRateLimited
	}

	var postResp PostMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&postResp); err != nil {
		return comms.PostResult{Platform: comms.PlatformSlack, ChannelRef: channelRef},
			fmt.Errorf("slack decode: %w", err)
	}
	if !postResp.OK {
		return comms.PostResult{Platform: comms.PlatformSlack, ChannelRef: channelRef},
			fmt.Errorf("slack api: %s", postResp.Error)
	}

	return comms.PostResult{
		Platform:   comms.PlatformSlack,
		ChannelRef: channelRef,
		MessageID:  postResp.Timestamp,
	}, nil
}

// HandleAction processes an interactive action from Slack.
func (c *Client) HandleAction(ctx context.Context, action comms.CardAction) (comms.ActionResult, error) {
	switch action.ActionType {
	case comms.ActionAcknowledge:
		return comms.ActionResult{OK: true, Message: fmt.Sprintf("Alert %s acknowledged by %s", action.AlertID, action.UserName)}, nil
	case comms.ActionTriage:
		return comms.ActionResult{OK: true, Message: fmt.Sprintf("Alert %s triaged by %s", action.AlertID, action.UserName)}, nil
	case comms.ActionWatchClip:
		return comms.ActionResult{OK: true, Message: "Opening clip viewer..."}, nil
	default:
		return comms.ActionResult{}, comms.ErrUnsupportedAction
	}
}

// HandleSlashCommand processes a /kaivue slash command.
func (c *Client) HandleSlashCommand(_ context.Context, cmd SlashCommand) (string, error) {
	parts := strings.Fields(cmd.Text)
	if len(parts) == 0 {
		return "Usage: /kaivue [status|alerts]", nil
	}
	switch parts[0] {
	case "status":
		return "Kaivue NVR: all systems operational.", nil
	case "alerts":
		return "Recent alerts: use the web dashboard for full history.", nil
	default:
		return fmt.Sprintf("Unknown subcommand: %s. Try /kaivue status or /kaivue alerts", parts[0]), nil
	}
}

// VerifySignature validates a Slack request signature (v0 signing secret).
func (c *Client) VerifySignature(timestamp, body, signature string) error {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return comms.ErrSignatureInvalid
	}
	// Reject requests older than 5 minutes.
	if abs(time.Now().Unix()-ts) > 300 {
		return comms.ErrSignatureInvalid
	}

	baseString := fmt.Sprintf("v0:%s:%s", timestamp, body)
	mac := hmac.New(sha256.New, []byte(c.cfg.SigningSecret))
	mac.Write([]byte(baseString))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return comms.ErrSignatureInvalid
	}
	return nil
}

// ParseInteraction parses a Slack interaction payload and converts it to
// a comms.CardAction.
func ParseInteraction(payload InteractionPayload, integrationID string) (comms.CardAction, error) {
	if len(payload.Actions) == 0 {
		return comms.CardAction{}, comms.ErrUnsupportedAction
	}
	a := payload.Actions[0]

	var actionType comms.ActionType
	switch a.ActionID {
	case "ack_alert":
		actionType = comms.ActionAcknowledge
	case "triage_alert":
		actionType = comms.ActionTriage
	case "watch_clip":
		actionType = comms.ActionWatchClip
	default:
		return comms.CardAction{}, comms.ErrUnsupportedAction
	}

	return comms.CardAction{
		ActionType:    actionType,
		AlertID:       a.Value,
		UserID:        payload.User.ID,
		UserName:      payload.User.Name,
		Platform:      comms.PlatformSlack,
		ChannelRef:    payload.Channel.ID,
		IntegrationID: integrationID,
		Timestamp:     time.Now().UTC(),
	}, nil
}

// buildAlertMessage creates a Block Kit message for an NVR alert.
func buildAlertMessage(channel string, alert comms.Alert) BlockKitMessage {
	blocks := []Block{
		{
			Type: "header",
			Text: &TextObj{Type: "plain_text", Text: alert.Title},
		},
		{
			Type: "section",
			Fields: []TextObj{
				{Type: "mrkdwn", Text: fmt.Sprintf("*Event:*\n%s", alert.EventType)},
				{Type: "mrkdwn", Text: fmt.Sprintf("*Camera:*\n%s", alert.CameraID)},
			},
		},
		{
			Type: "section",
			Text: &TextObj{Type: "mrkdwn", Text: alert.Body},
		},
	}

	// Action buttons
	actions := Block{
		Type:    "actions",
		BlockID: "alert_actions_" + alert.AlertID,
		Elements: []Element{
			{
				Type:     "button",
				Text:     &TextObj{Type: "plain_text", Text: "Acknowledge"},
				ActionID: "ack_alert",
				Value:    alert.AlertID,
				Style:    "primary",
			},
			{
				Type:     "button",
				Text:     &TextObj{Type: "plain_text", Text: "Triage"},
				ActionID: "triage_alert",
				Value:    alert.AlertID,
			},
		},
	}

	if alert.ClipURL != "" {
		actions.Elements = append(actions.Elements, Element{
			Type:     "button",
			Text:     &TextObj{Type: "plain_text", Text: "Watch Clip"},
			ActionID: "watch_clip",
			URL:      alert.ClipURL,
			Value:    alert.AlertID,
		})
	}

	blocks = append(blocks, actions)

	return BlockKitMessage{
		Channel: channel,
		Text:    alert.Title, // fallback
		Blocks:  blocks,
	}
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// readBody reads and returns the body as a string, rewinding for later use.
func readBody(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
