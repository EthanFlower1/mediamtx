package db

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// IntegrationConfig represents a configured third-party integration.
type IntegrationConfig struct {
	ID            string            `json:"id"`
	IntegrationID string            `json:"integration_id"`
	Enabled       bool              `json:"enabled"`
	Config        map[string]string `json:"config"`
	Status        string            `json:"status"`        // "connected", "disconnected", "error"
	LastTested    string            `json:"last_tested"`
	ErrorMessage  string            `json:"error_message"`
	CreatedAt     string            `json:"created_at"`
	UpdatedAt     string            `json:"updated_at"`
}

// ListIntegrationConfigs returns all configured integrations.
func (d *DB) ListIntegrationConfigs() ([]IntegrationConfig, error) {
	rows, err := d.Query(
		`SELECT id, integration_id, enabled, config, status, last_tested, error_message, created_at, updated_at
		 FROM integration_configs ORDER BY created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("list integration configs: %w", err)
	}
	defer rows.Close()

	var configs []IntegrationConfig
	for rows.Next() {
		var ic IntegrationConfig
		var enabledInt int
		var configJSON string
		if err := rows.Scan(&ic.ID, &ic.IntegrationID, &enabledInt, &configJSON,
			&ic.Status, &ic.LastTested, &ic.ErrorMessage, &ic.CreatedAt, &ic.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan integration config: %w", err)
		}
		ic.Enabled = enabledInt != 0
		ic.Config = make(map[string]string)
		_ = json.Unmarshal([]byte(configJSON), &ic.Config)
		configs = append(configs, ic)
	}
	return configs, rows.Err()
}

// GetIntegrationConfig returns a single integration config by ID.
func (d *DB) GetIntegrationConfig(id string) (*IntegrationConfig, error) {
	row := d.QueryRow(
		`SELECT id, integration_id, enabled, config, status, last_tested, error_message, created_at, updated_at
		 FROM integration_configs WHERE id = ?`, id,
	)
	var ic IntegrationConfig
	var enabledInt int
	var configJSON string
	err := row.Scan(&ic.ID, &ic.IntegrationID, &enabledInt, &configJSON,
		&ic.Status, &ic.LastTested, &ic.ErrorMessage, &ic.CreatedAt, &ic.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get integration config: %w", err)
	}
	ic.Enabled = enabledInt != 0
	ic.Config = make(map[string]string)
	_ = json.Unmarshal([]byte(configJSON), &ic.Config)
	return &ic, nil
}

// CreateIntegrationConfig creates a new integration config.
func (d *DB) CreateIntegrationConfig(integrationID string, enabled bool, config map[string]string) (*IntegrationConfig, error) {
	id := uuid.New().String()[:12]
	now := time.Now().UTC().Format(time.RFC3339)
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	enabledInt := 0
	if enabled {
		enabledInt = 1
	}

	_, err = d.Exec(
		`INSERT INTO integration_configs (id, integration_id, enabled, config, status, last_tested, error_message, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'disconnected', '', '', ?, ?)`,
		id, integrationID, enabledInt, string(configJSON), now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create integration config: %w", err)
	}
	return &IntegrationConfig{
		ID:            id,
		IntegrationID: integrationID,
		Enabled:       enabled,
		Config:        config,
		Status:        "disconnected",
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

// UpdateIntegrationConfig updates an existing integration config.
func (d *DB) UpdateIntegrationConfig(id string, enabled bool, config map[string]string) (*IntegrationConfig, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	enabledInt := 0
	if enabled {
		enabledInt = 1
	}

	res, err := d.Exec(
		`UPDATE integration_configs SET enabled = ?, config = ?, updated_at = ? WHERE id = ?`,
		enabledInt, string(configJSON), now, id,
	)
	if err != nil {
		return nil, fmt.Errorf("update integration config: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return nil, fmt.Errorf("integration config not found")
	}
	return d.GetIntegrationConfig(id)
}

// PatchIntegrationEnabled updates only the enabled field.
func (d *DB) PatchIntegrationEnabled(id string, enabled bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	res, err := d.Exec(
		`UPDATE integration_configs SET enabled = ?, updated_at = ? WHERE id = ?`,
		enabledInt, now, id,
	)
	if err != nil {
		return fmt.Errorf("patch integration enabled: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("integration config not found")
	}
	return nil
}

// UpdateIntegrationStatus sets the status and optionally the error message.
func (d *DB) UpdateIntegrationStatus(id, status, errorMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.Exec(
		`UPDATE integration_configs SET status = ?, error_message = ?, last_tested = ?, updated_at = ? WHERE id = ?`,
		status, errorMsg, now, now, id,
	)
	if err != nil {
		return fmt.Errorf("update integration status: %w", err)
	}
	return nil
}

// DeleteIntegrationConfig removes an integration config.
func (d *DB) DeleteIntegrationConfig(id string) error {
	res, err := d.Exec(`DELETE FROM integration_configs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete integration config: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("integration config not found")
	}
	return nil
}
