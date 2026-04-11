package summaries_test

import (
	"context"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/ml/summaries"
	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

// fakeDispatcher records every Dispatch call so tests can assert on the
// message contents.
type fakeDispatcher struct {
	mu       sync.Mutex
	messages []notifications.Message
}

func (f *fakeDispatcher) Dispatch(_ context.Context, msg notifications.Message) ([]notifications.DeliveryResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, msg)
	return []notifications.DeliveryResult{
		{
			MessageID: msg.ID,
			Recipient: msg.To[0].Address,
			State:     notifications.DeliveryStateDelivered,
			Timestamp: time.Now().UTC(),
		},
	}, nil
}

func (f *fakeDispatcher) Messages() []notifications.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]notifications.Message, len(f.messages))
	copy(out, f.messages)
	return out
}

// TestDeliveryService_DispatcherCalled verifies that when a dispatcher is
// wired, deliverToChannel constructs a real Message and calls Dispatch.
func TestDeliveryService_DispatcherCalled(t *testing.T) {
	var buf strings.Builder
	logger := log.New(&buf, "", 0)

	ds := summaries.NewDeliveryService(nil, logger)

	fd := &fakeDispatcher{}
	ds.SetDispatcher(fd)

	s := &summaries.Summary{
		SummaryID:   "sum-1",
		TenantID:    "tenant-abc",
		Period:      summaries.PeriodDaily,
		StartTime:   time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
		Text:        "Quiet day. 3 motion events on cam-1.",
		EventCount:  3,
		GeneratedAt: time.Date(2026, 4, 10, 6, 0, 0, 0, time.UTC),
	}

	targets := []notifications.DeliveryTarget{
		{
			UserID: "user-1@example.com",
			Channel: notifications.Channel{
				ChannelID:   "ch-1",
				TenantID:    "tenant-abc",
				ChannelType: notifications.ChannelEmail,
				Enabled:     true,
			},
		},
	}

	err := ds.DeliverToTargets(context.Background(), s, targets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := fd.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 dispatched message, got %d", len(msgs))
	}

	msg := msgs[0]
	if msg.TenantID != "tenant-abc" {
		t.Errorf("expected tenant-abc, got %s", msg.TenantID)
	}
	if msg.Type != notifications.MessageTypeEmail {
		t.Errorf("expected email type, got %s", msg.Type)
	}
	if len(msg.To) != 1 || msg.To[0].Address != "user-1@example.com" {
		t.Errorf("unexpected recipient: %+v", msg.To)
	}
	if msg.Body == "" {
		t.Error("expected non-empty body")
	}
	if msg.HTMLBody == "" {
		t.Error("expected non-empty HTML body for email channel")
	}
	if !strings.Contains(msg.Subject, "Daily") {
		t.Errorf("expected subject to contain 'Daily', got %s", msg.Subject)
	}
}

// TestDeliveryService_NilDispatcherFallback verifies that when no dispatcher
// is configured, deliverToChannel logs content but does not error.
func TestDeliveryService_NilDispatcherFallback(t *testing.T) {
	var buf strings.Builder
	logger := log.New(&buf, "", 0)

	ds := summaries.NewDeliveryService(nil, logger)
	// No SetDispatcher call — dispatcher remains nil.

	s := &summaries.Summary{
		SummaryID:   "sum-2",
		TenantID:    "tenant-xyz",
		Period:      summaries.PeriodWeekly,
		StartTime:   time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
		Text:        "Busy week.",
		EventCount:  42,
		GeneratedAt: time.Date(2026, 4, 10, 6, 0, 0, 0, time.UTC),
	}

	targets := []notifications.DeliveryTarget{
		{
			UserID: "user-2@example.com",
			Channel: notifications.Channel{
				ChannelID:   "ch-2",
				TenantID:    "tenant-xyz",
				ChannelType: notifications.ChannelWebhook,
				Enabled:     true,
			},
		},
	}

	err := ds.DeliverToTargets(context.Background(), s, targets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	logged := buf.String()
	if !strings.Contains(logged, "log-only") {
		t.Errorf("expected log-only fallback message, got: %s", logged)
	}
	if !strings.Contains(logged, "tenant-xyz") {
		t.Errorf("expected tenant ID in log, got: %s", logged)
	}
}
