package notifications_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
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

func newService(t *testing.T) *notifications.Service {
	t.Helper()
	db := openTestDB(t)
	svc, err := notifications.NewService(notifications.Config{
		DB:    db,
		IDGen: testIDGen,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func TestUpsertAndListChannels(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	ch := notifications.Channel{
		TenantID:    "tenant-1",
		ChannelType: notifications.ChannelEmail,
		Config:      `{"smtp":"mail.example.com"}`,
		Enabled:     true,
	}
	created, err := svc.UpsertChannel(ctx, ch)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if created.ChannelID == "" {
		t.Fatal("expected channel_id to be set")
	}

	channels, err := svc.ListChannels(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(channels))
	}
	if channels[0].ChannelType != notifications.ChannelEmail {
		t.Errorf("expected email, got %s", channels[0].ChannelType)
	}
}

func TestDeleteChannel(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	ch, _ := svc.UpsertChannel(ctx, notifications.Channel{
		TenantID:    "tenant-1",
		ChannelType: notifications.ChannelPush,
		Config:      "{}",
		Enabled:     true,
	})

	if err := svc.DeleteChannel(ctx, "tenant-1", ch.ChannelID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	err := svc.DeleteChannel(ctx, "tenant-1", ch.ChannelID)
	if err != notifications.ErrChannelNotFound {
		t.Errorf("expected ErrChannelNotFound, got %v", err)
	}
}

func TestSetAndGetPreferences(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	if err := svc.SetPreference(ctx, "tenant-1", "user-1", "camera.offline", notifications.ChannelEmail, true); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := svc.SetPreference(ctx, "tenant-1", "user-1", "camera.offline", notifications.ChannelPush, false); err != nil {
		t.Fatalf("set: %v", err)
	}

	prefs, err := svc.GetPreferences(ctx, "tenant-1", "user-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(prefs) != 2 {
		t.Fatalf("expected 2 prefs, got %d", len(prefs))
	}
}

func TestRouteNotification(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	// Set up email channel
	svc.UpsertChannel(ctx, notifications.Channel{
		TenantID:    "tenant-1",
		ChannelType: notifications.ChannelEmail,
		Config:      "{}",
		Enabled:     true,
	})
	// Set up push channel (disabled)
	svc.UpsertChannel(ctx, notifications.Channel{
		TenantID:    "tenant-1",
		ChannelType: notifications.ChannelPush,
		Config:      "{}",
		Enabled:     false,
	})

	// User wants email for camera.offline
	svc.SetPreference(ctx, "tenant-1", "user-1", "camera.offline", notifications.ChannelEmail, true)
	// User wants push for camera.offline (but push channel is disabled)
	svc.SetPreference(ctx, "tenant-1", "user-1", "camera.offline", notifications.ChannelPush, true)

	targets, err := svc.RouteNotification(ctx, "tenant-1", "camera.offline")
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	// Only email should route (push channel is disabled)
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Channel.ChannelType != notifications.ChannelEmail {
		t.Errorf("expected email target, got %s", targets[0].Channel.ChannelType)
	}
}

func TestLogDelivery(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	err := svc.LogDelivery(ctx, notifications.LogEntry{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		EventType:   "camera.offline",
		ChannelType: notifications.ChannelEmail,
		Status:      notifications.StatusSent,
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	svc.UpsertChannel(ctx, notifications.Channel{
		TenantID:    "tenant-1",
		ChannelType: notifications.ChannelEmail,
		Config:      "{}",
		Enabled:     true,
	})
	svc.UpsertChannel(ctx, notifications.Channel{
		TenantID:    "tenant-2",
		ChannelType: notifications.ChannelSMS,
		Config:      "{}",
		Enabled:     true,
	})

	ch1, _ := svc.ListChannels(ctx, "tenant-1")
	ch2, _ := svc.ListChannels(ctx, "tenant-2")

	if len(ch1) != 1 || ch1[0].ChannelType != notifications.ChannelEmail {
		t.Errorf("tenant-1 should only see email channel")
	}
	if len(ch2) != 1 || ch2[0].ChannelType != notifications.ChannelSMS {
		t.Errorf("tenant-2 should only see SMS channel")
	}
}
