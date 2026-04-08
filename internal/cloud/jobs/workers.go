package jobs

import (
	"context"
	"fmt"
	"log/slog"
)

// Wave 3 workers for the eight seeded kinds. Every worker is a thin
// stub that:
//
//  1. Type-asserts the payload (failing loudly on mismatch so tests
//     catch wiring bugs).
//  2. Logs a structured record with tenant_id + kind.
//  3. Returns nil — the real implementation ships under the ticket
//     referenced in the TODO comment.
//
// The contract every new worker MUST obey: read tenant id from the
// payload only, never from context or config. The runner has already
// verified the payload's tenant is known; cross-tenant state access
// from inside the worker is still a bug — use the tenant id to scope
// your DB queries.

// TenantWelcomeEmailWorker — TODO(KAI-371): call SendGrid.
type TenantWelcomeEmailWorker struct{ Log *slog.Logger }

func (*TenantWelcomeEmailWorker) Kind() Kind { return KindTenantWelcomeEmail }
func (w *TenantWelcomeEmailWorker) Work(ctx context.Context, job *Job) error {
	p, ok := job.Payload.(TenantWelcomeEmailPayload)
	if !ok {
		return fmt.Errorf("welcome_email: unexpected payload %T", job.Payload)
	}
	logFor(w.Log).InfoContext(ctx, "tenant.welcome_email (stub)",
		slog.String("tenant_id", p.Tenant),
		slog.String("admin_user", p.AdminUser),
	)
	return nil
}

// TenantBootstrapStripeWorker — TODO(KAI-361): create Connect account.
type TenantBootstrapStripeWorker struct{ Log *slog.Logger }

func (*TenantBootstrapStripeWorker) Kind() Kind { return KindTenantBootstrapStripe }
func (w *TenantBootstrapStripeWorker) Work(ctx context.Context, job *Job) error {
	p, ok := job.Payload.(TenantBootstrapStripePayload)
	if !ok {
		return fmt.Errorf("bootstrap_stripe: unexpected payload %T", job.Payload)
	}
	logFor(w.Log).InfoContext(ctx, "tenant.bootstrap_stripe (stub)",
		slog.String("tenant_id", p.Tenant),
		slog.String("billing_mode", p.BillingMode),
	)
	return nil
}

// TenantBootstrapZitadelWorker — TODO(KAI-223): create Zitadel org.
type TenantBootstrapZitadelWorker struct{ Log *slog.Logger }

func (*TenantBootstrapZitadelWorker) Kind() Kind { return KindTenantBootstrapZitadel }
func (w *TenantBootstrapZitadelWorker) Work(ctx context.Context, job *Job) error {
	p, ok := job.Payload.(TenantBootstrapZitadelPayload)
	if !ok {
		return fmt.Errorf("bootstrap_zitadel: unexpected payload %T", job.Payload)
	}
	logFor(w.Log).InfoContext(ctx, "tenant.bootstrap_zitadel (stub)",
		slog.String("tenant_id", p.Tenant),
		slog.String("org_name", p.OrgName),
	)
	return nil
}

// BulkPushConfigWorker — TODO(KAI-343): fan out config push.
type BulkPushConfigWorker struct{ Log *slog.Logger }

func (*BulkPushConfigWorker) Kind() Kind { return KindBulkPushConfig }
func (w *BulkPushConfigWorker) Work(ctx context.Context, job *Job) error {
	p, ok := job.Payload.(BulkPushConfigPayload)
	if !ok {
		return fmt.Errorf("bulk.push_config: unexpected payload %T", job.Payload)
	}
	logFor(w.Log).InfoContext(ctx, "bulk.push_config (stub)",
		slog.String("tenant_id", p.Tenant),
		slog.Int("customer_count", len(p.CustomerIDs)),
	)
	return nil
}

// CloudArchiveUploadTriggerWorker — TODO(KAI-258): upload segment.
type CloudArchiveUploadTriggerWorker struct{ Log *slog.Logger }

func (*CloudArchiveUploadTriggerWorker) Kind() Kind { return KindCloudArchiveUploadTrig }
func (w *CloudArchiveUploadTriggerWorker) Work(ctx context.Context, job *Job) error {
	p, ok := job.Payload.(CloudArchiveUploadTriggerPayload)
	if !ok {
		return fmt.Errorf("cloud_archive.upload_trigger: unexpected payload %T", job.Payload)
	}
	logFor(w.Log).InfoContext(ctx, "cloud_archive.upload_trigger (stub)",
		slog.String("tenant_id", p.Tenant),
		slog.String("segment_id", p.SegmentID),
	)
	return nil
}

// BillingMonthlyRollupWorker — TODO(KAI-363): compute rollup.
type BillingMonthlyRollupWorker struct{ Log *slog.Logger }

func (*BillingMonthlyRollupWorker) Kind() Kind { return KindBillingMonthlyRollup }
func (w *BillingMonthlyRollupWorker) Work(ctx context.Context, job *Job) error {
	p, ok := job.Payload.(BillingMonthlyRollupPayload)
	if !ok {
		return fmt.Errorf("billing.monthly_rollup: unexpected payload %T", job.Payload)
	}
	logFor(w.Log).InfoContext(ctx, "billing.monthly_rollup (stub)",
		slog.String("tenant_id", p.Tenant),
		slog.String("period", p.Period),
	)
	return nil
}

// AuditPartitionCreateNextWorker — TODO(KAI-218): pg_partman call.
type AuditPartitionCreateNextWorker struct{ Log *slog.Logger }

func (*AuditPartitionCreateNextWorker) Kind() Kind { return KindAuditPartitionCreateNext }
func (w *AuditPartitionCreateNextWorker) Work(ctx context.Context, job *Job) error {
	p, ok := job.Payload.(AuditPartitionCreateNextPayload)
	if !ok {
		return fmt.Errorf("audit.partition_create_next_month: unexpected payload %T", job.Payload)
	}
	logFor(w.Log).InfoContext(ctx, "audit.partition_create_next_month (stub)",
		slog.String("tenant_id", p.Tenant),
		slog.String("target_yym", p.TargetYYM),
	)
	return nil
}

// AuditDropExpiredWorker — TODO(KAI-218): retention drop.
type AuditDropExpiredWorker struct{ Log *slog.Logger }

func (*AuditDropExpiredWorker) Kind() Kind { return KindAuditDropExpired }
func (w *AuditDropExpiredWorker) Work(ctx context.Context, job *Job) error {
	p, ok := job.Payload.(AuditDropExpiredPayload)
	if !ok {
		return fmt.Errorf("audit.drop_expired_partitions: unexpected payload %T", job.Payload)
	}
	logFor(w.Log).InfoContext(ctx, "audit.drop_expired_partitions (stub)",
		slog.String("tenant_id", p.Tenant),
		slog.String("older_than_ym", p.OlderThanYM),
	)
	return nil
}

// logFor returns l if non-nil, else a discarding logger so workers can
// be constructed without wiring a logger in tests.
func logFor(l *slog.Logger) *slog.Logger {
	if l != nil {
		return l
	}
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// DefaultWorkers returns a fresh Worker slice covering all eight
// seeded kinds, sharing the supplied logger. Consumers can call
// MemoryEnqueuer.RegisterAll(jobs.DefaultWorkers(log)...).
func DefaultWorkers(log *slog.Logger) []Worker {
	return []Worker{
		&TenantWelcomeEmailWorker{Log: log},
		&TenantBootstrapStripeWorker{Log: log},
		&TenantBootstrapZitadelWorker{Log: log},
		&BulkPushConfigWorker{Log: log},
		&CloudArchiveUploadTriggerWorker{Log: log},
		&BillingMonthlyRollupWorker{Log: log},
		&AuditPartitionCreateNextWorker{Log: log},
		&AuditDropExpiredWorker{Log: log},
	}
}
