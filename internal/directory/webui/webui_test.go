package webui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFS_ContainsIndexHTML(t *testing.T) {
	data, err := IndexHTML()
	if err != nil {
		t.Fatalf("IndexHTML() error: %v", err)
	}
	if !strings.Contains(string(data), "Kaivue") {
		t.Error("index.html does not contain 'Kaivue'")
	}
}

func TestHandler_ServesIndexAtRoot(t *testing.T) {
	handler := Handler("/admin")

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Kaivue") {
		t.Error("response body does not contain 'Kaivue'")
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("expected text/html content-type, got %q", ct)
	}
}

func TestHandler_SpaFallback(t *testing.T) {
	handler := Handler("/admin")

	req := httptest.NewRequest(http.MethodGet, "/admin/cameras/cam-001/edit", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for SPA fallback, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Kaivue") {
		t.Error("SPA fallback did not serve index.html")
	}
}

func TestHandler_CacheControlHeader(t *testing.T) {
	handler := Handler("/admin")

	req := httptest.NewRequest(http.MethodGet, "/admin/cameras/cam-001/edit", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	cc := resp.Header.Get("Cache-Control")
	if cc != "no-store" {
		t.Errorf("expected Cache-Control: no-store, got %q", cc)
	}
}
