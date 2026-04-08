package jobs

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Clock is an injectable time source so tests can use a fake clock
// when verifying backoff timing.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// FakeClock is a test clock whose time only advances when Advance is
// called. Safe for concurrent use.
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewFakeClock returns a FakeClock anchored at t.
func NewFakeClock(t time.Time) *FakeClock { return &FakeClock{now: t} }

// Now implements Clock.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by d.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// MemoryEnqueuer is the in-process JobEnqueuer + JobRunner used by
// tests and local dev. It stores jobs in a slice, supports idempotency,
// synchronous execution, retries with exponential backoff, and a DLQ.
type MemoryEnqueuer struct {
	mu sync.Mutex

	clock    Clock
	verifier TenantVerifier
	stats    Stats

	workers map[Kind]Worker
	jobs    []*Job
	byKey   map[string]*Job // idempotency key → first job

	inFlight sync.WaitGroup
	closed   bool
}

// MemoryConfig configures a MemoryEnqueuer.
type MemoryConfig struct {
	Clock          Clock
	TenantVerifier TenantVerifier
}

// NewMemoryEnqueuer returns a ready-to-use backend. A nil verifier is
// treated as "every tenant is known" — use only in tests that do not
// care about Seam #4.
func NewMemoryEnqueuer(cfg MemoryConfig) *MemoryEnqueuer {
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}
	return &MemoryEnqueuer{
		clock:    cfg.Clock,
		verifier: cfg.TenantVerifier,
		workers:  make(map[Kind]Worker),
		byKey:    make(map[string]*Job),
	}
}

// Stats returns the metrics surface. The same pointer is returned on
// every call so a scraper can hold onto it.
func (m *MemoryEnqueuer) StatsPtr() *Stats { return &m.stats }

// Enqueue implements JobEnqueuer.
func (m *MemoryEnqueuer) Enqueue(ctx context.Context, kind Kind, payload TenantScoped, opts EnqueueOptions) (*Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateEnqueue(kind, payload); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, ErrShuttingDown
	}

	// Idempotency — duplicate key returns the first job and
	// increments Dropped so callers can distinguish "I created a
	// new one" from "I found an existing one" if they want to.
	if opts.IdempotencyKey != "" {
		if existing, ok := m.byKey[opts.IdempotencyKey]; ok {
			m.stats.Dropped.Add(1)
			return existing, nil
		}
	}

	policy := PolicyFor(kind)
	max := opts.MaxAttempts
	if max <= 0 {
		max = policy.MaxAttempts
	}

	runAt := opts.RunAt
	if runAt.IsZero() {
		runAt = m.clock.Now()
	}

	job := &Job{
		ID:             newID(),
		Kind:           kind,
		Payload:        payload,
		IdempotencyKey: opts.IdempotencyKey,
		TenantID:       payload.TenantID(),
		State:          StatePending,
		Attempts:       0,
		MaxAttempts:    max,
		EnqueuedAt:     m.clock.Now(),
		NextRunAt:      runAt,
	}
	m.jobs = append(m.jobs, job)
	if opts.IdempotencyKey != "" {
		m.byKey[opts.IdempotencyKey] = job
	}
	m.stats.Enqueued.Add(1)
	return job, nil
}

// Register adds a worker to the routing table. Workers returning a
// kind that already has one registered return an error so wire-up
// mistakes fail loudly.
func (m *MemoryEnqueuer) Register(w Worker) error {
	if w == nil {
		return errors.New("jobs: nil worker")
	}
	if !KnownKind(w.Kind()) {
		return fmt.Errorf("%w: %q", ErrUnknownKind, w.Kind())
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.workers[w.Kind()]; exists {
		return fmt.Errorf("jobs: worker already registered for %q", w.Kind())
	}
	m.workers[w.Kind()] = w
	return nil
}

// RegisterAll is a convenience for wiring up the full set at once.
func (m *MemoryEnqueuer) RegisterAll(ws ...Worker) error {
	for _, w := range ws {
		if err := m.Register(w); err != nil {
			return err
		}
	}
	return nil
}

// pickReady returns the first pending job whose NextRunAt is <= now.
// Must be called with m.mu held.
func (m *MemoryEnqueuer) pickReady(now time.Time) *Job {
	for _, j := range m.jobs {
		if j.State != StatePending {
			continue
		}
		if !j.NextRunAt.After(now) {
			return j
		}
	}
	return nil
}

// RunOnce executes a single ready job. Returns ErrNoJobs if none are
// ready.
func (m *MemoryEnqueuer) RunOnce(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrShuttingDown
	}
	now := m.clock.Now()
	job := m.pickReady(now)
	if job == nil {
		m.mu.Unlock()
		return ErrNoJobs
	}
	worker, ok := m.workers[job.Kind]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("%w: no worker for %q", ErrUnknownKind, job.Kind)
	}

	// Seam #4: verify tenant id against the known-tenant set.
	// Failure is terminal — we move the job directly to DLQ
	// because retries cannot fix cross-tenant poisoning.
	if m.verifier != nil && !m.verifier.KnownTenant(ctx, job.TenantID) {
		job.State = StateDLQ
		job.LastError = ErrTenantMismatch.Error()
		job.FailedAt = now
		m.stats.DLQ.Add(1)
		m.mu.Unlock()
		return fmt.Errorf("%w: job=%s tenant=%s", ErrTenantMismatch, job.ID, job.TenantID)
	}

	job.State = StateRunning
	job.Attempts++
	m.inFlight.Add(1)
	m.mu.Unlock()

	defer m.inFlight.Done()

	err := worker.Work(ctx, job)

	m.mu.Lock()
	defer m.mu.Unlock()
	if err == nil {
		job.State = StateSucceeded
		job.SucceededAt = m.clock.Now()
		m.stats.Succeeded.Add(1)
		return nil
	}

	m.stats.Failed.Add(1)
	job.LastError = err.Error()

	if job.Attempts >= job.MaxAttempts {
		job.State = StateDLQ
		job.FailedAt = m.clock.Now()
		m.stats.DLQ.Add(1)
		return nil
	}

	// Schedule the next attempt.
	policy := PolicyFor(job.Kind)
	delay := policy.BackoffFor(job.Attempts)
	job.State = StatePending
	job.NextRunAt = m.clock.Now().Add(delay)
	m.stats.Retried.Add(1)
	return nil
}

// RunUntilEmpty keeps calling RunOnce until no ready jobs remain. It
// respects the fake clock — jobs scheduled for the future remain
// pending until the clock advances.
func (m *MemoryEnqueuer) RunUntilEmpty(ctx context.Context) error {
	for {
		err := m.RunOnce(ctx)
		if errors.Is(err, ErrNoJobs) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// Shutdown marks the runner as closed, waits for any in-flight job,
// and rejects subsequent Enqueue calls.
func (m *MemoryEnqueuer) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()

	done := make(chan struct{})
	go func() {
		m.inFlight.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Jobs returns a sorted snapshot of every job for test assertions.
// The copy is shallow — callers must not mutate Payload.
func (m *MemoryEnqueuer) Jobs() []*Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Job, len(m.jobs))
	copy(out, m.jobs)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].EnqueuedAt.Before(out[j].EnqueuedAt)
	})
	return out
}

// JobsInState is a filter helper for tests.
func (m *MemoryEnqueuer) JobsInState(s State) []*Job {
	all := m.Jobs()
	out := make([]*Job, 0, len(all))
	for _, j := range all {
		if j.State == s {
			out = append(out, j)
		}
	}
	return out
}
