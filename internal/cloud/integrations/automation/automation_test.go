package automation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Shared trigger/action definitions
// ---------------------------------------------------------------------------

func TestSharedTriggers(t *testing.T) {
	triggers := SharedTriggers()
	if len(triggers) != 2 {
		t.Fatalf("expected 2 shared triggers, got %d", len(triggers))
	}
	keys := map[string]bool{}
	for _, tr := range triggers {
		keys[tr.Key] = true
	}
	for _, want := range []string{"camera_event", "alert"} {
		if !keys[want] {
			t.Errorf("missing trigger key %q", want)
		}
	}
}

func TestSharedActions(t *testing.T) {
	actions := SharedActions()
	if len(actions) != 2 {
		t.Fatalf("expected 2 shared actions, got %d", len(actions))
	}
	keys := map[string]bool{}
	for _, a := range actions {
		keys[a.Key] = true
	}
	for _, want := range []string{"create_clip", "send_notification"} {
		if !keys[want] {
			t.Errorf("missing action key %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Subscription store
// ---------------------------------------------------------------------------

func TestSubscriptionStore(t *testing.T) {
	s := NewSubscriptionStore()

	sub := &Subscription{
		ID:         "sub-1",
		Platform:   PlatformZapier,
		TriggerKey: "camera_event",
		WebhookURL: "https://hooks.zapier.com/1234",
		Active:     true,
	}
	s.Add(sub)

	if got := s.Get("sub-1"); got == nil {
		t.Fatal("expected subscription, got nil")
	}

	if got := s.ByTrigger("camera_event"); len(got) != 1 {
		t.Fatalf("expected 1 trigger sub, got %d", len(got))
	}

	if got := s.ByPlatform(PlatformZapier); len(got) != 1 {
		t.Fatalf("expected 1 zapier sub, got %d", len(got))
	}

	if got := s.All(); len(got) != 1 {
		t.Fatalf("expected 1 total sub, got %d", len(got))
	}

	if ok := s.Remove("sub-1"); !ok {
		t.Fatal("expected Remove to return true")
	}
	if ok := s.Remove("sub-1"); ok {
		t.Fatal("expected Remove to return false for missing sub")
	}
}

// ---------------------------------------------------------------------------
// Webhook handler: subscribe / unsubscribe
// ---------------------------------------------------------------------------

func TestHandleSubscribe(t *testing.T) {
	h := NewWebhookHandler()

	body := `{"platform":"zapier","trigger_key":"camera_event","webhook_url":"https://hooks.zapier.com/x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/automation/webhooks/subscribe", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleSubscribe(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var sr SubscribeResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		t.Fatal(err)
	}
	if sr.ID == "" {
		t.Fatal("expected non-empty subscription ID")
	}

	// Verify stored
	if s := h.Store.Get(sr.ID); s == nil {
		t.Fatal("subscription not found in store")
	}
}

func TestHandleSubscribe_Validation(t *testing.T) {
	h := NewWebhookHandler()

	body := `{"platform":"zapier"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/automation/webhooks/subscribe", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleSubscribe(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Result().StatusCode)
	}
}

func TestHandleSubscribe_WrongMethod(t *testing.T) {
	h := NewWebhookHandler()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.HandleSubscribe(w, req)
	if w.Result().StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Result().StatusCode)
	}
}

func TestHandleUnsubscribe(t *testing.T) {
	h := NewWebhookHandler()
	h.Store.Add(&Subscription{ID: "sub-99", Platform: PlatformMake, TriggerKey: "alert", WebhookURL: "https://example.com", Active: true})

	body := `{"id":"sub-99"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/automation/webhooks/unsubscribe", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleUnsubscribe(w, req)

	if w.Result().StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Result().StatusCode)
	}

	// Unknown ID
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"id":"nope"}`))
	w = httptest.NewRecorder()
	h.HandleUnsubscribe(w, req)
	if w.Result().StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Result().StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Sample data
// ---------------------------------------------------------------------------

func TestHandleSample(t *testing.T) {
	h := NewWebhookHandler()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sample/camera_event", nil)
	h.HandleSample(w, req, "camera_event")

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Result().StatusCode)
	}

	var data []json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if len(data) != 1 {
		t.Fatalf("expected 1 sample item, got %d", len(data))
	}
}

func TestHandleSample_Unknown(t *testing.T) {
	h := NewWebhookHandler()
	w := httptest.NewRecorder()
	h.HandleSample(w, httptest.NewRequest(http.MethodGet, "/", nil), "nope")
	if w.Result().StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Result().StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Action handling
// ---------------------------------------------------------------------------

func TestHandleAction(t *testing.T) {
	h := NewWebhookHandler()
	h.ActionHandler = func(_ context.Context, req ActionRequest) (*ActionResponse, error) {
		return &ActionResponse{Success: true, ID: "clip-42", Message: "created"}, nil
	}

	body := `{"camera_id":"cam-1","start_time":"2026-01-15T10:00:00Z","end_time":"2026-01-15T10:05:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/actions/create_clip", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleAction(w, req, "create_clip")

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Result().StatusCode)
	}

	var resp ActionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Success || resp.ID != "clip-42" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestHandleAction_NoHandler(t *testing.T) {
	h := NewWebhookHandler()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.HandleAction(w, req, "create_clip")
	if w.Result().StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", w.Result().StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Dispatch
// ---------------------------------------------------------------------------

func TestDispatch(t *testing.T) {
	var received atomic.Int32

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		body, _ := io.ReadAll(r.Body)
		var p TriggerPayload
		if err := json.Unmarshal(body, &p); err != nil {
			t.Errorf("unmarshal: %v", err)
		}
		if p.TriggerType != TriggerCameraEvent {
			t.Errorf("expected trigger_type camera_event, got %s", p.TriggerType)
		}
		// Check HMAC header present for subscriptions with a secret
		if sig := r.Header.Get("X-Hub-Signature-256"); sig == "" {
			t.Error("expected X-Hub-Signature-256 header")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	h := NewWebhookHandler()
	h.HTTPClient = target.Client()

	// Add two subscribers
	h.Store.Add(&Subscription{ID: "s1", Platform: PlatformZapier, TriggerKey: "camera_event", WebhookURL: target.URL, Secret: "sec1", Active: true})
	h.Store.Add(&Subscription{ID: "s2", Platform: PlatformN8N, TriggerKey: "camera_event", WebhookURL: target.URL, Secret: "sec2", Active: true})

	payload := TriggerPayload{
		TriggerType: TriggerCameraEvent,
		CameraID:    "cam-1",
		EventType:   "motion_detected",
		Timestamp:   time.Now().UTC(),
	}
	h.Dispatch(context.Background(), "camera_event", payload)

	if got := received.Load(); got != 2 {
		t.Fatalf("expected 2 deliveries, got %d", got)
	}
}

func TestDispatch_NoSubscribers(t *testing.T) {
	h := NewWebhookHandler()
	// Should not panic
	h.Dispatch(context.Background(), "camera_event", TriggerPayload{})
}

// ---------------------------------------------------------------------------
// Platform descriptors
// ---------------------------------------------------------------------------

func TestDefaultZapierApp(t *testing.T) {
	app := DefaultZapierApp("https://nvr.example.com")
	if app.Name != "Raikada" {
		t.Errorf("unexpected name %q", app.Name)
	}
	if app.Auth.Type != "oauth2" {
		t.Errorf("expected oauth2 auth, got %q", app.Auth.Type)
	}
	if len(app.Triggers) != 2 {
		t.Errorf("expected 2 triggers, got %d", len(app.Triggers))
	}
	if len(app.Actions) != 2 {
		t.Errorf("expected 2 actions, got %d", len(app.Actions))
	}
	// Check URLs contain the base
	if !strings.Contains(app.Triggers[0].SubscribeURL, "nvr.example.com") {
		t.Error("trigger subscribe URL missing base")
	}
}

func TestDefaultMakeApp(t *testing.T) {
	app := DefaultMakeApp("https://nvr.example.com")
	if app.Label != "Raikada" {
		t.Errorf("unexpected label %q", app.Label)
	}
	if app.Connection.Type != "oauth2" {
		t.Errorf("expected oauth2 connection, got %q", app.Connection.Type)
	}
	if len(app.Webhooks) != 2 {
		t.Errorf("expected 2 webhooks, got %d", len(app.Webhooks))
	}
	// 2 triggers + 2 actions = 4 modules
	if len(app.Modules) != 4 {
		t.Errorf("expected 4 modules, got %d", len(app.Modules))
	}
}

func TestDefaultN8NNode(t *testing.T) {
	node := DefaultN8NNode("https://nvr.example.com")
	if node.DisplayName != "Raikada" {
		t.Errorf("unexpected display name %q", node.DisplayName)
	}
	if len(node.Credentials) != 1 {
		t.Errorf("expected 1 credential block, got %d", len(node.Credentials))
	}
	if node.Credentials[0].Type != "oAuth2Api" {
		t.Errorf("expected oAuth2Api, got %q", node.Credentials[0].Type)
	}
	if len(node.Triggers) != 2 {
		t.Errorf("expected 2 triggers, got %d", len(node.Triggers))
	}
	if len(node.Actions) != 2 {
		t.Errorf("expected 2 actions, got %d", len(node.Actions))
	}
}

// ---------------------------------------------------------------------------
// RegisterRoutes integration test
// ---------------------------------------------------------------------------

func TestRegisterRoutes_Integration(t *testing.T) {
	h := NewWebhookHandler()
	h.ActionHandler = func(_ context.Context, req ActionRequest) (*ActionResponse, error) {
		return &ActionResponse{Success: true, ID: "test-id"}, nil
	}

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Subscribe
	subResp, err := http.Post(
		srv.URL+"/api/v1/integrations/automation/webhooks/subscribe",
		"application/json",
		strings.NewReader(`{"platform":"n8n","trigger_key":"alert","webhook_url":"https://n8n.example.com/webhook/abc"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if subResp.StatusCode != http.StatusCreated {
		t.Fatalf("subscribe: expected 201, got %d", subResp.StatusCode)
	}

	var sr SubscribeResponse
	_ = json.NewDecoder(subResp.Body).Decode(&sr)
	subResp.Body.Close()

	// Sample
	sampleResp, err := http.Get(srv.URL + "/api/v1/integrations/automation/webhooks/sample/camera_event")
	if err != nil {
		t.Fatal(err)
	}
	if sampleResp.StatusCode != http.StatusOK {
		t.Fatalf("sample: expected 200, got %d", sampleResp.StatusCode)
	}
	sampleResp.Body.Close()

	// Action
	actionResp, err := http.Post(
		srv.URL+"/api/v1/integrations/automation/actions/create_clip",
		"application/json",
		strings.NewReader(`{"camera_id":"cam-1"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if actionResp.StatusCode != http.StatusOK {
		t.Fatalf("action: expected 200, got %d", actionResp.StatusCode)
	}
	actionResp.Body.Close()

	// Platform descriptors
	for _, platform := range []string{"zapier", "make", "n8n"} {
		resp, err := http.Get(srv.URL + "/api/v1/integrations/automation/platforms/" + platform)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("platform %s: expected 200, got %d", platform, resp.StatusCode)
		}
		resp.Body.Close()
	}

	// Unsubscribe
	unsubResp, err := http.Post(
		srv.URL+"/api/v1/integrations/automation/webhooks/unsubscribe",
		"application/json",
		strings.NewReader(`{"id":"`+sr.ID+`"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if unsubResp.StatusCode != http.StatusNoContent {
		t.Fatalf("unsubscribe: expected 204, got %d", unsubResp.StatusCode)
	}
	unsubResp.Body.Close()
}
