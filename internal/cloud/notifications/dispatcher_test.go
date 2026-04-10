package notifications_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

func validMessage() notifications.Message {
	return notifications.Message{
		ID:       "msg-001",
		Type:     notifications.MessageTypeEmail,
		TenantID: "tenant-1",
		To:       []notifications.Recipient{{Address: "user@example.com"}},
		Subject:  "Test",
		Body:     "Hello",
	}
}

func newTestDispatcher(ch notifications.DeliveryChannel) (*notifications.Dispatcher, *notifications.MetricsCollector, *notifications.MemoryDeadLetterQueue) {
	reg := notifications.NewChannelRegistry()
	reg.Register(ch)
	metrics := notifications.NewMetricsCollector()
	dlq := notifications.NewMemoryDeadLetterQueue()
	d, _ := notifications.NewDispatcher(notifications.DispatcherConfig{
		Registry:       reg,
		Idempotency:    notifications.NewMemoryIdempotencyStore(),
		DLQ:            dlq,
		Metrics:        metrics,
		MaxRetries:     2,
		RetryBaseDelay: 1 * time.Millisecond, // fast for tests
		RetryMaxDelay:  5 * time.Millisecond,
	})
	return d, metrics, dlq
}

func TestDispatcherSuccess(t *testing.T) {
	ch := &fakeChannel{
		name:  "test_email",
		types: []notifications.MessageType{notifications.MessageTypeEmail},
	}
	d, metrics, _ := newTestDispatcher(ch)

	results, err := d.Dispatch(context.Background(), validMessage())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].State != notifications.DeliveryStateDelivered {
		t.Errorf("expected delivered, got %s", results[0].State)
	}

	snap := metrics.Snapshot()
	if len(snap) == 0 {
		t.Fatal("expected metrics snapshot")
	}
	found := false
	for _, s := range snap {
		if s.Channel == "test_email" && s.Sent == 1 {
			found = true
		}
	}
	if !found {
		t.Error("expected sent=1 for test_email")
	}
}

func TestDispatcherIdempotency(t *testing.T) {
	ch := &fakeChannel{
		name:  "test_email",
		types: []notifications.MessageType{notifications.MessageTypeEmail},
	}
	d, _, _ := newTestDispatcher(ch)
	ctx := context.Background()
	msg := validMessage()

	// First send succeeds.
	_, err := d.Dispatch(ctx, msg)
	if err != nil {
		t.Fatalf("first dispatch: %v", err)
	}

	// Second send with same ID is a duplicate.
	_, err = d.Dispatch(ctx, msg)
	if !errors.Is(err, notifications.ErrDuplicateMessage) {
		t.Errorf("expected ErrDuplicateMessage, got %v", err)
	}
}

func TestDispatcherRetryThenSuccess(t *testing.T) {
	var attempts atomic.Int32
	ch := &fakeChannel{
		name:  "retry_email",
		types: []notifications.MessageType{notifications.MessageTypeEmail},
		batchSendFn: func(_ context.Context, msg notifications.Message) ([]notifications.DeliveryResult, error) {
			n := attempts.Add(1)
			if n < 3 {
				return nil, errors.New("transient error")
			}
			return []notifications.DeliveryResult{{
				MessageID: msg.ID,
				Recipient: msg.To[0].Address,
				State:     notifications.DeliveryStateDelivered,
				Timestamp: time.Now().UTC(),
			}}, nil
		},
	}
	d, metrics, _ := newTestDispatcher(ch)

	results, err := d.Dispatch(context.Background(), validMessage())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].State != notifications.DeliveryStateDelivered {
		t.Error("expected delivered after retries")
	}

	snap := metrics.Snapshot()
	for _, s := range snap {
		if s.Channel == "retry_email" {
			if s.Retried != 2 {
				t.Errorf("expected 2 retries, got %d", s.Retried)
			}
		}
	}
}

func TestDispatcherDeadLetter(t *testing.T) {
	ch := &fakeChannel{
		name:  "fail_email",
		types: []notifications.MessageType{notifications.MessageTypeEmail},
		batchSendFn: func(_ context.Context, _ notifications.Message) ([]notifications.DeliveryResult, error) {
			return nil, errors.New("permanent failure")
		},
	}
	d, metrics, dlq := newTestDispatcher(ch)

	_, err := d.Dispatch(context.Background(), validMessage())
	if !errors.Is(err, notifications.ErrMaxRetriesExceeded) {
		t.Errorf("expected ErrMaxRetriesExceeded, got %v", err)
	}

	// Check DLQ.
	entries, err := dlq.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("dlq list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 DLQ entry, got %d", len(entries))
	}
	if entries[0].MessageID != "msg-001" {
		t.Errorf("expected message ID msg-001, got %s", entries[0].MessageID)
	}
	if entries[0].Channel != "fail_email" {
		t.Errorf("expected channel fail_email, got %s", entries[0].Channel)
	}

	// Check DLQ metric.
	snap := metrics.Snapshot()
	for _, s := range snap {
		if s.Channel == "fail_email" {
			if s.DLQ != 1 {
				t.Errorf("expected DLQ=1, got %d", s.DLQ)
			}
		}
	}
}

func TestDispatcherNoChannel(t *testing.T) {
	ch := &fakeChannel{
		name:  "email_only",
		types: []notifications.MessageType{notifications.MessageTypeEmail},
	}
	d, _, _ := newTestDispatcher(ch)

	msg := validMessage()
	msg.Type = notifications.MessageTypeSMS // no SMS channel registered

	_, err := d.Dispatch(context.Background(), msg)
	if !errors.Is(err, notifications.ErrNoChannel) {
		t.Errorf("expected ErrNoChannel, got %v", err)
	}
}

func TestDispatcherAutoGeneratesID(t *testing.T) {
	ch := &fakeChannel{
		name:  "test_email",
		types: []notifications.MessageType{notifications.MessageTypeEmail},
	}
	d, _, _ := newTestDispatcher(ch)

	msg := validMessage()
	msg.ID = "" // empty — should be auto-generated

	results, err := d.Dispatch(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].MessageID == "" {
		t.Error("expected auto-generated message ID")
	}
}

func TestDispatcherBatchSend(t *testing.T) {
	ch := &fakeChannel{
		name:  "test_email",
		types: []notifications.MessageType{notifications.MessageTypeEmail},
	}
	d, _, _ := newTestDispatcher(ch)

	msg := validMessage()
	msg.ID = "batch-001"
	msg.To = []notifications.Recipient{
		{Address: "a@example.com"},
		{Address: "b@example.com"},
		{Address: "c@example.com"},
	}

	results, err := d.Dispatch(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, r := range results {
		if r.State != notifications.DeliveryStateDelivered {
			t.Errorf("result %d: expected delivered, got %s", i, r.State)
		}
	}
}

func TestDispatcherContextCancellation(t *testing.T) {
	ch := &fakeChannel{
		name:  "slow_email",
		types: []notifications.MessageType{notifications.MessageTypeEmail},
		batchSendFn: func(_ context.Context, _ notifications.Message) ([]notifications.DeliveryResult, error) {
			return nil, errors.New("transient")
		},
	}
	d, _, _ := newTestDispatcher(ch)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := d.Dispatch(ctx, validMessage())
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}
