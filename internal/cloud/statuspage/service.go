package statuspage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
)

// IDGen generates a random hex ID.
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

// Service manages service health checks, incidents, and incident updates.
type Service struct {
	db    *clouddb.DB
	idGen IDGen
}

// NewService constructs a Service.
func NewService(cfg Config) (*Service, error) {
	if cfg.DB == nil {
		return nil, errors.New("statuspage: DB is required")
	}
	idGen := cfg.IDGen
	if idGen == nil {
		idGen = defaultIDGen
	}
	return &Service{db: cfg.DB, idGen: idGen}, nil
}

// UpsertHealthCheck creates or updates a service health check for a tenant.
func (s *Service) UpsertHealthCheck(ctx context.Context, hc HealthCheck) (HealthCheck, error) {
	now := time.Now().UTC()
	if hc.CheckID == "" {
		hc.CheckID = s.idGen()
	}
	hc.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO service_health_checks (check_id, tenant_id, service_name, display_name, status, last_checked_at, metadata, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (check_id, tenant_id) DO UPDATE SET
			service_name = excluded.service_name,
			display_name = excluded.display_name,
			status = excluded.status,
			last_checked_at = excluded.last_checked_at,
			metadata = excluded.metadata,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at`,
		hc.CheckID, hc.TenantID, hc.ServiceName, hc.DisplayName, hc.Status,
		hc.LastCheckedAt, hc.Metadata, hc.Enabled, now, now)
	if err != nil {
		return HealthCheck{}, fmt.Errorf("upsert health check: %w", err)
	}
	hc.CreatedAt = now
	return hc, nil
}

// ListHealthChecks returns all enabled health checks for a tenant.
func (s *Service) ListHealthChecks(ctx context.Context, tenantID string) ([]HealthCheck, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT check_id, tenant_id, service_name, display_name, status, last_checked_at, metadata, enabled, created_at, updated_at
		FROM service_health_checks
		WHERE tenant_id = ? AND enabled = 1
		ORDER BY service_name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list health checks: %w", err)
	}
	defer rows.Close()

	var out []HealthCheck
	for rows.Next() {
		var hc HealthCheck
		if err := rows.Scan(&hc.CheckID, &hc.TenantID, &hc.ServiceName, &hc.DisplayName,
			&hc.Status, &hc.LastCheckedAt, &hc.Metadata, &hc.Enabled, &hc.CreatedAt, &hc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan health check: %w", err)
		}
		out = append(out, hc)
	}
	return out, rows.Err()
}

// UpdateServiceStatus updates the status of a specific health check.
func (s *Service) UpdateServiceStatus(ctx context.Context, tenantID, checkID string, status ServiceStatus) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE service_health_checks SET status = ?, last_checked_at = ?, updated_at = ?
		WHERE tenant_id = ? AND check_id = ?`,
		status, now, now, tenantID, checkID)
	if err != nil {
		return fmt.Errorf("update service status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCheckNotFound
	}
	return nil
}

// DeleteHealthCheck removes a health check.
func (s *Service) DeleteHealthCheck(ctx context.Context, tenantID, checkID string) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM service_health_checks WHERE tenant_id = ? AND check_id = ?`,
		tenantID, checkID)
	if err != nil {
		return fmt.Errorf("delete health check: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCheckNotFound
	}
	return nil
}

// CreateIncident creates a new incident for a tenant.
func (s *Service) CreateIncident(ctx context.Context, inc Incident) (Incident, error) {
	now := time.Now().UTC()
	if inc.IncidentID == "" {
		inc.IncidentID = s.idGen()
	}
	if inc.StartedAt.IsZero() {
		inc.StartedAt = now
	}
	inc.CreatedAt = now
	inc.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO incidents (incident_id, tenant_id, title, severity, status, affected_services, started_at, resolved_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inc.IncidentID, inc.TenantID, inc.Title, inc.Severity, inc.Status,
		inc.AffectedServices, inc.StartedAt, inc.ResolvedAt, inc.CreatedAt, inc.UpdatedAt)
	if err != nil {
		return Incident{}, fmt.Errorf("create incident: %w", err)
	}
	return inc, nil
}

// ListActiveIncidents returns all non-resolved incidents for a tenant.
func (s *Service) ListActiveIncidents(ctx context.Context, tenantID string) ([]Incident, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT incident_id, tenant_id, title, severity, status, affected_services, started_at, resolved_at, created_at, updated_at
		FROM incidents
		WHERE tenant_id = ? AND status != 'resolved'
		ORDER BY started_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list active incidents: %w", err)
	}
	defer rows.Close()

	var out []Incident
	for rows.Next() {
		var inc Incident
		if err := rows.Scan(&inc.IncidentID, &inc.TenantID, &inc.Title, &inc.Severity, &inc.Status,
			&inc.AffectedServices, &inc.StartedAt, &inc.ResolvedAt, &inc.CreatedAt, &inc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan incident: %w", err)
		}
		out = append(out, inc)
	}
	return out, rows.Err()
}

// UpdateIncidentStatus updates an incident's status and optionally resolves it.
func (s *Service) UpdateIncidentStatus(ctx context.Context, tenantID, incidentID string, status IncidentStatus, message string) (IncidentUpdate, error) {
	now := time.Now().UTC()

	// Update the incident itself.
	var resolvedAt *time.Time
	if status == IncidentResolved {
		resolvedAt = &now
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE incidents SET status = ?, resolved_at = ?, updated_at = ?
		WHERE tenant_id = ? AND incident_id = ?`,
		status, resolvedAt, now, tenantID, incidentID)
	if err != nil {
		return IncidentUpdate{}, fmt.Errorf("update incident status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return IncidentUpdate{}, ErrIncidentNotFound
	}

	// Record the update.
	upd := IncidentUpdate{
		UpdateID:   s.idGen(),
		IncidentID: incidentID,
		TenantID:   tenantID,
		Status:     status,
		Message:    message,
		CreatedAt:  now,
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO incident_updates (update_id, incident_id, tenant_id, status, message, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		upd.UpdateID, upd.IncidentID, upd.TenantID, upd.Status, upd.Message, upd.CreatedAt)
	if err != nil {
		return IncidentUpdate{}, fmt.Errorf("insert incident update: %w", err)
	}
	return upd, nil
}

// ListIncidentUpdates returns all updates for an incident, oldest first.
func (s *Service) ListIncidentUpdates(ctx context.Context, tenantID, incidentID string) ([]IncidentUpdate, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT update_id, incident_id, tenant_id, status, message, created_at
		FROM incident_updates
		WHERE tenant_id = ? AND incident_id = ?
		ORDER BY created_at`, tenantID, incidentID)
	if err != nil {
		return nil, fmt.Errorf("list incident updates: %w", err)
	}
	defer rows.Close()

	var out []IncidentUpdate
	for rows.Next() {
		var u IncidentUpdate
		if err := rows.Scan(&u.UpdateID, &u.IncidentID, &u.TenantID, &u.Status, &u.Message, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan incident update: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// GetStatusSummary returns the aggregated status page for a tenant.
func (s *Service) GetStatusSummary(ctx context.Context, tenantID string) (StatusSummary, error) {
	services, err := s.ListHealthChecks(ctx, tenantID)
	if err != nil {
		return StatusSummary{}, err
	}
	incidents, err := s.ListActiveIncidents(ctx, tenantID)
	if err != nil {
		return StatusSummary{}, err
	}

	overall := StatusOperational
	for _, svc := range services {
		if statusWorseThan(svc.Status, overall) {
			overall = svc.Status
		}
	}

	return StatusSummary{
		OverallStatus:   overall,
		Services:        services,
		ActiveIncidents: incidents,
	}, nil
}

// statusWorseThan returns true if a is worse than b.
func statusWorseThan(a, b ServiceStatus) bool {
	order := map[ServiceStatus]int{
		StatusOperational: 0,
		StatusDegraded:    1,
		StatusPartialOut:  2,
		StatusMajorOut:    3,
	}
	return order[a] > order[b]
}
