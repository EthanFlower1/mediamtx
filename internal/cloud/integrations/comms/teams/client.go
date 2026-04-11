package teams

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/integrations/comms"
)

// Client implements comms.Sender for Microsoft Teams.
type Client struct {
	cfg  Config
	http *http.Client
}

// NewClient creates a Teams client from the given config.
func NewClient(cfg Config) (*Client, error) {
	if cfg.WebhookURL == "" && cfg.BaseURL == "" {
		return nil, comms.ErrNotConfigured
	}
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// Platform returns comms.PlatformTeams.
func (c *Client) Platform() comms.Platform {
	return comms.PlatformTeams
}

// PostAlert posts an alert as an Adaptive Card via the Teams Incoming Webhook.
func (c *Client) PostAlert(ctx context.Context, channelRef string, alert comms.Alert) (comms.PostResult, error) {
	card := buildAlertCard(alert)
	payload := WebhookPayload{
		Type: "message",
		Attachments: []Attachment{
			{
				ContentType: "application/vnd.microsoft.card.adaptive",
				ContentURL:  nil,
				Content:     card,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return comms.PostResult{Platform: comms.PlatformTeams, ChannelRef: channelRef},
			fmt.Errorf("teams marshal: %w", err)
	}

	webhookURL := c.cfg.WebhookURL
	if c.cfg.BaseURL != "" {
		webhookURL = c.cfg.BaseURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return comms.PostResult{Platform: comms.PlatformTeams, ChannelRef: channelRef}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return comms.PostResult{Platform: comms.PlatformTeams, ChannelRef: channelRef},
			fmt.Errorf("teams post: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusTooManyRequests {
		return comms.PostResult{Platform: comms.PlatformTeams, ChannelRef: channelRef},
			comms.ErrRateLimited
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return comms.PostResult{Platform: comms.PlatformTeams, ChannelRef: channelRef},
			fmt.Errorf("teams webhook returned %d: %s", resp.StatusCode, string(respBody))
	}

	return comms.PostResult{
		Platform:   comms.PlatformTeams,
		ChannelRef: channelRef,
		MessageID:  "", // Teams webhooks don't return a message ID
	}, nil
}

// HandleAction processes an interactive action from Teams.
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

// ParseInvoke converts a Teams invoke payload into a comms.CardAction.
func ParseInvoke(payload InvokePayload, integrationID string) (comms.CardAction, error) {
	var actionType comms.ActionType
	switch payload.Value.Action {
	case "ack":
		actionType = comms.ActionAcknowledge
	case "triage":
		actionType = comms.ActionTriage
	case "watch_clip":
		actionType = comms.ActionWatchClip
	default:
		return comms.CardAction{}, comms.ErrUnsupportedAction
	}

	return comms.CardAction{
		ActionType:    actionType,
		AlertID:       payload.Value.AlertID,
		UserID:        payload.From.ID,
		UserName:      payload.From.Name,
		Platform:      comms.PlatformTeams,
		ChannelRef:    payload.ChannelID,
		IntegrationID: integrationID,
		Timestamp:     time.Now().UTC(),
	}, nil
}

// buildAlertCard creates an Adaptive Card for an NVR alert.
func buildAlertCard(alert comms.Alert) AdaptiveCard {
	body := []CardElement{
		{
			Type:   "TextBlock",
			Text:   alert.Title,
			Size:   "Large",
			Weight: "Bolder",
			Wrap:   true,
		},
		{
			Type: "FactSet",
			Facts: []Fact{
				{Title: "Event", Value: alert.EventType},
				{Title: "Camera", Value: alert.CameraID},
				{Title: "Time", Value: alert.Timestamp.Format(time.RFC3339)},
			},
		},
		{
			Type: "TextBlock",
			Text: alert.Body,
			Wrap: true,
		},
	}

	actions := []CardAction{
		{
			Type:  "Action.Execute",
			Title: "Acknowledge",
			Verb:  "ack",
			Data:  map[string]string{"action": "ack", "alert_id": alert.AlertID},
		},
		{
			Type:  "Action.Execute",
			Title: "Triage",
			Verb:  "triage",
			Data:  map[string]string{"action": "triage", "alert_id": alert.AlertID},
		},
	}

	if alert.ClipURL != "" {
		actions = append(actions, CardAction{
			Type:  "Action.OpenUrl",
			Title: "Watch Clip",
			URL:   alert.ClipURL,
		})
	}

	return AdaptiveCard{
		Type:    "AdaptiveCard",
		Version: "1.4",
		Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
		Body:    body,
		Actions: actions,
	}
}
