package stripe

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/bluenviron/mediamtx/internal/cloud/metering"
)

// Reporter implements metering.UsageReporter by forwarding aggregates
// to Stripe's usage-record API via the Client interface.
type Reporter struct {
	cfg      Config
	client   Client
	accounts AccountResolver
	items    SubscriptionItemResolver
}

// compile-time check
var _ metering.UsageReporter = (*Reporter)(nil)

// NewReporter constructs a Reporter. Returns an error if Config is
// invalid (missing metric→price mappings).
func NewReporter(
	cfg Config,
	client Client,
	accounts AccountResolver,
	items SubscriptionItemResolver,
) (*Reporter, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, fmt.Errorf("stripe: Client required")
	}
	if accounts == nil {
		return nil, fmt.Errorf("stripe: AccountResolver required")
	}
	if items == nil {
		return nil, fmt.Errorf("stripe: SubscriptionItemResolver required")
	}
	return &Reporter{
		cfg:      cfg,
		client:   client,
		accounts: accounts,
		items:    items,
	}, nil
}

// ReportAggregate maps a metering.Aggregate to a Stripe usage record
// and forwards it to the Client.
//
// Idempotency key = prefix + tenant_id + metric + period_start_unix
// so re-running the aggregator for the same period is safe.
func (r *Reporter) ReportAggregate(ctx context.Context, agg metering.Aggregate) error {
	if strings.TrimSpace(agg.TenantID) == "" {
		return ErrMissingTenant
	}
	priceID, ok := r.cfg.MetricPriceMap[agg.Metric]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownMetric, agg.Metric)
	}

	connAcct, err := r.accounts.ResolveConnectedAccount(ctx, agg.TenantID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNoConnectedAccount, err)
	}

	subItemID, err := r.items.ResolveSubscriptionItem(ctx, connAcct, priceID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNoSubscriptionItem, err)
	}

	// Stripe usage records accept integer quantities. We ceil the
	// sum so partial units always round up (customer-friendly for
	// small values, prevents billing $0 for real usage).
	qty := int64(math.Ceil(agg.Sum))
	if qty <= 0 {
		// Zero or negative aggregates happen when there's no usage in
		// the period. Don't send a usage record — Stripe defaults to 0.
		return nil
	}

	prefix := r.cfg.IdempotencyKeyPrefix
	if prefix == "" {
		prefix = "kaivue-usage-"
	}
	idemKey := fmt.Sprintf("%s%s-%s-%d",
		prefix, agg.TenantID, agg.Metric, agg.PeriodStart.Unix())

	return r.client.CreateUsageRecord(ctx, UsageRecordParams{
		ConnectedAccountID: connAcct,
		SubscriptionItemID: subItemID,
		Quantity:           qty,
		Timestamp:          agg.PeriodEnd,
		IdempotencyKey:     idemKey,
		Action:             "set", // absolute for the period, not incremental
	})
}
