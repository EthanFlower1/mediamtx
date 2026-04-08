package tenants

import (
	"context"
	"sync"
	"time"
)

// JobEnqueuer is the narrow seam (KAI-234) the provisioning service uses to
// enqueue the welcome-email job. KAI-234 is landing the River-backed
// implementation in parallel; we take the interface here and feed a
// MemoryEnqueuer in tests.
//
// The enqueuer is deliberately one-method: the provisioning service does not
// care about job scheduling, retry policy, or queue selection — those are
// River's concern. It only cares that "something will later mail this
// welcome message".
type JobEnqueuer interface {
	EnqueueWelcomeEmail(ctx context.Context, job WelcomeEmailJob) error
}

// WelcomeEmailJob is the payload of a freshly queued welcome email. The
// TenantID + TenantType pair is denormalized for River's indexer; the
// InviteToken is the token the Zitadel adapter returned.
type WelcomeEmailJob struct {
	TenantID    string
	TenantType  string
	Email       string
	InviteToken string
	DisplayName string
	EnqueuedAt  time.Time
}

// MemoryEnqueuer is a test implementation that records enqueued jobs without
// actually sending anything.
type MemoryEnqueuer struct {
	mu   sync.Mutex
	jobs []WelcomeEmailJob
	// EnqueueErr, when non-nil, is returned once and then cleared.
	EnqueueErr error
}

// NewMemoryEnqueuer returns an empty in-memory enqueuer.
func NewMemoryEnqueuer() *MemoryEnqueuer {
	return &MemoryEnqueuer{}
}

// EnqueueWelcomeEmail implements JobEnqueuer.
func (m *MemoryEnqueuer) EnqueueWelcomeEmail(_ context.Context, job WelcomeEmailJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.EnqueueErr; err != nil {
		m.EnqueueErr = nil
		return err
	}
	if job.EnqueuedAt.IsZero() {
		job.EnqueuedAt = time.Now().UTC()
	}
	m.jobs = append(m.jobs, job)
	return nil
}

// Jobs returns a snapshot copy of the enqueued jobs.
func (m *MemoryEnqueuer) Jobs() []WelcomeEmailJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]WelcomeEmailJob, len(m.jobs))
	copy(out, m.jobs)
	return out
}
