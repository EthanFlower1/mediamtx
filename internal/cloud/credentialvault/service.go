package credentialvault

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Clock abstracts time for testing.
type Clock func() time.Time

// Config bundles the dependencies of a Service.
type Config struct {
	Backend    VaultBackend
	AuditHook  AuditHook
	Clock      Clock
}

// Service manages integrator signing credentials with per-tenant isolation.
type Service struct {
	backend   VaultBackend
	auditHook AuditHook
	clock     Clock
}

// NewService constructs a Service. Backend is required; AuditHook and Clock
// default to no-op and time.Now respectively.
func NewService(cfg Config) (*Service, error) {
	if cfg.Backend == nil {
		return nil, errors.New("credentialvault: backend is required")
	}
	clock := cfg.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	auditHook := cfg.AuditHook
	if auditHook == nil {
		auditHook = func(context.Context, AuditEvent) {}
	}
	return &Service{
		backend:   cfg.Backend,
		auditHook: auditHook,
		clock:     clock,
	}, nil
}

// secretPath builds the Secrets Manager path for a credential.
// Format: kaivue/{tenant_id}/mobile/{credential_type}
func secretPath(tenantID string, credType CredentialType) string {
	return fmt.Sprintf("kaivue/%s/mobile/%s", tenantID, credType)
}

// tenantPrefix returns the Secrets Manager prefix for listing all credentials
// belonging to a tenant.
func tenantPrefix(tenantID string) string {
	return fmt.Sprintf("kaivue/%s/mobile/", tenantID)
}

// StoreCredential stores a new signing credential for an integrator tenant.
func (s *Service) StoreCredential(ctx context.Context, req StoreRequest) (*Credential, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	path := secretPath(req.TenantID, req.Type)
	secretID, version, err := s.backend.Store(ctx, path, req.Value)
	if err != nil {
		return nil, fmt.Errorf("credentialvault: store: %w", err)
	}

	now := s.clock()
	cred := &Credential{
		TenantID:  req.TenantID,
		Type:      req.Type,
		Label:     req.Label,
		SecretID:  secretID,
		Version:   version,
		CreatedAt: now,
		RotatedAt: now,
		ExpiresAt: req.ExpiresAt,
	}

	s.auditHook(ctx, AuditEvent{
		Action:   "store",
		TenantID: req.TenantID,
		Type:     req.Type,
		SecretID: secretID,
	})

	return cred, nil
}

// GetCredential retrieves a credential's secret material.
func (s *Service) GetCredential(ctx context.Context, tenantID string, credType CredentialType) ([]byte, *Credential, error) {
	if tenantID == "" {
		return nil, nil, errors.New("credentialvault: tenant_id is required")
	}
	if !credType.IsValid() {
		return nil, nil, errors.New("credentialvault: invalid credential type")
	}

	path := secretPath(tenantID, credType)
	value, version, err := s.backend.Get(ctx, path)
	if err != nil {
		return nil, nil, fmt.Errorf("credentialvault: get: %w", err)
	}

	cred := &Credential{
		TenantID: tenantID,
		Type:     credType,
		SecretID: path,
		Version:  version,
	}
	return value, cred, nil
}

// RotateCredential replaces a credential's secret material with a new value.
func (s *Service) RotateCredential(ctx context.Context, req RotateRequest) (*Credential, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	path := secretPath(req.TenantID, req.Type)
	version, err := s.backend.Rotate(ctx, path, req.NewValue)
	if err != nil {
		return nil, fmt.Errorf("credentialvault: rotate: %w", err)
	}

	now := s.clock()
	cred := &Credential{
		TenantID:  req.TenantID,
		Type:      req.Type,
		SecretID:  path,
		Version:   version,
		RotatedAt: now,
		ExpiresAt: req.ExpiresAt,
	}

	s.auditHook(ctx, AuditEvent{
		Action:   "rotate",
		TenantID: req.TenantID,
		Type:     req.Type,
		SecretID: path,
	})

	return cred, nil
}

// DeleteCredential removes a credential from the vault.
func (s *Service) DeleteCredential(ctx context.Context, tenantID string, credType CredentialType) error {
	if tenantID == "" {
		return errors.New("credentialvault: tenant_id is required")
	}
	if !credType.IsValid() {
		return errors.New("credentialvault: invalid credential type")
	}

	path := secretPath(tenantID, credType)
	if err := s.backend.Delete(ctx, path); err != nil {
		return fmt.Errorf("credentialvault: delete: %w", err)
	}

	s.auditHook(ctx, AuditEvent{
		Action:   "delete",
		TenantID: tenantID,
		Type:     credType,
		SecretID: path,
	})

	return nil
}

// ListCredentials returns the credential types stored for a tenant.
func (s *Service) ListCredentials(ctx context.Context, tenantID string) ([]CredentialType, error) {
	if tenantID == "" {
		return nil, errors.New("credentialvault: tenant_id is required")
	}

	prefix := tenantPrefix(tenantID)
	paths, err := s.backend.List(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("credentialvault: list: %w", err)
	}

	types := make([]CredentialType, 0, len(paths))
	for _, p := range paths {
		// Extract credential type from path suffix
		if len(p) > len(prefix) {
			ct := CredentialType(p[len(prefix):])
			if ct.IsValid() {
				types = append(types, ct)
			}
		}
	}
	return types, nil
}
