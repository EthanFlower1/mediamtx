package support

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
)

// IDGen generates a random hex ID.
type IDGen func() string

func defaultIDGen() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Config bundles dependencies for Service.
type Config struct {
	DB    *clouddb.DB
	IDGen IDGen
}

// Service manages support tickets, comments, and provider configurations.
type Service struct {
	db    *clouddb.DB
	idGen IDGen
}

// NewService constructs a Service.
func NewService(cfg Config) (*Service, error) {
	if cfg.DB == nil {
		return nil, errors.New("support: DB is required")
	}
	idGen := cfg.IDGen
	if idGen == nil {
		idGen = defaultIDGen
	}
	return &Service{db: cfg.DB, idGen: idGen}, nil
}

// CreateTicket creates a new support ticket.
func (s *Service) CreateTicket(ctx context.Context, t Ticket) (Ticket, error) {
	now := time.Now().UTC()
	if t.TicketID == "" {
		t.TicketID = s.idGen()
	}
	if t.Provider == "" {
		t.Provider = ProviderInternal
	}
	if t.Status == "" {
		t.Status = StatusOpen
	}
	if t.Priority == "" {
		t.Priority = PriorityNormal
	}
	if t.Tags == "" {
		t.Tags = "[]"
	}
	if t.Metadata == "" {
		t.Metadata = "{}"
	}
	t.CreatedAt = now
	t.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO support_tickets (ticket_id, tenant_id, external_id, provider, subject, description, status, priority, requester_id, assignee_id, tags, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.TicketID, t.TenantID, t.ExternalID, t.Provider, t.Subject, t.Description,
		t.Status, t.Priority, t.RequesterID, t.AssigneeID, t.Tags, t.Metadata,
		t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return Ticket{}, fmt.Errorf("create ticket: %w", err)
	}
	return t, nil
}

// GetTicket retrieves a ticket by ID within a tenant.
func (s *Service) GetTicket(ctx context.Context, tenantID, ticketID string) (Ticket, error) {
	var t Ticket
	err := s.db.QueryRowContext(ctx, `
		SELECT ticket_id, tenant_id, external_id, provider, subject, description, status, priority, requester_id, assignee_id, tags, metadata, created_at, updated_at
		FROM support_tickets
		WHERE tenant_id = ? AND ticket_id = ?`,
		tenantID, ticketID).Scan(
		&t.TicketID, &t.TenantID, &t.ExternalID, &t.Provider, &t.Subject, &t.Description,
		&t.Status, &t.Priority, &t.RequesterID, &t.AssigneeID, &t.Tags, &t.Metadata,
		&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return Ticket{}, ErrTicketNotFound
	}
	return t, nil
}

// ListTickets returns tickets for a tenant filtered by status.
func (s *Service) ListTickets(ctx context.Context, tenantID string, status *TicketStatus) ([]Ticket, error) {
	var rows interface {
		Next() bool
		Scan(dest ...interface{}) error
		Close() error
		Err() error
	}
	var err error

	if status != nil {
		rows, err = s.db.QueryContext(ctx, `
			SELECT ticket_id, tenant_id, external_id, provider, subject, description, status, priority, requester_id, assignee_id, tags, metadata, created_at, updated_at
			FROM support_tickets
			WHERE tenant_id = ? AND status = ?
			ORDER BY updated_at DESC`, tenantID, *status)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT ticket_id, tenant_id, external_id, provider, subject, description, status, priority, requester_id, assignee_id, tags, metadata, created_at, updated_at
			FROM support_tickets
			WHERE tenant_id = ?
			ORDER BY updated_at DESC`, tenantID)
	}
	if err != nil {
		return nil, fmt.Errorf("list tickets: %w", err)
	}
	defer rows.Close()

	var out []Ticket
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(&t.TicketID, &t.TenantID, &t.ExternalID, &t.Provider, &t.Subject, &t.Description,
			&t.Status, &t.Priority, &t.RequesterID, &t.AssigneeID, &t.Tags, &t.Metadata,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan ticket: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpdateTicketStatus updates a ticket's status.
func (s *Service) UpdateTicketStatus(ctx context.Context, tenantID, ticketID string, status TicketStatus) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE support_tickets SET status = ?, updated_at = ?
		WHERE tenant_id = ? AND ticket_id = ?`,
		status, now, tenantID, ticketID)
	if err != nil {
		return fmt.Errorf("update ticket status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrTicketNotFound
	}
	return nil
}

// AddComment adds a comment to a ticket.
func (s *Service) AddComment(ctx context.Context, c Comment) (Comment, error) {
	now := time.Now().UTC()
	if c.CommentID == "" {
		c.CommentID = s.idGen()
	}
	if c.Source == "" {
		c.Source = SourceUser
	}
	c.CreatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO support_ticket_comments (comment_id, ticket_id, tenant_id, author_id, body, source, is_public, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.CommentID, c.TicketID, c.TenantID, c.AuthorID, c.Body, c.Source, c.IsPublic, c.CreatedAt)
	if err != nil {
		return Comment{}, fmt.Errorf("add comment: %w", err)
	}
	return c, nil
}

// ListComments returns all comments for a ticket, oldest first.
func (s *Service) ListComments(ctx context.Context, tenantID, ticketID string) ([]Comment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT comment_id, ticket_id, tenant_id, author_id, body, source, is_public, created_at
		FROM support_ticket_comments
		WHERE tenant_id = ? AND ticket_id = ?
		ORDER BY created_at`, tenantID, ticketID)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()

	var out []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.CommentID, &c.TicketID, &c.TenantID, &c.AuthorID, &c.Body, &c.Source, &c.IsPublic, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpsertProviderConfig creates or updates an external provider configuration.
func (s *Service) UpsertProviderConfig(ctx context.Context, pc ProviderConfig) (ProviderConfig, error) {
	now := time.Now().UTC()
	if pc.ConfigID == "" {
		pc.ConfigID = s.idGen()
	}
	if pc.APICredentials == "" {
		pc.APICredentials = "{}"
	}
	pc.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO support_provider_configs (config_id, tenant_id, provider, webhook_secret, api_credentials, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (config_id, tenant_id) DO UPDATE SET
			provider = excluded.provider,
			webhook_secret = excluded.webhook_secret,
			api_credentials = excluded.api_credentials,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at`,
		pc.ConfigID, pc.TenantID, pc.Provider, pc.WebhookSecret, pc.APICredentials,
		pc.Enabled, now, now)
	if err != nil {
		return ProviderConfig{}, fmt.Errorf("upsert provider config: %w", err)
	}
	pc.CreatedAt = now
	return pc, nil
}

// GetProviderConfig returns the provider configuration for a tenant and provider.
func (s *Service) GetProviderConfig(ctx context.Context, tenantID string, provider Provider) (ProviderConfig, error) {
	var pc ProviderConfig
	err := s.db.QueryRowContext(ctx, `
		SELECT config_id, tenant_id, provider, webhook_secret, api_credentials, enabled, created_at, updated_at
		FROM support_provider_configs
		WHERE tenant_id = ? AND provider = ?`,
		tenantID, provider).Scan(
		&pc.ConfigID, &pc.TenantID, &pc.Provider, &pc.WebhookSecret, &pc.APICredentials,
		&pc.Enabled, &pc.CreatedAt, &pc.UpdatedAt)
	if err != nil {
		return ProviderConfig{}, ErrProviderNotFound
	}
	return pc, nil
}
