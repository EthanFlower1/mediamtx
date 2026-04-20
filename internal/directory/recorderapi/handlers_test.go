package recorderapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	directorydb "github.com/bluenviron/mediamtx/internal/directory/db"
	"github.com/bluenviron/mediamtx/internal/directory/recorderapi"
	"github.com/bluenviron/mediamtx/internal/directory/recordercontrol"
)

func setupTest(t *testing.T) (*recorderapi.Handlers, *recorderapi.Store) {
	t.Helper()

	db, err := directorydb.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	store := recorderapi.NewStore(db.DB)
	rcStore := recordercontrol.NewStore(db.DB)
	eventBus := recordercontrol.NewEventBus()

	handlers := &recorderapi.Handlers{
		Store:    store,
		RCStore:  rcStore,
		EventBus: eventBus,
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}

	return handlers, store
}

func TestRegisterAndHeartbeat(t *testing.T) {
	handlers, store := setupTest(t)

	// Register a recorder.
	regBody := `{"recorder_id":"rec-001","hostname":"test-host","listen_addr":":8880","version":"1.0"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/recorders/register", bytes.NewReader([]byte(regBody)))
	w := httptest.NewRecorder()
	handlers.RegisterHandler()(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("register: got %d, body: %s", resp.StatusCode, body)
	}

	var regResp map[string]string
	json.NewDecoder(resp.Body).Decode(&regResp)

	if regResp["recorder_id"] != "rec-001" {
		t.Errorf("recorder_id = %s, want rec-001", regResp["recorder_id"])
	}
	serviceToken := regResp["service_token"]
	if serviceToken == "" {
		t.Fatal("service_token is empty")
	}

	// Verify the recorder is in the DB.
	rec, err := store.GetRecorder(context.Background(), "rec-001")
	if err != nil {
		t.Fatalf("get recorder: %v", err)
	}
	if rec.Hostname != "test-host" {
		t.Errorf("hostname = %s, want test-host", rec.Hostname)
	}

	// Validate the service token works.
	id, err := store.ValidateToken(context.Background(), serviceToken)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	if id != "rec-001" {
		t.Errorf("validated id = %s, want rec-001", id)
	}

	// Send a heartbeat.
	hbBody := `{"recorder_id":"rec-001","camera_count":4,"disk_used_pct":55.0,"uptime_sec":3600}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/recorders/heartbeat", bytes.NewReader([]byte(hbBody)))
	w2 := httptest.NewRecorder()
	handlers.HeartbeatHandler()(w2, req2)

	if w2.Result().StatusCode != http.StatusOK {
		body, _ := io.ReadAll(w2.Result().Body)
		t.Fatalf("heartbeat: got %d, body: %s", w2.Result().StatusCode, body)
	}

	// Verify health status updated.
	rec2, _ := store.GetRecorder(context.Background(), "rec-001")
	if rec2.HealthStatus != "healthy" {
		t.Errorf("health_status = %s, want healthy", rec2.HealthStatus)
	}
}

func TestCameraCRUD(t *testing.T) {
	handlers, _ := setupTest(t)

	// First register a recorder.
	regBody := `{"recorder_id":"rec-002","hostname":"cam-host","listen_addr":":8881"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/recorders/register", bytes.NewReader([]byte(regBody)))
	w := httptest.NewRecorder()
	handlers.RegisterHandler()(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("register failed: %d", w.Result().StatusCode)
	}

	// Create a camera.
	camBody := `{"camera_id":"cam-lobby","recorder_id":"rec-002","name":"Lobby Cam","stream_url":"rtsp://192.168.1.10/stream1","record_mode":"always"}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/cameras", bytes.NewReader([]byte(camBody)))
	w2 := httptest.NewRecorder()
	handlers.CreateCameraHandler()(w2, req2)

	if w2.Result().StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(w2.Result().Body)
		t.Fatalf("create camera: got %d, body: %s", w2.Result().StatusCode, body)
	}

	// List cameras for recorder.
	req3 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/cameras?recorder_id=rec-002"), nil)
	w3 := httptest.NewRecorder()
	handlers.ListCamerasHandler()(w3, req3)

	if w3.Result().StatusCode != http.StatusOK {
		t.Fatalf("list cameras: got %d", w3.Result().StatusCode)
	}

	var listResp struct {
		Items []recordercontrol.CameraRow `json:"items"`
	}
	json.NewDecoder(w3.Result().Body).Decode(&listResp)
	if len(listResp.Items) != 1 {
		t.Fatalf("expected 1 camera, got %d", len(listResp.Items))
	}
	if listResp.Items[0].CameraID != "cam-lobby" {
		t.Errorf("camera_id = %s, want cam-lobby", listResp.Items[0].CameraID)
	}

	// Delete camera.
	req4 := httptest.NewRequest(http.MethodDelete, "/api/v1/cameras?camera_id=cam-lobby", nil)
	w4 := httptest.NewRecorder()
	handlers.DeleteCameraHandler()(w4, req4)

	if w4.Result().StatusCode != http.StatusOK {
		body, _ := io.ReadAll(w4.Result().Body)
		t.Fatalf("delete camera: got %d, body: %s", w4.Result().StatusCode, body)
	}

	// Verify deleted.
	req5 := httptest.NewRequest(http.MethodGet, "/api/v1/cameras?recorder_id=rec-002", nil)
	w5 := httptest.NewRecorder()
	handlers.ListCamerasHandler()(w5, req5)

	var listResp2 struct {
		Items []recordercontrol.CameraRow `json:"items"`
	}
	json.NewDecoder(w5.Result().Body).Decode(&listResp2)
	if len(listResp2.Items) != 0 {
		t.Errorf("expected 0 cameras after delete, got %d", len(listResp2.Items))
	}
}

func TestBearerAuthMiddleware(t *testing.T) {
	db, err := directorydb.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	store := recorderapi.NewStore(db.DB)

	// Register a recorder and get a token.
	err = store.UpsertRecorder(context.Background(), recorderapi.RecorderRow{
		ID: "rec-auth-test", Hostname: "test",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	token, err := store.SetServiceToken(context.Background(), "rec-auth-test")
	if err != nil {
		t.Fatalf("set token: %v", err)
	}

	// Create a handler behind the middleware.
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := recorderapi.RecorderIDFromContext(r.Context())
		if !ok {
			http.Error(w, "no recorder id", 500)
			return
		}
		w.Write([]byte(id))
	})

	handler := recorderapi.BearerAuthMiddleware(store)(inner)

	t.Run("no token returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", w.Code)
		}
	})

	t.Run("wrong token returns 403", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("got %d, want 403", w.Code)
		}
	})

	t.Run("valid token returns recorder id", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("got %d, want 200", w.Code)
		}
		body, _ := io.ReadAll(w.Result().Body)
		if string(body) != "rec-auth-test" {
			t.Errorf("body = %q, want rec-auth-test", body)
		}
	})
}
