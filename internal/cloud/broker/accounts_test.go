package broker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAccountInfoEndpoint(t *testing.T) {
	store := newTestStore(t)

	tenantID, err := store.CreateTenant("Acme Corp", "admin@acme.com")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	apiKey, err := store.CreateAPIKey(tenantID, "default")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	handler := AccountHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/account", nil)
	req.Header.Set("X-API-Key", apiKey)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var tenant Tenant
	if err := json.NewDecoder(rec.Body).Decode(&tenant); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if tenant.ID != tenantID {
		t.Errorf("expected tenant_id %q, got %q", tenantID, tenant.ID)
	}
	if tenant.Name != "Acme Corp" {
		t.Errorf("expected name 'Acme Corp', got %q", tenant.Name)
	}
	if tenant.Email != "admin@acme.com" {
		t.Errorf("expected email 'admin@acme.com', got %q", tenant.Email)
	}
}

func TestAccountInfoBearerToken(t *testing.T) {
	store := newTestStore(t)

	tenantID, err := store.CreateTenant("Bearer Co", "bearer@example.com")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	apiKey, err := store.CreateAPIKey(tenantID, "default")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	handler := AccountHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/account", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var tenant Tenant
	if err := json.NewDecoder(rec.Body).Decode(&tenant); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if tenant.ID != tenantID {
		t.Errorf("expected tenant_id %q, got %q", tenantID, tenant.ID)
	}
}

func TestAccountInfoUnauthorized(t *testing.T) {
	store := newTestStore(t)
	handler := AccountHandler(store)

	// No API key at all.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/account", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}

	var errResp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp["error"] == "" {
		t.Error("expected non-empty error message")
	}

	// Invalid API key.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/account", nil)
	req.Header.Set("X-API-Key", "kvue_0000000000000000000000000000000000000000")
	rec = httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid key, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListKeysEndpoint(t *testing.T) {
	store := newTestStore(t)

	tenantID, err := store.CreateTenant("Keys Co", "keys@example.com")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	apiKey, err := store.CreateAPIKey(tenantID, "key-alpha")
	if err != nil {
		t.Fatalf("create api key 1: %v", err)
	}
	_, err = store.CreateAPIKey(tenantID, "key-beta")
	if err != nil {
		t.Fatalf("create api key 2: %v", err)
	}

	handler := ListKeysHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/account/keys", nil)
	req.Header.Set("X-API-Key", apiKey)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Keys []APIKeyInfo `json:"keys"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(resp.Keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(resp.Keys))
	}

	if resp.Keys[0].Name != "key-alpha" {
		t.Errorf("expected first key name 'key-alpha', got %q", resp.Keys[0].Name)
	}
	if resp.Keys[1].Name != "key-beta" {
		t.Errorf("expected second key name 'key-beta', got %q", resp.Keys[1].Name)
	}

	// Verify no secrets are exposed — only prefix (10 chars).
	for _, k := range resp.Keys {
		if len(k.Prefix) != 10 {
			t.Errorf("expected prefix length 10, got %d", len(k.Prefix))
		}
		if k.TenantID != tenantID {
			t.Errorf("expected tenant_id %q, got %q", tenantID, k.TenantID)
		}
	}
}
