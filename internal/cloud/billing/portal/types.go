package portal

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors.
var (
	ErrMissingTenant = errors.New("portal: tenant_id is required")
	ErrPlanNotFound  = errors.New("portal: plan not found for tenant")
	ErrNoInvoices    = errors.New("portal: no invoices found")
)

// UsageSummary is the current billing period's usage for a single metric.
type UsageSummary struct {
	Metric    string  `json:"metric"`
	Label     string  `json:"label"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
	Limit     float64 `json:"limit,omitempty"`
	UsagePct  float64 `json:"usage_pct,omitempty"`
}

// PlanInfo describes the tenant's current billing plan.
type PlanInfo struct {
	PlanID      string    `json:"plan_id"`
	PlanName    string    `json:"plan_name"`
	Tier        string    `json:"tier"`
	BillingMode string    `json:"billing_mode"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
}

// Invoice represents a single Stripe invoice.
type Invoice struct {
	InvoiceID   string    `json:"invoice_id"`
	Number      string    `json:"number"`
	Status      string    `json:"status"`
	AmountDue   int64     `json:"amount_due"`
	AmountPaid  int64     `json:"amount_paid"`
	Currency    string    `json:"currency"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	PDFURL      string    `json:"pdf_url,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// PortalSession is a Stripe billing portal session URL.
type PortalSession struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

// BillingOverview is the aggregate response for the billing portal page.
type BillingOverview struct {
	Plan    PlanInfo       `json:"plan"`
	Usage   []UsageSummary `json:"usage"`
	Balance int64          `json:"balance_cents"`
	Currency string        `json:"currency"`
}

// --- Seam interfaces ---

// UsageReader reads metering aggregates for the current billing period.
type UsageReader interface {
	GetCurrentPeriodUsage(ctx context.Context, tenantID string) ([]UsageSummary, error)
}

// PlanReader reads plan catalog and tenant plan assignments.
type PlanReader interface {
	GetTenantPlan(ctx context.Context, tenantID string) (*PlanInfo, error)
}

// InvoiceReader reads Stripe invoice history for a tenant.
type InvoiceReader interface {
	ListInvoices(ctx context.Context, tenantID string, limit int) ([]Invoice, error)
}

// PortalSessionCreator creates Stripe billing portal sessions.
type PortalSessionCreator interface {
	CreatePortalSession(ctx context.Context, tenantID, returnURL string) (*PortalSession, error)
}
