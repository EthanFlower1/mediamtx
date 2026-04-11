package monitoring

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// AlertHandler is a function that receives fired alerts. Implementations may
// send webhooks, page on-call engineers, or log to an audit trail.
type AlertHandler func(Alert) error

// AlertManager dispatches monitoring alerts to configured handlers. It
// supports multiple handlers (webhook, on-call integration, audit log) and
// deduplicates alerts within a cooldown window.
type AlertManager struct {
	mu       sync.RWMutex
	handlers []AlertHandler
	fired    []Alert

	// cooldown prevents the same alert type from firing repeatedly for the
	// same model within the cooldown window.
	cooldown     time.Duration
	lastFiredMap map[string]time.Time
}

// NewAlertManager creates an AlertManager with the given cooldown duration.
// Pass 0 for no cooldown (every alert fires).
func NewAlertManager(cooldown time.Duration) *AlertManager {
	return &AlertManager{
		cooldown:     cooldown,
		lastFiredMap: make(map[string]time.Time),
	}
}

// AddHandler registers an alert handler.
func (am *AlertManager) AddHandler(h AlertHandler) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.handlers = append(am.handlers, h)
}

// Fire dispatches an alert to all handlers, respecting the cooldown window.
func (am *AlertManager) Fire(alert Alert) error {
	am.mu.Lock()
	dedupKey := fmt.Sprintf("%s:%s:%s", alert.Type, alert.Key.String(), alert.Severity)

	if am.cooldown > 0 {
		if lastFired, ok := am.lastFiredMap[dedupKey]; ok {
			if time.Since(lastFired) < am.cooldown {
				am.mu.Unlock()
				return nil // suppressed by cooldown
			}
		}
	}
	am.lastFiredMap[dedupKey] = time.Now()
	am.fired = append(am.fired, alert)
	handlers := make([]AlertHandler, len(am.handlers))
	copy(handlers, am.handlers)
	am.mu.Unlock()

	var firstErr error
	for _, h := range handlers {
		if err := h(alert); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// FiredAlerts returns a copy of all alerts that have been fired.
func (am *AlertManager) FiredAlerts() []Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()
	out := make([]Alert, len(am.fired))
	copy(out, am.fired)
	return out
}

// -----------------------------------------------------------------------
// Built-in handlers
// -----------------------------------------------------------------------

// WebhookHandler returns an AlertHandler that POSTs alerts as JSON to the
// given URL. This integrates with PagerDuty, Opsgenie, Slack, or any
// generic webhook receiver.
func WebhookHandler(url string, client *http.Client) AlertHandler {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return func(alert Alert) error {
		payload := map[string]any{
			"id":          alert.ID,
			"severity":    alert.Severity,
			"type":        alert.Type,
			"model":       alert.Key.String(),
			"tenant_id":   alert.Key.TenantID,
			"model_id":    alert.Key.ModelID,
			"version":     alert.Key.Version,
			"message":     alert.Message,
			"value":       alert.Value,
			"threshold":   alert.Threshold,
			"timestamp":   alert.Timestamp.Format(time.RFC3339),
			"rotation_id": alert.OnCallRotationID,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("monitoring: marshal webhook payload: %w", err)
		}
		resp, err := client.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("monitoring: webhook POST to %s: %w", url, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return fmt.Errorf("monitoring: webhook returned %d", resp.StatusCode)
		}
		return nil
	}
}

// LogHandler returns an AlertHandler that logs alerts via the standard log
// package. Useful for development and as a fallback.
func LogHandler() AlertHandler {
	return func(alert Alert) error {
		fmt.Printf("[ALERT] severity=%s type=%s model=%s msg=%s value=%.4f threshold=%.4f\n",
			alert.Severity, alert.Type, alert.Key, alert.Message,
			alert.Value, alert.Threshold)
		return nil
	}
}
