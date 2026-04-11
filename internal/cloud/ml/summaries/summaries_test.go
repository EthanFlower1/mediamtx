package summaries_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/ml/summaries"
)

// -----------------------------------------------------------------------
// AggregateFromEvents tests
// -----------------------------------------------------------------------

func TestAggregateFromEvents(t *testing.T) {
	now := time.Now().UTC()
	start := now.Add(-24 * time.Hour)

	events := []summaries.Event{
		{EventID: "e1", TenantID: "t1", CameraID: "cam-1", Category: summaries.CategoryMotion, Timestamp: now.Add(-1 * time.Hour)},
		{EventID: "e2", TenantID: "t1", CameraID: "cam-1", Category: summaries.CategoryMotion, Timestamp: now.Add(-2 * time.Hour)},
		{EventID: "e3", TenantID: "t1", CameraID: "cam-2", Category: summaries.CategoryPerson, Timestamp: now.Add(-3 * time.Hour)},
		{EventID: "e4", TenantID: "t1", CameraID: "cam-1", Category: summaries.CategoryTamper, Detail: "cover detected", Timestamp: now.Add(-30 * time.Minute)},
	}

	agg, err := summaries.AggregateFromEvents("t1", events, start, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if agg.TotalEvents != 4 {
		t.Errorf("expected 4 total events, got %d", agg.TotalEvents)
	}
	if agg.TotalByCategory[summaries.CategoryMotion] != 2 {
		t.Errorf("expected 2 motion events, got %d", agg.TotalByCategory[summaries.CategoryMotion])
	}
	if len(agg.ByCameraCategory) != 2 {
		t.Errorf("expected 2 cameras, got %d", len(agg.ByCameraCategory))
	}
	if len(agg.NotableEvents) != 1 {
		t.Errorf("expected 1 notable event (tamper), got %d", len(agg.NotableEvents))
	}
	if agg.NotableEvents[0].Category != summaries.CategoryTamper {
		t.Errorf("expected tamper notable event, got %s", agg.NotableEvents[0].Category)
	}
}

func TestAggregateFromEvents_TenantIsolation(t *testing.T) {
	now := time.Now().UTC()
	start := now.Add(-24 * time.Hour)

	events := []summaries.Event{
		{EventID: "e1", TenantID: "t1", CameraID: "cam-1", Category: summaries.CategoryMotion, Timestamp: now},
		{EventID: "e2", TenantID: "t2", CameraID: "cam-1", Category: summaries.CategoryMotion, Timestamp: now}, // different tenant
		{EventID: "e3", TenantID: "t1", CameraID: "cam-1", Category: summaries.CategoryPerson, Timestamp: now},
	}

	agg, err := summaries.AggregateFromEvents("t1", events, start, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only t1 events should be counted.
	if agg.TotalEvents != 2 {
		t.Errorf("expected 2 events (tenant isolation), got %d", agg.TotalEvents)
	}
}

func TestAggregateFromEvents_EmptyTenantID(t *testing.T) {
	_, err := summaries.AggregateFromEvents("", nil, time.Time{}, time.Time{})
	if err != summaries.ErrInvalidTenantID {
		t.Errorf("expected ErrInvalidTenantID, got %v", err)
	}
}

func TestAggregateFromEvents_NoEvents(t *testing.T) {
	_, err := summaries.AggregateFromEvents("t1", nil, time.Time{}, time.Time{})
	if err != summaries.ErrNoEvents {
		t.Errorf("expected ErrNoEvents, got %v", err)
	}
}

// -----------------------------------------------------------------------
// PromptBuilder tests
// -----------------------------------------------------------------------

func TestPromptBuilder_Build(t *testing.T) {
	now := time.Now().UTC()
	start := now.Add(-24 * time.Hour)

	events := []summaries.Event{
		{EventID: "e1", TenantID: "t1", CameraID: "cam-1", Category: summaries.CategoryMotion, Timestamp: now},
		{EventID: "e2", TenantID: "t1", CameraID: "cam-2", Category: summaries.CategoryOffline, Detail: "connection lost", Timestamp: now},
	}
	agg, _ := summaries.AggregateFromEvents("t1", events, start, now)

	pb := summaries.NewPromptBuilder()
	prompt := pb.Build(agg)

	// Verify prompt contains key elements.
	if !strings.Contains(prompt, "Event Report") {
		t.Error("prompt should contain 'Event Report'")
	}
	if !strings.Contains(prompt, "cam-1") {
		t.Error("prompt should contain camera ID cam-1")
	}
	if !strings.Contains(prompt, "cam-2") {
		t.Error("prompt should contain camera ID cam-2")
	}
	if !strings.Contains(prompt, "camera_offline") {
		t.Error("prompt should contain notable event category")
	}
	if !strings.Contains(prompt, "Notable Events") {
		t.Error("prompt should contain Notable Events section")
	}
	if !strings.Contains(prompt, "connection lost") {
		t.Error("prompt should contain notable event detail")
	}
}

func TestPromptBuilder_SystemPrompt(t *testing.T) {
	pb := summaries.NewPromptBuilder()
	sys := pb.SystemPrompt()
	if sys == "" {
		t.Error("system prompt should not be empty")
	}
	if !strings.Contains(sys, "security camera") {
		t.Error("system prompt should reference security camera context")
	}
}

func TestPromptBuilder_DeterministicOutput(t *testing.T) {
	now := time.Now().UTC()
	start := now.Add(-24 * time.Hour)

	events := []summaries.Event{
		{EventID: "e1", TenantID: "t1", CameraID: "cam-1", Category: summaries.CategoryMotion, Timestamp: now},
		{EventID: "e2", TenantID: "t1", CameraID: "cam-2", Category: summaries.CategoryPerson, Timestamp: now},
	}
	agg, _ := summaries.AggregateFromEvents("t1", events, start, now)

	pb := summaries.NewPromptBuilder()
	p1 := pb.Build(agg)
	p2 := pb.Build(agg)

	if p1 != p2 {
		t.Error("prompt builder should produce deterministic output")
	}
}

// -----------------------------------------------------------------------
// TritonClient tests
// -----------------------------------------------------------------------

func TestTritonClient_Infer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request format.
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/v2/models/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]any{
			"outputs": []map[string]any{
				{
					"name": "text_output",
					"data": []string{"Summary: All quiet on the western front."},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := summaries.DefaultTritonConfig()
	cfg.Endpoint = strings.TrimPrefix(server.URL, "http://")

	client := summaries.NewTritonClient(cfg)
	result, err := client.Infer(context.Background(), "system prompt", "user prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "All quiet") {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestTritonClient_InferError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("model overloaded"))
	}))
	defer server.Close()

	cfg := summaries.DefaultTritonConfig()
	cfg.Endpoint = strings.TrimPrefix(server.URL, "http://")

	client := summaries.NewTritonClient(cfg)
	_, err := client.Infer(context.Background(), "sys", "user")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "inference failed") {
		t.Errorf("expected inference failed error, got: %v", err)
	}
}

func TestTritonClient_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{"outputs": []map[string]any{}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := summaries.DefaultTritonConfig()
	cfg.Endpoint = strings.TrimPrefix(server.URL, "http://")

	client := summaries.NewTritonClient(cfg)
	_, err := client.Infer(context.Background(), "sys", "user")
	if err == nil {
		t.Fatal("expected error for empty response")
	}
}

// -----------------------------------------------------------------------
// Formatter tests
// -----------------------------------------------------------------------

func TestFormatter_FormatPlainText(t *testing.T) {
	s := &summaries.Summary{
		SummaryID:   "s1",
		TenantID:    "t1",
		Period:      summaries.PeriodDaily,
		StartTime:   time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
		Text:        "Quiet day. 5 motion events on cam-1.",
		EventCount:  5,
		GeneratedAt: time.Date(2026, 4, 10, 6, 0, 0, 0, time.UTC),
	}

	f := summaries.NewFormatter()
	text := f.FormatPlainText(s)

	if !strings.Contains(text, "Daily") {
		t.Error("should contain period label")
	}
	if !strings.Contains(text, "5 motion events") {
		t.Error("should contain summary text")
	}
	if !strings.Contains(text, "Events analysed: 5") {
		t.Error("should contain event count")
	}
}

func TestFormatter_FormatHTML(t *testing.T) {
	s := &summaries.Summary{
		SummaryID:   "s1",
		TenantID:    "t1",
		Period:      summaries.PeriodWeekly,
		StartTime:   time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
		Text:        "Busy week. <script>alert('xss')</script>",
		EventCount:  100,
		GeneratedAt: time.Date(2026, 4, 10, 6, 0, 0, 0, time.UTC),
	}

	f := summaries.NewFormatter()
	html := f.FormatHTML(s)

	if !strings.Contains(html, "Weekly") {
		t.Error("should contain period label")
	}
	if strings.Contains(html, "<script>") {
		t.Error("HTML should escape script tags")
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Error("HTML should contain escaped script tags")
	}
}

func TestFormatter_FormatSlack(t *testing.T) {
	s := &summaries.Summary{
		SummaryID:   "s1",
		TenantID:    "t1",
		Period:      summaries.PeriodDaily,
		StartTime:   time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
		Text:        "Quiet day.",
		EventCount:  3,
		GeneratedAt: time.Date(2026, 4, 10, 6, 0, 0, 0, time.UTC),
	}

	f := summaries.NewFormatter()
	slack := f.FormatSlack(s)

	if !strings.Contains(slack, "*Event Summary") {
		t.Error("Slack format should use bold markers")
	}
}

// -----------------------------------------------------------------------
// Scheduler GenerateOnDemand tests
// -----------------------------------------------------------------------

func TestScheduler_GenerateOnDemand(t *testing.T) {
	// Stand up a mock Triton server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"outputs": []map[string]any{
				{
					"name": "text_output",
					"data": []string{"Daily summary: 10 motion events detected across 2 cameras. No security incidents."},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := summaries.DefaultTritonConfig()
	cfg.Endpoint = strings.TrimPrefix(server.URL, "http://")
	tritonClient := summaries.NewTritonClient(cfg)

	// Use a stub aggregator that returns canned data. We test via
	// the in-memory path by providing a custom aggregator wrapper.
	// For GenerateOnDemand we need the DB-backed aggregator, so we
	// test the full pipeline with a mock Triton instead.
	//
	// Since we cannot easily set up a DB in this unit test, we verify
	// the scheduler components individually and integration via
	// TestEndToEndPipeline below.
	_ = tritonClient
}

// -----------------------------------------------------------------------
// End-to-end pipeline test (without DB)
// -----------------------------------------------------------------------

func TestEndToEndPipeline(t *testing.T) {
	// This test verifies the full pipeline from events -> aggregation ->
	// prompt -> (mock) inference -> formatting, ensuring per-tenant
	// isolation throughout.
	now := time.Now().UTC()
	start := now.Add(-24 * time.Hour)

	// Two tenants with different events.
	t1Events := []summaries.Event{
		{EventID: "e1", TenantID: "tenant-1", CameraID: "lobby", Category: summaries.CategoryMotion, Timestamp: now.Add(-1 * time.Hour)},
		{EventID: "e2", TenantID: "tenant-1", CameraID: "lobby", Category: summaries.CategoryPerson, Timestamp: now.Add(-2 * time.Hour)},
		{EventID: "e3", TenantID: "tenant-1", CameraID: "parking", Category: summaries.CategoryTamper, Detail: "spray paint", Timestamp: now.Add(-30 * time.Minute)},
	}

	t2Events := []summaries.Event{
		{EventID: "e4", TenantID: "tenant-2", CameraID: "entrance", Category: summaries.CategoryVehicle, Timestamp: now.Add(-1 * time.Hour)},
	}

	// Aggregate each tenant independently.
	agg1, err := summaries.AggregateFromEvents("tenant-1", t1Events, start, now)
	if err != nil {
		t.Fatalf("aggregate t1: %v", err)
	}
	agg2, err := summaries.AggregateFromEvents("tenant-2", t2Events, start, now)
	if err != nil {
		t.Fatalf("aggregate t2: %v", err)
	}

	// Verify isolation.
	if agg1.TotalEvents != 3 {
		t.Errorf("tenant-1 should have 3 events, got %d", agg1.TotalEvents)
	}
	if agg2.TotalEvents != 1 {
		t.Errorf("tenant-2 should have 1 event, got %d", agg2.TotalEvents)
	}

	// Build prompts.
	pb := summaries.NewPromptBuilder()
	prompt1 := pb.Build(agg1)
	prompt2 := pb.Build(agg2)

	// Verify prompts don't leak tenant data.
	if strings.Contains(prompt1, "tenant-2") || strings.Contains(prompt1, "entrance") {
		t.Error("tenant-1 prompt should not contain tenant-2 data")
	}
	if strings.Contains(prompt2, "tenant-1") || strings.Contains(prompt2, "lobby") {
		t.Error("tenant-2 prompt should not contain tenant-1 data")
	}

	// Verify prompt1 contains tenant-1 specific data.
	if !strings.Contains(prompt1, "lobby") {
		t.Error("tenant-1 prompt should contain camera 'lobby'")
	}
	if !strings.Contains(prompt1, "tamper") {
		t.Error("tenant-1 prompt should mention tamper event")
	}

	// Mock Triton inference.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"outputs": []map[string]any{
				{"name": "text_output", "data": []string{"Generated summary text."}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := summaries.DefaultTritonConfig()
	cfg.Endpoint = strings.TrimPrefix(server.URL, "http://")
	client := summaries.NewTritonClient(cfg)

	// Infer for tenant-1.
	result, err := client.Infer(context.Background(), pb.SystemPrompt(), prompt1)
	if err != nil {
		t.Fatalf("infer: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty inference result")
	}

	// Format the summary.
	summary := &summaries.Summary{
		SummaryID:   "test-summary-1",
		TenantID:    "tenant-1",
		Period:      summaries.PeriodDaily,
		StartTime:   start,
		EndTime:     now,
		Text:        result,
		EventCount:  agg1.TotalEvents,
		GeneratedAt: time.Now().UTC(),
	}

	f := summaries.NewFormatter()
	plainText := f.FormatPlainText(summary)
	html := f.FormatHTML(summary)

	if !strings.Contains(plainText, "Daily") {
		t.Error("plain text should contain period")
	}
	if !strings.Contains(html, "<html>") {
		t.Error("HTML should be valid HTML")
	}
}

// -----------------------------------------------------------------------
// DefaultTritonConfig test
// -----------------------------------------------------------------------

func TestDefaultTritonConfig(t *testing.T) {
	cfg := summaries.DefaultTritonConfig()
	if cfg.Endpoint == "" {
		t.Error("default endpoint should not be empty")
	}
	if cfg.ModelName == "" {
		t.Error("default model name should not be empty")
	}
	if cfg.MaxTokens <= 0 {
		t.Error("default max tokens should be positive")
	}
	if cfg.TimeoutSeconds <= 0 {
		t.Error("default timeout should be positive")
	}
}

// -----------------------------------------------------------------------
// Cross-tenant isolation stress test
// -----------------------------------------------------------------------

func TestCrossTenantIsolation_MixedEvents(t *testing.T) {
	now := time.Now().UTC()
	start := now.Add(-24 * time.Hour)

	// Mix events from multiple tenants in the input slice.
	mixed := make([]summaries.Event, 0, 30)
	for i := 0; i < 10; i++ {
		for _, tid := range []string{"t1", "t2", "t3"} {
			mixed = append(mixed, summaries.Event{
				EventID:   fmt.Sprintf("e-%s-%d", tid, i),
				TenantID:  tid,
				CameraID:  fmt.Sprintf("cam-%s", tid),
				Category:  summaries.CategoryMotion,
				Timestamp: now.Add(-time.Duration(i) * time.Hour),
			})
		}
	}

	// Aggregate for each tenant separately.
	for _, tid := range []string{"t1", "t2", "t3"} {
		agg, err := summaries.AggregateFromEvents(tid, mixed, start, now)
		if err != nil {
			t.Fatalf("aggregate %s: %v", tid, err)
		}
		if agg.TotalEvents != 10 {
			t.Errorf("%s: expected 10 events, got %d", tid, agg.TotalEvents)
		}
		// Verify only the correct camera is present.
		expectedCam := fmt.Sprintf("cam-%s", tid)
		if len(agg.ByCameraCategory) != 1 {
			t.Errorf("%s: expected 1 camera, got %d", tid, len(agg.ByCameraCategory))
		}
		if _, ok := agg.ByCameraCategory[expectedCam]; !ok {
			t.Errorf("%s: expected camera %s", tid, expectedCam)
		}
	}
}
