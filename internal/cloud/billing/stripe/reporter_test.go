package stripe_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/billing/stripe"
	"github.com/bluenviron/mediamtx/internal/cloud/metering"
)

// ---------- fakes ----------

type fakeClient struct {
	calls []stripe.UsageRecordParams
	err   error
}

func (f *fakeClient) CreateUsageRecord(_ context.Context, p stripe.UsageRecordParams) error {
	f.calls = append(f.calls, p)
	return f.err
}

type fakeAccounts struct {
	mapping map[string]string
}

func (f *fakeAccounts) ResolveConnectedAccount(_ context.Context, tenantID string) (string, error) {
	acct, ok := f.mapping[tenantID]
	if !ok {
		return "", errors.New("not found")
	}
	return acct, nil
}

type fakeItems struct {
	mapping map[string]string // key = "acct:price"
}

func (f *fakeItems) ResolveSubscriptionItem(_ context.Context, acct, price string) (string, error) {
	item, ok := f.mapping[acct+":"+price]
	if !ok {
		return "", errors.New("not found")
	}
	return item, nil
}

func defaultConfig() stripe.Config {
	return stripe.Config{
		MetricPriceMap: map[metering.Metric]string{
			metering.MetricCameraHours:      "price_cam",
			metering.MetricRecordingBytes:    "price_rec",
			metering.MetricAIInferenceCount:  "price_ai",
		},
		IdempotencyKeyPrefix: "test-",
	}
}

func defaultFakes() (*fakeClient, *fakeAccounts, *fakeItems) {
	return &fakeClient{},
		&fakeAccounts{mapping: map[string]string{
			"tenant-a": "acct_aaaa",
			"tenant-b": "acct_bbbb",
		}},
		&fakeItems{mapping: map[string]string{
			"acct_aaaa:price_cam": "si_cam_a",
			"acct_aaaa:price_rec": "si_rec_a",
			"acct_aaaa:price_ai":  "si_ai_a",
			"acct_bbbb:price_cam": "si_cam_b",
		}}
}

func newReporter(t *testing.T) (*stripe.Reporter, *fakeClient) {
	t.Helper()
	client, accounts, items := defaultFakes()
	r, err := stripe.NewReporter(defaultConfig(), client, accounts, items)
	if err != nil {
		t.Fatalf("new reporter: %v", err)
	}
	return r, client
}

// ---------- tests ----------

func TestReporter_ReportAggregate_Happy(t *testing.T) {
	r, client := newReporter(t)
	ctx := context.Background()
	now := time.Now().UTC()

	agg := metering.Aggregate{
		TenantID:    "tenant-a",
		PeriodStart: now.Add(-24 * time.Hour),
		PeriodEnd:   now,
		Metric:      metering.MetricCameraHours,
		Sum:         42.7,
		Max:         10.0,
	}
	if err := r.ReportAggregate(ctx, agg); err != nil {
		t.Fatalf("report: %v", err)
	}
	if len(client.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(client.calls))
	}
	call := client.calls[0]
	if call.ConnectedAccountID != "acct_aaaa" {
		t.Errorf("acct = %q, want acct_aaaa", call.ConnectedAccountID)
	}
	if call.SubscriptionItemID != "si_cam_a" {
		t.Errorf("sub item = %q, want si_cam_a", call.SubscriptionItemID)
	}
	// 42.7 ceil = 43
	if call.Quantity != 43 {
		t.Errorf("qty = %d, want 43 (ceil of 42.7)", call.Quantity)
	}
	if call.Action != "set" {
		t.Errorf("action = %q, want set", call.Action)
	}
}

// TestReporter_TenantIsolation_Seam4 proves that a different tenant's
// account and subscription items are resolved correctly — tenant-a's
// usage never lands on tenant-b's Stripe account.
func TestReporter_TenantIsolation_Seam4(t *testing.T) {
	r, client := newReporter(t)
	ctx := context.Background()
	now := time.Now().UTC()

	aggA := metering.Aggregate{
		TenantID:    "tenant-a",
		PeriodStart: now.Add(-24 * time.Hour),
		PeriodEnd:   now,
		Metric:      metering.MetricCameraHours,
		Sum:         10,
	}
	aggB := metering.Aggregate{
		TenantID:    "tenant-b",
		PeriodStart: now.Add(-24 * time.Hour),
		PeriodEnd:   now,
		Metric:      metering.MetricCameraHours,
		Sum:         20,
	}
	if err := r.ReportAggregate(ctx, aggA); err != nil {
		t.Fatalf("report A: %v", err)
	}
	if err := r.ReportAggregate(ctx, aggB); err != nil {
		t.Fatalf("report B: %v", err)
	}
	if len(client.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(client.calls))
	}
	if client.calls[0].ConnectedAccountID != "acct_aaaa" {
		t.Errorf("call[0] acct = %q, want acct_aaaa", client.calls[0].ConnectedAccountID)
	}
	if client.calls[1].ConnectedAccountID != "acct_bbbb" {
		t.Errorf("call[1] acct = %q, want acct_bbbb", client.calls[1].ConnectedAccountID)
	}
	if client.calls[0].SubscriptionItemID != "si_cam_a" {
		t.Errorf("call[0] si = %q, want si_cam_a", client.calls[0].SubscriptionItemID)
	}
	if client.calls[1].SubscriptionItemID != "si_cam_b" {
		t.Errorf("call[1] si = %q, want si_cam_b", client.calls[1].SubscriptionItemID)
	}
}

func TestReporter_ZeroAggregateSkipped(t *testing.T) {
	r, client := newReporter(t)
	ctx := context.Background()

	agg := metering.Aggregate{
		TenantID:    "tenant-a",
		PeriodStart: time.Now().UTC().Add(-24 * time.Hour),
		PeriodEnd:   time.Now().UTC(),
		Metric:      metering.MetricCameraHours,
		Sum:         0,
	}
	if err := r.ReportAggregate(ctx, agg); err != nil {
		t.Fatalf("report: %v", err)
	}
	if len(client.calls) != 0 {
		t.Errorf("zero aggregate should not produce a usage record, got %d calls", len(client.calls))
	}
}

func TestReporter_MissingTenantRejected(t *testing.T) {
	r, _ := newReporter(t)
	err := r.ReportAggregate(context.Background(), metering.Aggregate{
		Metric: metering.MetricCameraHours,
		Sum:    1,
	})
	if !errors.Is(err, stripe.ErrMissingTenant) {
		t.Errorf("err = %v, want ErrMissingTenant", err)
	}
}

func TestReporter_UnknownTenantReturnsError(t *testing.T) {
	r, _ := newReporter(t)
	err := r.ReportAggregate(context.Background(), metering.Aggregate{
		TenantID: "ghost-tenant",
		Metric:   metering.MetricCameraHours,
		Sum:      1,
	})
	if err == nil {
		t.Fatal("expected error for unknown tenant")
	}
	if !errors.Is(err, stripe.ErrNoConnectedAccount) {
		t.Errorf("err = %v, want ErrNoConnectedAccount", err)
	}
}

func TestReporter_ClientErrorPropagated(t *testing.T) {
	client, accounts, items := defaultFakes()
	client.err = errors.New("stripe 429")
	r, err := stripe.NewReporter(defaultConfig(), client, accounts, items)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	err = r.ReportAggregate(context.Background(), metering.Aggregate{
		TenantID: "tenant-a",
		Metric:   metering.MetricCameraHours,
		Sum:      5,
	})
	if err == nil {
		t.Fatal("expected client error to propagate")
	}
}

func TestReporter_IdempotencyKeyStable(t *testing.T) {
	r, client := newReporter(t)
	ctx := context.Background()

	periodStart := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	agg := metering.Aggregate{
		TenantID:    "tenant-a",
		PeriodStart: periodStart,
		PeriodEnd:   periodStart.Add(24 * time.Hour),
		Metric:      metering.MetricCameraHours,
		Sum:         10,
	}

	// Report twice — idempotency key must be identical.
	_ = r.ReportAggregate(ctx, agg)
	_ = r.ReportAggregate(ctx, agg)
	if len(client.calls) != 2 {
		t.Fatalf("calls = %d", len(client.calls))
	}
	if client.calls[0].IdempotencyKey != client.calls[1].IdempotencyKey {
		t.Errorf("idempotency keys differ: %q vs %q",
			client.calls[0].IdempotencyKey, client.calls[1].IdempotencyKey)
	}
}

func TestConfig_Validate_MissingMetric(t *testing.T) {
	cfg := stripe.Config{
		MetricPriceMap: map[metering.Metric]string{
			metering.MetricCameraHours: "price_cam",
			// Missing recording_bytes and ai_inference_count
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for incomplete MetricPriceMap")
	}
}
