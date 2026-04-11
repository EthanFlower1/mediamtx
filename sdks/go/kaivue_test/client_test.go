package kaivue_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaivue/sdk-go/kaivue"
)

func TestCamerasList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/cameras" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Fatalf("missing or wrong API key header")
		}
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		resp := map[string]any{
			"cameras": []map[string]any{
				{
					"id":             "cam-001",
					"name":           "Front Door",
					"state":          "CAMERA_STATE_ONLINE",
					"recording_mode": "RECORDING_MODE_CONTINUOUS",
				},
			},
			"next_cursor": "",
			"total_count": 1,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := kaivue.NewClient(srv.URL, kaivue.WithAPIKey("test-key"))
	result, err := client.Cameras.List(context.Background(), &kaivue.ListCamerasRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Cameras) != 1 {
		t.Fatalf("expected 1 camera, got %d", len(result.Cameras))
	}
	if result.Cameras[0].Name != "Front Door" {
		t.Fatalf("expected camera name 'Front Door', got '%s'", result.Cameras[0].Name)
	}
	if result.Cameras[0].State != kaivue.CameraStateOnline {
		t.Fatalf("expected state Online, got '%s'", result.Cameras[0].State)
	}
}

func TestCameraGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/cameras/cam-001" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		resp := map[string]any{
			"camera": map[string]any{
				"id":   "cam-001",
				"name": "Front Door",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := kaivue.NewClient(srv.URL, kaivue.WithAPIKey("test-key"))
	cam, err := client.Cameras.Get(context.Background(), "cam-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cam.ID != "cam-001" {
		t.Fatalf("expected id cam-001, got %s", cam.ID)
	}
}

func TestNotFoundError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "req-999")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"message": "Camera not found"})
	}))
	defer srv.Close()

	client := kaivue.NewClient(srv.URL, kaivue.WithAPIKey("test-key"))
	_, err := client.Cameras.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !kaivue.IsNotFound(err) {
		t.Fatalf("expected NotFound error, got: %v", err)
	}
	apiErr := err.(*kaivue.APIError)
	if apiErr.RequestID != "req-999" {
		t.Fatalf("expected request_id req-999, got %s", apiErr.RequestID)
	}
}

func TestAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"message": "Invalid API key"})
	}))
	defer srv.Close()

	client := kaivue.NewClient(srv.URL, kaivue.WithAPIKey("bad-key"))
	_, err := client.Cameras.List(context.Background(), &kaivue.ListCamerasRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !kaivue.IsAuthError(err) {
		t.Fatalf("expected auth error, got: %v", err)
	}
}

func TestOAuthAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer my-token" {
			t.Fatalf("expected Bearer my-token, got %s", auth)
		}
		resp := map[string]any{
			"cameras":    []any{},
			"next_cursor": "",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := kaivue.NewClient(srv.URL, kaivue.WithOAuth("my-token"))
	_, err := client.Cameras.List(context.Background(), &kaivue.ListCamerasRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUsersList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"users": []map[string]any{
				{"id": "usr-001", "username": "jdoe", "email": "jdoe@example.com"},
			},
			"next_cursor": "",
			"total_count": 1,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := kaivue.NewClient(srv.URL, kaivue.WithAPIKey("k"))
	result, err := client.Users.List(context.Background(), &kaivue.ListUsersRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(result.Users))
	}
	if result.Users[0].Username != "jdoe" {
		t.Fatalf("expected username jdoe, got %s", result.Users[0].Username)
	}
}
