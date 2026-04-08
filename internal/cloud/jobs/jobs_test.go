package jobs

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// --- test fixtures -------------------------------------------------------

type testPayload struct {
	Tenant string
	Note   string
}

func (p testPayload) TenantID() string { return p.Tenant }

// recordingWorker is a Worker that remembers every job it was given
// and can be programmed to fail N times before succeeding.
type recordingWorker struct {
	kind     Kind
	failN    int32 // decrements; fails while > 0
	ran      int32
	lastJob  *Job
	alwaysErr error
}

func (r *recordingWorker) Kind() Kind { return r.kind }
func (r *recordingWorker) Work(_ context.Context, job *Job) error {
	atomic.AddInt32(&r.ran, 1)
	r.lastJob = job
	if r.alwaysErr != nil {
		return r.alwaysErr
	}
	if atomic.LoadInt32(&r.failN) > 0 {
		atomic.AddInt32(&r.failN, -1)
		return errors.New("transient boom")
	}
	return nil
}

func newTestEnqueuer(t *testing.T, knownTenants ...string) (*MemoryEnqueuer, *FakeClock) {
	t.Helper()
	clk := NewFakeClock(time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC))
	m := NewMemoryEnqueuer(MemoryConfig{
		Clock:          clk,
		TenantVerifier: NewMemoryTenantSet(knownTenants...),
	})
	return m, clk
}

// The test kinds we route through the memory backend. We reuse the
// real KindTenantWelcomeEmail for most tests and the other kinds when
// testing per-kind routing.
const tenantA = "tenant-a"
const tenantB = "tenant-b"

// --- 1. enqueue + synchronous run ---------------------------------------

func TestEnqueueAndSyncRun(t *testing.T) {
	m, _ := newTestEnqueuer(t, tenantA)
	w := &recordingWorker{kind: KindTenantWelcomeEmail}
	if err := m.Register(w); err != nil {
		t.Fatalf("register: %v", err)
	}

	job, err := m.Enqueue(context.Background(), KindTenantWelcomeEmail, testPayload{Tenant: tenantA}, EnqueueOptions{})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if job.State != StatePending {
		t.Fatalf("state = %q, want pending", job.State)
	}

	if err := m.RunOnce(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}
	if atomic.LoadInt32(&w.ran) != 1 {
		t.Fatalf("worker ran %d times, want 1", w.ran)
	}
	if job.State != StateSucceeded {
		t.Fatalf("state = %q, want succeeded", job.State)
	}
	if m.StatsPtr().Succeeded.Load() != 1 {
		t.Fatalf("succeeded counter = %d, want 1", m.StatsPtr().Succeeded.Load())
	}
}

// --- 2. per-kind routing -------------------------------------------------

func TestPerKindRouting(t *testing.T) {
	m, _ := newTestEnqueuer(t, tenantA, SystemTenant)

	workers := map[Kind]*recordingWorker{}
	for _, k := range AllKinds {
		w := &recordingWorker{kind: k}
		workers[k] = w
		if err := m.Register(w); err != nil {
			t.Fatalf("register %s: %v", k, err)
		}
	}

	enq := func(k Kind, p TenantScoped) {
		if _, err := m.Enqueue(context.Background(), k, p, EnqueueOptions{}); err != nil {
			t.Fatalf("enqueue %s: %v", k, err)
		}
	}
	enq(KindTenantWelcomeEmail, TenantWelcomeEmailPayload{Tenant: tenantA})
	enq(KindTenantBootstrapStripe, TenantBootstrapStripePayload{Tenant: tenantA})
	enq(KindTenantBootstrapZitadel, TenantBootstrapZitadelPayload{Tenant: tenantA})
	enq(KindBulkPushConfig, BulkPushConfigPayload{Tenant: tenantA})
	enq(KindCloudArchiveUploadTrig, CloudArchiveUploadTriggerPayload{Tenant: tenantA})
	enq(KindBillingMonthlyRollup, BillingMonthlyRollupPayload{Tenant: tenantA})
	enq(KindAuditPartitionCreateNext, AuditPartitionCreateNextPayload{Tenant: SystemTenant})
	enq(KindAuditDropExpired, AuditDropExpiredPayload{Tenant: SystemTenant})

	if err := m.RunUntilEmpty(context.Background()); err != nil {
		t.Fatalf("run until empty: %v", err)
	}
	for k, w := range workers {
		if atomic.LoadInt32(&w.ran) != 1 {
			t.Fatalf("worker %s ran %d times, want 1", k, w.ran)
		}
	}
}

// --- 3. retry with exponential backoff + fake clock ---------------------

func TestRetryWithExponentialBackoff(t *testing.T) {
	m, clk := newTestEnqueuer(t, tenantA)
	w := &recordingWorker{kind: KindTenantWelcomeEmail, failN: 2}
	if err := m.Register(w); err != nil {
		t.Fatal(err)
	}

	job, err := m.Enqueue(context.Background(), KindTenantWelcomeEmail, testPayload{Tenant: tenantA}, EnqueueOptions{})
	if err != nil {
		t.Fatal(err)
	}

	policy := PolicyFor(KindTenantWelcomeEmail)

	// Attempt 1 → fails, scheduled BaseDelay in the future.
	if err := m.RunOnce(context.Background()); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	if job.State != StatePending || job.Attempts != 1 {
		t.Fatalf("after attempt 1: state=%s attempts=%d", job.State, job.Attempts)
	}
	// Without advancing the clock the job is not yet ready.
	if err := m.RunOnce(context.Background()); !errors.Is(err, ErrNoJobs) {
		t.Fatalf("expected ErrNoJobs, got %v", err)
	}

	// Advance past attempt 1 backoff.
	clk.Advance(policy.BackoffFor(1) + time.Millisecond)
	if err := m.RunOnce(context.Background()); err != nil {
		t.Fatalf("run 2: %v", err)
	}
	if job.Attempts != 2 || job.State != StatePending {
		t.Fatalf("after attempt 2: state=%s attempts=%d", job.State, job.Attempts)
	}

	// Advance past attempt 2 backoff.
	clk.Advance(policy.BackoffFor(2) + time.Millisecond)
	if err := m.RunOnce(context.Background()); err != nil {
		t.Fatalf("run 3: %v", err)
	}
	if job.State != StateSucceeded {
		t.Fatalf("expected succeeded, got %s (err=%s)", job.State, job.LastError)
	}
	if m.StatsPtr().Retried.Load() != 2 {
		t.Fatalf("retried=%d want 2", m.StatsPtr().Retried.Load())
	}
	if m.StatsPtr().Failed.Load() != 2 {
		t.Fatalf("failed=%d want 2", m.StatsPtr().Failed.Load())
	}
}

// --- 4. DLQ after max attempts ------------------------------------------

func TestDLQAfterMaxAttempts(t *testing.T) {
	m, clk := newTestEnqueuer(t, tenantA)
	w := &recordingWorker{kind: KindTenantWelcomeEmail, alwaysErr: errors.New("perma boom")}
	if err := m.Register(w); err != nil {
		t.Fatal(err)
	}

	job, err := m.Enqueue(context.Background(), KindTenantWelcomeEmail, testPayload{Tenant: tenantA}, EnqueueOptions{MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		err := m.RunOnce(context.Background())
		if errors.Is(err, ErrNoJobs) {
			// need to advance clock to the next scheduled run
			clk.Advance(10 * time.Minute)
			continue
		}
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if job.State == StateDLQ {
			break
		}
	}
	if job.State != StateDLQ {
		t.Fatalf("expected DLQ, got %s", job.State)
	}
	if job.Attempts != 3 {
		t.Fatalf("attempts=%d want 3", job.Attempts)
	}
	if m.StatsPtr().DLQ.Load() != 1 {
		t.Fatalf("dlq counter=%d want 1", m.StatsPtr().DLQ.Load())
	}
}

// --- 5. idempotency -----------------------------------------------------

func TestIdempotency(t *testing.T) {
	m, _ := newTestEnqueuer(t, tenantA)
	if err := m.Register(&recordingWorker{kind: KindTenantWelcomeEmail}); err != nil {
		t.Fatal(err)
	}

	opts := EnqueueOptions{IdempotencyKey: "webhook-evt-42"}
	first, err := m.Enqueue(context.Background(), KindTenantWelcomeEmail, testPayload{Tenant: tenantA}, opts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Enqueue(context.Background(), KindTenantWelcomeEmail, testPayload{Tenant: tenantA, Note: "different"}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same job id, got %q vs %q", first.ID, second.ID)
	}
	if m.StatsPtr().Dropped.Load() != 1 {
		t.Fatalf("dropped=%d want 1", m.StatsPtr().Dropped.Load())
	}
	if len(m.Jobs()) != 1 {
		t.Fatalf("len(jobs)=%d want 1", len(m.Jobs()))
	}
}

// --- 6. cross-tenant isolation (chaos) ----------------------------------

func TestCrossTenantIsolation(t *testing.T) {
	// Only tenant A is registered. A job arrives carrying tenant B.
	m, _ := newTestEnqueuer(t, tenantA)
	w := &recordingWorker{kind: KindTenantWelcomeEmail}
	if err := m.Register(w); err != nil {
		t.Fatal(err)
	}

	job, err := m.Enqueue(context.Background(), KindTenantWelcomeEmail, testPayload{Tenant: tenantB}, EnqueueOptions{})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	err = m.RunOnce(context.Background())
	if !errors.Is(err, ErrTenantMismatch) {
		t.Fatalf("expected ErrTenantMismatch, got %v", err)
	}
	if atomic.LoadInt32(&w.ran) != 0 {
		t.Fatalf("worker MUST NOT have run, ran=%d", w.ran)
	}
	if job.State != StateDLQ {
		t.Fatalf("cross-tenant job state=%s want dlq", job.State)
	}
	if m.StatsPtr().DLQ.Load() != 1 {
		t.Fatalf("dlq=%d want 1", m.StatsPtr().DLQ.Load())
	}
}

// --- 7. graceful shutdown drains in-flight ------------------------------

func TestShutdownDrainsInFlight(t *testing.T) {
	m, _ := newTestEnqueuer(t, tenantA)

	// Worker blocks until released.
	release := make(chan struct{})
	entered := make(chan struct{})
	w := &blockingWorker{kind: KindTenantWelcomeEmail, entered: entered, release: release}
	if err := m.Register(w); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Enqueue(context.Background(), KindTenantWelcomeEmail, testPayload{Tenant: tenantA}, EnqueueOptions{}); err != nil {
		t.Fatal(err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- m.RunOnce(context.Background()) }()
	<-entered

	// Shutdown should block until we release the worker.
	shutdownDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		shutdownDone <- m.Shutdown(ctx)
	}()

	select {
	case <-shutdownDone:
		t.Fatal("shutdown returned before worker released")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	if err := <-runDone; err != nil {
		t.Fatalf("run: %v", err)
	}
	if err := <-shutdownDone; err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	// After shutdown, new enqueues fail.
	if _, err := m.Enqueue(context.Background(), KindTenantWelcomeEmail, testPayload{Tenant: tenantA}, EnqueueOptions{}); !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("expected ErrShuttingDown, got %v", err)
	}
}

type blockingWorker struct {
	kind    Kind
	entered chan struct{}
	release chan struct{}
}

func (b *blockingWorker) Kind() Kind { return b.kind }
func (b *blockingWorker) Work(_ context.Context, _ *Job) error {
	b.entered <- struct{}{}
	<-b.release
	return nil
}

// --- 8. unknown kind returns typed error --------------------------------

func TestUnknownKindEnqueueAndRun(t *testing.T) {
	m, _ := newTestEnqueuer(t, tenantA)
	_, err := m.Enqueue(context.Background(), Kind("nope.not.a.kind"), testPayload{Tenant: tenantA}, EnqueueOptions{})
	if !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("enqueue: expected ErrUnknownKind, got %v", err)
	}

	// And missing tenant id.
	_, err = m.Enqueue(context.Background(), KindTenantWelcomeEmail, testPayload{}, EnqueueOptions{})
	if !errors.Is(err, ErrMissingTenant) {
		t.Fatalf("expected ErrMissingTenant, got %v", err)
	}

	// Running with no worker for a registered kind.
	if _, err := m.Enqueue(context.Background(), KindBulkPushConfig, BulkPushConfigPayload{Tenant: tenantA}, EnqueueOptions{}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	err = m.RunOnce(context.Background())
	if !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("run: expected ErrUnknownKind, got %v", err)
	}
}

// --- 9. stubs for all 8 seeded kinds register cleanly -------------------

func TestDefaultWorkersRegister(t *testing.T) {
	m, _ := newTestEnqueuer(t, tenantA, SystemTenant)
	if err := m.RegisterAll(DefaultWorkers(nil)...); err != nil {
		t.Fatalf("register all: %v", err)
	}
	// Duplicate registration must fail.
	if err := m.Register(&TenantWelcomeEmailWorker{}); err == nil {
		t.Fatal("expected duplicate worker registration to fail")
	}

	// Enqueue + run one of each seeded kind and verify all succeed.
	payloads := map[Kind]TenantScoped{
		KindTenantWelcomeEmail:       TenantWelcomeEmailPayload{Tenant: tenantA, AdminUser: "u1"},
		KindTenantBootstrapStripe:    TenantBootstrapStripePayload{Tenant: tenantA, BillingMode: "direct"},
		KindTenantBootstrapZitadel:   TenantBootstrapZitadelPayload{Tenant: tenantA, OrgName: "Acme"},
		KindBulkPushConfig:           BulkPushConfigPayload{Tenant: tenantA, CustomerIDs: []string{"c1"}},
		KindCloudArchiveUploadTrig:   CloudArchiveUploadTriggerPayload{Tenant: tenantA, SegmentID: "seg1"},
		KindBillingMonthlyRollup:     BillingMonthlyRollupPayload{Tenant: tenantA, Period: "2026-04"},
		KindAuditPartitionCreateNext: AuditPartitionCreateNextPayload{Tenant: SystemTenant, TargetYYM: "2026-05"},
		KindAuditDropExpired:         AuditDropExpiredPayload{Tenant: SystemTenant, OlderThanYM: "2025-04"},
	}
	for _, k := range AllKinds {
		if _, err := m.Enqueue(context.Background(), k, payloads[k], EnqueueOptions{}); err != nil {
			t.Fatalf("enqueue %s: %v", k, err)
		}
	}
	if err := m.RunUntilEmpty(context.Background()); err != nil {
		t.Fatalf("run until empty: %v", err)
	}
	if got := len(m.JobsInState(StateSucceeded)); got != len(AllKinds) {
		t.Fatalf("succeeded jobs = %d, want %d", got, len(AllKinds))
	}
}

// --- 10. backoff curve unit -----------------------------------------------

func TestRetryPolicyBackoffFor(t *testing.T) {
	p := RetryPolicy{
		MaxAttempts: 5,
		BaseDelay:   1 * time.Second,
		MaxDelay:    10 * time.Second,
		Multiplier:  2.0,
	}
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{5, 10 * time.Second}, // capped
		{6, 10 * time.Second}, // still capped
	}
	for _, c := range cases {
		got := p.BackoffFor(c.attempt)
		if got != c.want {
			t.Errorf("attempt %d: got %s want %s", c.attempt, got, c.want)
		}
	}
}

// --- 11. river stub rejects with typed error (default build) ------------

func TestRiverStubRejects(t *testing.T) {
	r := NewRiverEnqueuer()
	_, err := r.Enqueue(context.Background(), KindTenantWelcomeEmail, testPayload{Tenant: tenantA}, EnqueueOptions{})
	if err == nil {
		t.Fatal("expected river stub to return an error in default build")
	}
}

// --- 12. snapshot sanity -------------------------------------------------

func TestStatsSnapshot(t *testing.T) {
	m, _ := newTestEnqueuer(t, tenantA)
	if err := m.Register(&recordingWorker{kind: KindTenantWelcomeEmail}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := m.Enqueue(context.Background(), KindTenantWelcomeEmail, testPayload{Tenant: tenantA, Note: fmt.Sprintf("%d", i)}, EnqueueOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.RunUntilEmpty(context.Background()); err != nil {
		t.Fatal(err)
	}
	snap := m.StatsPtr().Snapshot()
	if snap["jobs_enqueued"] != 3 || snap["jobs_succeeded"] != 3 {
		t.Fatalf("snapshot=%v", snap)
	}
}
