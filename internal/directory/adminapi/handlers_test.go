package adminapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	directorydb "github.com/bluenviron/mediamtx/internal/directory/db"
	"github.com/bluenviron/mediamtx/internal/directory/adminapi"
)

func setup(t *testing.T) *adminapi.Handlers {
	t.Helper()
	db, err := directorydb.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return &adminapi.Handlers{
		Store:  adminapi.NewStore(db.DB),
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}
}

func postJSON(handler http.HandlerFunc, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

func getJSON(handler http.HandlerFunc, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

func TestUserCRUD(t *testing.T) {
	h := setup(t)

	// Create user
	w := postJSON(h.UsersHandler(), "/api/v1/admin/users", map[string]string{
		"username": "admin",
		"password": "securepass123",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create user: got %d, body: %s", w.Code, w.Body.String())
	}
	var createResp map[string]string
	json.NewDecoder(w.Body).Decode(&createResp)
	if createResp["username"] != "admin" {
		t.Errorf("username = %s, want admin", createResp["username"])
	}

	// List users
	w2 := getJSON(h.UsersHandler(), "/api/v1/admin/users")
	if w2.Code != http.StatusOK {
		t.Fatalf("list users: %d", w2.Code)
	}
	var listResp struct{ Items []adminapi.User }
	json.NewDecoder(w2.Body).Decode(&listResp)
	if len(listResp.Items) != 1 {
		t.Fatalf("expected 1 user, got %d", len(listResp.Items))
	}

	// Delete user
	userID := createResp["id"]
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/by-id?id="+userID, nil)
	w3 := httptest.NewRecorder()
	h.UserByIDHandler()(w3, req)
	if w3.Code != http.StatusOK {
		t.Fatalf("delete user: %d", w3.Code)
	}

	// Verify deleted
	w4 := getJSON(h.UsersHandler(), "/api/v1/admin/users")
	var listResp2 struct{ Items []adminapi.User }
	json.NewDecoder(w4.Body).Decode(&listResp2)
	if len(listResp2.Items) != 0 {
		t.Errorf("expected 0 users after delete, got %d", len(listResp2.Items))
	}
}

func TestScheduleCRUD(t *testing.T) {
	h := setup(t)

	// Create
	w := postJSON(h.SchedulesHandler(), "/api/v1/admin/schedules", map[string]any{
		"mode":              "motion",
		"pre_roll_seconds":  5,
		"post_roll_seconds": 10,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create schedule: %d, body: %s", w.Code, w.Body.String())
	}

	// List
	w2 := getJSON(h.SchedulesHandler(), "/api/v1/admin/schedules")
	var listResp struct{ Items []adminapi.RecordingSchedule }
	json.NewDecoder(w2.Body).Decode(&listResp)
	if len(listResp.Items) != 1 {
		t.Fatalf("expected 1 schedule, got %d", len(listResp.Items))
	}
	if listResp.Items[0].Mode != "motion" {
		t.Errorf("mode = %s, want motion", listResp.Items[0].Mode)
	}
}

func TestAlertRuleCRUD(t *testing.T) {
	h := setup(t)

	w := postJSON(h.AlertRulesHandler(), "/api/v1/admin/alert-rules", map[string]any{
		"name":             "High Disk Usage",
		"rule_type":        "disk_usage",
		"threshold_value":  90.0,
		"enabled":          true,
		"notify_email":     true,
		"cooldown_minutes": 30,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create alert rule: %d, body: %s", w.Code, w.Body.String())
	}

	w2 := getJSON(h.AlertRulesHandler(), "/api/v1/admin/alert-rules")
	var listResp struct{ Items []adminapi.AlertRule }
	json.NewDecoder(w2.Body).Decode(&listResp)
	if len(listResp.Items) != 1 {
		t.Fatalf("expected 1 alert rule, got %d", len(listResp.Items))
	}
	if listResp.Items[0].Name != "High Disk Usage" {
		t.Errorf("name = %s, want High Disk Usage", listResp.Items[0].Name)
	}
	if !listResp.Items[0].Enabled {
		t.Error("expected enabled = true")
	}
}

func TestAuditLog(t *testing.T) {
	h := setup(t)

	// Post an audit event (as if from a recorder)
	w := postJSON(h.AuditHandler(), "/api/v1/admin/audit", map[string]string{
		"user_id":       "user-1",
		"username":      "admin",
		"recorder_id":   "rec-001",
		"action":        "login",
		"resource_type": "session",
		"details":       "login from 192.168.1.50",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("post audit: %d, body: %s", w.Code, w.Body.String())
	}

	// List audit entries
	w2 := getJSON(h.AuditHandler(), "/api/v1/admin/audit")
	var listResp struct{ Items []adminapi.AuditEntry }
	json.NewDecoder(w2.Body).Decode(&listResp)
	if len(listResp.Items) < 1 {
		t.Fatal("expected at least 1 audit entry")
	}
	if listResp.Items[0].Action != "login" {
		t.Errorf("action = %s, want login", listResp.Items[0].Action)
	}
}

func TestExportJobCRUD(t *testing.T) {
	h := setup(t)

	w := postJSON(h.ExportJobsHandler(), "/api/v1/admin/exports", map[string]string{
		"recorder_id":  "rec-001",
		"camera_id":    "cam-lobby",
		"start_time":   "2026-04-20T10:00:00Z",
		"end_time":     "2026-04-20T10:30:00Z",
		"format":       "mp4",
		"requested_by": "admin",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create export: %d, body: %s", w.Code, w.Body.String())
	}
	var createResp map[string]string
	json.NewDecoder(w.Body).Decode(&createResp)
	if createResp["status"] != "pending" {
		t.Errorf("status = %s, want pending", createResp["status"])
	}

	// List
	w2 := getJSON(h.ExportJobsHandler(), "/api/v1/admin/exports")
	var listResp struct{ Items []adminapi.ExportJob }
	json.NewDecoder(w2.Body).Decode(&listResp)
	if len(listResp.Items) != 1 {
		t.Fatalf("expected 1 export job, got %d", len(listResp.Items))
	}
	if listResp.Items[0].Status != "pending" {
		t.Errorf("status = %s, want pending", listResp.Items[0].Status)
	}
}

func TestRetentionCRUD(t *testing.T) {
	h := setup(t)

	w := postJSON(h.RetentionHandler(), "/api/v1/admin/retention", map[string]any{
		"hot_days":          7,
		"warm_days":         14,
		"delete_after_days": 30,
		"archive_tier":      "standard",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create retention: %d, body: %s", w.Code, w.Body.String())
	}

	w2 := getJSON(h.RetentionHandler(), "/api/v1/admin/retention")
	var listResp struct{ Items []adminapi.RetentionPolicy }
	json.NewDecoder(w2.Body).Decode(&listResp)
	if len(listResp.Items) != 1 {
		t.Fatalf("expected 1 retention policy, got %d", len(listResp.Items))
	}
	if listResp.Items[0].HotDays != 7 {
		t.Errorf("hot_days = %d, want 7", listResp.Items[0].HotDays)
	}
}
