package incidents

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	DB             *clouddb.DB
	PagerDuty      PagerDutyClient
	IDGen          IDGen
	DefaultRouting string // default PagerDuty routing key
}

// Service manages incident lifecycle, PagerDuty integration, on-call scheduling,
// runbook mapping, and post-mortem workflow.
type Service struct {
	db             *clouddb.DB
	pd             PagerDutyClient
	idGen          IDGen
	defaultRouting string
}

// NewService constructs a Service.
func NewService(cfg Config) (*Service, error) {
	if cfg.DB == nil {
		return nil, errors.New("incidents: DB is required")
	}
	idGen := cfg.IDGen
	if idGen == nil {
		idGen = defaultIDGen
	}
	return &Service{
		db:             cfg.DB,
		pd:             cfg.PagerDuty,
		idGen:          idGen,
		defaultRouting: cfg.DefaultRouting,
	}, nil
}

// HandleAlertmanagerWebhook processes a Prometheus Alertmanager webhook payload,
// creating or resolving incidents and triggering PagerDuty events.
func (s *Service) HandleAlertmanagerWebhook(ctx context.Context, tenantID string, payload AlertmanagerPayload) ([]Incident, error) {
	var incidents []Incident

	for _, alert := range payload.Alerts {
		alertName := alert.Labels["alertname"]
		severity := mapSeverity(alert.Labels["severity"])
		summary := alert.Annotations["summary"]
		if summary == "" {
			summary = alertName
		}
		source := alert.Labels["instance"]
		component := alert.Labels["job"]
		dedupKey := fmt.Sprintf("%s/%s/%s", tenantID, alertName, alert.Fingerprint)

		// Look up runbook for this alert.
		runbookURL, _ := s.GetRunbookURL(ctx, tenantID, alertName)

		if alert.Status == "resolved" {
			// Try to resolve existing incident.
			if err := s.resolveByDedupKey(ctx, tenantID, dedupKey); err != nil && !errors.Is(err, ErrIncidentNotFound) {
				return nil, fmt.Errorf("resolve incident: %w", err)
			}
			// Send PagerDuty resolve event.
			if s.pd != nil {
				routingKey := s.getRoutingKey(tenantID)
				if routingKey != "" {
					_, _ = s.pd.SendEvent(ctx, PagerDutyEvent{
						RoutingKey:  routingKey,
						EventAction: "resolve",
						DedupKey:    dedupKey,
						Payload: PagerDutyPayload{
							Summary:  summary + " [resolved]",
							Source:   source,
							Severity: string(severity),
						},
					})
				}
			}
			continue
		}

		// Create incident.
		inc := Incident{
			IncidentID:        s.idGen(),
			TenantID:          tenantID,
			AlertName:         alertName,
			Severity:          severity,
			Status:            StatusTriggered,
			Summary:           summary,
			Source:             source,
			AffectedComponent: component,
			PagerDutyDedupKey: dedupKey,
			RunbookURL:        runbookURL,
			TriggeredAt:       alert.StartsAt,
		}

		created, err := s.CreateIncident(ctx, inc)
		if err != nil {
			return nil, fmt.Errorf("create incident from alert: %w", err)
		}

		// Send PagerDuty trigger event.
		if s.pd != nil {
			routingKey := s.getRoutingKey(tenantID)
			if routingKey != "" {
				var links []PagerDutyLink
				if runbookURL != "" {
					links = append(links, PagerDutyLink{
						Href: runbookURL,
						Text: "Runbook: " + alertName,
					})
				}
				pdResp, err := s.pd.SendEvent(ctx, PagerDutyEvent{
					RoutingKey:  routingKey,
					EventAction: "trigger",
					DedupKey:    dedupKey,
					Payload: PagerDutyPayload{
						Summary:   summary,
						Source:    source,
						Severity:  string(severity),
						Component: component,
						Class:     alertName,
						CustomDetails: map[string]string{
							"tenant_id":   tenantID,
							"incident_id": created.IncidentID,
							"alert_name":  alertName,
							"runbook_url": runbookURL,
						},
					},
					Links: links,
				})
				if err == nil {
					created.PagerDutyKey = pdResp.DedupKey
				}
			}
		}

		incidents = append(incidents, created)
	}

	return incidents, nil
}

// CreateIncident creates a new incident record.
func (s *Service) CreateIncident(ctx context.Context, inc Incident) (Incident, error) {
	now := time.Now().UTC()
	if inc.IncidentID == "" {
		inc.IncidentID = s.idGen()
	}
	if inc.TriggeredAt.IsZero() {
		inc.TriggeredAt = now
	}
	inc.CreatedAt = now
	inc.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO paging_incidents (incident_id, tenant_id, alert_name, severity, status, summary, source, affected_component, pagerduty_key, pagerduty_dedup_key, runbook_url, triggered_at, acknowledged_at, resolved_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inc.IncidentID, inc.TenantID, inc.AlertName, inc.Severity, inc.Status,
		inc.Summary, inc.Source, inc.AffectedComponent, inc.PagerDutyKey,
		inc.PagerDutyDedupKey, inc.RunbookURL, inc.TriggeredAt,
		inc.AcknowledgedAt, inc.ResolvedAt, inc.CreatedAt, inc.UpdatedAt)
	if err != nil {
		return Incident{}, fmt.Errorf("create incident: %w", err)
	}
	return inc, nil
}

// GetIncident retrieves an incident by ID for a tenant.
func (s *Service) GetIncident(ctx context.Context, tenantID, incidentID string) (Incident, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT incident_id, tenant_id, alert_name, severity, status, summary, source, affected_component, pagerduty_key, pagerduty_dedup_key, runbook_url, triggered_at, acknowledged_at, resolved_at, created_at, updated_at
		FROM paging_incidents
		WHERE tenant_id = ? AND incident_id = ?`, tenantID, incidentID)

	var inc Incident
	err := row.Scan(&inc.IncidentID, &inc.TenantID, &inc.AlertName, &inc.Severity,
		&inc.Status, &inc.Summary, &inc.Source, &inc.AffectedComponent,
		&inc.PagerDutyKey, &inc.PagerDutyDedupKey, &inc.RunbookURL,
		&inc.TriggeredAt, &inc.AcknowledgedAt, &inc.ResolvedAt,
		&inc.CreatedAt, &inc.UpdatedAt)
	if err != nil {
		return Incident{}, fmt.Errorf("get incident: %w", err)
	}
	return inc, nil
}

// ListActiveIncidents returns all non-resolved incidents for a tenant.
func (s *Service) ListActiveIncidents(ctx context.Context, tenantID string) ([]Incident, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT incident_id, tenant_id, alert_name, severity, status, summary, source, affected_component, pagerduty_key, pagerduty_dedup_key, runbook_url, triggered_at, acknowledged_at, resolved_at, created_at, updated_at
		FROM paging_incidents
		WHERE tenant_id = ? AND status != 'resolved'
		ORDER BY triggered_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list active incidents: %w", err)
	}
	defer rows.Close()

	return scanIncidents(rows)
}

// AcknowledgeIncident marks an incident as acknowledged.
func (s *Service) AcknowledgeIncident(ctx context.Context, tenantID, incidentID string) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE paging_incidents SET status = ?, acknowledged_at = ?, updated_at = ?
		WHERE tenant_id = ? AND incident_id = ? AND status = ?`,
		StatusAcknowledged, now, now, tenantID, incidentID, StatusTriggered)
	if err != nil {
		return fmt.Errorf("acknowledge incident: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrIncidentNotFound
	}

	// Send PagerDuty acknowledge event.
	if s.pd != nil {
		inc, err := s.GetIncident(ctx, tenantID, incidentID)
		if err == nil && inc.PagerDutyDedupKey != "" {
			routingKey := s.getRoutingKey(tenantID)
			if routingKey != "" {
				_, _ = s.pd.SendEvent(ctx, PagerDutyEvent{
					RoutingKey:  routingKey,
					EventAction: "acknowledge",
					DedupKey:    inc.PagerDutyDedupKey,
					Payload: PagerDutyPayload{
						Summary:  inc.Summary,
						Source:   inc.Source,
						Severity: string(inc.Severity),
					},
				})
			}
		}
	}
	return nil
}

// ResolveIncident marks an incident as resolved.
func (s *Service) ResolveIncident(ctx context.Context, tenantID, incidentID string) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE paging_incidents SET status = ?, resolved_at = ?, updated_at = ?
		WHERE tenant_id = ? AND incident_id = ? AND status != ?`,
		StatusResolved, now, now, tenantID, incidentID, StatusResolved)
	if err != nil {
		return fmt.Errorf("resolve incident: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrIncidentNotFound
	}

	// Send PagerDuty resolve event.
	if s.pd != nil {
		inc, err := s.GetIncident(ctx, tenantID, incidentID)
		if err == nil && inc.PagerDutyDedupKey != "" {
			routingKey := s.getRoutingKey(tenantID)
			if routingKey != "" {
				_, _ = s.pd.SendEvent(ctx, PagerDutyEvent{
					RoutingKey:  routingKey,
					EventAction: "resolve",
					DedupKey:    inc.PagerDutyDedupKey,
					Payload: PagerDutyPayload{
						Summary:  inc.Summary + " [resolved]",
						Source:   inc.Source,
						Severity: string(inc.Severity),
					},
				})
			}
		}
	}
	return nil
}

// resolveByDedupKey resolves an incident by its PagerDuty dedup key.
func (s *Service) resolveByDedupKey(ctx context.Context, tenantID, dedupKey string) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE paging_incidents SET status = ?, resolved_at = ?, updated_at = ?
		WHERE tenant_id = ? AND pagerduty_dedup_key = ? AND status != ?`,
		StatusResolved, now, now, tenantID, dedupKey, StatusResolved)
	if err != nil {
		return fmt.Errorf("resolve by dedup key: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrIncidentNotFound
	}
	return nil
}

// --- Runbook mapping ---

// UpsertRunbook creates or updates a runbook mapping for an alert name.
func (s *Service) UpsertRunbook(ctx context.Context, rb RunbookMapping) (RunbookMapping, error) {
	now := time.Now().UTC()
	if rb.MappingID == "" {
		rb.MappingID = s.idGen()
	}
	rb.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runbook_mappings (mapping_id, tenant_id, alert_name, runbook_url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, alert_name) DO UPDATE SET
			runbook_url = excluded.runbook_url,
			updated_at = excluded.updated_at`,
		rb.MappingID, rb.TenantID, rb.AlertName, rb.RunbookURL, now, now)
	if err != nil {
		return RunbookMapping{}, fmt.Errorf("upsert runbook: %w", err)
	}
	rb.CreatedAt = now
	return rb, nil
}

// GetRunbookURL returns the runbook URL for a given alert name and tenant.
func (s *Service) GetRunbookURL(ctx context.Context, tenantID, alertName string) (string, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT runbook_url FROM runbook_mappings
		WHERE tenant_id = ? AND alert_name = ?`, tenantID, alertName)

	var url string
	if err := row.Scan(&url); err != nil {
		return "", ErrRunbookNotFound
	}
	return url, nil
}

// ListRunbooks returns all runbook mappings for a tenant.
func (s *Service) ListRunbooks(ctx context.Context, tenantID string) ([]RunbookMapping, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT mapping_id, tenant_id, alert_name, runbook_url, created_at, updated_at
		FROM runbook_mappings
		WHERE tenant_id = ?
		ORDER BY alert_name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list runbooks: %w", err)
	}
	defer rows.Close()

	var out []RunbookMapping
	for rows.Next() {
		var rb RunbookMapping
		if err := rows.Scan(&rb.MappingID, &rb.TenantID, &rb.AlertName, &rb.RunbookURL, &rb.CreatedAt, &rb.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan runbook: %w", err)
		}
		out = append(out, rb)
	}
	return out, rows.Err()
}

// DeleteRunbook removes a runbook mapping.
func (s *Service) DeleteRunbook(ctx context.Context, tenantID, alertName string) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM runbook_mappings WHERE tenant_id = ? AND alert_name = ?`,
		tenantID, alertName)
	if err != nil {
		return fmt.Errorf("delete runbook: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrRunbookNotFound
	}
	return nil
}

// --- On-call schedule ---

// UpsertOnCallSchedule creates or updates an on-call rotation entry.
func (s *Service) UpsertOnCallSchedule(ctx context.Context, oc OnCallSchedule) (OnCallSchedule, error) {
	now := time.Now().UTC()
	if oc.ScheduleID == "" {
		oc.ScheduleID = s.idGen()
	}
	oc.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO oncall_schedules (schedule_id, tenant_id, service_name, user_id, start_time, end_time, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (schedule_id) DO UPDATE SET
			service_name = excluded.service_name,
			user_id = excluded.user_id,
			start_time = excluded.start_time,
			end_time = excluded.end_time,
			updated_at = excluded.updated_at`,
		oc.ScheduleID, oc.TenantID, oc.ServiceName, oc.UserID, oc.StartTime, oc.EndTime, now, now)
	if err != nil {
		return OnCallSchedule{}, fmt.Errorf("upsert on-call schedule: %w", err)
	}
	oc.CreatedAt = now
	return oc, nil
}

// GetCurrentOnCall returns who is currently on-call for a service.
func (s *Service) GetCurrentOnCall(ctx context.Context, tenantID, serviceName string, at time.Time) (OnCallSchedule, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT schedule_id, tenant_id, service_name, user_id, start_time, end_time, created_at, updated_at
		FROM oncall_schedules
		WHERE tenant_id = ? AND service_name = ? AND start_time <= ? AND end_time > ?
		ORDER BY start_time DESC
		LIMIT 1`, tenantID, serviceName, at, at)

	var oc OnCallSchedule
	err := row.Scan(&oc.ScheduleID, &oc.TenantID, &oc.ServiceName, &oc.UserID,
		&oc.StartTime, &oc.EndTime, &oc.CreatedAt, &oc.UpdatedAt)
	if err != nil {
		return OnCallSchedule{}, ErrOnCallNotFound
	}
	return oc, nil
}

// ListOnCallSchedules returns all on-call schedules for a tenant.
func (s *Service) ListOnCallSchedules(ctx context.Context, tenantID string) ([]OnCallSchedule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT schedule_id, tenant_id, service_name, user_id, start_time, end_time, created_at, updated_at
		FROM oncall_schedules
		WHERE tenant_id = ?
		ORDER BY start_time`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list on-call schedules: %w", err)
	}
	defer rows.Close()

	var out []OnCallSchedule
	for rows.Next() {
		var oc OnCallSchedule
		if err := rows.Scan(&oc.ScheduleID, &oc.TenantID, &oc.ServiceName, &oc.UserID,
			&oc.StartTime, &oc.EndTime, &oc.CreatedAt, &oc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan on-call schedule: %w", err)
		}
		out = append(out, oc)
	}
	return out, rows.Err()
}

// --- Post-mortem workflow ---

// CreatePostMortem generates a post-mortem template pre-populated from an incident.
func (s *Service) CreatePostMortem(ctx context.Context, tenantID, incidentID string) (PostMortem, error) {
	inc, err := s.GetIncident(ctx, tenantID, incidentID)
	if err != nil {
		return PostMortem{}, fmt.Errorf("get incident for post-mortem: %w", err)
	}

	timeline := buildTimeline(inc)

	pm := PostMortem{
		PostMortemID:       s.idGen(),
		IncidentID:         incidentID,
		TenantID:           tenantID,
		Title:              fmt.Sprintf("Post-Mortem: %s", inc.Summary),
		Status:             PostMortemDraft,
		Summary:            fmt.Sprintf("Incident triggered by alert '%s' affecting %s.", inc.AlertName, inc.AffectedComponent),
		Timeline:           timeline,
		AffectedComponents: inc.AffectedComponent,
		RootCause:          "",
		ActionItems:        "[]",
		MetricsSnapshot:    "{}",
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO post_mortems (post_mortem_id, incident_id, tenant_id, title, status, summary, timeline, affected_components, root_cause, action_items, metrics_snapshot, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pm.PostMortemID, pm.IncidentID, pm.TenantID, pm.Title, pm.Status,
		pm.Summary, pm.Timeline, pm.AffectedComponents, pm.RootCause,
		pm.ActionItems, pm.MetricsSnapshot, pm.CreatedAt, pm.UpdatedAt)
	if err != nil {
		return PostMortem{}, fmt.Errorf("create post-mortem: %w", err)
	}
	return pm, nil
}

// GetPostMortem retrieves a post-mortem by ID.
func (s *Service) GetPostMortem(ctx context.Context, tenantID, postMortemID string) (PostMortem, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT post_mortem_id, incident_id, tenant_id, title, status, summary, timeline, affected_components, root_cause, action_items, metrics_snapshot, created_at, updated_at
		FROM post_mortems
		WHERE tenant_id = ? AND post_mortem_id = ?`, tenantID, postMortemID)

	var pm PostMortem
	err := row.Scan(&pm.PostMortemID, &pm.IncidentID, &pm.TenantID, &pm.Title,
		&pm.Status, &pm.Summary, &pm.Timeline, &pm.AffectedComponents,
		&pm.RootCause, &pm.ActionItems, &pm.MetricsSnapshot,
		&pm.CreatedAt, &pm.UpdatedAt)
	if err != nil {
		return PostMortem{}, ErrPostMortemNotFound
	}
	return pm, nil
}

// UpdatePostMortem updates a post-mortem's content and status.
func (s *Service) UpdatePostMortem(ctx context.Context, pm PostMortem) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE post_mortems SET title = ?, status = ?, summary = ?, timeline = ?,
			affected_components = ?, root_cause = ?, action_items = ?,
			metrics_snapshot = ?, updated_at = ?
		WHERE tenant_id = ? AND post_mortem_id = ?`,
		pm.Title, pm.Status, pm.Summary, pm.Timeline, pm.AffectedComponents,
		pm.RootCause, pm.ActionItems, pm.MetricsSnapshot, now,
		pm.TenantID, pm.PostMortemID)
	if err != nil {
		return fmt.Errorf("update post-mortem: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrPostMortemNotFound
	}
	return nil
}

// --- helpers ---

func (s *Service) getRoutingKey(_ string) string {
	return s.defaultRouting
}

func mapSeverity(s string) Severity {
	switch strings.ToLower(s) {
	case "critical", "page":
		return SeverityCritical
	case "error", "high":
		return SeverityError
	case "warning", "warn":
		return SeverityWarning
	default:
		return SeverityInfo
	}
}

func buildTimeline(inc Incident) string {
	var entries []map[string]string
	entries = append(entries, map[string]string{
		"time":  inc.TriggeredAt.Format(time.RFC3339),
		"event": "Incident triggered: " + inc.Summary,
	})
	if inc.AcknowledgedAt != nil {
		entries = append(entries, map[string]string{
			"time":  inc.AcknowledgedAt.Format(time.RFC3339),
			"event": "Incident acknowledged",
		})
	}
	if inc.ResolvedAt != nil {
		entries = append(entries, map[string]string{
			"time":  inc.ResolvedAt.Format(time.RFC3339),
			"event": "Incident resolved",
		})
	}
	b, _ := json.Marshal(entries)
	return string(b)
}

func scanIncidents(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]Incident, error) {
	var out []Incident
	for rows.Next() {
		var inc Incident
		if err := rows.Scan(&inc.IncidentID, &inc.TenantID, &inc.AlertName, &inc.Severity,
			&inc.Status, &inc.Summary, &inc.Source, &inc.AffectedComponent,
			&inc.PagerDutyKey, &inc.PagerDutyDedupKey, &inc.RunbookURL,
			&inc.TriggeredAt, &inc.AcknowledgedAt, &inc.ResolvedAt,
			&inc.CreatedAt, &inc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan incident: %w", err)
		}
		out = append(out, inc)
	}
	return out, rows.Err()
}
