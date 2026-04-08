// Package jobs is the Kaivue cloud control-plane background job system.
//
// It defines the JobEnqueuer / JobRunner interfaces consumed by other
// cloud services (tenant provisioning, billing, audit maintenance,
// etc.) and ships two backends:
//
//   - MemoryEnqueuer — an in-process test double that stores jobs in a
//     slice and can run them synchronously or on demand. Used by unit
//     tests and local dev.
//   - RiverEnqueuer — a stub that wraps github.com/riverqueue/river,
//     gated behind the `river` build tag. In the default build it is a
//     no-op placeholder so the package compiles without the real
//     dependency. See river_stub.go / river_real.go.
//
// The contract every worker MUST obey is documented in README.md:
// verify the tenant id on the payload against the known-tenant list
// BEFORE touching any state. This is Seam #4 (multi-tenant isolation)
// and there is a chaos test that proves it.
package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Kind identifies a job type. Kinds are closed: the registry rejects
// enqueues for unknown kinds so typos fail loudly at the call site.
type Kind string

const (
	KindTenantWelcomeEmail       Kind = "tenant.welcome_email"
	KindTenantBootstrapStripe    Kind = "tenant.bootstrap_stripe"
	KindTenantBootstrapZitadel   Kind = "tenant.bootstrap_zitadel"
	KindBulkPushConfig           Kind = "bulk.push_config"
	KindCloudArchiveUploadTrig   Kind = "cloud_archive.upload_trigger"
	KindBillingMonthlyRollup     Kind = "billing.monthly_rollup"
	KindAuditPartitionCreateNext Kind = "audit.partition_create_next_month"
	KindAuditDropExpired         Kind = "audit.drop_expired_partitions"
)

// AllKinds is the canonical list. Order is stable so tests can iterate.
var AllKinds = []Kind{
	KindTenantWelcomeEmail,
	KindTenantBootstrapStripe,
	KindTenantBootstrapZitadel,
	KindBulkPushConfig,
	KindCloudArchiveUploadTrig,
	KindBillingMonthlyRollup,
	KindAuditPartitionCreateNext,
	KindAuditDropExpired,
}

// State is the lifecycle of a single Job.
type State string

const (
	StatePending   State = "pending"
	StateRunning   State = "running"
	StateSucceeded State = "succeeded"
	StateFailed    State = "failed" // attempted again
	StateDLQ       State = "dlq"    // exceeded max attempts
)

// TenantScoped is the minimal interface every job payload MUST
// implement. The runner verifies this before routing to a worker.
//
// Implementations should return the tenant id derived from the
// authenticated caller at enqueue time. Never read it from user input
// on the consuming side.
type TenantScoped interface {
	TenantID() string
}

// Job is a single unit of work. It is returned by Enqueue and passed
// to workers. Fields are immutable once Scheduled.
type Job struct {
	ID             string
	Kind           Kind
	Payload        TenantScoped
	IdempotencyKey string
	TenantID       string

	State        State
	Attempts     int
	MaxAttempts  int
	LastError    string
	EnqueuedAt   time.Time
	NextRunAt    time.Time
	SucceededAt  time.Time
	FailedAt     time.Time
}

// EnqueueOptions are per-call knobs.
type EnqueueOptions struct {
	// IdempotencyKey — if non-empty, a duplicate enqueue with the
	// same key returns the first Job without scheduling a second.
	// Critical for webhook replays.
	IdempotencyKey string
	// MaxAttempts overrides the per-kind retry policy.
	MaxAttempts int
	// RunAt schedules the job for a future time. Zero = now.
	RunAt time.Time
}

// JobEnqueuer is the producer-side interface.
type JobEnqueuer interface {
	Enqueue(ctx context.Context, kind Kind, payload TenantScoped, opts EnqueueOptions) (*Job, error)
}

// Worker runs one kind of job.
type Worker interface {
	Kind() Kind
	Work(ctx context.Context, job *Job) error
}

// JobRunner is the consumer-side interface — it hosts workers and
// drives them to completion.
type JobRunner interface {
	Register(w Worker) error
	// RunOnce picks up one ready job and runs it to completion
	// (including retries scheduled for future attempts; those are
	// left pending). Returns io.EOF-equivalent ErrNoJobs when empty.
	RunOnce(ctx context.Context) error
	// Shutdown drains in-flight work with the supplied deadline.
	Shutdown(ctx context.Context) error
}

// -----------------------------------------------------------------------
// Errors
// -----------------------------------------------------------------------

var (
	// ErrUnknownKind is returned when enqueuing a kind that has no
	// registered retry policy OR running a job whose kind has no
	// worker.
	ErrUnknownKind = errors.New("jobs: unknown kind")
	// ErrTenantMismatch is returned when a worker is handed a job
	// whose tenant id is not in the known-tenant set. Seam #4.
	ErrTenantMismatch = errors.New("jobs: tenant id not in known tenant set")
	// ErrMissingTenant is returned when a payload's TenantID() is
	// empty at enqueue time.
	ErrMissingTenant = errors.New("jobs: payload missing tenant id")
	// ErrNoJobs is returned by RunOnce when the queue is empty.
	ErrNoJobs = errors.New("jobs: no ready jobs")
	// ErrShuttingDown is returned by Enqueue on a closed runner.
	ErrShuttingDown = errors.New("jobs: runner shutting down")
)

// -----------------------------------------------------------------------
// Retry policy
// -----------------------------------------------------------------------

// RetryPolicy is the exponential backoff config for a kind.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	// Multiplier is the backoff growth factor. >= 1.0.
	Multiplier float64
}

// DefaultRetry is the fallback policy applied to kinds that do not
// override it.
var DefaultRetry = RetryPolicy{
	MaxAttempts: 5,
	BaseDelay:   1 * time.Second,
	MaxDelay:    5 * time.Minute,
	Multiplier:  2.0,
}

// BackoffFor computes the delay before the given 1-indexed attempt.
// Attempt 1 = BaseDelay, attempt 2 = BaseDelay*Multiplier, capped at
// MaxDelay.
func (p RetryPolicy) BackoffFor(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	d := float64(p.BaseDelay)
	for i := 1; i < attempt; i++ {
		d *= p.Multiplier
		if d >= float64(p.MaxDelay) {
			return p.MaxDelay
		}
	}
	return time.Duration(d)
}

// kindPolicies is the per-kind retry override table. Missing kinds
// fall back to DefaultRetry.
var kindPolicies = map[Kind]RetryPolicy{
	// Welcome email — short backoff, it's user-facing.
	KindTenantWelcomeEmail: {MaxAttempts: 5, BaseDelay: 2 * time.Second, MaxDelay: 1 * time.Minute, Multiplier: 2.0},
	// Stripe/Zitadel bootstrap — slower, retry for up to ~30 minutes.
	KindTenantBootstrapStripe:  {MaxAttempts: 6, BaseDelay: 5 * time.Second, MaxDelay: 10 * time.Minute, Multiplier: 3.0},
	KindTenantBootstrapZitadel: {MaxAttempts: 6, BaseDelay: 5 * time.Second, MaxDelay: 10 * time.Minute, Multiplier: 3.0},
	// Rollup + retention — overnight batch, short retries are fine.
	KindBillingMonthlyRollup:     {MaxAttempts: 3, BaseDelay: 10 * time.Second, MaxDelay: 5 * time.Minute, Multiplier: 2.0},
	KindAuditPartitionCreateNext: {MaxAttempts: 3, BaseDelay: 10 * time.Second, MaxDelay: 5 * time.Minute, Multiplier: 2.0},
	KindAuditDropExpired:         {MaxAttempts: 3, BaseDelay: 10 * time.Second, MaxDelay: 5 * time.Minute, Multiplier: 2.0},
}

// PolicyFor returns the RetryPolicy for a kind, falling back to
// DefaultRetry.
func PolicyFor(k Kind) RetryPolicy {
	if p, ok := kindPolicies[k]; ok {
		return p
	}
	return DefaultRetry
}

// KnownKind reports whether a kind is in AllKinds.
func KnownKind(k Kind) bool {
	for _, x := range AllKinds {
		if x == k {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------
// Stats / metrics
// -----------------------------------------------------------------------

// Stats is the in-process metrics surface. Fields are atomic counters
// so they can be read by a scraper without locking the runner.
//
// These mirror the Prometheus counters we will expose via
// internal/shared/logging fields: jobs_enqueued, jobs_succeeded,
// jobs_failed (attempts), jobs_retried, jobs_dlq.
type Stats struct {
	Enqueued  atomic.Int64
	Succeeded atomic.Int64
	Failed    atomic.Int64
	Retried   atomic.Int64
	DLQ       atomic.Int64
	Dropped   atomic.Int64 // idempotency-duplicate drops
}

// Snapshot returns a point-in-time copy suitable for logging.
func (s *Stats) Snapshot() map[string]int64 {
	return map[string]int64{
		"jobs_enqueued":  s.Enqueued.Load(),
		"jobs_succeeded": s.Succeeded.Load(),
		"jobs_failed":    s.Failed.Load(),
		"jobs_retried":   s.Retried.Load(),
		"jobs_dlq":       s.DLQ.Load(),
		"jobs_dropped":   s.Dropped.Load(),
	}
}

// -----------------------------------------------------------------------
// ID generation
// -----------------------------------------------------------------------

// newID returns a 24-char hex id, matching the audit package style.
func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// -----------------------------------------------------------------------
// Tenant verifier
// -----------------------------------------------------------------------

// TenantVerifier is the hook the runner calls before dispatching a job
// to its worker. It MUST return true only if the tenant id is known
// and active.
//
// In production this is backed by the tenant repo. In tests the
// MemoryTenantSet is used.
type TenantVerifier interface {
	KnownTenant(ctx context.Context, tenantID string) bool
}

// MemoryTenantSet is a goroutine-safe TenantVerifier backed by a map.
type MemoryTenantSet struct {
	mu      sync.RWMutex
	tenants map[string]struct{}
}

// NewMemoryTenantSet returns a set pre-populated with the given ids.
func NewMemoryTenantSet(ids ...string) *MemoryTenantSet {
	s := &MemoryTenantSet{tenants: make(map[string]struct{})}
	for _, id := range ids {
		s.tenants[id] = struct{}{}
	}
	return s
}

// Add inserts a tenant id.
func (s *MemoryTenantSet) Add(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tenants[id] = struct{}{}
}

// Remove deletes a tenant id.
func (s *MemoryTenantSet) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tenants, id)
}

// KnownTenant implements TenantVerifier.
func (s *MemoryTenantSet) KnownTenant(_ context.Context, id string) bool {
	if id == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.tenants[id]
	return ok
}

// -----------------------------------------------------------------------
// Validate
// -----------------------------------------------------------------------

// validateEnqueue is shared by every backend so the error surface is
// identical regardless of which implementation the caller picked.
func validateEnqueue(kind Kind, payload TenantScoped) error {
	if !KnownKind(kind) {
		return fmt.Errorf("%w: %q", ErrUnknownKind, kind)
	}
	if payload == nil {
		return fmt.Errorf("%w: nil payload", ErrMissingTenant)
	}
	if payload.TenantID() == "" {
		return fmt.Errorf("%w: %s", ErrMissingTenant, kind)
	}
	return nil
}
