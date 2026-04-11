package brivo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// httpClient tests using httptest mock servers
// ---------------------------------------------------------------------------

func TestHTTPClient_ExchangeCode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("grant_type") != "authorization_code" {
			t.Errorf("unexpected grant_type: %s", r.FormValue("grant_type"))
		}
		if r.FormValue("code") != "auth-code" {
			t.Errorf("unexpected code: %s", r.FormValue("code"))
		}
		if r.FormValue("code_verifier") != "verifier-123" {
			t.Errorf("unexpected code_verifier: %s", r.FormValue("code_verifier"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "at-1",
			"refresh_token": "rt-1",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"scope":         "read write",
		})
	}))
	defer ts.Close()

	client := NewHTTPClient(HTTPClientConfig{
		OAuth: OAuthConfig{
			ClientID:     "cid",
			ClientSecret: "csecret",
			TokenURL:     ts.URL,
			RedirectURL:  "https://example.com/cb",
		},
	})

	tok, err := client.ExchangeCode(context.Background(), "auth-code", "verifier-123")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if tok.AccessToken != "at-1" {
		t.Errorf("expected at-1, got %s", tok.AccessToken)
	}
	if tok.RefreshToken != "rt-1" {
		t.Errorf("expected rt-1, got %s", tok.RefreshToken)
	}
}

func TestHTTPClient_ExchangeCode_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer ts.Close()

	client := NewHTTPClient(HTTPClientConfig{
		OAuth: OAuthConfig{TokenURL: ts.URL},
	})

	_, err := client.ExchangeCode(context.Background(), "bad-code", "v")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHTTPClient_RefreshToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("grant_type") != "refresh_token" {
			t.Errorf("unexpected grant_type: %s", r.FormValue("grant_type"))
		}
		if r.FormValue("refresh_token") != "old-rt" {
			t.Errorf("unexpected refresh_token: %s", r.FormValue("refresh_token"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "new-at",
			"refresh_token": "new-rt",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer ts.Close()

	client := NewHTTPClient(HTTPClientConfig{
		OAuth: OAuthConfig{
			ClientID:     "cid",
			ClientSecret: "csecret",
			TokenURL:     ts.URL,
		},
	})

	tok, err := client.RefreshToken(context.Background(), "old-rt")
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if tok.AccessToken != "new-at" {
		t.Errorf("expected new-at, got %s", tok.AccessToken)
	}
}

func TestHTTPClient_RefreshToken_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_token"}`))
	}))
	defer ts.Close()

	client := NewHTTPClient(HTTPClientConfig{
		OAuth: OAuthConfig{TokenURL: ts.URL},
	})

	_, err := client.RefreshToken(context.Background(), "bad-rt")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHTTPClient_ListSites(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sites" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{
				{"id": "s1", "siteName": "HQ"},
				{"id": "s2", "siteName": "Branch"},
			},
		})
	}))
	defer ts.Close()

	client := NewHTTPClient(HTTPClientConfig{APIURL: ts.URL})
	sites, err := client.ListSites(context.Background(), "test-token")
	if err != nil {
		t.Fatalf("ListSites: %v", err)
	}
	if len(sites) != 2 {
		t.Fatalf("expected 2 sites, got %d", len(sites))
	}
	if sites[0].Name != "HQ" {
		t.Errorf("expected HQ, got %s", sites[0].Name)
	}
}

func TestHTTPClient_ListSites_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden"))
	}))
	defer ts.Close()

	client := NewHTTPClient(HTTPClientConfig{APIURL: ts.URL})
	_, err := client.ListSites(context.Background(), "bad-token")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHTTPClient_ListDoors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sites/site-1/access-points" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{
				{"id": "d1", "name": "Main Entrance", "siteId": "site-1", "siteName": "HQ"},
			},
		})
	}))
	defer ts.Close()

	client := NewHTTPClient(HTTPClientConfig{APIURL: ts.URL})
	doors, err := client.ListDoors(context.Background(), "token", "site-1")
	if err != nil {
		t.Fatalf("ListDoors: %v", err)
	}
	if len(doors) != 1 {
		t.Fatalf("expected 1 door, got %d", len(doors))
	}
	if doors[0].Name != "Main Entrance" {
		t.Errorf("expected Main Entrance, got %s", doors[0].Name)
	}
}

func TestHTTPClient_SendEvent(t *testing.T) {
	var received NVREvent
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()

	client := NewHTTPClient(HTTPClientConfig{APIURL: ts.URL})
	event := NVREvent{
		TenantID: "t1",
		CameraID: "cam-1",
		DoorID:   "door-1",
		Action:   "lock",
		Reason:   "motion detected",
	}
	err := client.SendEvent(context.Background(), "token", event)
	if err != nil {
		t.Fatalf("SendEvent: %v", err)
	}
	if received.Action != "lock" {
		t.Errorf("expected lock, got %s", received.Action)
	}
}

func TestHTTPClient_SendEvent_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer ts.Close()

	client := NewHTTPClient(HTTPClientConfig{APIURL: ts.URL})
	err := client.SendEvent(context.Background(), "token", NVREvent{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHTTPClient_DefaultAPIURL(t *testing.T) {
	client := NewHTTPClient(HTTPClientConfig{})
	hc, ok := client.(*httpClient)
	if !ok {
		t.Fatal("expected *httpClient")
	}
	if hc.apiURL != "https://api.brivo.com/v1/api" {
		t.Errorf("unexpected default API URL: %s", hc.apiURL)
	}
}
