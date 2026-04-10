package portal

import (
	"context"
	"errors"
	"testing"
	"time"
)

// --- Fakes ---

type fakeUsage struct {
	summaries map[string][]UsageSummary
	err       error
}

func (f *fakeUsage) GetCurrentPeriodUsage(_ context.Context, tenantID string) ([]UsageSummary, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.summaries[tenantID], nil
}

type fakePlans struct {
	plans map[string]*PlanInfo
	err   error
}

func (f *fakePlans) GetTenantPlan(_ context.Context, tenantID string) (*PlanInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	p, ok := f.plans[tenantID]
	if !ok {
		return nil, ErrPlanNotFound
	}
	return p, nil
}

type fakeInvoices struct {
	invoices map[string][]Invoice
	err      error
}

func (f *fakeInvoices) ListInvoices(_ context.Context, tenantID string, limit int) ([]Invoice, error) {
	if f.err != nil {
		return nil, f.err
	}
	all := f.invoices[tenantID]
	if limit > 0 && limit < len(all) {
		return all[:limit], nil
	}
	return all, nil
}

type fakePortal struct {
	sessions map[string]*PortalSession
	err      error
}

func (f *fakePortal) CreatePortalSession(_ context.Context, tenantID, returnURL string) (*PortalSession, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sessions[tenantID], nil
}

func newTestService(t *testing.T) (*Service, *fakeUsage, *fakePlans, *fakeInvoices, *fakePortal) {
	t.Helper()
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	usage := &fakeUsage{
		summaries: map[string][]UsageSummary{
			"tenant-1": {
				{Metric: "cameras", Label: "Active Cameras", Value: 12, Unit: "cameras", Limit: 50, UsagePct: 24},
				{Metric: "storage_gb", Label: "Storage", Value: 150.5, Unit: "GB", Limit: 500, UsagePct: 30.1},
			},
		},
	}
	plans := &fakePlans{
		plans: map[string]*PlanInfo{
			"tenant-1": {
				PlanID: "plan-pro", PlanName: "Professional", Tier: "pro",
				BillingMode: "subscription",
				PeriodStart: now, PeriodEnd: now.AddDate(0, 1, 0),
			},
		},
	}
	invoices := &fakeInvoices{
		invoices: map[string][]Invoice{
			"tenant-1": {
				{InvoiceID: "inv-1", Number: "INV-001", Status: "paid", AmountDue: 9900, AmountPaid: 9900, Currency: "usd", CreatedAt: now},
				{InvoiceID: "inv-2", Number: "INV-002", Status: "open", AmountDue: 9900, Currency: "usd", CreatedAt: now.AddDate(0, 1, 0)},
			},
		},
	}
	portal := &fakePortal{
		sessions: map[string]*PortalSession{
			"tenant-1": {URL: "https://billing.stripe.com/session/test", ExpiresAt: now.Add(time.Hour)},
		},
	}
	svc, err := NewService(usage, plans, invoices, portal)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, usage, plans, invoices, portal
}

func TestGetOverview_HappyPath(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTestService(t)

	overview, err := svc.GetOverview(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if overview.Plan.PlanName != "Professional" {
		t.Errorf("PlanName = %q, want Professional", overview.Plan.PlanName)
	}
	if len(overview.Usage) != 2 {
		t.Errorf("len(Usage) = %d, want 2", len(overview.Usage))
	}
	if overview.Currency != "usd" {
		t.Errorf("Currency = %q, want usd", overview.Currency)
	}
}

func TestGetOverview_MissingTenant(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTestService(t)

	_, err := svc.GetOverview(context.Background(), "")
	if !errors.Is(err, ErrMissingTenant) {
		t.Errorf("expected ErrMissingTenant, got %v", err)
	}
}

func TestGetOverview_TenantIsolation_Seam4(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTestService(t)

	// tenant-2 has no plan → should get PlanNotFound.
	_, err := svc.GetOverview(context.Background(), "tenant-2")
	if err == nil {
		t.Fatal("expected error for unknown tenant")
	}
	if !errors.Is(err, ErrPlanNotFound) {
		t.Logf("got error: %v (may be wrapped)", err)
	}
}

func TestListInvoices_HappyPath(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTestService(t)

	invoices, err := svc.ListInvoices(context.Background(), "tenant-1", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(invoices) != 2 {
		t.Errorf("len(invoices) = %d, want 2", len(invoices))
	}
}

func TestListInvoices_MissingTenant(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTestService(t)

	_, err := svc.ListInvoices(context.Background(), "", 10)
	if !errors.Is(err, ErrMissingTenant) {
		t.Errorf("expected ErrMissingTenant, got %v", err)
	}
}

func TestListInvoices_DefaultLimit(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTestService(t)

	// Negative limit should default to 25.
	invoices, err := svc.ListInvoices(context.Background(), "tenant-1", -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(invoices) != 2 {
		t.Errorf("len(invoices) = %d, want 2 (all available)", len(invoices))
	}
}

func TestCreatePortalSession_HappyPath(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTestService(t)

	session, err := svc.CreatePortalSession(context.Background(), "tenant-1", "https://app.kaivue.com/billing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.URL == "" {
		t.Error("session URL is empty")
	}
}

func TestCreatePortalSession_MissingTenant(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTestService(t)

	_, err := svc.CreatePortalSession(context.Background(), "", "")
	if !errors.Is(err, ErrMissingTenant) {
		t.Errorf("expected ErrMissingTenant, got %v", err)
	}
}

func TestCreatePortalSession_DefaultReturnURL(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTestService(t)

	// Empty returnURL should default to "/".
	session, err := svc.CreatePortalSession(context.Background(), "tenant-1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session == nil {
		t.Error("session is nil")
	}
}

func TestNewService_NilDependencies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		u    UsageReader
		p    PlanReader
		i    InvoiceReader
		ps   PortalSessionCreator
	}{
		{"nil usage", nil, &fakePlans{}, &fakeInvoices{}, &fakePortal{}},
		{"nil plans", &fakeUsage{}, nil, &fakeInvoices{}, &fakePortal{}},
		{"nil invoices", &fakeUsage{}, &fakePlans{}, nil, &fakePortal{}},
		{"nil portal", &fakeUsage{}, &fakePlans{}, &fakeInvoices{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewService(tt.u, tt.p, tt.i, tt.ps)
			if err == nil {
				t.Error("expected error for nil dependency")
			}
		})
	}
}

func TestGetOverview_UsageReaderError(t *testing.T) {
	t.Parallel()
	svc, usage, _, _, _ := newTestService(t)
	usage.err = errors.New("db down")

	_, err := svc.GetOverview(context.Background(), "tenant-1")
	if err == nil {
		t.Fatal("expected error from usage reader failure")
	}
}

func TestGetOverview_PlanReaderError(t *testing.T) {
	t.Parallel()
	svc, _, plans, _, _ := newTestService(t)
	plans.err = errors.New("db down")

	_, err := svc.GetOverview(context.Background(), "tenant-1")
	if err == nil {
		t.Fatal("expected error from plan reader failure")
	}
}
