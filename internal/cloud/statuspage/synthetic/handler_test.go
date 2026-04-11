package synthetic_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/statuspage/synthetic"
)

func TestHealthHandler_AllOK(t *testing.T) {
	probes := map[string]synthetic.Prober{
		"api":       func() (string, time.Duration) { return "ok", 5 * time.Millisecond },
		"identity":  func() (string, time.Duration) { return "ok", 3 * time.Millisecond },
	}

	handler := synthetic.HealthHandler(probes)
	req := httptest.NewRequest(http.MethodGet, "/status/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp synthetic.HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected ok, got %s", resp.Status)
	}
	if len(resp.Components) != 2 {
		t.Errorf("expected 2 components, got %d", len(resp.Components))
	}
}

func TestHealthHandler_Degraded(t *testing.T) {
	probes := map[string]synthetic.Prober{
		"api":      func() (string, time.Duration) { return "ok", time.Millisecond },
		"identity": func() (string, time.Duration) { return "degraded", time.Millisecond },
	}

	handler := synthetic.HealthHandler(probes)
	req := httptest.NewRequest(http.MethodGet, "/status/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 even for degraded, got %d", rec.Code)
	}

	var resp synthetic.HealthResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Status != "degraded" {
		t.Errorf("expected degraded, got %s", resp.Status)
	}
}

func TestHealthHandler_Down(t *testing.T) {
	probes := map[string]synthetic.Prober{
		"api":      func() (string, time.Duration) { return "ok", time.Millisecond },
		"identity": func() (string, time.Duration) { return "down", time.Millisecond },
	}

	handler := synthetic.HealthHandler(probes)
	req := httptest.NewRequest(http.MethodGet, "/status/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}

	var resp synthetic.HealthResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Status != "down" {
		t.Errorf("expected down, got %s", resp.Status)
	}
}

func TestHealthHandler_MethodNotAllowed(t *testing.T) {
	handler := synthetic.HealthHandler(nil)
	req := httptest.NewRequest(http.MethodPost, "/status/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestHealthHandler_NoProbes(t *testing.T) {
	handler := synthetic.HealthHandler(map[string]synthetic.Prober{})
	req := httptest.NewRequest(http.MethodGet, "/status/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var resp synthetic.HealthResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Status != "ok" {
		t.Errorf("expected ok for no probes, got %s", resp.Status)
	}
}

func TestHTTPProber(t *testing.T) {
	// Test with a healthy server.
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthy.Close()

	prober := synthetic.HTTPProber(healthy.URL, 5*time.Second)
	status, latency := prober()
	if status != "ok" {
		t.Errorf("expected ok, got %s", status)
	}
	if latency <= 0 {
		t.Errorf("expected positive latency")
	}

	// Test with a server returning 500.
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer errServer.Close()

	prober = synthetic.HTTPProber(errServer.URL, 5*time.Second)
	status, _ = prober()
	if status != "down" {
		t.Errorf("expected down for 500, got %s", status)
	}

	// Test with unreachable server.
	prober = synthetic.HTTPProber("http://127.0.0.1:1", 100*time.Millisecond)
	status, _ = prober()
	if status != "down" {
		t.Errorf("expected down for unreachable, got %s", status)
	}
}

func TestDefaultChecks(t *testing.T) {
	urls := synthetic.ComponentURLs{
		CloudAPI:         "https://api.kaivue.io",
		Identity:         "https://identity.kaivue.io",
		Directory:        "https://directory.kaivue.io",
		IntegratorPortal: "https://portal.kaivue.io",
		AIInference:      "https://ai.kaivue.io",
		RecordingArchive: "https://archive.kaivue.io",
		Notifications:    "https://notify.kaivue.io",
		CloudRelay:       "https://relay.kaivue.io",
		MarketingSite:    "https://www.kaivue.io",
		Docs:             "https://docs.kaivue.io",
	}

	checks := synthetic.DefaultChecks(urls)
	if len(checks) != 10 {
		t.Errorf("expected 10 checks, got %d", len(checks))
	}

	// Verify first check.
	if checks[0].Name != "Cloud Control Plane" {
		t.Errorf("expected Cloud Control Plane, got %s", checks[0].Name)
	}
	if checks[0].URL != "https://api.kaivue.io/healthz" {
		t.Errorf("unexpected URL: %s", checks[0].URL)
	}
}
