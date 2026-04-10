// Package ingest implements the Directory-side server for the three
// DirectoryIngest RPCs: StreamCameraState, PublishSegmentIndex, and
// PublishAIEvents. The Recorder-side clients live in
// internal/recorder/directoryingest.
//
// Architecture:
//   Recorder directoryingest.Client → HTTP/NDJSON → Directory ingest.Handler → ingest.Store → SQLite
package ingest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Store persists ingest data into the Directory's SQLite database.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store backed by the given database connection.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// CameraStateRow is a single camera state update to upsert.
type CameraStateRow struct {
	CameraID           string
	RecorderID         string
	State              string
	ErrorMessage       string
	CurrentBitrateKbps int32
	CurrentFramerate   int32
	LastFrameAt        *time.Time
	ConfigVersion      int64
	ObservedAt         time.Time
}

// UpsertCameraStates upserts a batch of camera state rows in a single transaction.
func (s *Store) UpsertCameraStates(ctx context.Context, rows []CameraStateRow) error {
	if len(rows) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ingest: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO camera_states (
			camera_id, recorder_id, state, error_message,
			current_bitrate_kbps, current_framerate, last_frame_at,
			config_version, observed_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(camera_id, recorder_id) DO UPDATE SET
			state = excluded.state,
			error_message = excluded.error_message,
			current_bitrate_kbps = excluded.current_bitrate_kbps,
			current_framerate = excluded.current_framerate,
			last_frame_at = excluded.last_frame_at,
			config_version = excluded.config_version,
			observed_at = excluded.observed_at,
			updated_at = CURRENT_TIMESTAMP
	`)
	if err != nil {
		return fmt.Errorf("ingest: prepare camera_states: %w", err)
	}
	defer stmt.Close()

	for _, r := range rows {
		var lastFrame *string
		if r.LastFrameAt != nil {
			s := r.LastFrameAt.UTC().Format(time.RFC3339)
			lastFrame = &s
		}
		if _, err := stmt.ExecContext(ctx,
			r.CameraID, r.RecorderID, r.State, r.ErrorMessage,
			r.CurrentBitrateKbps, r.CurrentFramerate, lastFrame,
			r.ConfigVersion, r.ObservedAt.UTC().Format(time.RFC3339),
		); err != nil {
			return fmt.Errorf("ingest: upsert camera_state %s/%s: %w", r.CameraID, r.RecorderID, err)
		}
	}

	return tx.Commit()
}

// SegmentIndexRow is a single segment to insert.
type SegmentIndexRow struct {
	SegmentID   string
	CameraID    string
	RecorderID  string
	StartTime   time.Time
	EndTime     time.Time
	Bytes       int64
	Codec       string
	HasAudio    bool
	IsEventClip bool
	StorageTier string
	Sequence    int64
}

// InsertSegments inserts a batch of segment index rows in a single transaction.
// Duplicate segment_ids are silently ignored (INSERT OR IGNORE).
func (s *Store) InsertSegments(ctx context.Context, rows []SegmentIndexRow) error {
	if len(rows) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ingest: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO segment_index (
			segment_id, camera_id, recorder_id, start_time, end_time,
			bytes, codec, has_audio, is_event_clip, storage_tier, sequence
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("ingest: prepare segment_index: %w", err)
	}
	defer stmt.Close()

	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx,
			r.SegmentID, r.CameraID, r.RecorderID,
			r.StartTime.UTC().Format(time.RFC3339),
			r.EndTime.UTC().Format(time.RFC3339),
			r.Bytes, r.Codec, r.HasAudio, r.IsEventClip,
			r.StorageTier, r.Sequence,
		); err != nil {
			return fmt.Errorf("ingest: insert segment %s: %w", r.SegmentID, err)
		}
	}

	return tx.Commit()
}

// AIEventRow is a single AI event to insert.
type AIEventRow struct {
	EventID      string
	CameraID     string
	RecorderID   string
	Kind         string
	KindLabel    string
	ObservedAt   time.Time
	Confidence   float32
	BboxX        float32
	BboxY        float32
	BboxWidth    float32
	BboxHeight   float32
	TrackID      string
	SegmentID    string
	ThumbnailRef string
	Attributes   map[string]string
}

// InsertAIEvents inserts a batch of AI event rows in a single transaction.
// Duplicate event_ids are silently ignored.
func (s *Store) InsertAIEvents(ctx context.Context, rows []AIEventRow) error {
	if len(rows) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ingest: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO ai_events (
			event_id, camera_id, recorder_id, kind, kind_label,
			observed_at, confidence, bbox_x, bbox_y, bbox_width, bbox_height,
			track_id, segment_id, thumbnail_ref, attributes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("ingest: prepare ai_events: %w", err)
	}
	defer stmt.Close()

	for _, r := range rows {
		attrs := "{}"
		if len(r.Attributes) > 0 {
			b, _ := json.Marshal(r.Attributes)
			attrs = string(b)
		}
		if _, err := stmt.ExecContext(ctx,
			r.EventID, r.CameraID, r.RecorderID, r.Kind, r.KindLabel,
			r.ObservedAt.UTC().Format(time.RFC3339),
			r.Confidence, r.BboxX, r.BboxY, r.BboxWidth, r.BboxHeight,
			r.TrackID, r.SegmentID, r.ThumbnailRef, attrs,
		); err != nil {
			return fmt.Errorf("ingest: insert ai_event %s: %w", r.EventID, err)
		}
	}

	return tx.Commit()
}

// CameraState returns the latest state for a specific camera across all recorders.
func (s *Store) CameraState(ctx context.Context, cameraID string) ([]CameraStateRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT camera_id, recorder_id, state, error_message,
		        current_bitrate_kbps, current_framerate, last_frame_at,
		        config_version, observed_at
		 FROM camera_states WHERE camera_id = ? ORDER BY observed_at DESC`, cameraID)
	if err != nil {
		return nil, fmt.Errorf("ingest: query camera_states: %w", err)
	}
	defer rows.Close()

	var result []CameraStateRow
	for rows.Next() {
		var r CameraStateRow
		var lastFrame, observedAt sql.NullString
		if err := rows.Scan(
			&r.CameraID, &r.RecorderID, &r.State, &r.ErrorMessage,
			&r.CurrentBitrateKbps, &r.CurrentFramerate, &lastFrame,
			&r.ConfigVersion, &observedAt,
		); err != nil {
			return nil, fmt.Errorf("ingest: scan camera_state: %w", err)
		}
		if observedAt.Valid {
			t, _ := time.Parse(time.RFC3339, observedAt.String)
			r.ObservedAt = t
		}
		if lastFrame.Valid {
			t, _ := time.Parse(time.RFC3339, lastFrame.String)
			r.LastFrameAt = &t
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// AllCameraStates returns the latest state for all cameras.
func (s *Store) AllCameraStates(ctx context.Context) ([]CameraStateRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT camera_id, recorder_id, state, error_message,
		        current_bitrate_kbps, current_framerate, last_frame_at,
		        config_version, observed_at
		 FROM camera_states ORDER BY camera_id, observed_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("ingest: query all camera_states: %w", err)
	}
	defer rows.Close()

	var result []CameraStateRow
	for rows.Next() {
		var r CameraStateRow
		var lastFrame, observedAt sql.NullString
		if err := rows.Scan(
			&r.CameraID, &r.RecorderID, &r.State, &r.ErrorMessage,
			&r.CurrentBitrateKbps, &r.CurrentFramerate, &lastFrame,
			&r.ConfigVersion, &observedAt,
		); err != nil {
			return nil, fmt.Errorf("ingest: scan camera_state: %w", err)
		}
		if observedAt.Valid {
			t, _ := time.Parse(time.RFC3339, observedAt.String)
			r.ObservedAt = t
		}
		if lastFrame.Valid {
			t, _ := time.Parse(time.RFC3339, lastFrame.String)
			r.LastFrameAt = &t
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
