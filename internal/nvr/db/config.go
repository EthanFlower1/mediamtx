package db

import (
	"database/sql"
	"errors"
)

// GetConfig retrieves a configuration value by key. Returns ErrNotFound if no match.
func (d *DB) GetConfig(key string) (string, error) {
	var value string
	err := d.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

// SetConfig upserts a configuration key-value pair.
func (d *DB) SetConfig(key, value string) error {
	_, err := d.Exec(`
		INSERT INTO config (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

// DeleteConfig deletes a configuration entry by key. Returns ErrNotFound if no match.
func (d *DB) DeleteConfig(key string) error {
	res, err := d.Exec("DELETE FROM config WHERE key = ?", key)
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
