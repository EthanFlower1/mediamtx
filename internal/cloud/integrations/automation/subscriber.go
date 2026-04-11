package automation

import (
	"sync"
)

// Platform identifies which automation service owns a subscription.
type Platform string

const (
	PlatformZapier Platform = "zapier"
	PlatformMake   Platform = "make"
	PlatformN8N    Platform = "n8n"
)

// Subscription represents a webhook subscription from an automation platform.
type Subscription struct {
	ID          string      `json:"id"`
	Platform    Platform    `json:"platform"`
	TriggerKey  string      `json:"trigger_key"`
	WebhookURL  string      `json:"webhook_url"`
	Secret      string      `json:"secret,omitempty"`
	Active      bool        `json:"active"`
	Metadata    Metadata    `json:"metadata,omitempty"`
}

// SubscriptionStore is a thread-safe in-memory store for webhook
// subscriptions. A production deployment would persist these to the database.
type SubscriptionStore struct {
	mu   sync.RWMutex
	subs map[string]*Subscription
}

// NewSubscriptionStore returns an initialised store.
func NewSubscriptionStore() *SubscriptionStore {
	return &SubscriptionStore{subs: make(map[string]*Subscription)}
}

// Add inserts or replaces a subscription.
func (s *SubscriptionStore) Add(sub *Subscription) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs[sub.ID] = sub
}

// Remove deletes a subscription by ID. It returns true if the subscription
// existed.
func (s *SubscriptionStore) Remove(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.subs[id]
	delete(s.subs, id)
	return ok
}

// Get returns a subscription by ID, or nil.
func (s *SubscriptionStore) Get(id string) *Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.subs[id]
}

// ByTrigger returns all active subscriptions for the given trigger key.
func (s *SubscriptionStore) ByTrigger(triggerKey string) []*Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Subscription
	for _, sub := range s.subs {
		if sub.TriggerKey == triggerKey && sub.Active {
			out = append(out, sub)
		}
	}
	return out
}

// ByPlatform returns all subscriptions for the given platform.
func (s *SubscriptionStore) ByPlatform(p Platform) []*Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Subscription
	for _, sub := range s.subs {
		if sub.Platform == p {
			out = append(out, sub)
		}
	}
	return out
}

// All returns every subscription in the store.
func (s *SubscriptionStore) All() []*Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Subscription, 0, len(s.subs))
	for _, sub := range s.subs {
		out = append(out, sub)
	}
	return out
}
