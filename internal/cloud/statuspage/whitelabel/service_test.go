package whitelabel_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/statuspage"
	"github.com/bluenviron/mediamtx/internal/cloud/statuspage/whitelabel"
)

var seqID int

func testIDGen() string {
	seqID++
	return fmt.Sprintf("test-%04d", seqID)
}

func openTestDB(t *testing.T) *clouddb.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := "sqlite://" + filepath.Join(dir, "cloud.db")
	d, err := clouddb.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func newTestService(t *testing.T) (*whitelabel.Service, *statuspage.Service) {
	t.Helper()
	db := openTestDB(t)
	spSvc, err := statuspage.NewService(statuspage.Config{
		DB:    db,
		IDGen: testIDGen,
	})
	if err != nil {
		t.Fatalf("new statuspage service: %v", err)
	}
	svc, err := whitelabel.NewService(whitelabel.Config{
		DB:            db,
		StatusPageSvc: spSvc,
		IDGen:         testIDGen,
	})
	if err != nil {
		t.Fatalf("new whitelabel service: %v", err)
	}
	return svc, spSvc
}

func TestUpsertAndGetConfig(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	cfg := whitelabel.StatusPageConfig{
		IntegratorID:   "integrator-1",
		Subdomain:      "acmealarm",
		PageTitle:      "Acme Alarm Status",
		PrimaryColor:   "#0066FF",
		SecondaryColor: "#FFFFFF",
		AccentColor:    "#333333",
		HeaderBgColor:  "#FFFFFF",
		LogoURL:        "https://cdn.example.com/acme-logo.png",
		FooterText:     "Powered by Acme Alarm",
		ComponentIDs:   []string{"check-1", "check-2"},
		SupportURL:     "https://support.acmealarm.com",
		Enabled:        true,
	}

	saved, err := svc.UpsertConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if saved.IntegratorID != "integrator-1" {
		t.Errorf("expected integrator-1, got %s", saved.IntegratorID)
	}

	got, err := svc.GetConfig(ctx, "integrator-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Subdomain != "acmealarm" {
		t.Errorf("expected acmealarm, got %s", got.Subdomain)
	}
	if got.PageTitle != "Acme Alarm Status" {
		t.Errorf("expected Acme Alarm Status, got %s", got.PageTitle)
	}
	if len(got.ComponentIDs) != 2 {
		t.Errorf("expected 2 component IDs, got %d", len(got.ComponentIDs))
	}
}

func TestGetConfigBySubdomain(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	svc.UpsertConfig(ctx, whitelabel.StatusPageConfig{
		IntegratorID:   "integrator-1",
		Subdomain:      "acmealarm",
		PrimaryColor:   "#0066FF",
		SecondaryColor: "#FFF",
		AccentColor:    "#333",
		HeaderBgColor:  "#FFF",
		ComponentIDs:   []string{},
		Enabled:        true,
	})

	got, err := svc.GetConfigBySubdomain(ctx, "acmealarm")
	if err != nil {
		t.Fatalf("get by subdomain: %v", err)
	}
	if got.IntegratorID != "integrator-1" {
		t.Errorf("expected integrator-1, got %s", got.IntegratorID)
	}
}

func TestSubdomainUniqueness(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	base := whitelabel.StatusPageConfig{
		Subdomain:      "shared",
		PrimaryColor:   "#000",
		SecondaryColor: "#FFF",
		AccentColor:    "#333",
		HeaderBgColor:  "#FFF",
		ComponentIDs:   []string{},
		Enabled:        true,
	}

	cfg1 := base
	cfg1.IntegratorID = "integrator-1"
	if _, err := svc.UpsertConfig(ctx, cfg1); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}

	cfg2 := base
	cfg2.IntegratorID = "integrator-2"
	_, err := svc.UpsertConfig(ctx, cfg2)
	if err != whitelabel.ErrSubdomainTaken {
		t.Errorf("expected ErrSubdomainTaken, got %v", err)
	}
}

func TestInvalidSubdomain(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	cases := []string{"", "UPPER", "has space", "-start", "end-"}
	for _, sub := range cases {
		_, err := svc.UpsertConfig(ctx, whitelabel.StatusPageConfig{
			IntegratorID:   "int-1",
			Subdomain:      sub,
			PrimaryColor:   "#000",
			SecondaryColor: "#FFF",
			AccentColor:    "#333",
			HeaderBgColor:  "#FFF",
			ComponentIDs:   []string{},
			Enabled:        true,
		})
		if err == nil {
			t.Errorf("expected error for subdomain %q", sub)
		}
	}
}

func TestDeleteConfig(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	svc.UpsertConfig(ctx, whitelabel.StatusPageConfig{
		IntegratorID:   "integrator-1",
		Subdomain:      "acme",
		PrimaryColor:   "#000",
		SecondaryColor: "#FFF",
		AccentColor:    "#333",
		HeaderBgColor:  "#FFF",
		ComponentIDs:   []string{},
		Enabled:        true,
	})

	if err := svc.DeleteConfig(ctx, "integrator-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	err := svc.DeleteConfig(ctx, "integrator-1")
	if err != whitelabel.ErrConfigNotFound {
		t.Errorf("expected ErrConfigNotFound, got %v", err)
	}
}

func TestRenderPublicPage(t *testing.T) {
	svc, spSvc := newTestService(t)
	ctx := context.Background()

	// Create health checks under integrator-1 as the tenant.
	hc1, _ := spSvc.UpsertHealthCheck(ctx, statuspage.HealthCheck{
		TenantID:    "integrator-1",
		ServiceName: "cloud_api",
		DisplayName: "Cloud API",
		Status:      statuspage.StatusOperational,
		Metadata:    "{}",
		Enabled:     true,
	})
	hc2, _ := spSvc.UpsertHealthCheck(ctx, statuspage.HealthCheck{
		TenantID:    "integrator-1",
		ServiceName: "recording",
		DisplayName: "Recording",
		Status:      statuspage.StatusDegraded,
		Metadata:    "{}",
		Enabled:     true,
	})
	spSvc.UpsertHealthCheck(ctx, statuspage.HealthCheck{
		TenantID:    "integrator-1",
		ServiceName: "internal_only",
		DisplayName: "Internal Only",
		Status:      statuspage.StatusOperational,
		Metadata:    "{}",
		Enabled:     true,
	})

	// Create an incident.
	spSvc.CreateIncident(ctx, statuspage.Incident{
		TenantID:         "integrator-1",
		Title:            "Recording degraded",
		Severity:         statuspage.SeverityMinor,
		Status:           statuspage.IncidentInvestigating,
		AffectedServices: `["recording"]`,
	})

	// Config with component filter: only show cloud_api and recording.
	svc.UpsertConfig(ctx, whitelabel.StatusPageConfig{
		IntegratorID:   "integrator-1",
		Subdomain:      "acmealarm",
		PageTitle:      "Acme Alarm Status",
		PrimaryColor:   "#0066FF",
		SecondaryColor: "#FFFFFF",
		AccentColor:    "#333333",
		HeaderBgColor:  "#FFFFFF",
		ComponentIDs:   []string{hc1.CheckID, hc2.CheckID},
		Enabled:        true,
	})

	page, err := svc.RenderPublicPage(ctx, "integrator-1")
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if page.OverallStatus != "degraded" {
		t.Errorf("expected degraded, got %s", page.OverallStatus)
	}
	if len(page.Components) != 2 {
		t.Errorf("expected 2 filtered components, got %d", len(page.Components))
	}
	if len(page.ActiveIncidents) != 1 {
		t.Errorf("expected 1 active incident, got %d", len(page.ActiveIncidents))
	}
	if page.Config.PageTitle != "Acme Alarm Status" {
		t.Errorf("expected branded title, got %s", page.Config.PageTitle)
	}
}

func TestRenderPublicPageNoFilter(t *testing.T) {
	svc, spSvc := newTestService(t)
	ctx := context.Background()

	spSvc.UpsertHealthCheck(ctx, statuspage.HealthCheck{
		TenantID:    "integrator-1",
		ServiceName: "cloud_api",
		DisplayName: "Cloud API",
		Status:      statuspage.StatusOperational,
		Metadata:    "{}",
		Enabled:     true,
	})
	spSvc.UpsertHealthCheck(ctx, statuspage.HealthCheck{
		TenantID:    "integrator-1",
		ServiceName: "live_view",
		DisplayName: "Live View",
		Status:      statuspage.StatusOperational,
		Metadata:    "{}",
		Enabled:     true,
	})

	// Empty component_ids means show all.
	svc.UpsertConfig(ctx, whitelabel.StatusPageConfig{
		IntegratorID:   "integrator-1",
		Subdomain:      "acme",
		PrimaryColor:   "#000",
		SecondaryColor: "#FFF",
		AccentColor:    "#333",
		HeaderBgColor:  "#FFF",
		ComponentIDs:   []string{},
		Enabled:        true,
	})

	page, err := svc.RenderPublicPage(ctx, "integrator-1")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if len(page.Components) != 2 {
		t.Errorf("expected 2 components (all), got %d", len(page.Components))
	}
}

func TestRenderDisabledPage(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	svc.UpsertConfig(ctx, whitelabel.StatusPageConfig{
		IntegratorID:   "integrator-1",
		Subdomain:      "disabled",
		PrimaryColor:   "#000",
		SecondaryColor: "#FFF",
		AccentColor:    "#333",
		HeaderBgColor:  "#FFF",
		ComponentIDs:   []string{},
		Enabled:        false,
	})

	_, err := svc.RenderPublicPage(ctx, "integrator-1")
	if err != whitelabel.ErrPageDisabled {
		t.Errorf("expected ErrPageDisabled, got %v", err)
	}
}

func TestSubscriberLifecycle(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Subscribe.
	sub, err := svc.Subscribe(ctx, "integrator-1", "alice@example.com")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if sub.Email != "alice@example.com" {
		t.Errorf("expected alice@example.com, got %s", sub.Email)
	}
	if sub.Confirmed {
		t.Error("expected unconfirmed")
	}
	if sub.ConfirmToken == "" {
		t.Error("expected non-empty confirm token")
	}

	// Not yet in confirmed list.
	confirmed, _ := svc.ListConfirmedSubscribers(ctx, "integrator-1")
	if len(confirmed) != 0 {
		t.Errorf("expected 0 confirmed, got %d", len(confirmed))
	}

	// Confirm.
	if err := svc.ConfirmSubscriber(ctx, "integrator-1", sub.ConfirmToken); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// Now in confirmed list.
	confirmed, _ = svc.ListConfirmedSubscribers(ctx, "integrator-1")
	if len(confirmed) != 1 {
		t.Fatalf("expected 1 confirmed, got %d", len(confirmed))
	}
	if confirmed[0].Email != "alice@example.com" {
		t.Errorf("expected alice@example.com, got %s", confirmed[0].Email)
	}

	// Duplicate subscribe fails.
	_, err = svc.Subscribe(ctx, "integrator-1", "alice@example.com")
	if err != whitelabel.ErrSubscriberExists {
		t.Errorf("expected ErrSubscriberExists, got %v", err)
	}

	// Unsubscribe.
	if err := svc.Unsubscribe(ctx, "integrator-1", "alice@example.com"); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}

	// Confirm list empty again.
	confirmed, _ = svc.ListConfirmedSubscribers(ctx, "integrator-1")
	if len(confirmed) != 0 {
		t.Errorf("expected 0 after unsubscribe, got %d", len(confirmed))
	}
}

func TestSubscriberInvalidEmail(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.Subscribe(ctx, "int-1", "not-an-email")
	if err != whitelabel.ErrInvalidEmail {
		t.Errorf("expected ErrInvalidEmail, got %v", err)
	}
}

func TestConfirmBadToken(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.ConfirmSubscriber(ctx, "int-1", "bogus-token")
	if err != whitelabel.ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestCrossIntegratorIsolation(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	svc.UpsertConfig(ctx, whitelabel.StatusPageConfig{
		IntegratorID: "int-1", Subdomain: "alpha",
		PrimaryColor: "#000", SecondaryColor: "#FFF",
		AccentColor: "#333", HeaderBgColor: "#FFF",
		ComponentIDs: []string{}, Enabled: true,
	})
	svc.UpsertConfig(ctx, whitelabel.StatusPageConfig{
		IntegratorID: "int-2", Subdomain: "beta",
		PrimaryColor: "#000", SecondaryColor: "#FFF",
		AccentColor: "#333", HeaderBgColor: "#FFF",
		ComponentIDs: []string{}, Enabled: true,
	})

	// Subscribers are isolated.
	svc.Subscribe(ctx, "int-1", "user@example.com")
	svc.Subscribe(ctx, "int-2", "other@example.com")

	// Confirm both to test isolation in list.
	s1, _ := svc.Subscribe(ctx, "int-1", "confirmed@example.com")
	svc.ConfirmSubscriber(ctx, "int-1", s1.ConfirmToken)

	list1, _ := svc.ListConfirmedSubscribers(ctx, "int-1")
	list2, _ := svc.ListConfirmedSubscribers(ctx, "int-2")

	if len(list1) != 1 {
		t.Errorf("int-1 expected 1 confirmed, got %d", len(list1))
	}
	if len(list2) != 0 {
		t.Errorf("int-2 expected 0 confirmed, got %d", len(list2))
	}
}

func TestGetConfigByCustomDomain(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	svc.UpsertConfig(ctx, whitelabel.StatusPageConfig{
		IntegratorID:   "integrator-1",
		Subdomain:      "acme",
		CustomDomain:   "status.acmealarm.com",
		PrimaryColor:   "#000",
		SecondaryColor: "#FFF",
		AccentColor:    "#333",
		HeaderBgColor:  "#FFF",
		ComponentIDs:   []string{},
		Enabled:        true,
	})

	got, err := svc.GetConfigByCustomDomain(ctx, "status.acmealarm.com")
	if err != nil {
		t.Fatalf("get by custom domain: %v", err)
	}
	if got.IntegratorID != "integrator-1" {
		t.Errorf("expected integrator-1, got %s", got.IntegratorID)
	}
}

// --- HTTP Handler Tests ---

func TestHandlerAdminRoundTrip(t *testing.T) {
	svc, spSvc := newTestService(t)
	handler := whitelabel.NewHandler(svc)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Create some health checks first.
	ctx := context.Background()
	spSvc.UpsertHealthCheck(ctx, statuspage.HealthCheck{
		TenantID: "int-1", ServiceName: "api", DisplayName: "API",
		Status: statuspage.StatusOperational, Metadata: "{}", Enabled: true,
	})

	// PUT config.
	cfg := whitelabel.StatusPageConfig{
		Subdomain:      "acme",
		PageTitle:      "Acme Status",
		PrimaryColor:   "#0066FF",
		SecondaryColor: "#FFFFFF",
		AccentColor:    "#333333",
		HeaderBgColor:  "#FFFFFF",
		ComponentIDs:   []string{},
		Enabled:        true,
	}
	body, _ := json.Marshal(cfg)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/integrators/int-1/status-page", bytes.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("put status=%d body=%s", resp.StatusCode, string(b))
	}
	resp.Body.Close()

	// GET config.
	resp2, err := http.Get(srv.URL + "/api/v1/integrators/int-1/status-page")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("get status=%d", resp2.StatusCode)
	}
	var gotCfg whitelabel.StatusPageConfig
	json.NewDecoder(resp2.Body).Decode(&gotCfg)
	resp2.Body.Close()
	if gotCfg.Subdomain != "acme" {
		t.Errorf("expected acme, got %s", gotCfg.Subdomain)
	}

	// GET render.
	resp3, err := http.Get(srv.URL + "/api/v1/integrators/int-1/status-page/render")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if resp3.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp3.Body)
		t.Fatalf("render status=%d body=%s", resp3.StatusCode, string(b))
	}
	var page whitelabel.PublicStatusPage
	json.NewDecoder(resp3.Body).Decode(&page)
	resp3.Body.Close()
	if page.OverallStatus != "operational" {
		t.Errorf("expected operational, got %s", page.OverallStatus)
	}

	// Subscribe.
	subBody, _ := json.Marshal(map[string]string{"email": "bob@example.com"})
	resp4, err := http.Post(srv.URL+"/api/v1/integrators/int-1/status-page/subscribers",
		"application/json", bytes.NewReader(subBody))
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if resp4.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp4.Body)
		t.Fatalf("subscribe status=%d body=%s", resp4.StatusCode, string(b))
	}
	var sub whitelabel.Subscriber
	json.NewDecoder(resp4.Body).Decode(&sub)
	resp4.Body.Close()

	// Confirm.
	confirmBody, _ := json.Marshal(map[string]string{"token": sub.ConfirmToken})
	resp5, err := http.Post(srv.URL+"/api/v1/integrators/int-1/status-page/subscribers/confirm",
		"application/json", bytes.NewReader(confirmBody))
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if resp5.StatusCode != http.StatusOK {
		t.Errorf("confirm status=%d", resp5.StatusCode)
	}
	resp5.Body.Close()

	// List subscribers.
	resp6, err := http.Get(srv.URL + "/api/v1/integrators/int-1/status-page/subscribers")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var subs []whitelabel.Subscriber
	json.NewDecoder(resp6.Body).Decode(&subs)
	resp6.Body.Close()
	if len(subs) != 1 || subs[0].Email != "bob@example.com" {
		t.Errorf("expected 1 confirmed subscriber, got %+v", subs)
	}

	// DELETE config.
	delReq, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/integrators/int-1/status-page", nil)
	resp7, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if resp7.StatusCode != http.StatusNoContent {
		t.Errorf("delete status=%d", resp7.StatusCode)
	}
	resp7.Body.Close()

	// GET after delete should 404.
	resp8, _ := http.Get(srv.URL + "/api/v1/integrators/int-1/status-page")
	if resp8.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", resp8.StatusCode)
	}
	resp8.Body.Close()
}

func TestSubdomainMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub := whitelabel.SubdomainFrom(r.Context())
		w.Write([]byte(sub))
	})

	mw := whitelabel.SubdomainMiddleware("status.example.com")
	handler := mw(inner)

	// Valid subdomain.
	req := httptest.NewRequest("GET", "/status", nil)
	req.Host = "acme.status.example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	if rec.Body.String() != "acme" {
		t.Errorf("expected acme, got %s", rec.Body.String())
	}

	// No subdomain.
	req2 := httptest.NewRequest("GET", "/status", nil)
	req2.Host = "status.example.com"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Errorf("expected 404 for bare domain, got %d", rec2.Code)
	}

	// Wrong domain.
	req3 := httptest.NewRequest("GET", "/status", nil)
	req3.Host = "acme.other.com"
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusNotFound {
		t.Errorf("expected 404 for wrong domain, got %d", rec3.Code)
	}

	// Host with port.
	req4 := httptest.NewRequest("GET", "/status", nil)
	req4.Host = "acme.status.example.com:8443"
	rec4 := httptest.NewRecorder()
	handler.ServeHTTP(rec4, req4)
	if rec4.Code != 200 {
		t.Fatalf("status=%d (with port)", rec4.Code)
	}
	if rec4.Body.String() != "acme" {
		t.Errorf("expected acme with port, got %s", rec4.Body.String())
	}
}

func TestHandlerRoutesCoverage(t *testing.T) {
	svc, _ := newTestService(t)
	handler := whitelabel.NewHandler(svc)

	admin := handler.AdminRoutes()
	if len(admin) != 8 {
		t.Errorf("expected 8 admin routes, got %d", len(admin))
	}

	public := handler.PublicRoutes()
	if len(public) != 2 {
		t.Errorf("expected 2 public routes, got %d", len(public))
	}
}

func TestRenderHTML(t *testing.T) {
	svc, spSvc := newTestService(t)
	ctx := context.Background()

	// Create components under integrator-1.
	spSvc.UpsertHealthCheck(ctx, statuspage.HealthCheck{
		TenantID: "integrator-1", ServiceName: "cloud_api", DisplayName: "Cloud API",
		Status: statuspage.StatusOperational, Metadata: "{}", Enabled: true,
	})
	spSvc.UpsertHealthCheck(ctx, statuspage.HealthCheck{
		TenantID: "integrator-1", ServiceName: "recording", DisplayName: "Recording",
		Status: statuspage.StatusDegraded, Metadata: "{}", Enabled: true,
	})

	spSvc.CreateIncident(ctx, statuspage.Incident{
		TenantID: "integrator-1", Title: "Recording slow",
		Severity: statuspage.SeverityMinor, Status: statuspage.IncidentInvestigating,
		AffectedServices: `["recording"]`,
	})

	svc.UpsertConfig(ctx, whitelabel.StatusPageConfig{
		IntegratorID:   "integrator-1",
		Subdomain:      "acme",
		PageTitle:      "Acme Alarm Status",
		PrimaryColor:   "#0066FF",
		SecondaryColor: "#FFFFFF",
		AccentColor:    "#333333",
		HeaderBgColor:  "#FFFFFF",
		LogoURL:        "https://cdn.example.com/logo.png",
		FooterText:     "Powered by Acme Alarm",
		SupportURL:     "https://support.acme.com",
		ComponentIDs:   []string{},
		Enabled:        true,
	})

	page, err := svc.RenderPublicPage(ctx, "integrator-1")
	if err != nil {
		t.Fatalf("render page: %v", err)
	}

	html, err := whitelabel.RenderHTML(page)
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	body := string(html)

	// Verify branding appears in rendered HTML.
	for _, needle := range []string{
		"Acme Alarm Status",
		"#0066FF",
		"https://cdn.example.com/logo.png",
		"Powered by Acme Alarm",
		"https://support.acme.com",
		"Cloud API",
		"Recording",
		"Degraded Performance",
		"Recording slow",
	} {
		if !strings.Contains(body, needle) {
			t.Errorf("HTML missing %q", needle)
		}
	}
}

func TestPublicStatusHTMLEndpoint(t *testing.T) {
	svc, spSvc := newTestService(t)
	handler := whitelabel.NewHandler(svc)

	ctx := context.Background()
	spSvc.UpsertHealthCheck(ctx, statuspage.HealthCheck{
		TenantID: "int-1", ServiceName: "api", DisplayName: "API",
		Status: statuspage.StatusOperational, Metadata: "{}", Enabled: true,
	})
	svc.UpsertConfig(ctx, whitelabel.StatusPageConfig{
		IntegratorID:   "int-1",
		Subdomain:      "acme",
		PageTitle:      "Acme Status",
		PrimaryColor:   "#000",
		SecondaryColor: "#FFF",
		AccentColor:    "#333",
		HeaderBgColor:  "#FFF",
		ComponentIDs:   []string{},
		Enabled:        true,
	})

	mw := whitelabel.SubdomainMiddleware("status.example.com")
	srv := httptest.NewServer(mw(handler))
	defer srv.Close()

	// The httptest server doesn't let us set Host directly, so we use the
	// combined middleware via a manual request.
	req := httptest.NewRequest("GET", "/status.html", nil)
	req.Host = "acme.status.example.com"
	rec := httptest.NewRecorder()
	mw(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status.html status=%d body=%s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("expected text/html content type, got %s", ct)
	}
	if !strings.Contains(rec.Body.String(), "Acme Status") {
		t.Error("HTML should contain page title")
	}
}

func TestCustomDomainMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dom := whitelabel.CustomDomainFrom(r.Context())
		w.Write([]byte(dom))
	})

	mw := whitelabel.CustomDomainMiddleware()
	handler := mw(inner)

	req := httptest.NewRequest("GET", "/status", nil)
	req.Host = "status.acmealarm.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	if rec.Body.String() != "status.acmealarm.com" {
		t.Errorf("expected status.acmealarm.com, got %s", rec.Body.String())
	}

	// With port.
	req2 := httptest.NewRequest("GET", "/status", nil)
	req2.Host = "status.acmealarm.com:8443"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Body.String() != "status.acmealarm.com" {
		t.Errorf("expected port stripped, got %s", rec2.Body.String())
	}
}

func TestCombinedDomainMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub := whitelabel.SubdomainFrom(r.Context())
		dom := whitelabel.CustomDomainFrom(r.Context())
		if sub != "" {
			w.Write([]byte("sub:" + sub))
		} else if dom != "" {
			w.Write([]byte("dom:" + dom))
		} else {
			w.Write([]byte("none"))
		}
	})

	mw := whitelabel.CombinedDomainMiddleware("status.example.com")
	handler := mw(inner)

	// Subdomain match.
	req1 := httptest.NewRequest("GET", "/status", nil)
	req1.Host = "acme.status.example.com"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Body.String() != "sub:acme" {
		t.Errorf("expected sub:acme, got %s", rec1.Body.String())
	}

	// Custom domain (no match to base).
	req2 := httptest.NewRequest("GET", "/status", nil)
	req2.Host = "status.acmealarm.com"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Body.String() != "dom:status.acmealarm.com" {
		t.Errorf("expected dom:status.acmealarm.com, got %s", rec2.Body.String())
	}
}

func TestRenderPublicPageByCustomDomain(t *testing.T) {
	svc, spSvc := newTestService(t)
	ctx := context.Background()

	spSvc.UpsertHealthCheck(ctx, statuspage.HealthCheck{
		TenantID: "int-1", ServiceName: "api", DisplayName: "API",
		Status: statuspage.StatusOperational, Metadata: "{}", Enabled: true,
	})

	svc.UpsertConfig(ctx, whitelabel.StatusPageConfig{
		IntegratorID:   "int-1",
		Subdomain:      "acme",
		CustomDomain:   "status.acmealarm.com",
		PageTitle:      "Acme Alarm Status",
		PrimaryColor:   "#000",
		SecondaryColor: "#FFF",
		AccentColor:    "#333",
		HeaderBgColor:  "#FFF",
		ComponentIDs:   []string{},
		Enabled:        true,
	})

	page, err := svc.RenderPublicPageByCustomDomain(ctx, "status.acmealarm.com")
	if err != nil {
		t.Fatalf("render by custom domain: %v", err)
	}
	if page.Config.PageTitle != "Acme Alarm Status" {
		t.Errorf("expected branded title, got %s", page.Config.PageTitle)
	}
	if page.OverallStatus != "operational" {
		t.Errorf("expected operational, got %s", page.OverallStatus)
	}
}

func TestPublicStatusHTMLViaCustomDomain(t *testing.T) {
	svc, spSvc := newTestService(t)
	handler := whitelabel.NewHandler(svc)

	ctx := context.Background()
	spSvc.UpsertHealthCheck(ctx, statuspage.HealthCheck{
		TenantID: "int-1", ServiceName: "api", DisplayName: "API",
		Status: statuspage.StatusOperational, Metadata: "{}", Enabled: true,
	})
	svc.UpsertConfig(ctx, whitelabel.StatusPageConfig{
		IntegratorID:   "int-1",
		Subdomain:      "acme",
		CustomDomain:   "status.acmealarm.com",
		PageTitle:      "Acme Alarm Status",
		PrimaryColor:   "#0066FF",
		SecondaryColor: "#FFF",
		AccentColor:    "#333",
		HeaderBgColor:  "#FFF",
		ComponentIDs:   []string{},
		Enabled:        true,
	})

	mw := whitelabel.CombinedDomainMiddleware("status.example.com")

	// Request arrives at status.acmealarm.com (custom domain).
	req := httptest.NewRequest("GET", "/status.html", nil)
	req.Host = "status.acmealarm.com"
	rec := httptest.NewRecorder()
	mw(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Acme Alarm Status") {
		t.Error("HTML should contain branded page title via custom domain")
	}
	if !strings.Contains(rec.Body.String(), "#0066FF") {
		t.Error("HTML should contain integrator primary color")
	}
}
