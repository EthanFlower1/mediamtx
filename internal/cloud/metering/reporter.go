package metering

import "context"

// UsageReporter is the seam between this package and the billing /
// invoicing layer. KAI-361 (Stripe Connect) will implement this interface
// in internal/cloud/billing/stripe/ and wire it to Stripe's usage-record
// API. metering never imports Stripe — the adapter depends on this
// package, not the other way around.
type UsageReporter interface {
	ReportAggregate(ctx context.Context, agg Aggregate) error
}

// FanoutReporter pushes each aggregate to one or more UsageReporters.
// It is useful in tests (capture into a slice) and in production when
// both Stripe AND the customer billing portal (KAI-367) need the same
// rollup stream. Errors are propagated immediately on the first failing
// reporter so callers retry the whole batch on the next run.
type FanoutReporter struct {
	targets []UsageReporter
}

// NewFanoutReporter builds a FanoutReporter over the given targets.
func NewFanoutReporter(targets ...UsageReporter) *FanoutReporter {
	return &FanoutReporter{targets: targets}
}

// ReportAggregate fans an aggregate out to every target.
func (f *FanoutReporter) ReportAggregate(ctx context.Context, agg Aggregate) error {
	for _, t := range f.targets {
		if err := t.ReportAggregate(ctx, agg); err != nil {
			return err
		}
	}
	return nil
}

// ReportPeriod reads aggregates for a tenant+period from a Store and
// forwards every row to the reporter. This is the callable the KAI-232
// nightly job will invoke after Aggregator.Run completes.
func ReportPeriod(ctx context.Context, store *Store, reporter UsageReporter, f QueryFilter) error {
	aggs, err := store.ListAggregates(ctx, f)
	if err != nil {
		return err
	}
	for _, agg := range aggs {
		if err := reporter.ReportAggregate(ctx, agg); err != nil {
			return err
		}
	}
	return nil
}
