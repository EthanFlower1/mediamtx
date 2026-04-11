package automation_test

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/statuspage/automation"
	"github.com/bluenviron/mediamtx/internal/cloud/statuspage/provider"
)

// mockQuerier returns predetermined values for queries.
type mockQuerier struct {
	mu      sync.Mutex
	results map[string]float64
	errors  map[string]error
}

func (m *mockQuerier) Query(_ context.Context, query string) (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err, ok := m.errors[query]; ok {
		return 0, err
	}
	if v, ok := m.results[query]; ok {
		return v, nil
	}
	return 0, fmt.Errorf("no mock result for query: %s", query)
}

func (m *mockQuerier) SetResult(query string, val float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results[query] = val
}

// mockProvider tracks UpdateComponentStatus calls.
type mockProvider struct {
	mu      sync.Mutex
	updates map[string]provider.ComponentStatus
}

func newMockProvider() *mockProvider {
	return &mockProvider{updates: make(map[string]provider.ComponentStatus)}
}

func (m *mockProvider) ListComponents(_ context.Context) ([]provider.Component, error) {
	return nil, nil
}
func (m *mockProvider) GetComponent(_ context.Context, _ string) (provider.Component, error) {
	return provider.Component{}, nil
}
func (m *mockProvider) CreateComponent(_ context.Context, c provider.Component) (provider.Component, error) {
	return c, nil
}
func (m *mockProvider) UpdateComponent(_ context.Context, _ string, c provider.Component) (provider.Component, error) {
	return c, nil
}
func (m *mockProvider) UpdateComponentStatus(_ context.Context, id string, status provider.ComponentStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updates[id] = status
	return nil
}
func (m *mockProvider) DeleteComponent(_ context.Context, _ string) error { return nil }
func (m *mockProvider) ListComponentGroups(_ context.Context) ([]provider.ComponentGroup, error) {
	return nil, nil
}
func (m *mockProvider) CreateComponentGroup(_ context.Context, g provider.ComponentGroup) (provider.ComponentGroup, error) {
	return g, nil
}
func (m *mockProvider) ListUnresolvedIncidents(_ context.Context) ([]provider.Incident, error) {
	return nil, nil
}
func (m *mockProvider) CreateIncident(_ context.Context, req provider.CreateIncidentRequest) (provider.Incident, error) {
	return provider.Incident{Name: req.Name}, nil
}
func (m *mockProvider) UpdateIncident(_ context.Context, _ string, _ provider.UpdateIncidentRequest) (provider.Incident, error) {
	return provider.Incident{}, nil
}

func (m *mockProvider) LastStatus(id string) provider.ComponentStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updates[id]
}

func TestEvaluateOnce_StatusTransition(t *testing.T) {
	q := &mockQuerier{
		results: map[string]float64{
			`up{job="cloud-apiserver"}`: 1.0,
			`up{job="identity"}`:        0.0,
		},
	}
	p := newMockProvider()

	rules := []automation.ComponentRule{
		{
			ComponentID:     "comp-api",
			ComponentName:   "Cloud Control Plane",
			PrometheusQuery: `up{job="cloud-apiserver"}`,
			Thresholds: []automation.Threshold{
				{Min: 1, Max: 2, Status: provider.ComponentOperational},
				{Min: 0, Max: 1, Status: provider.ComponentMajorOutage},
			},
		},
		{
			ComponentID:     "comp-id",
			ComponentName:   "Identity",
			PrometheusQuery: `up{job="identity"}`,
			Thresholds: []automation.Threshold{
				{Min: 1, Max: 2, Status: provider.ComponentOperational},
				{Min: 0, Max: 1, Status: provider.ComponentMajorOutage},
			},
		},
	}

	svc, err := automation.NewService(automation.Config{
		Provider: p,
		Querier:  q,
		Rules:    rules,
		Interval: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	results := svc.EvaluateOnce(context.Background())

	// API should be operational.
	if results["comp-api"] != provider.ComponentOperational {
		t.Errorf("expected operational for api, got %s", results["comp-api"])
	}
	if p.LastStatus("comp-api") != provider.ComponentOperational {
		t.Errorf("expected provider to receive operational for api")
	}

	// Identity should be major outage.
	if results["comp-id"] != provider.ComponentMajorOutage {
		t.Errorf("expected major_outage for identity, got %s", results["comp-id"])
	}
	if p.LastStatus("comp-id") != provider.ComponentMajorOutage {
		t.Errorf("expected provider to receive major_outage for identity")
	}
}

func TestEvaluateOnce_NoChangeNoPush(t *testing.T) {
	q := &mockQuerier{
		results: map[string]float64{
			`up{job="api"}`: 1.0,
		},
	}
	p := newMockProvider()

	rules := []automation.ComponentRule{
		{
			ComponentID:     "comp-1",
			ComponentName:   "API",
			PrometheusQuery: `up{job="api"}`,
			Thresholds: []automation.Threshold{
				{Min: 1, Max: 2, Status: provider.ComponentOperational},
				{Min: 0, Max: 1, Status: provider.ComponentMajorOutage},
			},
		},
	}

	svc, _ := automation.NewService(automation.Config{
		Provider: p,
		Querier:  q,
		Rules:    rules,
	})

	// First evaluation: should push.
	svc.EvaluateOnce(context.Background())
	if p.LastStatus("comp-1") != provider.ComponentOperational {
		t.Fatal("expected first eval to push")
	}

	// Reset mock to detect no new call.
	p.mu.Lock()
	delete(p.updates, "comp-1")
	p.mu.Unlock()

	// Second evaluation with same value: should NOT push.
	svc.EvaluateOnce(context.Background())
	if p.LastStatus("comp-1") != "" {
		t.Error("expected no push on second eval with same status")
	}
}

func TestEvaluateOnce_QueryError(t *testing.T) {
	q := &mockQuerier{
		results: map[string]float64{},
		errors: map[string]error{
			`up{job="api"}`: fmt.Errorf("connection refused"),
		},
	}
	p := newMockProvider()

	rules := []automation.ComponentRule{
		{
			ComponentID:     "comp-1",
			ComponentName:   "API",
			PrometheusQuery: `up{job="api"}`,
			Thresholds: []automation.Threshold{
				{Min: 1, Max: 2, Status: provider.ComponentOperational},
			},
		},
	}

	svc, _ := automation.NewService(automation.Config{
		Provider: p,
		Querier:  q,
		Rules:    rules,
	})

	results := svc.EvaluateOnce(context.Background())
	if _, ok := results["comp-1"]; ok {
		t.Error("expected no result for failed query")
	}
}

func TestEvaluateOnce_NoThresholdMatch(t *testing.T) {
	q := &mockQuerier{
		results: map[string]float64{
			`up{job="api"}`: 42.0, // no threshold covers this
		},
	}
	p := newMockProvider()

	rules := []automation.ComponentRule{
		{
			ComponentID:     "comp-1",
			ComponentName:   "API",
			PrometheusQuery: `up{job="api"}`,
			Thresholds: []automation.Threshold{
				{Min: 0, Max: 2, Status: provider.ComponentOperational},
			},
		},
	}

	svc, _ := automation.NewService(automation.Config{
		Provider: p,
		Querier:  q,
		Rules:    rules,
	})

	results := svc.EvaluateOnce(context.Background())
	if _, ok := results["comp-1"]; ok {
		t.Error("expected no result when no threshold matches")
	}
}

func TestComponentRule_Evaluate(t *testing.T) {
	rule := automation.ComponentRule{
		Thresholds: []automation.Threshold{
			{Min: 0.95, Max: math.Inf(1), Status: provider.ComponentOperational},
			{Min: 0.8, Max: 0.95, Status: provider.ComponentDegradedPerformance},
			{Min: 0.5, Max: 0.8, Status: provider.ComponentPartialOutage},
			{Min: 0, Max: 0.5, Status: provider.ComponentMajorOutage},
		},
	}

	tests := []struct {
		value float64
		want  provider.ComponentStatus
	}{
		{1.0, provider.ComponentOperational},
		{0.95, provider.ComponentOperational},
		{0.9, provider.ComponentDegradedPerformance},
		{0.7, provider.ComponentPartialOutage},
		{0.3, provider.ComponentMajorOutage},
		{0.0, provider.ComponentMajorOutage},
	}

	for _, tt := range tests {
		got := rule.Evaluate(tt.value)
		if got != tt.want {
			t.Errorf("Evaluate(%v) = %s, want %s", tt.value, got, tt.want)
		}
	}
}

func TestStartStop(t *testing.T) {
	q := &mockQuerier{
		results: map[string]float64{
			`up{job="api"}`: 1.0,
		},
	}
	p := newMockProvider()

	svc, _ := automation.NewService(automation.Config{
		Provider: p,
		Querier:  q,
		Rules: []automation.ComponentRule{
			{
				ComponentID:     "comp-1",
				ComponentName:   "API",
				PrometheusQuery: `up{job="api"}`,
				Thresholds: []automation.Threshold{
					{Min: 1, Max: 2, Status: provider.ComponentOperational},
				},
			},
		},
		Interval: 50 * time.Millisecond,
	})

	go svc.Start(context.Background())

	// Wait for at least one evaluation.
	time.Sleep(100 * time.Millisecond)
	svc.Stop()

	if svc.CachedStatus("comp-1") != provider.ComponentOperational {
		t.Error("expected cached status after start/stop")
	}
}

func TestNewServiceValidation(t *testing.T) {
	_, err := automation.NewService(automation.Config{})
	if err == nil {
		t.Fatal("expected error for empty config")
	}

	_, err = automation.NewService(automation.Config{
		Provider: newMockProvider(),
	})
	if err == nil {
		t.Fatal("expected error for missing querier")
	}

	_, err = automation.NewService(automation.Config{
		Provider: newMockProvider(),
		Querier:  &mockQuerier{},
	})
	if err == nil {
		t.Fatal("expected error for missing rules")
	}
}

func TestDefaultComponentRules(t *testing.T) {
	rules := automation.DefaultComponentRules()
	if len(rules) != 10 {
		t.Errorf("expected 10 default rules, got %d", len(rules))
	}

	// Verify all required components are present.
	names := make(map[string]bool)
	for _, r := range rules {
		names[r.ComponentName] = true
	}
	required := []string{
		"Cloud Control Plane", "Identity", "Cloud Directory",
		"Integrator Portal", "AI Inference", "Recording Archive",
		"Notifications", "Cloud Relay", "Marketing Site", "Docs",
	}
	for _, name := range required {
		if !names[name] {
			t.Errorf("missing required component: %s", name)
		}
	}
}
