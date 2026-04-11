package db

import "time"

// Track represents a cross-camera person re-identification track.
// Each track groups sightings of the same person across multiple cameras.
type Track struct {
	ID          int64  `json:"id"`
	Label       string `json:"label"`        // user-friendly label, e.g. "Person A"
	Status      string `json:"status"`        // "active", "closed"
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	DetectionID int64  `json:"detection_id"` // source detection that started this track
}

// Sighting is a single appearance of a tracked person on one camera.
type Sighting struct {
	ID         int64   `json:"id"`
	TrackID    int64   `json:"track_id"`
	CameraID   string  `json:"camera_id"`
	CameraName string  `json:"camera_name"`
	Timestamp  string  `json:"timestamp"`
	EndTime    string  `json:"end_time,omitempty"`
	Confidence float64 `json:"confidence"`
	Thumbnail  string  `json:"thumbnail,omitempty"`
}

// TrackWithSightings is a Track enriched with its sightings for API responses.
type TrackWithSightings struct {
	Track
	Sightings  []Sighting `json:"sightings"`
	CameraCount int       `json:"camera_count"`
}

// InsertTrack creates a new cross-camera track.
func (d *DB) InsertTrack(t *Track) error {
	now := time.Now().UTC().Format(timeFormat)
	if t.Status == "" {
		t.Status = "active"
	}
	res, err := d.Exec(`
		INSERT INTO cross_camera_tracks (label, status, created_at, updated_at, detection_id)
		VALUES (?, ?, ?, ?, ?)`,
		t.Label, t.Status, now, now, t.DetectionID,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	t.ID = id
	t.CreatedAt = now
	t.UpdatedAt = now
	return nil
}

// GetTrack returns a track by ID.
func (d *DB) GetTrack(id int64) (*Track, error) {
	row := d.QueryRow(`
		SELECT id, label, status, created_at, updated_at, detection_id
		FROM cross_camera_tracks WHERE id = ?`, id)
	t := &Track{}
	err := row.Scan(&t.ID, &t.Label, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.DetectionID)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// InsertSighting adds a sighting to a track.
func (d *DB) InsertSighting(s *Sighting) error {
	res, err := d.Exec(`
		INSERT INTO cross_camera_sightings (track_id, camera_id, timestamp, end_time, confidence, thumbnail)
		VALUES (?, ?, ?, ?, ?, ?)`,
		s.TrackID, s.CameraID, s.Timestamp, s.EndTime, s.Confidence, s.Thumbnail,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	s.ID = id
	// Update parent track timestamp.
	_, _ = d.Exec(`UPDATE cross_camera_tracks SET updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(timeFormat), s.TrackID)
	return nil
}

// GetTrackWithSightings returns a track with all its sightings joined with
// camera names and a distinct camera count.
func (d *DB) GetTrackWithSightings(trackID int64) (*TrackWithSightings, error) {
	t, err := d.GetTrack(trackID)
	if err != nil {
		return nil, err
	}

	rows, err := d.Query(`
		SELECT s.id, s.track_id, s.camera_id, COALESCE(c.name, s.camera_id),
			s.timestamp, s.end_time, s.confidence, COALESCE(s.thumbnail, '')
		FROM cross_camera_sightings s
		LEFT JOIN cameras c ON s.camera_id = c.id
		WHERE s.track_id = ?
		ORDER BY s.timestamp ASC`, trackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cameras := map[string]struct{}{}
	var sightings []Sighting
	for rows.Next() {
		var s Sighting
		if err := rows.Scan(&s.ID, &s.TrackID, &s.CameraID, &s.CameraName,
			&s.Timestamp, &s.EndTime, &s.Confidence, &s.Thumbnail); err != nil {
			return nil, err
		}
		sightings = append(sightings, s)
		cameras[s.CameraID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &TrackWithSightings{
		Track:       *t,
		Sightings:   sightings,
		CameraCount: len(cameras),
	}, nil
}

// ListTracks returns recent tracks, most recent first.
func (d *DB) ListTracks(limit int) ([]*TrackWithSightings, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := d.Query(`
		SELECT id, label, status, created_at, updated_at, detection_id
		FROM cross_camera_tracks
		ORDER BY updated_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*TrackWithSightings
	for rows.Next() {
		t := &Track{}
		if err := rows.Scan(&t.ID, &t.Label, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.DetectionID); err != nil {
			return nil, err
		}
		// Fetch sightings for each track.
		tw, err := d.GetTrackWithSightings(t.ID)
		if err != nil {
			continue
		}
		results = append(results, tw)
	}
	return results, rows.Err()
}

// FindTrackByDetection looks up an existing track started from a given detection.
func (d *DB) FindTrackByDetection(detectionID int64) (*Track, error) {
	row := d.QueryRow(`
		SELECT id, label, status, created_at, updated_at, detection_id
		FROM cross_camera_tracks WHERE detection_id = ?`, detectionID)
	t := &Track{}
	err := row.Scan(&t.ID, &t.Label, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.DetectionID)
	if err != nil {
		return nil, err
	}
	return t, nil
}
