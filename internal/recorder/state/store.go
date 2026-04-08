package state

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver per CLAUDE.md
)

// ErrNotFound is returned by Store methods when a requested row does
// not exist in the cache.
var ErrNotFound = errors.New("state: not found")

// timeFmt is the ISO-8601 layout used for all timestamp columns. It
// matches the format used elsewhere in the codebase (internal/nvr/db).
const timeFmt = "2006-01-02T15:04:05.000Z07:00"

// Store is the Recorder-side local SQLite cache. It is safe for
// concurrent use by multiple goroutines.
type Store struct {
	db     *sql.DB
	path   string
	crypto Cryptostore
}

// Options configures Open.
type Options struct {
	// Cryptostore is the encryption backend used for RTSP credentials.
	// If nil, NoopCryptostore is used and credentials are stored in the
	// clear — intended for tests and local dev only.
	Cryptostore Cryptostore
}

// Open opens (or creates) the Recorder local cache at path, enables
// foreign keys + WAL, and runs any pending migrations.
func Open(path string, opts Options) (*Store, error) {
	dsn := path + "?" + url.Values{
		"_pragma": []string{
			"foreign_keys(1)",
			"journal_mode(WAL)",
			"busy_timeout(10000)",
		},
	}.Encode()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open state db: %w", err)
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("state db ping: %w", err)
	}

	crypto := opts.Cryptostore
	if crypto == nil {
		crypto = NoopCryptostore{}
	}

	s := &Store{db: db, path: path, crypto: crypto}
	if err := s.runMigrations(); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return s, nil
}

// Close closes the underlying database handle.
func (s *Store) Close() error { return s.db.Close() }

// Path returns the filesystem path of the cache database.
func (s *Store) Path() string { return s.path }

// DB exposes the raw *sql.DB for advanced callers (e.g. health probes).
// Most callers should use the typed methods instead.
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) runMigrations() error {
	if _, err := s.db.Exec(
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`,
	); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	for _, m := range migrations {
		var count int
		if err := s.db.QueryRow(
			`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version,
		).Scan(&count); err != nil {
			return fmt.Errorf("check migration %d: %w", m.version, err)
		}
		if count > 0 {
			continue
		}
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.version, err)
		}
		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("exec migration %d: %w", m.version, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations (version) VALUES (?)`, m.version,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}
	}
	return nil
}

// SchemaVersion returns the highest applied migration version. Useful
// for health checks and tests.
func (s *Store) SchemaVersion() (int, error) {
	var v sql.NullInt64
	if err := s.db.QueryRow(
		`SELECT MAX(version) FROM schema_migrations`,
	).Scan(&v); err != nil {
		return 0, err
	}
	if !v.Valid {
		return 0, nil
	}
	return int(v.Int64), nil
}

// ---- assigned_cameras --------------------------------------------------

// UpsertCamera inserts or updates an assigned camera row. The camera's
// RTSP password (passed through AssignedCamera.RTSPPassword) is encrypted
// via the configured Cryptostore and written to rtsp_credentials. The
// plaintext is never persisted.
func (s *Store) UpsertCamera(ctx context.Context, cam AssignedCamera) error {
	if cam.CameraID == "" {
		return errors.New("state: UpsertCamera: empty camera_id")
	}
	if cam.Config.ID == "" {
		cam.Config.ID = cam.CameraID
	}
	configJSON, err := json.Marshal(cam.Config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	var cipher []byte
	if cam.RTSPPassword != "" {
		cipher, err = s.crypto.Encrypt(ctx, []byte(cam.RTSPPassword))
		if err != nil {
			return fmt.Errorf("encrypt rtsp credentials: %w", err)
		}
	}
	now := time.Now().UTC()
	assignedAt := cam.AssignedAt
	if assignedAt.IsZero() {
		assignedAt = now
	}
	var lastPush any
	if cam.LastStatePushAt != nil {
		lastPush = cam.LastStatePushAt.UTC().Format(timeFmt)
	}

	_, err = s.db.ExecContext(ctx, `
INSERT INTO assigned_cameras (
	camera_id, config, config_version, rtsp_credentials,
	assigned_at, updated_at, last_state_push_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(camera_id) DO UPDATE SET
	config = excluded.config,
	config_version = excluded.config_version,
	rtsp_credentials = excluded.rtsp_credentials,
	updated_at = excluded.updated_at,
	last_state_push_at = COALESCE(excluded.last_state_push_at, assigned_cameras.last_state_push_at)
`,
		cam.CameraID,
		string(configJSON),
		cam.ConfigVersion,
		cipher,
		assignedAt.UTC().Format(timeFmt),
		now.Format(timeFmt),
		lastPush,
	)
	if err != nil {
		return fmt.Errorf("upsert assigned_cameras: %w", err)
	}
	return nil
}

// RemoveCamera deletes an assigned camera and all of its segment_index
// rows. It is a no-op (returns nil) if the camera is not present.
func (s *Store) RemoveCamera(ctx context.Context, cameraID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM segment_index WHERE camera_id = ?`, cameraID,
	); err != nil {
		tx.Rollback()
		return fmt.Errorf("delete segments: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM assigned_cameras WHERE camera_id = ?`, cameraID,
	); err != nil {
		tx.Rollback()
		return fmt.Errorf("delete camera: %w", err)
	}
	return tx.Commit()
}

// GetCamera returns the AssignedCamera with the given id. The RTSP
// password is decrypted via the configured Cryptostore and materialized
// onto the returned struct. Returns ErrNotFound if the camera is absent.
func (s *Store) GetCamera(ctx context.Context, cameraID string) (AssignedCamera, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT camera_id, config, config_version, rtsp_credentials,
       assigned_at, updated_at, last_state_push_at
FROM assigned_cameras
WHERE camera_id = ?
`, cameraID)
	cam, err := scanAssignedCamera(ctx, row, s.crypto)
	if errors.Is(err, sql.ErrNoRows) {
		return AssignedCamera{}, ErrNotFound
	}
	return cam, err
}

// ListAssigned returns all assigned cameras, sorted by camera_id.
func (s *Store) ListAssigned(ctx context.Context) ([]AssignedCamera, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT camera_id, config, config_version, rtsp_credentials,
       assigned_at, updated_at, last_state_push_at
FROM assigned_cameras
ORDER BY camera_id
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AssignedCamera
	for rows.Next() {
		cam, err := scanAssignedCamera(ctx, rows, s.crypto)
		if err != nil {
			return nil, err
		}
		out = append(out, cam)
	}
	return out, rows.Err()
}

// scanner is satisfied by *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanAssignedCamera(ctx context.Context, sc scanner, crypto Cryptostore) (AssignedCamera, error) {
	var (
		cam       AssignedCamera
		configStr string
		cipher    []byte
		assignedS string
		updatedS  string
		lastPushS sql.NullString
	)
	if err := sc.Scan(
		&cam.CameraID,
		&configStr,
		&cam.ConfigVersion,
		&cipher,
		&assignedS,
		&updatedS,
		&lastPushS,
	); err != nil {
		return AssignedCamera{}, err
	}
	if err := json.Unmarshal([]byte(configStr), &cam.Config); err != nil {
		return AssignedCamera{}, fmt.Errorf("unmarshal config: %w", err)
	}
	t, err := parseTime(assignedS)
	if err != nil {
		return AssignedCamera{}, fmt.Errorf("parse assigned_at: %w", err)
	}
	cam.AssignedAt = t
	t, err = parseTime(updatedS)
	if err != nil {
		return AssignedCamera{}, fmt.Errorf("parse updated_at: %w", err)
	}
	cam.UpdatedAt = t
	if lastPushS.Valid {
		t, err := parseTime(lastPushS.String)
		if err != nil {
			return AssignedCamera{}, fmt.Errorf("parse last_state_push_at: %w", err)
		}
		cam.LastStatePushAt = &t
	}
	if len(cipher) > 0 {
		pt, err := crypto.Decrypt(ctx, cipher)
		if err != nil {
			return AssignedCamera{}, fmt.Errorf("decrypt rtsp credentials: %w", err)
		}
		cam.RTSPPassword = string(pt)
	}
	return cam, nil
}

// MarkStatePushed records that the Recorder successfully pushed a state
// update for cameraID to the Directory at t. Used by the state-push
// loop to compute which cameras are stale.
func (s *Store) MarkStatePushed(ctx context.Context, cameraID string, t time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE assigned_cameras SET last_state_push_at = ? WHERE camera_id = ?`,
		t.UTC().Format(timeFmt), cameraID,
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

// ---- local_state -------------------------------------------------------

// SetState sets a key in the free-form local_state table. value is
// JSON-encoded. Pass nil to store a JSON null.
func (s *Store) SetState(ctx context.Context, key string, value any) error {
	if key == "" {
		return errors.New("state: SetState: empty key")
	}
	buf, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal state value: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO local_state (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value
`, key, string(buf))
	return err
}

// GetState loads a key from local_state into dest (which must be a
// pointer). Returns ErrNotFound if the key is not present.
func (s *Store) GetState(ctx context.Context, key string, dest any) error {
	var raw string
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM local_state WHERE key = ?`, key,
	).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), dest)
}

// DeleteState removes a key from local_state. No-op if absent.
func (s *Store) DeleteState(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM local_state WHERE key = ?`, key)
	return err
}

// ---- segment_index -----------------------------------------------------

// AppendSegment inserts (or replaces) a segment_index row. Segments are
// keyed by (camera_id, start_ts) so re-appending the same segment is
// idempotent.
func (s *Store) AppendSegment(ctx context.Context, seg Segment) error {
	if seg.CameraID == "" {
		return errors.New("state: AppendSegment: empty camera_id")
	}
	if seg.StartTS.IsZero() || seg.EndTS.IsZero() {
		return errors.New("state: AppendSegment: zero timestamps")
	}
	if seg.EndTS.Before(seg.StartTS) {
		return errors.New("state: AppendSegment: end_ts before start_ts")
	}
	uploaded := 0
	if seg.UploadedToCloudArchive {
		uploaded = 1
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO segment_index (
	camera_id, start_ts, end_ts, path, size_bytes, uploaded_to_cloud_archive
) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(camera_id, start_ts) DO UPDATE SET
	end_ts = excluded.end_ts,
	path = excluded.path,
	size_bytes = excluded.size_bytes,
	uploaded_to_cloud_archive = excluded.uploaded_to_cloud_archive
`,
		seg.CameraID,
		seg.StartTS.UTC().Format(timeFmt),
		seg.EndTS.UTC().Format(timeFmt),
		seg.Path,
		seg.SizeBytes,
		uploaded,
	)
	return err
}

// QuerySegments returns all segments for cameraID whose [start_ts,end_ts]
// overlaps [from,to]. Results are ordered by start_ts ascending.
//
// Two segments overlap when:  seg.start_ts < to  AND  seg.end_ts > from.
func (s *Store) QuerySegments(
	ctx context.Context, cameraID string, from, to time.Time,
) ([]Segment, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT camera_id, start_ts, end_ts, path, size_bytes, uploaded_to_cloud_archive
FROM segment_index
WHERE camera_id = ?
  AND start_ts < ?
  AND end_ts   > ?
ORDER BY start_ts ASC
`,
		cameraID,
		to.UTC().Format(timeFmt),
		from.UTC().Format(timeFmt),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Segment
	for rows.Next() {
		var (
			seg      Segment
			startS   string
			endS     string
			uploaded int
		)
		if err := rows.Scan(
			&seg.CameraID, &startS, &endS, &seg.Path, &seg.SizeBytes, &uploaded,
		); err != nil {
			return nil, err
		}
		if seg.StartTS, err = parseTime(startS); err != nil {
			return nil, err
		}
		if seg.EndTS, err = parseTime(endS); err != nil {
			return nil, err
		}
		seg.UploadedToCloudArchive = uploaded != 0
		out = append(out, seg)
	}
	return out, rows.Err()
}

// MarkSegmentUploaded flips the uploaded_to_cloud_archive flag on a
// single segment (identified by camera + start_ts).
func (s *Store) MarkSegmentUploaded(
	ctx context.Context, cameraID string, startTS time.Time,
) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE segment_index
SET uploaded_to_cloud_archive = 1
WHERE camera_id = ? AND start_ts = ?
`, cameraID, startTS.UTC().Format(timeFmt))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- reconcile ---------------------------------------------------------

// ReconcileAssignments diffs snapshot (the authoritative list of cameras
// the Directory says belong to this Recorder) against the current cache,
// applies all changes inside a single transaction, and returns a
// ReconcileDiff describing what changed.
//
// Semantics:
//
//   - Cameras in the snapshot but not the cache  -> Added   + upserted
//   - Cameras in both with different config      -> Updated + upserted
//   - Cameras in the cache but not the snapshot  -> Removed + deleted
//   - Cameras in both with identical config      -> Unchanged
//
// "Different config" is determined first by ConfigVersion (if non-zero
// on both sides) and then by byte-identical JSON of CameraConfig +
// equal RTSPPassword. The whole reconcile runs in one transaction so
// the cache is never observable in a half-applied state.
func (s *Store) ReconcileAssignments(
	ctx context.Context, snapshot []AssignedCamera,
) (ReconcileDiff, error) {
	var diff ReconcileDiff

	current, err := s.ListAssigned(ctx)
	if err != nil {
		return diff, fmt.Errorf("list current: %w", err)
	}

	currentByID := make(map[string]AssignedCamera, len(current))
	for _, c := range current {
		currentByID[c.CameraID] = c
	}
	snapByID := make(map[string]AssignedCamera, len(snapshot))
	for _, c := range snapshot {
		if c.CameraID == "" && c.Config.ID != "" {
			c.CameraID = c.Config.ID
		}
		snapByID[c.CameraID] = c
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return diff, err
	}
	rollback := true
	defer func() {
		if rollback {
			tx.Rollback()
		}
	}()

	// Added + Updated + Unchanged
	for id, incoming := range snapByID {
		existing, ok := currentByID[id]
		if !ok {
			diff.Added = append(diff.Added, id)
			if err := upsertTx(ctx, tx, incoming, s.crypto); err != nil {
				return diff, fmt.Errorf("insert %s: %w", id, err)
			}
			continue
		}
		changed, err := cameraDiffers(existing, incoming)
		if err != nil {
			return diff, err
		}
		if changed {
			diff.Updated = append(diff.Updated, id)
			if err := upsertTx(ctx, tx, incoming, s.crypto); err != nil {
				return diff, fmt.Errorf("update %s: %w", id, err)
			}
		} else {
			diff.Unchanged = append(diff.Unchanged, id)
		}
	}

	// Removed
	for id := range currentByID {
		if _, ok := snapByID[id]; ok {
			continue
		}
		diff.Removed = append(diff.Removed, id)
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM segment_index WHERE camera_id = ?`, id,
		); err != nil {
			return diff, fmt.Errorf("delete segments %s: %w", id, err)
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM assigned_cameras WHERE camera_id = ?`, id,
		); err != nil {
			return diff, fmt.Errorf("delete camera %s: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return diff, err
	}
	rollback = false

	sort.Strings(diff.Added)
	sort.Strings(diff.Updated)
	sort.Strings(diff.Removed)
	sort.Strings(diff.Unchanged)
	return diff, nil
}

// upsertTx is the inner-transaction equivalent of UpsertCamera. It
// encrypts the RTSP password via the provided Cryptostore before
// writing. Must be called from inside an open *sql.Tx.
func upsertTx(
	ctx context.Context, tx *sql.Tx, cam AssignedCamera, crypto Cryptostore,
) error {
	if cam.CameraID == "" {
		return errors.New("state: upsertTx: empty camera_id")
	}
	if cam.Config.ID == "" {
		cam.Config.ID = cam.CameraID
	}
	configJSON, err := json.Marshal(cam.Config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	var cipher []byte
	if cam.RTSPPassword != "" {
		cipher, err = crypto.Encrypt(ctx, []byte(cam.RTSPPassword))
		if err != nil {
			return fmt.Errorf("encrypt rtsp credentials: %w", err)
		}
	}
	now := time.Now().UTC()
	assignedAt := cam.AssignedAt
	if assignedAt.IsZero() {
		assignedAt = now
	}
	var lastPush any
	if cam.LastStatePushAt != nil {
		lastPush = cam.LastStatePushAt.UTC().Format(timeFmt)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO assigned_cameras (
	camera_id, config, config_version, rtsp_credentials,
	assigned_at, updated_at, last_state_push_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(camera_id) DO UPDATE SET
	config = excluded.config,
	config_version = excluded.config_version,
	rtsp_credentials = excluded.rtsp_credentials,
	updated_at = excluded.updated_at,
	last_state_push_at = COALESCE(excluded.last_state_push_at, assigned_cameras.last_state_push_at)
`,
		cam.CameraID,
		string(configJSON),
		cam.ConfigVersion,
		cipher,
		assignedAt.UTC().Format(timeFmt),
		now.Format(timeFmt),
		lastPush,
	)
	return err
}

// cameraDiffers reports whether incoming differs meaningfully from
// existing. It intentionally ignores AssignedAt / UpdatedAt /
// LastStatePushAt (those are Store-maintained).
func cameraDiffers(existing, incoming AssignedCamera) (bool, error) {
	// Prefer explicit config_version if both sides are non-zero.
	if existing.ConfigVersion != 0 && incoming.ConfigVersion != 0 {
		if existing.ConfigVersion != incoming.ConfigVersion {
			return true, nil
		}
		// Versions are equal — assume stable. Fall through to deep
		// check as a belt-and-braces for when a Directory forgets to
		// bump the version.
	}
	a, err := json.Marshal(existing.Config)
	if err != nil {
		return false, fmt.Errorf("marshal existing config: %w", err)
	}
	b, err := json.Marshal(incoming.Config)
	if err != nil {
		return false, fmt.Errorf("marshal incoming config: %w", err)
	}
	if !bytes.Equal(a, b) {
		return true, nil
	}
	if existing.RTSPPassword != incoming.RTSPPassword {
		return true, nil
	}
	return false, nil
}

// ---- helpers -----------------------------------------------------------

// parseTime tolerates both the canonical timeFmt this package writes
// and the slightly-different ISO layout used by the legacy nvr/db
// (`strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`), so migrations from the
// main NVR DB over to the Recorder cache don't trip on format drift.
func parseTime(s string) (time.Time, error) {
	layouts := []string{
		timeFmt,
		"2006-01-02T15:04:05.000Z",
		time.RFC3339Nano,
		time.RFC3339,
	}
	var firstErr error
	for _, layout := range layouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t.UTC(), nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return time.Time{}, firstErr
}
