package automation

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// WebhookHandler is the unified HTTP handler that serves all three automation
// platforms. It manages subscriptions and dispatches trigger payloads.
type WebhookHandler struct {
	Store      *SubscriptionStore
	HTTPClient *http.Client // injectable for testing

	// ActionHandler is called when an action request arrives.  If nil the
	// handler returns 501 Not Implemented.
	ActionHandler func(ctx context.Context, req ActionRequest) (*ActionResponse, error)
}

// NewWebhookHandler creates a WebhookHandler with sensible defaults.
func NewWebhookHandler() *WebhookHandler {
	return &WebhookHandler{
		Store:      NewSubscriptionStore(),
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// ---- subscription management ------------------------------------------------

// SubscribeRequest is the JSON body for POST /webhooks/subscribe.
type SubscribeRequest struct {
	Platform   Platform `json:"platform"`
	TriggerKey string   `json:"trigger_key"`
	WebhookURL string   `json:"webhook_url"`
	Secret     string   `json:"secret,omitempty"`
}

// SubscribeResponse is returned after a successful subscription.
type SubscribeResponse struct {
	ID string `json:"id"`
}

// HandleSubscribe registers a new webhook subscription.
func (h *WebhookHandler) HandleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.WebhookURL == "" || req.TriggerKey == "" || req.Platform == "" {
		http.Error(w, "webhook_url, trigger_key, and platform are required", http.StatusBadRequest)
		return
	}

	sub := &Subscription{
		ID:         uuid.New().String(),
		Platform:   req.Platform,
		TriggerKey: req.TriggerKey,
		WebhookURL: req.WebhookURL,
		Secret:     req.Secret,
		Active:     true,
	}
	h.Store.Add(sub)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(SubscribeResponse{ID: sub.ID})
}

// UnsubscribeRequest is the JSON body for POST /webhooks/unsubscribe.
type UnsubscribeRequest struct {
	ID string `json:"id"`
}

// HandleUnsubscribe removes a webhook subscription.
func (h *WebhookHandler) HandleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req UnsubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if !h.Store.Remove(req.ID) {
		http.Error(w, "subscription not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleSample returns sample data for the given trigger key. Zapier's
// REST-hook protocol uses this to show fields at design-time.
func (h *WebhookHandler) HandleSample(w http.ResponseWriter, r *http.Request, triggerKey string) {
	for _, t := range SharedTriggers() {
		if t.Key == triggerKey {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]any{t.SampleData})
			return
		}
	}
	http.Error(w, "unknown trigger key", http.StatusNotFound)
}

// ---- action handling --------------------------------------------------------

// HandleAction is the unified endpoint for executing an action.
func (h *WebhookHandler) HandleAction(w http.ResponseWriter, r *http.Request, actionKey string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.ActionHandler == nil {
		http.Error(w, "action handler not configured", http.StatusNotImplemented)
		return
	}

	var req ActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.ActionType = ActionType(actionKey)

	resp, err := h.ActionHandler(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// ---- trigger dispatch -------------------------------------------------------

// Dispatch sends a trigger payload to every active subscriber of the given
// trigger key. Deliveries run concurrently; errors are logged but not
// returned.
func (h *WebhookHandler) Dispatch(ctx context.Context, triggerKey string, payload TriggerPayload) {
	subs := h.Store.ByTrigger(triggerKey)
	if len(subs) == 0 {
		return
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("automation: failed to marshal trigger payload: %v", err)
		return
	}

	var wg sync.WaitGroup
	for _, sub := range subs {
		wg.Add(1)
		go func(s *Subscription) {
			defer wg.Done()
			if err := h.deliver(ctx, s, body); err != nil {
				log.Printf("automation: delivery to %s (%s) failed: %v", s.WebhookURL, s.Platform, err)
			}
		}(sub)
	}
	wg.Wait()
}

func (h *WebhookHandler) deliver(ctx context.Context, sub *Subscription, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Automation-Platform", string(sub.Platform))

	if sub.Secret != "" {
		mac := hmac.New(sha256.New, []byte(sub.Secret))
		mac.Write(body)
		req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	resp, err := h.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ---- HTTP mux helper --------------------------------------------------------

// RegisterRoutes attaches all webhook/action endpoints to the given ServeMux.
func (h *WebhookHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/integrations/automation/webhooks/subscribe", h.HandleSubscribe)
	mux.HandleFunc("/api/v1/integrations/automation/webhooks/unsubscribe", h.HandleUnsubscribe)

	// Sample endpoint per trigger key
	for _, t := range SharedTriggers() {
		key := t.Key
		mux.HandleFunc("/api/v1/integrations/automation/webhooks/sample/"+key, func(w http.ResponseWriter, r *http.Request) {
			h.HandleSample(w, r, key)
		})
	}

	// Action endpoints
	for _, a := range SharedActions() {
		key := a.Key
		mux.HandleFunc("/api/v1/integrations/automation/actions/"+key, func(w http.ResponseWriter, r *http.Request) {
			h.HandleAction(w, r, key)
		})
	}

	// Platform descriptor endpoints
	mux.HandleFunc("/api/v1/integrations/automation/platforms/zapier", func(w http.ResponseWriter, r *http.Request) {
		// baseURL is derived from the request
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}
		base := scheme + "://" + r.Host
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DefaultZapierApp(base))
	})

	mux.HandleFunc("/api/v1/integrations/automation/platforms/make", func(w http.ResponseWriter, r *http.Request) {
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}
		base := scheme + "://" + r.Host
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DefaultMakeApp(base))
	})

	mux.HandleFunc("/api/v1/integrations/automation/platforms/n8n", func(w http.ResponseWriter, r *http.Request) {
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}
		base := scheme + "://" + r.Host
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DefaultN8NNode(base))
	})
}
