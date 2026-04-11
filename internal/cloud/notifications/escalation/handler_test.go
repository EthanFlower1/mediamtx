package escalation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications/escalation"
)

func claimsExtractor(tenantID, userID string) escalation.ClaimsExtractor {
	return func(r *http.Request) (string, string, bool) {
		return tenantID, userID, true
	}
}

func setupHandler(t *testing.T) (*escalation.Handler, *escalation.Service, *testClock) {
	t.Helper()
	notifier := &fakeNotifier{}
	pd := &fakePagerDuty{}
	svc, clock := newService(t, notifier, pd)
	h := escalation.NewHandler(svc, claimsExtractor("tenant-1", "user-1"))
	return h, svc, clock
}

func TestHandlerCreateAndListChains(t *testing.T) {
	h, _, _ := setupHandler(t)

	// Create chain.
	body := `{"name":"Test Chain","description":"test","enabled":true,"steps":[{"target_type":"user","target_id":"u1","channel_type":"push","timeout_seconds":300}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/escalation/chains", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.Dispatch(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created map[string]any
	json.NewDecoder(w.Body).Decode(&created)
	if created["chain_id"] == "" {
		t.Error("expected chain_id in response")
	}

	// List chains.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/escalation/chains", nil)
	w = httptest.NewRecorder()
	h.Dispatch(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var listed map[string]any
	json.NewDecoder(w.Body).Decode(&listed)
	chains := listed["chains"].([]any)
	if len(chains) != 1 {
		t.Errorf("expected 1 chain, got %d", len(chains))
	}
}

func TestHandlerGetChain(t *testing.T) {
	h, svc, _ := setupHandler(t)
	ctx := context.Background()

	chain, _, _ := svc.CreateChain(ctx, escalation.Chain{
		TenantID: "tenant-1",
		Name:     "Get Test",
		Enabled:  true,
	}, []escalation.Step{
		{TargetType: escalation.TargetUser, TargetID: "u1", ChannelType: escalation.ChannelPush, TimeoutSeconds: 300},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/escalation/chains/"+chain.ChainID, nil)
	w := httptest.NewRecorder()
	h.Dispatch(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	steps := resp["steps"].([]any)
	if len(steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(steps))
	}
}

func TestHandlerDeleteChain(t *testing.T) {
	h, svc, _ := setupHandler(t)
	ctx := context.Background()

	chain, _, _ := svc.CreateChain(ctx, escalation.Chain{
		TenantID: "tenant-1",
		Name:     "Delete Test",
		Enabled:  true,
	}, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/escalation/chains/"+chain.ChainID, nil)
	w := httptest.NewRecorder()
	h.Dispatch(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}

	// Delete again -> 404.
	w = httptest.NewRecorder()
	h.Dispatch(w, httptest.NewRequest(http.MethodDelete, "/api/v1/escalation/chains/"+chain.ChainID, nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandlerAckAlert(t *testing.T) {
	h, svc, _ := setupHandler(t)
	ctx := context.Background()

	chain, _, _ := svc.CreateChain(ctx, escalation.Chain{
		TenantID: "tenant-1",
		Name:     "Ack Handler",
		Enabled:  true,
	}, []escalation.Step{
		{TargetType: escalation.TargetUser, TargetID: "u1", ChannelType: escalation.ChannelPush, TimeoutSeconds: 300},
	})

	svc.StartEscalation(ctx, "tenant-1", "alert-h1", chain.ChainID)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/alert-h1/ack", nil)
	w := httptest.NewRecorder()
	h.Dispatch(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["state"] != "acknowledged" {
		t.Errorf("expected acknowledged state, got %v", resp["state"])
	}

	// Double-ack -> 409.
	w = httptest.NewRecorder()
	h.Dispatch(w, httptest.NewRequest(http.MethodPost, "/api/v1/alerts/alert-h1/ack", nil))
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestHandlerGetAlertEscalation(t *testing.T) {
	h, svc, _ := setupHandler(t)
	ctx := context.Background()

	chain, _, _ := svc.CreateChain(ctx, escalation.Chain{
		TenantID: "tenant-1",
		Name:     "Get Alert",
		Enabled:  true,
	}, []escalation.Step{
		{TargetType: escalation.TargetUser, TargetID: "u1", ChannelType: escalation.ChannelPush, TimeoutSeconds: 300},
	})

	svc.StartEscalation(ctx, "tenant-1", "alert-get", chain.ChainID)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/alert-get/escalation", nil)
	w := httptest.NewRecorder()
	h.Dispatch(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["state"] != "notified" {
		t.Errorf("expected notified, got %v", resp["state"])
	}
}

func TestHandlerAlertNotFound(t *testing.T) {
	h, _, _ := setupHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/nonexistent/ack", nil)
	w := httptest.NewRecorder()
	h.Dispatch(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandlerNotFound(t *testing.T) {
	h, _, _ := setupHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
	w := httptest.NewRecorder()
	h.Dispatch(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
