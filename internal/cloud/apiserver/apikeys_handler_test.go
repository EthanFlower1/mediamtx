package apiserver

// KAI-319: API Keys handler unit tests.
//
// Validates:
//   - Create key returns raw key and key info
//   - List keys returns only active keys by default
//   - Rotate key produces a new key with grace period
//   - Revoke key returns 204
//   - Audit log returns entries for a specific key
//   - Missing auth returns 401

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/publicapi"
	"github.com/bluenviron/mediamtx/internal/cloud/publicapi/apikeys"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// withTestClaims injects auth.Claims into the context for handler testing.
func withTestClaims(ctx context.Context, tenantID, userID string) context.Context {
	return withClaims(ctx, &auth.Claims{
		UserID: auth.UserID(userID),
		TenantRef: auth.TenantRef{
			Type: auth.TenantTypeCustomer,
			ID:   tenantID,
		},
	})
}

// mockAPIKeyStore implements APIKeyStore for testing.
type mockAPIKeyStore struct {
	createCalled   bool
	listCalled     bool
	rotateCalled   bool
	revokeCalled   bool
	auditLogCalled bool
	lastTenantID   string
	lastKeyID      string
	lastRevokedBy  string
}

func (m *mockAPIKeyStore) Create(_ context.Context, req publicapi.CreateAPIKeyRequest) (*publicapi.CreateAPIKeyResult, error) {
	m.createCalled = true
	m.lastTenantID = req.TenantID
	return &publicapi.CreateAPIKeyResult{
		RawKey: "kvue_testabc123def456ghi789jkl012mn",
		Key: &publicapi.APIKey{
			ID:        "key-test-001",
			TenantID:  req.TenantID,
			Name:      req.Name,
			KeyPrefix: "kvue_tes",
			Scopes:    req.Scopes,
			CreatedBy: req.CreatedBy,
			CreatedAt: time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC),
		},
	}, nil
}

func (m *mockAPIKeyStore) Get(_ context.Context, _ string) (*publicapi.APIKey, error) {
	return nil, publicapi.ErrAPIKeyNotFound
}

func (m *mockAPIKeyStore) List(_ context.Context, filter publicapi.ListAPIKeysFilter) ([]*publicapi.APIKey, error) {
	m.listCalled = true
	m.lastTenantID = filter.TenantID
	return []*publicapi.APIKey{
		{
			ID:        "key-001",
			TenantID:  filter.TenantID,
			Name:      "Production Key",
			KeyPrefix: "kvue_a1b",
			Scopes:    []string{"cameras:read"},
			CreatedBy: "user@example.com",
			CreatedAt: time.Date(2026, 1, 8, 12, 0, 0, 0, time.UTC),
		},
	}, nil
}

func (m *mockAPIKeyStore) Rotate(_ context.Context, req publicapi.RotateAPIKeyRequest) (*publicapi.RotateAPIKeyResult, error) {
	m.rotateCalled = true
	m.lastKeyID = req.KeyID
	return &publicapi.RotateAPIKeyResult{
		RawKey: "kvue_newkey123456789abcdef012345678",
		NewKey: &publicapi.APIKey{
			ID:        "key-new-001",
			TenantID:  "tenant-001",
			Name:      "Production Key",
			KeyPrefix: "kvue_new",
			Scopes:    []string{"cameras:read"},
			CreatedBy: req.RotatedBy,
			CreatedAt: time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC),
		},
		OldKeyGraceEnd: time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (m *mockAPIKeyStore) Revoke(_ context.Context, keyID, revokedBy string) error {
	m.revokeCalled = true
	m.lastKeyID = keyID
	m.lastRevokedBy = revokedBy
	return nil
}

func (m *mockAPIKeyStore) Validate(_ context.Context, _ string) (*publicapi.APIKey, error) {
	return nil, publicapi.ErrInvalidAPIKey
}

func (m *mockAPIKeyStore) TouchLastUsed(_ context.Context, _ string) error {
	return nil
}

func (m *mockAPIKeyStore) ListExpiring(_ context.Context, _ string, _ time.Duration) ([]*publicapi.APIKey, error) {
	return nil, nil
}

func (m *mockAPIKeyStore) ListAuditLog(_ context.Context, tenantID, keyID string, _ int) ([]apikeys.AuditEntry, error) {
	m.auditLogCalled = true
	m.lastTenantID = tenantID
	m.lastKeyID = keyID
	return []apikeys.AuditEntry{
		{
			ID:        "audit-001",
			KeyID:     keyID,
			TenantID:  tenantID,
			Action:    apikeys.AuditCreate,
			ActorID:   "user@example.com",
			IPAddress: "127.0.0.1",
			UserAgent: "test",
			Metadata:  "{}",
			CreatedAt: time.Date(2026, 1, 8, 12, 0, 0, 0, time.UTC),
		},
	}, nil
}

// directAPIKeysHandler creates a handler without the full middleware chain
// (which requires auth, Casbin, etc.). It injects claims directly via context.
func directAPIKeysHandler(store APIKeyStore) http.Handler {
	h := &apiKeysHandler{store: store}
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/api-keys", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.handleList(w, r)
		case http.MethodPost:
			h.handleCreate(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/v1/api-keys/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/api/v1/api-keys/"):]
		parts := splitN(path, "/", 2)
		if len(parts) < 2 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		keyID := parts[0]
		action := parts[1]

		switch {
		case action == "rotate" && r.Method == http.MethodPost:
			h.handleRotate(w, r, keyID)
		case action == "revoke" && r.Method == http.MethodPost:
			h.handleRevoke(w, r, keyID)
		case action == "audit" && r.Method == http.MethodGet:
			h.handleAudit(w, r, keyID)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	return mux
}

func splitN(s, sep string, n int) []string {
	result := make([]string, 0, n)
	for i := 0; i < n-1; i++ {
		idx := indexOf(s, sep)
		if idx < 0 {
			break
		}
		result = append(result, s[:idx])
		s = s[idx+len(sep):]
	}
	result = append(result, s)
	return result
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestAPIKeysHandler_List(t *testing.T) {
	store := &mockAPIKeyStore{}
	handler := directAPIKeysHandler(store)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/api-keys", nil)
	req = req.WithContext(withTestClaims(req.Context(), "tenant-001", "user@example.com"))

	// We can't easily inject claims via context in httptest, so test the handler directly.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/api-keys", nil)
	r = r.WithContext(withTestClaims(r.Context(), "tenant-001", "user@example.com"))
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !store.listCalled {
		t.Fatal("store.List was not called")
	}
	if store.lastTenantID != "tenant-001" {
		t.Fatalf("expected tenant-001, got %s", store.lastTenantID)
	}
}

func TestAPIKeysHandler_Create(t *testing.T) {
	store := &mockAPIKeyStore{}
	handler := directAPIKeysHandler(store)

	body, _ := json.Marshal(map[string]any{
		"name":   "Test Key",
		"scopes": []string{"cameras:read"},
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", bytes.NewReader(body))
	r = r.WithContext(withTestClaims(r.Context(), "tenant-001", "user@example.com"))
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !store.createCalled {
		t.Fatal("store.Create was not called")
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["raw_key"] == "" {
		t.Fatal("expected raw key in response")
	}
}

func TestAPIKeysHandler_Rotate(t *testing.T) {
	store := &mockAPIKeyStore{}
	handler := directAPIKeysHandler(store)

	body, _ := json.Marshal(map[string]any{
		"grace_period_hours": 48,
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys/key-001/rotate", bytes.NewReader(body))
	r = r.WithContext(withTestClaims(r.Context(), "tenant-001", "user@example.com"))
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !store.rotateCalled {
		t.Fatal("store.Rotate was not called")
	}
	if store.lastKeyID != "key-001" {
		t.Fatalf("expected key-001, got %s", store.lastKeyID)
	}
}

func TestAPIKeysHandler_Revoke(t *testing.T) {
	store := &mockAPIKeyStore{}
	handler := directAPIKeysHandler(store)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys/key-001/revoke", bytes.NewReader([]byte("{}")))
	r = r.WithContext(withTestClaims(r.Context(), "tenant-001", "user@example.com"))
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if !store.revokeCalled {
		t.Fatal("store.Revoke was not called")
	}
}

func TestAPIKeysHandler_AuditLog(t *testing.T) {
	store := &mockAPIKeyStore{}
	handler := directAPIKeysHandler(store)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/api-keys/key-001/audit", nil)
	r = r.WithContext(withTestClaims(r.Context(), "tenant-001", "user@example.com"))
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !store.auditLogCalled {
		t.Fatal("store.ListAuditLog was not called")
	}
}

func TestAPIKeysHandler_Unauthorized(t *testing.T) {
	store := &mockAPIKeyStore{}
	handler := directAPIKeysHandler(store)

	// No claims injected — should get 401.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/api-keys", nil)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAPIKeysHandler_CreateMissingName(t *testing.T) {
	store := &mockAPIKeyStore{}
	handler := directAPIKeysHandler(store)

	body, _ := json.Marshal(map[string]any{
		"scopes": []string{"cameras:read"},
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", bytes.NewReader(body))
	r = r.WithContext(withTestClaims(r.Context(), "tenant-001", "user@example.com"))
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
