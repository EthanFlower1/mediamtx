package pdk

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// WebhookHandler processes inbound PDK event webhooks. It validates the
// HMAC-SHA256 signature, parses the payload, and delegates to the Service
// for persistence and video correlation.
type WebhookHandler struct {
	svc *Service
}

// NewWebhookHandler creates a handler wired to the given service.
func NewWebhookHandler(svc *Service) *WebhookHandler {
	return &WebhookHandler{svc: svc}
}

// ServeHTTP implements http.Handler. It expects:
//   - POST with JSON body
//   - X-PDK-Signature header containing hex-encoded HMAC-SHA256 of the body
//   - X-PDK-Tenant header identifying the Kaivue tenant
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID := r.Header.Get("X-PDK-Tenant")
	if tenantID == "" {
		http.Error(w, "missing X-PDK-Tenant header", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MiB max
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}

	// Look up the tenant's webhook secret for signature verification.
	cfg, err := h.svc.GetConfig(r.Context(), tenantID)
	if err != nil {
		http.Error(w, "tenant config not found", http.StatusUnauthorized)
		return
	}

	sig := r.Header.Get("X-PDK-Signature")
	if !verifySignature(body, sig, cfg.WebhookSecret) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	payload.Raw = string(body)

	if err := h.svc.IngestWebhookEvent(r.Context(), tenantID, payload); err != nil {
		http.Error(w, fmt.Sprintf("ingest failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// verifySignature checks the HMAC-SHA256 signature of the webhook body.
func verifySignature(body []byte, signatureHex, secret string) bool {
	if secret == "" || signatureHex == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signatureHex))
}
