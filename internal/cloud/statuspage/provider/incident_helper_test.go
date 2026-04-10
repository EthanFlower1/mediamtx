package provider_test

import (
	"context"
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/statuspage/provider"
)

// mockProvider records calls to the Provider interface.
type mockProvider struct {
	lastCreateReq  provider.CreateIncidentRequest
	lastUpdateReq  provider.UpdateIncidentRequest
	lastUpdateID   string
	createCalls    int
	updateCalls    int
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
func (m *mockProvider) UpdateComponentStatus(_ context.Context, _ string, _ provider.ComponentStatus) error {
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
	m.createCalls++
	m.lastCreateReq = req
	return provider.Incident{ID: "inc-manual-1", Name: req.Name, Status: req.Status}, nil
}
func (m *mockProvider) UpdateIncident(_ context.Context, id string, req provider.UpdateIncidentRequest) (provider.Incident, error) {
	m.updateCalls++
	m.lastUpdateID = id
	m.lastUpdateReq = req
	return provider.Incident{ID: id, Status: req.Status}, nil
}

func TestCreateManualIncident(t *testing.T) {
	mp := &mockProvider{}
	helper := provider.NewManualIncidentHelper(mp)

	inc, err := helper.CreateManualIncident(context.Background(), provider.CreateManualIncidentRequest{
		Title:                "Cloud API elevated latency",
		Body:                 "Investigating elevated p99 latency on the Cloud API.",
		Impact:               provider.ImpactMajor,
		AffectedComponentIDs: []string{"comp-api", "comp-relay"},
		Notify:               true,
	})
	if err != nil {
		t.Fatalf("CreateManualIncident: %v", err)
	}
	if inc.ID != "inc-manual-1" {
		t.Errorf("unexpected ID: %s", inc.ID)
	}
	if mp.createCalls != 1 {
		t.Errorf("expected 1 create call, got %d", mp.createCalls)
	}

	// Verify the request sent to the provider.
	if mp.lastCreateReq.Name != "Cloud API elevated latency" {
		t.Errorf("unexpected name: %s", mp.lastCreateReq.Name)
	}
	if mp.lastCreateReq.Status != provider.IncidentInvestigating {
		t.Errorf("expected investigating status, got %s", mp.lastCreateReq.Status)
	}
	if mp.lastCreateReq.ImpactOverride != provider.ImpactMajor {
		t.Errorf("expected major impact, got %s", mp.lastCreateReq.ImpactOverride)
	}
	if !mp.lastCreateReq.DeliverNotifications {
		t.Error("expected notifications to be delivered")
	}
	// Default component status should be degraded_performance.
	for _, id := range []string{"comp-api", "comp-relay"} {
		if mp.lastCreateReq.Components[id] != provider.ComponentDegradedPerformance {
			t.Errorf("expected degraded_performance for %s, got %s", id, mp.lastCreateReq.Components[id])
		}
	}
}

func TestCreateManualIncident_CustomComponentStatus(t *testing.T) {
	mp := &mockProvider{}
	helper := provider.NewManualIncidentHelper(mp)

	_, err := helper.CreateManualIncident(context.Background(), provider.CreateManualIncidentRequest{
		Title:                "Complete outage",
		Body:                 "All services down.",
		Impact:               provider.ImpactCritical,
		AffectedComponentIDs: []string{"comp-api"},
		ComponentStatus:      provider.ComponentMajorOutage,
		Notify:               true,
	})
	if err != nil {
		t.Fatalf("CreateManualIncident: %v", err)
	}
	if mp.lastCreateReq.Components["comp-api"] != provider.ComponentMajorOutage {
		t.Errorf("expected major_outage, got %s", mp.lastCreateReq.Components["comp-api"])
	}
}

func TestCreateManualIncident_Validation(t *testing.T) {
	mp := &mockProvider{}
	helper := provider.NewManualIncidentHelper(mp)

	_, err := helper.CreateManualIncident(context.Background(), provider.CreateManualIncidentRequest{
		AffectedComponentIDs: []string{"comp-1"},
	})
	if err == nil {
		t.Fatal("expected error for empty title")
	}

	_, err = helper.CreateManualIncident(context.Background(), provider.CreateManualIncidentRequest{
		Title: "Something broke",
	})
	if err == nil {
		t.Fatal("expected error for no components")
	}
}

func TestResolveIncident(t *testing.T) {
	mp := &mockProvider{}
	helper := provider.NewManualIncidentHelper(mp)

	_, err := helper.ResolveIncident(context.Background(), "inc-1", "Root cause fixed, service restored.", []string{"comp-api", "comp-relay"})
	if err != nil {
		t.Fatalf("ResolveIncident: %v", err)
	}
	if mp.updateCalls != 1 {
		t.Errorf("expected 1 update call, got %d", mp.updateCalls)
	}
	if mp.lastUpdateID != "inc-1" {
		t.Errorf("expected inc-1, got %s", mp.lastUpdateID)
	}
	if mp.lastUpdateReq.Status != provider.IncidentResolved {
		t.Errorf("expected resolved, got %s", mp.lastUpdateReq.Status)
	}
	for _, id := range []string{"comp-api", "comp-relay"} {
		if mp.lastUpdateReq.Components[id] != provider.ComponentOperational {
			t.Errorf("expected operational for %s after resolve", id)
		}
	}
}

func TestPostUpdate(t *testing.T) {
	mp := &mockProvider{}
	helper := provider.NewManualIncidentHelper(mp)

	_, err := helper.PostUpdate(context.Background(), "inc-1", provider.IncidentIdentified, "Found the root cause.")
	if err != nil {
		t.Fatalf("PostUpdate: %v", err)
	}
	if mp.lastUpdateReq.Status != provider.IncidentIdentified {
		t.Errorf("expected identified, got %s", mp.lastUpdateReq.Status)
	}
	if mp.lastUpdateReq.Body != "Found the root cause." {
		t.Errorf("unexpected body: %s", mp.lastUpdateReq.Body)
	}
}
