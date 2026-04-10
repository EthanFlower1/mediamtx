package models

import (
	"context"
	"errors"

	"github.com/bluenviron/mediamtx/internal/shared/inference"
)

// RegistryAdapter bridges the cloud model Store to the
// inference.ModelRegistry interface used by the inference runtime (KAI-280).
// It resolves model names to approved versions, falling back to
// platform-builtin models when the tenant has no match.
type RegistryAdapter struct {
	store    Store
	tenantID string
}

// NewRegistryAdapter constructs a RegistryAdapter scoped to a single tenant.
func NewRegistryAdapter(store Store, tenantID string) *RegistryAdapter {
	return &RegistryAdapter{store: store, tenantID: tenantID}
}

// Resolve satisfies inference.ModelRegistry. It looks up the latest approved
// model by name, first within the adapter's tenant, then falling back to the
// platform-builtin tenant.
//
// The returned bytes are nil because model bytes are not stored in the
// database — they live in S3/R2 and are referenced via file_ref. The caller
// (KAI-291 upload pipeline) is responsible for fetching the actual bytes from
// the file_ref on the returned model. For now we return nil bytes + the
// version string + nil error, which satisfies the interface contract.
func (r *RegistryAdapter) Resolve(ctx context.Context, modelID string) ([]byte, string, error) {
	// Try tenant-scoped first.
	m, err := r.store.ResolveApproved(ctx, r.tenantID, modelID)
	if err == nil {
		return nil, m.Version, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, "", err
	}

	// Fall back to platform-builtin models.
	m, err = r.store.ResolveApproved(ctx, PlatformBuiltinTenantID, modelID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, "", inference.ErrModelNotFound
		}
		return nil, "", err
	}
	return nil, m.Version, nil
}

// Compile-time interface check.
var _ inference.ModelRegistry = (*RegistryAdapter)(nil)
