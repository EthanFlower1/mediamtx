package fcm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

// DefaultEndpoint is the FCM v1 HTTP API base URL.
const DefaultEndpoint = "https://fcm.googleapis.com"

// TokenSource provides OAuth2 access tokens for the FCM v1 API.
// In production this wraps a Google service account credential;
// tests provide a static token.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// Config configures the FCM adapter.
type Config struct {
	ProjectID   string
	TokenSource TokenSource
	HTTPClient  *http.Client

	// Endpoint overrides the FCM API base URL (for testing).
	Endpoint string
}

// Channel is the FCM delivery channel adapter.
type Channel struct {
	projectID   string
	tokenSource TokenSource
	client      *http.Client
	endpoint    string

	mu        sync.Mutex
	sent      int64
	failed    int64
	removed   int64
}

// compile-time interface check
var _ notifications.PushDeliveryChannel = (*Channel)(nil)

// New constructs a ready-to-use FCM channel.
func New(cfg Config) (*Channel, error) {
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("fcm: project_id required")
	}
	if cfg.TokenSource == nil {
		return nil, fmt.Errorf("fcm: token source required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	return &Channel{
		projectID:   cfg.ProjectID,
		tokenSource: cfg.TokenSource,
		client:      client,
		endpoint:    endpoint,
	}, nil
}

// Type implements DeliveryChannel.
func (c *Channel) Type() notifications.ChannelType {
	return notifications.ChannelPush
}

// Send implements DeliveryChannel.
func (c *Channel) Send(ctx context.Context, msg notifications.PushMessage) (notifications.PushDeliveryResult, error) {
	token, err := c.tokenSource.Token(ctx)
	if err != nil {
		return notifications.PushDeliveryResult{
			Target:       msg.Target,
			State:        notifications.PushStateFailed,
			ErrorMessage: fmt.Sprintf("token source: %v", err),
		}, nil
	}

	payload := c.buildPayload(msg)
	body, err := json.Marshal(payload)
	if err != nil {
		return notifications.PushDeliveryResult{
			Target:       msg.Target,
			State:        notifications.PushStateFailed,
			ErrorMessage: fmt.Sprintf("marshal: %v", err),
		}, nil
	}

	url := fmt.Sprintf("%s/v1/projects/%s/messages:send", c.endpoint, c.projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return notifications.PushDeliveryResult{
			Target:       msg.Target,
			State:        notifications.PushStateFailed,
			ErrorMessage: err.Error(),
		}, nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.client.Do(req)
	if err != nil {
		atomic.AddInt64(&c.failed, 1)
		return notifications.PushDeliveryResult{
			Target:       msg.Target,
			State:        notifications.PushStateFailed,
			ErrorMessage: err.Error(),
		}, nil
	}
	defer resp.Body.Close()

	return c.parseResponse(resp, msg.Target)
}

// BatchSend implements DeliveryChannel. FCM v1 does not have a native batch
// endpoint, so we fan out individual sends.
func (c *Channel) BatchSend(ctx context.Context, msg notifications.BatchMessage) ([]notifications.PushDeliveryResult, error) {
	results := make([]notifications.PushDeliveryResult, len(msg.Targets))
	for i, tgt := range msg.Targets {
		single := notifications.PushMessage{
			MessageID:   fmt.Sprintf("%s-%d", msg.MessageID, i),
			TenantID:    msg.TenantID,
			UserID:      tgt.UserID,
			Target:      tgt.DeviceToken,
			Platform:    tgt.Platform,
			Title:       msg.Title,
			Body:        msg.Body,
			Data:        msg.Data,
			ImageURL:    msg.ImageURL,
			Priority:    msg.Priority,
			TTL:         msg.TTL,
			CollapseKey: msg.CollapseKey,
			Badge:       msg.Badge,
			Sound:       msg.Sound,
		}
		result, err := c.Send(ctx, single)
		if err != nil {
			results[i] = notifications.PushDeliveryResult{
				Target:       tgt.DeviceToken,
				State:        notifications.PushStateFailed,
				ErrorMessage: err.Error(),
			}
			continue
		}
		results[i] = result
	}
	return results, nil
}

// CheckHealth implements DeliveryChannel. We validate that we can obtain an
// access token — a lightweight check that does not hit the FCM send endpoint.
func (c *Channel) CheckHealth(ctx context.Context) error {
	_, err := c.tokenSource.Token(ctx)
	if err != nil {
		return fmt.Errorf("fcm health: %w", err)
	}
	return nil
}

// Metrics returns delivery statistics.
func (c *Channel) Metrics() notifications.PushDeliveryResult {
	return notifications.PushDeliveryResult{}
}

// Stats returns cumulative counters.
func (c *Channel) Stats() (sent, failed, removed int64) {
	return atomic.LoadInt64(&c.sent), atomic.LoadInt64(&c.failed), atomic.LoadInt64(&c.removed)
}

// ---------- internal ----------

type fcmRequest struct {
	Message fcmMessage `json:"message"`
}

type fcmMessage struct {
	Token        string            `json:"token"`
	Notification *fcmNotification  `json:"notification,omitempty"`
	Data         map[string]string `json:"data,omitempty"`
	Android      *fcmAndroid       `json:"android,omitempty"`
	Webpush      *fcmWebpush       `json:"webpush,omitempty"`
}

type fcmNotification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Image string `json:"image,omitempty"`
}

type fcmAndroid struct {
	Priority string         `json:"priority,omitempty"`
	TTL      string         `json:"ttl,omitempty"`
	Notification *fcmAndroidNotification `json:"notification,omitempty"`
	CollapseKey  string     `json:"collapse_key,omitempty"`
}

type fcmAndroidNotification struct {
	Sound string `json:"sound,omitempty"`
}

type fcmWebpush struct {
	Headers map[string]string `json:"headers,omitempty"`
}

type fcmResponse struct {
	Name  string    `json:"name"`
	Error *fcmError `json:"error,omitempty"`
}

type fcmError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

func (c *Channel) buildPayload(msg notifications.PushMessage) fcmRequest {
	m := fcmMessage{
		Token: msg.Target,
		Notification: &fcmNotification{
			Title: msg.Title,
			Body:  msg.Body,
			Image: msg.ImageURL,
		},
		Data: msg.Data,
	}

	// Android-specific options
	android := &fcmAndroid{}
	if msg.Priority == "high" {
		android.Priority = "HIGH"
	} else {
		android.Priority = "NORMAL"
	}
	if msg.TTL > 0 {
		android.TTL = fmt.Sprintf("%ds", int(msg.TTL.Seconds()))
	}
	if msg.CollapseKey != "" {
		android.CollapseKey = msg.CollapseKey
	}
	if msg.Sound != "" {
		android.Notification = &fcmAndroidNotification{Sound: msg.Sound}
	}
	m.Android = android

	return fcmRequest{Message: m}
}

func (c *Channel) parseResponse(resp *http.Response, target string) (notifications.PushDeliveryResult, error) {
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		atomic.AddInt64(&c.failed, 1)
		return notifications.PushDeliveryResult{
			Target:       target,
			State:        notifications.PushStateFailed,
			ErrorMessage: fmt.Sprintf("read response: %v", err),
		}, nil
	}

	if resp.StatusCode == http.StatusOK {
		var r fcmResponse
		_ = json.Unmarshal(respBody, &r)
		atomic.AddInt64(&c.sent, 1)
		return notifications.PushDeliveryResult{
			Target:     target,
			State:      notifications.PushStateDelivered,
			PlatformID: r.Name,
		}, nil
	}

	var r fcmResponse
	_ = json.Unmarshal(respBody, &r)

	result := notifications.PushDeliveryResult{
		Target: target,
		State:  notifications.PushStateFailed,
	}
	if r.Error != nil {
		result.ErrorCode = r.Error.Status
		result.ErrorMessage = r.Error.Message
	}

	switch resp.StatusCode {
	case http.StatusNotFound, http.StatusGone:
		// Token is invalid or unregistered
		result.ShouldRemoveToken = true
		result.State = notifications.PushStateUnreachable
		atomic.AddInt64(&c.removed, 1)
	case http.StatusTooManyRequests:
		result.State = notifications.PushStateThrottled
	default:
		atomic.AddInt64(&c.failed, 1)
	}

	return result, nil
}
