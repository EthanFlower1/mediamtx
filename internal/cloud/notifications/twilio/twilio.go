// Package twilio implements the notifications.DeliveryChannel interface
// for SMS, Voice (critical-ack), and WhatsApp delivery via the Twilio
// REST API (KAI-476).
//
// Features:
//   - SMS via Twilio Messaging API.
//   - Voice calls with TwiML <Say> for critical alert acknowledgement.
//   - WhatsApp via Twilio WhatsApp Business API.
//   - Per-tenant messaging service SID / from-number resolution.
//   - Health check via the Twilio /Accounts/{SID}.json endpoint.
//
// The adapter depends on a CredentialResolver seam for testability.
package twilio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

// -----------------------------------------------------------------------
// Credential resolver (seam)
// -----------------------------------------------------------------------

// Credentials holds the per-tenant Twilio configuration.
type Credentials struct {
	AccountSID        string
	AuthToken         string
	FromNumber        string // E.164 for SMS/Voice
	MessagingService  string // optional Twilio messaging service SID
	WhatsAppFrom      string // e.g., "whatsapp:+14155238886"
}

// CredentialResolver maps a tenant ID to Twilio credentials.
type CredentialResolver interface {
	Resolve(ctx context.Context, tenantID string) (Credentials, error)
}

// StaticCredentialResolver is a test double.
type StaticCredentialResolver struct {
	Creds Credentials
}

func (s *StaticCredentialResolver) Resolve(_ context.Context, _ string) (Credentials, error) {
	return s.Creds, nil
}

// -----------------------------------------------------------------------
// HTTP client seam
// -----------------------------------------------------------------------

// HTTPDoer is the minimal HTTP interface. *http.Client satisfies this.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// -----------------------------------------------------------------------
// Config
// -----------------------------------------------------------------------

// Config holds adapter dependencies.
type Config struct {
	Resolver CredentialResolver
	Client   HTTPDoer
	BaseURL  string // defaults to "https://api.twilio.com"
}

// -----------------------------------------------------------------------
// Adapter
// -----------------------------------------------------------------------

// Adapter implements notifications.DeliveryChannel for Twilio
// (SMS + Voice + WhatsApp).
type Adapter struct {
	resolver CredentialResolver
	client   HTTPDoer
	baseURL  string
}

// New creates a Twilio adapter.
func New(cfg Config) (*Adapter, error) {
	if cfg.Resolver == nil {
		return nil, fmt.Errorf("twilio: CredentialResolver is required")
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.twilio.com"
	}
	return &Adapter{
		resolver: cfg.Resolver,
		client:   cfg.Client,
		baseURL:  cfg.BaseURL,
	}, nil
}

func (a *Adapter) Name() string { return "twilio" }

func (a *Adapter) SupportedTypes() []notifications.MessageType {
	return []notifications.MessageType{
		notifications.MessageTypeSMS,
		notifications.MessageTypeVoice,
		notifications.MessageTypeWhatsApp,
	}
}

func (a *Adapter) Send(ctx context.Context, msg notifications.Message) (notifications.DeliveryResult, error) {
	if len(msg.To) == 0 {
		return notifications.DeliveryResult{}, fmt.Errorf("twilio: no recipients")
	}
	single := msg
	single.To = msg.To[:1]
	results, err := a.BatchSend(ctx, single)
	if err != nil {
		return notifications.DeliveryResult{}, err
	}
	return results[0], nil
}

func (a *Adapter) BatchSend(ctx context.Context, msg notifications.Message) ([]notifications.DeliveryResult, error) {
	if err := msg.Validate(); err != nil {
		return nil, fmt.Errorf("twilio: %w", err)
	}

	creds, err := a.resolver.Resolve(ctx, msg.TenantID)
	if err != nil {
		return nil, fmt.Errorf("twilio: resolve credentials: %w", err)
	}

	results := make([]notifications.DeliveryResult, 0, len(msg.To))
	for _, recipient := range msg.To {
		result, sendErr := a.sendOne(ctx, msg, recipient, creds)
		if sendErr != nil {
			// Return error to trigger dispatcher retry on transient failure.
			return nil, sendErr
		}
		results = append(results, result)
	}
	return results, nil
}

func (a *Adapter) sendOne(
	ctx context.Context,
	msg notifications.Message,
	recipient notifications.Recipient,
	creds Credentials,
) (notifications.DeliveryResult, error) {
	switch msg.Type {
	case notifications.MessageTypeSMS:
		return a.sendSMS(ctx, msg, recipient, creds)
	case notifications.MessageTypeVoice:
		return a.sendVoice(ctx, msg, recipient, creds)
	case notifications.MessageTypeWhatsApp:
		return a.sendWhatsApp(ctx, msg, recipient, creds)
	default:
		return notifications.DeliveryResult{}, fmt.Errorf("twilio: unsupported message type: %s", msg.Type)
	}
}

func (a *Adapter) sendSMS(
	ctx context.Context,
	msg notifications.Message,
	recipient notifications.Recipient,
	creds Credentials,
) (notifications.DeliveryResult, error) {
	form := url.Values{}
	form.Set("To", recipient.Address)
	form.Set("Body", msg.Body)

	if creds.MessagingService != "" {
		form.Set("MessagingServiceSid", creds.MessagingService)
	} else {
		form.Set("From", creds.FromNumber)
	}

	if msg.ID != "" {
		// Twilio does not have native idempotency headers for messages
		// but we can use StatusCallback for tracking.
		if msg.CallbackURL != "" {
			form.Set("StatusCallback", msg.CallbackURL)
		}
	}

	return a.postMessage(ctx, msg, recipient, creds, form)
}

func (a *Adapter) sendVoice(
	ctx context.Context,
	msg notifications.Message,
	recipient notifications.Recipient,
	creds Credentials,
) (notifications.DeliveryResult, error) {
	// Build inline TwiML for the voice call.
	twiml := fmt.Sprintf(
		`<Response><Say voice="alice">%s</Say><Pause length="1"/><Say voice="alice">Press any key to acknowledge.</Say><Gather numDigits="1" timeout="30"/></Response>`,
		xmlEscape(msg.Body),
	)

	form := url.Values{}
	form.Set("To", recipient.Address)
	form.Set("From", creds.FromNumber)
	form.Set("Twiml", twiml)

	if msg.CallbackURL != "" {
		form.Set("StatusCallback", msg.CallbackURL)
	}

	return a.postCall(ctx, msg, recipient, creds, form)
}

func (a *Adapter) sendWhatsApp(
	ctx context.Context,
	msg notifications.Message,
	recipient notifications.Recipient,
	creds Credentials,
) (notifications.DeliveryResult, error) {
	form := url.Values{}
	// WhatsApp recipients/senders must be prefixed with "whatsapp:".
	to := recipient.Address
	if !strings.HasPrefix(to, "whatsapp:") {
		to = "whatsapp:" + to
	}
	form.Set("To", to)

	from := creds.WhatsAppFrom
	if from == "" {
		from = "whatsapp:" + creds.FromNumber
	}
	form.Set("From", from)

	if msg.TemplateID != "" {
		// Twilio WhatsApp uses Content SID for templates.
		form.Set("ContentSid", msg.TemplateID)
		if len(msg.TemplateData) > 0 {
			vars, _ := json.Marshal(msg.TemplateData)
			form.Set("ContentVariables", string(vars))
		}
	} else {
		form.Set("Body", msg.Body)
	}

	if msg.CallbackURL != "" {
		form.Set("StatusCallback", msg.CallbackURL)
	}

	return a.postMessage(ctx, msg, recipient, creds, form)
}

func (a *Adapter) postMessage(
	ctx context.Context,
	msg notifications.Message,
	recipient notifications.Recipient,
	creds Credentials,
	form url.Values,
) (notifications.DeliveryResult, error) {
	endpoint := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Messages.json",
		a.baseURL, creds.AccountSID)
	return a.doPost(ctx, msg, recipient, creds, endpoint, form)
}

func (a *Adapter) postCall(
	ctx context.Context,
	msg notifications.Message,
	recipient notifications.Recipient,
	creds Credentials,
	form url.Values,
) (notifications.DeliveryResult, error) {
	endpoint := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Calls.json",
		a.baseURL, creds.AccountSID)
	return a.doPost(ctx, msg, recipient, creds, endpoint, form)
}

func (a *Adapter) doPost(
	ctx context.Context,
	msg notifications.Message,
	recipient notifications.Recipient,
	creds Credentials,
	endpoint string,
	form url.Values,
) (notifications.DeliveryResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return notifications.DeliveryResult{}, fmt.Errorf("twilio: new request: %w", err)
	}
	req.SetBasicAuth(creds.AccountSID, creds.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.client.Do(req)
	if err != nil {
		return notifications.DeliveryResult{}, fmt.Errorf("twilio: http do: %w", err)
	}
	defer resp.Body.Close()

	now := time.Now().UTC()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	// Twilio returns 201 Created on success.
	if resp.StatusCode == http.StatusCreated {
		var parsed struct {
			SID string `json:"sid"`
		}
		_ = json.Unmarshal(respBody, &parsed)

		return notifications.DeliveryResult{
			MessageID:         msg.ID,
			ProviderMessageID: parsed.SID,
			Recipient:         recipient.Address,
			State:             notifications.DeliveryStateDelivered,
			Timestamp:         now,
		}, nil
	}

	errMsg := fmt.Sprintf("twilio: HTTP %d: %s", resp.StatusCode, string(respBody))

	// Retryable: 429 or 5xx.
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return notifications.DeliveryResult{}, fmt.Errorf("%s (retryable)", errMsg)
	}

	// Permanent failure.
	return notifications.DeliveryResult{
		MessageID:    msg.ID,
		Recipient:    recipient.Address,
		State:        notifications.DeliveryStateFailed,
		ErrorMessage: errMsg,
		Timestamp:    now,
	}, nil
}

func (a *Adapter) CheckHealth(ctx context.Context) (notifications.HealthStatus, error) {
	creds, err := a.resolver.Resolve(ctx, "")
	if err != nil {
		return notifications.HealthStatus{
			Healthy:   false,
			Message:   fmt.Sprintf("twilio: cannot resolve credentials: %v", err),
			CheckedAt: time.Now().UTC(),
		}, nil
	}

	start := time.Now()
	endpoint := fmt.Sprintf("%s/2010-04-01/Accounts/%s.json",
		a.baseURL, creds.AccountSID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return notifications.HealthStatus{}, err
	}
	req.SetBasicAuth(creds.AccountSID, creds.AuthToken)

	resp, err := a.client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return notifications.HealthStatus{
			Healthy:   false,
			Latency:   elapsed,
			Message:   fmt.Sprintf("twilio: health check failed: %v", err),
			CheckedAt: time.Now().UTC(),
		}, nil
	}
	defer resp.Body.Close()

	healthy := resp.StatusCode >= 200 && resp.StatusCode < 300
	status := fmt.Sprintf("twilio: HTTP %d", resp.StatusCode)
	return notifications.HealthStatus{
		Healthy:   healthy,
		Latency:   elapsed,
		Message:   status,
		CheckedAt: time.Now().UTC(),
	}, nil
}

// xmlEscape escapes special XML characters for TwiML injection safety.
func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}
