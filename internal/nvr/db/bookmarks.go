package db

import (
	"database/sql"
	"errors"
	"time"
)

type Bookmark struct {
	ID        int64  `json:"id"`
	CameraID  string `json:"camera_id"`
	Timestamp string `json:"timestamp"`
	Label     string `json:"label"`
	CreatedBy string `json:"created_by,omitempty"`
	CreatedAt string `json:"created_at"`
}

func (d *DB) InsertBookmark(b *Bookmark) error {
	if b.CreatedAt == "" {
		b.CreatedAt = time.Now().UTC().Format(timeFormat)
	}
	res, err := d.Exec(`
        INSERT INTO bookmarks (camera_id, timestamp, label, created_by, created_at)
        VALUES (?, ?, ?, ?, ?)`,
		b.CameraID, b.Timestamp, b.Label, b.CreatedBy, b.CreatedAt)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	b.ID = id
	return nil
}

func (d *DB) GetBookmarks(cameraID string, start, end time.Time) ([]Bookmark, error) {
	rows, err := d.Query(`
        SELECT id, camera_id, timestamp, label, COALESCE(created_by, ''), created_at
        FROM bookmarks
        WHERE camera_id = ? AND timestamp >= ? AND timestamp < ?
        ORDER BY timestamp`,
		cameraID, start.UTC().Format(timeFormat), end.UTC().Format(timeFormat))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookmarks []Bookmark
	for rows.Next() {
		var b Bookmark
		if err := rows.Scan(&b.ID, &b.CameraID, &b.Timestamp, &b.Label, &b.CreatedBy, &b.CreatedAt); err != nil {
			return nil, err
		}
		bookmarks = append(bookmarks, b)
	}
	return bookmarks, rows.Err()
}

func (d *DB) UpdateBookmark(id int64, label string) error {
	res, err := d.Exec("UPDATE bookmarks SET label = ? WHERE id = ?", label, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) DeleteBookmark(id int64) error {
	res, err := d.Exec("DELETE FROM bookmarks WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) GetBookmark(id int64) (*Bookmark, error) {
	b := &Bookmark{}
	err := d.QueryRow(`
        SELECT id, camera_id, timestamp, label, COALESCE(created_by, ''), created_at
        FROM bookmarks WHERE id = ?`, id).
		Scan(&b.ID, &b.CameraID, &b.Timestamp, &b.Label, &b.CreatedBy, &b.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return b, err
}
