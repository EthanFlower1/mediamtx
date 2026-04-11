package slack_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/integrations/comms"
	"github.com/bluenviron/mediamtx/internal/cloud/integrations/comms/slack"
)

func TestNewClientNotConfigured(t *testing.T) {
	_, err := slack.NewClient(slack.Config{})
	if err != comms.ErrNotConfigured {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
}

func TestPlatform(t *testing.T) {
	c, _ := slack.NewClient(slack.Config{BotToken: "xoxb-test"})
	if c.Platform() != comms.PlatformSlack {
		t.Errorf("expected slack, got %s", c.Platform())
	}
}

func TestPostAlert(t *testing.T) {
	var received slack.BlockKitMessage

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat.postMessage" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer xoxb-test" {
			t.Errorf("bad auth header")
		}

		json.NewDecoder(r.Body).Decode(&received)
		json.NewEncoder(w).Encode(slack.PostMessageResponse{
			OK:        true,
			Channel:   "C12345",
			Timestamp: "1234567890.123456",
		})
	}))
	defer srv.Close()

	c, err := slack.NewClient(slack.Config{
		BotToken: "xoxb-test",
		BaseURL:  srv.URL,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	alert := comms.Alert{
		AlertID:   "alert-001",
		TenantID:  "tenant-1",
		EventType: "camera.offline",
		CameraID:  "cam-front",
		Title:     "Camera Offline",
		Body:      "Front camera went offline.",
		ClipURL:   "https://app.kaivue.com/clips/abc",
		Timestamp: time.Now(),
	}

	result, err := c.PostAlert(context.Background(), "C12345", alert)
	if err != nil {
		t.Fatalf("post alert: %v", err)
	}
	if result.MessageID != "1234567890.123456" {
		t.Errorf("expected message ID, got %s", result.MessageID)
	}
	if result.Platform != comms.PlatformSlack {
		t.Errorf("expected slack platform")
	}
	if received.Channel != "C12345" {
		t.Errorf("expected channel C12345, got %s", received.Channel)
	}
	// Should have header, section fields, section body, actions = 4 blocks
	if len(received.Blocks) != 4 {
		t.Errorf("expected 4 blocks, got %d", len(received.Blocks))
	}
}

func TestPostAlertRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c, _ := slack.NewClient(slack.Config{BotToken: "xoxb-test", BaseURL: srv.URL})

	_, err := c.PostAlert(context.Background(), "C12345", comms.Alert{
		AlertID: "a1", Timestamp: time.Now(),
	})
	if err != comms.ErrRateLimited {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}
}

func TestPostAlertAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(slack.PostMessageResponse{OK: false, Error: "channel_not_found"})
	}))
	defer srv.Close()

	c, _ := slack.NewClient(slack.Config{BotToken: "xoxb-test", BaseURL: srv.URL})

	_, err := c.PostAlert(context.Background(), "C12345", comms.Alert{
		AlertID: "a1", Timestamp: time.Now(),
	})
	if err == nil {
		t.Error("expected error for API failure")
	}
}

func TestHandleAction(t *testing.T) {
	c, _ := slack.NewClient(slack.Config{BotToken: "xoxb-test"})

	tests := []struct {
		action comms.ActionType
		ok     bool
	}{
		{comms.ActionAcknowledge, true},
		{comms.ActionTriage, true},
		{comms.ActionWatchClip, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			result, err := c.HandleAction(context.Background(), comms.CardAction{
				ActionType: tt.action,
				AlertID:    "alert-001",
				UserName:   "testuser",
			})
			if err != nil {
				t.Fatalf("handle action: %v", err)
			}
			if result.OK != tt.ok {
				t.Errorf("expected OK=%v", tt.ok)
			}
		})
	}
}

func TestHandleActionUnsupported(t *testing.T) {
	c, _ := slack.NewClient(slack.Config{BotToken: "xoxb-test"})

	_, err := c.HandleAction(context.Background(), comms.CardAction{
		ActionType: "unknown",
	})
	if err != comms.ErrUnsupportedAction {
		t.Errorf("expected ErrUnsupportedAction, got %v", err)
	}
}

func TestSlashCommand(t *testing.T) {
	c, _ := slack.NewClient(slack.Config{BotToken: "xoxb-test"})

	tests := []struct {
		text     string
		contains string
	}{
		{"status", "operational"},
		{"alerts", "dashboard"},
		{"unknown", "Unknown subcommand"},
		{"", "Usage"},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			resp, err := c.HandleSlashCommand(context.Background(), slack.SlashCommand{Text: tt.text})
			if err != nil {
				t.Fatalf("slash command: %v", err)
			}
			if resp == "" {
				t.Error("expected non-empty response")
			}
		})
	}
}

func TestVerifySignature(t *testing.T) {
	secret := "test-signing-secret"
	c, _ := slack.NewClient(slack.Config{BotToken: "xoxb-test", SigningSecret: secret})

	ts := fmt.Sprintf("%d", time.Now().Unix())
	body := `{"test":"data"}`
	baseString := fmt.Sprintf("v0:%s:%s", ts, body)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(baseString))
	sig := "v0=" + hex.EncodeToString(mac.Sum(nil))

	if err := c.VerifySignature(ts, body, sig); err != nil {
		t.Errorf("valid signature rejected: %v", err)
	}
}

func TestVerifySignatureInvalid(t *testing.T) {
	c, _ := slack.NewClient(slack.Config{BotToken: "xoxb-test", SigningSecret: "secret"})

	err := c.VerifySignature(fmt.Sprintf("%d", time.Now().Unix()), "body", "v0=invalid")
	if err != comms.ErrSignatureInvalid {
		t.Errorf("expected ErrSignatureInvalid, got %v", err)
	}
}

func TestVerifySignatureExpired(t *testing.T) {
	c, _ := slack.NewClient(slack.Config{BotToken: "xoxb-test", SigningSecret: "secret"})

	oldTs := fmt.Sprintf("%d", time.Now().Add(-10*time.Minute).Unix())
	err := c.VerifySignature(oldTs, "body", "v0=whatever")
	if err != comms.ErrSignatureInvalid {
		t.Errorf("expected ErrSignatureInvalid for old timestamp, got %v", err)
	}
}

func TestParseInteraction(t *testing.T) {
	payload := slack.InteractionPayload{
		User: slack.User{ID: "U123", Name: "testuser"},
		Actions: []slack.InteractionAction{
			{ActionID: "ack_alert", Value: "alert-001"},
		},
	}
	payload.Channel.ID = "C12345"

	action, err := slack.ParseInteraction(payload, "slack-1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if action.ActionType != comms.ActionAcknowledge {
		t.Errorf("expected ack, got %s", action.ActionType)
	}
	if action.AlertID != "alert-001" {
		t.Errorf("expected alert-001, got %s", action.AlertID)
	}
	if action.Platform != comms.PlatformSlack {
		t.Errorf("expected slack platform")
	}
}

func TestParseInteractionNoActions(t *testing.T) {
	_, err := slack.ParseInteraction(slack.InteractionPayload{}, "slack-1")
	if err != comms.ErrUnsupportedAction {
		t.Errorf("expected ErrUnsupportedAction, got %v", err)
	}
}

func TestParseInteractionUnknownAction(t *testing.T) {
	payload := slack.InteractionPayload{
		Actions: []slack.InteractionAction{
			{ActionID: "unknown_action", Value: "alert-001"},
		},
	}
	_, err := slack.ParseInteraction(payload, "slack-1")
	if err != comms.ErrUnsupportedAction {
		t.Errorf("expected ErrUnsupportedAction, got %v", err)
	}
}

func TestExchangeCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth.v2.access" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(slack.OAuthResponse{
			OK:          true,
			AccessToken: "xoxb-new-token",
			BotUserID:   "B123",
		})
	}))
	defer srv.Close()

	c, _ := slack.NewClient(slack.Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		BaseURL:      srv.URL,
	})

	resp, err := c.ExchangeCode(context.Background(), "auth-code-123")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if resp.AccessToken != "xoxb-new-token" {
		t.Errorf("expected xoxb-new-token, got %s", resp.AccessToken)
	}
}

func TestExchangeCodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(slack.OAuthResponse{OK: false, Error: "invalid_code"})
	}))
	defer srv.Close()

	c, _ := slack.NewClient(slack.Config{ClientID: "cid", ClientSecret: "cs", BaseURL: srv.URL})

	_, err := c.ExchangeCode(context.Background(), "bad-code")
	if err == nil {
		t.Error("expected error for invalid code")
	}
}
