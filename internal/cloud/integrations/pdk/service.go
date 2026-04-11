package pdk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
)

// IDGen generates a unique identifier.
type IDGen func() string

func defaultIDGen() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Config bundles dependencies for Service.
type Config struct {
	DB    *clouddb.DB
	IDGen IDGen
}

// Service manages PDK integration configs, door inventory, events, and
// door-camera mappings within the Kaivue cloud control plane.
type Service struct {
	db    *clouddb.DB
	idGen IDGen
}

// NewService constructs a Service.
func NewService(cfg Config) (*Service, error) {
	if cfg.DB == nil {
		return nil, errors.New("pdk: DB is required")
	}
	idGen := cfg.IDGen
	if idGen == nil {
		idGen = defaultIDGen
	}
	return &Service{db: cfg.DB, idGen: idGen}, nil
}

// ---------- Integration config ----------

// UpsertConfig creates or updates the PDK integration config for a tenant.
func (s *Service) UpsertConfig(ctx context.Context, cfg IntegrationConfig) (IntegrationConfig, error) {
	now := time.Now().UTC()
	if cfg.ConfigID == "" {
		cfg.ConfigID = s.idGen()
	}
	cfg.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pdk_configs
			(config_id, tenant_id, api_endpoint, client_id, client_secret,
			 panel_id, webhook_secret, enabled, status, last_sync_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id) DO UPDATE SET
			api_endpoint  = excluded.api_endpoint,
			client_id     = excluded.client_id,
			client_secret = excluded.client_secret,
			panel_id      = excluded.panel_id,
			webhook_secret = excluded.webhook_secret,
			enabled       = excluded.enabled,
			status        = excluded.status,
			last_sync_at  = excluded.last_sync_at,
			updated_at    = excluded.updated_at`,
		cfg.ConfigID, cfg.TenantID, cfg.APIEndpoint, cfg.ClientID, cfg.ClientSecret,
		cfg.PanelID, cfg.WebhookSecret, cfg.Enabled, cfg.Status,
		cfg.LastSyncAt, now, now)
	if err != nil {
		return IntegrationConfig{}, fmt.Errorf("upsert pdk config: %w", err)
	}
	cfg.CreatedAt = now
	return cfg, nil
}

// GetConfig retrieves the PDK integration config for a tenant.
func (s *Service) GetConfig(ctx context.Context, tenantID string) (*IntegrationConfig, error) {
	var cfg IntegrationConfig
	err := s.db.QueryRowContext(ctx, `
		SELECT config_id, tenant_id, api_endpoint, client_id, client_secret,
		       panel_id, webhook_secret, enabled, status, last_sync_at, created_at, updated_at
		FROM pdk_configs
		WHERE tenant_id = ?`, tenantID).Scan(
		&cfg.ConfigID, &cfg.TenantID, &cfg.APIEndpoint, &cfg.ClientID, &cfg.ClientSecret,
		&cfg.PanelID, &cfg.WebhookSecret, &cfg.Enabled, &cfg.Status,
		&cfg.LastSyncAt, &cfg.CreatedAt, &cfg.UpdatedAt)
	if err != nil {
		return nil, ErrConfigNotFound
	}
	return &cfg, nil
}

// DeleteConfig removes the PDK integration for a tenant.
func (s *Service) DeleteConfig(ctx context.Context, tenantID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM pdk_configs WHERE tenant_id = ?`, tenantID)
	if err != nil {
		return fmt.Errorf("delete pdk config: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrConfigNotFound
	}
	return nil
}

// ---------- Doors ----------

// UpsertDoor creates or updates a door record.
func (s *Service) UpsertDoor(ctx context.Context, door Door) (Door, error) {
	now := time.Now().UTC()
	if door.DoorID == "" {
		door.DoorID = s.idGen()
	}
	door.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pdk_doors
			(door_id, tenant_id, pdk_door_id, name, location, is_locked, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, pdk_door_id) DO UPDATE SET
			name       = excluded.name,
			location   = excluded.location,
			is_locked  = excluded.is_locked,
			updated_at = excluded.updated_at`,
		door.DoorID, door.TenantID, door.PDKDoorID, door.Name,
		door.Location, door.IsLocked, now, now)
	if err != nil {
		return Door{}, fmt.Errorf("upsert door: %w", err)
	}
	door.CreatedAt = now
	return door, nil
}

// ListDoors returns all doors for a tenant.
func (s *Service) ListDoors(ctx context.Context, tenantID string) ([]Door, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT door_id, tenant_id, pdk_door_id, name, location, is_locked, created_at, updated_at
		FROM pdk_doors
		WHERE tenant_id = ?
		ORDER BY name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list doors: %w", err)
	}
	defer rows.Close()

	var out []Door
	for rows.Next() {
		var d Door
		if err := rows.Scan(&d.DoorID, &d.TenantID, &d.PDKDoorID, &d.Name,
			&d.Location, &d.IsLocked, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan door: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetDoorByPDKID looks up a door by its PDK-assigned ID within a tenant.
func (s *Service) GetDoorByPDKID(ctx context.Context, tenantID, pdkDoorID string) (*Door, error) {
	var d Door
	err := s.db.QueryRowContext(ctx, `
		SELECT door_id, tenant_id, pdk_door_id, name, location, is_locked, created_at, updated_at
		FROM pdk_doors
		WHERE tenant_id = ? AND pdk_door_id = ?`, tenantID, pdkDoorID).Scan(
		&d.DoorID, &d.TenantID, &d.PDKDoorID, &d.Name,
		&d.Location, &d.IsLocked, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, ErrDoorNotFound
	}
	return &d, nil
}

// ---------- Events ----------

// InsertEvent records a door event.
func (s *Service) InsertEvent(ctx context.Context, event DoorEvent) (DoorEvent, error) {
	now := time.Now().UTC()
	if event.EventID == "" {
		event.EventID = s.idGen()
	}
	event.CreatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pdk_door_events
			(event_id, tenant_id, door_id, pdk_event_id, event_type,
			 person_name, credential, occurred_at, raw_payload, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.EventID, event.TenantID, event.DoorID, event.PDKEventID,
		event.EventType, event.PersonName, event.Credential,
		event.OccurredAt, event.RawPayload, now)
	if err != nil {
		return DoorEvent{}, fmt.Errorf("insert event: %w", err)
	}
	return event, nil
}

// ListEvents returns recent events for a tenant, ordered newest-first.
func (s *Service) ListEvents(ctx context.Context, tenantID string, limit int) ([]DoorEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT event_id, tenant_id, door_id, pdk_event_id, event_type,
		       person_name, credential, occurred_at, raw_payload, created_at
		FROM pdk_door_events
		WHERE tenant_id = ?
		ORDER BY occurred_at DESC
		LIMIT ?`, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	var out []DoorEvent
	for rows.Next() {
		var e DoorEvent
		if err := rows.Scan(&e.EventID, &e.TenantID, &e.DoorID, &e.PDKEventID,
			&e.EventType, &e.PersonName, &e.Credential,
			&e.OccurredAt, &e.RawPayload, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------- Door-Camera Mappings ----------

// UpsertMapping creates or updates a door-to-camera mapping.
func (s *Service) UpsertMapping(ctx context.Context, m DoorCameraMapping) (DoorCameraMapping, error) {
	now := time.Now().UTC()
	if m.MappingID == "" {
		m.MappingID = s.idGen()
	}
	m.CreatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pdk_door_camera_mappings
			(mapping_id, tenant_id, door_id, camera_path, pre_buffer_sec, post_buffer_sec, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, door_id, camera_path) DO UPDATE SET
			pre_buffer_sec  = excluded.pre_buffer_sec,
			post_buffer_sec = excluded.post_buffer_sec`,
		m.MappingID, m.TenantID, m.DoorID, m.CameraPath,
		m.PreBuffer, m.PostBuffer, now)
	if err != nil {
		return DoorCameraMapping{}, fmt.Errorf("upsert mapping: %w", err)
	}
	return m, nil
}

// ListMappings returns all camera mappings for a specific door.
func (s *Service) ListMappings(ctx context.Context, tenantID, doorID string) ([]DoorCameraMapping, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT mapping_id, tenant_id, door_id, camera_path, pre_buffer_sec, post_buffer_sec, created_at
		FROM pdk_door_camera_mappings
		WHERE tenant_id = ? AND door_id = ?
		ORDER BY camera_path`, tenantID, doorID)
	if err != nil {
		return nil, fmt.Errorf("list mappings: %w", err)
	}
	defer rows.Close()

	var out []DoorCameraMapping
	for rows.Next() {
		var m DoorCameraMapping
		if err := rows.Scan(&m.MappingID, &m.TenantID, &m.DoorID, &m.CameraPath,
			&m.PreBuffer, &m.PostBuffer, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan mapping: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteMapping removes a door-camera mapping.
func (s *Service) DeleteMapping(ctx context.Context, tenantID, mappingID string) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM pdk_door_camera_mappings WHERE tenant_id = ? AND mapping_id = ?`,
		tenantID, mappingID)
	if err != nil {
		return fmt.Errorf("delete mapping: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrMappingNotFound
	}
	return nil
}

// ---------- Webhook event ingestion ----------

// IngestWebhookEvent processes an inbound PDK webhook: resolves the door,
// persists the event, and creates video correlations.
func (s *Service) IngestWebhookEvent(ctx context.Context, tenantID string, payload WebhookPayload) error {
	// Resolve the door from PDK door ID.
	door, err := s.GetDoorByPDKID(ctx, tenantID, payload.DoorID)
	if err != nil {
		// If the door is unknown, auto-create a stub record.
		newDoor, uErr := s.UpsertDoor(ctx, Door{
			TenantID:  tenantID,
			PDKDoorID: payload.DoorID,
			Name:      "Door " + payload.DoorID,
		})
		if uErr != nil {
			return fmt.Errorf("auto-create door: %w", uErr)
		}
		door = &newDoor
	}

	event := DoorEvent{
		EventID:    s.idGen(),
		TenantID:   tenantID,
		DoorID:     door.DoorID,
		PDKEventID: payload.EventID,
		EventType:  EventType(payload.EventType),
		PersonName: payload.PersonName,
		Credential: payload.Credential,
		OccurredAt: payload.Timestamp,
		RawPayload: payload.Raw,
	}

	event, err = s.InsertEvent(ctx, event)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	// Correlate with video.
	correlations, err := s.CorrelateEvent(ctx, tenantID, event)
	if err != nil {
		return fmt.Errorf("correlate event: %w", err)
	}
	if len(correlations) > 0 {
		if err := s.persistCorrelations(ctx, correlations); err != nil {
			return fmt.Errorf("persist correlations: %w", err)
		}
	}

	return nil
}
