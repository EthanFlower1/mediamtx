package db

import (
	"database/sql"
	"errors"
	"time"
)

// WebhookConfig represents a webhook configuration that triggers on detections.
type WebhookConfig struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	Secret         string `json:"secret,omitempty"`   // HMAC-SHA256 signing secret
	CameraID       string `json:"camera_id,omitempty"` // empty = all cameras
	EventTypes     string `json:"event_types"`         // comma-separated: "detection,motion"
	ObjectClasses  string `json:"object_classes,omitempty"` // comma-separated filter: "person,car"
	MinConfidence  float64 `json:"min_confidence"`
	Enabled        bool   `json:"enabled"`
	MaxRetries     int    `json:"max_retries"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// WebhookDelivery represents a single delivery attempt for a webhook.
type WebhookDelivery struct {
	ID             int64  `json:"id"`
	WebhookID      string `json:"webhook_id"`
	EventType      string `json:"event_type"`
	Payload        string `json:"payload"`
	ResponseStatus int    `json:"response_status"`
	ResponseBody   string `json:"response_body,omitempty"`
	Error          string `json:"error,omitempty"`
	Attempt        int    `json:"attempt"`
	Status         string `json:"status"` // pending, success, failed, retrying
	CreatedAt      string `json:"created_at"`
	CompletedAt    string `json:"completed_at,omitempty"`
	NextRetryAt    string `json:"next_retry_at,omitempty"`
}

// InsertWebhookConfig creates a new webhook configuration.
func (d *DB) InsertWebhookConfig(w *WebhookConfig) error {
	now := time.Now().UTC().Format(timeFormat)
	if w.CreatedAt == "" {
		w.CreatedAt = now
	}
	if w.UpdatedAt == "" {
		w.UpdatedAt = now
	}
	if w.MaxRetries == 0 {
		w.MaxRetries = 3
	}
	if w.TimeoutSeconds == 0 {
		w.TimeoutSeconds = 10
	}
	_, err := d.Exec(`
		INSERT INTO webhook_configs (id, name, url, secret, camera_id, event_types,
			object_classes, min_confidence, enabled, max_retries, timeout_seconds,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.Name, w.URL, w.Secret, w.CameraID, w.EventTypes,
		w.ObjectClasses, w.MinConfidence, w.Enabled, w.MaxRetries, w.TimeoutSeconds,
		w.CreatedAt, w.UpdatedAt,
	)
	return err
}

// UpdateWebhookConfig updates an existing webhook configuration.
func (d *DB) UpdateWebhookConfig(w *WebhookConfig) error {
	w.UpdatedAt = time.Now().UTC().Format(timeFormat)
	res, err := d.Exec(`
		UPDATE webhook_configs SET name = ?, url = ?, secret = ?, camera_id = ?,
			event_types = ?, object_classes = ?, min_confidence = ?, enabled = ?,
			max_retries = ?, timeout_seconds = ?, updated_at = ?
		WHERE id = ?`,
		w.Name, w.URL, w.Secret, w.CameraID,
		w.EventTypes, w.ObjectClasses, w.MinConfidence, w.Enabled,
		w.MaxRetries, w.TimeoutSeconds, w.UpdatedAt,
		w.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetWebhookConfig retrieves a webhook configuration by ID.
func (d *DB) GetWebhookConfig(id string) (*WebhookConfig, error) {
	w := &WebhookConfig{}
	err := d.QueryRow(`
		SELECT id, name, url, secret, camera_id, event_types, object_classes,
			min_confidence, enabled, max_retries, timeout_seconds, created_at, updated_at
		FROM webhook_configs WHERE id = ?`, id).Scan(
		&w.ID, &w.Name, &w.URL, &w.Secret, &w.CameraID, &w.EventTypes,
		&w.ObjectClasses, &w.MinConfidence, &w.Enabled, &w.MaxRetries,
		&w.TimeoutSeconds, &w.CreatedAt, &w.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return w, err
}

// ListWebhookConfigs returns all webhook configurations.
func (d *DB) ListWebhookConfigs() ([]*WebhookConfig, error) {
	rows, err := d.Query(`
		SELECT id, name, url, secret, camera_id, event_types, object_classes,
			min_confidence, enabled, max_retries, timeout_seconds, created_at, updated_at
		FROM webhook_configs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []*WebhookConfig
	for rows.Next() {
		w := &WebhookConfig{}
		if err := rows.Scan(
			&w.ID, &w.Name, &w.URL, &w.Secret, &w.CameraID, &w.EventTypes,
			&w.ObjectClasses, &w.MinConfidence, &w.Enabled, &w.MaxRetries,
			&w.TimeoutSeconds, &w.CreatedAt, &w.UpdatedAt,
		); err != nil {
			return nil, err
		}
		configs = append(configs, w)
	}
	return configs, rows.Err()
}

// DeleteWebhookConfig deletes a webhook configuration by ID.
func (d *DB) DeleteWebhookConfig(id string) error {
	res, err := d.Exec(`DELETE FROM webhook_configs WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListEnabledWebhookConfigs returns all enabled webhook configurations.
func (d *DB) ListEnabledWebhookConfigs() ([]*WebhookConfig, error) {
	rows, err := d.Query(`
		SELECT id, name, url, secret, camera_id, event_types, object_classes,
			min_confidence, enabled, max_retries, timeout_seconds, created_at, updated_at
		FROM webhook_configs WHERE enabled = 1 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []*WebhookConfig
	for rows.Next() {
		w := &WebhookConfig{}
		if err := rows.Scan(
			&w.ID, &w.Name, &w.URL, &w.Secret, &w.CameraID, &w.EventTypes,
			&w.ObjectClasses, &w.MinConfidence, &w.Enabled, &w.MaxRetries,
			&w.TimeoutSeconds, &w.CreatedAt, &w.UpdatedAt,
		); err != nil {
			return nil, err
		}
		configs = append(configs, w)
	}
	return configs, rows.Err()
}

// InsertWebhookDelivery records a webhook delivery attempt.
func (d *DB) InsertWebhookDelivery(del *WebhookDelivery) error {
	if del.CreatedAt == "" {
		del.CreatedAt = time.Now().UTC().Format(timeFormat)
	}
	res, err := d.Exec(`
		INSERT INTO webhook_deliveries (webhook_id, event_type, payload,
			response_status, response_body, error, attempt, status,
			created_at, completed_at, next_retry_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		del.WebhookID, del.EventType, del.Payload,
		del.ResponseStatus, del.ResponseBody, del.Error, del.Attempt, del.Status,
		del.CreatedAt, del.CompletedAt, del.NextRetryAt,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	del.ID = id
	return nil
}

// UpdateWebhookDelivery updates a delivery record after a retry attempt.
func (d *DB) UpdateWebhookDelivery(del *WebhookDelivery) error {
	_, err := d.Exec(`
		UPDATE webhook_deliveries SET response_status = ?, response_body = ?,
			error = ?, attempt = ?, status = ?, completed_at = ?, next_retry_at = ?
		WHERE id = ?`,
		del.ResponseStatus, del.ResponseBody, del.Error, del.Attempt,
		del.Status, del.CompletedAt, del.NextRetryAt, del.ID,
	)
	return err
}

// ListWebhookDeliveries returns deliveries for a webhook config, newest first.
func (d *DB) ListWebhookDeliveries(webhookID string, limit int) ([]*WebhookDelivery, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.Query(`
		SELECT id, webhook_id, event_type, payload, response_status, response_body,
			error, attempt, status, created_at, COALESCE(completed_at, ''),
			COALESCE(next_retry_at, '')
		FROM webhook_deliveries WHERE webhook_id = ?
		ORDER BY created_at DESC LIMIT ?`, webhookID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deliveries []*WebhookDelivery
	for rows.Next() {
		del := &WebhookDelivery{}
		if err := rows.Scan(
			&del.ID, &del.WebhookID, &del.EventType, &del.Payload,
			&del.ResponseStatus, &del.ResponseBody, &del.Error,
			&del.Attempt, &del.Status, &del.CreatedAt, &del.CompletedAt,
			&del.NextRetryAt,
		); err != nil {
			return nil, err
		}
		deliveries = append(deliveries, del)
	}
	return deliveries, rows.Err()
}

// ListPendingWebhookDeliveries returns deliveries that need retrying.
func (d *DB) ListPendingWebhookDeliveries() ([]*WebhookDelivery, error) {
	now := time.Now().UTC().Format(timeFormat)
	rows, err := d.Query(`
		SELECT id, webhook_id, event_type, payload, response_status, response_body,
			error, attempt, status, created_at, COALESCE(completed_at, ''),
			COALESCE(next_retry_at, '')
		FROM webhook_deliveries
		WHERE status = 'retrying' AND next_retry_at <= ?
		ORDER BY next_retry_at`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deliveries []*WebhookDelivery
	for rows.Next() {
		del := &WebhookDelivery{}
		if err := rows.Scan(
			&del.ID, &del.WebhookID, &del.EventType, &del.Payload,
			&del.ResponseStatus, &del.ResponseBody, &del.Error,
			&del.Attempt, &del.Status, &del.CreatedAt, &del.CompletedAt,
			&del.NextRetryAt,
		); err != nil {
			return nil, err
		}
		deliveries = append(deliveries, del)
	}
	return deliveries, rows.Err()
}

// CleanupOldWebhookDeliveries removes deliveries older than the given duration.
func (d *DB) CleanupOldWebhookDeliveries(olderThan time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(timeFormat)
	res, err := d.Exec(`DELETE FROM webhook_deliveries WHERE created_at < ? AND status IN ('success', 'failed')`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
