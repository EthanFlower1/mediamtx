package broker

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testPassword = "correct horse battery staple"

func TestSignupSuccess(t *testing.T) {
	store := newTestStore(t)
	handler := SignupHandler(store)

	body := `{"company_name":"Acme Corp","email":"admin@acme.com","password":"` + testPassword + `","name":"Admin"}`
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
	if resp.UserID == "" {
		t.Error("expected non-empty user_id")
	}
	if resp.APIKey == "" {
		t.Error("expected non-empty api_key")
	}
	if !strings.HasPrefix(resp.APIKey, "kvue_") {
		t.Errorf("expected kvue_ prefix, got %s", resp.APIKey)
	}
	if len(resp.APIKey) != 45 {
		t.Errorf("expected 45-char API key, got %d chars", len(resp.APIKey))
	}

	tenant, err := store.GetTenant(resp.TenantID)
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if tenant.Name != "Acme Corp" {
		t.Errorf("expected tenant name 'Acme Corp', got %q", tenant.Name)
	}

	// Verify the user was created with a working password.
	user, err := store.VerifyPassword("admin@acme.com", testPassword)
	if err != nil {
		t.Fatalf("verify password: %v", err)
	}
	if user.ID != resp.UserID {
		t.Errorf("user ID mismatch: %s vs %s", user.ID, resp.UserID)
	}
	if user.TenantID != resp.TenantID {
		t.Errorf("user tenant mismatch: %s vs %s", user.TenantID, resp.TenantID)
	}
}

func TestSignupDuplicateEmail(t *testing.T) {
	store := newTestStore(t)
	handler := SignupHandler(store)

	body := `{"company_name":"Acme Corp","email":"dup@acme.com","password":"` + testPassword + `"}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/signup", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first signup: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/signup", bytes.NewBufferString(body))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second signup: expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSignupMissingFields(t *testing.T) {
	store := newTestStore(t)
	handler := SignupHandler(store)

	cases := []struct {
		name string
		body string
	}{
		{"empty company_name", `{"company_name":"","email":"test@example.com","password":"` + testPassword + `"}`},
		{"empty email", `{"company_name":"Test Co","email":"","password":"` + testPassword + `"}`},
		{"empty password", `{"company_name":"Test Co","email":"a@b.com","password":""}`},
		{"missing fields", `{}`},
		{"whitespace company_name", `{"company_name":"  ","email":"a@b.com","password":"` + testPassword + `"}`},
		{"short password", `{"company_name":"Test","email":"a@b.com","password":"short"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/signup", bytes.NewBufferString(tc.body))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}
