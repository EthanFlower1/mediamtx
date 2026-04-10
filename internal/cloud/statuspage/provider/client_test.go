package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/statuspage/provider"
)

const testPageID = "page-001"

// newTestClient creates a Client that talks to a local httptest.Server.
func newTestClient(t *testing.T, handler http.Handler) (*provider.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, err := provider.NewClient(provider.ClientConfig{
		APIKey:     "test-key",
		PageID:     testPageID,
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c, srv
}

func TestListComponents(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/pages/"+testPageID+"/components", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "OAuth test-key" {
			t.Errorf("expected auth header, got %q", got)
		}
		json.NewEncoder(w).Encode([]provider.Component{
			{ID: "comp-1", Name: "Cloud Control Plane", Status: provider.ComponentOperational},
			{ID: "comp-2", Name: "Identity", Status: provider.ComponentDegradedPerformance},
		})
	})

	c, _ := newTestClient(t, mux)
	comps, err := c.ListComponents(context.Background())
	if err != nil {
		t.Fatalf("ListComponents: %v", err)
	}
	if len(comps) != 2 {
		t.Fatalf("expected 2 components, got %d", len(comps))
	}
	if comps[0].Name != "Cloud Control Plane" {
		t.Errorf("unexpected name: %s", comps[0].Name)
	}
}

func TestCreateComponent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/pages/"+testPageID+"/components", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var body map[string]provider.Component
		json.NewDecoder(r.Body).Decode(&body)
		comp := body["component"]
		comp.ID = "comp-new"
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(comp)
	})

	c, _ := newTestClient(t, mux)
	comp, err := c.CreateComponent(context.Background(), provider.Component{
		Name:        "AI Inference",
		Description: "AI model inference pipeline",
		Status:      provider.ComponentOperational,
		Showcase:    true,
	})
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	if comp.ID != "comp-new" {
		t.Errorf("expected comp-new, got %s", comp.ID)
	}
}

func TestUpdateComponentStatus(t *testing.T) {
	var gotStatus string
	mux := http.NewServeMux()
	mux.HandleFunc("/pages/"+testPageID+"/components/comp-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		var body map[string]map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		gotStatus = body["component"]["status"]
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(provider.Component{ID: "comp-1", Status: provider.ComponentStatus(gotStatus)})
	})

	c, _ := newTestClient(t, mux)
	err := c.UpdateComponentStatus(context.Background(), "comp-1", provider.ComponentMajorOutage)
	if err != nil {
		t.Fatalf("UpdateComponentStatus: %v", err)
	}
	if gotStatus != "major_outage" {
		t.Errorf("expected major_outage, got %s", gotStatus)
	}
}

func TestCreateIncident(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/pages/"+testPageID+"/incidents", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(provider.Incident{
			ID:     "inc-1",
			Name:   "Cloud API degradation",
			Status: provider.IncidentInvestigating,
		})
	})

	c, _ := newTestClient(t, mux)
	inc, err := c.CreateIncident(context.Background(), provider.CreateIncidentRequest{
		Name:   "Cloud API degradation",
		Status: provider.IncidentInvestigating,
		Body:   "Investigating increased latency on the Cloud API.",
		ComponentIDs: []string{"comp-1"},
		Components: map[string]provider.ComponentStatus{
			"comp-1": provider.ComponentDegradedPerformance,
		},
		DeliverNotifications: true,
	})
	if err != nil {
		t.Fatalf("CreateIncident: %v", err)
	}
	if inc.ID != "inc-1" {
		t.Errorf("expected inc-1, got %s", inc.ID)
	}
}

func TestUpdateIncident(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/pages/"+testPageID+"/incidents/inc-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(provider.Incident{
			ID:     "inc-1",
			Status: provider.IncidentResolved,
		})
	})

	c, _ := newTestClient(t, mux)
	inc, err := c.UpdateIncident(context.Background(), "inc-1", provider.UpdateIncidentRequest{
		Status: provider.IncidentResolved,
		Body:   "Issue resolved. Root cause was a misconfigured load balancer.",
	})
	if err != nil {
		t.Fatalf("UpdateIncident: %v", err)
	}
	if inc.Status != provider.IncidentResolved {
		t.Errorf("expected resolved, got %s", inc.Status)
	}
}

func TestListUnresolvedIncidents(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/pages/"+testPageID+"/incidents/unresolved", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]provider.Incident{
			{ID: "inc-1", Name: "API degradation", Status: provider.IncidentMonitoring},
		})
	})

	c, _ := newTestClient(t, mux)
	incs, err := c.ListUnresolvedIncidents(context.Background())
	if err != nil {
		t.Fatalf("ListUnresolvedIncidents: %v", err)
	}
	if len(incs) != 1 {
		t.Fatalf("expected 1, got %d", len(incs))
	}
}

func TestAPIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/pages/"+testPageID+"/components", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized"}`))
	})

	c, _ := newTestClient(t, mux)
	_, err := c.ListComponents(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got: %v", err)
	}
}

func TestNewClientValidation(t *testing.T) {
	_, err := provider.NewClient(provider.ClientConfig{})
	if err == nil {
		t.Fatal("expected error for empty config")
	}

	_, err = provider.NewClient(provider.ClientConfig{APIKey: "key"})
	if err == nil {
		t.Fatal("expected error for missing PageID")
	}
}

func TestListComponentGroups(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/pages/"+testPageID+"/component-groups", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]provider.ComponentGroup{
			{ID: "grp-1", Name: "Infrastructure"},
		})
	})

	c, _ := newTestClient(t, mux)
	groups, err := c.ListComponentGroups(context.Background())
	if err != nil {
		t.Fatalf("ListComponentGroups: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != "Infrastructure" {
		t.Errorf("unexpected groups: %+v", groups)
	}
}

func TestDeleteComponent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/pages/"+testPageID+"/components/comp-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	c, _ := newTestClient(t, mux)
	if err := c.DeleteComponent(context.Background(), "comp-1"); err != nil {
		t.Fatalf("DeleteComponent: %v", err)
	}
}
