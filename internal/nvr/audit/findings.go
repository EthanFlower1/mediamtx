//go:build integration

// Package audit provides structured finding and report types for the
// recording-pipeline integration audit.
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Severity constants classify the impact of a finding.
const (
	SeverityDataLoss    = "data-loss"
	SeverityCorruption  = "corruption"
	SeverityGap         = "gap"
	SeverityRecoverable = "recoverable"
)

// Finding represents a single audit observation.
type Finding struct {
	Scenario     string `json:"scenario"`
	Layer        string `json:"layer"`
	Severity     string `json:"severity"`
	Description  string `json:"description"`
	Reproduction string `json:"reproduction"`
	DataImpact   string `json:"data_impact"`
	Recovery     string `json:"recovery"`
}

// Report collects findings produced during an audit run.
type Report struct {
	mu       sync.Mutex
	Findings []Finding `json:"findings"`
	RunAt    time.Time `json:"run_at"`
}

// NewReport creates a new Report timestamped to now.
func NewReport() *Report {
	return &Report{
		RunAt: time.Now(),
	}
}

// Add appends a finding to the report in a thread-safe manner.
func (r *Report) Add(f Finding) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Findings = append(r.Findings, f)
}

// WriteJSON serializes the report to a JSON file at the given path.
func (r *Report) WriteJSON(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// WriteMarkdown writes a human-readable markdown summary to the given path.
func (r *Report) WriteMarkdown(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var b strings.Builder
	b.WriteString("# Recording Pipeline Audit Report\n\n")
	b.WriteString(fmt.Sprintf("**Run at:** %s\n\n", r.RunAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("**Total findings:** %d\n\n", len(r.Findings)))

	if len(r.Findings) == 0 {
		b.WriteString("No findings recorded.\n")
		return os.WriteFile(path, []byte(b.String()), 0o644)
	}

	for i, f := range r.Findings {
		b.WriteString(fmt.Sprintf("## Finding %d: %s\n\n", i+1, f.Scenario))
		b.WriteString(fmt.Sprintf("- **Layer:** %s\n", f.Layer))
		b.WriteString(fmt.Sprintf("- **Severity:** %s\n", f.Severity))
		b.WriteString(fmt.Sprintf("- **Description:** %s\n", f.Description))
		b.WriteString(fmt.Sprintf("- **Reproduction:** %s\n", f.Reproduction))
		b.WriteString(fmt.Sprintf("- **Data impact:** %s\n", f.DataImpact))
		b.WriteString(fmt.Sprintf("- **Recovery:** %s\n\n", f.Recovery))
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}
