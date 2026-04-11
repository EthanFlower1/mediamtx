package subscribers_test

import (
	"context"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/statuspage/subscribers"
)

func openTestDB(t *testing.T) *clouddb.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := "sqlite://" + filepath.Join(dir, "cloud.db")
	d, err := clouddb.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

var seqID int

func testIDGen() string {
	seqID++
	return fmt.Sprintf("test-%04d", seqID)
}

// fakeDispatcher records dispatch calls for assertions.
type fakeDispatcher struct {
	calls []dispatchCall
}

type dispatchCall struct {
	Sub subscribers.Subscriber
	Evt subscribers.StatusEvent
}

func (f *fakeDispatcher) Dispatch(sub subscribers.Subscriber, evt subscribers.StatusEvent) error {
	f.calls = append(f.calls, dispatchCall{Sub: sub, Evt: evt})
	return nil
}

func newService(t *testing.T, dispatcher subscribers.Dispatcher) *subscribers.Service {
	t.Helper()
	db := openTestDB(t)
	svc, err := subscribers.NewService(subscribers.Config{
		DB:         db,
		IDGen:      testIDGen,
		Dispatcher: dispatcher,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func TestSubscribeAndList(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	sub, err := svc.Subscribe(ctx, subscribers.Subscriber{
		TenantID:    "tenant-1",
		ChannelType: subscribers.ChannelEmail,
		ChannelConfig: `{"email":"admin@example.com"}`,
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if sub.SubscriberID == "" {
		t.Fatal("expected subscriber_id to be set")
	}

	subs, err := svc.ListSubscribers(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscriber, got %d", len(subs))
	}
	if subs[0].ChannelType != subscribers.ChannelEmail {
		t.Errorf("expected email, got %s", subs[0].ChannelType)
	}
}

func TestUnsubscribe(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	sub, _ := svc.Subscribe(ctx, subscribers.Subscriber{
		TenantID:    "tenant-1",
		ChannelType: subscribers.ChannelWebhook,
		ChannelConfig: `{"url":"https://example.com/hook"}`,
	})

	if err := svc.Unsubscribe(ctx, "tenant-1", sub.SubscriberID); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}

	// Second unsubscribe should fail.
	err := svc.Unsubscribe(ctx, "tenant-1", sub.SubscriberID)
	if err != subscribers.ErrSubscriberNotFound {
		t.Errorf("expected ErrSubscriberNotFound, got %v", err)
	}

	// List should be empty.
	subs, _ := svc.ListSubscribers(ctx, "tenant-1")
	if len(subs) != 0 {
		t.Errorf("expected 0 subscribers after unsubscribe, got %d", len(subs))
	}
}

func TestConfirmSubscriber(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	sub, _ := svc.Subscribe(ctx, subscribers.Subscriber{
		TenantID:    "tenant-1",
		ChannelType: subscribers.ChannelEmail,
		ChannelConfig: `{"email":"user@example.com"}`,
	})

	if sub.Confirmed {
		t.Fatal("new email subscriber should not be confirmed")
	}

	if err := svc.ConfirmSubscriber(ctx, "tenant-1", sub.SubscriberID); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	got, err := svc.GetSubscriber(ctx, "tenant-1", sub.SubscriberID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.Confirmed {
		t.Error("expected confirmed after ConfirmSubscriber")
	}
}

func TestRSSAutoConfirmed(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	sub, err := svc.Subscribe(ctx, subscribers.Subscriber{
		TenantID:    "tenant-1",
		ChannelType: subscribers.ChannelRSS,
	})
	if err != nil {
		t.Fatalf("subscribe rss: %v", err)
	}
	if !sub.Confirmed {
		t.Error("RSS subscribers should be auto-confirmed")
	}
}

func TestInvalidChannelType(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	_, err := svc.Subscribe(ctx, subscribers.Subscriber{
		TenantID:    "tenant-1",
		ChannelType: "pigeon",
	})
	if err != subscribers.ErrInvalidChannel {
		t.Errorf("expected ErrInvalidChannel, got %v", err)
	}
}

func TestFanOutOnStatusChange(t *testing.T) {
	disp := &fakeDispatcher{}
	svc := newService(t, disp)
	ctx := context.Background()

	// Create two subscribers: one confirmed email, one unconfirmed SMS.
	emailSub, _ := svc.Subscribe(ctx, subscribers.Subscriber{
		TenantID:    "tenant-1",
		ChannelType: subscribers.ChannelEmail,
		ChannelConfig: `{"email":"admin@example.com"}`,
	})
	svc.ConfirmSubscriber(ctx, "tenant-1", emailSub.SubscriberID)

	svc.Subscribe(ctx, subscribers.Subscriber{
		TenantID:    "tenant-1",
		ChannelType: subscribers.ChannelSMS,
		ChannelConfig: `{"phone":"+1555000111"}`,
	})
	// SMS subscriber is NOT confirmed.

	// Create a confirmed Slack subscriber.
	slackSub, _ := svc.Subscribe(ctx, subscribers.Subscriber{
		TenantID:    "tenant-1",
		ChannelType: subscribers.ChannelSlack,
		ChannelConfig: `{"webhook_url":"https://hooks.slack.com/xxx"}`,
	})
	svc.ConfirmSubscriber(ctx, "tenant-1", slackSub.SubscriberID)

	evt := subscribers.StatusEvent{
		TenantID:           "tenant-1",
		EventType:          subscribers.EventIncidentCreated,
		Title:              "API degraded",
		Description:        "The cloud API is experiencing elevated latency.",
		AffectedComponents: `["cloud_api"]`,
		Severity:           "major",
	}

	_, dispatched, err := svc.RecordEvent(ctx, evt)
	if err != nil {
		t.Fatalf("record event: %v", err)
	}

	// Only confirmed email + Slack should be dispatched (not unconfirmed SMS, not RSS).
	if dispatched != 2 {
		t.Errorf("expected 2 dispatches, got %d", dispatched)
	}
	if len(disp.calls) != 2 {
		t.Fatalf("expected 2 dispatcher calls, got %d", len(disp.calls))
	}
}

func TestFanOutComponentFilter(t *testing.T) {
	disp := &fakeDispatcher{}
	svc := newService(t, disp)
	ctx := context.Background()

	// Subscriber watching only "recording" component.
	sub, _ := svc.Subscribe(ctx, subscribers.Subscriber{
		TenantID:        "tenant-1",
		ChannelType:     subscribers.ChannelWebhook,
		ChannelConfig:   `{"url":"https://example.com/hook"}`,
		ComponentFilter: `["recording"]`,
	})
	svc.ConfirmSubscriber(ctx, "tenant-1", sub.SubscriberID)

	// Event affecting cloud_api only — should NOT match.
	_, dispatched, _ := svc.RecordEvent(ctx, subscribers.StatusEvent{
		TenantID:           "tenant-1",
		EventType:          subscribers.EventStatusChange,
		Title:              "API degraded",
		AffectedComponents: `["cloud_api"]`,
	})
	if dispatched != 0 {
		t.Errorf("expected 0 dispatches for non-matching component, got %d", dispatched)
	}

	// Event affecting recording — should match.
	_, dispatched, _ = svc.RecordEvent(ctx, subscribers.StatusEvent{
		TenantID:           "tenant-1",
		EventType:          subscribers.EventStatusChange,
		Title:              "Recording degraded",
		AffectedComponents: `["recording"]`,
	})
	if dispatched != 1 {
		t.Errorf("expected 1 dispatch for matching component, got %d", dispatched)
	}
}

func TestRSSFeedGeneration(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	svc.RecordEvent(ctx, subscribers.StatusEvent{
		TenantID:    "tenant-1",
		EventType:   subscribers.EventIncidentCreated,
		Title:       "API outage",
		Description: "Major API outage detected.",
		Severity:    "critical",
	})
	svc.RecordEvent(ctx, subscribers.StatusEvent{
		TenantID:    "tenant-1",
		EventType:   subscribers.EventIncidentResolved,
		Title:       "API restored",
		Description: "Service restored to normal.",
		Severity:    "critical",
	})

	data, err := svc.RSSFeed(ctx, "tenant-1", "https://status.example.com", 50)
	if err != nil {
		t.Fatalf("rss feed: %v", err)
	}

	// Should be valid XML.
	var feed struct {
		XMLName xml.Name `xml:"rss"`
		Channel struct {
			Title string `xml:"title"`
			Items []struct {
				Title    string `xml:"title"`
				Category string `xml:"category"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(data, &feed); err != nil {
		t.Fatalf("parse rss xml: %v", err)
	}

	if !strings.Contains(feed.Channel.Title, "tenant-1") {
		t.Errorf("expected tenant-1 in title, got %s", feed.Channel.Title)
	}
	if len(feed.Channel.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(feed.Channel.Items))
	}
	// Items should be most recent first.
	if feed.Channel.Items[0].Title != "API restored" {
		t.Errorf("expected most recent first, got %s", feed.Channel.Items[0].Title)
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	svc.Subscribe(ctx, subscribers.Subscriber{
		TenantID:    "tenant-1",
		ChannelType: subscribers.ChannelEmail,
		ChannelConfig: `{"email":"t1@example.com"}`,
	})
	svc.Subscribe(ctx, subscribers.Subscriber{
		TenantID:    "tenant-2",
		ChannelType: subscribers.ChannelSlack,
		ChannelConfig: `{"webhook_url":"https://slack/t2"}`,
	})

	s1, _ := svc.ListSubscribers(ctx, "tenant-1")
	s2, _ := svc.ListSubscribers(ctx, "tenant-2")

	if len(s1) != 1 || s1[0].ChannelType != subscribers.ChannelEmail {
		t.Errorf("tenant-1 should only see email subscriber")
	}
	if len(s2) != 1 || s2[0].ChannelType != subscribers.ChannelSlack {
		t.Errorf("tenant-2 should only see slack subscriber")
	}
}

func TestAllChannelTypes(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	channels := []subscribers.ChannelType{
		subscribers.ChannelEmail,
		subscribers.ChannelSMS,
		subscribers.ChannelWebhook,
		subscribers.ChannelRSS,
		subscribers.ChannelSlack,
		subscribers.ChannelTeams,
	}
	for _, ch := range channels {
		_, err := svc.Subscribe(ctx, subscribers.Subscriber{
			TenantID:    "tenant-1",
			ChannelType: ch,
			ChannelConfig: fmt.Sprintf(`{"type":"%s"}`, ch),
		})
		if err != nil {
			t.Errorf("subscribe %s: %v", ch, err)
		}
	}

	subs, _ := svc.ListSubscribers(ctx, "tenant-1")
	if len(subs) != len(channels) {
		t.Errorf("expected %d subscribers, got %d", len(channels), len(subs))
	}
}

func TestConfirmNotFound(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	err := svc.ConfirmSubscriber(ctx, "tenant-1", "nonexistent")
	if err != subscribers.ErrSubscriberNotFound {
		t.Errorf("expected ErrSubscriberNotFound, got %v", err)
	}
}

func TestListEventsEmpty(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	events, err := svc.ListEvents(ctx, "tenant-1", 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}
