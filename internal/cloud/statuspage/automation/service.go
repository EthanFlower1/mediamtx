package automation

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/statuspage/provider"
)

// Config holds dependencies for the automation Service.
type Config struct {
	// Provider is the Statuspage.io API client.
	Provider provider.Provider

	// Querier queries Prometheus for metric values.
	Querier PrometheusQuerier

	// Rules are the component-to-query mappings.
	Rules []ComponentRule

	// Interval is how often the automation loop evaluates rules.
	// Defaults to 60s if zero.
	Interval time.Duration

	// Logger is optional; defaults to slog.Default().
	Logger *slog.Logger
}

// Service runs the Prometheus-to-Statuspage.io automation loop.
type Service struct {
	provider provider.Provider
	querier  PrometheusQuerier
	rules    []ComponentRule
	interval time.Duration
	logger   *slog.Logger

	mu    sync.Mutex
	cache map[string]provider.ComponentStatus // componentID -> last-pushed status

	cancel context.CancelFunc
	done   chan struct{}
}

// NewService constructs an automation Service. Call Start to begin the loop.
func NewService(cfg Config) (*Service, error) {
	if cfg.Provider == nil {
		return nil, fmt.Errorf("automation: Provider is required")
	}
	if cfg.Querier == nil {
		return nil, fmt.Errorf("automation: Querier is required")
	}
	if len(cfg.Rules) == 0 {
		return nil, fmt.Errorf("automation: at least one Rule is required")
	}
	interval := cfg.Interval
	if interval == 0 {
		interval = 60 * time.Second
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		provider: cfg.Provider,
		querier:  cfg.Querier,
		rules:    cfg.Rules,
		interval: interval,
		logger:   logger,
		cache:    make(map[string]provider.ComponentStatus),
		done:     make(chan struct{}),
	}, nil
}

// Start begins the background automation loop. It blocks until ctx is
// cancelled or Stop is called.
func (s *Service) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)
	defer close(s.done)

	// Run immediately on start, then on interval.
	s.evaluate(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.evaluate(ctx)
		}
	}
}

// Stop cancels the automation loop and waits for it to finish.
func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	<-s.done
}

// EvaluateOnce runs a single evaluation cycle synchronously. Useful for
// testing without starting the background loop.
func (s *Service) EvaluateOnce(ctx context.Context) map[string]provider.ComponentStatus {
	return s.evaluate(ctx)
}

// evaluate runs one cycle: query Prometheus, compute statuses, push changes.
func (s *Service) evaluate(ctx context.Context) map[string]provider.ComponentStatus {
	results := make(map[string]provider.ComponentStatus)

	for _, rule := range s.rules {
		if ctx.Err() != nil {
			return results
		}

		value, err := s.querier.Query(ctx, rule.PrometheusQuery)
		if err != nil {
			s.logger.Warn("prometheus query failed",
				"component", rule.ComponentName,
				"query", rule.PrometheusQuery,
				"error", err,
			)
			continue
		}

		newStatus := rule.Evaluate(value)
		if newStatus == "" {
			s.logger.Warn("no threshold matched",
				"component", rule.ComponentName,
				"value", value,
			)
			continue
		}

		results[rule.ComponentID] = newStatus

		// Only push if status changed.
		s.mu.Lock()
		prev := s.cache[rule.ComponentID]
		changed := prev != newStatus
		if changed {
			s.cache[rule.ComponentID] = newStatus
		}
		s.mu.Unlock()

		if !changed {
			continue
		}

		s.logger.Info("component status transition",
			"component", rule.ComponentName,
			"component_id", rule.ComponentID,
			"from", string(prev),
			"to", string(newStatus),
		)

		if rule.ComponentID == "" {
			// ComponentID not yet provisioned; skip API call.
			continue
		}

		if err := s.provider.UpdateComponentStatus(ctx, rule.ComponentID, newStatus); err != nil {
			s.logger.Error("failed to update statuspage component",
				"component", rule.ComponentName,
				"component_id", rule.ComponentID,
				"status", string(newStatus),
				"error", err,
			)
			// Roll back cache so we retry next cycle.
			s.mu.Lock()
			s.cache[rule.ComponentID] = prev
			s.mu.Unlock()
		}
	}
	return results
}

// CachedStatus returns the last-known status for a component. Returns empty
// string if not yet evaluated.
func (s *Service) CachedStatus(componentID string) provider.ComponentStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cache[componentID]
}
