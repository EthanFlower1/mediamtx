package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Stream role constants define the purposes a camera stream can serve.
const (
	StreamRoleLiveView    = "live_view"
	StreamRoleRecording   = "recording"
	StreamRoleMobile      = "mobile"
	StreamRoleAIDetection = "ai_detection"
)

// CameraStream represents a single RTSP stream belonging to a camera.
type CameraStream struct {
	ID           string `json:"id"`
	CameraID     string `json:"camera_id"`
	Name         string `json:"name"`
	RTSPURL      string `json:"rtsp_url"`
	ProfileToken string `json:"profile_token"`
	VideoCodec   string `json:"video_codec"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Roles        string `json:"roles"`
	CreatedAt    string `json:"created_at"`
}

// HasRole reports whether the stream is assigned the given role.
// Roles are stored as a comma-separated string (e.g. "live_view,recording").
func (s *CameraStream) HasRole(role string) bool {
	for _, r := range s.RoleList() {
		if r == role {
			return true
		}
	}
	return false
}

// RoleList returns the stream's roles as a slice of strings.
func (s *CameraStream) RoleList() []string {
	if s.Roles == "" {
		return nil
	}
	parts := strings.Split(s.Roles, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	return result
}

// CreateCameraStream inserts a new stream record. A UUID and timestamp are
// generated automatically if not already set.
func (d *DB) CreateCameraStream(s *CameraStream) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	if s.CreatedAt == "" {
		s.CreatedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	}

	_, err := d.Exec(`
		INSERT INTO camera_streams (id, camera_id, name, rtsp_url, profile_token,
			video_codec, width, height, roles, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.CameraID, s.Name, s.RTSPURL, s.ProfileToken,
		s.VideoCodec, s.Width, s.Height, s.Roles, s.CreatedAt,
	)
	return err
}

// ListCameraStreams returns all streams for a camera, ordered by resolution
// descending (largest first) so the highest-quality stream comes first.
func (d *DB) ListCameraStreams(cameraID string) ([]*CameraStream, error) {
	rows, err := d.Query(`
		SELECT id, camera_id, name, rtsp_url, profile_token, video_codec,
			width, height, roles, created_at
		FROM camera_streams
		WHERE camera_id = ?
		ORDER BY (width * height) DESC, created_at ASC`, cameraID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var streams []*CameraStream
	for rows.Next() {
		s := &CameraStream{}
		if err := rows.Scan(
			&s.ID, &s.CameraID, &s.Name, &s.RTSPURL, &s.ProfileToken,
			&s.VideoCodec, &s.Width, &s.Height, &s.Roles, &s.CreatedAt,
		); err != nil {
			return nil, err
		}
		streams = append(streams, s)
	}
	return streams, rows.Err()
}

// GetCameraStream retrieves a single stream by its ID. Returns ErrNotFound if
// no record exists.
func (d *DB) GetCameraStream(id string) (*CameraStream, error) {
	s := &CameraStream{}
	err := d.QueryRow(`
		SELECT id, camera_id, name, rtsp_url, profile_token, video_codec,
			width, height, roles, created_at
		FROM camera_streams WHERE id = ?`, id,
	).Scan(
		&s.ID, &s.CameraID, &s.Name, &s.RTSPURL, &s.ProfileToken,
		&s.VideoCodec, &s.Width, &s.Height, &s.Roles, &s.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// UpdateCameraStream updates the mutable fields of an existing stream.
// Returns ErrNotFound if no record with s.ID exists.
func (d *DB) UpdateCameraStream(s *CameraStream) error {
	res, err := d.Exec(`
		UPDATE camera_streams
		SET name = ?, rtsp_url = ?, profile_token = ?, video_codec = ?,
			width = ?, height = ?, roles = ?
		WHERE id = ?`,
		s.Name, s.RTSPURL, s.ProfileToken, s.VideoCodec,
		s.Width, s.Height, s.Roles, s.ID,
	)
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteCameraStream deletes a stream by its ID. Returns ErrNotFound if no
// record exists.
func (d *DB) DeleteCameraStream(id string) error {
	res, err := d.Exec("DELETE FROM camera_streams WHERE id = ?", id)
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ResolveStreamURL returns the RTSP URL of the best stream for a camera that
// carries the requested role. If no stream record is found it falls back to
// the legacy cameras.rtsp_url column. Returns ErrNotFound when neither source
// has a URL.
func (d *DB) ResolveStreamURL(cameraID, role string) (string, error) {
	var rtspURL string
	err := d.QueryRow(`
		SELECT rtsp_url FROM camera_streams
		WHERE camera_id = ?
		  AND (',' || roles || ',' LIKE '%,' || ? || ',%')
		ORDER BY (width * height) DESC
		LIMIT 1`, cameraID, role,
	).Scan(&rtspURL)
	if err == nil {
		return rtspURL, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	// Fall back to the legacy cameras.rtsp_url column.
	err = d.QueryRow(
		`SELECT COALESCE(rtsp_url, '') FROM cameras WHERE id = ?`, cameraID,
	).Scan(&rtspURL)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if rtspURL == "" {
		return "", ErrNotFound
	}
	return rtspURL, nil
}

// ResolveStream returns the full CameraStream object for a camera that carries
// the requested role, ordered by resolution descending. Falls back to building
// a synthetic stream from cameras.rtsp_url when no stream record exists.
// Returns ErrNotFound when neither source has a URL.
func (d *DB) ResolveStream(cameraID, role string) (*CameraStream, error) {
	s := &CameraStream{}
	err := d.QueryRow(`
		SELECT id, camera_id, name, rtsp_url, profile_token, video_codec,
			width, height, roles, created_at
		FROM camera_streams
		WHERE camera_id = ?
		  AND (',' || roles || ',' LIKE '%,' || ? || ',%')
		ORDER BY (width * height) DESC
		LIMIT 1`, cameraID, role,
	).Scan(
		&s.ID, &s.CameraID, &s.Name, &s.RTSPURL, &s.ProfileToken,
		&s.VideoCodec, &s.Width, &s.Height, &s.Roles, &s.CreatedAt,
	)
	if err == nil {
		return s, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// Fall back to the legacy cameras.rtsp_url column.
	var rtspURL string
	err = d.QueryRow(
		`SELECT COALESCE(rtsp_url, '') FROM cameras WHERE id = ?`, cameraID,
	).Scan(&rtspURL)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if rtspURL == "" {
		return nil, ErrNotFound
	}

	return &CameraStream{
		CameraID: cameraID,
		Name:     "Main Stream",
		RTSPURL:  rtspURL,
		Roles:    "live_view,recording,ai_detection,mobile",
	}, nil
}
