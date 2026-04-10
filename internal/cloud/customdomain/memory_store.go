package customdomain

import (
	"context"
	"fmt"
	"sync"
)

// MemoryStore is an in-memory DomainStore for testing.
type MemoryStore struct {
	mu      sync.RWMutex
	domains map[string]*Domain // key: tenantID + "|" + domain
}

// NewMemoryStore returns a new in-memory domain store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{domains: make(map[string]*Domain)}
}

func storeKey(tenantID, domain string) string {
	return tenantID + "|" + domain
}

func (s *MemoryStore) Insert(_ context.Context, d *Domain) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := storeKey(d.TenantID, d.Domain)
	if _, exists := s.domains[key]; exists {
		return fmt.Errorf("domain already exists: %s for tenant %s", d.Domain, d.TenantID)
	}
	cp := *d
	s.domains[key] = &cp
	return nil
}

func (s *MemoryStore) GetByTenantAndDomain(_ context.Context, tenantID, domain string) (*Domain, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.domains[storeKey(tenantID, domain)]
	if !ok {
		return nil, fmt.Errorf("domain not found: %s for tenant %s", domain, tenantID)
	}
	cp := *d
	return &cp, nil
}

func (s *MemoryStore) Update(_ context.Context, d *Domain) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := storeKey(d.TenantID, d.Domain)
	if _, ok := s.domains[key]; !ok {
		return fmt.Errorf("domain not found: %s for tenant %s", d.Domain, d.TenantID)
	}
	cp := *d
	s.domains[key] = &cp
	return nil
}

func (s *MemoryStore) Delete(_ context.Context, tenantID, domain string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := storeKey(tenantID, domain)
	if _, ok := s.domains[key]; !ok {
		return fmt.Errorf("domain not found: %s for tenant %s", domain, tenantID)
	}
	delete(s.domains, key)
	return nil
}

func (s *MemoryStore) ListByTenant(_ context.Context, tenantID string) ([]*Domain, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Domain
	for _, d := range s.domains {
		if d.TenantID == tenantID {
			cp := *d
			result = append(result, &cp)
		}
	}
	return result, nil
}

// MockDNSResolver is a test DNS resolver that returns preconfigured CNAME results.
type MockDNSResolver struct {
	Results map[string]string // host -> cname
	Err     error
}

func (m *MockDNSResolver) LookupCNAME(_ context.Context, host string) (string, error) {
	if m.Err != nil {
		return "", m.Err
	}
	cname, ok := m.Results[host]
	if !ok {
		return "", fmt.Errorf("no CNAME record for %s", host)
	}
	return cname, nil
}

// MockCertProvider is a test cert provider.
type MockCertProvider struct {
	ProvisionResult string
	ProvisionErr    error
	RevokeErr       error
	Provisioned     []string
	Revoked         []string
}

func (m *MockCertProvider) Provision(_ context.Context, domain string) (string, error) {
	if m.ProvisionErr != nil {
		return "", m.ProvisionErr
	}
	m.Provisioned = append(m.Provisioned, domain)
	if m.ProvisionResult != "" {
		return m.ProvisionResult, nil
	}
	return fmt.Sprintf("arn:aws:acm:us-east-1:123456:certificate/%s", domain), nil
}

func (m *MockCertProvider) Revoke(_ context.Context, certID string) error {
	m.Revoked = append(m.Revoked, certID)
	return m.RevokeErr
}
