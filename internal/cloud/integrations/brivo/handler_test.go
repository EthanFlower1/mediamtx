package brivo

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestHandler builds a Handler backed by in-memory stores for httptest.
func newTestHandler(t *testing.T) (*Handler, *Service, *mockAPIClient) {
	t.Helper()
	api := &mockAPIClient{}
	svc, err := NewService(Config{
		OAuth: OAuthConfig{
			ClientID:      "client-id",
			ClientSecret:  "client-secret",
			RedirectURL:   "https://example.com/callback",
			WebhookSecret: "wh-secret",
		},
		Tokens:    NewMemoryTokenStore(),
		Conns:     NewMemoryConnectionStore(),
		Mappings:  NewMemoryMappingStore(),
		Events:    NewMemoryEventLog(),
		Snapshots: &mockSnapshotService{},
		API:       api,
		Clock:     fixedClock,
	})
	if err != nil {
		t.Fatal(err)
	}

	h := NewHandler(HandlerConfig{
		Service: svc,
		Tenant:  func(r *http.Request) string { return r.Header.Get("X-Tenant-ID") },
	})
	return h, svc, api
}

func doReq(h http.Handler, method, path string, body interface{}, tenant string) *httptest.ResponseRecorder {
	var reqBody *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = &bytes.Buffer{}
	}
	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	if tenant != "" {
		req.Header.Set("X-Tenant-ID", tenant)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func decodeJSON(t *testing.T, rr *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(rr.Body).Decode(v); err != nil {
		t.Fatalf("decode JSON: %v (body: %s)", err, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Handler tests
// ---------------------------------------------------------------------------

func TestHandler_AuthBegin(t *testing.T) {
	h, _, _ := newTestHandler(t)
	rr := doReq(h, "GET", "/integrations/brivo/auth", nil, "tenant-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	decodeJSON(t, rr, &resp)
	if !strings.HasPrefix(resp["authorize_url"], "https://auth.brivo.com/oauth/authorize?") {
		t.Errorf("unexpected auth URL: %s", resp["authorize_url"])
	}
}

func TestHandler_AuthBegin_MissingTenant(t *testing.T) {
	h, _, _ := newTestHandler(t)
	rr := doReq(h, "GET", "/integrations/brivo/auth", nil, "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHandler_TestConnection_OK(t *testing.T) {
	h, svc, _ := newTestHandler(t)
	ctx := context.Background()
	_ = svc.oauth.tokens.StoreToken(ctx, "tenant-1", TokenPair{
		AccessToken: "tok", RefreshToken: "ref", ExpiresAt: time.Now().Add(time.Hour),
	})

	rr := doReq(h, "POST", "/integrations/brivo/test", nil, "tenant-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	decodeJSON(t, rr, &resp)
	if resp["status"] != "ok" {
		t.Errorf("expected ok, got %s", resp["status"])
	}
}

func TestHandler_TestConnection_Fail(t *testing.T) {
	h, svc, api := newTestHandler(t)
	api.listSitesFn = func(_ context.Context, _ string) ([]BrivoSite, error) {
		return nil, fmt.Errorf("api error")
	}
	ctx := context.Background()
	_ = svc.oauth.tokens.StoreToken(ctx, "tenant-1", TokenPair{
		AccessToken: "tok", RefreshToken: "ref", ExpiresAt: time.Now().Add(time.Hour),
	})

	rr := doReq(h, "POST", "/integrations/brivo/test", nil, "tenant-1")
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestHandler_GetConnection_NotFound(t *testing.T) {
	h, _, _ := newTestHandler(t)
	rr := doReq(h, "GET", "/integrations/brivo/connection", nil, "tenant-1")
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestHandler_Disconnect(t *testing.T) {
	h, svc, _ := newTestHandler(t)
	ctx := context.Background()

	// Pre-populate.
	_ = svc.oauth.tokens.StoreToken(ctx, "tenant-1", TokenPair{
		AccessToken: "tok", RefreshToken: "ref", ExpiresAt: time.Now().Add(time.Hour),
	})
	_ = svc.conns.Upsert(ctx, Connection{ID: "c1", TenantID: "tenant-1", Status: ConnStatusActive})

	rr := doReq(h, "DELETE", "/integrations/brivo/connection", nil, "tenant-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_ListSites(t *testing.T) {
	h, svc, _ := newTestHandler(t)
	ctx := context.Background()
	_ = svc.oauth.tokens.StoreToken(ctx, "tenant-1", TokenPair{
		AccessToken: "tok", RefreshToken: "ref", ExpiresAt: time.Now().Add(time.Hour),
	})

	rr := doReq(h, "GET", "/integrations/brivo/sites", nil, "tenant-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var sites []BrivoSite
	decodeJSON(t, rr, &sites)
	if len(sites) != 1 {
		t.Errorf("expected 1 site, got %d", len(sites))
	}
}

func TestHandler_ListDoors(t *testing.T) {
	h, svc, _ := newTestHandler(t)
	ctx := context.Background()
	_ = svc.oauth.tokens.StoreToken(ctx, "tenant-1", TokenPair{
		AccessToken: "tok", RefreshToken: "ref", ExpiresAt: time.Now().Add(time.Hour),
	})

	rr := doReq(h, "GET", "/integrations/brivo/doors?site_id=site-1", nil, "tenant-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var doors []BrivoDoor
	decodeJSON(t, rr, &doors)
	if len(doors) != 1 {
		t.Errorf("expected 1 door, got %d", len(doors))
	}
}

func TestHandler_Mappings_CRUD(t *testing.T) {
	h, _, _ := newTestHandler(t)

	// Create mapping.
	mapping := DoorCameraMapping{
		ID:          "map-1",
		BrivoDoorID: "door-1",
		DoorName:    "Main",
		CameraID:    "cam-1",
		CameraName:  "Lobby",
	}
	rr := doReq(h, "POST", "/integrations/brivo/mappings", mapping, "tenant-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("POST mapping: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// List mappings.
	rr = doReq(h, "GET", "/integrations/brivo/mappings", nil, "tenant-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET mappings: expected 200, got %d", rr.Code)
	}
	var mappings []DoorCameraMapping
	decodeJSON(t, rr, &mappings)
	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(mappings))
	}
	if mappings[0].TenantID != "tenant-1" {
		t.Errorf("expected tenant-1, got %s", mappings[0].TenantID)
	}

	// Delete mapping.
	rr = doReq(h, "DELETE", "/integrations/brivo/mappings/map-1", nil, "tenant-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE mapping: expected 200, got %d", rr.Code)
	}

	// Verify empty.
	rr = doReq(h, "GET", "/integrations/brivo/mappings", nil, "tenant-1")
	decodeJSON(t, rr, &mappings)
	if len(mappings) != 0 {
		t.Errorf("expected 0 mappings after delete, got %d", len(mappings))
	}
}

func TestHandler_Webhook(t *testing.T) {
	h, svc, _ := newTestHandler(t)
	ctx := context.Background()

	// Set up a mapping for event correlation.
	_ = svc.SetDoorCameraMapping(ctx, DoorCameraMapping{
		ID: "m1", TenantID: "tenant-1", BrivoDoorID: "door-1", CameraID: "cam-1",
	})

	event := DoorEvent{
		EventID:     "evt-1",
		TenantID:    "tenant-1",
		BrivoSiteID: "site-1",
		BrivoDoorID: "door-1",
		EventType:   DoorEventUnlock,
		OccurredAt:  fixedTime,
	}
	body, _ := json.Marshal(event)

	// Compute HMAC signature.
	mac := hmac.New(sha256.New, []byte("wh-secret"))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/integrations/brivo/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Brivo-Signature", sig)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result DoorEvent
	decodeJSON(t, rr, &result)
	if result.EventID != "evt-1" {
		t.Errorf("expected evt-1, got %s", result.EventID)
	}
}

func TestHandler_Webhook_InvalidSignature(t *testing.T) {
	h, _, _ := newTestHandler(t)

	body := []byte(`{"event_id":"x"}`)
	req := httptest.NewRequest("POST", "/integrations/brivo/webhook", bytes.NewReader(body))
	req.Header.Set("X-Brivo-Signature", "bad-sig")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHandler_ListEvents_Empty(t *testing.T) {
	h, _, _ := newTestHandler(t)
	rr := doReq(h, "GET", "/integrations/brivo/events", nil, "tenant-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var events []DoorEvent
	decodeJSON(t, rr, &events)
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}
