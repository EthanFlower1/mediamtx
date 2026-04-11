package teams_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/integrations/comms"
	"github.com/bluenviron/mediamtx/internal/cloud/integrations/comms/teams"
)

func TestNewClientNotConfigured(t *testing.T) {
	_, err := teams.NewClient(teams.Config{})
	if err != comms.ErrNotConfigured {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
}

func TestPlatform(t *testing.T) {
	c, _ := teams.NewClient(teams.Config{WebhookURL: "https://example.com/webhook"})
	if c.Platform() != comms.PlatformTeams {
		t.Errorf("expected teams, got %s", c.Platform())
	}
}

func TestPostAlert(t *testing.T) {
	var received teams.WebhookPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("bad content type: %s", r.Header.Get("Content-Type"))
		}
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("1"))
	}))
	defer srv.Close()

	c, err := teams.NewClient(teams.Config{BaseURL: srv.URL})
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

	result, err := c.PostAlert(context.Background(), "teams-general", alert)
	if err != nil {
		t.Fatalf("post alert: %v", err)
	}
	if result.Platform != comms.PlatformTeams {
		t.Errorf("expected teams platform")
	}

	if len(received.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(received.Attachments))
	}
	card := received.Attachments[0].Content
	if card.Type != "AdaptiveCard" {
		t.Errorf("expected AdaptiveCard, got %s", card.Type)
	}
	if card.Version != "1.4" {
		t.Errorf("expected version 1.4, got %s", card.Version)
	}
	// body: title, factset, body text = 3 elements
	if len(card.Body) != 3 {
		t.Errorf("expected 3 body elements, got %d", len(card.Body))
	}
	// actions: ack, triage, watch clip = 3
	if len(card.Actions) != 3 {
		t.Errorf("expected 3 actions (ack, triage, watch clip), got %d", len(card.Actions))
	}
}

func TestPostAlertNoClipURL(t *testing.T) {
	var received teams.WebhookPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, _ := teams.NewClient(teams.Config{BaseURL: srv.URL})

	c.PostAlert(context.Background(), "ch", comms.Alert{
		AlertID: "a1", EventType: "camera.offline", Timestamp: time.Now(),
	})

	card := received.Attachments[0].Content
	// Without clip URL: ack, triage = 2 actions
	if len(card.Actions) != 2 {
		t.Errorf("expected 2 actions without clip URL, got %d", len(card.Actions))
	}
}

func TestPostAlertRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c, _ := teams.NewClient(teams.Config{BaseURL: srv.URL})

	_, err := c.PostAlert(context.Background(), "ch", comms.Alert{
		AlertID: "a1", Timestamp: time.Now(),
	})
	if err != comms.ErrRateLimited {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}
}

func TestPostAlertServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	c, _ := teams.NewClient(teams.Config{BaseURL: srv.URL})

	_, err := c.PostAlert(context.Background(), "ch", comms.Alert{
		AlertID: "a1", Timestamp: time.Now(),
	})
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestHandleAction(t *testing.T) {
	c, _ := teams.NewClient(teams.Config{WebhookURL: "https://example.com/webhook"})

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
	c, _ := teams.NewClient(teams.Config{WebhookURL: "https://example.com/webhook"})

	_, err := c.HandleAction(context.Background(), comms.CardAction{
		ActionType: "unknown",
	})
	if err != comms.ErrUnsupportedAction {
		t.Errorf("expected ErrUnsupportedAction, got %v", err)
	}
}

func TestParseInvoke(t *testing.T) {
	payload := teams.InvokePayload{
		Type: "invoke",
		Name: "adaptiveCard/action",
	}
	payload.Value.Action = "ack"
	payload.Value.AlertID = "alert-001"
	payload.From.ID = "user-123"
	payload.From.Name = "Test User"
	payload.ChannelID = "teams-ch-1"

	action, err := teams.ParseInvoke(payload, "teams-1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if action.ActionType != comms.ActionAcknowledge {
		t.Errorf("expected ack, got %s", action.ActionType)
	}
	if action.AlertID != "alert-001" {
		t.Errorf("expected alert-001, got %s", action.AlertID)
	}
	if action.Platform != comms.PlatformTeams {
		t.Errorf("expected teams platform")
	}
	if action.IntegrationID != "teams-1" {
		t.Errorf("expected teams-1, got %s", action.IntegrationID)
	}
}

func TestParseInvokeAllActions(t *testing.T) {
	tests := []struct {
		action   string
		expected comms.ActionType
	}{
		{"ack", comms.ActionAcknowledge},
		{"triage", comms.ActionTriage},
		{"watch_clip", comms.ActionWatchClip},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			payload := teams.InvokePayload{}
			payload.Value.Action = tt.action
			payload.Value.AlertID = "a1"

			action, err := teams.ParseInvoke(payload, "teams-1")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if action.ActionType != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, action.ActionType)
			}
		})
	}
}

func TestParseInvokeUnknown(t *testing.T) {
	payload := teams.InvokePayload{}
	payload.Value.Action = "unknown"

	_, err := teams.ParseInvoke(payload, "teams-1")
	if err != comms.ErrUnsupportedAction {
		t.Errorf("expected ErrUnsupportedAction, got %v", err)
	}
}
