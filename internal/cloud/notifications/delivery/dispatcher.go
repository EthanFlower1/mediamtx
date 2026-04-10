package delivery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

// IDGen generates a random hex ID.
type IDGen func() string

func defaultIDGen() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// UserResolver looks up a user's contact info (email, phone) for delivery.
// Production wires this to the user store; tests use a fake.
type UserResolver interface {
	ResolveContact(ctx context.Context, tenantID, userID string, channelType notifications.ChannelType) (string, error)
}

// DispatcherConfig bundles dependencies for the Dispatcher.
type DispatcherConfig struct {
	DB           *clouddb.DB
	NotifService *notifications.Service
	Senders      map[notifications.ChannelType]Sender
	RateLimiter  *RateLimiter
	UserResolver UserResolver
	IDGen        IDGen
}

// Dispatcher orchestrates notification delivery: routing -> rate limiting ->
// sending -> status logging. It is the top-level entry point for dispatching
// a notification event to all subscribed users via their preferred channels.
type Dispatcher struct {
	db           *clouddb.DB
	notifService *notifications.Service
	senders      map[notifications.ChannelType]Sender
	rateLimiter  *RateLimiter
	userResolver UserResolver
	idGen        IDGen
}

// NewDispatcher constructs a Dispatcher.
func NewDispatcher(cfg DispatcherConfig) (*Dispatcher, error) {
	if cfg.DB == nil {
		return nil, errors.New("delivery: DB is required")
	}
	if cfg.NotifService == nil {
		return nil, errors.New("delivery: NotifService is required")
	}
	if cfg.RateLimiter == nil {
		cfg.RateLimiter = NewRateLimiter()
	}
	if cfg.IDGen == nil {
		cfg.IDGen = defaultIDGen
	}
	return &Dispatcher{
		db:           cfg.DB,
		notifService: cfg.NotifService,
		senders:      cfg.Senders,
		rateLimiter:  cfg.RateLimiter,
		userResolver: cfg.UserResolver,
		idGen:        cfg.IDGen,
	}, nil
}

// Dispatch sends a notification for the given event type to all subscribed
// users in the tenant. It resolves delivery targets via RouteNotification,
// applies per-tenant rate limits, delivers via the appropriate sender, and
// logs each attempt.
func (d *Dispatcher) Dispatch(ctx context.Context, tenantID, eventType, subject, body string) ([]notifications.LogEntry, error) {
	targets, err := d.notifService.RouteNotification(ctx, tenantID, eventType)
	if err != nil {
		return nil, fmt.Errorf("dispatch: route: %w", err)
	}

	var entries []notifications.LogEntry

	for _, target := range targets {
		entry := notifications.LogEntry{
			LogID:       d.idGen(),
			TenantID:    tenantID,
			UserID:      target.UserID,
			EventType:   eventType,
			ChannelType: target.Channel.ChannelType,
		}

		// Rate limit check.
		rlKey := fmt.Sprintf("%s:%s", tenantID, target.Channel.ChannelType)
		if !d.rateLimiter.Allow(rlKey) {
			entry.Status = notifications.StatusSuppressed
			entry.ErrorMessage = "rate limit exceeded"
			d.logEntry(ctx, entry)
			entries = append(entries, entry)
			continue
		}

		// Look up sender for this channel type.
		sender, ok := d.senders[target.Channel.ChannelType]
		if !ok {
			entry.Status = notifications.StatusFailed
			entry.ErrorMessage = fmt.Sprintf("no sender configured for channel type %s", target.Channel.ChannelType)
			d.logEntry(ctx, entry)
			entries = append(entries, entry)
			continue
		}

		// Resolve user contact info.
		var to string
		if d.userResolver != nil {
			to, err = d.userResolver.ResolveContact(ctx, tenantID, target.UserID, target.Channel.ChannelType)
			if err != nil {
				entry.Status = notifications.StatusFailed
				entry.ErrorMessage = fmt.Sprintf("resolve contact: %v", err)
				d.logEntry(ctx, entry)
				entries = append(entries, entry)
				continue
			}
		}

		// Send.
		result := sender.Send(ctx, Message{
			TenantID:  tenantID,
			UserID:    target.UserID,
			EventType: eventType,
			To:        to,
			Subject:   subject,
			Body:      body,
		})

		if result.Error != nil {
			entry.Status = notifications.StatusFailed
			entry.ErrorMessage = result.Error.Error()
		} else {
			entry.Status = notifications.StatusSent
			now := time.Now().UTC()
			entry.SentAt = &now
		}

		d.logEntry(ctx, entry)
		entries = append(entries, entry)
	}

	return entries, nil
}

// UpsertRateLimit creates or updates a rate limit config and applies it
// to the in-memory rate limiter.
func (d *Dispatcher) UpsertRateLimit(ctx context.Context, rl RateLimit) (RateLimit, error) {
	now := time.Now().UTC()
	if rl.RateLimitID == "" {
		rl.RateLimitID = d.idGen()
	}
	rl.UpdatedAt = now

	_, err := d.db.ExecContext(ctx, `
		INSERT INTO notification_rate_limits (rate_limit_id, tenant_id, channel_type, window_seconds, max_count, burst, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (rate_limit_id, tenant_id) DO UPDATE SET
			channel_type = excluded.channel_type,
			window_seconds = excluded.window_seconds,
			max_count = excluded.max_count,
			burst = excluded.burst,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at`,
		rl.RateLimitID, rl.TenantID, rl.ChannelType, rl.WindowSeconds,
		rl.MaxCount, rl.Burst, rl.Enabled, now, now)
	if err != nil {
		return RateLimit{}, fmt.Errorf("upsert rate limit: %w", err)
	}
	rl.CreatedAt = now

	// Apply to in-memory limiter.
	if rl.Enabled {
		key := fmt.Sprintf("%s:%s", rl.TenantID, rl.ChannelType)
		d.rateLimiter.Configure(key, rl.WindowSeconds, rl.MaxCount)
	}

	return rl, nil
}

// GetRateLimit returns the rate limit config for a tenant and channel type.
func (d *Dispatcher) GetRateLimit(ctx context.Context, tenantID, channelType string) (RateLimit, error) {
	var rl RateLimit
	err := d.db.QueryRowContext(ctx, `
		SELECT rate_limit_id, tenant_id, channel_type, window_seconds, max_count, burst, enabled, created_at, updated_at
		FROM notification_rate_limits
		WHERE tenant_id = ? AND channel_type = ?`,
		tenantID, channelType).Scan(
		&rl.RateLimitID, &rl.TenantID, &rl.ChannelType, &rl.WindowSeconds,
		&rl.MaxCount, &rl.Burst, &rl.Enabled, &rl.CreatedAt, &rl.UpdatedAt)
	if err != nil {
		return RateLimit{}, fmt.Errorf("get rate limit: %w", err)
	}
	return rl, nil
}

// UpsertProviderConfig creates or updates a delivery provider config.
func (d *Dispatcher) UpsertProviderConfig(ctx context.Context, pc ProviderConfig) (ProviderConfig, error) {
	now := time.Now().UTC()
	if pc.ProviderID == "" {
		pc.ProviderID = d.idGen()
	}
	if pc.Credentials == "" {
		pc.Credentials = "{}"
	}
	pc.UpdatedAt = now

	_, err := d.db.ExecContext(ctx, `
		INSERT INTO notification_delivery_providers (provider_id, tenant_id, channel_type, provider_name, credentials, from_address, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (provider_id, tenant_id) DO UPDATE SET
			channel_type = excluded.channel_type,
			provider_name = excluded.provider_name,
			credentials = excluded.credentials,
			from_address = excluded.from_address,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at`,
		pc.ProviderID, pc.TenantID, pc.ChannelType, pc.ProviderName,
		pc.Credentials, pc.FromAddress, pc.Enabled, now, now)
	if err != nil {
		return ProviderConfig{}, fmt.Errorf("upsert provider config: %w", err)
	}
	pc.CreatedAt = now
	return pc, nil
}

// GetProviderConfig returns the delivery provider config for a tenant and channel type.
func (d *Dispatcher) GetProviderConfig(ctx context.Context, tenantID, channelType string) (ProviderConfig, error) {
	var pc ProviderConfig
	err := d.db.QueryRowContext(ctx, `
		SELECT provider_id, tenant_id, channel_type, provider_name, credentials, from_address, enabled, created_at, updated_at
		FROM notification_delivery_providers
		WHERE tenant_id = ? AND channel_type = ?`,
		tenantID, channelType).Scan(
		&pc.ProviderID, &pc.TenantID, &pc.ChannelType, &pc.ProviderName,
		&pc.Credentials, &pc.FromAddress, &pc.Enabled, &pc.CreatedAt, &pc.UpdatedAt)
	if err != nil {
		return ProviderConfig{}, fmt.Errorf("get provider config: %w", err)
	}
	return pc, nil
}

func (d *Dispatcher) logEntry(ctx context.Context, entry notifications.LogEntry) {
	_ = d.notifService.LogDelivery(ctx, entry)
}
