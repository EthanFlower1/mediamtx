// Package monitoring implements model drift detection and performance
// monitoring for the Kaivue AI/ML platform (KAI-293).
//
// It tracks per-model accuracy, false-positive rate, and inference latency
// via Prometheus counters/histograms, performs statistical drift detection
// on input feature distributions using KL divergence and Population
// Stability Index (PSI), and fires alerts when models drop below
// configurable baselines.
//
// Key components:
//
//   - Collector:         Prometheus metric registration and recording
//   - DistributionTracker: Maintains reference and live feature histograms
//   - DriftDetector:     Runs periodic checks for distribution shift
//   - AlertManager:      Integrates drift/perf alerts with on-call rotation
//   - DashboardConfig:   Auto-provisions per-model Grafana dashboards
//   - AuditExporter:     Exports SOC 2 audit evidence for model monitoring
//
// Multi-tenant invariant: every exported method accepts a tenantID
// parameter. Cross-tenant data access is impossible by construction.
package monitoring
