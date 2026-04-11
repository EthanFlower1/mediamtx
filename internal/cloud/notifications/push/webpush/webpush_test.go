package webpush_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
	"github.com/bluenviron/mediamtx/internal/cloud/notifications/push/webpush"
)

// generateVAPIDKeys creates a test VAPID key pair.
func generateVAPIDKeys(t *testing.T) (pub, priv string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	pubBytes := elliptic.Marshal(key.Curve, key.X, key.Y)
	pub = base64.RawURLEncoding.EncodeToString(pubBytes)
	priv = base64.RawURLEncoding.EncodeToString(key.D.Bytes())
	return pub, priv
}

// makeSubscription creates a fake browser subscription for testing.
// The endpoint points to the given test server URL.
func makeSubscription(t *testing.T, endpoint string) string {
	t.Helper()
	// Generate a subscriber key pair for encryption
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate subscriber key: %v", err)
	}
	p256dhBytes := elliptic.Marshal(key.Curve, key.X, key.Y)
	p256dh := base64.RawURLEncoding.EncodeToString(p256dhBytes)

	authBytes := make([]byte, 16)
	rand.Read(authBytes)
	auth := base64.RawURLEncoding.EncodeToString(authBytes)

	sub := map[string]interface{}{
		"endpoint": endpoint,
		"keys": map[string]string{
			"p256dh": p256dh,
			"auth":   auth,
		},
	}
	b, _ := json.Marshal(sub)
	return string(b)
}

func TestSend_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") != "aes128gcm" {
			t.Errorf("unexpected content-encoding: %s", r.Header.Get("Content-Encoding"))
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("missing authorization header")
		}
		w.Header().Set("Location", "https://push.example.com/receipt/123")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	pub, priv := generateVAPIDKeys(t)
	ch, err := webpush.New(webpush.Config{
		VAPIDPublicKey:  pub,
		VAPIDPrivateKey: priv,
		Subject:         "mailto:test@kaivue.com",
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	sub := makeSubscription(t, srv.URL+"/push/v1/abc")

	result, err := ch.Send(context.Background(), notifications.PushMessage{
		MessageID: "msg-1",
		Target:    sub,
		Title:     "Test Alert",
		Body:      "Camera offline",
		Priority:  "high",
		TTL:       5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if result.State != notifications.PushStateDelivered {
		t.Errorf("expected delivered, got %s (err: %s)", result.State, result.ErrorMessage)
	}
}

func TestSend_SubscriptionExpired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	defer srv.Close()

	pub, priv := generateVAPIDKeys(t)
	ch, _ := webpush.New(webpush.Config{
		VAPIDPublicKey:  pub,
		VAPIDPrivateKey: priv,
		Subject:         "mailto:test@kaivue.com",
	})

	sub := makeSubscription(t, srv.URL+"/push/v1/xyz")

	result, err := ch.Send(context.Background(), notifications.PushMessage{
		Target: sub,
		Title:  "Test",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if result.State != notifications.PushStateUnreachable {
		t.Errorf("expected unreachable, got %s", result.State)
	}
	if !result.ShouldRemoveToken {
		t.Error("expected ShouldRemoveToken=true")
	}
}

func TestSend_RateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	pub, priv := generateVAPIDKeys(t)
	ch, _ := webpush.New(webpush.Config{
		VAPIDPublicKey:  pub,
		VAPIDPrivateKey: priv,
		Subject:         "mailto:test@kaivue.com",
	})

	sub := makeSubscription(t, srv.URL+"/push/v1/abc")

	result, _ := ch.Send(context.Background(), notifications.PushMessage{
		Target: sub,
	})
	if result.State != notifications.PushStateThrottled {
		t.Errorf("expected throttled, got %s", result.State)
	}
}

func TestSend_InvalidSubscription(t *testing.T) {
	pub, priv := generateVAPIDKeys(t)
	ch, _ := webpush.New(webpush.Config{
		VAPIDPublicKey:  pub,
		VAPIDPrivateKey: priv,
		Subject:         "mailto:test@kaivue.com",
	})

	result, _ := ch.Send(context.Background(), notifications.PushMessage{
		Target: "not-json",
	})
	if result.State != notifications.PushStateFailed {
		t.Errorf("expected failed, got %s", result.State)
	}
}

func TestBatchSend(t *testing.T) {
	var count int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count++
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	pub, priv := generateVAPIDKeys(t)
	ch, _ := webpush.New(webpush.Config{
		VAPIDPublicKey:  pub,
		VAPIDPrivateKey: priv,
		Subject:         "mailto:test@kaivue.com",
	})

	sub1 := makeSubscription(t, srv.URL+"/push/v1/a")
	sub2 := makeSubscription(t, srv.URL+"/push/v1/b")

	results, err := ch.BatchSend(context.Background(), notifications.BatchMessage{
		MessageID: "batch-1",
		Targets: []notifications.Target{
			{UserID: "u1", DeviceToken: sub1, Platform: notifications.PlatformWebPush},
			{UserID: "u2", DeviceToken: sub2, Platform: notifications.PlatformWebPush},
		},
		Title: "Batch",
		Body:  "Test",
	})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if count != 2 {
		t.Errorf("expected 2 requests, got %d", count)
	}
}

func TestCheckHealth(t *testing.T) {
	pub, priv := generateVAPIDKeys(t)
	ch, _ := webpush.New(webpush.Config{
		VAPIDPublicKey:  pub,
		VAPIDPrivateKey: priv,
		Subject:         "mailto:test@kaivue.com",
	})
	if err := ch.CheckHealth(context.Background()); err != nil {
		t.Errorf("health check should pass: %v", err)
	}
}

func TestType(t *testing.T) {
	pub, priv := generateVAPIDKeys(t)
	ch, _ := webpush.New(webpush.Config{
		VAPIDPublicKey:  pub,
		VAPIDPrivateKey: priv,
		Subject:         "mailto:test@kaivue.com",
	})
	if ch.Type() != notifications.ChannelPush {
		t.Errorf("expected push, got %s", ch.Type())
	}
}

func TestNew_ValidationErrors(t *testing.T) {
	pub, priv := generateVAPIDKeys(t)
	tests := []struct {
		name string
		cfg  webpush.Config
	}{
		{"missing public key", webpush.Config{VAPIDPrivateKey: priv, Subject: "mailto:x"}},
		{"missing private key", webpush.Config{VAPIDPublicKey: pub, Subject: "mailto:x"}},
		{"missing subject", webpush.Config{VAPIDPublicKey: pub, VAPIDPrivateKey: priv}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := webpush.New(tt.cfg)
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	pub, priv := generateVAPIDKeys(t)
	ch, _ := webpush.New(webpush.Config{
		VAPIDPublicKey:  pub,
		VAPIDPrivateKey: priv,
		Subject:         "mailto:test@kaivue.com",
	})

	sub := makeSubscription(t, srv.URL+"/push/v1/a")
	ch.Send(context.Background(), notifications.PushMessage{Target: sub, Title: "x"})

	sent, failed, removed := ch.Stats()
	if sent != 1 {
		t.Errorf("expected 1 sent, got %d", sent)
	}
	if failed != 0 || removed != 0 {
		t.Errorf("expected 0 failed/removed, got %d/%d", failed, removed)
	}
}
