package bosch

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// Manager is the top-level orchestrator for Bosch panel integrations.
// It manages the lifecycle of panel clients, event ingesters, and action
// routing across all configured panels within a tenant.
type Manager struct {
	dispatcher CameraActionDispatcher
	router     *ActionRouter

	mu      sync.Mutex
	panels  map[string]*panelSession // key: panelID
}

// panelSession holds the runtime state for one connected panel.
type panelSession struct {
	config   PanelConfig
	client   *Client
	ingester *EventIngester
}

// NewManager creates a Bosch integration manager.
func NewManager(dispatcher CameraActionDispatcher) *Manager {
	router := NewActionRouter(dispatcher)
	return &Manager{
		dispatcher: dispatcher,
		router:     router,
		panels:     make(map[string]*panelSession),
	}
}

// AddPanel configures and starts a connection to a Bosch panel.
func (m *Manager) AddPanel(ctx context.Context, cfg PanelConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.panels[cfg.ID]; exists {
		return fmt.Errorf("bosch: panel %s already configured", cfg.ID)
	}

	ingester := NewEventIngester(cfg.ID, cfg.TenantID)
	ingester.OnEvent(m.router.HandleEvent)

	clientCfg := ClientConfig{
		Host:     cfg.Host,
		Port:     cfg.Port,
		AuthCode: cfg.AuthCode,
		Series:   cfg.Series,
	}

	client := NewClient(clientCfg, ingester.HandleFrame)
	client.Start(ctx)

	m.panels[cfg.ID] = &panelSession{
		config:   cfg,
		client:   client,
		ingester: ingester,
	}

	log.Printf("[bosch] panel %s (%s) added to manager", cfg.ID, cfg.DisplayName)
	return nil
}

// RemovePanel stops and removes a panel connection.
func (m *Manager) RemovePanel(panelID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.panels[panelID]
	if !ok {
		return fmt.Errorf("bosch: panel %s not found", panelID)
	}

	session.client.Stop()
	m.router.RemoveMappings(panelID)
	delete(m.panels, panelID)

	log.Printf("[bosch] panel %s removed from manager", panelID)
	return nil
}

// SetZoneMappings updates the zone-to-camera mappings for a panel.
func (m *Manager) SetZoneMappings(panelID string, mappings []ZoneCameraMapping) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.panels[panelID]; !ok {
		return fmt.Errorf("bosch: panel %s not found", panelID)
	}

	m.router.SetMappings(panelID, mappings)
	return nil
}

// PanelState returns the connection state for a panel.
func (m *Manager) PanelState(panelID string) (ConnectionState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.panels[panelID]
	if !ok {
		return "", fmt.Errorf("bosch: panel %s not found", panelID)
	}

	return session.client.State(), nil
}

// ListPanels returns the IDs and states of all managed panels.
func (m *Manager) ListPanels() map[string]ConnectionState {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make(map[string]ConnectionState, len(m.panels))
	for id, session := range m.panels {
		result[id] = session.client.State()
	}
	return result
}

// StopAll stops all panel connections. Called on shutdown.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, session := range m.panels {
		session.client.Stop()
		log.Printf("[bosch] panel %s stopped", id)
	}
	m.panels = make(map[string]*panelSession)
}
