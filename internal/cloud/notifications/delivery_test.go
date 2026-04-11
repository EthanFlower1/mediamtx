package notifications_test

import (
	"context"
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

// fakePushChannel is a minimal PushDeliveryChannel for registry tests.
type fakePushChannel struct {
	ct notifications.ChannelType
}

func (f *fakePushChannel) Type() notifications.ChannelType { return f.ct }
func (f *fakePushChannel) Send(_ context.Context, _ notifications.PushMessage) (notifications.PushDeliveryResult, error) {
	return notifications.PushDeliveryResult{State: notifications.PushStateDelivered}, nil
}
func (f *fakePushChannel) BatchSend(_ context.Context, msg notifications.BatchMessage) ([]notifications.PushDeliveryResult, error) {
	out := make([]notifications.PushDeliveryResult, len(msg.Targets))
	for i := range out {
		out[i] = notifications.PushDeliveryResult{State: notifications.PushStateDelivered}
	}
	return out, nil
}
func (f *fakePushChannel) CheckHealth(_ context.Context) error { return nil }

func TestPushChannelRegistry(t *testing.T) {
	reg := notifications.NewPushChannelRegistry()

	push := &fakePushChannel{ct: notifications.ChannelPush}
	email := &fakePushChannel{ct: notifications.ChannelEmail}

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
