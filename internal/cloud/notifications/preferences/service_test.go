package preferences_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
	"github.com/bluenviron/mediamtx/internal/cloud/notifications/preferences"
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
	return fmt.Sprintf("pref-%04d", seqID)
}

func fixedNow(t time.Time) preferences.NowFunc {
	return func() time.Time { return t }
}

func newService(t *testing.T, now preferences.NowFunc) *preferences.Service {
	t.Helper()
	db := openTestDB(t)
	cfg := preferences.Config{
		DB:    db,
		IDGen: testIDGen,
		Now:   now,
	}
	svc, err := preferences.New(cfg)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func TestUpsertAndList(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	p := preferences.Pref{
		TenantID:  "t1",
		UserID:    "u1",
		CameraID:  "cam-1",
		EventType: "motion",
		Channels:  []notifications.ChannelType{notifications.ChannelEmail, notifications.ChannelPush},
		SeverityMin: preferences.SeverityWarning,
		Enabled:   true,
	}
	created, err := svc.Upsert(ctx, p)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if created.PrefID == "" {
		t.Fatal("expected pref_id")
	}

	list, err := svc.List(ctx, "t1", "u1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}
	if list[0].CameraID != "cam-1" {
		t.Errorf("camera_id: got %q", list[0].CameraID)
	}
	if len(list[0].Channels) != 2 {
		t.Errorf("channels: got %d", len(list[0].Channels))
	}
	if list[0].SeverityMin != preferences.SeverityWarning {
		t.Errorf("severity: got %q", list[0].SeverityMin)
	}
}

func TestUpsertUpdatesExisting(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	p := preferences.Pref{
		TenantID:  "t1",
		UserID:    "u1",
		CameraID:  "cam-1",
		EventType: "motion",
		Channels:  []notifications.ChannelType{notifications.ChannelEmail},
		Enabled:   true,
	}
	_, err := svc.Upsert(ctx, p)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Update channels
	p.Channels = []notifications.ChannelType{notifications.ChannelSMS}
	_, err = svc.Upsert(ctx, p)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	list, err := svc.List(ctx, "t1", "u1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d (upsert created duplicate)", len(list))
	}
	if list[0].Channels[0] != notifications.ChannelSMS {
		t.Errorf("expected sms, got %s", list[0].Channels[0])
	}
}

func TestDelete(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	created, _ := svc.Upsert(ctx, preferences.Pref{
		TenantID:  "t1",
		UserID:    "u1",
		EventType: "motion",
		Channels:  []notifications.ChannelType{notifications.ChannelEmail},
		Enabled:   true,
	})

	if err := svc.Delete(ctx, created.PrefID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := svc.Delete(ctx, created.PrefID); err != preferences.ErrPrefNotFound {
		t.Errorf("expected ErrPrefNotFound, got %v", err)
	}
}

func TestResolve_Priority(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	// Priority 4: wildcard camera + wildcard event (default)
	svc.Upsert(ctx, preferences.Pref{
		TenantID:    "t1",
		UserID:      "u1",
		CameraID:    "",
		EventType:   "",
		Channels:    []notifications.ChannelType{notifications.ChannelEmail},
		SeverityMin: preferences.SeverityInfo,
		Enabled:     true,
	})
	// Priority 3: wildcard camera + specific event
	svc.Upsert(ctx, preferences.Pref{
		TenantID:    "t1",
		UserID:      "u1",
		CameraID:    "",
		EventType:   "motion",
		Channels:    []notifications.ChannelType{notifications.ChannelPush},
		SeverityMin: preferences.SeverityWarning,
		Enabled:     true,
	})
	// Priority 2: specific camera + wildcard event
	svc.Upsert(ctx, preferences.Pref{
		TenantID:    "t1",
		UserID:      "u1",
		CameraID:    "cam-1",
		EventType:   "",
		Channels:    []notifications.ChannelType{notifications.ChannelSMS},
		SeverityMin: preferences.SeverityCritical,
		Enabled:     true,
	})
	// Priority 1: specific camera + specific event
	svc.Upsert(ctx, preferences.Pref{
		TenantID:    "t1",
		UserID:      "u1",
		CameraID:    "cam-1",
		EventType:   "motion",
		Channels:    []notifications.ChannelType{notifications.ChannelWebhook},
		SeverityMin: preferences.SeverityInfo,
		Enabled:     true,
	})

	tests := []struct {
		name     string
		camera   string
		event    string
		wantChan notifications.ChannelType
	}{
		{"exact match", "cam-1", "motion", notifications.ChannelWebhook},
		{"camera wildcard event", "cam-1", "offline", notifications.ChannelSMS},
		{"wildcard camera exact event", "cam-2", "motion", notifications.ChannelPush},
		{"full wildcard", "cam-2", "offline", notifications.ChannelEmail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pref, err := svc.Resolve(ctx, "t1", "u1", tt.camera, tt.event)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if len(pref.Channels) != 1 || pref.Channels[0] != tt.wantChan {
				t.Errorf("expected %s, got %v", tt.wantChan, pref.Channels)
			}
		})
	}
}

func TestResolve_NotFound(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	_, err := svc.Resolve(ctx, "t1", "u1", "cam-1", "motion")
	if err != preferences.ErrPrefNotFound {
		t.Errorf("expected ErrPrefNotFound, got %v", err)
	}
}

func TestSeverityFiltering(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	svc.Upsert(ctx, preferences.Pref{
		TenantID:    "t1",
		UserID:      "u1",
		Channels:    []notifications.ChannelType{notifications.ChannelEmail},
		SeverityMin: preferences.SeverityWarning,
		Enabled:     true,
	})

	// Info event should be suppressed (below warning threshold).
	rd, err := svc.ResolveDelivery(ctx, "t1", "u1", "cam-1", "motion", preferences.SeverityInfo)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !rd.Suppressed {
		t.Error("expected info event to be suppressed with warning threshold")
	}

	// Warning event should pass.
	rd, err = svc.ResolveDelivery(ctx, "t1", "u1", "cam-1", "motion", preferences.SeverityWarning)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if rd.Suppressed {
		t.Error("warning event should not be suppressed")
	}

	// Critical event should pass.
	rd, err = svc.ResolveDelivery(ctx, "t1", "u1", "cam-1", "motion", preferences.SeverityCritical)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if rd.Suppressed {
		t.Error("critical event should not be suppressed")
	}
}

func TestQuietHours_Suppressed(t *testing.T) {
	// Fix time to Wednesday 2026-04-08 at 23:30 UTC.
	frozen := time.Date(2026, 4, 8, 23, 30, 0, 0, time.UTC)
	svc := newService(t, fixedNow(frozen))
	ctx := context.Background()

	svc.Upsert(ctx, preferences.Pref{
		TenantID:      "t1",
		UserID:        "u1",
		Channels:      []notifications.ChannelType{notifications.ChannelEmail},
		SeverityMin:   preferences.SeverityInfo,
		QuietStart:    "22:00",
		QuietEnd:      "06:00",
		QuietTimezone: "UTC",
		Enabled:       true,
	})

	rd, err := svc.ResolveDelivery(ctx, "t1", "u1", "any-cam", "motion", preferences.SeverityInfo)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !rd.Suppressed {
		t.Error("expected suppression during quiet hours (23:30 is within 22:00-06:00)")
	}
	if rd.Reason == "" {
		t.Error("expected reason for suppression")
	}
}

func TestQuietHours_NotSuppressed(t *testing.T) {
	// Fix time to Wednesday 2026-04-08 at 14:00 UTC.
	frozen := time.Date(2026, 4, 8, 14, 0, 0, 0, time.UTC)
	svc := newService(t, fixedNow(frozen))
	ctx := context.Background()

	svc.Upsert(ctx, preferences.Pref{
		TenantID:      "t1",
		UserID:        "u1",
		Channels:      []notifications.ChannelType{notifications.ChannelEmail},
		SeverityMin:   preferences.SeverityInfo,
		QuietStart:    "22:00",
		QuietEnd:      "06:00",
		QuietTimezone: "UTC",
		Enabled:       true,
	})

	rd, err := svc.ResolveDelivery(ctx, "t1", "u1", "any-cam", "motion", preferences.SeverityInfo)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if rd.Suppressed {
		t.Error("should not be suppressed at 14:00 (outside 22:00-06:00)")
	}
}

func TestQuietHours_DayOfWeekFilter(t *testing.T) {
	// Wednesday = 3
	frozen := time.Date(2026, 4, 8, 23, 30, 0, 0, time.UTC)
	svc := newService(t, fixedNow(frozen))
	ctx := context.Background()

	// Quiet hours only on weekends (Sat=6, Sun=0).
	svc.Upsert(ctx, preferences.Pref{
		TenantID:      "t1",
		UserID:        "u1",
		Channels:      []notifications.ChannelType{notifications.ChannelEmail},
		SeverityMin:   preferences.SeverityInfo,
		QuietStart:    "22:00",
		QuietEnd:      "06:00",
		QuietTimezone: "UTC",
		QuietDays:     []int{0, 6}, // Sun, Sat only
		Enabled:       true,
	})

	rd, err := svc.ResolveDelivery(ctx, "t1", "u1", "any-cam", "motion", preferences.SeverityInfo)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if rd.Suppressed {
		t.Error("should not be suppressed on Wednesday when quiet is weekends-only")
	}
}

func TestQuietHours_SameDayRange(t *testing.T) {
	// 09:00-17:00 range, current time 12:00.
	frozen := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	svc := newService(t, fixedNow(frozen))
	ctx := context.Background()

	svc.Upsert(ctx, preferences.Pref{
		TenantID:      "t1",
		UserID:        "u1",
		Channels:      []notifications.ChannelType{notifications.ChannelEmail},
		SeverityMin:   preferences.SeverityInfo,
		QuietStart:    "09:00",
		QuietEnd:      "17:00",
		QuietTimezone: "UTC",
		Enabled:       true,
	})

	rd, err := svc.ResolveDelivery(ctx, "t1", "u1", "any-cam", "motion", preferences.SeverityInfo)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !rd.Suppressed {
		t.Error("12:00 should be suppressed in 09:00-17:00 range")
	}
}

func TestQuietHours_Timezone(t *testing.T) {
	// 03:00 UTC = 22:00 US/Eastern (previous day, EDT = UTC-4).
	// But we are in April so EDT applies: UTC-4 means 03:00 UTC = 23:00 EDT.
	frozen := time.Date(2026, 4, 8, 3, 0, 0, 0, time.UTC)
	svc := newService(t, fixedNow(frozen))
	ctx := context.Background()

	svc.Upsert(ctx, preferences.Pref{
		TenantID:      "t1",
		UserID:        "u1",
		Channels:      []notifications.ChannelType{notifications.ChannelEmail},
		SeverityMin:   preferences.SeverityInfo,
		QuietStart:    "22:00",
		QuietEnd:      "06:00",
		QuietTimezone: "America/New_York",
		Enabled:       true,
	})

	rd, err := svc.ResolveDelivery(ctx, "t1", "u1", "any-cam", "motion", preferences.SeverityInfo)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	// 03:00 UTC = 23:00 EDT, which is within 22:00-06:00 EDT.
	if !rd.Suppressed {
		t.Error("should be suppressed: 03:00 UTC = 23:00 EDT, within 22:00-06:00 quiet window")
	}
}

func TestResolveChannels_Convenience(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	svc.Upsert(ctx, preferences.Pref{
		TenantID:    "t1",
		UserID:      "u1",
		Channels:    []notifications.ChannelType{notifications.ChannelEmail, notifications.ChannelPush},
		SeverityMin: preferences.SeverityInfo,
		Enabled:     true,
	})

	channels, err := svc.ResolveChannels(ctx, "t1", "u1", "cam-1", "motion", preferences.SeverityInfo)
	if err != nil {
		t.Fatalf("resolve channels: %v", err)
	}
	if len(channels) != 2 {
		t.Errorf("expected 2 channels, got %d", len(channels))
	}
}

func TestResolveChannels_Suppressed(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	svc.Upsert(ctx, preferences.Pref{
		TenantID:    "t1",
		UserID:      "u1",
		Channels:    []notifications.ChannelType{notifications.ChannelEmail},
		SeverityMin: preferences.SeverityCritical,
		Enabled:     true,
	})

	channels, err := svc.ResolveChannels(ctx, "t1", "u1", "cam-1", "motion", preferences.SeverityInfo)
	if err != nil {
		t.Fatalf("resolve channels: %v", err)
	}
	if channels != nil {
		t.Errorf("expected nil channels for suppressed delivery, got %v", channels)
	}
}

func TestResolveChannels_NoPref(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	channels, err := svc.ResolveChannels(ctx, "t1", "u1", "cam-1", "motion", preferences.SeverityInfo)
	if err != nil {
		t.Fatalf("resolve channels: %v", err)
	}
	if channels != nil {
		t.Errorf("expected nil channels when no pref exists, got %v", channels)
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	svc.Upsert(ctx, preferences.Pref{
		TenantID:  "t1",
		UserID:    "u1",
		Channels:  []notifications.ChannelType{notifications.ChannelEmail},
		Enabled:   true,
	})
	svc.Upsert(ctx, preferences.Pref{
		TenantID:  "t2",
		UserID:    "u1",
		Channels:  []notifications.ChannelType{notifications.ChannelSMS},
		Enabled:   true,
	})

	list1, _ := svc.List(ctx, "t1", "u1")
	list2, _ := svc.List(ctx, "t2", "u1")

	if len(list1) != 1 || list1[0].Channels[0] != notifications.ChannelEmail {
		t.Errorf("t1 should see email, got %v", list1)
	}
	if len(list2) != 1 || list2[0].Channels[0] != notifications.ChannelSMS {
		t.Errorf("t2 should see sms, got %v", list2)
	}

	// Resolve for t1 should not see t2 prefs.
	pref, err := svc.Resolve(ctx, "t1", "u1", "any", "any")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if pref.Channels[0] != notifications.ChannelEmail {
		t.Errorf("expected email, got %s", pref.Channels[0])
	}
}

func TestDisabledPrefSkipped(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	// Only preference is disabled.
	svc.Upsert(ctx, preferences.Pref{
		TenantID:  "t1",
		UserID:    "u1",
		Channels:  []notifications.ChannelType{notifications.ChannelEmail},
		Enabled:   false,
	})

	_, err := svc.Resolve(ctx, "t1", "u1", "cam-1", "motion")
	if err != preferences.ErrPrefNotFound {
		t.Errorf("expected ErrPrefNotFound for disabled pref, got %v", err)
	}
}
