package push_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
	"github.com/bluenviron/mediamtx/internal/cloud/notifications/push"
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

func newStore(t *testing.T) *push.Store {
	t.Helper()
	db := openTestDB(t)
	s, err := push.NewStore(push.StoreConfig{DB: db, IDGen: testIDGen})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return s
}

func TestRegisterAndListDevices(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	dt := push.DeviceToken{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Platform:    notifications.PlatformFCM,
		DeviceToken: "fcm-token-abc",
		DeviceName:  "Pixel 9",
	}

	created, err := s.RegisterDevice(ctx, dt)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if created.TokenID == "" {
		t.Fatal("expected token_id to be set")
	}

	// Register a second device
	dt2 := push.DeviceToken{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Platform:    notifications.PlatformAPNs,
		DeviceToken: "apns-token-xyz",
		DeviceName:  "iPhone 17",
	}
	_, err = s.RegisterDevice(ctx, dt2)
	if err != nil {
		t.Fatalf("register second: %v", err)
	}

	devices, err := s.ListDevices(ctx, "tenant-1", "user-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
}

func TestRegisterDeviceUpsert(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	dt := push.DeviceToken{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Platform:    notifications.PlatformFCM,
		DeviceToken: "fcm-token-same",
		DeviceName:  "Old Name",
	}
	_, err := s.RegisterDevice(ctx, dt)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Upsert with new name
	dt.DeviceName = "New Name"
	_, err = s.RegisterDevice(ctx, dt)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	devices, err := s.ListDevices(ctx, "tenant-1", "user-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device after upsert, got %d", len(devices))
	}
	if devices[0].DeviceName != "New Name" {
		t.Errorf("expected 'New Name', got %q", devices[0].DeviceName)
	}
}

func TestDeregisterDevice(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	_, err := s.RegisterDevice(ctx, push.DeviceToken{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Platform:    notifications.PlatformFCM,
		DeviceToken: "fcm-token-del",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	err = s.DeregisterDevice(ctx, "tenant-1", "user-1", "fcm-token-del")
	if err != nil {
		t.Fatalf("deregister: %v", err)
	}

	// Should fail on second attempt
	err = s.DeregisterDevice(ctx, "tenant-1", "user-1", "fcm-token-del")
	if err != push.ErrTokenNotFound {
		t.Errorf("expected ErrTokenNotFound, got %v", err)
	}
}

func TestDeregisterByToken(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	// Same token registered for two users
	for _, uid := range []string{"user-1", "user-2"} {
		_, err := s.RegisterDevice(ctx, push.DeviceToken{
			TenantID:    "tenant-1",
			UserID:      uid,
			Platform:    notifications.PlatformFCM,
			DeviceToken: "shared-token",
		})
		if err != nil {
			t.Fatalf("register %s: %v", uid, err)
		}
	}

	err := s.DeregisterByToken(ctx, "tenant-1", "shared-token")
	if err != nil {
		t.Fatalf("deregister by token: %v", err)
	}

	for _, uid := range []string{"user-1", "user-2"} {
		devices, err := s.ListDevices(ctx, "tenant-1", uid)
		if err != nil {
			t.Fatalf("list %s: %v", uid, err)
		}
		if len(devices) != 0 {
			t.Errorf("expected 0 devices for %s, got %d", uid, len(devices))
		}
	}
}

func TestListDevicesByPlatform(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	for _, p := range []notifications.Platform{notifications.PlatformFCM, notifications.PlatformAPNs} {
		_, err := s.RegisterDevice(ctx, push.DeviceToken{
			TenantID:    "tenant-1",
			UserID:      "user-1",
			Platform:    p,
			DeviceToken: "token-" + string(p),
		})
		if err != nil {
			t.Fatalf("register %s: %v", p, err)
		}
	}

	devices, err := s.ListDevicesByPlatform(ctx, "tenant-1", "user-1", notifications.PlatformFCM)
	if err != nil {
		t.Fatalf("list by platform: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 FCM device, got %d", len(devices))
	}
	if devices[0].Platform != notifications.PlatformFCM {
		t.Errorf("expected FCM, got %s", devices[0].Platform)
	}
}

func TestRegisterDeviceValidation(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	tests := []struct {
		name string
		dt   push.DeviceToken
		want error
	}{
		{"missing tenant", push.DeviceToken{UserID: "u", Platform: notifications.PlatformFCM, DeviceToken: "t"}, push.ErrMissingTenantID},
		{"missing user", push.DeviceToken{TenantID: "t", Platform: notifications.PlatformFCM, DeviceToken: "t"}, push.ErrMissingUserID},
		{"missing token", push.DeviceToken{TenantID: "t", UserID: "u", Platform: notifications.PlatformFCM}, push.ErrMissingToken},
		{"invalid platform", push.DeviceToken{TenantID: "t", UserID: "u", Platform: "bad", DeviceToken: "t"}, push.ErrInvalidPlatform},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.RegisterDevice(ctx, tt.dt)
			if err != tt.want {
				t.Errorf("expected %v, got %v", tt.want, err)
			}
		})
	}
}

// ---------- Credential tests ----------

func TestUpsertAndGetCredential(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	cred := push.PushCredential{
		TenantID:    "tenant-1",
		Platform:    notifications.PlatformFCM,
		Credentials: `{"project_id":"my-project","service_account_json":"{}"}`,
		Enabled:     true,
	}

	created, err := s.UpsertCredential(ctx, cred)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if created.CredentialID == "" {
		t.Fatal("expected credential_id to be set")
	}

	got, err := s.GetCredential(ctx, "tenant-1", notifications.PlatformFCM)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Platform != notifications.PlatformFCM {
		t.Errorf("expected FCM, got %s", got.Platform)
	}
}

func TestUpsertCredentialOverwrite(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	cred := push.PushCredential{
		TenantID:    "tenant-1",
		Platform:    notifications.PlatformAPNs,
		Credentials: `{"key_id":"old"}`,
		Enabled:     true,
	}
	_, err := s.UpsertCredential(ctx, cred)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	cred.Credentials = `{"key_id":"new"}`
	_, err = s.UpsertCredential(ctx, cred)
	if err != nil {
		t.Fatalf("upsert overwrite: %v", err)
	}

	got, err := s.GetCredential(ctx, "tenant-1", notifications.PlatformAPNs)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Credentials != `{"key_id":"new"}` {
		t.Errorf("expected new credentials, got %q", got.Credentials)
	}
}

func TestDeleteCredential(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	_, err := s.UpsertCredential(ctx, push.PushCredential{
		TenantID:    "tenant-1",
		Platform:    notifications.PlatformWebPush,
		Credentials: `{}`,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	err = s.DeleteCredential(ctx, "tenant-1", notifications.PlatformWebPush)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	err = s.DeleteCredential(ctx, "tenant-1", notifications.PlatformWebPush)
	if err != push.ErrCredentialNotFound {
		t.Errorf("expected ErrCredentialNotFound, got %v", err)
	}
}

func TestListCredentials(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	for _, p := range []notifications.Platform{notifications.PlatformFCM, notifications.PlatformAPNs, notifications.PlatformWebPush} {
		_, err := s.UpsertCredential(ctx, push.PushCredential{
			TenantID:    "tenant-1",
			Platform:    p,
			Credentials: `{}`,
			Enabled:     true,
		})
		if err != nil {
			t.Fatalf("upsert %s: %v", p, err)
		}
	}

	creds, err := s.ListCredentials(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(creds) != 3 {
		t.Fatalf("expected 3 credentials, got %d", len(creds))
	}
}

func TestCredentialTenantIsolation(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	_, _ = s.UpsertCredential(ctx, push.PushCredential{
		TenantID: "tenant-1", Platform: notifications.PlatformFCM, Credentials: `{}`, Enabled: true,
	})
	_, _ = s.UpsertCredential(ctx, push.PushCredential{
		TenantID: "tenant-2", Platform: notifications.PlatformAPNs, Credentials: `{}`, Enabled: true,
	})

	c1, _ := s.ListCredentials(ctx, "tenant-1")
	c2, _ := s.ListCredentials(ctx, "tenant-2")

	if len(c1) != 1 || c1[0].Platform != notifications.PlatformFCM {
		t.Errorf("tenant-1 should only see FCM")
	}
	if len(c2) != 1 || c2[0].Platform != notifications.PlatformAPNs {
		t.Errorf("tenant-2 should only see APNs")
	}
}
