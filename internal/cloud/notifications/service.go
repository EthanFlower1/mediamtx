package notifications

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

// Service manages notification channels, user preferences, and delivery routing.
type Service struct {
	db    *clouddb.DB
	idGen IDGen
}

// NewService constructs a Service.
func NewService(cfg Config) (*Service, error) {
	if cfg.DB == nil {
		return nil, errors.New("notifications: DB is required")
	}
	idGen := cfg.IDGen
	if idGen == nil {
		idGen = defaultIDGen
	}
	return &Service{db: cfg.DB, idGen: idGen}, nil
}

// UpsertChannel creates or updates a notification channel for a tenant.
func (s *Service) UpsertChannel(ctx context.Context, ch Channel) (Channel, error) {
	now := time.Now().UTC()
	if ch.ChannelID == "" {
		ch.ChannelID = s.idGen()
	}
	ch.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO notification_channels (channel_id, tenant_id, channel_type, config, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (channel_id, tenant_id) DO UPDATE SET
			channel_type = excluded.channel_type,
			config = excluded.config,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at`,
		ch.ChannelID, ch.TenantID, ch.ChannelType, ch.Config, ch.Enabled, now, now)
	if err != nil {
		return Channel{}, fmt.Errorf("upsert channel: %w", err)
	}
	ch.CreatedAt = now
	return ch, nil
}

// ListChannels returns all notification channels for a tenant.
func (s *Service) ListChannels(ctx context.Context, tenantID string) ([]Channel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT channel_id, tenant_id, channel_type, config, enabled, created_at, updated_at
		FROM notification_channels
		WHERE tenant_id = ?
		ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	defer rows.Close()

	var out []Channel
	for rows.Next() {
		var ch Channel
		if err := rows.Scan(&ch.ChannelID, &ch.TenantID, &ch.ChannelType, &ch.Config, &ch.Enabled, &ch.CreatedAt, &ch.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan channel: %w", err)
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

// DeleteChannel removes a notification channel.
func (s *Service) DeleteChannel(ctx context.Context, tenantID, channelID string) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM notification_channels WHERE tenant_id = ? AND channel_id = ?`,
		tenantID, channelID)
	if err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrChannelNotFound
	}
	return nil
}

// SetPreference upserts a user's notification preference for a specific
// event type and channel type.
func (s *Service) SetPreference(ctx context.Context, tenantID, userID, eventType string, channelType ChannelType, enabled bool) error {
	now := time.Now().UTC()
	id := s.idGen()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO notification_preferences (preference_id, tenant_id, user_id, event_type, channel_type, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, user_id, event_type, channel_type) DO UPDATE SET
			enabled = excluded.enabled,
			updated_at = excluded.updated_at`,
		id, tenantID, userID, eventType, channelType, enabled, now, now)
	if err != nil {
		return fmt.Errorf("set preference: %w", err)
	}
	return nil
}

// GetPreferences returns all notification preferences for a user within a tenant.
func (s *Service) GetPreferences(ctx context.Context, tenantID, userID string) ([]Preference, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT preference_id, tenant_id, user_id, event_type, channel_type, enabled, created_at, updated_at
		FROM notification_preferences
		WHERE tenant_id = ? AND user_id = ?
		ORDER BY event_type, channel_type`, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("get preferences: %w", err)
	}
	defer rows.Close()

	var out []Preference
	for rows.Next() {
		var p Preference
		if err := rows.Scan(&p.PreferenceID, &p.TenantID, &p.UserID, &p.EventType, &p.ChannelType, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan preference: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// RouteNotification resolves which channels should receive a notification
// for the given event type. It returns all enabled channels for which at
// least one user in the tenant has an enabled preference matching the
// event type and channel type.
func (s *Service) RouteNotification(ctx context.Context, tenantID, eventType string) ([]DeliveryTarget, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.user_id, c.channel_id, c.tenant_id, c.channel_type, c.config, c.enabled, c.created_at, c.updated_at
		FROM notification_preferences p
		JOIN notification_channels c ON c.tenant_id = p.tenant_id AND c.channel_type = p.channel_type
		WHERE p.tenant_id = ? AND p.event_type = ? AND p.enabled = 1 AND c.enabled = 1`,
		tenantID, eventType)
	if err != nil {
		return nil, fmt.Errorf("route notification: %w", err)
	}
	defer rows.Close()

	var out []DeliveryTarget
	for rows.Next() {
		var dt DeliveryTarget
		if err := rows.Scan(&dt.UserID, &dt.Channel.ChannelID, &dt.Channel.TenantID, &dt.Channel.ChannelType,
			&dt.Channel.Config, &dt.Channel.Enabled, &dt.Channel.CreatedAt, &dt.Channel.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan delivery target: %w", err)
		}
		out = append(out, dt)
	}
	return out, rows.Err()
}

// LogDelivery records a notification delivery attempt.
func (s *Service) LogDelivery(ctx context.Context, entry LogEntry) error {
	if entry.LogID == "" {
		entry.LogID = s.idGen()
	}
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO notification_log (log_id, tenant_id, user_id, event_type, channel_type, status, error_message, sent_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.LogID, entry.TenantID, entry.UserID, entry.EventType, entry.ChannelType,
		entry.Status, entry.ErrorMessage, entry.SentAt, now)
	if err != nil {
		return fmt.Errorf("log delivery: %w", err)
	}
	return nil
}
