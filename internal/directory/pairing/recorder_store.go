package pairing

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	directorydb "github.com/bluenviron/mediamtx/internal/directory/db"
)

// RecorderRow is the shape written to and read from the recorders table.
type RecorderRow struct {
	RecorderID   string
	TenantID     string
	DevicePubkey string
	OSRelease    string
	HardwareJSON string
	TokenID      string
	EnrolledAt   time.Time
}

// RecorderStore is the SQLite-backed repository for enrolled Recorder nodes.
// It is safe for concurrent use.
type RecorderStore struct {
	db *directorydb.DB
}

// NewRecorderStore constructs a RecorderStore backed by the given directory DB.
func NewRecorderStore(db *directorydb.DB) *RecorderStore {
	return &RecorderStore{db: db}
}

// Insert writes a new recorder row. recorderID must be a fresh UUID generated
// by the caller. Returns an error if the insert fails (e.g. duplicate token_id
// due to a second check-in attempt for the same token, which is prevented
// upstream by Store.Redeem).
func (s *RecorderStore) Insert(ctx context.Context, row RecorderRow) error {
	hwJSON := row.HardwareJSON
	if hwJSON == "" {
		hwJSON = "{}"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO recorders
			(id, tenant_id, device_pubkey, os_release, hardware_json, token_id, enrolled_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		row.RecorderID,
		row.TenantID,
		row.DevicePubkey,
		row.OSRelease,
		hwJSON,
		row.TokenID,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("recorder/store: insert: %w", err)
	}
	return nil
}

// marshalHardware serializes a CheckInHardware to JSON for storage.
// Returns "{}" on error rather than failing the check-in.
func marshalHardware(hw CheckInHardware) string {
	b, err := json.Marshal(hw)
	if err != nil {
		return "{}"
	}
	return string(b)
}
