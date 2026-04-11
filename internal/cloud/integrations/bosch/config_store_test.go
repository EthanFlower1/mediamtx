package bosch

import (
	"encoding/json"
	"testing"
)

func TestConfigStore_SaveAndGetPanel(t *testing.T) {
	store := NewConfigStore()

	cfg := PanelConfig{
		ID:          "panel-1",
		TenantID:    "tenant-1",
		DisplayName: "Front Entrance",
		Host:        "192.168.1.100",
		Port:        7700,
		AuthCode:    "1234",
		Series:      PanelSeriesB,
		Enabled:     true,
	}

	if err := store.SavePanel(cfg); err != nil {
		t.Fatalf("SavePanel: %v", err)
	}

	got, err := store.GetPanel("tenant-1", "panel-1")
	if err != nil {
		t.Fatalf("GetPanel: %v", err)
	}

	if got.DisplayName != "Front Entrance" {
		t.Errorf("DisplayName: got %q want %q", got.DisplayName, "Front Entrance")
	}
	if got.Host != "192.168.1.100" {
		t.Errorf("Host: got %q want %q", got.Host, "192.168.1.100")
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestConfigStore_SavePanelValidation(t *testing.T) {
	store := NewConfigStore()

	tests := []struct {
		name string
		cfg  PanelConfig
	}{
		{"empty tenant", PanelConfig{ID: "p1", Host: "h"}},
		{"empty id", PanelConfig{TenantID: "t1", Host: "h"}},
		{"empty host", PanelConfig{ID: "p1", TenantID: "t1"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := store.SavePanel(tc.cfg); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestConfigStore_SavePanelUpdate(t *testing.T) {
	store := NewConfigStore()

	cfg := PanelConfig{
		ID:       "panel-1",
		TenantID: "tenant-1",
		Host:     "192.168.1.100",
		Port:     7700,
		AuthCode: "1234",
		Series:   PanelSeriesB,
		Enabled:  true,
	}

	_ = store.SavePanel(cfg)
	got1, _ := store.GetPanel("tenant-1", "panel-1")
	created := got1.CreatedAt

	// Update.
	cfg.Host = "192.168.1.200"
	_ = store.SavePanel(cfg)

	got2, _ := store.GetPanel("tenant-1", "panel-1")
	if got2.Host != "192.168.1.200" {
		t.Errorf("Host after update: got %q want %q", got2.Host, "192.168.1.200")
	}
	if got2.CreatedAt != created {
		t.Error("CreatedAt should be preserved on update")
	}
	if !got2.UpdatedAt.After(got2.CreatedAt) || got2.UpdatedAt.Equal(got2.CreatedAt) {
		// UpdatedAt might equal CreatedAt if test runs fast enough.
		// Just ensure it's set.
		if got2.UpdatedAt.IsZero() {
			t.Error("UpdatedAt should be set on update")
		}
	}
}

func TestConfigStore_DefaultPort(t *testing.T) {
	store := NewConfigStore()

	cfg := PanelConfig{
		ID:       "panel-1",
		TenantID: "tenant-1",
		Host:     "192.168.1.100",
		// Port intentionally omitted.
		AuthCode: "1234",
		Series:   PanelSeriesB,
	}

	_ = store.SavePanel(cfg)
	got, _ := store.GetPanel("tenant-1", "panel-1")
	if got.Port != DefaultPort {
		t.Errorf("Port: got %d want %d", got.Port, DefaultPort)
	}
}

func TestConfigStore_ListPanels(t *testing.T) {
	store := NewConfigStore()

	for i := 0; i < 3; i++ {
		_ = store.SavePanel(PanelConfig{
			ID:       "panel-" + string(rune('a'+i)),
			TenantID: "tenant-1",
			Host:     "192.168.1.100",
			AuthCode: "1234",
			Series:   PanelSeriesB,
		})
	}

	panels := store.ListPanels("tenant-1")
	if len(panels) != 3 {
		t.Errorf("ListPanels: got %d want 3", len(panels))
	}

	// Different tenant should be empty.
	panels2 := store.ListPanels("tenant-2")
	if len(panels2) != 0 {
		t.Errorf("ListPanels other tenant: got %d want 0", len(panels2))
	}
}

func TestConfigStore_DeletePanel(t *testing.T) {
	store := NewConfigStore()

	cfg := PanelConfig{
		ID:       "panel-1",
		TenantID: "tenant-1",
		Host:     "192.168.1.100",
		AuthCode: "1234",
		Series:   PanelSeriesB,
	}

	_ = store.SavePanel(cfg)

	// Add mappings for this panel.
	_ = store.SaveMappings("tenant-1", "panel-1", []ZoneCameraMapping{
		{ID: "m1", ZoneNumber: 1, CameraIDs: []string{"cam-a"}},
	})

	// Delete.
	if err := store.DeletePanel("tenant-1", "panel-1"); err != nil {
		t.Fatalf("DeletePanel: %v", err)
	}

	// Panel should be gone.
	_, err := store.GetPanel("tenant-1", "panel-1")
	if err != ErrPanelNotFound {
		t.Errorf("expected ErrPanelNotFound, got %v", err)
	}

	// Mappings should also be gone.
	mappings, _ := store.GetMappings("tenant-1", "panel-1")
	if len(mappings) != 0 {
		t.Errorf("mappings after delete: got %d want 0", len(mappings))
	}

	// Delete non-existent.
	err = store.DeletePanel("tenant-1", "panel-999")
	if err != ErrPanelNotFound {
		t.Errorf("expected ErrPanelNotFound, got %v", err)
	}
}

func TestConfigStore_GetPanelNotFound(t *testing.T) {
	store := NewConfigStore()

	_, err := store.GetPanel("tenant-1", "panel-1")
	if err != ErrPanelNotFound {
		t.Errorf("expected ErrPanelNotFound, got %v", err)
	}
}

func TestConfigStore_SaveAndGetMappings(t *testing.T) {
	store := NewConfigStore()

	_ = store.SavePanel(PanelConfig{
		ID:       "panel-1",
		TenantID: "tenant-1",
		Host:     "192.168.1.100",
		AuthCode: "1234",
		Series:   PanelSeriesB,
	})

	mappings := []ZoneCameraMapping{
		{
			ID:         "m1",
			ZoneNumber: 1,
			ZoneName:   "Front Door",
			CameraIDs:  []string{"cam-a", "cam-b"},
			Actions:    []Action{{Type: ActionRecord, Duration: 30}},
			Enabled:    true,
		},
		{
			ID:         "m2",
			ZoneNumber: 2,
			ZoneName:   "Back Door",
			CameraIDs:  []string{"cam-c"},
			Actions:    []Action{{Type: ActionPTZPreset, PTZPreset: "backyard"}},
			Enabled:    true,
		},
	}

	if err := store.SaveMappings("tenant-1", "panel-1", mappings); err != nil {
		t.Fatalf("SaveMappings: %v", err)
	}

	got, err := store.GetMappings("tenant-1", "panel-1")
	if err != nil {
		t.Fatalf("GetMappings: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("mappings: got %d want 2", len(got))
	}

	// Verify tenant and panel IDs are stamped.
	for _, m := range got {
		if m.TenantID != "tenant-1" {
			t.Errorf("TenantID: got %q want %q", m.TenantID, "tenant-1")
		}
		if m.PanelID != "panel-1" {
			t.Errorf("PanelID: got %q want %q", m.PanelID, "panel-1")
		}
	}
}

func TestConfigStore_SaveMappings_PanelNotFound(t *testing.T) {
	store := NewConfigStore()

	err := store.SaveMappings("tenant-1", "panel-999", nil)
	if err != ErrPanelNotFound {
		t.Errorf("expected ErrPanelNotFound, got %v", err)
	}
}

func TestConfigStore_ExportJSON(t *testing.T) {
	store := NewConfigStore()

	_ = store.SavePanel(PanelConfig{
		ID:       "panel-1",
		TenantID: "tenant-1",
		Host:     "192.168.1.100",
		AuthCode: "1234",
		Series:   PanelSeriesB,
	})

	_ = store.SaveMappings("tenant-1", "panel-1", []ZoneCameraMapping{
		{ID: "m1", ZoneNumber: 1, CameraIDs: []string{"cam-a"}, Enabled: true},
	})

	data, err := store.ExportJSON("tenant-1")
	if err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}

	// Verify it's valid JSON.
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if _, ok := parsed["panels"]; !ok {
		t.Error("missing panels key in export")
	}
	if _, ok := parsed["mappings"]; !ok {
		t.Error("missing mappings key in export")
	}
}

func TestConfigStore_TenantIsolation(t *testing.T) {
	store := NewConfigStore()

	_ = store.SavePanel(PanelConfig{
		ID:       "panel-1",
		TenantID: "tenant-1",
		Host:     "192.168.1.100",
		AuthCode: "1234",
		Series:   PanelSeriesB,
	})

	_ = store.SavePanel(PanelConfig{
		ID:       "panel-1",
		TenantID: "tenant-2",
		Host:     "10.0.0.1",
		AuthCode: "5678",
		Series:   PanelSeriesG,
	})

	// Each tenant sees only their own panel.
	p1, _ := store.GetPanel("tenant-1", "panel-1")
	p2, _ := store.GetPanel("tenant-2", "panel-1")

	if p1.Host != "192.168.1.100" {
		t.Errorf("tenant-1 host: got %q want %q", p1.Host, "192.168.1.100")
	}
	if p2.Host != "10.0.0.1" {
		t.Errorf("tenant-2 host: got %q want %q", p2.Host, "10.0.0.1")
	}

	// Cross-tenant access is impossible.
	_, err := store.GetPanel("tenant-3", "panel-1")
	if err != ErrPanelNotFound {
		t.Error("cross-tenant access should fail")
	}
}
