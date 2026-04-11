package itsm

import (
	"context"
	"fmt"
	"sync"
)

// MemStore is an in-memory Store implementation for testing.
type MemStore struct {
	mu      sync.RWMutex
	configs map[string]map[string]ProviderConfig // tenantID -> configID -> config
	rules   map[string]map[string]RoutingRule    // tenantID -> ruleID -> rule
}

// NewMemStore creates an empty in-memory store.
func NewMemStore() *MemStore {
	return &MemStore{
		configs: make(map[string]map[string]ProviderConfig),
		rules:   make(map[string]map[string]RoutingRule),
	}
}

func (m *MemStore) UpsertProviderConfig(_ context.Context, cfg ProviderConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.configs[cfg.TenantID] == nil {
		m.configs[cfg.TenantID] = make(map[string]ProviderConfig)
	}
	m.configs[cfg.TenantID][cfg.ConfigID] = cfg
	return nil
}

func (m *MemStore) GetProviderConfig(_ context.Context, tenantID, configID string) (ProviderConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if t, ok := m.configs[tenantID]; ok {
		if c, ok := t[configID]; ok {
			return c, nil
		}
	}
	return ProviderConfig{}, fmt.Errorf("%w: config_id=%s", ErrProviderNotFound, configID)
}

func (m *MemStore) ListProviderConfigs(_ context.Context, tenantID string) ([]ProviderConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []ProviderConfig
	for _, c := range m.configs[tenantID] {
		out = append(out, c)
	}
	return out, nil
}

func (m *MemStore) DeleteProviderConfig(_ context.Context, tenantID, configID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.configs[tenantID]; ok {
		if _, ok := t[configID]; ok {
			delete(t, configID)
			return nil
		}
	}
	return fmt.Errorf("%w: config_id=%s", ErrProviderNotFound, configID)
}

func (m *MemStore) UpsertRoutingRule(_ context.Context, rule RoutingRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rules[rule.TenantID] == nil {
		m.rules[rule.TenantID] = make(map[string]RoutingRule)
	}
	m.rules[rule.TenantID][rule.RuleID] = rule
	return nil
}

func (m *MemStore) GetRoutingRule(_ context.Context, tenantID, ruleID string) (RoutingRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if t, ok := m.rules[tenantID]; ok {
		if r, ok := t[ruleID]; ok {
			return r, nil
		}
	}
	return RoutingRule{}, fmt.Errorf("%w: rule_id=%s", ErrRuleNotFound, ruleID)
}

func (m *MemStore) ListRoutingRules(_ context.Context, tenantID string) ([]RoutingRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []RoutingRule
	for _, r := range m.rules[tenantID] {
		out = append(out, r)
	}
	return out, nil
}

func (m *MemStore) DeleteRoutingRule(_ context.Context, tenantID, ruleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.rules[tenantID]; ok {
		if _, ok := t[ruleID]; ok {
			delete(t, ruleID)
			return nil
		}
	}
	return fmt.Errorf("%w: rule_id=%s", ErrRuleNotFound, ruleID)
}
