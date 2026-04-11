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

// ListConfigByPrefix returns all config values whose key starts with the given prefix.
func (d *DB) ListConfigByPrefix(prefix string) ([]string, error) {
	rows, err := d.Query("SELECT value FROM config WHERE key LIKE ?", prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var val string
		if err := rows.Scan(&val); err != nil {
			return nil, err
		}
		values = append(values, val)
	}
	return values, rows.Err()
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
