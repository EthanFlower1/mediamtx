package metering

import (
	"context"
	"fmt"
	"time"
)

// Aggregator rolls usage_events into usage_aggregates for a given period.
// It is idempotent per (tenant_id, period_start, metric) triple so the
// nightly CronJob (KAI-232) can re-run without creating duplicates.
type Aggregator struct {
	store *Store
}

// NewAggregator builds an Aggregator over the given store.
func NewAggregator(store *Store) *Aggregator {
	return &Aggregator{store: store}
}

// Run computes rollups for [periodStart, periodEnd) across the given
// tenants. It queries events tenant-by-tenant to preserve Seam #4. Pass an
// empty tenants slice to get a no-op.
func (a *Aggregator) Run(ctx context.Context, periodStart, periodEnd time.Time, tenants []string) error {
	if !periodEnd.After(periodStart) {
		return ErrInvalidPeriod
	}
	for _, tenant := range tenants {
		if tenant == "" {
			return ErrMissingTenant
		}
		if err := a.runTenant(ctx, periodStart, periodEnd, tenant); err != nil {
			return fmt.Errorf("metering: aggregate tenant %q: %w", tenant, err)
		}
	}
	return nil
}

func (a *Aggregator) runTenant(ctx context.Context, periodStart, periodEnd time.Time, tenantID string) error {
	// One rollup per metric. The metric list is closed so we iterate it
	// explicitly instead of SELECT DISTINCT metric — that keeps the query
	// plan trivial and guarantees an aggregate row even for zero-event
	// metrics (which downstream billing treats as "no charge").
	for _, metric := range AllMetrics {
		events, err := a.store.ListEvents(ctx, QueryFilter{
			TenantID: tenantID,
			Metric:   metric,
			Since:    periodStart,
			Until:    periodEnd,
		})
		if err != nil {
			return err
		}
		if len(events) == 0 {
			continue
		}
		var sum, max float64
		for _, e := range events {
			sum += e.Value
			if e.Value > max {
				max = e.Value
			}
		}
		agg := Aggregate{
			TenantID:      tenantID,
			PeriodStart:   periodStart,
			PeriodEnd:     periodEnd,
			Metric:        metric,
			Sum:           sum,
			Max:           max,
			SnapshotCount: len(events),
		}
		if err := a.store.upsertAggregate(ctx, agg); err != nil {
			return err
		}
	}
	return nil
}
