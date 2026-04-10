package stripe

import (
	"context"
	"errors"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/metering"
)

// Sentinel errors.
var (
	ErrMissingTenant       = errors.New("stripe: tenant_id required")
	ErrNoConnectedAccount  = errors.New("stripe: tenant has no Stripe connected account")
	ErrNoSubscriptionItem  = errors.New("stripe: no subscription item found for metric")
	ErrUnknownMetric       = errors.New("stripe: metric has no Stripe price mapping")
	ErrClientError         = errors.New("stripe: API call failed")
)

// Config is injected at startup.
type Config struct {
	// MetricPriceMap maps each metering.Metric to a Stripe price id.
	// Comes from the plan catalog (KAI-363). Every Metric in
	// metering.AllMetrics MUST be present.
	MetricPriceMap map[metering.Metric]string

	// IdempotencyKeyPrefix is prepended to the usage-record
	// idempotency key. Defaults to "kaivue-usage-" if empty.
	IdempotencyKeyPrefix string
}

// Validate returns an error if a required metric is unmapped.
func (c Config) Validate() error {
	for _, m := range metering.AllMetrics {
		if _, ok := c.MetricPriceMap[m]; !ok {
			return errors.New("stripe: Config.MetricPriceMap missing mapping for " + string(m))
		}
	}
	return nil
}

// AccountResolver maps a Kaivue tenant_id to a Stripe connected
// account id. The production implementation queries the tenants table
// where the connected_account_id column lives (KAI-363 added it).
type AccountResolver interface {
	ResolveConnectedAccount(ctx context.Context, tenantID string) (string, error)
}

// SubscriptionItemResolver maps a (connected account, price id) to a
// Stripe subscription item id. In the Stripe Connect model the
// platform creates a subscription for the connected account; each
// metered price on that subscription has a subscription_item_id that
// the usage-record API requires.
type SubscriptionItemResolver interface {
	ResolveSubscriptionItem(ctx context.Context, connectedAccountID, priceID string) (string, error)
}

// Client abstracts the Stripe HTTP surface. The production adapter
// calls the official Go SDK; tests use a capturing fake.
type Client interface {
	// CreateUsageRecord reports usage to Stripe for a specific
	// subscription item on a connected account. The timestamp is
	// the period end, quantity is the integer portion of the
	// aggregate sum (Stripe usage records are integers), and the
	// idempotencyKey prevents duplicate billing on retry.
	CreateUsageRecord(ctx context.Context, params UsageRecordParams) error
}

// UsageRecordParams is the input to Client.CreateUsageRecord.
type UsageRecordParams struct {
	ConnectedAccountID string
	SubscriptionItemID string
	Quantity           int64
	Timestamp          time.Time
	IdempotencyKey     string
	Action             string // "set" or "increment"
}
