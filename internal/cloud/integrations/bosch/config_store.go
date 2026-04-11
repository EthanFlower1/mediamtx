package bosch

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrPanelNotFound  = errors.New("bosch: panel config not found")
	ErrMappingNotFound = errors.New("bosch: zone mapping not found")
)

// ConfigStore provides an in-memory, per-tenant configuration store for
// Bosch panel configs and zone-to-camera mappings. In production this
// backs onto the cloud database; the in-memory implementation is used
// for development and tests.
type ConfigStore struct {
	mu       sync.RWMutex
	panels   map[string]map[string]PanelConfig        // tenant -> panelID -> config
	mappings map[string]map[string][]ZoneCameraMapping // tenant -> panelID -> mappings
}

// NewConfigStore creates an empty configuration store.
func NewConfigStore() *ConfigStore {
	return &ConfigStore{
		panels:   make(map[string]map[string]PanelConfig),
		mappings: make(map[string]map[string][]ZoneCameraMapping),
	}
}

// SavePanel persists a panel configuration. If the panel ID already
// exists for the tenant, it is updated; otherwise a new entry is created.
func (s *ConfigStore) SavePanel(cfg PanelConfig) error {
	if cfg.TenantID == "" {
		return errors.New("bosch: tenant_id is required")
	}
	if cfg.ID == "" {
		return errors.New("bosch: panel id is required")
	}
	if cfg.Host == "" {
		return errors.New("bosch: host is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.panels[cfg.TenantID] == nil {
		s.panels[cfg.TenantID] = make(map[string]PanelConfig)
	}

	now := time.Now().UTC()
	if existing, ok := s.panels[cfg.TenantID][cfg.ID]; ok {
		cfg.CreatedAt = existing.CreatedAt
		cfg.UpdatedAt = now
	} else {
		cfg.CreatedAt = now
		cfg.UpdatedAt = now
	}

	if cfg.Port == 0 {
		cfg.Port = DefaultPort
	}

	s.panels[cfg.TenantID][cfg.ID] = cfg
	return nil
}

// GetPanel retrieves a panel configuration by tenant and panel ID.
func (s *ConfigStore) GetPanel(tenantID, panelID string) (PanelConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tenantPanels, ok := s.panels[tenantID]
	if !ok {
		return PanelConfig{}, ErrPanelNotFound
	}
	cfg, ok := tenantPanels[panelID]
	if !ok {
		return PanelConfig{}, ErrPanelNotFound
	}
	return cfg, nil
}

// ListPanels returns all panel configurations for a tenant.
func (s *ConfigStore) ListPanels(tenantID string) []PanelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tenantPanels := s.panels[tenantID]
	result := make([]PanelConfig, 0, len(tenantPanels))
	for _, cfg := range tenantPanels {
		result = append(result, cfg)
	}
	return result
}

// DeletePanel removes a panel configuration and all its zone mappings.
func (s *ConfigStore) DeletePanel(tenantID, panelID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tenantPanels, ok := s.panels[tenantID]
	if !ok {
		return ErrPanelNotFound
	}
	if _, ok := tenantPanels[panelID]; !ok {
		return ErrPanelNotFound
	}

	delete(tenantPanels, panelID)

	// Also remove associated mappings.
	if tenantMappings, ok := s.mappings[tenantID]; ok {
		delete(tenantMappings, panelID)
	}
	return nil
}

// SaveMappings replaces the zone-to-camera mappings for a panel.
func (s *ConfigStore) SaveMappings(tenantID, panelID string, mappings []ZoneCameraMapping) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Verify panel exists.
	tenantPanels, ok := s.panels[tenantID]
	if !ok {
		return ErrPanelNotFound
	}
	if _, ok := tenantPanels[panelID]; !ok {
		return ErrPanelNotFound
	}

	if s.mappings[tenantID] == nil {
		s.mappings[tenantID] = make(map[string][]ZoneCameraMapping)
	}

	// Stamp tenant and panel IDs.
	for i := range mappings {
		mappings[i].TenantID = tenantID
		mappings[i].PanelID = panelID
	}

	s.mappings[tenantID][panelID] = mappings
	return nil
}

// GetMappings returns the zone-to-camera mappings for a panel.
func (s *ConfigStore) GetMappings(tenantID, panelID string) ([]ZoneCameraMapping, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tenantMappings, ok := s.mappings[tenantID]
	if !ok {
		return nil, nil
	}
	return tenantMappings[panelID], nil
}

// ExportJSON serializes the entire store for a tenant to JSON (backup/debug).
func (s *ConfigStore) ExportJSON(tenantID string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	export := struct {
		Panels   map[string]PanelConfig        `json:"panels"`
		Mappings map[string][]ZoneCameraMapping `json:"mappings"`
	}{
		Panels:   s.panels[tenantID],
		Mappings: s.mappings[tenantID],
	}

	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("bosch: export config: %w", err)
	}
	return data, nil
}
