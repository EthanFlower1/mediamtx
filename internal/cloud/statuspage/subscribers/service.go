package subscribers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	DB         *clouddb.DB
	IDGen      IDGen
	Dispatcher Dispatcher // nil = no-op (useful for RSS-only setups)
}

// Service manages status subscriptions, event logging, and fan-out dispatch.
type Service struct {
	db         *clouddb.DB
	idGen      IDGen
	dispatcher Dispatcher
}

// NewService constructs a Service.
func NewService(cfg Config) (*Service, error) {
	if cfg.DB == nil {
		return nil, errors.New("subscribers: DB is required")
	}
	idGen := cfg.IDGen
	if idGen == nil {
		idGen = defaultIDGen
	}
	return &Service{
		db:         cfg.DB,
		idGen:      idGen,
		dispatcher: cfg.Dispatcher,
	}, nil
}

// Subscribe creates a new subscription for a tenant.
func (s *Service) Subscribe(ctx context.Context, sub Subscriber) (Subscriber, error) {
	if err := validateChannelType(sub.ChannelType); err != nil {
		return Subscriber{}, err
	}
	now := time.Now().UTC()
	if sub.SubscriberID == "" {
		sub.SubscriberID = s.idGen()
	}
	if sub.ChannelConfig == "" {
		sub.ChannelConfig = "{}"
	}
	if sub.ComponentFilter == "" {
		sub.ComponentFilter = "[]"
	}
	// RSS subscriptions are auto-confirmed since there is no push delivery.
	if sub.ChannelType == ChannelRSS {
		sub.Confirmed = true
	}
	sub.CreatedAt = now
	sub.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO status_subscribers (subscriber_id, tenant_id, channel_type, channel_config, component_filter, confirmed, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sub.SubscriberID, sub.TenantID, sub.ChannelType, sub.ChannelConfig,
		sub.ComponentFilter, sub.Confirmed, sub.CreatedAt, sub.UpdatedAt)
	if err != nil {
		return Subscriber{}, fmt.Errorf("subscribe: %w", err)
	}
	return sub, nil
}

// Unsubscribe removes a subscription.
func (s *Service) Unsubscribe(ctx context.Context, tenantID, subscriberID string) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM status_subscribers WHERE tenant_id = ? AND subscriber_id = ?`,
		tenantID, subscriberID)
	if err != nil {
		return fmt.Errorf("unsubscribe: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSubscriberNotFound
	}
	return nil
}

// ConfirmSubscriber marks a subscriber as confirmed (e.g. after email verification).
func (s *Service) ConfirmSubscriber(ctx context.Context, tenantID, subscriberID string) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE status_subscribers SET confirmed = 1, updated_at = ?
		WHERE tenant_id = ? AND subscriber_id = ?`,
		now, tenantID, subscriberID)
	if err != nil {
		return fmt.Errorf("confirm subscriber: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSubscriberNotFound
	}
	return nil
}

// ListSubscribers returns all subscribers for a tenant.
func (s *Service) ListSubscribers(ctx context.Context, tenantID string) ([]Subscriber, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT subscriber_id, tenant_id, channel_type, channel_config, component_filter, confirmed, created_at, updated_at
		FROM status_subscribers
		WHERE tenant_id = ?
		ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list subscribers: %w", err)
	}
	defer rows.Close()

	var out []Subscriber
	for rows.Next() {
		var sub Subscriber
		if err := rows.Scan(&sub.SubscriberID, &sub.TenantID, &sub.ChannelType,
			&sub.ChannelConfig, &sub.ComponentFilter, &sub.Confirmed,
			&sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan subscriber: %w", err)
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

// GetSubscriber returns a single subscriber by ID.
func (s *Service) GetSubscriber(ctx context.Context, tenantID, subscriberID string) (Subscriber, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT subscriber_id, tenant_id, channel_type, channel_config, component_filter, confirmed, created_at, updated_at
		FROM status_subscribers
		WHERE tenant_id = ? AND subscriber_id = ?`,
		tenantID, subscriberID)

	var sub Subscriber
	err := row.Scan(&sub.SubscriberID, &sub.TenantID, &sub.ChannelType,
		&sub.ChannelConfig, &sub.ComponentFilter, &sub.Confirmed,
		&sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		return Subscriber{}, ErrSubscriberNotFound
	}
	return sub, nil
}

// RecordEvent persists a status event and fans out to matching subscribers.
// Only confirmed subscribers whose component_filter matches (or is empty)
// receive the notification.
func (s *Service) RecordEvent(ctx context.Context, evt StatusEvent) (StatusEvent, int, error) {
	now := time.Now().UTC()
	if evt.EventID == "" {
		evt.EventID = s.idGen()
	}
	if evt.AffectedComponents == "" {
		evt.AffectedComponents = "[]"
	}
	evt.CreatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO status_events (event_id, tenant_id, event_type, title, description, affected_components, severity, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		evt.EventID, evt.TenantID, evt.EventType, evt.Title, evt.Description,
		evt.AffectedComponents, evt.Severity, evt.CreatedAt)
	if err != nil {
		return StatusEvent{}, 0, fmt.Errorf("record event: %w", err)
	}

	// Fan out to matching subscribers.
	dispatched, err := s.fanOut(ctx, evt)
	if err != nil {
		return evt, 0, fmt.Errorf("fan out: %w", err)
	}
	return evt, dispatched, nil
}

// ListEvents returns status events for a tenant, most recent first.
func (s *Service) ListEvents(ctx context.Context, tenantID string, limit int) ([]StatusEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT event_id, tenant_id, event_type, title, description, affected_components, severity, created_at
		FROM status_events
		WHERE tenant_id = ?
		ORDER BY created_at DESC
		LIMIT ?`, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	var out []StatusEvent
	for rows.Next() {
		var e StatusEvent
		if err := rows.Scan(&e.EventID, &e.TenantID, &e.EventType, &e.Title,
			&e.Description, &e.AffectedComponents, &e.Severity, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// fanOut dispatches the event to all confirmed subscribers whose component
// filter matches. RSS subscribers are skipped (they poll the feed).
func (s *Service) fanOut(ctx context.Context, evt StatusEvent) (int, error) {
	subs, err := s.ListSubscribers(ctx, evt.TenantID)
	if err != nil {
		return 0, err
	}

	dispatched := 0
	for _, sub := range subs {
		if !sub.Confirmed {
			continue
		}
		// RSS subscribers consume the feed endpoint; no push dispatch.
		if sub.ChannelType == ChannelRSS {
			continue
		}
		if !componentMatches(sub.ComponentFilter, evt.AffectedComponents) {
			continue
		}
		if s.dispatcher != nil {
			if err := s.dispatcher.Dispatch(sub, evt); err != nil {
				// Log but continue dispatching to remaining subscribers.
				continue
			}
		}
		dispatched++
	}
	return dispatched, nil
}

// componentMatches returns true if the subscriber's filter matches the
// event's affected components. An empty filter (or "[]") matches everything.
func componentMatches(filterJSON, affectedJSON string) bool {
	var filter []string
	if err := json.Unmarshal([]byte(filterJSON), &filter); err != nil || len(filter) == 0 {
		return true // no filter = match all
	}
	var affected []string
	if err := json.Unmarshal([]byte(affectedJSON), &affected); err != nil {
		return true // can't parse = match to be safe
	}
	filterSet := make(map[string]struct{}, len(filter))
	for _, f := range filter {
		filterSet[f] = struct{}{}
	}
	for _, a := range affected {
		if _, ok := filterSet[a]; ok {
			return true
		}
	}
	return false
}

func validateChannelType(ct ChannelType) error {
	switch ct {
	case ChannelEmail, ChannelSMS, ChannelWebhook, ChannelRSS, ChannelSlack, ChannelTeams:
		return nil
	default:
		return ErrInvalidChannel
	}
}
