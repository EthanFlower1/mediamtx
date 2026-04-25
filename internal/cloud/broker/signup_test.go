package broker

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSignupSuccess(t *testing.T) {
	store := newTestStore(t)
	handler := SignupHandler(store)

	body := `{"company_name":"Acme Corp","email":"admin@acme.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/signup", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp SignupResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.TenantID == "" {
		t.Error("expected non-empty tenant_id")
	}
	if resp.APIKey == "" {
		t.Error("expected non-empty api_key")
	}
	if !strings.HasPrefix(resp.APIKey, "kvue_") {
		t.Errorf("expected kvue_ prefix, got %s", resp.APIKey)
	}
	if len(resp.APIKey) != 45 { // "kvue_" (5) + 40 hex chars
		t.Errorf("expected 45-char API key, got %d chars", len(resp.APIKey))
	}
	if resp.Message == "" {
		t.Error("expected non-empty message")
	}

	// Verify tenant was persisted via store methods.
	tenant, err := store.GetTenant(resp.TenantID)
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if tenant.Name != "Acme Corp" {
		t.Errorf("expected tenant name 'Acme Corp', got %q", tenant.Name)
	}

	// Verify API key was persisted.
	keys, err := store.ListAPIKeys(resp.TenantID)
	if err != nil {
		t.Fatalf("list api keys: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("expected 1 api_key row, got %d", len(keys))
	}
}

func TestSignupDuplicateEmail(t *testing.T) {
	store := newTestStore(t)
	handler := SignupHandler(store)

	body := `{"company_name":"Acme Corp","email":"dup@acme.com"}`

	// First signup should succeed.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/signup", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("first signup: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Second signup with same email should fail with 409.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/signup", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("second signup: expected 409, got %d: %s", rec.Code, rec.Body.String())
	}

	var errResp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp["error"] == "" {
		t.Error("expected non-empty error message")
	}
}

func TestSignupMissingFields(t *testing.T) {
	store := newTestStore(t)
	handler := SignupHandler(store)

	cases := []struct {
		name string
		body string
	}{
		{"empty company_name", `{"company_name":"","email":"test@example.com"}`},
		{"empty email", `{"company_name":"Test Co","email":""}`},
		{"both empty", `{"company_name":"","email":""}`},
		{"missing fields", `{}`},
		{"whitespace only", `{"company_name":"  ","email":"  "}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/signup", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}
