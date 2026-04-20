package legacydb

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// MaintenanceConfig holds configuration for periodic database maintenance tasks.
type MaintenanceConfig struct {
	// WALCheckpointInterval is how often to run WAL checkpoint (TRUNCATE mode).
	// Zero disables periodic checkpointing (SQLite auto-checkpoints still apply).
	WALCheckpointInterval time.Duration

	// VacuumHour is the hour of the day (0-23) when VACUUM should run.
	// Use -1 to disable scheduled VACUUM.
	VacuumHour int

	// VacuumDay controls which day of the week VACUUM runs (0=Sunday, 6=Saturday).
	// Use -1 to run every day at VacuumHour.
	VacuumDay int
}

// DefaultMaintenanceConfig returns sensible defaults for database maintenance.
func DefaultMaintenanceConfig() MaintenanceConfig {
	return MaintenanceConfig{
		WALCheckpointInterval: 4 * time.Hour,
		VacuumHour:            3, // 3 AM
		VacuumDay:             0, // Sunday
	}
}

// DBHealth holds the result of a database health check.
type DBHealth struct {
	IntegrityOK   bool   `json:"integrity_ok"`
	IntegrityMsg  string `json:"integrity_message,omitempty"`
	WALSizePages  int64  `json:"wal_size_pages"`
	PageCount     int64  `json:"page_count"`
	PageSize      int64  `json:"page_size"`
	FreeListCount int64  `json:"freelist_count"`
	FileSizeBytes int64  `json:"file_size_bytes"`
	LastCheckpoint string `json:"last_checkpoint,omitempty"`
	LastVacuum     string `json:"last_vacuum,omitempty"`
}

// CheckIntegrity runs PRAGMA integrity_check and returns whether the database
// is healthy. This can take time on large databases; use with care.
func (d *DB) CheckIntegrity() (ok bool, message string, err error) {
	rows, err := d.Query("PRAGMA integrity_check")
	if err != nil {
		return false, "", fmt.Errorf("run integrity_check: %w", err)
	}
	defer rows.Close()

	var messages []string
	for rows.Next() {
		var msg string
		if err := rows.Scan(&msg); err != nil {
			return false, "", fmt.Errorf("scan integrity_check result: %w", err)
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return false, "", fmt.Errorf("iterate integrity_check results: %w", err)
	}

	if len(messages) == 1 && messages[0] == "ok" {
		return true, "ok", nil
	}
	return false, strings.Join(messages, "; "), nil
}

// QuickIntegrityCheck runs PRAGMA quick_check which is faster than a full
// integrity_check but does not verify index consistency.
func (d *DB) QuickIntegrityCheck() (ok bool, message string, err error) {
	rows, err := d.Query("PRAGMA quick_check")
	if err != nil {
		return false, "", fmt.Errorf("run quick_check: %w", err)
	}
	defer rows.Close()

	var messages []string
	for rows.Next() {
		var msg string
		if err := rows.Scan(&msg); err != nil {
			return false, "", fmt.Errorf("scan quick_check result: %w", err)
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return false, "", fmt.Errorf("iterate quick_check results: %w", err)
	}

	if len(messages) == 1 && messages[0] == "ok" {
		return true, "ok", nil
	}
	return false, strings.Join(messages, "; "), nil
}

// WALCheckpoint performs a WAL checkpoint in TRUNCATE mode, which reclaims
// WAL file space. Returns the number of WAL frames and checkpointed frames.
func (d *DB) WALCheckpoint() (walFrames, checkpointed int64, err error) {
	// TRUNCATE mode: checkpoint and truncate the WAL file to zero size.
	err = d.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(
		new(int64), // busy flag (ignored)
		&walFrames,
		&checkpointed,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("wal_checkpoint: %w", err)
	}
	return walFrames, checkpointed, nil
}

// Vacuum runs VACUUM to rebuild the database file, reclaiming free pages
// and defragmenting. This is a blocking operation and requires exclusive access.
func (d *DB) Vacuum() error {
	_, err := d.Exec("VACUUM")
	if err != nil {
		return fmt.Errorf("vacuum: %w", err)
	}
	return nil
}

// GetDBHealth returns current database health metrics.
func (d *DB) GetDBHealth() (*DBHealth, error) {
	h := &DBHealth{}

	_ = d.QueryRow("PRAGMA page_count").Scan(&h.PageCount)
	_ = d.QueryRow("PRAGMA page_size").Scan(&h.PageSize)
	_ = d.QueryRow("PRAGMA freelist_count").Scan(&h.FreeListCount)

	h.FileSizeBytes = h.PageCount * h.PageSize

	// WAL size: count of pages in the WAL file.
	var busy int64
	err := d.QueryRow("PRAGMA wal_checkpoint(PASSIVE)").Scan(&busy, &h.WALSizePages, new(int64))
	if err != nil {
		// Not fatal; WAL metrics just won't be available.
		h.WALSizePages = -1
	}

	// Read last maintenance timestamps from config table.
	h.LastCheckpoint, _ = d.GetConfig("maintenance_last_checkpoint")
	h.LastVacuum, _ = d.GetConfig("maintenance_last_vacuum")

	// Run a quick check for integrity status.
	ok, msg, err := d.QuickIntegrityCheck()
	if err != nil {
		h.IntegrityOK = false
		h.IntegrityMsg = fmt.Sprintf("check failed: %v", err)
	} else {
		h.IntegrityOK = ok
		h.IntegrityMsg = msg
	}

	return h, nil
}

// MaintenanceRunner runs periodic database maintenance tasks in the background.
type MaintenanceRunner struct {
	db     *DB
	config MaintenanceConfig
	cancel context.CancelFunc
	done   chan struct{}
}

// StartMaintenance starts the background maintenance goroutine. The caller must
// call Stop() to clean up. The onAlert callback is invoked when corruption or
// errors are detected; it may be nil.
func (d *DB) StartMaintenance(cfg MaintenanceConfig, onAlert func(alertType, message string)) *MaintenanceRunner {
	ctx, cancel := context.WithCancel(context.Background())
	r := &MaintenanceRunner{
		db:     d,
		config: cfg,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	go r.run(ctx, onAlert)
	return r
}

// Stop halts the maintenance runner and waits for the goroutine to exit.
func (r *MaintenanceRunner) Stop() {
	r.cancel()
	<-r.done
}

func (r *MaintenanceRunner) run(ctx context.Context, onAlert func(string, string)) {
	defer close(r.done)

	if onAlert == nil {
		onAlert = func(_, _ string) {}
	}

	// Run startup integrity check.
	r.runStartupIntegrityCheck(onAlert)

	// Determine tick interval: use the shorter of WAL checkpoint and 1 minute
	// (to check if it's time for VACUUM).
	tickInterval := 1 * time.Minute
	if r.config.WALCheckpointInterval > 0 && r.config.WALCheckpointInterval < tickInterval {
		tickInterval = r.config.WALCheckpointInterval
	}

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	var lastCheckpoint time.Time
	var lastVacuumDate string

	for {
		select {
		case <-ctx.Done():
			// Run a final checkpoint before shutdown.
			if _, _, err := r.db.WALCheckpoint(); err != nil {
				log.Printf("[NVR] [db-maintenance] final checkpoint error: %v", err)
			}
			return

		case now := <-ticker.C:
			// WAL checkpoint.
			if r.config.WALCheckpointInterval > 0 && now.Sub(lastCheckpoint) >= r.config.WALCheckpointInterval {
				walFrames, checkpointed, err := r.db.WALCheckpoint()
				if err != nil {
					log.Printf("[NVR] [db-maintenance] WAL checkpoint error: %v", err)
					onAlert("db_checkpoint_error", fmt.Sprintf("WAL checkpoint failed: %v", err))
				} else {
					if walFrames > 0 {
						log.Printf("[NVR] [db-maintenance] WAL checkpoint: %d frames, %d checkpointed", walFrames, checkpointed)
					}
					ts := now.UTC().Format(time.RFC3339)
					_ = r.db.SetConfig("maintenance_last_checkpoint", ts)
				}
				lastCheckpoint = now
			}

			// VACUUM: run during the configured low-activity window.
			if r.config.VacuumHour >= 0 {
				today := now.Format("2006-01-02")
				inWindow := now.Hour() == r.config.VacuumHour && lastVacuumDate != today
				rightDay := r.config.VacuumDay < 0 || int(now.Weekday()) == r.config.VacuumDay

				if inWindow && rightDay {
					log.Printf("[NVR] [db-maintenance] starting scheduled VACUUM")
					start := time.Now()
					if err := r.db.Vacuum(); err != nil {
						log.Printf("[NVR] [db-maintenance] VACUUM error: %v", err)
						onAlert("db_vacuum_error", fmt.Sprintf("VACUUM failed: %v", err))
					} else {
						elapsed := time.Since(start)
						log.Printf("[NVR] [db-maintenance] VACUUM completed in %v", elapsed)
						ts := now.UTC().Format(time.RFC3339)
						_ = r.db.SetConfig("maintenance_last_vacuum", ts)
					}
					lastVacuumDate = today
				}
			}
		}
	}
}

func (r *MaintenanceRunner) runStartupIntegrityCheck(onAlert func(string, string)) {
	log.Printf("[NVR] [db-maintenance] running startup integrity check")
	ok, msg, err := r.db.CheckIntegrity()
	if err != nil {
		log.Printf("[NVR] [db-maintenance] integrity check error: %v", err)
		onAlert("db_integrity_error", fmt.Sprintf(
			"Database integrity check failed to execute: %v. "+
				"Consider backing up the database and running 'PRAGMA integrity_check' manually.", err))
		return
	}
	if !ok {
		log.Printf("[NVR] [db-maintenance] DATABASE CORRUPTION DETECTED: %s", msg)
		onAlert("db_corruption", fmt.Sprintf(
			"Database corruption detected: %s. "+
				"Recovery steps: (1) Stop the NVR. (2) Copy the database file as a backup. "+
				"(3) Try running '.recover' in the sqlite3 CLI to export salvageable data. "+
				"(4) If the database is unusable, delete it and let the NVR recreate it from scratch.",
			msg))
	} else {
		log.Printf("[NVR] [db-maintenance] integrity check passed")
	}
}
