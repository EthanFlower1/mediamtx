package integrations

import (
	"testing"
)

func TestDefaultRegistry_HasNineIntegrations(t *testing.T) {
	r := DefaultRegistry()
	got := len(r.List())
	if got != 9 {
		t.Fatalf("DefaultRegistry: expected 9 integrations, got %d", got)
	}
}

func TestDefaultRegistry_AllIDs(t *testing.T) {
	r := DefaultRegistry()
	want := []string{
		"bosch", "brivo", "dmp", "make_n8n", "openpath",
		"pagerduty_opsgenie", "pdk", "slack_teams", "zapier",
	}
	list := r.List()
	if len(list) != len(want) {
		t.Fatalf("expected %d integrations, got %d", len(want), len(list))
	}
	for i, info := range list {
		if info.ID != want[i] {
			t.Errorf("index %d: expected ID %q, got %q", i, want[i], info.ID)
		}
	}
}

func TestRegistry_Get(t *testing.T) {
	r := DefaultRegistry()

	info, ok := r.Get("brivo")
	if !ok {
		t.Fatal("Get(brivo) returned false")
	}
	if info.DisplayName != "Brivo" {
		t.Errorf("expected DisplayName %q, got %q", "Brivo", info.DisplayName)
	}
	if info.Category != "access_control" {
		t.Errorf("expected Category %q, got %q", "access_control", info.Category)
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("Get(nonexistent) should return false")
	}
}

func TestRegistry_ListByCategory(t *testing.T) {
	r := DefaultRegistry()

	tests := []struct {
		category string
		wantIDs  []string
	}{
		{"access_control", []string{"brivo", "openpath", "pdk"}},
		{"alarm_panel", []string{"bosch", "dmp"}},
		{"itsm", []string{"pagerduty_opsgenie"}},
		{"comms", []string{"slack_teams"}},
		{"automation", []string{"make_n8n", "zapier"}},
		{"unknown_category", nil},
	}

	for _, tc := range tests {
		t.Run(tc.category, func(t *testing.T) {
			got := r.ListByCategory(tc.category)
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("ListByCategory(%q): expected %d, got %d", tc.category, len(tc.wantIDs), len(got))
			}
			for i, info := range got {
				if info.ID != tc.wantIDs[i] {
					t.Errorf("index %d: expected %q, got %q", i, tc.wantIDs[i], info.ID)
				}
			}
		})
	}
}

func TestRegistry_RegisterAndGet_RoundTrip(t *testing.T) {
	r := NewRegistry()

	info := IntegrationInfo{
		ID:          "custom",
		DisplayName: "Custom Integration",
		Category:    "test",
		Description: "A test integration",
		Features:    []string{"feature_a", "feature_b"},
	}
	r.Register(info)

	got, ok := r.Get("custom")
	if !ok {
		t.Fatal("Get(custom) returned false after Register")
	}
	if got.ID != info.ID {
		t.Errorf("ID: expected %q, got %q", info.ID, got.ID)
	}
	if got.DisplayName != info.DisplayName {
		t.Errorf("DisplayName: expected %q, got %q", info.DisplayName, got.DisplayName)
	}
	if got.Category != info.Category {
		t.Errorf("Category: expected %q, got %q", info.Category, got.Category)
	}
	if got.Description != info.Description {
		t.Errorf("Description: expected %q, got %q", info.Description, got.Description)
	}
	if len(got.Features) != len(info.Features) {
		t.Fatalf("Features length: expected %d, got %d", len(info.Features), len(got.Features))
	}
	for i, f := range got.Features {
		if f != info.Features[i] {
			t.Errorf("Features[%d]: expected %q, got %q", i, info.Features[i], f)
		}
	}
}

func TestRegistry_RegisterOverwrites(t *testing.T) {
	r := NewRegistry()
	r.Register(IntegrationInfo{ID: "x", DisplayName: "First"})
	r.Register(IntegrationInfo{ID: "x", DisplayName: "Second"})

	got, ok := r.Get("x")
	if !ok {
		t.Fatal("Get(x) returned false")
	}
	if got.DisplayName != "Second" {
		t.Errorf("expected overwritten DisplayName %q, got %q", "Second", got.DisplayName)
	}
}

func TestRegistry_ListSorted(t *testing.T) {
	r := NewRegistry()
	r.Register(IntegrationInfo{ID: "zebra"})
	r.Register(IntegrationInfo{ID: "apple"})
	r.Register(IntegrationInfo{ID: "mango"})

	list := r.List()
	if len(list) != 3 {
		t.Fatalf("expected 3, got %d", len(list))
	}
	if list[0].ID != "apple" || list[1].ID != "mango" || list[2].ID != "zebra" {
		t.Errorf("expected sorted order [apple, mango, zebra], got [%s, %s, %s]",
			list[0].ID, list[1].ID, list[2].ID)
	}
}

func TestNewRegistry_Empty(t *testing.T) {
	r := NewRegistry()
	if len(r.List()) != 0 {
		t.Error("NewRegistry should be empty")
	}
	_, ok := r.Get("anything")
	if ok {
		t.Error("Get on empty registry should return false")
	}
}
