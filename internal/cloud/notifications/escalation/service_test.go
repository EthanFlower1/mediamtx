package escalation_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/notifications/escalation"
)

// ---------- test helpers ----------

func openTestDB(t *testing.T) *clouddb.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := "sqlite://" + filepath.Join(dir, "cloud.db")
	d, err := clouddb.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

var (
	seqMu sync.Mutex
	seqID int
)

func testIDGen() string {
	seqMu.Lock()
	defer seqMu.Unlock()
	seqID++
	return fmt.Sprintf("test-%04d", seqID)
}

type fakeNotifier struct {
	mu      sync.Mutex
	calls   []notifyCall
}

type notifyCall struct {
	AlertID string
	Step    escalation.Step
}

func (f *fakeNotifier) Notify(alertID string, step escalation.Step) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, notifyCall{AlertID: alertID, Step: step})
	return nil
}

func (f *fakeNotifier) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeNotifier) LastCall() notifyCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[len(f.calls)-1]
}

type fakePagerDuty struct {
	mu    sync.Mutex
	calls []pdCall
}

type pdCall struct {
	AlertID     string
	TenantID    string
	Description string
}

func (f *fakePagerDuty) CreateIncident(alertID, tenantID, description string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, pdCall{AlertID: alertID, TenantID: tenantID, Description: description})
	return nil
}

func (f *fakePagerDuty) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock() *testClock {
	return &testClock{now: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newService(t *testing.T, notifier escalation.Notifier, pd escalation.PagerDutyClient) (*escalation.Service, *testClock) {
	t.Helper()
	db := openTestDB(t)
	clock := newTestClock()
	svc, err := escalation.NewService(escalation.Config{
		DB:              db,
		IDGen:           testIDGen,
		Clock:           clock.Now,
		Notifier:        notifier,
		PagerDutyClient: pd,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc, clock
}

// ---------- Chain CRUD tests ----------

func TestCreateAndGetChain(t *testing.T) {
	svc, _ := newService(t, nil, nil)
	ctx := context.Background()

	chain, steps, err := svc.CreateChain(ctx, escalation.Chain{
		TenantID:    "tenant-1",
		Name:        "Camera Offline",
		Description: "Escalate camera offline alerts",
		Enabled:     true,
	}, []escalation.Step{
		{TargetType: escalation.TargetUser, TargetID: "user-1", ChannelType: escalation.ChannelPush, TimeoutSeconds: 300},
		{TargetType: escalation.TargetUser, TargetID: "user-2", ChannelType: escalation.ChannelEmail, TimeoutSeconds: 600},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if chain.ChainID == "" {
		t.Fatal("expected chain_id to be set")
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if steps[0].StepOrder != 1 || steps[1].StepOrder != 2 {
		t.Errorf("expected step orders 1,2; got %d,%d", steps[0].StepOrder, steps[1].StepOrder)
	}

	// Get chain back.
	got, err := svc.GetChain(ctx, "tenant-1", chain.ChainID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Camera Offline" {
		t.Errorf("expected 'Camera Offline', got %q", got.Name)
	}
}

func TestListChains(t *testing.T) {
	svc, _ := newService(t, nil, nil)
	ctx := context.Background()

	svc.CreateChain(ctx, escalation.Chain{TenantID: "tenant-1", Name: "Chain A", Enabled: true}, nil)
	svc.CreateChain(ctx, escalation.Chain{TenantID: "tenant-1", Name: "Chain B", Enabled: true}, nil)
	svc.CreateChain(ctx, escalation.Chain{TenantID: "tenant-2", Name: "Chain C", Enabled: true}, nil)

	chains, err := svc.ListChains(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(chains) != 2 {
		t.Fatalf("expected 2 chains for tenant-1, got %d", len(chains))
	}
}

func TestDeleteChain(t *testing.T) {
	svc, _ := newService(t, nil, nil)
	ctx := context.Background()

	chain, _, _ := svc.CreateChain(ctx, escalation.Chain{
		TenantID: "tenant-1",
		Name:     "Deleteme",
		Enabled:  true,
	}, []escalation.Step{
		{TargetType: escalation.TargetUser, TargetID: "user-1", ChannelType: escalation.ChannelPush, TimeoutSeconds: 300},
	})

	if err := svc.DeleteChain(ctx, "tenant-1", chain.ChainID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := svc.GetChain(ctx, "tenant-1", chain.ChainID)
	if err != escalation.ErrChainNotFound {
		t.Errorf("expected ErrChainNotFound, got %v", err)
	}

	// Steps should be gone too.
	steps, err := svc.GetSteps(ctx, "tenant-1", chain.ChainID)
	if err != nil {
		t.Fatalf("get steps: %v", err)
	}
	if len(steps) != 0 {
		t.Errorf("expected 0 steps after delete, got %d", len(steps))
	}
}

func TestCreateChainValidation(t *testing.T) {
	svc, _ := newService(t, nil, nil)
	ctx := context.Background()

	_, _, err := svc.CreateChain(ctx, escalation.Chain{TenantID: "tenant-1"}, nil)
	if err == nil {
		t.Fatal("expected error for missing name")
	}

	_, _, err = svc.CreateChain(ctx, escalation.Chain{Name: "Test"}, nil)
	if err == nil {
		t.Fatal("expected error for missing tenant_id")
	}
}

// ---------- Escalation state machine tests ----------

func TestStartEscalation(t *testing.T) {
	notifier := &fakeNotifier{}
	svc, _ := newService(t, notifier, nil)
	ctx := context.Background()

	chain, _, _ := svc.CreateChain(ctx, escalation.Chain{
		TenantID: "tenant-1",
		Name:     "Test Chain",
		Enabled:  true,
	}, []escalation.Step{
		{TargetType: escalation.TargetUser, TargetID: "user-1", ChannelType: escalation.ChannelPush, TimeoutSeconds: 300},
		{TargetType: escalation.TargetUser, TargetID: "user-2", ChannelType: escalation.ChannelEmail, TimeoutSeconds: 600},
	})

	ae, err := svc.StartEscalation(ctx, "tenant-1", "alert-001", chain.ChainID)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if ae.State != escalation.StateNotified {
		t.Errorf("expected state notified, got %s", ae.State)
	}
	if ae.CurrentStep != 1 {
		t.Errorf("expected current_step 1, got %d", ae.CurrentStep)
	}
	if notifier.CallCount() != 1 {
		t.Errorf("expected 1 notify call, got %d", notifier.CallCount())
	}
	if notifier.LastCall().Step.TargetID != "user-1" {
		t.Errorf("expected notify for user-1, got %s", notifier.LastCall().Step.TargetID)
	}
}

func TestStartEscalationNoSteps(t *testing.T) {
	svc, _ := newService(t, nil, nil)
	ctx := context.Background()

	chain, _, _ := svc.CreateChain(ctx, escalation.Chain{
		TenantID: "tenant-1",
		Name:     "Empty Chain",
		Enabled:  true,
	}, nil)

	_, err := svc.StartEscalation(ctx, "tenant-1", "alert-002", chain.ChainID)
	if err != escalation.ErrChainNoSteps {
		t.Errorf("expected ErrChainNoSteps, got %v", err)
	}
}

func TestAcknowledgeStopsEscalation(t *testing.T) {
	notifier := &fakeNotifier{}
	svc, clock := newService(t, notifier, nil)
	ctx := context.Background()

	chain, _, _ := svc.CreateChain(ctx, escalation.Chain{
		TenantID: "tenant-1",
		Name:     "Ack Test",
		Enabled:  true,
	}, []escalation.Step{
		{TargetType: escalation.TargetUser, TargetID: "user-1", ChannelType: escalation.ChannelPush, TimeoutSeconds: 300},
		{TargetType: escalation.TargetUser, TargetID: "user-2", ChannelType: escalation.ChannelEmail, TimeoutSeconds: 300},
	})

	svc.StartEscalation(ctx, "tenant-1", "alert-ack", chain.ChainID)

	// Acknowledge before timeout.
	ae, err := svc.AcknowledgeAlert(ctx, "tenant-1", "alert-ack", "user-1")
	if err != nil {
		t.Fatalf("ack: %v", err)
	}
	if ae.State != escalation.StateAcknowledged {
		t.Errorf("expected acknowledged, got %s", ae.State)
	}
	if ae.AckedBy != "user-1" {
		t.Errorf("expected acked_by user-1, got %s", ae.AckedBy)
	}

	// Advancing time and processing should NOT escalate further.
	clock.Advance(10 * time.Minute)
	processed, err := svc.ProcessTimeouts(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if processed != 0 {
		t.Errorf("expected 0 processed (ack should stop escalation), got %d", processed)
	}

	// Double-ack returns error.
	_, err = svc.AcknowledgeAlert(ctx, "tenant-1", "alert-ack", "user-1")
	if err != escalation.ErrAlreadyAcknowledged {
		t.Errorf("expected ErrAlreadyAcknowledged, got %v", err)
	}
}

func TestEscalationProgression(t *testing.T) {
	notifier := &fakeNotifier{}
	svc, clock := newService(t, notifier, nil)
	ctx := context.Background()

	chain, _, _ := svc.CreateChain(ctx, escalation.Chain{
		TenantID: "tenant-1",
		Name:     "Progression",
		Enabled:  true,
	}, []escalation.Step{
		{TargetType: escalation.TargetUser, TargetID: "user-1", ChannelType: escalation.ChannelPush, TimeoutSeconds: 300},
		{TargetType: escalation.TargetUser, TargetID: "user-2", ChannelType: escalation.ChannelEmail, TimeoutSeconds: 600},
	})

	svc.StartEscalation(ctx, "tenant-1", "alert-prog", chain.ChainID)
	if notifier.CallCount() != 1 {
		t.Fatalf("expected 1 notify after start, got %d", notifier.CallCount())
	}

	// Advance past tier 1 timeout (300s).
	clock.Advance(6 * time.Minute)
	processed, err := svc.ProcessTimeouts(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 processed, got %d", processed)
	}

	// Should have notified tier 2.
	if notifier.CallCount() != 2 {
		t.Fatalf("expected 2 notify calls, got %d", notifier.CallCount())
	}
	if notifier.LastCall().Step.TargetID != "user-2" {
		t.Errorf("expected tier 2 notify for user-2, got %s", notifier.LastCall().Step.TargetID)
	}

	// Check state is notified at step 2.
	ae, err := svc.GetAlertEscalation(ctx, "tenant-1", "alert-prog")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if ae.CurrentStep != 2 {
		t.Errorf("expected current_step 2, got %d", ae.CurrentStep)
	}
	if ae.State != escalation.StateNotified {
		t.Errorf("expected state notified, got %s", ae.State)
	}
}

func TestPagerDutyFallback(t *testing.T) {
	notifier := &fakeNotifier{}
	pd := &fakePagerDuty{}
	svc, clock := newService(t, notifier, pd)
	ctx := context.Background()

	chain, _, _ := svc.CreateChain(ctx, escalation.Chain{
		TenantID: "tenant-1",
		Name:     "PD Fallback",
		Enabled:  true,
	}, []escalation.Step{
		{TargetType: escalation.TargetUser, TargetID: "user-1", ChannelType: escalation.ChannelPush, TimeoutSeconds: 300},
		{TargetType: escalation.TargetPagerDuty, TargetID: "pd-service-1", ChannelType: escalation.ChannelPagerDuty, TimeoutSeconds: 300},
	})

	svc.StartEscalation(ctx, "tenant-1", "alert-pd", chain.ChainID)

	// Tier 1 timeout -> escalate to PagerDuty tier.
	clock.Advance(6 * time.Minute)
	processed, _ := svc.ProcessTimeouts(ctx, "tenant-1")
	if processed != 1 {
		t.Fatalf("expected 1 processed, got %d", processed)
	}

	// PagerDuty should have been called when advancing to PD tier.
	if pd.CallCount() != 1 {
		t.Fatalf("expected 1 PD call, got %d", pd.CallCount())
	}

	// Tier 2 (PD) timeout -> should reach pagerduty_fallback terminal state.
	clock.Advance(6 * time.Minute)
	processed, _ = svc.ProcessTimeouts(ctx, "tenant-1")
	if processed != 1 {
		t.Fatalf("expected 1 processed, got %d", processed)
	}

	ae, err := svc.GetAlertEscalation(ctx, "tenant-1", "alert-pd")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if ae.State != escalation.StatePagerDutyFallback {
		t.Errorf("expected pagerduty_fallback state, got %s", ae.State)
	}

	// PD called again at exhaustion.
	if pd.CallCount() != 2 {
		t.Errorf("expected 2 PD calls total, got %d", pd.CallCount())
	}
}

func TestExhaustedWithoutPagerDuty(t *testing.T) {
	notifier := &fakeNotifier{}
	svc, clock := newService(t, notifier, nil)
	ctx := context.Background()

	chain, _, _ := svc.CreateChain(ctx, escalation.Chain{
		TenantID: "tenant-1",
		Name:     "No PD",
		Enabled:  true,
	}, []escalation.Step{
		{TargetType: escalation.TargetUser, TargetID: "user-1", ChannelType: escalation.ChannelPush, TimeoutSeconds: 60},
	})

	svc.StartEscalation(ctx, "tenant-1", "alert-exhaust", chain.ChainID)

	// Timeout the only tier.
	clock.Advance(2 * time.Minute)
	svc.ProcessTimeouts(ctx, "tenant-1")

	ae, _ := svc.GetAlertEscalation(ctx, "tenant-1", "alert-exhaust")
	if ae.State != escalation.StateExhausted {
		t.Errorf("expected exhausted state, got %s", ae.State)
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	svc, _ := newService(t, nil, nil)
	ctx := context.Background()

	svc.CreateChain(ctx, escalation.Chain{TenantID: "tenant-1", Name: "T1", Enabled: true}, nil)
	svc.CreateChain(ctx, escalation.Chain{TenantID: "tenant-2", Name: "T2", Enabled: true}, nil)

	t1, _ := svc.ListChains(ctx, "tenant-1")
	t2, _ := svc.ListChains(ctx, "tenant-2")

	if len(t1) != 1 || t1[0].Name != "T1" {
		t.Errorf("tenant-1 should only see T1")
	}
	if len(t2) != 1 || t2[0].Name != "T2" {
		t.Errorf("tenant-2 should only see T2")
	}
}

func TestGetAlertNotFound(t *testing.T) {
	svc, _ := newService(t, nil, nil)
	ctx := context.Background()

	_, err := svc.GetAlertEscalation(ctx, "tenant-1", "nonexistent")
	if err != escalation.ErrAlertNotFound {
		t.Errorf("expected ErrAlertNotFound, got %v", err)
	}
}
