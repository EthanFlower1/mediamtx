package suppression

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
)

// DefaultClusterWindow is the default time window for grouping related events.
const DefaultClusterWindow = 5 * time.Minute

// DefaultFalsePositiveThreshold is the default ratio of dismissals to total
// events above which a camera+eventType pair is considered a false positive.
const DefaultFalsePositiveThreshold = 0.7

// DefaultFalsePositiveLookback is how far back to look for dismissals.
const DefaultFalsePositiveLookback = 7 * 24 * time.Hour // 7 days

// DefaultMinDismissals is the minimum number of dismissals before the FP
// detector kicks in.
const DefaultMinDismissals = 5

// Config bundles dependencies for Engine.
type Config struct {
	DB                     *clouddb.DB
	IDGen                  func() string
	ClusterWindow          time.Duration
	FalsePositiveThreshold float64
	FalsePositiveLookback  time.Duration
	MinDismissals          int
}

// Engine is the alert suppression engine. It evaluates incoming events against
// clustering, activity baselines, and false positive history to decide whether
// to suppress a notification.
type Engine struct {
	db                     *clouddb.DB
	idGen                  func() string
	clusterer              *Clusterer
	fpThreshold            float64
	fpLookback             time.Duration
	minDismissals          int
}

func defaultIDGen() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NewEngine constructs an Engine.
func NewEngine(cfg Config) (*Engine, error) {
	if cfg.DB == nil {
		return nil, errors.New("suppression: DB is required")
	}
	idGen := cfg.IDGen
	if idGen == nil {
		idGen = defaultIDGen
	}
	window := cfg.ClusterWindow
	if window == 0 {
		window = DefaultClusterWindow
	}
	fpThreshold := cfg.FalsePositiveThreshold
	if fpThreshold == 0 {
		fpThreshold = DefaultFalsePositiveThreshold
	}
	fpLookback := cfg.FalsePositiveLookback
	if fpLookback == 0 {
		fpLookback = DefaultFalsePositiveLookback
	}
	minDismissals := cfg.MinDismissals
	if minDismissals == 0 {
		minDismissals = DefaultMinDismissals
	}

	return &Engine{
		db:            cfg.DB,
		idGen:         idGen,
		clusterer:     NewClusterer(window, idGen),
		fpThreshold:   fpThreshold,
		fpLookback:    fpLookback,
		minDismissals: minDismissals,
	}, nil
}

// Evaluate processes an incoming event and returns a suppression decision.
// The evaluation order is:
//  1. Check sensitivity (0.0 = skip all suppression)
//  2. False positive detection
//  3. Activity baseline check
//  4. Event clustering
func (e *Engine) Evaluate(ctx context.Context, ev Event) (Decision, error) {
	sensitivity, err := e.getSensitivity(ctx, ev.TenantID, ev.CameraID)
	if err != nil {
		return Decision{}, fmt.Errorf("get sensitivity: %w", err)
	}

	// Sensitivity 0.0 means no suppression at all.
	if sensitivity == 0.0 {
		return Decision{Suppress: false}, nil
	}

	// 1. Check false positive history.
	if sensitivity >= 0.3 {
		isFP, err := e.isFalsePositive(ctx, ev, sensitivity)
		if err != nil {
			return Decision{}, fmt.Errorf("false positive check: %w", err)
		}
		if isFP {
			d := Decision{
				Suppress: true,
				Reason:   ReasonFalsePos,
			}
			if err := e.recordSuppressed(ctx, ev, d); err != nil {
				return Decision{}, err
			}
			return d, nil
		}
	}

	// 2. Check activity baseline.
	if sensitivity >= 0.4 {
		isHigh, err := e.isHighActivity(ctx, ev, sensitivity)
		if err != nil {
			return Decision{}, fmt.Errorf("activity baseline check: %w", err)
		}
		if isHigh {
			d := Decision{
				Suppress: true,
				Reason:   ReasonHighActivity,
			}
			if err := e.recordSuppressed(ctx, ev, d); err != nil {
				return Decision{}, err
			}
			return d, nil
		}
	}

	// 3. Event clustering.
	if sensitivity >= 0.2 {
		clusterID, size, isNew := e.clusterer.Add(ev)
		if !isNew {
			summary := e.clusterer.Summary(clusterID)
			d := Decision{
				Suppress:       true,
				Reason:         ReasonClustered,
				ClusterID:      clusterID,
				ClusterSize:    size,
				ClusterSummary: summary,
			}
			if err := e.recordSuppressed(ctx, ev, d); err != nil {
				return Decision{}, err
			}
			return d, nil
		}
	}

	return Decision{Suppress: false}, nil
}

// getSensitivity returns the suppression sensitivity for a camera.
// Returns 0.5 (default) if no setting exists.
func (e *Engine) getSensitivity(ctx context.Context, tenantID, cameraID string) (float64, error) {
	var sensitivity float64
	err := e.db.QueryRowContext(ctx,
		`SELECT sensitivity FROM suppression_settings WHERE tenant_id = ? AND camera_id = ?`,
		tenantID, cameraID).Scan(&sensitivity)
	if err != nil {
		// No row means use default.
		return 0.5, nil //nolint:nilerr
	}
	return sensitivity, nil
}

// SetSensitivity updates the suppression sensitivity for a camera.
func (e *Engine) SetSensitivity(ctx context.Context, tenantID, cameraID string, sensitivity float64) error {
	if sensitivity < 0 || sensitivity > 1 {
		return errors.New("suppression: sensitivity must be between 0.0 and 1.0")
	}
	now := time.Now().UTC()
	_, err := e.db.ExecContext(ctx, `
		INSERT INTO suppression_settings (tenant_id, camera_id, sensitivity, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, camera_id) DO UPDATE SET
			sensitivity = excluded.sensitivity,
			updated_at = excluded.updated_at`,
		tenantID, cameraID, sensitivity, now, now)
	if err != nil {
		return fmt.Errorf("set sensitivity: %w", err)
	}
	return nil
}

// GetSensitivity returns the suppression sensitivity for a camera.
func (e *Engine) GetSensitivity(ctx context.Context, tenantID, cameraID string) (float64, error) {
	return e.getSensitivity(ctx, tenantID, cameraID)
}

// isFalsePositive checks whether this camera+eventType combination has a high
// dismissal rate, suggesting it is a recurring false positive.
func (e *Engine) isFalsePositive(ctx context.Context, ev Event, sensitivity float64) (bool, error) {
	cutoff := ev.Timestamp.Add(-e.fpLookback)

	var count int
	err := e.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM event_dismissals
		 WHERE tenant_id = ? AND camera_id = ? AND event_type = ? AND dismissed_at > ?`,
		ev.TenantID, ev.CameraID, ev.EventType, cutoff).Scan(&count)
	if err != nil {
		return false, err
	}

	// Scale the threshold inversely with sensitivity: higher sensitivity = lower threshold.
	threshold := int(float64(e.minDismissals) * (1.1 - sensitivity))
	if threshold < 2 {
		threshold = 2
	}

	return count >= threshold, nil
}

// RecordDismissal records that a user dismissed an event without action,
// feeding the false positive detector.
func (e *Engine) RecordDismissal(ctx context.Context, tenantID, cameraID, eventType string) error {
	id := e.idGen()
	now := time.Now().UTC()
	_, err := e.db.ExecContext(ctx,
		`INSERT INTO event_dismissals (dismissal_id, tenant_id, camera_id, event_type, dismissed_at)
		 VALUES (?, ?, ?, ?, ?)`,
		id, tenantID, cameraID, eventType, now)
	if err != nil {
		return fmt.Errorf("record dismissal: %w", err)
	}
	return nil
}

// isHighActivity checks whether the current time slot has expected high
// activity for this camera, suggesting the event is expected and not urgent.
func (e *Engine) isHighActivity(ctx context.Context, ev Event, sensitivity float64) (bool, error) {
	hour := ev.Timestamp.Hour()
	dow := int(ev.Timestamp.Weekday())

	var baseline Baseline
	err := e.db.QueryRowContext(ctx,
		`SELECT avg_count, stddev_count, sample_days FROM activity_baselines
		 WHERE tenant_id = ? AND camera_id = ? AND hour_of_day = ? AND day_of_week = ? AND event_type = ?`,
		ev.TenantID, ev.CameraID, hour, dow, ev.EventType).Scan(
		&baseline.AvgCount, &baseline.StddevCount, &baseline.SampleDays)
	if err != nil {
		// No baseline data means we cannot suppress.
		return false, nil //nolint:nilerr
	}

	// Need enough sample data to be meaningful.
	if baseline.SampleDays < 3 {
		return false, nil
	}

	// Suppress if the average activity for this slot is above a threshold
	// scaled by sensitivity. A higher sensitivity means lower threshold.
	// At sensitivity=1.0, suppress if avg >= 3 events/hour.
	// At sensitivity=0.4, suppress if avg >= 10 events/hour.
	threshold := 15.0 * (1.0 - sensitivity) // ranges from 0 (sens=1) to 9 (sens=0.4)
	if threshold < 3 {
		threshold = 3
	}

	return baseline.AvgCount >= threshold, nil
}

// UpdateBaseline upserts an activity baseline entry.
func (e *Engine) UpdateBaseline(ctx context.Context, b Baseline) error {
	now := time.Now().UTC()
	_, err := e.db.ExecContext(ctx, `
		INSERT INTO activity_baselines
			(tenant_id, camera_id, hour_of_day, day_of_week, event_type, avg_count, stddev_count, sample_days, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, camera_id, hour_of_day, day_of_week, event_type) DO UPDATE SET
			avg_count = excluded.avg_count,
			stddev_count = excluded.stddev_count,
			sample_days = excluded.sample_days,
			updated_at = excluded.updated_at`,
		b.TenantID, b.CameraID, b.HourOfDay, b.DayOfWeek, b.EventType,
		b.AvgCount, b.StddevCount, b.SampleDays, now)
	if err != nil {
		return fmt.Errorf("update baseline: %w", err)
	}
	return nil
}

// recordSuppressed persists a suppressed alert record.
func (e *Engine) recordSuppressed(ctx context.Context, ev Event, d Decision) error {
	id := e.idGen()
	now := time.Now().UTC()
	_, err := e.db.ExecContext(ctx, `
		INSERT INTO suppressed_alerts
			(alert_id, tenant_id, camera_id, event_type, reason, cluster_id, cluster_size, cluster_summary, original_event, suppressed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, ev.TenantID, ev.CameraID, ev.EventType, d.Reason,
		d.ClusterID, d.ClusterSize, d.ClusterSummary, ev.Payload, now)
	if err != nil {
		return fmt.Errorf("record suppressed alert: %w", err)
	}
	return nil
}

// ListSuppressed returns suppressed alerts for a tenant, optionally filtered by camera.
func (e *Engine) ListSuppressed(ctx context.Context, tenantID, cameraID string, limit int) ([]SuppressedAlert, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `SELECT alert_id, tenant_id, camera_id, event_type, reason,
		cluster_id, cluster_size, cluster_summary, original_event, suppressed_at
		FROM suppressed_alerts WHERE tenant_id = ?`
	args := []interface{}{tenantID}

	if cameraID != "" {
		query += " AND camera_id = ?"
		args = append(args, cameraID)
	}
	query += " ORDER BY suppressed_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list suppressed: %w", err)
	}
	defer rows.Close()

	var out []SuppressedAlert
	for rows.Next() {
		var sa SuppressedAlert
		if err := rows.Scan(&sa.AlertID, &sa.TenantID, &sa.CameraID, &sa.EventType,
			&sa.Reason, &sa.ClusterID, &sa.ClusterSize, &sa.ClusterSummary,
			&sa.OriginalEvent, &sa.SuppressedAt); err != nil {
			return nil, fmt.Errorf("scan suppressed alert: %w", err)
		}
		out = append(out, sa)
	}
	return out, rows.Err()
}

// PruneClusters removes expired clusters from the in-memory clusterer.
func (e *Engine) PruneClusters(now time.Time) {
	e.clusterer.Prune(now)
}
