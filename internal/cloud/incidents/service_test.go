package incidents_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/incidents"
)

func openTestDB(t *testing.T) *clouddb.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := "sqlite://" + filepath.Join(dir, "cloud.db")
	d, err := clouddb.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

var seqID int

func testIDGen() string {
	seqID++
	return fmt.Sprintf("test-%04d", seqID)
}

func newService(t *testing.T, pd incidents.PagerDutyClient) *incidents.Service {
	t.Helper()
	db := openTestDB(t)
	svc, err := incidents.NewService(incidents.Config{
		DB:             db,
		PagerDuty:      pd,
		IDGen:          testIDGen,
		DefaultRouting: "test-routing-key",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

// mockPagerDuty records events sent to PagerDuty for assertions.
type mockPagerDuty struct {
	events []incidents.PagerDutyEvent
}

func (m *mockPagerDuty) SendEvent(_ context.Context, event incidents.PagerDutyEvent) (incidents.PagerDutyResponse, error) {
	m.events = append(m.events, event)
	return incidents.PagerDutyResponse{
		Status:   "success",
		Message:  "Event processed",
		DedupKey: event.DedupKey,
	}, nil
}

func TestNewServiceRequiresDB(t *testing.T) {
	_, err := incidents.NewService(incidents.Config{})
	if err == nil {
		t.Fatal("expected error when DB is nil")
	}
}

func TestCreateAndGetIncident(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	inc := incidents.Incident{
		TenantID:          "tenant-1",
		AlertName:         "HighCPU",
		Severity:          incidents.SeverityCritical,
		Status:            incidents.StatusTriggered,
		Summary:           "CPU usage above 90%",
		Source:             "node-1",
		AffectedComponent: "recording",
	}

	created, err := svc.CreateIncident(ctx, inc)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.IncidentID == "" {
		t.Fatal("expected incident_id")
	}

	got, err := svc.GetIncident(ctx, "tenant-1", created.IncidentID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.AlertName != "HighCPU" {
		t.Errorf("expected HighCPU, got %s", got.AlertName)
	}
	if got.Severity != incidents.SeverityCritical {
		t.Errorf("expected critical, got %s", got.Severity)
	}
}

func TestListActiveIncidents(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	svc.CreateIncident(ctx, incidents.Incident{
		TenantID:  "tenant-1",
		AlertName: "DiskFull",
		Severity:  incidents.SeverityError,
		Status:    incidents.StatusTriggered,
		Summary:   "Disk at 95%",
	})
	svc.CreateIncident(ctx, incidents.Incident{
		TenantID:  "tenant-1",
		AlertName: "HighMemory",
		Severity:  incidents.SeverityWarning,
		Status:    incidents.StatusTriggered,
		Summary:   "Memory at 85%",
	})

	active, err := svc.ListActiveIncidents(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("expected 2 active, got %d", len(active))
	}
}

func TestAcknowledgeIncident(t *testing.T) {
	mock := &mockPagerDuty{}
	svc := newService(t, mock)
	ctx := context.Background()

	inc, _ := svc.CreateIncident(ctx, incidents.Incident{
		TenantID:          "tenant-1",
		AlertName:         "HighCPU",
		Severity:          incidents.SeverityCritical,
		Status:            incidents.StatusTriggered,
		Summary:           "CPU above 90%",
		PagerDutyDedupKey: "tenant-1/HighCPU/abc",
	})

	if err := svc.AcknowledgeIncident(ctx, "tenant-1", inc.IncidentID); err != nil {
		t.Fatalf("ack: %v", err)
	}

	got, _ := svc.GetIncident(ctx, "tenant-1", inc.IncidentID)
	if got.Status != incidents.StatusAcknowledged {
		t.Errorf("expected acknowledged, got %s", got.Status)
	}
	if got.AcknowledgedAt == nil {
		t.Error("expected acknowledged_at to be set")
	}

	// PagerDuty should have received an acknowledge event.
	if len(mock.events) != 1 {
		t.Fatalf("expected 1 PD event, got %d", len(mock.events))
	}
	if mock.events[0].EventAction != "acknowledge" {
		t.Errorf("expected acknowledge action, got %s", mock.events[0].EventAction)
	}
}

func TestAcknowledgeNotFound(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	err := svc.AcknowledgeIncident(ctx, "tenant-1", "nonexistent")
	if err != incidents.ErrIncidentNotFound {
		t.Errorf("expected ErrIncidentNotFound, got %v", err)
	}
}

func TestResolveIncident(t *testing.T) {
	mock := &mockPagerDuty{}
	svc := newService(t, mock)
	ctx := context.Background()

	inc, _ := svc.CreateIncident(ctx, incidents.Incident{
		TenantID:          "tenant-1",
		AlertName:         "DiskFull",
		Severity:          incidents.SeverityError,
		Status:            incidents.StatusTriggered,
		Summary:           "Disk 95%",
		PagerDutyDedupKey: "tenant-1/DiskFull/xyz",
	})

	if err := svc.ResolveIncident(ctx, "tenant-1", inc.IncidentID); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	got, _ := svc.GetIncident(ctx, "tenant-1", inc.IncidentID)
	if got.Status != incidents.StatusResolved {
		t.Errorf("expected resolved, got %s", got.Status)
	}
	if got.ResolvedAt == nil {
		t.Error("expected resolved_at to be set")
	}

	// No longer in active list.
	active, _ := svc.ListActiveIncidents(ctx, "tenant-1")
	if len(active) != 0 {
		t.Fatalf("expected 0 active, got %d", len(active))
	}

	// PagerDuty resolve event sent.
	if len(mock.events) != 1 {
		t.Fatalf("expected 1 PD event, got %d", len(mock.events))
	}
	if mock.events[0].EventAction != "resolve" {
		t.Errorf("expected resolve action, got %s", mock.events[0].EventAction)
	}
}

func TestResolveNotFound(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	err := svc.ResolveIncident(ctx, "tenant-1", "nonexistent")
	if err != incidents.ErrIncidentNotFound {
		t.Errorf("expected ErrIncidentNotFound, got %v", err)
	}
}

// --- Runbook tests ---

func TestUpsertAndGetRunbook(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	rb, err := svc.UpsertRunbook(ctx, incidents.RunbookMapping{
		TenantID:   "tenant-1",
		AlertName:  "HighCPU",
		RunbookURL: "https://wiki.example.com/runbooks/high-cpu",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if rb.MappingID == "" {
		t.Fatal("expected mapping_id")
	}

	url, err := svc.GetRunbookURL(ctx, "tenant-1", "HighCPU")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if url != "https://wiki.example.com/runbooks/high-cpu" {
		t.Errorf("unexpected url: %s", url)
	}
}

func TestGetRunbookNotFound(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	_, err := svc.GetRunbookURL(ctx, "tenant-1", "nonexistent")
	if err != incidents.ErrRunbookNotFound {
		t.Errorf("expected ErrRunbookNotFound, got %v", err)
	}
}

func TestListRunbooks(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	svc.UpsertRunbook(ctx, incidents.RunbookMapping{
		TenantID: "tenant-1", AlertName: "HighCPU", RunbookURL: "https://wiki.example.com/cpu",
	})
	svc.UpsertRunbook(ctx, incidents.RunbookMapping{
		TenantID: "tenant-1", AlertName: "DiskFull", RunbookURL: "https://wiki.example.com/disk",
	})

	rbs, err := svc.ListRunbooks(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rbs) != 2 {
		t.Fatalf("expected 2, got %d", len(rbs))
	}
}

func TestDeleteRunbook(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	svc.UpsertRunbook(ctx, incidents.RunbookMapping{
		TenantID: "tenant-1", AlertName: "HighCPU", RunbookURL: "https://example.com",
	})

	if err := svc.DeleteRunbook(ctx, "tenant-1", "HighCPU"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	err := svc.DeleteRunbook(ctx, "tenant-1", "HighCPU")
	if err != incidents.ErrRunbookNotFound {
		t.Errorf("expected ErrRunbookNotFound, got %v", err)
	}
}

// --- On-call schedule tests ---

func TestUpsertAndGetOnCall(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	now := time.Now().UTC()
	start := now.Add(-1 * time.Hour)
	end := now.Add(7 * time.Hour)

	oc, err := svc.UpsertOnCallSchedule(ctx, incidents.OnCallSchedule{
		TenantID:    "tenant-1",
		ServiceName: "recording",
		UserID:      "user-alice",
		StartTime:   start,
		EndTime:     end,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if oc.ScheduleID == "" {
		t.Fatal("expected schedule_id")
	}

	current, err := svc.GetCurrentOnCall(ctx, "tenant-1", "recording", now)
	if err != nil {
		t.Fatalf("get current: %v", err)
	}
	if current.UserID != "user-alice" {
		t.Errorf("expected user-alice, got %s", current.UserID)
	}
}

func TestGetOnCallNotFound(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	_, err := svc.GetCurrentOnCall(ctx, "tenant-1", "recording", time.Now())
	if err != incidents.ErrOnCallNotFound {
		t.Errorf("expected ErrOnCallNotFound, got %v", err)
	}
}

func TestListOnCallSchedules(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	now := time.Now().UTC()
	svc.UpsertOnCallSchedule(ctx, incidents.OnCallSchedule{
		TenantID: "tenant-1", ServiceName: "recording", UserID: "alice",
		StartTime: now, EndTime: now.Add(8 * time.Hour),
	})
	svc.UpsertOnCallSchedule(ctx, incidents.OnCallSchedule{
		TenantID: "tenant-1", ServiceName: "recording", UserID: "bob",
		StartTime: now.Add(8 * time.Hour), EndTime: now.Add(16 * time.Hour),
	})

	schedules, err := svc.ListOnCallSchedules(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(schedules) != 2 {
		t.Fatalf("expected 2, got %d", len(schedules))
	}
}

// --- Post-mortem tests ---

func TestCreatePostMortem(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	inc, _ := svc.CreateIncident(ctx, incidents.Incident{
		TenantID:          "tenant-1",
		AlertName:         "HighCPU",
		Severity:          incidents.SeverityCritical,
		Status:            incidents.StatusTriggered,
		Summary:           "CPU above 90%",
		AffectedComponent: "recording",
	})

	// Resolve it first so timeline has all entries.
	svc.ResolveIncident(ctx, "tenant-1", inc.IncidentID)

	pm, err := svc.CreatePostMortem(ctx, "tenant-1", inc.IncidentID)
	if err != nil {
		t.Fatalf("create pm: %v", err)
	}
	if pm.PostMortemID == "" {
		t.Fatal("expected post_mortem_id")
	}
	if pm.Title == "" {
		t.Error("expected title to be populated")
	}
	if pm.Status != incidents.PostMortemDraft {
		t.Errorf("expected draft status, got %s", pm.Status)
	}
	if pm.AffectedComponents != "recording" {
		t.Errorf("expected recording, got %s", pm.AffectedComponents)
	}

	// Timeline should be JSON array.
	var timeline []map[string]string
	if err := json.Unmarshal([]byte(pm.Timeline), &timeline); err != nil {
		t.Fatalf("timeline parse: %v", err)
	}
	if len(timeline) < 1 {
		t.Error("expected at least 1 timeline entry")
	}
}

func TestGetPostMortem(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	inc, _ := svc.CreateIncident(ctx, incidents.Incident{
		TenantID: "tenant-1", AlertName: "HighCPU", Severity: incidents.SeverityCritical,
		Status: incidents.StatusTriggered, Summary: "CPU above 90%",
	})

	pm, _ := svc.CreatePostMortem(ctx, "tenant-1", inc.IncidentID)

	got, err := svc.GetPostMortem(ctx, "tenant-1", pm.PostMortemID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.IncidentID != inc.IncidentID {
		t.Errorf("expected incident_id %s, got %s", inc.IncidentID, got.IncidentID)
	}
}

func TestGetPostMortemNotFound(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	_, err := svc.GetPostMortem(ctx, "tenant-1", "nonexistent")
	if err != incidents.ErrPostMortemNotFound {
		t.Errorf("expected ErrPostMortemNotFound, got %v", err)
	}
}

func TestUpdatePostMortem(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	inc, _ := svc.CreateIncident(ctx, incidents.Incident{
		TenantID: "tenant-1", AlertName: "HighCPU", Severity: incidents.SeverityCritical,
		Status: incidents.StatusTriggered, Summary: "CPU above 90%",
	})

	pm, _ := svc.CreatePostMortem(ctx, "tenant-1", inc.IncidentID)
	pm.RootCause = "Thread pool exhaustion from leak in recording pipeline"
	pm.Status = incidents.PostMortemInReview
	pm.ActionItems = `[{"action":"Increase thread pool limit","owner":"alice"}]`

	if err := svc.UpdatePostMortem(ctx, pm); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := svc.GetPostMortem(ctx, "tenant-1", pm.PostMortemID)
	if got.RootCause != pm.RootCause {
		t.Errorf("expected root cause to be updated")
	}
	if got.Status != incidents.PostMortemInReview {
		t.Errorf("expected in_review, got %s", got.Status)
	}
}

func TestUpdatePostMortemNotFound(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	err := svc.UpdatePostMortem(ctx, incidents.PostMortem{
		PostMortemID: "nonexistent",
		TenantID:     "tenant-1",
	})
	if err != incidents.ErrPostMortemNotFound {
		t.Errorf("expected ErrPostMortemNotFound, got %v", err)
	}
}

// --- Alertmanager webhook tests ---

func TestHandleAlertmanagerWebhookFiring(t *testing.T) {
	mock := &mockPagerDuty{}
	svc := newService(t, mock)
	ctx := context.Background()

	// Set up runbook so it gets linked.
	svc.UpsertRunbook(ctx, incidents.RunbookMapping{
		TenantID:   "tenant-1",
		AlertName:  "HighCPU",
		RunbookURL: "https://wiki.example.com/runbooks/high-cpu",
	})

	payload := incidents.AlertmanagerPayload{
		Version: "4",
		Status:  "firing",
		Alerts: []incidents.Alert{
			{
				Status: "firing",
				Labels: map[string]string{
					"alertname": "HighCPU",
					"severity":  "critical",
					"instance":  "node-1:9090",
					"job":       "recording",
				},
				Annotations: map[string]string{
					"summary": "CPU usage above 90% on node-1",
				},
				StartsAt:    time.Now().UTC(),
				Fingerprint: "abc123",
			},
		},
	}

	created, err := svc.HandleAlertmanagerWebhook(ctx, "tenant-1", payload)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("expected 1 incident, got %d", len(created))
	}
	if created[0].AlertName != "HighCPU" {
		t.Errorf("expected HighCPU, got %s", created[0].AlertName)
	}
	if created[0].RunbookURL != "https://wiki.example.com/runbooks/high-cpu" {
		t.Errorf("expected runbook URL, got %s", created[0].RunbookURL)
	}

	// PagerDuty should have received a trigger event with a runbook link.
	if len(mock.events) != 1 {
		t.Fatalf("expected 1 PD event, got %d", len(mock.events))
	}
	evt := mock.events[0]
	if evt.EventAction != "trigger" {
		t.Errorf("expected trigger, got %s", evt.EventAction)
	}
	if len(evt.Links) != 1 {
		t.Fatalf("expected 1 link (runbook), got %d", len(evt.Links))
	}
	if evt.Links[0].Href != "https://wiki.example.com/runbooks/high-cpu" {
		t.Errorf("expected runbook link, got %s", evt.Links[0].Href)
	}
}

func TestHandleAlertmanagerWebhookResolved(t *testing.T) {
	mock := &mockPagerDuty{}
	svc := newService(t, mock)
	ctx := context.Background()

	// Create a firing incident first.
	dedupKey := "tenant-1/HighCPU/abc123"
	svc.CreateIncident(ctx, incidents.Incident{
		TenantID:          "tenant-1",
		AlertName:         "HighCPU",
		Severity:          incidents.SeverityCritical,
		Status:            incidents.StatusTriggered,
		Summary:           "CPU above 90%",
		PagerDutyDedupKey: dedupKey,
	})

	payload := incidents.AlertmanagerPayload{
		Version: "4",
		Status:  "resolved",
		Alerts: []incidents.Alert{
			{
				Status: "resolved",
				Labels: map[string]string{
					"alertname": "HighCPU",
					"severity":  "critical",
					"instance":  "node-1:9090",
				},
				Annotations: map[string]string{
					"summary": "CPU usage above 90% on node-1",
				},
				Fingerprint: "abc123",
			},
		},
	}

	result, err := svc.HandleAlertmanagerWebhook(ctx, "tenant-1", payload)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	// Resolved alerts don't create new incidents.
	if len(result) != 0 {
		t.Fatalf("expected 0 new incidents, got %d", len(result))
	}

	// Original incident should be resolved.
	active, _ := svc.ListActiveIncidents(ctx, "tenant-1")
	if len(active) != 0 {
		t.Errorf("expected 0 active after resolve, got %d", len(active))
	}

	// PagerDuty should have received a resolve event.
	if len(mock.events) != 1 {
		t.Fatalf("expected 1 PD event, got %d", len(mock.events))
	}
	if mock.events[0].EventAction != "resolve" {
		t.Errorf("expected resolve, got %s", mock.events[0].EventAction)
	}
}

// --- Cross-tenant isolation ---

func TestCrossTenantIsolation(t *testing.T) {
	svc := newService(t, nil)
	ctx := context.Background()

	svc.CreateIncident(ctx, incidents.Incident{
		TenantID: "tenant-1", AlertName: "HighCPU", Severity: incidents.SeverityCritical,
		Status: incidents.StatusTriggered, Summary: "t1 issue",
	})
	svc.CreateIncident(ctx, incidents.Incident{
		TenantID: "tenant-2", AlertName: "DiskFull", Severity: incidents.SeverityError,
		Status: incidents.StatusTriggered, Summary: "t2 issue",
	})

	t1, _ := svc.ListActiveIncidents(ctx, "tenant-1")
	t2, _ := svc.ListActiveIncidents(ctx, "tenant-2")
	if len(t1) != 1 || t1[0].AlertName != "HighCPU" {
		t.Errorf("tenant-1 should only see HighCPU")
	}
	if len(t2) != 1 || t2[0].AlertName != "DiskFull" {
		t.Errorf("tenant-2 should only see DiskFull")
	}

	// Runbook isolation.
	svc.UpsertRunbook(ctx, incidents.RunbookMapping{
		TenantID: "tenant-1", AlertName: "HighCPU", RunbookURL: "https://t1.example.com",
	})
	_, err := svc.GetRunbookURL(ctx, "tenant-2", "HighCPU")
	if err != incidents.ErrRunbookNotFound {
		t.Error("tenant-2 should not see tenant-1 runbooks")
	}
}

// --- HTTP PagerDuty client test with mock server ---

func TestHTTPPagerDutyClient(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content type")
		}

		var event incidents.PagerDutyEvent
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if event.RoutingKey != "test-key" {
			t.Errorf("expected test-key, got %s", event.RoutingKey)
		}

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(incidents.PagerDutyResponse{
			Status:   "success",
			Message:  "Event processed",
			DedupKey: event.DedupKey,
		})
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := incidents.NewHTTPPagerDutyClient(server.Client(), server.URL)

	resp, err := client.SendEvent(context.Background(), incidents.PagerDutyEvent{
		RoutingKey:  "test-key",
		EventAction: "trigger",
		DedupKey:    "test-dedup-123",
		Payload: incidents.PagerDutyPayload{
			Summary:  "Test alert",
			Source:   "test-node",
			Severity: "critical",
		},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if resp.Status != "success" {
		t.Errorf("expected success, got %s", resp.Status)
	}
	if resp.DedupKey != "test-dedup-123" {
		t.Errorf("expected test-dedup-123, got %s", resp.DedupKey)
	}
}

func TestHTTPPagerDutyClientError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status":"invalid","message":"bad routing key"}`))
	}))
	defer server.Close()

	client := incidents.NewHTTPPagerDutyClient(server.Client(), server.URL)
	_, err := client.SendEvent(context.Background(), incidents.PagerDutyEvent{
		RoutingKey:  "bad-key",
		EventAction: "trigger",
		Payload: incidents.PagerDutyPayload{
			Summary:  "Test",
			Source:   "test",
			Severity: "info",
		},
	})
	if err == nil {
		t.Fatal("expected error for bad request")
	}
}
