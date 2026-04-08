package pairing

import (
	"context"
	"log/slog"
	"time"
)

// SweeperConfig parameterises the background expiry sweeper.
type SweeperConfig struct {
	// Store is the pairing token store to sweep. Required.
	Store *Store
	// Interval is how often the sweeper runs. Defaults to 1 minute.
	Interval time.Duration
	// Logger is the slog logger. nil defaults to slog.Default().
	Logger *slog.Logger
	// Metrics is the counter set. nil = no-op.
	Metrics *Metrics
}

// RunSweeper runs the background expiry sweeper until ctx is cancelled. It
// periodically marks all pending tokens whose ExpiresAt has passed as
// 'expired'. This is a standalone goroutine function — callers should launch
// it with go RunSweeper(...).
//
// The River job system (KAI-234) is the preferred durable scheduler in the
// cloud path. For the on-prem Directory, which runs without River, a simple
// in-process ticker is sufficient because the sweeper only performs an
// idempotent UPDATE — missing a tick due to a restart is harmless (the Redeem
// WHERE expires_at > NOW() guard already enforces TTL at redemption time).
func RunSweeper(ctx context.Context, cfg SweeperConfig) {
	if cfg.Store == nil {
		return
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = 1 * time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := cfg.Store.MarkExpired(ctx)
			if err != nil {
				log.Error("pairing: sweeper: mark expired failed", "error", err)
				continue
			}
			if n > 0 {
				log.Info("pairing: sweeper: expired tokens swept", "count", n)
				if cfg.Metrics != nil {
					cfg.Metrics.Expired.Add(uint64(n))
				}
			}
		}
	}
}
