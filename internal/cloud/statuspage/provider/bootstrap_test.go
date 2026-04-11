package provider_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/statuspage/provider"
)

// bootstrapMockProvider tracks all create calls for bootstrap testing.
type bootstrapMockProvider struct {
	mu         sync.Mutex
	components []provider.Component
	groups     []provider.ComponentGroup
	nextCompID int
	nextGrpID  int
}

func newBootstrapMock() *bootstrapMockProvider {
	return &bootstrapMockProvider{}
}

func (m *bootstrapMockProvider) ListComponents(_ context.Context) ([]provider.Component, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]provider.Component, len(m.components))
	copy(out, m.components)
	return out, nil
}

func (m *bootstrapMockProvider) GetComponent(_ context.Context, id string) (provider.Component, error) {
	return provider.Component{ID: id}, nil
}

func (m *bootstrapMockProvider) CreateComponent(_ context.Context, c provider.Component) (provider.Component, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextCompID++
	c.ID = fmt.Sprintf("comp-%d", m.nextCompID)
	m.components = append(m.components, c)
	return c, nil
}

func (m *bootstrapMockProvider) UpdateComponent(_ context.Context, _ string, c provider.Component) (provider.Component, error) {
	return c, nil
}

func (m *bootstrapMockProvider) UpdateComponentStatus(_ context.Context, _ string, _ provider.ComponentStatus) error {
	return nil
}

func (m *bootstrapMockProvider) DeleteComponent(_ context.Context, _ string) error { return nil }

func (m *bootstrapMockProvider) ListComponentGroups(_ context.Context) ([]provider.ComponentGroup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]provider.ComponentGroup, len(m.groups))
	copy(out, m.groups)
	return out, nil
}

func (m *bootstrapMockProvider) CreateComponentGroup(_ context.Context, g provider.ComponentGroup) (provider.ComponentGroup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextGrpID++
	g.ID = fmt.Sprintf("grp-%d", m.nextGrpID)
	m.groups = append(m.groups, g)
	return g, nil
}

func (m *bootstrapMockProvider) ListUnresolvedIncidents(_ context.Context) ([]provider.Incident, error) {
	return nil, nil
}

func (m *bootstrapMockProvider) CreateIncident(_ context.Context, req provider.CreateIncidentRequest) (provider.Incident, error) {
	return provider.Incident{Name: req.Name}, nil
}

func (m *bootstrapMockProvider) UpdateIncident(_ context.Context, id string, _ provider.UpdateIncidentRequest) (provider.Incident, error) {
	return provider.Incident{ID: id}, nil
}

func TestBootstrap_CreatesAllComponents(t *testing.T) {
	mp := newBootstrapMock()
	desired := provider.DefaultComponents()

	result, err := provider.Bootstrap(context.Background(), mp, desired, nil)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	if result.Created != len(desired) {
		t.Errorf("expected %d created, got %d", len(desired), result.Created)
	}
	if result.Existing != 0 {
		t.Errorf("expected 0 existing, got %d", result.Existing)
	}

	// All components should have IDs.
	for _, d := range desired {
		if _, ok := result.ComponentsByName[d.Name]; !ok {
			t.Errorf("missing component: %s", d.Name)
		}
	}

	// Groups should be created.
	expectedGroups := map[string]bool{
		"Infrastructure":  true,
		"Applications":    true,
		"Storage":         true,
		"Streaming":       true,
		"Web Properties":  true,
	}
	for name := range expectedGroups {
		if _, ok := result.GroupsByName[name]; !ok {
			t.Errorf("missing group: %s", name)
		}
	}
}

func TestBootstrap_Idempotent(t *testing.T) {
	mp := newBootstrapMock()
	desired := provider.DefaultComponents()

	// First run: creates everything.
	r1, err := provider.Bootstrap(context.Background(), mp, desired, nil)
	if err != nil {
		t.Fatalf("first bootstrap: %v", err)
	}
	if r1.Created != len(desired) {
		t.Fatalf("expected %d created, got %d", len(desired), r1.Created)
	}

	// Second run: everything already exists.
	r2, err := provider.Bootstrap(context.Background(), mp, desired, nil)
	if err != nil {
		t.Fatalf("second bootstrap: %v", err)
	}
	if r2.Created != 0 {
		t.Errorf("expected 0 created on second run, got %d", r2.Created)
	}
	if r2.Existing != len(desired) {
		t.Errorf("expected %d existing on second run, got %d", len(desired), r2.Existing)
	}
}

func TestDefaultComponents(t *testing.T) {
	comps := provider.DefaultComponents()
	if len(comps) != 10 {
		t.Errorf("expected 10 components, got %d", len(comps))
	}

	names := make(map[string]bool)
	for _, c := range comps {
		names[c.Name] = true
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
