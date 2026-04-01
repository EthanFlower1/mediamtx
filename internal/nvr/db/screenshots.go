package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Screenshot represents a camera screenshot stored on disk.
type Screenshot struct {
	ID        int64  `json:"id"`
	CameraID  string `json:"camera_id"`
	FilePath  string `json:"file_path"`
	FileSize  int64  `json:"file_size"`
	CreatedAt string `json:"created_at"`
}

// InsertScreenshot inserts a new screenshot record. CreatedAt is set to the
// current UTC time if empty. s.ID is populated from the database auto-increment.
func (d *DB) InsertScreenshot(s *Screenshot) error {
	if s.CreatedAt == "" {
		s.CreatedAt = time.Now().UTC().Format(timeFormat)
	}
	res, err := d.Exec(`
        INSERT INTO screenshots (camera_id, file_path, file_size, created_at)
        VALUES (?, ?, ?, ?)`,
		s.CameraID, s.FilePath, s.FileSize, s.CreatedAt)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	s.ID = id
	return nil
}

// ListScreenshots returns a paginated list of screenshots with an optional
// camera filter. sort must be "asc" or "desc" (defaults to "desc"). perPage is
// clamped to [1, 100] and defaults to 20 when <= 0. Returns the matching
// screenshots, the total count across all pages, and any error.
func (d *DB) ListScreenshots(cameraID, sort string, page, perPage int) ([]*Screenshot, int, error) {
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	if page <= 0 {
		page = 1
	}

	order := "DESC"
	if sort == "asc" {
		order = "ASC"
	}

	var (
		countQuery  string
		selectQuery string
		args        []interface{}
	)

	if cameraID != "" {
		countQuery = `SELECT COUNT(*) FROM screenshots WHERE camera_id = ?`
		selectQuery = fmt.Sprintf(`
            SELECT id, camera_id, file_path, file_size, created_at
            FROM screenshots
            WHERE camera_id = ?
            ORDER BY created_at %s
            LIMIT ? OFFSET ?`, order)
		args = []interface{}{cameraID}
	} else {
		countQuery = `SELECT COUNT(*) FROM screenshots`
		selectQuery = fmt.Sprintf(`
            SELECT id, camera_id, file_path, file_size, created_at
            FROM screenshots
            ORDER BY created_at %s
            LIMIT ? OFFSET ?`, order)
		args = []interface{}{}
	}

	var total int
	if err := d.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	selectArgs := append(args, perPage, offset)

	rows, err := d.Query(selectQuery, selectArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var screenshots []*Screenshot
	for rows.Next() {
		s := &Screenshot{}
		if err := rows.Scan(&s.ID, &s.CameraID, &s.FilePath, &s.FileSize, &s.CreatedAt); err != nil {
			return nil, 0, err
		}
		screenshots = append(screenshots, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return screenshots, total, nil
}

// GetScreenshot retrieves a screenshot by its ID. Returns ErrNotFound if no
// row matches.
func (d *DB) GetScreenshot(id int64) (*Screenshot, error) {
	s := &Screenshot{}
	err := d.QueryRow(`
        SELECT id, camera_id, file_path, file_size, created_at
        FROM screenshots WHERE id = ?`, id).
		Scan(&s.ID, &s.CameraID, &s.FilePath, &s.FileSize, &s.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// DeleteScreenshot deletes the screenshot with the given ID. Returns
// ErrNotFound if no row was deleted.
func (d *DB) DeleteScreenshot(id int64) error {
	res, err := d.Exec("DELETE FROM screenshots WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
