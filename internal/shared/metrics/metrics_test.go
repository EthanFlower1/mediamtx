package metrics_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/bluenviron/mediamtx/internal/shared/metrics"
)

// -----------------------------------------------------------------------
// Registry constructors
// -----------------------------------------------------------------------

func TestNew_ReturnsNonNil(t *testing.T) {
	reg := metrics.New()
	if reg == nil {
		t.Fatal("metrics.New() returned nil")
	}
}

func TestRegistry_Constructors(t *testing.T) {
	reg := metrics.New()

	if c := reg.NewCounter(prometheus.CounterOpts{Name: "c1", Help: "h"}); c == nil {
		t.Error("NewCounter returned nil")
	}
	if c := reg.NewCounterVec(prometheus.CounterOpts{Name: "cv1", Help: "h"}, []string{"l1"}); c == nil {
		t.Error("NewCounterVec returned nil")
	}
	if h := reg.NewHistogram(prometheus.HistogramOpts{Name: "h1", Help: "h"}); h == nil {
		t.Error("NewHistogram returned nil")
	}
	if h := reg.NewHistogramVec(prometheus.HistogramOpts{Name: "hv1", Help: "h"}, []string{"l1"}); h == nil {
		t.Error("NewHistogramVec returned nil")
	}
	if g := reg.NewGauge(prometheus.GaugeOpts{Name: "g1", Help: "h"}); g == nil {
		t.Error("NewGauge returned nil")
	}
	if g := reg.NewGaugeVec(prometheus.GaugeOpts{Name: "gv1", Help: "h"}, []string{"l1"}); g == nil {
		t.Error("NewGaugeVec returned nil")
	}
}

// -----------------------------------------------------------------------
// Standard metrics Init — idempotent
// -----------------------------------------------------------------------

func TestInit_Idempotent(t *testing.T) {
	reg := metrics.New()
	info := metrics.BuildInfo{Version: "v1.0.0", Commit: "abc123", GoVersion: "go1.22"}

	std1 := metrics.Init(reg, info)
	std2 := metrics.Init(reg, info) // second call must not panic

	if std1 == nil || std2 == nil {
		t.Fatal("Init returned nil")
	}
}

// -----------------------------------------------------------------------
// Build info gauge
// -----------------------------------------------------------------------

func TestBuildInfo_Labels(t *testing.T) {
	reg := metrics.New()
	info := metrics.BuildInfo{Version: "v0.1.0", Commit: "deadbeef", GoVersion: "go1.99"}
	metrics.Init(reg, info)

	body := scrape(t, reg)
	if !strings.Contains(body, `version="v0.1.0"`) {
		t.Errorf("build_info missing version label in:\n%s", body)
	}
	if !strings.Contains(body, `commit="deadbeef"`) {
		t.Errorf("build_info missing commit label in:\n%s", body)
	}
	if !strings.Contains(body, `kaivue_build_info`) {
		t.Errorf("build_info metric missing from scrape body:\n%s", body)
	}
}

// -----------------------------------------------------------------------
// /metrics HTTP handler returns 200
// -----------------------------------------------------------------------

func TestHTTPHandler_Returns200(t *testing.T) {
	reg := metrics.New()
	metrics.Init(reg, metrics.BuildInfo{Version: "v1"})

	srv := httptest.NewServer(reg.HTTPHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// -----------------------------------------------------------------------
// Cross-tenant isolation
// -----------------------------------------------------------------------

// TestCrossTenantIsolation verifies that incrementing a counter for
// tenant_id=A does not alter the counter value for tenant_id=B.
func TestCrossTenantIsolation(t *testing.T) {
	reg := metrics.New()
	cv := reg.NewCounterVec(
		prometheus.CounterOpts{Name: "tenant_test_events_total", Help: "h"},
		[]string{"tenant_id", "event"},
	)

	cv.WithLabelValues("tenant-A", "login").Inc()
	cv.WithLabelValues("tenant-A", "login").Inc()
	cv.WithLabelValues("tenant-B", "login").Inc()

	mfs, err := reg.Unwrap().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	var aCount, bCount float64
	for _, mf := range mfs {
		if mf.GetName() != "tenant_test_events_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			switch labelValue(m.GetLabel(), "tenant_id") {
			case "tenant-A":
				aCount = m.GetCounter().GetValue()
			case "tenant-B":
				bCount = m.GetCounter().GetValue()
			}
		}
	}

	if aCount != 2 {
		t.Errorf("tenant-A count = %v, want 2", aCount)
	}
	if bCount != 1 {
		t.Errorf("tenant-B count = %v, want 1", bCount)
	}
}

// -----------------------------------------------------------------------
// HTTP middleware increments standard metrics
// -----------------------------------------------------------------------

func TestHTTPMiddleware_IncrementsMetrics(t *testing.T) {
	reg := metrics.New()
	std := metrics.Init(reg, metrics.BuildInfo{Version: "v1"})

	mw := metrics.HTTPMiddleware(std, "test-component")

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/cameras", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	mfs, err := reg.Unwrap().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	found := false
	for _, mf := range mfs {
		if mf.GetName() != "kaivue_requests_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			if labelValue(m.GetLabel(), "component") == "test-component" {
				found = true
				if v := m.GetCounter().GetValue(); v != 1 {
					t.Errorf("kaivue_requests_total for test-component = %v, want 1", v)
				}
			}
		}
	}
	if !found {
		t.Error("kaivue_requests_total with component=test-component not found after request")
	}
}

// TestHTTPMiddleware_NilStandard verifies fail-open: nil Standard returns a
// transparent pass-through.
func TestHTTPMiddleware_NilStandard(t *testing.T) {
	mw := metrics.HTTPMiddleware(nil, "x")
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	mw(inner).ServeHTTP(rr, req)
	if rr.Code != http.StatusTeapot {
		t.Errorf("pass-through changed status: got %d", rr.Code)
	}
}

// -----------------------------------------------------------------------
// Component metric constructors
// -----------------------------------------------------------------------

func TestNewRecorderControlMetrics(t *testing.T) {
	reg := metrics.New()
	m := metrics.NewRecorderControlMetrics(reg)
	if m == nil || m.Connected == nil || m.EventsAppliedTotal == nil || m.ReconnectsTotal == nil {
		t.Fatal("NewRecorderControlMetrics returned incomplete struct")
	}
	m.Connected.WithLabelValues("rec-001").Set(1)
	m.EventsAppliedTotal.WithLabelValues("camera_added").Inc()
	m.ReconnectsTotal.WithLabelValues("stream_drop").Inc()
}

func TestNewDirectoryIngestMetrics(t *testing.T) {
	reg := metrics.New()
	m := metrics.NewDirectoryIngestMetrics(reg)
	if m == nil || m.MessagesTotal == nil || m.BackpressureDropsTotal == nil {
		t.Fatal("NewDirectoryIngestMetrics returned incomplete struct")
	}
	m.MessagesTotal.WithLabelValues("camera_state", "ok").Inc()
	m.BackpressureDropsTotal.Inc()
}

func TestNewStreamsMetrics(t *testing.T) {
	reg := metrics.New()
	m := metrics.NewStreamsMetrics(reg)
	if m == nil || m.MintedTotal == nil || m.TTLSeconds == nil {
		t.Fatal("NewStreamsMetrics returned incomplete struct")
	}
	m.MintedTotal.WithLabelValues("live", "hls", "ok").Inc()
	m.TTLSeconds.Observe(300)
}

func TestNewCertMgrMetrics(t *testing.T) {
	reg := metrics.New()
	m := metrics.NewCertMgrMetrics(reg)
	if m == nil || m.CertExpiresAt == nil || m.RenewalsTotal == nil || m.ReEnrollmentsTotal == nil {
		t.Fatal("NewCertMgrMetrics returned incomplete struct")
	}
	m.CertExpiresAt.Set(1712000000)
	m.RenewalsTotal.WithLabelValues("ok").Inc()
	m.ReEnrollmentsTotal.WithLabelValues("error").Inc()
}

func TestNewRelationshipsMetrics(t *testing.T) {
	reg := metrics.New()
	m := metrics.NewRelationshipsMetrics(reg)
	if m == nil || m.GrantedTotal == nil || m.RevokedTotal == nil {
		t.Fatal("NewRelationshipsMetrics returned incomplete struct")
	}
	m.GrantedTotal.Inc()
	m.RevokedTotal.Inc()
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

func scrape(t *testing.T, reg *metrics.Registry) string {
	t.Helper()
	srv := httptest.NewServer(reg.HTTPHandler())
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func labelValue(labels []*dto.LabelPair, name string) string {
	for _, lp := range labels {
		if lp.GetName() == name {
			return lp.GetValue()
		}
	}
	return ""
}
