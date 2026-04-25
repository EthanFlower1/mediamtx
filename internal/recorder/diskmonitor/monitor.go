// Package diskmonitor polls the recordings disk for capacity and
// enforces retention by deleting expired recordings. This is a
// hardening layer for the bedrock recording invariant: the recorder
// MUST not run out of disk space silently.
package diskmonitor

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

const (
	defaultInterval                   = 60 * time.Second
	defaultRetentionThresholdPercent  = 90.0
	defaultRetentionHysteresisPct     = 5.0
	retentionBatchSize                = 50
	warnThresholdPercent              = 80.0
	errorThresholdPercent             = 95.0
)

// Database is the subset of the recordings DB consumed by the Monitor.
type Database interface {
	// ListExpiredRecordings returns recordings past their retention
	// policy, oldest first. Limit caps the result size.
	ListExpiredRecordings(ctx context.Context, limit int) ([]ExpiredRecording, error)
	// DeleteRecording removes the recording from the DB. Caller is
	// responsible for unlinking the file.
	DeleteRecording(ctx context.Context, recordingID int64) error
}

// ExpiredRecording holds the minimal data the Monitor needs to delete a
// recording that is past its retention policy.
type ExpiredRecording struct {
	ID       int64
	FilePath string
	FileSize int64
}

// Stats holds the most recent disk metrics observed by the Monitor.
type Stats struct {
	UsedBytes     int64
	CapacityBytes int64
	UsedPercent   float64
	LastPolled    time.Time
}

// Config holds all parameters for constructing a Monitor.
type Config struct {
	// RecordingsPath is the directory whose disk usage is monitored.
	RecordingsPath string
	// DB queries the recordings table for retention candidates.
	DB Database
	// Interval between poll cycles. Defaults to 60s.
	Interval time.Duration
	// RetentionThresholdPercent triggers retention deletion when
	// disk usage exceeds this percent. Defaults to 90.
	RetentionThresholdPercent float64
	// Logger receives structured logs.
	Logger *slog.Logger
}

func (c *Config) interval() time.Duration {
	if c.Interval > 0 {
		return c.Interval
	}
	return defaultInterval
}

func (c *Config) retentionThreshold() float64 {
	if c.RetentionThresholdPercent > 0 {
		return c.RetentionThresholdPercent
	}
	return defaultRetentionThresholdPercent
}

func (c *Config) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// Monitor polls the recordings disk and enforces retention when the
// disk exceeds the configured threshold.
type Monitor struct {
	cfg    Config
	statfs func(path string) (used, capacity int64, err error)
	stats  atomic.Pointer[Stats]
	cycle  <-chan time.Time
	ticker *time.Ticker
}

// New constructs a Monitor. Panics on nil DB.
func New(cfg Config) *Monitor {
	if cfg.DB == nil {
		panic("diskmonitor: Config.DB must not be nil")
	}
	ticker := time.NewTicker(cfg.interval())
	m := &Monitor{
		cfg:    cfg,
		statfs: realStatfs,
		ticker: ticker,
		cycle:  ticker.C,
	}
	// Initialise stats so Stats() never returns a nil-pointer dereference.
	initial := &Stats{}
	m.stats.Store(initial)
	return m
}

// Run blocks until ctx is done. Polls + enforces retention each cycle.
func (m *Monitor) Run(ctx context.Context) {
	if m.ticker != nil {
		defer m.ticker.Stop()
	}

	log := m.cfg.logger()

	// Perform an immediate first poll before waiting for the first tick.
	m.poll(ctx, log)

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.cycle:
			m.poll(ctx, log)
		}
	}
}

// Stats returns the most recent disk metrics (safe to call from any goroutine).
func (m *Monitor) Stats() Stats {
	if p := m.stats.Load(); p != nil {
		return *p
	}
	return Stats{}
}

// poll performs one disk-check + optional retention cycle.
func (m *Monitor) poll(ctx context.Context, log *slog.Logger) {
	path := m.cfg.RecordingsPath
	if path == "" {
		path = "."
	}

	usedBytes, capacityBytes, err := m.statfs(path)
	if err != nil {
		log.Error("diskmonitor: statfs error",
			slog.String("path", path),
			slog.String("error", err.Error()))
		return
	}

	var usedPct float64
	if capacityBytes > 0 {
		usedPct = float64(usedBytes) / float64(capacityBytes) * 100.0
	}

	now := time.Now()
	current := &Stats{
		UsedBytes:     usedBytes,
		CapacityBytes: capacityBytes,
		UsedPercent:   usedPct,
		LastPolled:    now,
	}
	m.stats.Store(current)

	// Structured log at appropriate level.
	switch {
	case usedPct >= errorThresholdPercent:
		log.Error("diskmonitor: disk usage critical",
			slog.String("path", path),
			slog.Float64("used_pct", usedPct),
			slog.Int64("used_bytes", usedBytes),
			slog.Int64("capacity_bytes", capacityBytes))
	case usedPct >= warnThresholdPercent:
		log.Warn("diskmonitor: disk usage high",
			slog.String("path", path),
			slog.Float64("used_pct", usedPct),
			slog.Int64("used_bytes", usedBytes),
			slog.Int64("capacity_bytes", capacityBytes))
	default:
		log.Info("diskmonitor: disk usage ok",
			slog.String("path", path),
			slog.Float64("used_pct", usedPct),
			slog.Int64("used_bytes", usedBytes),
			slog.Int64("capacity_bytes", capacityBytes))
	}

	// Enforce retention if above threshold.
	threshold := m.cfg.retentionThreshold()
	if usedPct >= threshold {
		m.enforceRetention(ctx, log, path, threshold)
	}
}

// enforceRetention deletes expired recordings oldest-first until disk
// usage drops below (threshold - hysteresis).
func (m *Monitor) enforceRetention(ctx context.Context, log *slog.Logger, path string, threshold float64) {
	stopBelow := threshold - defaultRetentionHysteresisPct

	for {
		// Re-read current usage before each batch.
		usedBytes, cap, err := m.statfs(path)
		if err != nil {
			log.Error("diskmonitor: statfs error during retention",
				slog.String("path", path),
				slog.String("error", err.Error()))
			return
		}
		_ = cap // capacityBytes already obtained; cap may differ but we use the live one.

		var currentPct float64
		if cap > 0 {
			currentPct = float64(usedBytes) / float64(cap) * 100.0
		}

		if currentPct < stopBelow {
			log.Info("diskmonitor: disk below hysteresis target; stopping retention",
				slog.Float64("used_pct", currentPct),
				slog.Float64("stop_below_pct", stopBelow))
			return
		}

		expired, err := m.cfg.DB.ListExpiredRecordings(ctx, retentionBatchSize)
		if err != nil {
			log.Error("diskmonitor: ListExpiredRecordings error",
				slog.String("error", err.Error()))
			return
		}
		if len(expired) == 0 {
			log.Error("diskmonitor: disk over threshold but no expired recordings to delete; operator action needed",
				slog.Float64("used_pct", currentPct),
				slog.Float64("threshold_pct", threshold))
			return
		}

		for _, rec := range expired {
			// Re-check usage before each individual delete.
			u2, c2, err2 := m.statfs(path)
			if err2 == nil && c2 > 0 {
				pct2 := float64(u2) / float64(c2) * 100.0
				if pct2 < stopBelow {
					log.Info("diskmonitor: disk below hysteresis target; stopping mid-batch",
						slog.Float64("used_pct", pct2),
						slog.Float64("stop_below_pct", stopBelow))
					return
				}
			}

			// Unlink the file first, then remove the DB record.
			if err := os.Remove(rec.FilePath); err != nil && !os.IsNotExist(err) {
				log.Warn("diskmonitor: failed to remove file",
					slog.String("path", rec.FilePath),
					slog.String("error", err.Error()))
			}
			if err := m.cfg.DB.DeleteRecording(ctx, rec.ID); err != nil {
				log.Warn("diskmonitor: failed to delete recording from DB",
					slog.Int64("id", rec.ID),
					slog.String("error", err.Error()))
			} else {
				log.Info("diskmonitor: deleted expired recording",
					slog.Int64("id", rec.ID),
					slog.String("file", rec.FilePath),
					slog.Int64("size_bytes", rec.FileSize))
			}
		}
	}
}

// realStatfs wraps unix.Statfs to return (usedBytes, capacityBytes, error).
// Used is computed as (total - available) to represent space actually consumed.
func realStatfs(path string) (used, capacity int64, err error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	// Bsize may be int32 or int64 depending on platform; cast to int64 safely.
	blockSize := int64(stat.Bsize)
	total := int64(stat.Blocks) * blockSize
	avail := int64(stat.Bavail) * blockSize
	usedBytes := total - avail
	return usedBytes, total, nil
}

// ---------------------------------------------------------------------------
// DBAdapter — adapts *sql.DB (embedded in *nvrdb.DB) to the Database interface.
// ---------------------------------------------------------------------------

// DBAdapterImpl is the concrete adapter returned by NewDBAdapter.
type DBAdapterImpl struct {
	list   func(ctx context.Context, limit int) ([]ExpiredRecording, error)
	delete func(ctx context.Context, id int64) error
}

// ListExpiredRecordings implements Database.
func (a *DBAdapterImpl) ListExpiredRecordings(ctx context.Context, limit int) ([]ExpiredRecording, error) {
	return a.list(ctx, limit)
}

// DeleteRecording implements Database.
func (a *DBAdapterImpl) DeleteRecording(ctx context.Context, id int64) error {
	return a.delete(ctx, id)
}

// NewDBAdapter wraps a *sql.DB (or any type that embeds it, such as *nvrdb.DB)
// as a Database.
//
// The adapter selects recordings that ended before their camera's retention
// cutoff. The query joins recordings to cameras to find the retention_days
// for each camera, then identifies recordings whose end_time is older than
// NOW() - retention_days. A retention_days of 0 means "keep forever" —
// those recordings are excluded.
//
// NOTE: nvrdb.DB does not expose ListExpiredRecordings / DeleteRecording
// methods that match the Database interface. This adapter implements them
// directly in SQL. This is flagged in the task report.
//
// Callers pass nvrDB.DB (the embedded *sql.DB) — e.g.:
//
//	diskmonitor.NewDBAdapter(nvrDB.DB)
func NewDBAdapter(db *sql.DB) Database {
	return &DBAdapterImpl{
		list: func(ctx context.Context, limit int) ([]ExpiredRecording, error) {
			const q = `
				SELECT r.id, r.file_path, r.file_size
				FROM recordings r
				JOIN cameras c ON c.id = r.camera_id
				WHERE c.retention_days > 0
				  AND r.end_time < datetime('now', '-' || c.retention_days || ' days')
				ORDER BY r.start_time ASC
				LIMIT ?`
			rows, err := db.QueryContext(ctx, q, limit)
			if err != nil {
				return nil, fmt.Errorf("diskmonitor: ListExpiredRecordings: %w", err)
			}
			defer rows.Close()

			var out []ExpiredRecording
			for rows.Next() {
				var e ExpiredRecording
				if err := rows.Scan(&e.ID, &e.FilePath, &e.FileSize); err != nil {
					return nil, fmt.Errorf("diskmonitor: ListExpiredRecordings scan: %w", err)
				}
				out = append(out, e)
			}
			return out, rows.Err()
		},
		delete: func(ctx context.Context, id int64) error {
			res, err := db.ExecContext(ctx, `DELETE FROM recordings WHERE id = ?`, id)
			if err != nil {
				return fmt.Errorf("diskmonitor: DeleteRecording %d: %w", id, err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return fmt.Errorf("diskmonitor: DeleteRecording %d rows-affected: %w", id, err)
			}
			if n == 0 {
				return fmt.Errorf("diskmonitor: DeleteRecording %d: not found", id)
			}
			return nil
		},
	}
}
