package legacydb

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Federation represents a federation group that this NVR belongs to.
type Federation struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// FederationPeer represents a peer NVR in a federation.
type FederationPeer struct {
	ID           string `json:"id"`
	FederationID string `json:"federation_id"`
	Token        string `json:"token"`
	Status       string `json:"status"` // "pending", "connected", "disconnected"
	JoinedAt     string `json:"joined_at"`
}

// GetFederation returns the current federation, or nil if none exists.
func (d *DB) GetFederation() (*Federation, error) {
	row := d.DB.QueryRow(
		`SELECT id, name, created_at FROM federations LIMIT 1`,
	)
	var f Federation
	err := row.Scan(&f.ID, &f.Name, &f.CreatedAt)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("get federation: %w", err)
	}
	return &f, nil
}

// ListFederationPeers returns all peers for a given federation.
func (d *DB) ListFederationPeers(federationID string) ([]FederationPeer, error) {
	rows, err := d.DB.Query(
		`SELECT id, federation_id, token, status, joined_at
		 FROM federation_peers WHERE federation_id = ?`, federationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list federation peers: %w", err)
	}
	defer rows.Close()

	var peers []FederationPeer
	for rows.Next() {
		var p FederationPeer
		if err := rows.Scan(&p.ID, &p.FederationID, &p.Token, &p.Status, &p.JoinedAt); err != nil {
			return nil, fmt.Errorf("scan federation peer: %w", err)
		}
		peers = append(peers, p)
	}
	return peers, rows.Err()
}

// CreateFederation creates a new federation.
func (d *DB) CreateFederation(name string) (*Federation, error) {
	id := uuid.New().String()[:12]
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := d.DB.Exec(
		`INSERT INTO federations (id, name, created_at) VALUES (?, ?, ?)`,
		id, name, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create federation: %w", err)
	}
	return &Federation{ID: id, Name: name, CreatedAt: now}, nil
}

// DeleteFederation removes a federation and its peers.
func (d *DB) DeleteFederation(id string) error {
	_, err := d.DB.Exec(`DELETE FROM federation_peers WHERE federation_id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete federation peers: %w", err)
	}
	_, err = d.DB.Exec(`DELETE FROM federations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete federation: %w", err)
	}
	return nil
}

// AddFederationPeer adds a peer to a federation.
func (d *DB) AddFederationPeer(federationID, token string) (*FederationPeer, error) {
	id := uuid.New().String()[:12]
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := d.DB.Exec(
		`INSERT INTO federation_peers (id, federation_id, token, status, joined_at)
		 VALUES (?, ?, ?, 'pending', ?)`,
		id, federationID, token, now,
	)
	if err != nil {
		return nil, fmt.Errorf("add federation peer: %w", err)
	}
	return &FederationPeer{
		ID:           id,
		FederationID: federationID,
		Token:        token,
		Status:       "pending",
		JoinedAt:     now,
	}, nil
}

// RemoveFederationPeer removes a peer from its federation.
func (d *DB) RemoveFederationPeer(id string) error {
	_, err := d.DB.Exec(`DELETE FROM federation_peers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("remove federation peer: %w", err)
	}
	return nil
}
