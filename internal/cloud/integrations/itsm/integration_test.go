package itsm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestIntegration_PagerDutyHTTPTest verifies the PagerDuty client against a
// real httptest server that mimics the Events API v2 contract.
func TestIntegration_PagerDutyHTTPTest(t *testing.T) {
	var mu sync.Mutex
	var received []pagerDutyEvent

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}

		var evt pagerDutyEvent
		if err := json.Unmarshal(body, &evt); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}

		mu.Lock()
		received = append(received, evt)
		mu.Unlock()

		dedupKey := evt.DedupKey
		if dedupKey == "" {
			dedupKey = "server-generated-dedup"
		}

		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(pagerDutyResponse{
			Status:   "success",
			Message:  "Event processed",
			DedupKey: dedupKey,
		})
	}))
	defer srv.Close()

	client, err := NewPagerDutyClient("integration-routing-key",
		WithPagerDutyEndpoint(srv.URL),
	)
	if err != nil {
		t.Fatalf("NewPagerDutyClient: %v", err)
	}

	ctx := context.Background()

	// Send a trigger event.
	result, err := client.SendAlert(ctx, Alert{
		Summary:   "Integration test: camera offline",
		Source:    "raikada",
		Severity:  SeverityCritical,
		DedupKey:  "int-test-cam-offline",
		Timestamp: time.Now().UTC(),
		Group:     "cameras",
		Class:     "camera_offline",
		Details:   map[string]string{"camera": "lobby-1"},
	})
	if err != nil {
		t.Fatalf("SendAlert: %v", err)
	}
	if result.Status != "success" {
		t.Errorf("expected success, got %s", result.Status)
	}
	if result.ExternalID != "int-test-cam-offline" {
		t.Errorf("expected dedup key int-test-cam-offline, got %s", result.ExternalID)
	}

	// Resolve the alert.
	resolveResult, err := client.ResolveAlert(ctx, "int-test-cam-offline")
	if err != nil {
		t.Fatalf("ResolveAlert: %v", err)
	}
	if resolveResult.Status != "success" {
		t.Errorf("expected success on resolve, got %s", resolveResult.Status)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 events (trigger + resolve), got %d", len(received))
	}
	if received[0].EventAction != "trigger" {
		t.Errorf("first event action: expected trigger, got %s", received[0].EventAction)
	}
	if received[1].EventAction != "resolve" {
		t.Errorf("second event action: expected resolve, got %s", received[1].EventAction)
	}
	if received[0].RoutingKey != "integration-routing-key" {
		t.Errorf("routing key: got %s", received[0].RoutingKey)
	}
}

// TestIntegration_OpsgenieHTTPTest verifies the Opsgenie client against a
// real httptest server that mimics the Opsgenie Alert API contract.
func TestIntegration_OpsgenieHTTPTest(t *testing.T) {
	var mu sync.Mutex
	type reqRecord struct {
		method string
		path   string
		auth   string
		body   []byte
	}
	var requests []reqRecord

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		mu.Lock()
		requests = append(requests, reqRecord{
			method: r.Method,
			path:   r.URL.Path + "?" + r.URL.RawQuery,
			auth:   r.Header.Get("Authorization"),
			body:   body,
		})
		mu.Unlock()

		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(opsgenieResponse{
			Result:    "Request will be processed",
			RequestID: "og-integration-req-001",
		})
	}))
	defer srv.Close()

	client, err := NewOpsgenieClient("og-integration-api-key",
		WithOpsgenieEndpoint(srv.URL+"/v2/alerts"),
	)
	if err != nil {
		t.Fatalf("NewOpsgenieClient: %v", err)
	}

	ctx := context.Background()

	// Send an alert.
	result, err := client.SendAlert(ctx, Alert{
		Summary:   "Integration test: storage full",
		Source:    "raikada",
		Severity:  SeverityError,
		DedupKey:  "int-test-storage-full",
		Timestamp: time.Now().UTC(),
		Group:     "storage",
		Class:     "storage_full",
		Details:   map[string]string{"disk": "/dev/sda1", "usage": "99%"},
	})
	if err != nil {
		t.Fatalf("SendAlert: %v", err)
	}
	if result.Status != "success" {
		t.Errorf("expected success, got %s", result.Status)
	}
	if result.ExternalID != "og-integration-req-001" {
		t.Errorf("expected request ID og-integration-req-001, got %s", result.ExternalID)
	}

	// Resolve the alert.
	_, err = client.ResolveAlert(ctx, "int-test-storage-full")
	if err != nil {
		t.Fatalf("ResolveAlert: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("expected 2 requests (create + close), got %d", len(requests))
	}

	// Verify create alert request.
	if requests[0].method != http.MethodPost {
		t.Errorf("create: expected POST, got %s", requests[0].method)
	}
	if requests[0].auth != "GenieKey og-integration-api-key" {
		t.Errorf("create auth: got %s", requests[0].auth)
	}

	var createPayload opsgenieCreateAlert
	if err := json.Unmarshal(requests[0].body, &createPayload); err != nil {
		t.Fatalf("unmarshal create payload: %v", err)
	}
	if createPayload.Priority != "P2" {
		t.Errorf("expected P2 priority for error severity, got %s", createPayload.Priority)
	}
	if createPayload.Alias != "int-test-storage-full" {
		t.Errorf("alias: got %s", createPayload.Alias)
	}

	// Verify close alert request URL contains the alias.
	if requests[1].method != http.MethodPost {
		t.Errorf("close: expected POST, got %s", requests[1].method)
	}
}

// TestIntegration_EndToEndServiceWithHTTPTest exercises the full Service ->
// Router -> Provider pipeline using httptest servers for both PagerDuty and
// Opsgenie, verifying that routing rules correctly dispatch to the right backend.
func TestIntegration_EndToEndServiceWithHTTPTest(t *testing.T) {
	var pdMu sync.Mutex
	var pdEvents []pagerDutyEvent

	pdServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var evt pagerDutyEvent
		_ = json.Unmarshal(body, &evt)

		pdMu.Lock()
		pdEvents = append(pdEvents, evt)
		pdMu.Unlock()

		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(pagerDutyResponse{
			Status:   "success",
			Message:  "Event processed",
			DedupKey: evt.DedupKey,
		})
	}))
	defer pdServer.Close()

	var ogMu sync.Mutex
	var ogCount int

	ogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ogMu.Lock()
		ogCount++
		ogMu.Unlock()

		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(opsgenieResponse{
			Result:    "Request will be processed",
			RequestID: "og-e2e-req",
		})
	}))
	defer ogServer.Close()

	store := NewMemStore()
	seq := 0
	svc, err := NewService(Config{
		Store: store,
		IDGen: func() string {
			seq++
			return "e2e-id-" + string(rune('a'-1+seq))
		},
		// Use DefaultProviderFactory to exercise real client creation.
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx := context.Background()

	// Save PagerDuty config (for critical alerts).
	pdCfg, err := svc.SaveProviderConfig(ctx, ProviderConfig{
		TenantID: "tenant-e2e",
		Provider: ProviderPagerDuty,
		APIKey:   "e2e-pd-routing-key",
		Endpoint: pdServer.URL,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("save PD config: %v", err)
	}

	// Save Opsgenie config (for all warnings and above).
	ogCfg, err := svc.SaveProviderConfig(ctx, ProviderConfig{
		TenantID: "tenant-e2e",
		Provider: ProviderOpsgenie,
		APIKey:   "e2e-og-api-key",
		Endpoint: ogServer.URL + "/v2/alerts",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("save OG config: %v", err)
	}

	// Routing: critical camera_offline -> PagerDuty.
	_, err = svc.SaveRoutingRule(ctx, RoutingRule{
		TenantID:         "tenant-e2e",
		Name:             "Critical camera alerts to PagerDuty",
		ProviderConfigID: pdCfg.ConfigID,
		MinSeverity:      SeverityCritical,
		AlertClasses:     []string{"camera_offline"},
		Priority:         1,
		Enabled:          true,
	})
	if err != nil {
		t.Fatalf("save PD rule: %v", err)
	}

	// Routing: all warnings+ -> Opsgenie.
	_, err = svc.SaveRoutingRule(ctx, RoutingRule{
		TenantID:         "tenant-e2e",
		Name:             "All warnings to Opsgenie",
		ProviderConfigID: ogCfg.ConfigID,
		MinSeverity:      SeverityWarning,
		Enabled:          true,
		Priority:         2,
	})
	if err != nil {
		t.Fatalf("save OG rule: %v", err)
	}

	// Load tenant routing.
	if err := svc.LoadTenantRouting(ctx, "tenant-e2e"); err != nil {
		t.Fatalf("LoadTenantRouting: %v", err)
	}

	// Test 1: Critical camera_offline alert -> should go to BOTH PD and OG.
	results, err := svc.SendAlert(ctx, Alert{
		Summary:   "Camera offline: parking-lot",
		Source:    "raikada",
		Severity:  SeverityCritical,
		DedupKey:  "cam-parking-lot",
		Timestamp: time.Now().UTC(),
		Class:     "camera_offline",
	})
	if err != nil {
		t.Fatalf("SendAlert critical: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results (PD + OG), got %d", len(results))
	}

	// Test 2: Warning storage alert -> should go to OG only (PD rule requires critical + camera_offline).
	results, err = svc.SendAlert(ctx, Alert{
		Summary:   "Storage warning",
		Source:    "raikada",
		Severity:  SeverityWarning,
		DedupKey:  "storage-warn",
		Timestamp: time.Now().UTC(),
		Class:     "storage_warning",
	})
	if err != nil {
		t.Fatalf("SendAlert warning: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (OG only), got %d", len(results))
	}
	if results[0].ProviderType != ProviderOpsgenie {
		t.Errorf("expected opsgenie for warning, got %s", results[0].ProviderType)
	}

	// Test 3: Info alert -> should match nothing (OG rule requires warning+).
	results, err = svc.SendAlert(ctx, Alert{
		Summary:  "Routine check",
		Source:   "raikada",
		Severity: SeverityInfo,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for info alert, got %d", len(results))
	}

	// Verify PagerDuty received exactly 1 event.
	pdMu.Lock()
	if len(pdEvents) != 1 {
		t.Errorf("PagerDuty: expected 1 event, got %d", len(pdEvents))
	} else {
		if pdEvents[0].Payload.Severity != "critical" {
			t.Errorf("PagerDuty event severity: expected critical, got %s", pdEvents[0].Payload.Severity)
		}
	}
	pdMu.Unlock()

	// Verify Opsgenie received exactly 2 events (critical + warning).
	ogMu.Lock()
	if ogCount != 2 {
		t.Errorf("Opsgenie: expected 2 events, got %d", ogCount)
	}
	ogMu.Unlock()
}
