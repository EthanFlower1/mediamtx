package db

import "time"

// QueuedCommand represents an ONVIF command stored for offline execution.
type QueuedCommand struct {
	ID           string `json:"id"`
	CameraID     string `json:"camera_id"`
	CommandType  string `json:"command_type"`
	Payload      string `json:"payload"`
	Status       string `json:"status"` // "pending", "executed", "failed"
	ErrorMessage string `json:"error_message,omitempty"`
	QueuedAt     string `json:"queued_at"`
	ExecutedAt   string `json:"executed_at,omitempty"`
}

// InsertQueuedCommand stores a command for later execution.
func (d *DB) InsertQueuedCommand(cmd *QueuedCommand) error {
	_, err := d.Exec(
		`INSERT INTO queued_commands (id, camera_id, command_type, payload, status, queued_at)
		 VALUES (?, ?, ?, ?, 'pending', ?)`,
		cmd.ID, cmd.CameraID, cmd.CommandType, cmd.Payload,
		time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
	)
	return err
}

// ListPendingCommands returns all pending commands for a camera, oldest first.
func (d *DB) ListPendingCommands(cameraID string) ([]*QueuedCommand, error) {
	rows, err := d.Query(
		`SELECT id, camera_id, command_type, payload, status, COALESCE(error_message, ''), queued_at, COALESCE(executed_at, '')
		 FROM queued_commands
		 WHERE camera_id = ? AND status = 'pending'
		 ORDER BY queued_at ASC`,
		cameraID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cmds []*QueuedCommand
	for rows.Next() {
		c := &QueuedCommand{}
		if err := rows.Scan(&c.ID, &c.CameraID, &c.CommandType, &c.Payload, &c.Status, &c.ErrorMessage, &c.QueuedAt, &c.ExecutedAt); err != nil {
			return nil, err
		}
		cmds = append(cmds, c)
	}
	return cmds, rows.Err()
}

// UpdateCommandStatus marks a queued command as executed or failed.
func (d *DB) UpdateCommandStatus(id, status, errMsg string) error {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	_, err := d.Exec(
		`UPDATE queued_commands SET status = ?, error_message = ?, executed_at = ? WHERE id = ?`,
		status, errMsg, now, id,
	)
	return err
}

// DeleteQueuedCommands removes all commands for a camera.
func (d *DB) DeleteQueuedCommands(cameraID string) error {
	_, err := d.Exec(`DELETE FROM queued_commands WHERE camera_id = ?`, cameraID)
	return err
}

// ListQueuedCommands returns all commands for a camera regardless of status.
func (d *DB) ListQueuedCommands(cameraID string, limit int) ([]*QueuedCommand, error) {
	query := `SELECT id, camera_id, command_type, payload, status, COALESCE(error_message, ''), queued_at, COALESCE(executed_at, '')
	          FROM queued_commands
	          WHERE camera_id = ?
	          ORDER BY queued_at DESC`
	args := []interface{}{cameraID}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cmds []*QueuedCommand
	for rows.Next() {
		c := &QueuedCommand{}
		if err := rows.Scan(&c.ID, &c.CameraID, &c.CommandType, &c.Payload, &c.Status, &c.ErrorMessage, &c.QueuedAt, &c.ExecutedAt); err != nil {
			return nil, err
		}
		cmds = append(cmds, c)
	}
	return cmds, rows.Err()
}
