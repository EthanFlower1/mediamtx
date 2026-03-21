package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// SavedClip represents a bookmarked clip in the database.
type SavedClip struct {
	ID        string `json:"id"`
	CameraID  string `json:"camera_id"`
	Name      string `json:"name"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Tags      string `json:"tags"`
	Notes     string `json:"notes"`
	CreatedAt string `json:"created_at"`
}

// CreateSavedClip inserts a new saved clip into the database.
// If clip.ID is empty, a new UUID is generated.
func (d *DB) CreateSavedClip(clip *SavedClip) error {
	if clip.ID == "" {
		clip.ID = uuid.New().String()
	}
	if clip.CreatedAt == "" {
		clip.CreatedAt = time.Now().UTC().Format(timeFormat)
	}

	_, err := d.Exec(`
		INSERT INTO saved_clips (id, camera_id, name, start_time, end_time, tags, notes, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		clip.ID, clip.CameraID, clip.Name, clip.StartTime, clip.EndTime,
		clip.Tags, clip.Notes, clip.CreatedAt,
	)
	return err
}

// GetSavedClip retrieves a saved clip by its ID. Returns ErrNotFound if no match.
func (d *DB) GetSavedClip(id string) (*SavedClip, error) {
	clip := &SavedClip{}
	err := d.QueryRow(`
		SELECT id, camera_id, name, start_time, end_time, tags, notes, created_at
		FROM saved_clips WHERE id = ?`, id,
	).Scan(
		&clip.ID, &clip.CameraID, &clip.Name, &clip.StartTime, &clip.EndTime,
		&clip.Tags, &clip.Notes, &clip.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return clip, nil
}

// ListSavedClips returns saved clips, optionally filtered by camera ID.
// If cameraID is empty, all clips are returned.
func (d *DB) ListSavedClips(cameraID string) ([]*SavedClip, error) {
	var rows *sql.Rows
	var err error

	if cameraID != "" {
		rows, err = d.Query(`
			SELECT id, camera_id, name, start_time, end_time, tags, notes, created_at
			FROM saved_clips WHERE camera_id = ?
			ORDER BY created_at DESC`, cameraID)
	} else {
		rows, err = d.Query(`
			SELECT id, camera_id, name, start_time, end_time, tags, notes, created_at
			FROM saved_clips ORDER BY created_at DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clips []*SavedClip
	for rows.Next() {
		clip := &SavedClip{}
		if err := rows.Scan(
			&clip.ID, &clip.CameraID, &clip.Name, &clip.StartTime, &clip.EndTime,
			&clip.Tags, &clip.Notes, &clip.CreatedAt,
		); err != nil {
			return nil, err
		}
		clips = append(clips, clip)
	}
	return clips, rows.Err()
}

// DeleteSavedClip deletes a saved clip by its ID. Returns ErrNotFound if no match.
func (d *DB) DeleteSavedClip(id string) error {
	res, err := d.Exec("DELETE FROM saved_clips WHERE id = ?", id)
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
