package itsm

import (
	"context"
	"testing"
	"time"
)

func newTestService(t *testing.T) (*Service, *MemStore) {
	t.Helper()
	store := NewMemStore()
	seq := 0
	svc, err := NewService(Config{
		Store: store,
		IDGen: func() string {
			seq++
			return "id-" + string(rune('0'+seq))
		},
		Factory: func(cfg ProviderConfig) (Provider, error) {
			return &mockProvider{providerType: cfg.Provider}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, store
}

func TestService_NilStore(t *testing.T) {
	_, err := NewService(Config{})
	if err == nil {
		t.Error("expected error for nil store")
	}
}

func TestService_SaveAndListProviderConfigs(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	cfg := ProviderConfig{
		TenantID: "t1",
		Provider: ProviderPagerDuty,
		APIKey:   "test-key",
		Enabled:  true,
	}

	saved, err := svc.SaveProviderConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("SaveProviderConfig: %v", err)
	}
	if saved.ConfigID == "" {
		t.Error("expected generated config ID")
	}
	if saved.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}

	list, err := svc.ListProviderConfigs(ctx, "t1")
	if err != nil {
		t.Fatalf("ListProviderConfigs: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 config, got %d", len(list))
	}
}

func TestService_GetProviderConfig(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	saved, _ := svc.SaveProviderConfig(ctx, ProviderConfig{
		TenantID: "t1",
		Provider: ProviderOpsgenie,
		APIKey:   "og-key",
		Enabled:  true,
	})

	got, err := svc.GetProviderConfig(ctx, "t1", saved.ConfigID)
	if err != nil {
		t.Fatalf("GetProviderConfig: %v", err)
	}
	if got.Provider != ProviderOpsgenie {
		t.Errorf("expected opsgenie, got %s", got.Provider)
	}
}

func TestService_DeleteProviderConfig(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	saved, _ := svc.SaveProviderConfig(ctx, ProviderConfig{
		TenantID: "t1",
		Provider: ProviderPagerDuty,
		APIKey:   "key",
		Enabled:  true,
	})

	err := svc.DeleteProviderConfig(ctx, "t1", saved.ConfigID)
	if err != nil {
		t.Fatalf("DeleteProviderConfig: %v", err)
	}

	list, _ := svc.ListProviderConfigs(ctx, "t1")
	if len(list) != 0 {
		t.Errorf("expected 0 configs after delete, got %d", len(list))
	}
}

func TestService_SaveAndListRoutingRules(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	rule := RoutingRule{
		TenantID:         "t1",
		Name:             "Critical to PD",
		ProviderConfigID: "cfg-1",
		MinSeverity:      SeverityCritical,
		AlertClasses:     []string{"camera_offline"},
		Priority:         1,
		Enabled:          true,
	}

	saved, err := svc.SaveRoutingRule(ctx, rule)
	if err != nil {
		t.Fatalf("SaveRoutingRule: %v", err)
	}
	if saved.RuleID == "" {
		t.Error("expected generated rule ID")
	}

	list, err := svc.ListRoutingRules(ctx, "t1")
	if err != nil {
		t.Fatalf("ListRoutingRules: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(list))
	}
	if list[0].Name != "Critical to PD" {
		t.Errorf("expected rule name 'Critical to PD', got %s", list[0].Name)
	}
}

func TestService_DeleteRoutingRule(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	saved, _ := svc.SaveRoutingRule(ctx, RoutingRule{
		TenantID:         "t1",
		Name:             "test",
		ProviderConfigID: "cfg-1",
		Enabled:          true,
	})

	err := svc.DeleteRoutingRule(ctx, "t1", saved.RuleID)
	if err != nil {
		t.Fatal(err)
	}

	list, _ := svc.ListRoutingRules(ctx, "t1")
	if len(list) != 0 {
		t.Errorf("expected 0 rules after delete, got %d", len(list))
	}
}

func TestService_SendAlertEndToEnd(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Save a provider config and a routing rule.
	pdCfg, _ := svc.SaveProviderConfig(ctx, ProviderConfig{
		TenantID: "t1",
		Provider: ProviderPagerDuty,
		APIKey:   "pd-key",
		Enabled:  true,
	})

	_, _ = svc.SaveRoutingRule(ctx, RoutingRule{
		TenantID:         "t1",
		Name:             "All errors to PD",
		ProviderConfigID: pdCfg.ConfigID,
		MinSeverity:      SeverityError,
		Enabled:          true,
		Priority:         1,
	})

	// Load routing into the router.
	if err := svc.LoadTenantRouting(ctx, "t1"); err != nil {
		t.Fatalf("LoadTenantRouting: %v", err)
	}

	// Send a critical alert.
	results, err := svc.SendAlert(ctx, Alert{
		Summary:   "Camera offline: lobby",
		Source:    "mediamtx-nvr",
		Severity:  SeverityCritical,
		DedupKey:  "cam-lobby-offline",
		Timestamp: time.Now().UTC(),
		Class:     "camera_offline",
	})
	if err != nil {
		t.Fatalf("SendAlert: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "success" {
		t.Errorf("expected success, got %s", results[0].Status)
	}

	// Send an info alert (below threshold, should not match).
	results, err = svc.SendAlert(ctx, Alert{
		Summary:  "System check passed",
		Source:   "mediamtx-nvr",
		Severity: SeverityInfo,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for info alert, got %d", len(results))
	}
}

func TestService_TestProvider(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	saved, _ := svc.SaveProviderConfig(ctx, ProviderConfig{
		TenantID: "t1",
		Provider: ProviderPagerDuty,
		APIKey:   "key",
		Enabled:  true,
	})

	err := svc.TestProvider(ctx, "t1", saved.ConfigID)
	if err != nil {
		t.Fatalf("TestProvider: %v", err)
	}
}

func TestService_LoadTenantRouting_DisabledProvider(t *testing.T) {
	svc, store := newTestService(t)
	ctx := context.Background()

	// Directly insert a disabled config.
	_ = store.UpsertProviderConfig(ctx, ProviderConfig{
		ConfigID: "disabled-cfg",
		TenantID: "t1",
		Provider: ProviderPagerDuty,
		APIKey:   "key",
		Enabled:  false,
	})

	// Should not error, just skip.
	err := svc.LoadTenantRouting(ctx, "t1")
	if err != nil {
		t.Fatalf("LoadTenantRouting: %v", err)
	}
}

func TestService_DefaultProviderFactory_PagerDuty(t *testing.T) {
	p, err := DefaultProviderFactory(ProviderConfig{
		Provider: ProviderPagerDuty,
		APIKey:   "test-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Type() != ProviderPagerDuty {
		t.Errorf("expected pagerduty, got %s", p.Type())
	}
}

func TestService_DefaultProviderFactory_Opsgenie(t *testing.T) {
	p, err := DefaultProviderFactory(ProviderConfig{
		Provider: ProviderOpsgenie,
		APIKey:   "test-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Type() != ProviderOpsgenie {
		t.Errorf("expected opsgenie, got %s", p.Type())
	}
}

func TestService_DefaultProviderFactory_Unknown(t *testing.T) {
	_, err := DefaultProviderFactory(ProviderConfig{
		Provider: "unknown",
		APIKey:   "key",
	})
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}
