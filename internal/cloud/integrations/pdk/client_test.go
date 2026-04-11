package pdk_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/integrations/pdk"
)

func TestClient_Authenticate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.FormValue("grant_type") != "client_credentials" {
			t.Errorf("expected client_credentials, got %s", r.FormValue("grant_type"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pdk.TokenResponse{
			AccessToken: "test-token-abc",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		})
	}))
	defer ts.Close()

	client := pdk.NewClient(pdk.ClientConfig{
		HTTPClient:   ts.Client(),
		Endpoint:     ts.URL,
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		PanelID:      "panel-1",
	})

	err := client.Authenticate(context.Background())
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	// Second call should use cached token (no server request).
	err = client.Authenticate(context.Background())
	if err != nil {
		t.Fatalf("cached authenticate: %v", err)
	}
}

func TestClient_ListDoors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			json.NewEncoder(w).Encode(pdk.TokenResponse{
				AccessToken: "tok", TokenType: "Bearer", ExpiresIn: 3600,
			})
		case "/api/panels/panel-1/doors":
			if r.Header.Get("Authorization") != "Bearer tok" {
				t.Error("missing auth header")
			}
			json.NewEncoder(w).Encode([]pdk.PDKDoor{
				{ID: "d1", Name: "Front Door", Location: "Lobby", IsLocked: true},
				{ID: "d2", Name: "Back Door", Location: "Rear", IsLocked: false},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client := pdk.NewClient(pdk.ClientConfig{
		HTTPClient: ts.Client(),
		Endpoint:   ts.URL,
		ClientID:   "id",
		ClientSecret: "secret",
		PanelID:    "panel-1",
	})

	doors, err := client.ListDoors(context.Background())
	if err != nil {
		t.Fatalf("list doors: %v", err)
	}
	if len(doors) != 2 {
		t.Fatalf("expected 2 doors, got %d", len(doors))
	}
	if doors[0].Name != "Front Door" {
		t.Errorf("expected Front Door, got %s", doors[0].Name)
	}
}

func TestClient_GetDoor(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			json.NewEncoder(w).Encode(pdk.TokenResponse{
				AccessToken: "tok", TokenType: "Bearer", ExpiresIn: 3600,
			})
		case "/api/panels/panel-1/doors/d1":
			json.NewEncoder(w).Encode(pdk.PDKDoor{
				ID: "d1", Name: "Main", Location: "Entrance", IsLocked: true,
			})
		case "/api/panels/panel-1/doors/missing":
			w.WriteHeader(http.StatusNotFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client := pdk.NewClient(pdk.ClientConfig{
		HTTPClient: ts.Client(), Endpoint: ts.URL,
		ClientID: "id", ClientSecret: "secret", PanelID: "panel-1",
	})

	door, err := client.GetDoor(context.Background(), "d1")
	if err != nil {
		t.Fatalf("get door: %v", err)
	}
	if door.Name != "Main" {
		t.Errorf("expected Main, got %s", door.Name)
	}

	_, err = client.GetDoor(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing door")
	}
}

func TestClient_ListEvents(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			json.NewEncoder(w).Encode(pdk.TokenResponse{
				AccessToken: "tok", TokenType: "Bearer", ExpiresIn: 3600,
			})
		case "/api/panels/panel-1/events":
			json.NewEncoder(w).Encode([]pdk.PDKEvent{
				{
					ID: "ev1", DoorID: "d1", EventType: "access.granted",
					PersonName: "Alice", Credential: "card-1",
					Timestamp: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client := pdk.NewClient(pdk.ClientConfig{
		HTTPClient: ts.Client(), Endpoint: ts.URL,
		ClientID: "id", ClientSecret: "secret", PanelID: "panel-1",
	})

	since := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 4, 10, 23, 59, 59, 0, time.UTC)
	events, err := client.ListEvents(context.Background(), since, until)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].PersonName != "Alice" {
		t.Errorf("expected Alice, got %s", events[0].PersonName)
	}
}

func TestClient_UnlockDoor(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			json.NewEncoder(w).Encode(pdk.TokenResponse{
				AccessToken: "tok", TokenType: "Bearer", ExpiresIn: 3600,
			})
		case "/api/panels/panel-1/doors/d1/unlock":
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client := pdk.NewClient(pdk.ClientConfig{
		HTTPClient: ts.Client(), Endpoint: ts.URL,
		ClientID: "id", ClientSecret: "secret", PanelID: "panel-1",
	})

	err := client.UnlockDoor(context.Background(), "d1")
	if err != nil {
		t.Fatalf("unlock: %v", err)
	}
}

func TestClient_AuthFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	defer ts.Close()

	client := pdk.NewClient(pdk.ClientConfig{
		HTTPClient: ts.Client(), Endpoint: ts.URL,
		ClientID: "bad", ClientSecret: "bad", PanelID: "p1",
	})

	err := client.Authenticate(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
}
