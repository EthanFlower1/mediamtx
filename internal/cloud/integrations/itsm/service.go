package itsm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// IDGen generates a random hex identifier.
type IDGen func() string

func defaultIDGen() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ProviderFactory creates a Provider from a ProviderConfig.
type ProviderFactory func(cfg ProviderConfig) (Provider, error)

// DefaultProviderFactory creates PagerDuty or Opsgenie clients from config.
func DefaultProviderFactory(cfg ProviderConfig) (Provider, error) {
	switch cfg.Provider {
	case ProviderPagerDuty:
		opts := []PagerDutyOption{}
		if cfg.Endpoint != "" {
			opts = append(opts, WithPagerDutyEndpoint(cfg.Endpoint))
		}
		return NewPagerDutyClient(cfg.APIKey, opts...)

	case ProviderOpsgenie:
		opts := []OpsgenieOption{}
		if cfg.Endpoint != "" {
			opts = append(opts, WithOpsgenieEndpoint(cfg.Endpoint))
		}
		return NewOpsgenieClient(cfg.APIKey, opts...)

	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProvider, cfg.Provider)
	}
}

// Config bundles dependencies for Service.
type Config struct {
	// Store persists provider configurations and routing rules. Required.
	Store Store

	// Factory creates provider clients from configs. Defaults to DefaultProviderFactory.
	Factory ProviderFactory

	// IDGen generates unique identifiers. Defaults to random hex.
	IDGen IDGen
}

// Store is the persistence interface for ITSM configurations and routing rules.
type Store interface {
	// Provider configs
	UpsertProviderConfig(ctx context.Context, cfg ProviderConfig) error
	GetProviderConfig(ctx context.Context, tenantID, configID string) (ProviderConfig, error)
	ListProviderConfigs(ctx context.Context, tenantID string) ([]ProviderConfig, error)
	DeleteProviderConfig(ctx context.Context, tenantID, configID string) error

	// Routing rules
	UpsertRoutingRule(ctx context.Context, rule RoutingRule) error
	GetRoutingRule(ctx context.Context, tenantID, ruleID string) (RoutingRule, error)
	ListRoutingRules(ctx context.Context, tenantID string) ([]RoutingRule, error)
	DeleteRoutingRule(ctx context.Context, tenantID, ruleID string) error
}

// Service is the top-level ITSM integration service that manages provider
// configurations, routing rules, and alert dispatching.
type Service struct {
	store   Store
	factory ProviderFactory
	idGen   IDGen
	router  *Router
}

// NewService constructs a new ITSM integration service.
func NewService(cfg Config) (*Service, error) {
	if cfg.Store == nil {
		return nil, errors.New("itsm: store is required")
	}
	factory := cfg.Factory
	if factory == nil {
		factory = DefaultProviderFactory
	}
	idGen := cfg.IDGen
	if idGen == nil {
		idGen = defaultIDGen
	}
	return &Service{
		store:   cfg.Store,
		factory: factory,
		idGen:   idGen,
		router:  NewRouter(),
	}, nil
}

// LoadTenantRouting loads a tenant's provider configs and routing rules into
// the in-memory router. Call this on startup or after config changes.
func (s *Service) LoadTenantRouting(ctx context.Context, tenantID string) error {
	configs, err := s.store.ListProviderConfigs(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("itsm: load provider configs: %w", err)
	}

	for _, cfg := range configs {
		if !cfg.Enabled {
			s.router.RemoveProvider(cfg.ConfigID)
			continue
		}
		p, err := s.factory(cfg)
		if err != nil {
			return fmt.Errorf("itsm: create provider %s: %w", cfg.ConfigID, err)
		}
		s.router.RegisterProvider(cfg.ConfigID, p)
	}

	rules, err := s.store.ListRoutingRules(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("itsm: load routing rules: %w", err)
	}
	s.router.SetRules(rules)

	return nil
}

// SaveProviderConfig persists a provider configuration.
func (s *Service) SaveProviderConfig(ctx context.Context, cfg ProviderConfig) (ProviderConfig, error) {
	now := time.Now().UTC()
	if cfg.ConfigID == "" {
		cfg.ConfigID = s.idGen()
		cfg.CreatedAt = now
	}
	cfg.UpdatedAt = now

	// Validate the config by attempting to create a provider.
	if _, err := s.factory(cfg); err != nil {
		return ProviderConfig{}, err
	}

	if err := s.store.UpsertProviderConfig(ctx, cfg); err != nil {
		return ProviderConfig{}, fmt.Errorf("itsm: save provider config: %w", err)
	}
	return cfg, nil
}

// GetProviderConfig retrieves a provider configuration.
func (s *Service) GetProviderConfig(ctx context.Context, tenantID, configID string) (ProviderConfig, error) {
	return s.store.GetProviderConfig(ctx, tenantID, configID)
}

// ListProviderConfigs returns all provider configs for a tenant.
func (s *Service) ListProviderConfigs(ctx context.Context, tenantID string) ([]ProviderConfig, error) {
	return s.store.ListProviderConfigs(ctx, tenantID)
}

// DeleteProviderConfig removes a provider configuration and unregisters it.
func (s *Service) DeleteProviderConfig(ctx context.Context, tenantID, configID string) error {
	if err := s.store.DeleteProviderConfig(ctx, tenantID, configID); err != nil {
		return err
	}
	s.router.RemoveProvider(configID)
	return nil
}

// SaveRoutingRule persists a routing rule.
func (s *Service) SaveRoutingRule(ctx context.Context, rule RoutingRule) (RoutingRule, error) {
	now := time.Now().UTC()
	if rule.RuleID == "" {
		rule.RuleID = s.idGen()
		rule.CreatedAt = now
	}
	rule.UpdatedAt = now

	if err := s.store.UpsertRoutingRule(ctx, rule); err != nil {
		return RoutingRule{}, fmt.Errorf("itsm: save routing rule: %w", err)
	}
	return rule, nil
}

// GetRoutingRule retrieves a routing rule.
func (s *Service) GetRoutingRule(ctx context.Context, tenantID, ruleID string) (RoutingRule, error) {
	return s.store.GetRoutingRule(ctx, tenantID, ruleID)
}

// ListRoutingRules returns all routing rules for a tenant.
func (s *Service) ListRoutingRules(ctx context.Context, tenantID string) ([]RoutingRule, error) {
	return s.store.ListRoutingRules(ctx, tenantID)
}

// DeleteRoutingRule removes a routing rule.
func (s *Service) DeleteRoutingRule(ctx context.Context, tenantID, ruleID string) error {
	return s.store.DeleteRoutingRule(ctx, tenantID, ruleID)
}

// SendAlert dispatches an alert through the routing engine.
func (s *Service) SendAlert(ctx context.Context, alert Alert) ([]AlertResult, error) {
	return s.router.Route(ctx, alert)
}

// TestProvider sends a test alert to verify a specific provider config.
func (s *Service) TestProvider(ctx context.Context, tenantID, configID string) error {
	cfg, err := s.store.GetProviderConfig(ctx, tenantID, configID)
	if err != nil {
		return err
	}
	p, err := s.factory(cfg)
	if err != nil {
		return err
	}
	return p.TestConnection(ctx)
}
