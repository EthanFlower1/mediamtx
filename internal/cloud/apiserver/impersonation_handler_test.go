package apiserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	"github.com/bluenviron/mediamtx/internal/cloud/identity/crosstenant"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// --- fakes ------------------------------------------------------------------

type fakeImpersonationService struct {
	mintResult *crosstenant.ScopedToken
	mintErr    error
	revokeErr  error
}

func (f *fakeImpersonationService) MintScopedToken(_ context.Context, _ auth.UserID, _ string) (*crosstenant.ScopedToken, error) {
	return f.mintResult, f.mintErr
}

func (f *fakeImpersonationService) RevokeScopedSession(_ context.Context, _ string) error {
	return f.revokeErr
}

type fakeAuditRecorder struct {
	entries []audit.Entry
}

func (f *fakeAuditRecorder) Record(_ context.Context, e audit.Entry) error {
	f.entries = append(f.entries, e)
	return nil
}

func (f *fakeAuditRecorder) Query(_ context.Context, filter audit.QueryFilter) ([]audit.Entry, error) {
	var result []audit.Entry
	for _, e := range f.entries {
		if e.TenantID == filter.TenantID || (filter.IncludeImpersonatedTenant && e.ImpersonatedTenantID != nil && *e.ImpersonatedTenantID == filter.TenantID) {
			result = append(result, e)
		}
	}
	return result, nil
}

func (f *fakeAuditRecorder) Export(_ context.Context, _ audit.QueryFilter, _ audit.ExportFormat, _ interface{ Write([]byte) (int, error) }) error {
	return nil
}

// --- tests ------------------------------------------------------------------

func TestImpersonationStart(t *testing.T) {
	svc := &fakeImpersonationService{
		mintResult: &crosstenant.ScopedToken{
			Token:            "test-token",
			SessionID:        "sess-123",
			ExpiresAt:        time.Now().Add(15 * time.Minute),
			CustomerTenantID: "cust-001",
			PermissionScope:  []string{"cameras.read", "streams.read"},
		},
	}
	recorder := &fakeAuditRecorder{}
	sessionStore := crosstenant.NewInMemorySessionStore()

	h := &impersonationHandler{
		service:       svc,
		auditRecorder: recorder,
		sessionStore:  sessionStore,
	}

	body, _ := json.Marshal(startImpersonationRequest{
		CustomerTenantID: "cust-001",
		Reason:           "Troubleshooting camera issue",
		ConsentAcked:     true,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/impersonation/start", bytes.NewReader(body))
	req = req.WithContext(withClaims(req.Context(), &auth.Claims{
		UserID:    "integrator-user-1",
		TenantRef: auth.TenantRef{Type: auth.TenantTypeIntegrator, ID: "int-001"},
	}))

	rr := httptest.NewRecorder()
	h.start(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp startImpersonationResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.SessionID != "sess-123" {
		t.Errorf("expected session_id sess-123, got %s", resp.SessionID)
	}
	if resp.Token != "test-token" {
		t.Errorf("expected token test-token, got %s", resp.Token)
	}
	if len(resp.PermissionScope) != 2 {
		t.Errorf("expected 2 permissions, got %d", len(resp.PermissionScope))
	}
	if resp.TTLSeconds <= 0 {
		t.Errorf("expected positive TTL, got %d", resp.TTLSeconds)
	}

	// Verify audit entry was recorded.
	if len(recorder.entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(recorder.entries))
	}
	if recorder.entries[0].Action != "impersonation.session_start" {
		t.Errorf("expected action impersonation.session_start, got %s", recorder.entries[0].Action)
	}
}

func TestImpersonationStartRequiresConsent(t *testing.T) {
	h := &impersonationHandler{
		service:       &fakeImpersonationService{},
		auditRecorder: &fakeAuditRecorder{},
		sessionStore:  crosstenant.NewInMemorySessionStore(),
	}

	body, _ := json.Marshal(startImpersonationRequest{
		CustomerTenantID: "cust-001",
		ConsentAcked:     false,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/impersonation/start", bytes.NewReader(body))
	req = req.WithContext(withClaims(req.Context(), &auth.Claims{
		UserID:    "integrator-user-1",
		TenantRef: auth.TenantRef{Type: auth.TenantTypeIntegrator, ID: "int-001"},
	}))

	rr := httptest.NewRecorder()
	h.start(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without consent, got %d", rr.Code)
	}
}

func TestImpersonationEnd(t *testing.T) {
	svc := &fakeImpersonationService{}
	recorder := &fakeAuditRecorder{}

	h := &impersonationHandler{
		service:       svc,
		auditRecorder: recorder,
		sessionStore:  crosstenant.NewInMemorySessionStore(),
	}

	body, _ := json.Marshal(endImpersonationRequest{SessionID: "sess-123"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/impersonation/end", bytes.NewReader(body))
	req = req.WithContext(withClaims(req.Context(), &auth.Claims{
		UserID:    "integrator-user-1",
		TenantRef: auth.TenantRef{Type: auth.TenantTypeIntegrator, ID: "int-001"},
	}))

	rr := httptest.NewRecorder()
	h.end(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	if len(recorder.entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(recorder.entries))
	}
	if recorder.entries[0].Action != "impersonation.session_end" {
		t.Errorf("expected action impersonation.session_end, got %s", recorder.entries[0].Action)
	}
}

func TestImpersonationNoRelationship(t *testing.T) {
	svc := &fakeImpersonationService{
		mintErr: crosstenant.ErrNoRelationship,
	}

	h := &impersonationHandler{
		service:       svc,
		auditRecorder: &fakeAuditRecorder{},
		sessionStore:  crosstenant.NewInMemorySessionStore(),
	}

	body, _ := json.Marshal(startImpersonationRequest{
		CustomerTenantID: "cust-999",
		ConsentAcked:     true,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/impersonation/start", bytes.NewReader(body))
	req = req.WithContext(withClaims(req.Context(), &auth.Claims{
		UserID:    "integrator-user-1",
		TenantRef: auth.TenantRef{Type: auth.TenantTypeIntegrator, ID: "int-001"},
	}))

	rr := httptest.NewRecorder()
	h.start(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}
