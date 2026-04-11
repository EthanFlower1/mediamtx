package apns

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
	"github.com/golang-jwt/jwt/v5"
)

// Endpoints for Apple Push Notification service.
const (
	ProductionEndpoint = "https://api.push.apple.com"
	SandboxEndpoint    = "https://api.sandbox.push.apple.com"
)

// Config configures the APNs adapter.
type Config struct {
	KeyID      string
	TeamID     string
	BundleID   string
	PrivateKey string // PEM-encoded .p8 (ECDSA P-256)
	Production bool

	HTTPClient *http.Client

	// Endpoint overrides the APNs URL (for testing).
	Endpoint string
}

// Channel is the APNs delivery channel adapter.
type Channel struct {
	keyID    string
	teamID   string
	bundleID string
	key      *ecdsa.PrivateKey
	endpoint string
	client   *http.Client

	// JWT token cache — APNs tokens are valid for 1 hour.
	mu        sync.Mutex
	token     string
	tokenExp  time.Time

	sent    int64
	failed  int64
	removed int64
}

// compile-time interface check
var _ notifications.PushDeliveryChannel = (*Channel)(nil)

// New constructs a ready-to-use APNs channel.
func New(cfg Config) (*Channel, error) {
	if cfg.KeyID == "" {
		return nil, fmt.Errorf("apns: key_id required")
	}
	if cfg.TeamID == "" {
		return nil, fmt.Errorf("apns: team_id required")
	}
	if cfg.BundleID == "" {
		return nil, fmt.Errorf("apns: bundle_id required")
	}
	if cfg.PrivateKey == "" {
		return nil, fmt.Errorf("apns: private_key required")
	}

	key, err := parseP8Key(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("apns: parse key: %w", err)
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		if cfg.Production {
			endpoint = ProductionEndpoint
		} else {
			endpoint = SandboxEndpoint
		}
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &Channel{
		keyID:    cfg.KeyID,
		teamID:   cfg.TeamID,
		bundleID: cfg.BundleID,
		key:      key,
		endpoint: endpoint,
		client:   client,
	}, nil
}

// Type implements DeliveryChannel.
func (c *Channel) Type() notifications.ChannelType {
	return notifications.ChannelPush
}

// Send implements DeliveryChannel.
func (c *Channel) Send(ctx context.Context, msg notifications.PushMessage) (notifications.PushDeliveryResult, error) {
	token, err := c.getToken()
	if err != nil {
		return notifications.PushDeliveryResult{
			Target:       msg.Target,
			State:        notifications.PushStateFailed,
			ErrorMessage: fmt.Sprintf("jwt: %v", err),
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

	url := fmt.Sprintf("%s/3/device/%s", c.endpoint, msg.Target)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return notifications.PushDeliveryResult{
			Target:       msg.Target,
			State:        notifications.PushStateFailed,
			ErrorMessage: err.Error(),
		}, nil
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "bearer "+token)
	req.Header.Set("apns-topic", c.bundleID)

	if msg.Priority == "high" {
		req.Header.Set("apns-priority", "10")
	} else {
		req.Header.Set("apns-priority", "5")
	}
	if msg.TTL > 0 {
		exp := time.Now().Add(msg.TTL).Unix()
		req.Header.Set("apns-expiration", fmt.Sprintf("%d", exp))
	}
	if msg.CollapseKey != "" {
		req.Header.Set("apns-collapse-id", msg.CollapseKey)
	}

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

// BatchSend implements DeliveryChannel.
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

// CheckHealth implements DeliveryChannel. Validates that the signing key
// can produce a valid JWT.
func (c *Channel) CheckHealth(ctx context.Context) error {
	_, err := c.getToken()
	if err != nil {
		return fmt.Errorf("apns health: %w", err)
	}
	return nil
}

// Stats returns cumulative counters.
func (c *Channel) Stats() (sent, failed, removed int64) {
	return atomic.LoadInt64(&c.sent), atomic.LoadInt64(&c.failed), atomic.LoadInt64(&c.removed)
}

// ---------- internal ----------

type apnsPayload struct {
	Aps  apnsAps           `json:"aps"`
	Data map[string]string  `json:"data,omitempty"`
}

type apnsAps struct {
	Alert    apnsAlert `json:"alert"`
	Badge    *int      `json:"badge,omitempty"`
	Sound    string    `json:"sound,omitempty"`
	MutableContent int `json:"mutable-content,omitempty"`
}

type apnsAlert struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type apnsErrorResponse struct {
	Reason string `json:"reason"`
}

func (c *Channel) buildPayload(msg notifications.PushMessage) apnsPayload {
	aps := apnsAps{
		Alert: apnsAlert{
			Title: msg.Title,
			Body:  msg.Body,
		},
		Badge: msg.Badge,
		Sound: msg.Sound,
	}
	if msg.ImageURL != "" {
		aps.MutableContent = 1
	}

	p := apnsPayload{Aps: aps}
	if len(msg.Data) > 0 {
		p.Data = msg.Data
	}
	if msg.ImageURL != "" {
		if p.Data == nil {
			p.Data = make(map[string]string)
		}
		p.Data["image_url"] = msg.ImageURL
	}
	return p
}

func (c *Channel) getToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.tokenExp) {
		return c.token, nil
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss": c.teamID,
		"iat": now.Unix(),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	t.Header["kid"] = c.keyID

	signed, err := t.SignedString(c.key)
	if err != nil {
		return "", err
	}

	c.token = signed
	c.tokenExp = now.Add(50 * time.Minute) // refresh before 1h expiry
	return c.token, nil
}

func (c *Channel) parseResponse(resp *http.Response, target string) (notifications.PushDeliveryResult, error) {
	if resp.StatusCode == http.StatusOK {
		atomic.AddInt64(&c.sent, 1)
		apnsID := resp.Header.Get("apns-id")
		return notifications.PushDeliveryResult{
			Target:     target,
			State:      notifications.PushStateDelivered,
			PlatformID: apnsID,
		}, nil
	}

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var apnsErr apnsErrorResponse
	_ = json.Unmarshal(respBody, &apnsErr)

	result := notifications.PushDeliveryResult{
		Target:       target,
		State:        notifications.PushStateFailed,
		ErrorCode:    apnsErr.Reason,
		ErrorMessage: apnsErr.Reason,
	}

	switch apnsErr.Reason {
	case "BadDeviceToken", "Unregistered", "ExpiredToken":
		result.ShouldRemoveToken = true
		result.State = notifications.PushStateUnreachable
		atomic.AddInt64(&c.removed, 1)
	case "TooManyRequests":
		result.State = notifications.PushStateThrottled
	default:
		atomic.AddInt64(&c.failed, 1)
	}

	return result, nil
}

// parseP8Key parses a PEM-encoded ECDSA P-256 private key (.p8 format).
func parseP8Key(pemStr string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ecKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not ECDSA")
	}
	return ecKey, nil
}
