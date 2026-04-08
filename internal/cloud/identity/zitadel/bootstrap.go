package zitadel

import (
	"context"
	"errors"
	"net/http"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// Integrator describes an integrator tenant for BootstrapIntegrator. It
// deliberately mirrors the KAI-227 tenant provisioning DTO so that service
// doesn't have to translate field-by-field when wiring the adapter in.
type Integrator struct {
	TenantID    string // Kaivue-internal integrator tenant ID
	DisplayName string
}

// CustomerTenant describes a customer tenant underneath a parent integrator.
type CustomerTenant struct {
	TenantID       string
	DisplayName    string
	ParentIntegratorOrgID string
}

// BootstrapIntegrator creates a Zitadel org for the given integrator and
// registers the TenantRef → orgID mapping in the adapter's cache. It is
// idempotent: if an org with the same name already exists, the existing
// org ID is returned.
//
// Errors are fail-closed: any transport or Zitadel failure returns a raw
// error (callers handle it), but the mapping cache is NOT populated on
// failure so a half-bootstrapped tenant is never used.
func (a *Adapter) BootstrapIntegrator(ctx context.Context, in Integrator) (string, error) {
	if in.TenantID == "" || in.DisplayName == "" {
		return "", errors.New("zitadel: BootstrapIntegrator requires TenantID and DisplayName")
	}
	req := orgCreateRequest{Name: in.DisplayName}
	var resp orgCreateResponse
	err := a.client.doJSON(ctx, http.MethodPost, "/management/v1/orgs", a.cfg.PlatformOrgID, req, &resp)
	if err != nil {
		if se := errAsSDK(err); se != nil && se.HTTPStatus == http.StatusConflict {
			// Already exists — this is idempotent success, but we don't
			// know the ID without a follow-up lookup. Fall through to a
			// search by name.
			return a.findOrgByName(ctx, in.DisplayName)
		}
		return "", err
	}
	if resp.ID == "" {
		return "", errors.New("zitadel: empty org id in create response")
	}
	tenant := auth.TenantRef{Type: auth.TenantTypeIntegrator, ID: in.TenantID}
	a.RegisterTenantMapping(tenant, resp.ID)
	return resp.ID, nil
}

// BootstrapCustomerTenant creates a Zitadel org under a parent integrator's
// org. The parent org ID must be the Zitadel ID (not the Kaivue TenantID)
// because Zitadel's org API takes a native ID.
func (a *Adapter) BootstrapCustomerTenant(ctx context.Context, ct CustomerTenant) (string, error) {
	if ct.TenantID == "" || ct.DisplayName == "" || ct.ParentIntegratorOrgID == "" {
		return "", errors.New("zitadel: BootstrapCustomerTenant requires TenantID, DisplayName, ParentIntegratorOrgID")
	}
	req := orgCreateRequest{Name: ct.DisplayName}
	var resp orgCreateResponse
	if err := a.client.doJSON(ctx, http.MethodPost, "/management/v1/orgs", ct.ParentIntegratorOrgID, req, &resp); err != nil {
		if se := errAsSDK(err); se != nil && se.HTTPStatus == http.StatusConflict {
			return a.findOrgByName(ctx, ct.DisplayName)
		}
		return "", err
	}
	if resp.ID == "" {
		return "", errors.New("zitadel: empty org id in create response")
	}
	tenant := auth.TenantRef{Type: auth.TenantTypeCustomer, ID: ct.TenantID}
	a.RegisterTenantMapping(tenant, resp.ID)
	return resp.ID, nil
}

// ProvisionUser creates a user inside a tenant's org. This is a thin
// convenience wrapper around CreateUser that the tenant-provisioning
// service (KAI-227) calls during initial tenant setup.
func (a *Adapter) ProvisionUser(ctx context.Context, tenant auth.TenantRef, spec auth.UserSpec) (*auth.User, error) {
	return a.CreateUser(ctx, tenant, spec)
}

// findOrgByName is a minimal search used by the idempotent Bootstrap
// helpers to recover an existing org's ID after a 409 conflict.
func (a *Adapter) findOrgByName(ctx context.Context, name string) (string, error) {
	var resp struct {
		Result []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
	}
	body := map[string]any{
		"queries": []map[string]any{
			{"nameQuery": map[string]any{"name": name, "method": "TEXT_QUERY_METHOD_EQUALS"}},
		},
	}
	if err := a.client.doJSON(ctx, http.MethodPost, "/management/v1/orgs/_search", a.cfg.PlatformOrgID, body, &resp); err != nil {
		return "", err
	}
	for _, o := range resp.Result {
		if o.Name == name {
			return o.ID, nil
		}
	}
	return "", errors.New("zitadel: org conflict but not found by name")
}
