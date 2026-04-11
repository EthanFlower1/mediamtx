package comms

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// IDGen generates a random hex ID.
type IDGen func() string

func defaultIDGen() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// RouterConfig bundles dependencies for the Router.
type RouterConfig struct {
	IDGen IDGen
}

// Router manages routing rules and dispatches alerts to the appropriate
// platform senders.
type Router struct {
	mu      sync.RWMutex
	rules   map[string][]RoutingRule // tenantID -> rules
	senders map[string]Sender        // integrationID -> sender
	idGen   IDGen
}

// NewRouter creates a Router.
func NewRouter(cfg RouterConfig) *Router {
	idGen := cfg.IDGen
	if idGen == nil {
		idGen = defaultIDGen
	}
	return &Router{
		rules:   make(map[string][]RoutingRule),
		senders: make(map[string]Sender),
		idGen:   idGen,
	}
}

// RegisterSender registers a platform sender under the given integration ID.
func (r *Router) RegisterSender(integrationID string, s Sender) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.senders[integrationID] = s
}

// UpsertRule creates or updates a routing rule.
func (r *Router) UpsertRule(rule RoutingRule) (RoutingRule, error) {
	if rule.TenantID == "" {
		return RoutingRule{}, errors.New("comms: tenant_id is required")
	}
	if len(rule.EventTypes) == 0 {
		return RoutingRule{}, errors.New("comms: at least one event type is required")
	}
	if rule.IntegrationID == "" {
		return RoutingRule{}, errors.New("comms: integration_id is required")
	}

	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()

	if rule.RuleID == "" {
		rule.RuleID = r.idGen()
		rule.CreatedAt = now
	}
	rule.UpdatedAt = now

	// Replace existing rule with same ID, or append.
	rules := r.rules[rule.TenantID]
	found := false
	for i, existing := range rules {
		if existing.RuleID == rule.RuleID {
			rules[i] = rule
			found = true
			break
		}
	}
	if !found {
		rules = append(rules, rule)
	}
	r.rules[rule.TenantID] = rules
	return rule, nil
}

// DeleteRule removes a routing rule.
func (r *Router) DeleteRule(tenantID, ruleID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rules := r.rules[tenantID]
	for i, rule := range rules {
		if rule.RuleID == ruleID {
			r.rules[tenantID] = append(rules[:i], rules[i+1:]...)
			return nil
		}
	}
	return ErrRuleNotFound
}

// ListRules returns all routing rules for a tenant.
func (r *Router) ListRules(tenantID string) []RoutingRule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]RoutingRule, len(r.rules[tenantID]))
	copy(out, r.rules[tenantID])
	return out
}

// Dispatch sends an alert to all matching channels based on routing rules.
// It returns a result per matched channel; errors are captured per-channel
// rather than aborting the entire dispatch.
func (r *Router) Dispatch(ctx context.Context, alert Alert) []PostResult {
	r.mu.RLock()
	rules := r.rules[alert.TenantID]
	// Copy senders map ref under lock
	senders := make(map[string]Sender, len(r.senders))
	for k, v := range r.senders {
		senders[k] = v
	}
	r.mu.RUnlock()

	var results []PostResult
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if !matchesEvent(rule.EventTypes, alert.EventType) {
			continue
		}
		sender, ok := senders[rule.IntegrationID]
		if !ok {
			results = append(results, PostResult{
				Platform:   rule.Platform,
				ChannelRef: rule.ChannelRef,
				Err:        fmt.Errorf("%w: integration %s", ErrNotConfigured, rule.IntegrationID),
			})
			continue
		}
		res, err := sender.PostAlert(ctx, rule.ChannelRef, alert)
		res.Err = err
		results = append(results, res)
	}
	return results
}

// HandleAction delegates an interactive action to the appropriate sender.
func (r *Router) HandleAction(ctx context.Context, action CardAction) (ActionResult, error) {
	r.mu.RLock()
	sender, ok := r.senders[action.IntegrationID]
	r.mu.RUnlock()

	if !ok {
		return ActionResult{}, fmt.Errorf("%w: integration %s", ErrNotConfigured, action.IntegrationID)
	}
	return sender.HandleAction(ctx, action)
}

// matchesEvent checks if an event type matches any of the rule's patterns.
func matchesEvent(patterns []string, eventType string) bool {
	for _, p := range patterns {
		if p == "*" || p == eventType {
			return true
		}
	}
	return false
}
