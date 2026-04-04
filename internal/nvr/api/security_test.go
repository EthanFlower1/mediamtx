package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupTestRouter(cfg SecurityConfig) *gin.Engine {
	r := gin.New()
	r.Use(CORSMiddleware(cfg))
	r.Use(SecurityHeadersMiddleware(cfg))
	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return r
}

func TestCORSMiddleware_AllowedOrigin(t *testing.T) {
	cfg := DefaultSecurityConfig()
	cfg.CORSAllowedOrigins = []string{"https://admin.example.com"}

	r := setupTestRouter(cfg)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://admin.example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "https://admin.example.com" {
		t.Errorf("expected allowed origin, got %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSMiddleware_DisallowedOrigin(t *testing.T) {
	cfg := DefaultSecurityConfig()
	cfg.CORSAllowedOrigins = []string{"https://admin.example.com"}

	r := setupTestRouter(cfg)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("expected no CORS header for disallowed origin, got %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSMiddleware_Preflight(t *testing.T) {
	cfg := DefaultSecurityConfig()
	cfg.CORSAllowedOrigins = []string{"https://admin.example.com"}

	r := setupTestRouter(cfg)

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://admin.example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 for preflight, got %d", w.Code)
	}
}

func TestCORSMiddleware_NoOrigin(t *testing.T) {
	cfg := DefaultSecurityConfig()
	r := setupTestRouter(cfg)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("expected no CORS header when no Origin sent")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	cfg := DefaultSecurityConfig()
	r := setupTestRouter(cfg)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	checks := map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}
	for header, expected := range checks {
		if got := w.Header().Get(header); got != expected {
			t.Errorf("%s: expected %q, got %q", header, expected, got)
		}
	}

	if csp := w.Header().Get("Content-Security-Policy"); csp == "" {
		t.Error("expected Content-Security-Policy header to be set")
	}
}

func TestSecurityHeaders_CustomCSP(t *testing.T) {
	cfg := DefaultSecurityConfig()
	cfg.ContentSecurityPolicy = "default-src 'none'"

	r := setupTestRouter(cfg)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Content-Security-Policy"); got != "default-src 'none'" {
		t.Errorf("expected custom CSP, got %q", got)
	}
}

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(10, 5, 60)
	defer rl.Stop()

	// First 5 should be allowed (burst).
	for i := 0; i < 5; i++ {
		if !rl.Allow("1.2.3.4") {
			t.Fatalf("request %d should be allowed", i)
		}
	}

	// 6th should be denied (burst exhausted).
	if rl.Allow("1.2.3.4") {
		t.Error("expected request to be denied after burst exhausted")
	}
}

func TestRateLimiter_DifferentIPs(t *testing.T) {
	rl := NewRateLimiter(10, 2, 60)
	defer rl.Stop()

	// Exhaust IP1.
	rl.Allow("1.1.1.1")
	rl.Allow("1.1.1.1")

	// IP2 should still be allowed.
	if !rl.Allow("2.2.2.2") {
		t.Error("different IP should have its own bucket")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	rl := NewRateLimiter(100, 2, 60)
	defer rl.Stop()

	r := gin.New()
	r.Use(RateLimitMiddleware(rl))
	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// First 2 requests should succeed.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, w.Code)
		}
	}

	// 3rd request should be rate limited.
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
}

func TestIntToStr(t *testing.T) {
	cases := []struct {
		in  int
		out string
	}{
		{0, "0"},
		{3600, "3600"},
		{-1, "-1"},
		{42, "42"},
	}
	for _, tc := range cases {
		if got := intToStr(tc.in); got != tc.out {
			t.Errorf("intToStr(%d) = %q, want %q", tc.in, got, tc.out)
		}
	}
}
