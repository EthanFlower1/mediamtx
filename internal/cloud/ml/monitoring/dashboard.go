package monitoring

import (
	"encoding/json"
	"fmt"
)

// DashboardConfig generates Grafana dashboard JSON for per-model monitoring.
// The generated dashboards include panels for inference latency, accuracy,
// FP rate, drift KL divergence, drift PSI, and alert history.
type DashboardConfig struct {
	cfg Config
}

// NewDashboardConfig creates a DashboardConfig with the given monitoring config.
func NewDashboardConfig(cfg Config) *DashboardConfig {
	return &DashboardConfig{cfg: cfg}
}

// GenerateModelDashboard produces a Grafana dashboard JSON for a specific model.
func (dc *DashboardConfig) GenerateModelDashboard(key ModelKey) ([]byte, error) {
	if key.TenantID == "" {
		return nil, ErrInvalidTenantID
	}
	if key.ModelID == "" {
		return nil, ErrInvalidModelID
	}

	labelFilter := fmt.Sprintf(`tenant_id="%s", model_id="%s", version="%s"`,
		key.TenantID, key.ModelID, key.Version)

	dashboard := map[string]any{
		"title": fmt.Sprintf("Model Monitoring: %s/%s:%s", key.TenantID, key.ModelID, key.Version),
		"uid":   fmt.Sprintf("model-%s-%s-%s", key.TenantID, key.ModelID, key.Version),
		"tags":  []string{"kaivue", "model-monitoring", "auto-provisioned"},
		"templating": map[string]any{
			"list": []map[string]any{},
		},
		"panels": []map[string]any{
			dc.panel(1, "Inference Rate", "rate", 0, 0,
				fmt.Sprintf(`rate(kaivue_model_inference_total{%s}[5m])`, labelFilter),
				"req/s"),
			dc.panel(2, "Inference Latency (p99)", "graph", 12, 0,
				fmt.Sprintf(`histogram_quantile(0.99, rate(kaivue_model_inference_latency_seconds_bucket{%s}[5m]))`, labelFilter),
				"seconds"),
			dc.panel(3, "Accuracy", "gauge", 0, 8,
				fmt.Sprintf(`kaivue_model_accuracy_pct{%s}`, labelFilter),
				"percent"),
			dc.panel(4, "False Positive Rate", "gauge", 6, 8,
				fmt.Sprintf(`kaivue_model_false_positive_rate_pct{%s}`, labelFilter),
				"percent"),
			dc.panel(5, "KL Divergence", "graph", 12, 8,
				fmt.Sprintf(`kaivue_model_drift_kl_divergence{%s}`, labelFilter),
				""),
			dc.panel(6, "Population Stability Index", "graph", 0, 16,
				fmt.Sprintf(`kaivue_model_drift_psi{%s}`, labelFilter),
				""),
			dc.panel(7, "Drift Alerts (cumulative)", "graph", 12, 16,
				fmt.Sprintf(`kaivue_model_drift_alerts_total{%s}`, labelFilter),
				""),
			dc.panel(8, "Error Rate", "graph", 0, 24,
				fmt.Sprintf(`rate(kaivue_model_inference_errors_total{%s}[5m]) / rate(kaivue_model_inference_total{%s}[5m])`, labelFilter, labelFilter),
				"percent"),
			dc.thresholdPanel(9, "KL Divergence Threshold", 12, 24,
				fmt.Sprintf(`kaivue_model_drift_kl_divergence{%s}`, labelFilter),
				dc.cfg.KLDivergenceThreshold),
			dc.thresholdPanel(10, "PSI Threshold", 0, 32,
				fmt.Sprintf(`kaivue_model_drift_psi{%s}`, labelFilter),
				dc.cfg.PSIThreshold),
		},
		"time": map[string]string{
			"from": "now-24h",
			"to":   "now",
		},
		"refresh":       "30s",
		"schemaVersion": 39,
		"version":       1,
	}

	return json.MarshalIndent(dashboard, "", "  ")
}

// GenerateOverviewDashboard produces a Grafana dashboard summarizing all
// models for a tenant.
func (dc *DashboardConfig) GenerateOverviewDashboard(tenantID string) ([]byte, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}

	labelFilter := fmt.Sprintf(`tenant_id="%s"`, tenantID)

	dashboard := map[string]any{
		"title": fmt.Sprintf("Model Monitoring Overview: %s", tenantID),
		"uid":   fmt.Sprintf("model-overview-%s", tenantID),
		"tags":  []string{"kaivue", "model-monitoring", "overview", "auto-provisioned"},
		"panels": []map[string]any{
			dc.panel(1, "Total Inference Rate (all models)", "graph", 0, 0,
				fmt.Sprintf(`sum by (model_id) (rate(kaivue_model_inference_total{%s}[5m]))`, labelFilter),
				"req/s"),
			dc.panel(2, "Max KL Divergence (all models)", "graph", 12, 0,
				fmt.Sprintf(`max by (model_id) (kaivue_model_drift_kl_divergence{%s})`, labelFilter),
				""),
			dc.panel(3, "Accuracy by Model", "graph", 0, 8,
				fmt.Sprintf(`kaivue_model_accuracy_pct{%s}`, labelFilter),
				"percent"),
			dc.panel(4, "Drift Alerts by Model", "graph", 12, 8,
				fmt.Sprintf(`sum by (model_id) (kaivue_model_drift_alerts_total{%s})`, labelFilter),
				""),
		},
		"time": map[string]string{
			"from": "now-24h",
			"to":   "now",
		},
		"refresh":       "1m",
		"schemaVersion": 39,
		"version":       1,
	}

	return json.MarshalIndent(dashboard, "", "  ")
}

func (dc *DashboardConfig) panel(id int, title, panelType string, x, y int, expr, unit string) map[string]any {
	return map[string]any{
		"id":    id,
		"title": title,
		"type":  panelType,
		"gridPos": map[string]int{
			"h": 8,
			"w": 12,
			"x": x,
			"y": y,
		},
		"targets": []map[string]any{
			{
				"expr":         expr,
				"legendFormat": "{{model_id}}:{{version}}",
				"refId":        "A",
			},
		},
		"fieldConfig": map[string]any{
			"defaults": map[string]any{
				"unit": unit,
			},
		},
	}
}

func (dc *DashboardConfig) thresholdPanel(id int, title string, x, y int, expr string, threshold float64) map[string]any {
	p := dc.panel(id, title, "graph", x, y, expr, "")
	p["thresholds"] = []map[string]any{
		{
			"value": threshold,
			"op":    "gt",
			"fill":  true,
			"line":  true,
			"colorMode": "critical",
		},
	}
	return p
}
