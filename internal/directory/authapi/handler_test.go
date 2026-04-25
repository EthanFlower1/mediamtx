package authapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test doubles ---

type fakeProvider struct {
	session *Session
	err     error
	revoked []SessionID
}

func (f *fakeProvider) AuthenticateLocal(_ context.Context, _ TenantRef, _, _ string) (*Session, error) {
	return f.session, f.err
}

func (f *fakeProvider) RefreshSession(_ context.Context, _ string) (*Session, error) {
	return f.session, f.err
}

func (f *fakeProvider) RevokeSession(_ context.Context, sid SessionID) error {
	f.revoked = append(f.revoked, sid)
	return f.err
}

func (f *fakeProvider) RevokeByRefreshToken(_ context.Context, _ string) error {
	return f.err
}

type ctxKey struct{}

func testTenantResolver(_ *http.Request) TenantRef {
	return TenantRef{Type: "customer", ID: "test-tenant"}
}

func testSessionFromCtx(ctx context.Context) (SessionID, bool) {
	sid, ok := ctx.Value(ctxKey{}).(SessionID)
	return sid, ok
}

func withSession(r *http.Request, sid SessionID) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxKey{}, sid))
}

var testSession = &Session{
	ID:           "sess-123",
	UserID:       "user-1",
	AccessToken:  "access-token-xyz",
	RefreshToken: "refresh-token-abc",
	IDToken:      "",
	ExpiresAt:    time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
}

// --- Login tests ---

func TestLogin_Success(t *testing.T) {
	h := NewHandlers(&fakeProvider{session: testSession}, testTenantResolver, testSessionFromCtx, nil)

	body := `{"username":"admin","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Login().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp LoginResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "access-token-xyz", resp.AccessToken)
	assert.Equal(t, "refresh-token-abc", resp.RefreshToken)
	assert.Equal(t, "sess-123", resp.SessionID)
}

func TestLogin_InvalidCredentials(t *testing.T) {
	h := NewHandlers(&fakeProvider{err: fmt.Errorf("invalid credentials")}, testTenantResolver, testSessionFromCtx, nil)

	body := `{"username":"admin","password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Login().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "UNAUTHORIZED", resp["code"])
	assert.Contains(t, resp["message"], "invalid credentials")
}

func TestLogin_MissingFields(t *testing.T) {
	h := NewHandlers(&fakeProvider{session: testSession}, testTenantResolver, testSessionFromCtx, nil)

	tests := []struct {
		name string
		body string
	}{
		{"missing username", `{"password":"secret"}`},
		{"missing password", `{"username":"admin"}`},
		{"empty both", `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			h.Login().ServeHTTP(rec, req)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestLogin_InvalidJSON(t *testing.T) {
	h := NewHandlers(&fakeProvider{session: testSession}, testTenantResolver, testSessionFromCtx, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader("not json"))
	rec := httptest.NewRecorder()

	h.Login().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestLogin_WrongMethod(t *testing.T) {
	h := NewHandlers(&fakeProvider{session: testSession}, testTenantResolver, testSessionFromCtx, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil)
	rec := httptest.NewRecorder()

	h.Login().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

// --- Refresh tests ---

func TestRefresh_Success(t *testing.T) {
	h := NewHandlers(&fakeProvider{session: testSession}, testTenantResolver, testSessionFromCtx, nil)

	body := `{"refresh_token":"refresh-token-abc"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Refresh().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp LoginResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "access-token-xyz", resp.AccessToken)
}

func TestRefresh_InvalidToken(t *testing.T) {
	h := NewHandlers(&fakeProvider{err: fmt.Errorf("session not found")}, testTenantResolver, testSessionFromCtx, nil)

	body := `{"refresh_token":"expired-token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Refresh().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRefresh_MissingToken(t *testing.T) {
	h := NewHandlers(&fakeProvider{session: testSession}, testTenantResolver, testSessionFromCtx, nil)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Refresh().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRefresh_WrongMethod(t *testing.T) {
	h := NewHandlers(&fakeProvider{session: testSession}, testTenantResolver, testSessionFromCtx, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/refresh", nil)
	rec := httptest.NewRecorder()

	h.Refresh().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

// --- Logout tests ---

func TestLogout_Success(t *testing.T) {
	fp := &fakeProvider{}
	h := NewHandlers(fp, testTenantResolver, testSessionFromCtx, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req = withSession(req, "sess-123")
	rec := httptest.NewRecorder()

	h.Logout().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, []SessionID{"sess-123"}, fp.revoked)
}

func TestLogout_NoSessionNoBody(t *testing.T) {
	h := NewHandlers(&fakeProvider{}, testTenantResolver, testSessionFromCtx, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	rec := httptest.NewRecorder()

	h.Logout().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestLogout_ByRefreshToken(t *testing.T) {
	fp := &fakeProvider{}
	h := NewHandlers(fp, testTenantResolver, testSessionFromCtx, nil)

	body := `{"refresh_token":"my-refresh-token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Logout().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestLogout_RevokeError(t *testing.T) {
	h := NewHandlers(&fakeProvider{err: fmt.Errorf("db error")}, testTenantResolver, testSessionFromCtx, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req = withSession(req, "sess-456")
	rec := httptest.NewRecorder()

	h.Logout().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestLogout_WrongMethod(t *testing.T) {
	h := NewHandlers(&fakeProvider{}, testTenantResolver, testSessionFromCtx, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/logout", nil)
	rec := httptest.NewRecorder()

	h.Logout().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
