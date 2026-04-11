package reid

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
)

// Store is the data access interface for re-id tracks and sightings.
type Store interface {
	// CreateTrack inserts a new track.
	CreateTrack(ctx context.Context, input CreateTrackInput) (Track, error)

	// GetTrack retrieves a track by id within a tenant.
	GetTrack(ctx context.Context, tenantID, id string) (Track, error)

	// UpdateTrackMatch updates a track after a new match: refreshes the
	// embedding (exponential moving average), last_camera, last_seen, and
	// increments match_count.
	UpdateTrackMatch(ctx context.Context, input UpdateTrackInput) error

	// ListActiveTracks returns tracks last seen within the given time window,
	// scoped to a tenant. Optionally filtered by last_camera.
	ListActiveTracks(ctx context.Context, tenantID string, since time.Time, cameraID *string) ([]Track, error)

	// CreateSighting records a detection sighting linked to a track.
	CreateSighting(ctx context.Context, input CreateSightingInput) (Sighting, error)

	// ListSightings returns sightings for a track, ordered by seen_at desc.
	ListSightings(ctx context.Context, tenantID, trackID string, limit int) ([]Sighting, error)

	// DeleteTrack removes a track and its sightings.
	DeleteTrack(ctx context.Context, tenantID, id string) error
}

// CreateTrackInput holds the fields required to create a new track.
type CreateTrackInput struct {
	TenantID  string
	Embedding []float32
	CameraID  string
	SeenAt    time.Time
}

// UpdateTrackInput holds the fields for updating a track after a new match.
type UpdateTrackInput struct {
	TenantID     string
	TrackID      string
	Embedding    []float32
	LastCamera   string
	LastSeen     time.Time
}

// CreateSightingInput holds the fields for recording a sighting.
type CreateSightingInput struct {
	TenantID   string
	TrackID    string
	CameraID   string
	Embedding  []float32
	Confidence float64
	BBoxX      float64
	BBoxY      float64
	BBoxW      float64
	BBoxH      float64
	SeenAt     time.Time
}

// -----------------------------------------------------------------------
// SQL-backed Store
// -----------------------------------------------------------------------

type trackStore struct {
	db *clouddb.DB
}

// NewStore creates a SQL-backed Store for re-id tracks and sightings.
func NewStore(db *clouddb.DB) Store {
	return &trackStore{db: db}
}

func (s *trackStore) ph(i int) string {
	return s.db.Placeholder(i)
}

// -----------------------------------------------------------------------
// CreateTrack
// -----------------------------------------------------------------------

func (s *trackStore) CreateTrack(ctx context.Context, input CreateTrackInput) (Track, error) {
	if input.TenantID == "" {
		return Track{}, ErrInvalidTenantID
	}

	id := uuid.New().String()
	now := time.Now().UTC()
	seenAt := input.SeenAt
	if seenAt.IsZero() {
		seenAt = now
	}

	embBytes := EmbeddingToBytes(input.Embedding)
	embDim := len(input.Embedding)

	q := fmt.Sprintf(
		`INSERT INTO reid_tracks
		    (id, tenant_id, embedding, embedding_dim, first_camera, last_camera,
		     first_seen, last_seen, match_count, metadata, created_at, updated_at)
		 VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)`,
		s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6),
		s.ph(7), s.ph(8), s.ph(9), s.ph(10), s.ph(11), s.ph(12),
	)

	_, err := s.db.ExecContext(ctx, q,
		id, input.TenantID, embBytes, embDim, input.CameraID, input.CameraID,
		seenAt, seenAt, 1, "{}", now, now,
	)
	if err != nil {
		return Track{}, fmt.Errorf("reid.CreateTrack: %w", err)
	}

	return Track{
		ID:           id,
		TenantID:     input.TenantID,
		Embedding:    input.Embedding,
		EmbeddingDim: embDim,
		FirstCamera:  input.CameraID,
		LastCamera:   input.CameraID,
		FirstSeen:    seenAt,
		LastSeen:     seenAt,
		MatchCount:   1,
		Metadata:     "{}",
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// -----------------------------------------------------------------------
// GetTrack
// -----------------------------------------------------------------------

func (s *trackStore) GetTrack(ctx context.Context, tenantID, id string) (Track, error) {
	if tenantID == "" {
		return Track{}, ErrInvalidTenantID
	}
	if id == "" {
		return Track{}, ErrInvalidID
	}

	q := fmt.Sprintf(
		`SELECT id, tenant_id, embedding, embedding_dim, first_camera, last_camera,
		        first_seen, last_seen, match_count, metadata, created_at, updated_at
		 FROM reid_tracks
		 WHERE tenant_id = %s AND id = %s`,
		s.ph(1), s.ph(2),
	)
	row := s.db.QueryRowContext(ctx, q, tenantID, id)
	t, err := scanTrack(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Track{}, ErrNotFound
	}
	if err != nil {
		return Track{}, fmt.Errorf("reid.GetTrack: %w", err)
	}
	return t, nil
}

// -----------------------------------------------------------------------
// UpdateTrackMatch
// -----------------------------------------------------------------------

func (s *trackStore) UpdateTrackMatch(ctx context.Context, input UpdateTrackInput) error {
	if input.TenantID == "" {
		return ErrInvalidTenantID
	}
	if input.TrackID == "" {
		return ErrInvalidID
	}

	embBytes := EmbeddingToBytes(input.Embedding)
	now := time.Now().UTC()

	q := fmt.Sprintf(
		`UPDATE reid_tracks
		 SET embedding = %s, last_camera = %s, last_seen = %s,
		     match_count = match_count + 1, updated_at = %s
		 WHERE tenant_id = %s AND id = %s`,
		s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6),
	)

	res, err := s.db.ExecContext(ctx, q,
		embBytes, input.LastCamera, input.LastSeen, now,
		input.TenantID, input.TrackID,
	)
	if err != nil {
		return fmt.Errorf("reid.UpdateTrackMatch: %w", err)
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

// -----------------------------------------------------------------------
// ListActiveTracks
// -----------------------------------------------------------------------

func (s *trackStore) ListActiveTracks(ctx context.Context, tenantID string, since time.Time, cameraID *string) ([]Track, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}

	args := []any{tenantID, since}
	idx := 3

	where := fmt.Sprintf("tenant_id = %s AND last_seen >= %s", s.ph(1), s.ph(2))
	if cameraID != nil {
		where += fmt.Sprintf(" AND last_camera = %s", s.ph(idx))
		args = append(args, *cameraID)
		idx++
	}

	q := fmt.Sprintf(
		`SELECT id, tenant_id, embedding, embedding_dim, first_camera, last_camera,
		        first_seen, last_seen, match_count, metadata, created_at, updated_at
		 FROM reid_tracks
		 WHERE %s
		 ORDER BY last_seen DESC`,
		where,
	)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("reid.ListActiveTracks: %w", err)
	}
	defer rows.Close()
	return scanTracks(rows)
}

// -----------------------------------------------------------------------
// CreateSighting
// -----------------------------------------------------------------------

func (s *trackStore) CreateSighting(ctx context.Context, input CreateSightingInput) (Sighting, error) {
	if input.TenantID == "" {
		return Sighting{}, ErrInvalidTenantID
	}

	id := uuid.New().String()
	now := time.Now().UTC()
	seenAt := input.SeenAt
	if seenAt.IsZero() {
		seenAt = now
	}

	embBytes := EmbeddingToBytes(input.Embedding)

	q := fmt.Sprintf(
		`INSERT INTO reid_sightings
		    (id, tenant_id, track_id, camera_id, embedding, confidence,
		     bbox_x, bbox_y, bbox_w, bbox_h, seen_at, created_at)
		 VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)`,
		s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6),
		s.ph(7), s.ph(8), s.ph(9), s.ph(10), s.ph(11), s.ph(12),
	)

	_, err := s.db.ExecContext(ctx, q,
		id, input.TenantID, input.TrackID, input.CameraID, embBytes,
		input.Confidence, input.BBoxX, input.BBoxY, input.BBoxW, input.BBoxH,
		seenAt, now,
	)
	if err != nil {
		return Sighting{}, fmt.Errorf("reid.CreateSighting: %w", err)
	}

	return Sighting{
		ID:         id,
		TenantID:   input.TenantID,
		TrackID:    input.TrackID,
		CameraID:   input.CameraID,
		Embedding:  input.Embedding,
		Confidence: input.Confidence,
		BBoxX:      input.BBoxX,
		BBoxY:      input.BBoxY,
		BBoxW:      input.BBoxW,
		BBoxH:      input.BBoxH,
		SeenAt:     seenAt,
		CreatedAt:  now,
	}, nil
}

// -----------------------------------------------------------------------
// ListSightings
// -----------------------------------------------------------------------

func (s *trackStore) ListSightings(ctx context.Context, tenantID, trackID string, limit int) ([]Sighting, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}
	if trackID == "" {
		return nil, ErrInvalidID
	}
	if limit <= 0 {
		limit = 100
	}

	q := fmt.Sprintf(
		`SELECT id, tenant_id, track_id, camera_id, embedding, confidence,
		        bbox_x, bbox_y, bbox_w, bbox_h, seen_at, created_at
		 FROM reid_sightings
		 WHERE tenant_id = %s AND track_id = %s
		 ORDER BY seen_at DESC
		 LIMIT %d`,
		s.ph(1), s.ph(2), limit,
	)

	rows, err := s.db.QueryContext(ctx, q, tenantID, trackID)
	if err != nil {
		return nil, fmt.Errorf("reid.ListSightings: %w", err)
	}
	defer rows.Close()
	return scanSightings(rows)
}

// -----------------------------------------------------------------------
// DeleteTrack
// -----------------------------------------------------------------------

func (s *trackStore) DeleteTrack(ctx context.Context, tenantID, id string) error {
	if tenantID == "" {
		return ErrInvalidTenantID
	}
	if id == "" {
		return ErrInvalidID
	}

	// Delete sightings first.
	qSight := fmt.Sprintf(
		`DELETE FROM reid_sightings WHERE tenant_id = %s AND track_id = %s`,
		s.ph(1), s.ph(2),
	)
	if _, err := s.db.ExecContext(ctx, qSight, tenantID, id); err != nil {
		return fmt.Errorf("reid.DeleteTrack sightings: %w", err)
	}

	// Delete track.
	qTrack := fmt.Sprintf(
		`DELETE FROM reid_tracks WHERE tenant_id = %s AND id = %s`,
		s.ph(1), s.ph(2),
	)
	res, err := s.db.ExecContext(ctx, qTrack, tenantID, id)
	if err != nil {
		return fmt.Errorf("reid.DeleteTrack: %w", err)
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

// -----------------------------------------------------------------------
// scan helpers
// -----------------------------------------------------------------------

type scanner interface {
	Scan(dest ...any) error
}

func scanTrack(row scanner) (Track, error) {
	var t Track
	var embBytes []byte
	var metadata string

	err := row.Scan(
		&t.ID, &t.TenantID, &embBytes, &t.EmbeddingDim,
		&t.FirstCamera, &t.LastCamera, &t.FirstSeen, &t.LastSeen,
		&t.MatchCount, &metadata, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return Track{}, err
	}
	t.Embedding = BytesToEmbedding(embBytes)
	t.Metadata = metadata
	return t, nil
}

func scanTracks(rows *sql.Rows) ([]Track, error) {
	var out []Track
	for rows.Next() {
		t, err := scanTrack(rows)
		if err != nil {
			return nil, fmt.Errorf("reid scan track: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reid tracks rows: %w", err)
	}
	if out == nil {
		out = []Track{}
	}
	return out, nil
}

func scanSighting(row scanner) (Sighting, error) {
	var s Sighting
	var embBytes []byte

	err := row.Scan(
		&s.ID, &s.TenantID, &s.TrackID, &s.CameraID, &embBytes,
		&s.Confidence, &s.BBoxX, &s.BBoxY, &s.BBoxW, &s.BBoxH,
		&s.SeenAt, &s.CreatedAt,
	)
	if err != nil {
		return Sighting{}, err
	}
	s.Embedding = BytesToEmbedding(embBytes)
	return s, nil
}

func scanSightings(rows *sql.Rows) ([]Sighting, error) {
	var out []Sighting
	for rows.Next() {
		s, err := scanSighting(rows)
		if err != nil {
			return nil, fmt.Errorf("reid scan sighting: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reid sightings rows: %w", err)
	}
	if out == nil {
		out = []Sighting{}
	}
	return out, nil
}
