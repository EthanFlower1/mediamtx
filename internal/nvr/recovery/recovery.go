package recovery

import (
	"fmt"
	"os"
)

// Logger abstracts logging for the recovery system.
type Logger interface {
	Log(level, format string, args ...interface{})
}

// stdLogger writes to stderr.
type stdLogger struct{}

func (l *stdLogger) Log(level, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "NVR recovery [%s]: %s\n", level, msg)
}

// RunConfig holds the parameters for a recovery run.
type RunConfig struct {
	RecordDirs []string
	DB         DBQuerier
	Reconciler Reconciler
	Logger     Logger
}

// RunResult summarizes the full recovery run.
type RunResult struct {
	Scanned       int
	Repaired      int
	AlreadyOK     int
	Unrecoverable int
	Reconcile     ReconcileResult
}

// Run performs startup recovery: scan for incomplete segments, repair them,
// and reconcile the database. This should be called synchronously during
// NVR initialization, before fragment backfill and recorder startup.
func Run(cfg RunConfig) (RunResult, error) {
	if cfg.Logger == nil {
		cfg.Logger = &stdLogger{}
	}

	cfg.Logger.Log("info", "starting recovery scan across %d directories", len(cfg.RecordDirs))

	// Phase 1: Scan.
	candidates, err := ScanForCandidates(cfg.RecordDirs, cfg.DB)
	if err != nil {
		return RunResult{}, fmt.Errorf("scan: %w", err)
	}

	if len(candidates) == 0 {
		cfg.Logger.Log("info", "no incomplete segments found")
		return RunResult{}, nil
	}

	cfg.Logger.Log("info", "found %d candidate segments", len(candidates))

	// Phase 2: Repair.
	var outcomes []RepairOutcome
	var result RunResult
	result.Scanned = len(candidates)

	for _, c := range candidates {
		repairResult, err := RepairSegment(c.FilePath)
		if err != nil {
			cfg.Logger.Log("warn", "repair failed for %s: %v", c.FilePath, err)
			result.Unrecoverable++
			continue
		}

		outcomes = append(outcomes, RepairOutcome{
			Candidate: c,
			Result:    repairResult,
		})

		switch {
		case repairResult.Repaired:
			result.Repaired++
			cfg.Logger.Log("info", "repaired %s: %s", c.FilePath, repairResult.Detail)
		case repairResult.AlreadyComplete:
			result.AlreadyOK++
			cfg.Logger.Log("debug", "already complete: %s", c.FilePath)
		case repairResult.Unrecoverable:
			result.Unrecoverable++
			cfg.Logger.Log("warn", "unrecoverable: %s — %s", c.FilePath, repairResult.Detail)
		}
	}

	// Phase 3: Reconcile.
	reconcileResult, err := Reconcile(outcomes, cfg.Reconciler)
	if err != nil {
		return result, fmt.Errorf("reconcile: %w", err)
	}
	result.Reconcile = reconcileResult

	cfg.Logger.Log("info", "recovery complete: scanned=%d repaired=%d ok=%d unrecoverable=%d inserted=%d updated=%d corrupt=%d",
		result.Scanned, result.Repaired, result.AlreadyOK, result.Unrecoverable,
		reconcileResult.Inserted, reconcileResult.Updated, reconcileResult.MarkedCorrupt)

	return result, nil
}
