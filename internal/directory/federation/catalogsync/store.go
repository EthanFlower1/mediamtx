// Package catalogsync implements periodic pull-based synchronisation of
// cameras, users, and groups from federated peer Directories (KAI-271).
//
// Each peer gets its own background goroutine that calls the peer's
// ListCameras, ListUsers, and ListGroups RPCs and upserts the results
// into local SQLite cache tables. Per-peer failures are isolated: one
// unreachable peer does not block synchronisation of the others.
package catalogsync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	directorydb "github.com/bluenviron/mediamtx/internal/directory/db"
)

// SyncStatus represents the synchronisation state of a peer.
type SyncStatus string

const (
	StatusPending SyncStatus = "pending"
	StatusSynced  SyncStatus = "synced"
	StatusError   SyncStatus = "error"
	StatusStale   SyncStatus = "stale"
)

// PeerRecord is the DB row shape for a federation peer.
type PeerRecord struct {
	PeerID     string
	Name       string
	Endpoint   string
	TenantID   string
	TenantType int32
	LastSyncAt *time.Time
	SyncStatus SyncStatus
	SyncError  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CachedCamera is a locally cached camera from a federated peer.
type CachedCamera struct {
	ID           string
	PeerID       string
	Name         string
	RecorderID   string
	Manufacturer string
	Model        string
	IPAddress    string
	State        int32
	Labels       []string
	SyncedAt     time.Time
}

// CachedUser is a locally cached user from a federated peer.
type CachedUser struct {
	ID          string
	PeerID      string
	Username    string
	Email       string
	DisplayName string
	Groups      []string
	Disabled    bool
	SyncedAt    time.Time
}

// CachedGroup is a locally cached group from a federated peer.
type CachedGroup struct {
	ID          string
	PeerID      string
	Name        string
	Description string
	SyncedAt    time.Time
}

// Store manages the federated catalog cache tables in SQLite.
type Store struct {
	db *directorydb.DB
}

// NewStore constructs a Store backed by the directory DB.
func NewStore(db *directorydb.DB) *Store {
	return &Store{db: db}
}

// UpsertPeer creates or updates the federation_peers row. This is called
// when a new peer is registered and on every sync cycle to update status.
func (s *Store) UpsertPeer(ctx context.Context, p *PeerRecord) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO federation_peers (peer_id, name, endpoint, tenant_id, tenant_type, last_sync_at, sync_status, sync_error, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(peer_id) DO UPDATE SET
		   name = excluded.name,
		   endpoint = excluded.endpoint,
		   tenant_id = excluded.tenant_id,
		   tenant_type = excluded.tenant_type,
		   last_sync_at = excluded.last_sync_at,
		   sync_status = excluded.sync_status,
		   sync_error = excluded.sync_error,
		   updated_at = CURRENT_TIMESTAMP`,
		p.PeerID, p.Name, p.Endpoint, p.TenantID, p.TenantType,
		formatNullableTime(p.LastSyncAt), string(p.SyncStatus), p.SyncError,
	)
	if err != nil {
		return fmt.Errorf("catalogsync/store: upsert peer: %w", err)
	}
	return nil
}

// GetPeer returns the peer record for the given peer ID.
func (s *Store) GetPeer(ctx context.Context, peerID string) (*PeerRecord, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT peer_id, name, endpoint, tenant_id, tenant_type,
		        last_sync_at, sync_status, sync_error, created_at, updated_at
		   FROM federation_peers WHERE peer_id = ?`, peerID)
	return scanPeer(row)
}

// ListPeers returns all registered federation peers.
func (s *Store) ListPeers(ctx context.Context) ([]*PeerRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT peer_id, name, endpoint, tenant_id, tenant_type,
		        last_sync_at, sync_status, sync_error, created_at, updated_at
		   FROM federation_peers ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("catalogsync/store: list peers: %w", err)
	}
	defer rows.Close()

	var out []*PeerRecord
	for rows.Next() {
		p, err := scanPeerRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ReplaceCameras atomically replaces all cached cameras for the given peer.
func (s *Store) ReplaceCameras(ctx context.Context, peerID string, cameras []CachedCamera) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("catalogsync/store: begin tx cameras: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM federated_cameras WHERE peer_id = ?`, peerID); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("catalogsync/store: delete cameras: %w", err)
	}
	for _, c := range cameras {
		labelsJSON, _ := json.Marshal(c.Labels)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO federated_cameras (id, peer_id, name, recorder_id, manufacturer, model, ip_address, state, labels, synced_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.ID, peerID, c.Name, c.RecorderID, c.Manufacturer, c.Model, c.IPAddress, c.State, string(labelsJSON), c.SyncedAt.UTC().Format(time.RFC3339),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("catalogsync/store: insert camera %s: %w", c.ID, err)
		}
	}
	return tx.Commit()
}

// ReplaceUsers atomically replaces all cached users for the given peer.
func (s *Store) ReplaceUsers(ctx context.Context, peerID string, users []CachedUser) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("catalogsync/store: begin tx users: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM federated_users WHERE peer_id = ?`, peerID); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("catalogsync/store: delete users: %w", err)
	}
	for _, u := range users {
		groupsJSON, _ := json.Marshal(u.Groups)
		disabled := 0
		if u.Disabled {
			disabled = 1
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO federated_users (id, peer_id, username, email, display_name, groups, disabled, synced_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			u.ID, peerID, u.Username, u.Email, u.DisplayName, string(groupsJSON), disabled, u.SyncedAt.UTC().Format(time.RFC3339),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("catalogsync/store: insert user %s: %w", u.ID, err)
		}
	}
	return tx.Commit()
}

// ReplaceGroups atomically replaces all cached groups for the given peer.
func (s *Store) ReplaceGroups(ctx context.Context, peerID string, groups []CachedGroup) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("catalogsync/store: begin tx groups: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM federated_groups WHERE peer_id = ?`, peerID); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("catalogsync/store: delete groups: %w", err)
	}
	for _, g := range groups {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO federated_groups (id, peer_id, name, description, synced_at)
			 VALUES (?, ?, ?, ?, ?)`,
			g.ID, peerID, g.Name, g.Description, g.SyncedAt.UTC().Format(time.RFC3339),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("catalogsync/store: insert group %s: %w", g.ID, err)
		}
	}
	return tx.Commit()
}

// ListCamerasByPeer returns all cached cameras for the given peer.
func (s *Store) ListCamerasByPeer(ctx context.Context, peerID string) ([]CachedCamera, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, peer_id, name, recorder_id, manufacturer, model, ip_address, state, labels, synced_at
		   FROM federated_cameras WHERE peer_id = ? ORDER BY name ASC`, peerID)
	if err != nil {
		return nil, fmt.Errorf("catalogsync/store: list cameras: %w", err)
	}
	defer rows.Close()

	var out []CachedCamera
	for rows.Next() {
		var c CachedCamera
		var labelsJSON, syncedRaw string
		if err := rows.Scan(&c.ID, &c.PeerID, &c.Name, &c.RecorderID, &c.Manufacturer, &c.Model, &c.IPAddress, &c.State, &labelsJSON, &syncedRaw); err != nil {
			return nil, fmt.Errorf("catalogsync/store: scan camera: %w", err)
		}
		_ = json.Unmarshal([]byte(labelsJSON), &c.Labels)
		if t, err := time.Parse(time.RFC3339, syncedRaw); err == nil {
			c.SyncedAt = t.UTC()
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListUsersByPeer returns all cached users for the given peer.
func (s *Store) ListUsersByPeer(ctx context.Context, peerID string) ([]CachedUser, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, peer_id, username, email, display_name, groups, disabled, synced_at
		   FROM federated_users WHERE peer_id = ? ORDER BY username ASC`, peerID)
	if err != nil {
		return nil, fmt.Errorf("catalogsync/store: list users: %w", err)
	}
	defer rows.Close()

	var out []CachedUser
	for rows.Next() {
		var u CachedUser
		var groupsJSON, syncedRaw string
		var disabled int
		if err := rows.Scan(&u.ID, &u.PeerID, &u.Username, &u.Email, &u.DisplayName, &groupsJSON, &disabled, &syncedRaw); err != nil {
			return nil, fmt.Errorf("catalogsync/store: scan user: %w", err)
		}
		_ = json.Unmarshal([]byte(groupsJSON), &u.Groups)
		u.Disabled = disabled != 0
		if t, err := time.Parse(time.RFC3339, syncedRaw); err == nil {
			u.SyncedAt = t.UTC()
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ListGroupsByPeer returns all cached groups for the given peer.
func (s *Store) ListGroupsByPeer(ctx context.Context, peerID string) ([]CachedGroup, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, peer_id, name, description, synced_at
		   FROM federated_groups WHERE peer_id = ? ORDER BY name ASC`, peerID)
	if err != nil {
		return nil, fmt.Errorf("catalogsync/store: list groups: %w", err)
	}
	defer rows.Close()

	var out []CachedGroup
	for rows.Next() {
		var g CachedGroup
		var syncedRaw string
		if err := rows.Scan(&g.ID, &g.PeerID, &g.Name, &g.Description, &syncedRaw); err != nil {
			return nil, fmt.Errorf("catalogsync/store: scan group: %w", err)
		}
		if t, err := time.Parse(time.RFC3339, syncedRaw); err == nil {
			g.SyncedAt = t.UTC()
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// MarkStale updates peers whose last successful sync is older than the
// given threshold to "stale" status. Returns the number of rows updated.
func (s *Store) MarkStale(ctx context.Context, threshold time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE federation_peers
		    SET sync_status = 'stale', updated_at = CURRENT_TIMESTAMP
		  WHERE sync_status = 'synced'
		    AND (last_sync_at IS NULL OR last_sync_at < ?)`,
		threshold.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("catalogsync/store: mark stale: %w", err)
	}
	return res.RowsAffected()
}

// --- helpers ----------------------------------------------------------------

func formatNullableTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func scanPeer(row *sql.Row) (*PeerRecord, error) {
	var p PeerRecord
	var lastSyncRaw, createdRaw, updatedRaw sql.NullString
	if err := row.Scan(
		&p.PeerID, &p.Name, &p.Endpoint, &p.TenantID, &p.TenantType,
		&lastSyncRaw, (*string)(&p.SyncStatus), &p.SyncError, &createdRaw, &updatedRaw,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("catalogsync/store: peer not found")
		}
		return nil, fmt.Errorf("catalogsync/store: scan peer: %w", err)
	}
	parseTimes(&p, lastSyncRaw, createdRaw, updatedRaw)
	return &p, nil
}

type scannable interface {
	Scan(dest ...interface{}) error
}

func scanPeerRows(row scannable) (*PeerRecord, error) {
	var p PeerRecord
	var lastSyncRaw, createdRaw, updatedRaw sql.NullString
	if err := row.Scan(
		&p.PeerID, &p.Name, &p.Endpoint, &p.TenantID, &p.TenantType,
		&lastSyncRaw, (*string)(&p.SyncStatus), &p.SyncError, &createdRaw, &updatedRaw,
	); err != nil {
		return nil, fmt.Errorf("catalogsync/store: scan peer: %w", err)
	}
	parseTimes(&p, lastSyncRaw, createdRaw, updatedRaw)
	return &p, nil
}

func parseTimes(p *PeerRecord, lastSyncRaw, createdRaw, updatedRaw sql.NullString) {
	if lastSyncRaw.Valid && lastSyncRaw.String != "" {
		if t, err := time.Parse(time.RFC3339, lastSyncRaw.String); err == nil {
			t = t.UTC()
			p.LastSyncAt = &t
		}
	}
	if createdRaw.Valid {
		if t, err := time.Parse(time.RFC3339, createdRaw.String); err == nil {
			p.CreatedAt = t.UTC()
		}
	}
	if updatedRaw.Valid {
		if t, err := time.Parse(time.RFC3339, updatedRaw.String); err == nil {
			p.UpdatedAt = t.UTC()
		}
	}
}
