package notifications

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// -----------------------------------------------------------------------
// Errors
// -----------------------------------------------------------------------

var (
	// ErrNoChannel is returned when no registered channel supports
	// the requested message type.
	ErrNoChannel = errors.New("notifications: no channel supports message type")

	// ErrDuplicateMessage is returned (as a non-error) when a
	// message ID has already been delivered. The caller can inspect
	// the returned DeliveryResult for the original outcome.
	ErrDuplicateMessage = errors.New("notifications: duplicate message (idempotent skip)")

	// ErrMaxRetriesExceeded is returned when a message exhausts its
	// retry budget and is moved to the dead-letter queue.
	ErrMaxRetriesExceeded = errors.New("notifications: max retries exceeded, message dead-lettered")
)

// -----------------------------------------------------------------------
// Idempotency store
// -----------------------------------------------------------------------

// IdempotencyStore tracks message IDs that have already been
// processed. Implementations MUST be safe for concurrent use.
type IdempotencyStore interface {
	// Exists reports whether messageID has been recorded.
	Exists(ctx context.Context, messageID string) (bool, error)
	// Record stores messageID so future Exists calls return true.
	// It MUST be idempotent: recording the same ID twice is a no-op.
	Record(ctx context.Context, messageID string) error
}

// MemoryIdempotencyStore is an in-process IdempotencyStore for tests
// and single-node deployments. Production should use a Redis or DB
// backed implementation.
type MemoryIdempotencyStore struct {
	mu  sync.RWMutex
	ids map[string]struct{}
}

// NewMemoryIdempotencyStore creates a store.
func NewMemoryIdempotencyStore() *MemoryIdempotencyStore {
	return &MemoryIdempotencyStore{ids: make(map[string]struct{})}
}

func (s *MemoryIdempotencyStore) Exists(_ context.Context, id string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.ids[id]
	return ok, nil
}

func (s *MemoryIdempotencyStore) Record(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ids[id] = struct{}{}
	return nil
}

// -----------------------------------------------------------------------
// Dead-letter queue
// -----------------------------------------------------------------------

// DeadLetter is a message that exhausted its retry budget.
type DeadLetter struct {
	MessageID  string        `json:"message_id"`
	Message    Message       `json:"message"`
	LastError  string        `json:"last_error"`
	Attempts   int           `json:"attempts"`
	Channel    string        `json:"channel"`
	CreatedAt  time.Time     `json:"created_at"`
}

// DeadLetterQueue stores messages that could not be delivered.
type DeadLetterQueue interface {
	// Enqueue adds a message to the DLQ.
	Enqueue(ctx context.Context, dl DeadLetter) error
	// List returns up to limit entries, newest first.
	List(ctx context.Context, limit int) ([]DeadLetter, error)
	// Len returns the number of entries.
	Len(ctx context.Context) (int, error)
}

// MemoryDeadLetterQueue is an in-process DLQ for tests.
type MemoryDeadLetterQueue struct {
	mu      sync.Mutex
	entries []DeadLetter
}

// NewMemoryDeadLetterQueue creates a DLQ.
func NewMemoryDeadLetterQueue() *MemoryDeadLetterQueue {
	return &MemoryDeadLetterQueue{}
}

func (q *MemoryDeadLetterQueue) Enqueue(_ context.Context, dl DeadLetter) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.entries = append(q.entries, dl)
	return nil
}

func (q *MemoryDeadLetterQueue) List(_ context.Context, limit int) ([]DeadLetter, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := len(q.entries)
	if limit <= 0 || limit > n {
		limit = n
	}
	// newest first
	out := make([]DeadLetter, limit)
	for i := 0; i < limit; i++ {
		out[i] = q.entries[n-1-i]
	}
	return out, nil
}

func (q *MemoryDeadLetterQueue) Len(_ context.Context) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.entries), nil
}

// -----------------------------------------------------------------------
// Suppressor interface
// -----------------------------------------------------------------------

// SuppressionDecision is the outcome of a suppression evaluation.
type SuppressionDecision struct {
	Suppress bool
	Reason   string
}

// Suppressor evaluates whether a notification should be suppressed.
// Implementations typically check clustering, false-positive history,
// and activity baselines. The Dispatcher treats a nil Suppressor as
// "no suppression" for backward compatibility.
type Suppressor interface {
	// EvaluateMessage decides whether msg should be suppressed.
	EvaluateMessage(ctx context.Context, msg Message) (SuppressionDecision, error)
}

// -----------------------------------------------------------------------
// Dispatcher
// -----------------------------------------------------------------------

// DispatcherConfig holds dependencies for the Dispatcher.
type DispatcherConfig struct {
	Registry    *ChannelRegistry
	Idempotency IdempotencyStore
	DLQ         DeadLetterQueue
	Metrics     *MetricsCollector

	// Suppressor is an optional suppression engine. When non-nil the
	// Dispatcher calls EvaluateMessage before sending. If the decision
	// is to suppress, the message is not delivered and a
	// DeliveryStateSuppressed result is returned instead.
	Suppressor Suppressor

	// MaxRetries is the number of times to retry a failed delivery
	// before dead-lettering. Defaults to 3.
	MaxRetries int

	// RetryBaseDelay is the initial delay for exponential backoff.
	// Defaults to 1 second.
	RetryBaseDelay time.Duration

	// RetryMaxDelay caps the backoff. Defaults to 30 seconds.
	RetryMaxDelay time.Duration
}

// Dispatcher routes messages to the appropriate DeliveryChannel,
// enforces idempotency, retries transient failures, and dead-letters
// permanently failed messages.
type Dispatcher struct {
	registry    *ChannelRegistry
	idempotency IdempotencyStore
	dlq         DeadLetterQueue
	metrics     *MetricsCollector
	suppressor  Suppressor
	maxRetries  int
	baseDelay   time.Duration
	maxDelay    time.Duration
}

// NewDispatcher creates a Dispatcher with the given config.
func NewDispatcher(cfg DispatcherConfig) (*Dispatcher, error) {
	if cfg.Registry == nil {
		return nil, errors.New("notifications: registry is required")
	}
	if cfg.Idempotency == nil {
		cfg.Idempotency = NewMemoryIdempotencyStore()
	}
	if cfg.DLQ == nil {
		cfg.DLQ = NewMemoryDeadLetterQueue()
	}
	if cfg.Metrics == nil {
		cfg.Metrics = NewMetricsCollector()
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetryBaseDelay <= 0 {
		cfg.RetryBaseDelay = 1 * time.Second
	}
	if cfg.RetryMaxDelay <= 0 {
		cfg.RetryMaxDelay = 30 * time.Second
	}
	return &Dispatcher{
		registry:    cfg.Registry,
		idempotency: cfg.Idempotency,
		dlq:         cfg.DLQ,
		metrics:     cfg.Metrics,
		suppressor:  cfg.Suppressor,
		maxRetries:  cfg.MaxRetries,
		baseDelay:   cfg.RetryBaseDelay,
		maxDelay:    cfg.RetryMaxDelay,
	}, nil
}

// Dispatch sends a message through the appropriate channel with
// idempotency, retry, and DLQ semantics.
func (d *Dispatcher) Dispatch(ctx context.Context, msg Message) ([]DeliveryResult, error) {
	// Auto-generate ID if missing.
	if msg.ID == "" {
		msg.ID = generateID()
	}

	// Validate the message.
	if err := msg.Validate(); err != nil {
		return nil, err
	}

	// Idempotency check.
	dup, err := d.idempotency.Exists(ctx, msg.ID)
	if err != nil {
		return nil, fmt.Errorf("notifications: idempotency check: %w", err)
	}
	if dup {
		return nil, ErrDuplicateMessage
	}

	// Suppression check (optional — nil suppressor means no suppression).
	if d.suppressor != nil {
		decision, sErr := d.suppressor.EvaluateMessage(ctx, msg)
		if sErr != nil {
			return nil, fmt.Errorf("notifications: suppression check: %w", sErr)
		}
		if decision.Suppress {
			_ = d.idempotency.Record(ctx, msg.ID)
			results := make([]DeliveryResult, len(msg.To))
			for i, r := range msg.To {
				results[i] = DeliveryResult{
					MessageID:    msg.ID,
					Recipient:    r.Address,
					State:        DeliveryStateSuppressed,
					ErrorMessage: decision.Reason,
					Timestamp:    time.Now().UTC(),
				}
			}
			return results, nil
		}
	}

	// Find a channel that supports this message type.
	channels := d.registry.ForType(msg.Type)
	if len(channels) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoChannel, msg.Type)
	}

	// Use the first matching channel. Future: channel selection
	// strategy (failover, load-balanced, etc.).
	ch := channels[0]

	// Retry loop with exponential backoff.
	var results []DeliveryResult
	var lastErr error
	for attempt := 0; attempt <= d.maxRetries; attempt++ {
		if attempt > 0 {
			d.metrics.RecordRetried(ch.Name(), 1)
			delay := d.backoff(attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		start := time.Now()
		results, lastErr = ch.BatchSend(ctx, msg)
		elapsed := time.Since(start)
		d.metrics.RecordLatency(ch.Name(), elapsed)

		if lastErr == nil {
			// Count successes and failures from results.
			for _, r := range results {
				if r.State == DeliveryStateDelivered {
					d.metrics.RecordSent(ch.Name(), 1)
				} else if r.State == DeliveryStateFailed {
					d.metrics.RecordFailed(ch.Name(), 1)
				}
			}
			// Record idempotency key.
			_ = d.idempotency.Record(ctx, msg.ID)
			return results, nil
		}
	}

	// Exhausted retries — dead-letter.
	d.metrics.RecordFailed(ch.Name(), 1)
	d.metrics.RecordDLQ(ch.Name(), 1)
	errMsg := ""
	if lastErr != nil {
		errMsg = lastErr.Error()
	}
	_ = d.dlq.Enqueue(ctx, DeadLetter{
		MessageID: msg.ID,
		Message:   msg,
		LastError: errMsg,
		Attempts:  d.maxRetries + 1,
		Channel:   ch.Name(),
		CreatedAt: time.Now().UTC(),
	})

	return nil, fmt.Errorf("%w: %s after %d attempts: %v",
		ErrMaxRetriesExceeded, ch.Name(), d.maxRetries+1, lastErr)
}

// backoff computes exponential backoff delay for the given 1-indexed
// attempt, capped at maxDelay.
func (d *Dispatcher) backoff(attempt int) time.Duration {
	delay := d.baseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay > d.maxDelay {
			return d.maxDelay
		}
	}
	return delay
}

// Metrics returns the dispatcher's metrics collector so callers can
// scrape counters.
func (d *Dispatcher) Metrics() *MetricsCollector {
	return d.metrics
}

// DLQ returns the dispatcher's dead-letter queue.
func (d *Dispatcher) DLQ() DeadLetterQueue {
	return d.dlq
}

// generateID creates a 32-hex-char random ID.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
