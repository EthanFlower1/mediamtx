package delivery_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
	"github.com/bluenviron/mediamtx/internal/cloud/notifications/delivery"
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

// fakeSESClient implements delivery.SESClient.
type fakeSESClient struct {
	sent []fakeSESMessage
	fail bool
}

type fakeSESMessage struct {
	From, To, Subject, Body string
}

func (f *fakeSESClient) SendEmail(_ context.Context, from, to, subject, body string) (string, error) {
	if f.fail {
		return "", errors.New("ses: simulated failure")
	}
	f.sent = append(f.sent, fakeSESMessage{from, to, subject, body})
	return fmt.Sprintf("ses-msg-%d", len(f.sent)), nil
}

// fakeTwilioClient implements delivery.TwilioClient.
type fakeTwilioClient struct {
	sent []fakeSMS
	fail bool
}

type fakeSMS struct {
	From, To, Body string
}

func (f *fakeTwilioClient) SendSMS(_ context.Context, from, to, body string) (string, error) {
	if f.fail {
		return "", errors.New("twilio: simulated failure")
	}
	f.sent = append(f.sent, fakeSMS{from, to, body})
	return fmt.Sprintf("twilio-sid-%d", len(f.sent)), nil
}

// fakeUserResolver implements delivery.UserResolver.
type fakeUserResolver struct {
	contacts map[string]string // "tenantID:userID:channelType" -> contact
}

func (f *fakeUserResolver) ResolveContact(_ context.Context, tenantID, userID string, channelType notifications.ChannelType) (string, error) {
	key := fmt.Sprintf("%s:%s:%s", tenantID, userID, channelType)
	c, ok := f.contacts[key]
	if !ok {
		return "", fmt.Errorf("no contact for %s", key)
	}
	return c, nil
}

func setupDispatcher(t *testing.T) (*delivery.Dispatcher, *fakeSESClient, *fakeTwilioClient) {
	t.Helper()
	db := openTestDB(t)
	ctx := context.Background()

	notifSvc, err := notifications.NewService(notifications.Config{
		DB:    db,
		IDGen: testIDGen,
	})
	if err != nil {
		t.Fatalf("new notif service: %v", err)
	}

	// Set up an email channel for tenant-1
	notifSvc.UpsertChannel(ctx, notifications.Channel{
		TenantID: "tenant-1", ChannelType: notifications.ChannelEmail,
		Config: "{}", Enabled: true,
	})
	// Set up an SMS channel for tenant-1
	notifSvc.UpsertChannel(ctx, notifications.Channel{
		TenantID: "tenant-1", ChannelType: notifications.ChannelSMS,
		Config: "{}", Enabled: true,
	})
	// User-1 wants email for camera.offline
	notifSvc.SetPreference(ctx, "tenant-1", "user-1", "camera.offline", notifications.ChannelEmail, true)
	// User-1 wants SMS for camera.offline
	notifSvc.SetPreference(ctx, "tenant-1", "user-1", "camera.offline", notifications.ChannelSMS, true)

	sesClient := &fakeSESClient{}
	twilioClient := &fakeTwilioClient{}

	resolver := &fakeUserResolver{
		contacts: map[string]string{
			"tenant-1:user-1:email": "user1@example.com",
			"tenant-1:user-1:sms":   "+15551234567",
		},
	}

	disp, err := delivery.NewDispatcher(delivery.DispatcherConfig{
		DB:           db,
		NotifService: notifSvc,
		Senders: map[notifications.ChannelType]delivery.Sender{
			notifications.ChannelEmail: &delivery.SESSender{Client: sesClient, FromAddress: "noreply@kaivue.io"},
			notifications.ChannelSMS:   &delivery.TwilioSender{Client: twilioClient, FromNumber: "+15559999999"},
		},
		RateLimiter:  delivery.NewRateLimiter(),
		UserResolver: resolver,
		IDGen:        testIDGen,
	})
	if err != nil {
		t.Fatalf("new dispatcher: %v", err)
	}
	return disp, sesClient, twilioClient
}

func TestDispatchEmailAndSMS(t *testing.T) {
	disp, sesClient, twilioClient := setupDispatcher(t)
	ctx := context.Background()

	entries, err := disp.Dispatch(ctx, "tenant-1", "camera.offline", "Camera Offline", "Camera 3 went offline")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Check both were sent
	sentCount := 0
	for _, e := range entries {
		if e.Status == notifications.StatusSent {
			sentCount++
		}
	}
	if sentCount != 2 {
		t.Errorf("expected 2 sent, got %d", sentCount)
	}

	if len(sesClient.sent) != 1 {
		t.Errorf("expected 1 SES send, got %d", len(sesClient.sent))
	} else {
		if sesClient.sent[0].To != "user1@example.com" {
			t.Errorf("SES to = %q", sesClient.sent[0].To)
		}
	}
	if len(twilioClient.sent) != 1 {
		t.Errorf("expected 1 Twilio send, got %d", len(twilioClient.sent))
	} else {
		if twilioClient.sent[0].To != "+15551234567" {
			t.Errorf("Twilio to = %q", twilioClient.sent[0].To)
		}
	}
}

func TestDispatchSESFailure(t *testing.T) {
	disp, sesClient, _ := setupDispatcher(t)
	ctx := context.Background()

	sesClient.fail = true

	entries, err := disp.Dispatch(ctx, "tenant-1", "camera.offline", "Camera Offline", "Camera 3 went offline")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	var emailEntry *notifications.LogEntry
	for i := range entries {
		if entries[i].ChannelType == notifications.ChannelEmail {
			emailEntry = &entries[i]
			break
		}
	}
	if emailEntry == nil {
		t.Fatal("expected email entry")
	}
	if emailEntry.Status != notifications.StatusFailed {
		t.Errorf("expected failed, got %s", emailEntry.Status)
	}
	if emailEntry.ErrorMessage == "" {
		t.Error("expected error message")
	}
}

func TestDispatchRateLimited(t *testing.T) {
	disp, _, _ := setupDispatcher(t)
	ctx := context.Background()

	// Configure a very tight rate limit: 1 per hour for email
	disp.UpsertRateLimit(ctx, delivery.RateLimit{
		TenantID:      "tenant-1",
		ChannelType:   "email",
		WindowSeconds: 3600,
		MaxCount:      1,
		Burst:         0,
		Enabled:       true,
	})

	// First dispatch should succeed
	entries1, _ := disp.Dispatch(ctx, "tenant-1", "camera.offline", "Alert", "First")
	emailSent := false
	for _, e := range entries1 {
		if e.ChannelType == notifications.ChannelEmail && e.Status == notifications.StatusSent {
			emailSent = true
		}
	}
	if !emailSent {
		t.Error("first email should have been sent")
	}

	// Second dispatch — email should be rate-limited
	entries2, _ := disp.Dispatch(ctx, "tenant-1", "camera.offline", "Alert", "Second")
	emailSuppressed := false
	for _, e := range entries2 {
		if e.ChannelType == notifications.ChannelEmail && e.Status == notifications.StatusSuppressed {
			emailSuppressed = true
		}
	}
	if !emailSuppressed {
		t.Error("second email should have been suppressed by rate limit")
	}
}

func TestUpsertAndGetRateLimit(t *testing.T) {
	disp, _, _ := setupDispatcher(t)
	ctx := context.Background()

	rl, err := disp.UpsertRateLimit(ctx, delivery.RateLimit{
		TenantID:      "tenant-1",
		ChannelType:   "email",
		WindowSeconds: 3600,
		MaxCount:      100,
		Burst:         10,
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if rl.RateLimitID == "" {
		t.Fatal("expected rate_limit_id")
	}

	got, err := disp.GetRateLimit(ctx, "tenant-1", "email")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.MaxCount != 100 {
		t.Errorf("max_count = %d, want 100", got.MaxCount)
	}
}

func TestUpsertAndGetProviderConfig(t *testing.T) {
	disp, _, _ := setupDispatcher(t)
	ctx := context.Background()

	pc, err := disp.UpsertProviderConfig(ctx, delivery.ProviderConfig{
		TenantID:     "tenant-1",
		ChannelType:  "email",
		ProviderName: delivery.ProviderSES,
		FromAddress:  "noreply@tenant1.kaivue.io",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if pc.ProviderID == "" {
		t.Fatal("expected provider_id")
	}

	got, err := disp.GetProviderConfig(ctx, "tenant-1", "email")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ProviderName != delivery.ProviderSES {
		t.Errorf("provider_name = %s, want ses", got.ProviderName)
	}
	if got.FromAddress != "noreply@tenant1.kaivue.io" {
		t.Errorf("from_address = %q", got.FromAddress)
	}
}

func TestRateLimiterSlidingWindow(t *testing.T) {
	rl := delivery.NewRateLimiter()
	rl.Configure("test", 3600, 3)

	for i := 0; i < 3; i++ {
		if !rl.Allow("test") {
			t.Errorf("expected allow on attempt %d", i)
		}
	}
	if rl.Allow("test") {
		t.Error("expected deny after max_count reached")
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	notifSvc, _ := notifications.NewService(notifications.Config{DB: db, IDGen: testIDGen})

	disp, _ := delivery.NewDispatcher(delivery.DispatcherConfig{
		DB:           db,
		NotifService: notifSvc,
		Senders:      map[notifications.ChannelType]delivery.Sender{},
		IDGen:        testIDGen,
	})

	// Rate limits for two tenants
	disp.UpsertRateLimit(ctx, delivery.RateLimit{
		TenantID: "tenant-1", ChannelType: "email", WindowSeconds: 3600, MaxCount: 100, Enabled: true,
	})
	disp.UpsertRateLimit(ctx, delivery.RateLimit{
		TenantID: "tenant-2", ChannelType: "sms", WindowSeconds: 60, MaxCount: 10, Enabled: true,
	})

	rl1, _ := disp.GetRateLimit(ctx, "tenant-1", "email")
	rl2, _ := disp.GetRateLimit(ctx, "tenant-2", "sms")

	if rl1.MaxCount != 100 {
		t.Errorf("tenant-1 max_count = %d", rl1.MaxCount)
	}
	if rl2.MaxCount != 10 {
		t.Errorf("tenant-2 max_count = %d", rl2.MaxCount)
	}

	// Provider configs for two tenants
	disp.UpsertProviderConfig(ctx, delivery.ProviderConfig{
		TenantID: "tenant-1", ChannelType: "email", ProviderName: delivery.ProviderSES,
		FromAddress: "t1@kaivue.io", Enabled: true,
	})
	disp.UpsertProviderConfig(ctx, delivery.ProviderConfig{
		TenantID: "tenant-2", ChannelType: "sms", ProviderName: delivery.ProviderTwilio,
		FromAddress: "+15550000000", Enabled: true,
	})

	pc1, _ := disp.GetProviderConfig(ctx, "tenant-1", "email")
	pc2, _ := disp.GetProviderConfig(ctx, "tenant-2", "sms")

	if pc1.ProviderName != delivery.ProviderSES {
		t.Errorf("tenant-1 provider = %s", pc1.ProviderName)
	}
	if pc2.ProviderName != delivery.ProviderTwilio {
		t.Errorf("tenant-2 provider = %s", pc2.ProviderName)
	}
}
