package monitoring

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"
)

// AuditExporter collects and exports SOC 2 audit evidence for model
// monitoring activity. It stores records in memory and supports export
// to JSON and CSV formats for compliance reporting.
type AuditExporter struct {
	mu      sync.RWMutex
	records []AuditRecord
	cfg     Config
}

// NewAuditExporter creates an AuditExporter with the given config.
func NewAuditExporter(cfg Config) *AuditExporter {
	return &AuditExporter{cfg: cfg}
}

// Record appends an audit record.
func (ae *AuditExporter) Record(rec AuditRecord) {
	ae.mu.Lock()
	defer ae.mu.Unlock()
	ae.records = append(ae.records, rec)
}

// Records returns a copy of all stored audit records.
func (ae *AuditExporter) Records() []AuditRecord {
	ae.mu.RLock()
	defer ae.mu.RUnlock()
	out := make([]AuditRecord, len(ae.records))
	copy(out, ae.records)
	return out
}

// RecordsByTenant returns audit records filtered by tenant.
func (ae *AuditExporter) RecordsByTenant(tenantID string) []AuditRecord {
	ae.mu.RLock()
	defer ae.mu.RUnlock()
	var out []AuditRecord
	for _, r := range ae.records {
		if r.TenantID == tenantID {
			out = append(out, r)
		}
	}
	if out == nil {
		out = []AuditRecord{}
	}
	return out
}

// RecordsByTimeRange returns audit records within the given time range.
func (ae *AuditExporter) RecordsByTimeRange(start, end time.Time) []AuditRecord {
	ae.mu.RLock()
	defer ae.mu.RUnlock()
	var out []AuditRecord
	for _, r := range ae.records {
		if !r.Timestamp.Before(start) && !r.Timestamp.After(end) {
			out = append(out, r)
		}
	}
	if out == nil {
		out = []AuditRecord{}
	}
	return out
}

// ExportJSON writes all audit records as a JSON array to the writer.
func (ae *AuditExporter) ExportJSON(w io.Writer) error {
	records := ae.Records()
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(records)
}

// ExportJSONForTenant writes tenant-scoped audit records as JSON.
func (ae *AuditExporter) ExportJSONForTenant(w io.Writer, tenantID string) error {
	records := ae.RecordsByTenant(tenantID)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(records)
}

// ExportCSV writes all audit records as CSV to the writer.
func (ae *AuditExporter) ExportCSV(w io.Writer) error {
	records := ae.Records()
	return ae.writeCSV(w, records)
}

// ExportCSVForTenant writes tenant-scoped audit records as CSV.
func (ae *AuditExporter) ExportCSVForTenant(w io.Writer, tenantID string) error {
	records := ae.RecordsByTenant(tenantID)
	return ae.writeCSV(w, records)
}

func (ae *AuditExporter) writeCSV(w io.Writer, records []AuditRecord) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	// Header
	header := []string{
		"id", "timestamp", "tenant_id", "model_id", "model_version",
		"event_type", "details", "kl_divergence", "psi",
		"accuracy", "fp_rate", "latency_p99_ms", "alert_fired", "exported_at",
	}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("monitoring: write CSV header: %w", err)
	}

	for _, r := range records {
		row := []string{
			r.ID,
			r.Timestamp.Format(time.RFC3339),
			r.TenantID,
			r.ModelID,
			r.ModelVersion,
			r.EventType,
			r.Details,
			optFloat(r.KLDivergence),
			optFloat(r.PSI),
			optFloat(r.Accuracy),
			optFloat(r.FPRate),
			optFloat(r.LatencyP99Ms),
			strconv.FormatBool(r.AlertFired),
			r.ExportedAt.Format(time.RFC3339),
		}
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("monitoring: write CSV row: %w", err)
		}
	}
	return nil
}

// PruneOlderThan removes records older than the retention period.
func (ae *AuditExporter) PruneOlderThan(cutoff time.Time) int {
	ae.mu.Lock()
	defer ae.mu.Unlock()
	var kept []AuditRecord
	pruned := 0
	for _, r := range ae.records {
		if r.Timestamp.Before(cutoff) {
			pruned++
		} else {
			kept = append(kept, r)
		}
	}
	ae.records = kept
	return pruned
}

func optFloat(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', 6, 64)
}
