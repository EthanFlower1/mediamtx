package pdk

import (
	"context"
	"fmt"
	"time"
)

// CorrelateEvent looks up door-camera mappings for the event's door and
// produces VideoCorrelation records for each linked camera. The caller
// (typically IngestWebhookEvent) persists the returned correlations.
func (s *Service) CorrelateEvent(ctx context.Context, tenantID string, event DoorEvent) ([]VideoCorrelation, error) {
	mappings, err := s.ListMappings(ctx, tenantID, event.DoorID)
	if err != nil {
		return nil, fmt.Errorf("list mappings for door %s: %w", event.DoorID, err)
	}
	if len(mappings) == 0 {
		return nil, nil // no cameras mapped to this door
	}

	var correlations []VideoCorrelation
	for _, m := range mappings {
		pre := time.Duration(m.PreBuffer) * time.Second
		post := time.Duration(m.PostBuffer) * time.Second
		if pre == 0 {
			pre = 10 * time.Second // default pre-buffer
		}
		if post == 0 {
			post = 30 * time.Second // default post-buffer
		}

		correlations = append(correlations, VideoCorrelation{
			CorrelationID: s.idGen(),
			TenantID:      tenantID,
			EventID:       event.EventID,
			CameraPath:    m.CameraPath,
			ClipStart:     event.OccurredAt.Add(-pre),
			ClipEnd:       event.OccurredAt.Add(post),
		})
	}
	return correlations, nil
}

// persistCorrelations saves a batch of VideoCorrelation records.
func (s *Service) persistCorrelations(ctx context.Context, correlations []VideoCorrelation) error {
	for _, c := range correlations {
		now := time.Now().UTC()
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO pdk_video_correlations
				(correlation_id, tenant_id, event_id, camera_path, clip_start, clip_end, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			c.CorrelationID, c.TenantID, c.EventID, c.CameraPath,
			c.ClipStart, c.ClipEnd, now)
		if err != nil {
			return fmt.Errorf("insert correlation %s: %w", c.CorrelationID, err)
		}
	}
	return nil
}

// GetCorrelations returns all video correlations for a given door event.
func (s *Service) GetCorrelations(ctx context.Context, tenantID, eventID string) ([]VideoCorrelation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT correlation_id, tenant_id, event_id, camera_path, clip_start, clip_end, created_at
		FROM pdk_video_correlations
		WHERE tenant_id = ? AND event_id = ?
		ORDER BY created_at`, tenantID, eventID)
	if err != nil {
		return nil, fmt.Errorf("query correlations: %w", err)
	}
	defer rows.Close()

	var out []VideoCorrelation
	for rows.Next() {
		var vc VideoCorrelation
		if err := rows.Scan(&vc.CorrelationID, &vc.TenantID, &vc.EventID,
			&vc.CameraPath, &vc.ClipStart, &vc.ClipEnd, &vc.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan correlation: %w", err)
		}
		out = append(out, vc)
	}
	return out, rows.Err()
}
