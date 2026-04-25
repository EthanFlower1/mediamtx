package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// memUpdateStore is a simple in-memory UpdateStore for tests.
type memUpdateStore struct {
	mu      sync.Mutex
	records []*UpdateRecord
	nextID  int64
}

func (s *memUpdateStore) InsertUpdateRecord(rec *UpdateRecord) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	r := *rec
	r.ID = int(s.nextID)
	s.records = append(s.records, &r)
	return s.nextID, nil
}

func (s *memUpdateStore) UpdateUpdateRecord(id int64, status string, errMsg string, rollbackAvailable bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.records {
		if int64(r.ID) == id {
			r.Status = status
			r.ErrorMessage = errMsg
			r.RollbackAvailable = rollbackAvailable
			return nil
		}
	}
	return fmt.Errorf("record %d not found", id)
}

func (s *memUpdateStore) ListUpdateHistory(limit int) ([]*UpdateRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*UpdateRecord, 0, len(s.records))
	for i := len(s.records) - 1; i >= 0 && (limit <= 0 || len(out) < limit); i-- {
		cp := *s.records[i]
		out = append(out, &cp)
	}
	return out, nil
}

func setupTestDB(t *testing.T) *memUpdateStore {
	t.Helper()
	return &memUpdateStore{}
}

func TestCheckNoUpdate(t *testing.T) {
	// Serve a fake GitHub release with the same version as current.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(githubRelease{
			TagName:     "v1.0.0",
			PublishedAt: "2025-01-01T00:00:00Z",
			Body:        "Release notes",
		})
	}))
	defer server.Close()

	d := setupTestDB(t)
	m := New(d, "v1.0.0")
	m.HTTPClient = server.Client()
	// Override the GitHub URL by replacing owner/repo to use our test server.
	m.GitHubOwner = ""
	m.GitHubRepo = ""

	// We need to override the URL construction. Instead, we'll use a custom HTTP client
	// that redirects to our test server.
	m.HTTPClient = &http.Client{
		Transport: &redirectTransport{target: server.URL},
	}

	result, err := m.Check()
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}

	if result.UpdateAvailable {
		t.Error("expected no update available when versions match")
	}
	if result.CurrentVersion != "v1.0.0" {
		t.Errorf("expected current version v1.0.0, got %s", result.CurrentVersion)
	}
}

func TestCheckUpdateAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(githubRelease{
			TagName:     "v2.0.0",
			PublishedAt: "2025-06-01T00:00:00Z",
			Body:        "New features",
			Assets: []struct {
				Name               string `json:"name"`
				BrowserDownloadURL string `json:"browser_download_url"`
				Size               int64  `json:"size"`
			}{},
		})
	}))
	defer server.Close()

	d := setupTestDB(t)
	m := New(d, "v1.0.0")
	m.HTTPClient = &http.Client{
		Transport: &redirectTransport{target: server.URL},
	}

	result, err := m.Check()
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}

	if !result.UpdateAvailable {
		t.Error("expected update available")
	}
	if result.LatestVersion != "v2.0.0" {
		t.Errorf("expected latest version v2.0.0, got %s", result.LatestVersion)
	}
	if result.Release == nil {
		t.Fatal("expected release info")
	}
	if result.Release.ReleaseNotes != "New features" {
		t.Errorf("expected release notes 'New features', got %q", result.Release.ReleaseNotes)
	}
}

func TestUpdateHistory(t *testing.T) {
	d := setupTestDB(t)

	// Insert a record.
	rec := &UpdateRecord{
		FromVersion: "v1.0.0",
		ToVersion:   "v2.0.0",
		Status:      "completed",
		StartedAt:   "2025-01-01T00:00:00Z",
		InitiatedBy: "admin",
	}
	id, err := d.InsertUpdateRecord(rec)
	if err != nil {
		t.Fatalf("InsertUpdateRecord: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}

	// Update it.
	if err := d.UpdateUpdateRecord(id, "completed", "", true); err != nil {
		t.Fatalf("UpdateUpdateRecord: %v", err)
	}

	// List.
	records, err := d.ListUpdateHistory(10)
	if err != nil {
		t.Fatalf("ListUpdateHistory: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].FromVersion != "v1.0.0" {
		t.Errorf("expected from_version v1.0.0, got %s", records[0].FromVersion)
	}
	if records[0].Status != "completed" {
		t.Errorf("expected status completed, got %s", records[0].Status)
	}
	if !records[0].RollbackAvailable {
		t.Error("expected rollback_available to be true")
	}
}

func TestApplyNoDownloadURL(t *testing.T) {
	d := setupTestDB(t)
	m := New(d, "v1.0.0")

	result, err := m.Apply(&ReleaseInfo{Version: "v2.0.0"}, "admin")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Success {
		t.Error("expected failure when no download URL")
	}
}

func TestConcurrentApplyBlocked(t *testing.T) {
	d := setupTestDB(t)
	m := New(d, "v1.0.0")

	// Simulate an in-progress update.
	m.mu.Lock()
	m.applying = true
	m.mu.Unlock()

	_, err := m.Apply(&ReleaseInfo{
		Version:     "v2.0.0",
		DownloadURL: "http://example.com/binary",
	}, "admin")
	if err == nil {
		t.Error("expected error when update already in progress")
	}

	// Cleanup.
	m.mu.Lock()
	m.applying = false
	m.mu.Unlock()
}

// redirectTransport redirects all requests to the target URL.
type redirectTransport struct {
	target string
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	newReq := req.Clone(req.Context())
	newReq.URL.Scheme = "http"
	newReq.URL.Host = t.target[len("http://"):]
	return http.DefaultTransport.RoundTrip(newReq)
}
