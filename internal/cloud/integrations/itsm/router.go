package itsm

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// severityRank maps severity to a numeric rank for comparison.
var severityRank = map[Severity]int{
	SeverityInfo:     0,
	SeverityWarning:  1,
	SeverityError:    2,
	SeverityCritical: 3,
}

// Router evaluates routing rules to determine which providers should receive
// a given alert, then dispatches the alert to all matched providers.
type Router struct {
	mu        sync.RWMutex
	rules     []RoutingRule
	providers map[string]Provider // keyed by ProviderConfigID
}

// NewRouter creates a new alert router.
func NewRouter() *Router {
	return &Router{
		providers: make(map[string]Provider),
	}
}

// RegisterProvider adds a provider keyed by its config ID.
func (r *Router) RegisterProvider(configID string, p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[configID] = p
}

// RemoveProvider unregisters a provider.
func (r *Router) RemoveProvider(configID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.providers, configID)
}

// SetRules replaces the current routing rules. Rules are sorted by priority
// (ascending) on set.
func (r *Router) SetRules(rules []RoutingRule) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sorted := make([]RoutingRule, len(rules))
	copy(sorted, rules)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})
	r.rules = sorted
}

// Route evaluates all enabled routing rules against the alert and dispatches
// to each matched provider. It returns results from all matched providers.
// If no rules match, an empty slice is returned (not an error).
func (r *Router) Route(ctx context.Context, alert Alert) ([]AlertResult, error) {
	r.mu.RLock()
	rules := r.rules
	providers := make(map[string]Provider, len(r.providers))
	for k, v := range r.providers {
		providers[k] = v
	}
	r.mu.RUnlock()

	matched := matchRules(rules, alert)
	if len(matched) == 0 {
		return nil, nil
	}

	// Deduplicate provider config IDs so we don't send the same alert twice
	// to the same provider.
	seen := make(map[string]bool)
	var results []AlertResult
	var firstErr error

	for _, rule := range matched {
		if seen[rule.ProviderConfigID] {
			continue
		}
		seen[rule.ProviderConfigID] = true

		p, ok := providers[rule.ProviderConfigID]
		if !ok {
			if firstErr == nil {
				firstErr = fmt.Errorf("%w: config_id=%s", ErrProviderNotFound, rule.ProviderConfigID)
			}
			continue
		}

		result, err := p.SendAlert(ctx, alert)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			results = append(results, AlertResult{
				ProviderType: p.Type(),
				Status:       "error",
				Message:      err.Error(),
			})
			continue
		}
		results = append(results, result)
	}

	return results, firstErr
}

// matchRules returns all enabled rules that match the given alert.
func matchRules(rules []RoutingRule, alert Alert) []RoutingRule {
	var matched []RoutingRule
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if !severityMatches(rule.MinSeverity, alert.Severity) {
			continue
		}
		if !classMatches(rule.AlertClasses, alert.Class) {
			continue
		}
		matched = append(matched, rule)
	}
	return matched
}

// severityMatches returns true if the alert severity meets or exceeds the
// minimum severity specified by the rule. An empty min severity matches all.
func severityMatches(minSeverity, alertSeverity Severity) bool {
	if minSeverity == "" {
		return true
	}
	minRank, ok := severityRank[minSeverity]
	if !ok {
		return true
	}
	alertRank, ok := severityRank[alertSeverity]
	if !ok {
		return false
	}
	return alertRank >= minRank
}

// classMatches returns true if the alert class matches one of the allowed
// classes or if the allowed list is empty (matches all).
func classMatches(allowedClasses []string, alertClass string) bool {
	if len(allowedClasses) == 0 {
		return true
	}
	for _, c := range allowedClasses {
		if c == alertClass {
			return true
		}
	}
	return false
}
