package comms_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/integrations/comms"
)

var seqID int

func testIDGen() string {
	seqID++
	return fmt.Sprintf("test-%04d", seqID)
}

// stubSender implements comms.Sender for testing.
type stubSender struct {
	platform comms.Platform
	posted   []comms.Alert
	actions  []comms.CardAction
	postErr  error
}

func (s *stubSender) Platform() comms.Platform { return s.platform }

func (s *stubSender) PostAlert(_ context.Context, channelRef string, alert comms.Alert) (comms.PostResult, error) {
	s.posted = append(s.posted, alert)
	if s.postErr != nil {
		return comms.PostResult{Platform: s.platform, ChannelRef: channelRef}, s.postErr
	}
	return comms.PostResult{
		Platform:   s.platform,
		ChannelRef: channelRef,
		MessageID:  "msg-" + alert.AlertID,
	}, nil
}

func (s *stubSender) HandleAction(_ context.Context, action comms.CardAction) (comms.ActionResult, error) {
	s.actions = append(s.actions, action)
	return comms.ActionResult{OK: true, Message: "handled"}, nil
}

func TestRouterDispatch(t *testing.T) {
	r := comms.NewRouter(comms.RouterConfig{IDGen: testIDGen})

	slackSender := &stubSender{platform: comms.PlatformSlack}
	teamsSender := &stubSender{platform: comms.PlatformTeams}

	r.RegisterSender("slack-1", slackSender)
	r.RegisterSender("teams-1", teamsSender)

	// Rule: camera.offline -> Slack
	r.UpsertRule(comms.RoutingRule{
		TenantID:      "tenant-1",
		EventTypes:    []string{"camera.offline"},
		Platform:      comms.PlatformSlack,
		ChannelRef:    "C12345",
		IntegrationID: "slack-1",
		Enabled:       true,
	})
	// Rule: all events -> Teams
	r.UpsertRule(comms.RoutingRule{
		TenantID:      "tenant-1",
		EventTypes:    []string{"*"},
		Platform:      comms.PlatformTeams,
		ChannelRef:    "teams-general",
		IntegrationID: "teams-1",
		Enabled:       true,
	})

	alert := comms.Alert{
		AlertID:   "alert-001",
		TenantID:  "tenant-1",
		EventType: "camera.offline",
		CameraID:  "cam-front",
		Title:     "Camera Offline",
		Body:      "Front camera went offline.",
		Timestamp: time.Now(),
	}

	results := r.Dispatch(context.Background(), alert)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if len(slackSender.posted) != 1 {
		t.Errorf("expected 1 Slack post, got %d", len(slackSender.posted))
	}
	if len(teamsSender.posted) != 1 {
		t.Errorf("expected 1 Teams post, got %d", len(teamsSender.posted))
	}
}

func TestRouterDispatchNoMatch(t *testing.T) {
	r := comms.NewRouter(comms.RouterConfig{IDGen: testIDGen})

	slackSender := &stubSender{platform: comms.PlatformSlack}
	r.RegisterSender("slack-1", slackSender)

	r.UpsertRule(comms.RoutingRule{
		TenantID:      "tenant-1",
		EventTypes:    []string{"camera.offline"},
		Platform:      comms.PlatformSlack,
		ChannelRef:    "C12345",
		IntegrationID: "slack-1",
		Enabled:       true,
	})

	alert := comms.Alert{
		AlertID:   "alert-002",
		TenantID:  "tenant-1",
		EventType: "motion.detected",
		CameraID:  "cam-back",
		Title:     "Motion Detected",
		Body:      "Motion in backyard.",
		Timestamp: time.Now(),
	}

	results := r.Dispatch(context.Background(), alert)
	if len(results) != 0 {
		t.Fatalf("expected 0 results for unmatched event, got %d", len(results))
	}
	if len(slackSender.posted) != 0 {
		t.Errorf("expected 0 Slack posts, got %d", len(slackSender.posted))
	}
}

func TestRouterDisabledRule(t *testing.T) {
	r := comms.NewRouter(comms.RouterConfig{IDGen: testIDGen})

	sender := &stubSender{platform: comms.PlatformSlack}
	r.RegisterSender("slack-1", sender)

	r.UpsertRule(comms.RoutingRule{
		TenantID:      "tenant-1",
		EventTypes:    []string{"*"},
		Platform:      comms.PlatformSlack,
		ChannelRef:    "C12345",
		IntegrationID: "slack-1",
		Enabled:       false, // disabled
	})

	results := r.Dispatch(context.Background(), comms.Alert{
		AlertID:   "alert-003",
		TenantID:  "tenant-1",
		EventType: "camera.offline",
		Timestamp: time.Now(),
	})

	if len(results) != 0 {
		t.Fatalf("expected 0 results for disabled rule, got %d", len(results))
	}
}

func TestRouterMissingSender(t *testing.T) {
	r := comms.NewRouter(comms.RouterConfig{IDGen: testIDGen})

	// Rule referencing a non-registered sender
	r.UpsertRule(comms.RoutingRule{
		TenantID:      "tenant-1",
		EventTypes:    []string{"*"},
		Platform:      comms.PlatformSlack,
		ChannelRef:    "C12345",
		IntegrationID: "nonexistent",
		Enabled:       true,
	})

	results := r.Dispatch(context.Background(), comms.Alert{
		AlertID:   "alert-004",
		TenantID:  "tenant-1",
		EventType: "camera.offline",
		Timestamp: time.Now(),
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err == nil {
		t.Error("expected error for missing sender")
	}
}

func TestRouterUpsertAndDelete(t *testing.T) {
	r := comms.NewRouter(comms.RouterConfig{IDGen: testIDGen})

	rule, err := r.UpsertRule(comms.RoutingRule{
		TenantID:      "tenant-1",
		EventTypes:    []string{"*"},
		Platform:      comms.PlatformSlack,
		ChannelRef:    "C12345",
		IntegrationID: "slack-1",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if rule.RuleID == "" {
		t.Fatal("expected rule_id to be set")
	}

	rules := r.ListRules("tenant-1")
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	if err := r.DeleteRule("tenant-1", rule.RuleID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	rules = r.ListRules("tenant-1")
	if len(rules) != 0 {
		t.Fatalf("expected 0 rules after delete, got %d", len(rules))
	}
}

func TestRouterDeleteNotFound(t *testing.T) {
	r := comms.NewRouter(comms.RouterConfig{IDGen: testIDGen})
	err := r.DeleteRule("tenant-1", "nonexistent")
	if err != comms.ErrRuleNotFound {
		t.Errorf("expected ErrRuleNotFound, got %v", err)
	}
}

func TestRouterHandleAction(t *testing.T) {
	r := comms.NewRouter(comms.RouterConfig{IDGen: testIDGen})

	sender := &stubSender{platform: comms.PlatformSlack}
	r.RegisterSender("slack-1", sender)

	result, err := r.HandleAction(context.Background(), comms.CardAction{
		ActionType:    comms.ActionAcknowledge,
		AlertID:       "alert-005",
		UserID:        "U123",
		UserName:      "testuser",
		IntegrationID: "slack-1",
	})
	if err != nil {
		t.Fatalf("handle action: %v", err)
	}
	if !result.OK {
		t.Error("expected OK result")
	}
	if len(sender.actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(sender.actions))
	}
}

func TestRouterHandleActionMissingSender(t *testing.T) {
	r := comms.NewRouter(comms.RouterConfig{IDGen: testIDGen})

	_, err := r.HandleAction(context.Background(), comms.CardAction{
		IntegrationID: "nonexistent",
	})
	if err == nil {
		t.Error("expected error for missing sender")
	}
}

func TestRouterCrossTenantIsolation(t *testing.T) {
	r := comms.NewRouter(comms.RouterConfig{IDGen: testIDGen})

	s1 := &stubSender{platform: comms.PlatformSlack}
	s2 := &stubSender{platform: comms.PlatformTeams}
	r.RegisterSender("slack-t1", s1)
	r.RegisterSender("teams-t2", s2)

	r.UpsertRule(comms.RoutingRule{
		TenantID: "tenant-1", EventTypes: []string{"*"},
		Platform: comms.PlatformSlack, ChannelRef: "C1",
		IntegrationID: "slack-t1", Enabled: true,
	})
	r.UpsertRule(comms.RoutingRule{
		TenantID: "tenant-2", EventTypes: []string{"*"},
		Platform: comms.PlatformTeams, ChannelRef: "T1",
		IntegrationID: "teams-t2", Enabled: true,
	})

	// Dispatch for tenant-1 only
	r.Dispatch(context.Background(), comms.Alert{
		AlertID: "a1", TenantID: "tenant-1", EventType: "camera.offline",
		Timestamp: time.Now(),
	})

	if len(s1.posted) != 1 {
		t.Errorf("tenant-1 slack: expected 1 post, got %d", len(s1.posted))
	}
	if len(s2.posted) != 0 {
		t.Errorf("tenant-2 teams: expected 0 posts, got %d", len(s2.posted))
	}
}

func TestUpsertRuleValidation(t *testing.T) {
	r := comms.NewRouter(comms.RouterConfig{IDGen: testIDGen})

	tests := []struct {
		name string
		rule comms.RoutingRule
	}{
		{"missing tenant", comms.RoutingRule{EventTypes: []string{"*"}, IntegrationID: "x"}},
		{"missing events", comms.RoutingRule{TenantID: "t1", IntegrationID: "x"}},
		{"missing integration", comms.RoutingRule{TenantID: "t1", EventTypes: []string{"*"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := r.UpsertRule(tt.rule)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}
