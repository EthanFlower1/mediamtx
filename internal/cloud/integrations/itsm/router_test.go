package itsm

import (
	"context"
	"testing"
	"time"
)

// mockProvider is a test double for Provider.
type mockProvider struct {
	providerType ProviderType
	sendFunc     func(ctx context.Context, alert Alert) (AlertResult, error)
	resolveFunc  func(ctx context.Context, dedupKey string) (AlertResult, error)
}

func (m *mockProvider) Type() ProviderType { return m.providerType }

func (m *mockProvider) SendAlert(ctx context.Context, alert Alert) (AlertResult, error) {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, alert)
	}
	return AlertResult{
		ProviderType: m.providerType,
		ExternalID:   "mock-ext-id",
		Status:       "success",
		Timestamp:    time.Now().UTC(),
	}, nil
}

func (m *mockProvider) ResolveAlert(ctx context.Context, dedupKey string) (AlertResult, error) {
	if m.resolveFunc != nil {
		return m.resolveFunc(ctx, dedupKey)
	}
	return AlertResult{Status: "success"}, nil
}

func (m *mockProvider) TestConnection(_ context.Context) error { return nil }

func TestRouter_BasicRouting(t *testing.T) {
	router := NewRouter()
	pd := &mockProvider{providerType: ProviderPagerDuty}
	router.RegisterProvider("pd-config-1", pd)

	router.SetRules([]RoutingRule{
		{
			RuleID:           "rule-1",
			TenantID:         "t1",
			Name:             "All critical to PagerDuty",
			ProviderConfigID: "pd-config-1",
			MinSeverity:      SeverityCritical,
			Enabled:          true,
			Priority:         1,
		},
	})

	// Critical alert should match.
	results, err := router.Route(context.Background(), Alert{
		Summary:  "Camera down",
		Source:   "nvr",
		Severity: SeverityCritical,
	})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ProviderType != ProviderPagerDuty {
		t.Errorf("expected pagerduty, got %s", results[0].ProviderType)
	}

	// Warning alert should NOT match (below critical threshold).
	results, err = router.Route(context.Background(), Alert{
		Summary:  "Low disk",
		Source:   "nvr",
		Severity: SeverityWarning,
	})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for warning, got %d", len(results))
	}
}

func TestRouter_MultipleProviders(t *testing.T) {
	router := NewRouter()
	pd := &mockProvider{providerType: ProviderPagerDuty}
	og := &mockProvider{providerType: ProviderOpsgenie}
	router.RegisterProvider("pd-1", pd)
	router.RegisterProvider("og-1", og)

	router.SetRules([]RoutingRule{
		{
			RuleID:           "r1",
			ProviderConfigID: "pd-1",
			MinSeverity:      SeverityCritical,
			Enabled:          true,
			Priority:         1,
		},
		{
			RuleID:           "r2",
			ProviderConfigID: "og-1",
			MinSeverity:      SeverityWarning,
			Enabled:          true,
			Priority:         2,
		},
	})

	// Critical alert should match both rules.
	results, err := router.Route(context.Background(), Alert{
		Summary:  "System failure",
		Source:   "nvr",
		Severity: SeverityCritical,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestRouter_ClassFiltering(t *testing.T) {
	router := NewRouter()
	pd := &mockProvider{providerType: ProviderPagerDuty}
	router.RegisterProvider("pd-1", pd)

	router.SetRules([]RoutingRule{
		{
			RuleID:           "r1",
			ProviderConfigID: "pd-1",
			AlertClasses:     []string{"camera_offline", "storage_full"},
			Enabled:          true,
			Priority:         1,
		},
	})

	// Matching class.
	results, _ := router.Route(context.Background(), Alert{
		Summary:  "cam down",
		Source:   "nvr",
		Severity: SeverityCritical,
		Class:    "camera_offline",
	})
	if len(results) != 1 {
		t.Errorf("expected 1 result for matching class, got %d", len(results))
	}

	// Non-matching class.
	results, _ = router.Route(context.Background(), Alert{
		Summary:  "network issue",
		Source:   "nvr",
		Severity: SeverityCritical,
		Class:    "network_error",
	})
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-matching class, got %d", len(results))
	}
}

func TestRouter_DisabledRule(t *testing.T) {
	router := NewRouter()
	pd := &mockProvider{providerType: ProviderPagerDuty}
	router.RegisterProvider("pd-1", pd)

	router.SetRules([]RoutingRule{
		{
			RuleID:           "r1",
			ProviderConfigID: "pd-1",
			Enabled:          false, // disabled
			Priority:         1,
		},
	})

	results, _ := router.Route(context.Background(), Alert{
		Summary:  "test",
		Source:   "nvr",
		Severity: SeverityCritical,
	})
	if len(results) != 0 {
		t.Errorf("expected 0 results for disabled rule, got %d", len(results))
	}
}

func TestRouter_DedupProviders(t *testing.T) {
	var callCount int
	router := NewRouter()
	pd := &mockProvider{
		providerType: ProviderPagerDuty,
		sendFunc: func(_ context.Context, _ Alert) (AlertResult, error) {
			callCount++
			return AlertResult{Status: "success", ProviderType: ProviderPagerDuty}, nil
		},
	}
	router.RegisterProvider("pd-1", pd)

	// Two rules pointing to the same provider.
	router.SetRules([]RoutingRule{
		{RuleID: "r1", ProviderConfigID: "pd-1", Enabled: true, Priority: 1},
		{RuleID: "r2", ProviderConfigID: "pd-1", Enabled: true, Priority: 2},
	})

	results, _ := router.Route(context.Background(), Alert{
		Summary:  "test",
		Source:   "nvr",
		Severity: SeverityCritical,
	})

	if callCount != 1 {
		t.Errorf("expected provider called once (dedup), got %d", callCount)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestRouter_MissingProvider(t *testing.T) {
	router := NewRouter()
	// No providers registered.
	router.SetRules([]RoutingRule{
		{RuleID: "r1", ProviderConfigID: "missing-id", Enabled: true, Priority: 1},
	})

	_, err := router.Route(context.Background(), Alert{
		Summary:  "test",
		Source:   "nvr",
		Severity: SeverityCritical,
	})
	if err == nil {
		t.Error("expected error for missing provider")
	}
}

func TestRouter_RemoveProvider(t *testing.T) {
	router := NewRouter()
	pd := &mockProvider{providerType: ProviderPagerDuty}
	router.RegisterProvider("pd-1", pd)
	router.RemoveProvider("pd-1")

	router.SetRules([]RoutingRule{
		{RuleID: "r1", ProviderConfigID: "pd-1", Enabled: true, Priority: 1},
	})

	_, err := router.Route(context.Background(), Alert{
		Summary:  "test",
		Source:   "nvr",
		Severity: SeverityCritical,
	})
	if err == nil {
		t.Error("expected error after provider removal")
	}
}

func TestSeverityMatches(t *testing.T) {
	tests := []struct {
		min    Severity
		alert  Severity
		expect bool
	}{
		{"", SeverityInfo, true},
		{"", SeverityCritical, true},
		{SeverityInfo, SeverityInfo, true},
		{SeverityInfo, SeverityCritical, true},
		{SeverityWarning, SeverityInfo, false},
		{SeverityCritical, SeverityError, false},
		{SeverityCritical, SeverityCritical, true},
		{SeverityError, SeverityError, true},
		{SeverityError, SeverityCritical, true},
	}

	for _, tt := range tests {
		name := string(tt.min) + "_" + string(tt.alert)
		if tt.min == "" {
			name = "empty_" + string(tt.alert)
		}
		t.Run(name, func(t *testing.T) {
			got := severityMatches(tt.min, tt.alert)
			if got != tt.expect {
				t.Errorf("severityMatches(%q, %q) = %v, want %v", tt.min, tt.alert, got, tt.expect)
			}
		})
	}
}
