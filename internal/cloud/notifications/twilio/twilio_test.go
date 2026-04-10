package twilio_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
	"github.com/bluenviron/mediamtx/internal/cloud/notifications/twilio"
)

func defaultCreds() twilio.Credentials {
	return twilio.Credentials{
		AccountSID:   "AC_test_sid",
		AuthToken:    "test_token",
		FromNumber:   "+15005550006",
		WhatsAppFrom: "whatsapp:+14155238886",
	}
}

func newTestAdapter(t *testing.T, handler http.HandlerFunc) *twilio.Adapter {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	a, err := twilio.New(twilio.Config{
		Resolver: &twilio.StaticCredentialResolver{Creds: defaultCreds()},
		Client:   srv.Client(),
		BaseURL:  srv.URL,
	})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	return a
}

func TestTwilioName(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {})
	if a.Name() != "twilio" {
		t.Errorf("expected name twilio, got %s", a.Name())
	}
}

func TestTwilioSupportedTypes(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {})
	types := a.SupportedTypes()
	if len(types) != 3 {
		t.Fatalf("expected 3 types, got %d", len(types))
	}
	typeSet := make(map[notifications.MessageType]bool)
	for _, mt := range types {
		typeSet[mt] = true
	}
	for _, expected := range []notifications.MessageType{
		notifications.MessageTypeSMS,
		notifications.MessageTypeVoice,
		notifications.MessageTypeWhatsApp,
	} {
		if !typeSet[expected] {
			t.Errorf("expected type %s to be supported", expected)
		}
	}
}

func TestTwilioSMSSendSuccess(t *testing.T) {
	var capturedPath string
	var capturedBody string
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		r.ParseForm()
		capturedBody = r.PostForm.Encode()
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"sid":"SM_test_sid_123"}`))
	})

	msg := notifications.Message{
		ID:       "sms-001",
		Type:     notifications.MessageTypeSMS,
		TenantID: "tenant-1",
		To:       []notifications.Recipient{{Address: "+14155551234"}},
		Body:     "Camera Front Door is offline",
	}

	result, err := a.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if result.State != notifications.DeliveryStateDelivered {
		t.Errorf("expected delivered, got %s", result.State)
	}
	if result.ProviderMessageID != "SM_test_sid_123" {
		t.Errorf("expected provider SID SM_test_sid_123, got %s", result.ProviderMessageID)
	}

	expectedPath := "/2010-04-01/Accounts/AC_test_sid/Messages.json"
	if capturedPath != expectedPath {
		t.Errorf("expected path %s, got %s", expectedPath, capturedPath)
	}
	if !strings.Contains(capturedBody, "To=%2B14155551234") {
		t.Errorf("expected To in body, got: %s", capturedBody)
	}
}

func TestTwilioVoiceCallSuccess(t *testing.T) {
	var capturedPath string
	var capturedForm string
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		r.ParseForm()
		capturedForm = r.PostForm.Get("Twiml")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"sid":"CA_test_call_456"}`))
	})

	msg := notifications.Message{
		ID:       "voice-001",
		Type:     notifications.MessageTypeVoice,
		TenantID: "tenant-1",
		To:       []notifications.Recipient{{Address: "+14155551234"}},
		Body:     "Critical alert: server room temperature exceeded 90 degrees",
	}

	result, err := a.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if result.State != notifications.DeliveryStateDelivered {
		t.Errorf("expected delivered, got %s", result.State)
	}
	if result.ProviderMessageID != "CA_test_call_456" {
		t.Errorf("expected provider SID CA_test_call_456, got %s", result.ProviderMessageID)
	}

	expectedPath := "/2010-04-01/Accounts/AC_test_sid/Calls.json"
	if capturedPath != expectedPath {
		t.Errorf("expected path %s, got %s", expectedPath, capturedPath)
	}

	// Verify TwiML contains the message and Gather for ack.
	if !strings.Contains(capturedForm, "<Response>") {
		t.Error("expected TwiML Response tag")
	}
	if !strings.Contains(capturedForm, "<Say") {
		t.Error("expected TwiML Say tag")
	}
	if !strings.Contains(capturedForm, "<Gather") {
		t.Error("expected TwiML Gather tag for acknowledgement")
	}
}

func TestTwilioWhatsAppSuccess(t *testing.T) {
	var capturedBody string
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		capturedBody = r.PostForm.Encode()
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"sid":"WA_test_789"}`))
	})

	msg := notifications.Message{
		ID:       "wa-001",
		Type:     notifications.MessageTypeWhatsApp,
		TenantID: "tenant-1",
		To:       []notifications.Recipient{{Address: "+14155551234"}},
		Body:     "Alert: motion detected",
	}

	result, err := a.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if result.State != notifications.DeliveryStateDelivered {
		t.Errorf("expected delivered, got %s", result.State)
	}

	// Verify whatsapp: prefix was added.
	if !strings.Contains(capturedBody, "whatsapp") {
		t.Errorf("expected whatsapp prefix in To, got: %s", capturedBody)
	}
}

func TestTwilioWhatsAppWithTemplate(t *testing.T) {
	var capturedContentSid string
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		capturedContentSid = r.PostForm.Get("ContentSid")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"sid":"WA_tmpl_111"}`))
	})

	msg := notifications.Message{
		ID:           "wa-tmpl-001",
		Type:         notifications.MessageTypeWhatsApp,
		TenantID:     "tenant-1",
		To:           []notifications.Recipient{{Address: "whatsapp:+14155551234"}},
		Body:         "fallback",
		TemplateID:   "HX1234567890",
		TemplateData: map[string]string{"1": "Front Door"},
	}

	_, err := a.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if capturedContentSid != "HX1234567890" {
		t.Errorf("expected ContentSid HX1234567890, got %s", capturedContentSid)
	}
}

func TestTwilioBatchSendSMS(t *testing.T) {
	callCount := 0
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"sid":"SM_batch_` + string(rune('0'+callCount)) + `"}`))
	})

	msg := notifications.Message{
		ID:       "batch-sms-001",
		Type:     notifications.MessageTypeSMS,
		TenantID: "tenant-1",
		To: []notifications.Recipient{
			{Address: "+14155551111"},
			{Address: "+14155552222"},
			{Address: "+14155553333"},
		},
		Body: "Batch alert",
	}

	results, err := a.BatchSend(context.Background(), msg)
	if err != nil {
		t.Fatalf("batch send: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if callCount != 3 {
		t.Errorf("expected 3 API calls (one per recipient), got %d", callCount)
	}
}

func TestTwilioRetryableError(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"message":"service unavailable"}`))
	})

	msg := notifications.Message{
		ID:       "retry-sms-001",
		Type:     notifications.MessageTypeSMS,
		TenantID: "tenant-1",
		To:       []notifications.Recipient{{Address: "+14155551234"}},
		Body:     "test",
	}

	_, err := a.BatchSend(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error on 503")
	}
	if !strings.Contains(err.Error(), "retryable") {
		t.Errorf("expected retryable in error, got: %s", err.Error())
	}
}

func TestTwilioPermanentError(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message":"invalid number"}`))
	})

	msg := notifications.Message{
		ID:       "perm-sms-001",
		Type:     notifications.MessageTypeSMS,
		TenantID: "tenant-1",
		To:       []notifications.Recipient{{Address: "bad-number"}},
		Body:     "test",
	}

	results, err := a.BatchSend(context.Background(), msg)
	// Permanent failure for SMS returns the result (not an error to retry).
	if err != nil {
		t.Fatalf("expected no error for permanent failure, got: %v", err)
	}
	if len(results) != 1 || results[0].State != notifications.DeliveryStateFailed {
		t.Error("expected failed state on permanent error")
	}
}

func TestTwilioCheckHealthSuccess(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/Accounts/") && strings.HasSuffix(r.URL.Path, ".json") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"active"}`))
		}
	})

	hs, err := a.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	if !hs.Healthy {
		t.Error("expected healthy=true")
	}
	if hs.Latency <= 0 {
		t.Error("expected positive latency")
	}
}

func TestTwilioCheckHealthUnhealthy(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	hs, err := a.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	if hs.Healthy {
		t.Error("expected healthy=false")
	}
}

func TestTwilioUnsupportedType(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {})
	msg := notifications.Message{
		ID:       "email-001",
		Type:     notifications.MessageTypeEmail,
		TenantID: "tenant-1",
		To:       []notifications.Recipient{{Address: "user@example.com"}},
		Body:     "test",
	}
	_, err := a.Send(context.Background(), msg)
	if err == nil {
		t.Error("expected error for unsupported email type on twilio")
	}
}

func TestTwilioVoiceXMLEscape(t *testing.T) {
	var capturedTwiml string
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		capturedTwiml = r.PostForm.Get("Twiml")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"sid":"CA_esc"}`))
	})

	msg := notifications.Message{
		ID:       "voice-esc-001",
		Type:     notifications.MessageTypeVoice,
		TenantID: "tenant-1",
		To:       []notifications.Recipient{{Address: "+14155551234"}},
		Body:     `Alert: temp > 90 & humidity < 20`,
	}

	_, err := a.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	// Verify XML special chars are escaped.
	if strings.Contains(capturedTwiml, "&h") || strings.Contains(capturedTwiml, "< 20") {
		// The raw < and & should be escaped.
		if strings.Contains(capturedTwiml, "& ") && !strings.Contains(capturedTwiml, "&amp;") {
			t.Error("expected & to be escaped to &amp;")
		}
	}
	if !strings.Contains(capturedTwiml, "&amp;") {
		t.Error("expected &amp; in escaped TwiML")
	}
	if !strings.Contains(capturedTwiml, "&lt;") {
		t.Error("expected &lt; in escaped TwiML")
	}
}
