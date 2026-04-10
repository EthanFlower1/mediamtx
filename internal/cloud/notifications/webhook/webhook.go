// Package webhook implements a DeliveryChannel adapter for outbound
// webhooks with HMAC-SHA256 request signing and configurable retry.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

// Config configures the outbound webhook adapter.
type Config struct {
	// URL is the customer-configured webhook endpoint.
	URL string
	// Secret is the HMAC-SHA256 signing secret. If empty, requests are unsigned.
	Secret string
	// MaxRetries is the number of retry attempts on failure (default 2).
	MaxRetries int
	// RetryDelay is the base delay between retries (default 1s, doubles each attempt).
	RetryDelay time.Duration
	// HTTPClient overrides the default HTTP client.
	HTTPClient *http.Client
}

// Adapter delivers notifications to a customer-configured webhook URL
// with HMAC-SHA256 signing for payload verification.
type Adapter struct {
	url        string
	secret     string
	maxRetries int
	retryDelay time.Duration
	client     *http.Client
}

// New creates an outbound webhook adapter.
func New(cfg Config) (*Adapter, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("webhook: URL is required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 2
	}
	retryDelay := cfg.RetryDelay
	if retryDelay <= 0 {
		retryDelay = time.Second
	}
	return &Adapter{
		url:        cfg.URL,
		secret:     cfg.Secret,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
		client:     client,
	}, nil
}

// Type returns the channel type.
func (a *Adapter) Type() notifications.ChannelType {
	return notifications.ChannelWebhook
}

// Send delivers a single message to the webhook endpoint with HMAC signing.
func (a *Adapter) Send(ctx context.Context, msg notifications.Message) notifications.DeliveryResult {
	body, err := json.Marshal(msg)
	if err != nil {
		return fail(msg.ID, fmt.Sprintf("marshal: %v", err))
	}

	var lastErr string
	delay := a.retryDelay
	for attempt := 0; attempt <= a.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return fail(msg.ID, fmt.Sprintf("context canceled after %d attempts: %v", attempt, ctx.Err()))
			case <-time.After(delay):
				delay *= 2
			}
		}

		result, errMsg := a.doSend(ctx, msg.ID, body)
		if errMsg == "" {
			return result
		}
		lastErr = errMsg
	}

	return fail(msg.ID, fmt.Sprintf("all %d attempts failed, last: %s", a.maxRetries+1, lastErr))
}

func (a *Adapter) doSend(ctx context.Context, msgID string, body []byte) (notifications.DeliveryResult, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, bytes.NewReader(body))
	if err != nil {
		return notifications.DeliveryResult{}, fmt.Sprintf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "KaiVue-Webhook/1.0")

	if a.secret != "" {
		sig := ComputeHMAC(body, a.secret)
		req.Header.Set("X-Kaivue-Signature", sig)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return notifications.DeliveryResult{}, fmt.Sprintf("post: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode >= 500 {
		return notifications.DeliveryResult{}, fmt.Sprintf("server error HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Client errors are not retryable.
		return fail(msgID, fmt.Sprintf("webhook returned HTTP %d", resp.StatusCode)), ""
	}

	return notifications.DeliveryResult{
		MessageID:   msgID,
		ChannelType: notifications.ChannelWebhook,
		State:       notifications.DeliverySuccess,
	}, ""
}

// BatchSend delivers multiple messages sequentially.
func (a *Adapter) BatchSend(ctx context.Context, msgs []notifications.Message) []notifications.DeliveryResult {
	results := make([]notifications.DeliveryResult, 0, len(msgs))
	for _, msg := range msgs {
		results = append(results, a.Send(ctx, msg))
	}
	return results
}

// CheckHealth sends a GET to the webhook URL to verify reachability.
func (a *Adapter) CheckHealth(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.url, nil)
	if err != nil {
		return fmt.Errorf("webhook health: %w", err)
	}
	req.Header.Set("User-Agent", "KaiVue-Webhook/1.0")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook health: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	// Any non-5xx response means the endpoint is reachable.
	if resp.StatusCode >= 500 {
		return fmt.Errorf("webhook health: server error (HTTP %d)", resp.StatusCode)
	}
	return nil
}

// ComputeHMAC computes the HMAC-SHA256 hex digest for the given body and secret.
// Exported so webhook consumers can verify signatures.
func ComputeHMAC(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// VerifyHMAC checks that the provided signature matches the expected HMAC.
func VerifyHMAC(body []byte, secret, signature string) bool {
	expected := ComputeHMAC(body, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

func fail(msgID, errMsg string) notifications.DeliveryResult {
	return notifications.DeliveryResult{
		MessageID:    msgID,
		ChannelType:  notifications.ChannelWebhook,
		State:        notifications.DeliveryFailure,
		ErrorMessage: errMsg,
	}
}
