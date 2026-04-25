package broker

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// helper: signs up a tenant + user and returns (tenantID, userID, email, password).
func newTestUser(t *testing.T, store *Store, email string) (string, string) {
	t.Helper()
	body := `{"company_name":"Test","email":"` + email + `","password":"` + testPassword + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/signup", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	SignupHandler(store).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("signup failed: %d %s", rec.Code, rec.Body.String())
	}
	var resp SignupResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	return resp.TenantID, resp.UserID
}

func TestLoginSuccess(t *testing.T) {
	store := newTestStore(t)
	tenantID, userID := newTestUser(t, store, "user@example.com")

	body := `{"email":"user@example.com","password":"` + testPassword + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	LoginHandler(store, SessionConfig{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp SessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.User == nil || resp.User.ID != userID {
		t.Errorf("user mismatch: got %+v", resp.User)
	}
	if resp.Tenant == nil || resp.Tenant.ID != tenantID {
		t.Errorf("tenant mismatch: got %+v", resp.Tenant)
	}

	// Cookie should be set, HttpOnly, with the right name.
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session cookie")
	}
	if !sessionCookie.HttpOnly {
		t.Error("expected HttpOnly")
	}
	if sessionCookie.Value == "" {
		t.Error("expected non-empty cookie value")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	store := newTestStore(t)
	newTestUser(t, store, "user@example.com")

	body := `{"email":"user@example.com","password":"wrong-password-here"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	LoginHandler(store, SessionConfig{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName && c.Value != "" {
			t.Error("should not set session cookie on bad credentials")
		}
	}
}

func TestLoginUnknownEmail(t *testing.T) {
	store := newTestStore(t)
	body := `{"email":"nobody@example.com","password":"anything"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	LoginHandler(store, SessionConfig{}).ServeHTTP(rec, req)

	// Same response shape as wrong-password — no user enumeration.
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestSessionRequiresCookie(t *testing.T) {
	store := newTestStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	rec := httptest.NewRecorder()
	SessionHandler(store).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestSessionWithCookie(t *testing.T) {
	store := newTestStore(t)
	_, userID := newTestUser(t, store, "user@example.com")

	// Log in to mint a cookie.
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/login",
		strings.NewReader(`{"email":"user@example.com","password":"`+testPassword+`"}`))
	loginRec := httptest.NewRecorder()
	LoginHandler(store, SessionConfig{}).ServeHTTP(loginRec, loginReq)
	cookies := loginRec.Result().Cookies()

	// Hit /session with the cookie.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	SessionHandler(store).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp SessionResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.User == nil || resp.User.ID != userID {
		t.Errorf("user mismatch: %+v", resp.User)
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	store := newTestStore(t)
	newTestUser(t, store, "user@example.com")

	// Log in.
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/login",
		strings.NewReader(`{"email":"user@example.com","password":"`+testPassword+`"}`))
	loginRec := httptest.NewRecorder()
	LoginHandler(store, SessionConfig{}).ServeHTTP(loginRec, loginReq)
	cookies := loginRec.Result().Cookies()

	// Log out.
	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/logout", nil)
	for _, c := range cookies {
		logoutReq.AddCookie(c)
	}
	logoutRec := httptest.NewRecorder()
	LogoutHandler(store, SessionConfig{}).ServeHTTP(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", logoutRec.Code)
	}

	// Same cookie should now fail at /session.
	sessionReq := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	for _, c := range cookies {
		sessionReq.AddCookie(c)
	}
	sessionRec := httptest.NewRecorder()
	SessionHandler(store).ServeHTTP(sessionRec, sessionReq)
	if sessionRec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 after logout, got %d", sessionRec.Code)
	}
}

func TestRefreshRotatesSession(t *testing.T) {
	store := newTestStore(t)
	newTestUser(t, store, "user@example.com")

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/login",
		strings.NewReader(`{"email":"user@example.com","password":"`+testPassword+`"}`))
	loginRec := httptest.NewRecorder()
	LoginHandler(store, SessionConfig{}).ServeHTTP(loginRec, loginReq)

	originalCookies := loginRec.Result().Cookies()
	var oldToken string
	for _, c := range originalCookies {
		if c.Name == SessionCookieName {
			oldToken = c.Value
		}
	}
	if oldToken == "" {
		t.Fatal("no old token")
	}

	refreshReq := httptest.NewRequest(http.MethodPost, "/api/v1/session/refresh", nil)
	for _, c := range originalCookies {
		refreshReq.AddCookie(c)
	}
	refreshRec := httptest.NewRecorder()
	RefreshHandler(store, SessionConfig{}).ServeHTTP(refreshRec, refreshReq)
	if refreshRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", refreshRec.Code)
	}

	var newToken string
	for _, c := range refreshRec.Result().Cookies() {
		if c.Name == SessionCookieName {
			newToken = c.Value
		}
	}
	if newToken == "" {
		t.Fatal("no new token")
	}
	if newToken == oldToken {
		t.Error("expected rotated token, got same")
	}

	// Old token should be revoked.
	if _, err := store.ValidateSession(oldToken); err == nil {
		t.Error("old token should be invalid after rotation")
	}
	// New token should work.
	if _, err := store.ValidateSession(newToken); err != nil {
		t.Errorf("new token should validate: %v", err)
	}
}

func TestSessionExpired(t *testing.T) {
	store := newTestStore(t)
	tenantID, userID := newTestUser(t, store, "user@example.com")

	// Mint a session with a past expiry by going through the store directly.
	token, err := store.CreateSession(userID, tenantID, -time.Second)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := store.ValidateSession(token); err == nil {
		t.Error("expected expired session to fail validation")
	}
}
