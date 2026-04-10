package portal

import (
	"context"
	"errors"
	"fmt"
)

// Service implements the billing portal business logic.
type Service struct {
	usage   UsageReader
	plans   PlanReader
	invoices InvoiceReader
	portal  PortalSessionCreator
}

// NewService constructs a billing portal service. All dependencies are required.
func NewService(usage UsageReader, plans PlanReader, invoices InvoiceReader, portal PortalSessionCreator) (*Service, error) {
	if usage == nil {
		return nil, errors.New("portal: UsageReader is required")
	}
	if plans == nil {
		return nil, errors.New("portal: PlanReader is required")
	}
	if invoices == nil {
		return nil, errors.New("portal: InvoiceReader is required")
	}
	if portal == nil {
		return nil, errors.New("portal: PortalSessionCreator is required")
	}
	return &Service{
		usage:    usage,
		plans:    plans,
		invoices: invoices,
		portal:   portal,
	}, nil
}

// GetOverview returns the billing overview for a tenant: plan info + current
// usage metrics. This is the primary endpoint the React admin billing page calls.
func (s *Service) GetOverview(ctx context.Context, tenantID string) (*BillingOverview, error) {
	if tenantID == "" {
		return nil, ErrMissingTenant
	}

	plan, err := s.plans.GetTenantPlan(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("portal: get plan: %w", err)
	}

	usage, err := s.usage.GetCurrentPeriodUsage(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("portal: get usage: %w", err)
	}

	return &BillingOverview{
		Plan:     *plan,
		Usage:    usage,
		Currency: "usd",
	}, nil
}

// ListInvoices returns recent invoices for a tenant.
func (s *Service) ListInvoices(ctx context.Context, tenantID string, limit int) ([]Invoice, error) {
	if tenantID == "" {
		return nil, ErrMissingTenant
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	return s.invoices.ListInvoices(ctx, tenantID, limit)
}

// CreatePortalSession creates a Stripe billing portal session that redirects
// the customer to manage their payment methods, view invoices, etc.
func (s *Service) CreatePortalSession(ctx context.Context, tenantID, returnURL string) (*PortalSession, error) {
	if tenantID == "" {
		return nil, ErrMissingTenant
	}
	if returnURL == "" {
		returnURL = "/"
	}
	return s.portal.CreatePortalSession(ctx, tenantID, returnURL)
}
