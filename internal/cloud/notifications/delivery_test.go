package notifications_test

import (
	"context"
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

// fakeChannel is a minimal DeliveryChannel for registry tests.
type fakeChannel struct {
	ct notifications.ChannelType
}

func (f *fakeChannel) Type() notifications.ChannelType { return f.ct }
func (f *fakeChannel) Send(_ context.Context, _ notifications.Message) (notifications.DeliveryResult, error) {
	return notifications.DeliveryResult{State: notifications.StateDelivered}, nil
}
func (f *fakeChannel) BatchSend(_ context.Context, msg notifications.BatchMessage) ([]notifications.DeliveryResult, error) {
	out := make([]notifications.DeliveryResult, len(msg.Targets))
	for i := range out {
		out[i] = notifications.DeliveryResult{State: notifications.StateDelivered}
	}
	return out, nil
}
func (f *fakeChannel) CheckHealth(_ context.Context) error { return nil }

func TestChannelRegistry(t *testing.T) {
	reg := notifications.NewChannelRegistry()

	push := &fakeChannel{ct: notifications.ChannelPush}
	email := &fakeChannel{ct: notifications.ChannelEmail}

	reg.Register(push)
	reg.Register(email)

	if got := reg.Get(notifications.ChannelPush); got != push {
		t.Error("expected push channel")
	}
	if got := reg.Get(notifications.ChannelEmail); got != email {
		t.Error("expected email channel")
	}
	if got := reg.Get(notifications.ChannelSMS); got != nil {
		t.Error("expected nil for unregistered channel")
	}

	all := reg.All()
	if len(all) != 2 {
		t.Errorf("expected 2 channels, got %d", len(all))
	}
}
