package automation

import "github.com/bluenviron/mediamtx/internal/cloud/statuspage/provider"

// ComponentRule maps a Statuspage.io component to a Prometheus query and
// threshold-based degradation rules.
type ComponentRule struct {
	// ComponentID is the Statuspage.io component ID.
	ComponentID string `json:"component_id"`

	// ComponentName is a human-readable label for logging.
	ComponentName string `json:"component_name"`

	// PrometheusQuery is a PromQL instant-query expression. The result must
	// be a scalar or single-element vector. Example:
	//   up{job="cloud-apiserver"} == 1
	PrometheusQuery string `json:"prometheus_query"`

	// Thresholds define status transitions based on the query result value.
	// They are evaluated in order; the first match wins. If no threshold
	// matches, the component is left at its current status.
	Thresholds []Threshold `json:"thresholds"`
}

// Threshold maps a value range to a component status.
type Threshold struct {
	// Min is the inclusive lower bound (use -Inf for no lower bound).
	Min float64 `json:"min"`

	// Max is the exclusive upper bound (use +Inf for no upper bound).
	Max float64 `json:"max"`

	// Status is the component status to set when the query value falls
	// within [Min, Max).
	Status provider.ComponentStatus `json:"status"`
}

// Evaluate returns the component status for a given metric value, or empty
// string if no threshold matches.
func (r *ComponentRule) Evaluate(value float64) provider.ComponentStatus {
	for _, th := range r.Thresholds {
		if value >= th.Min && value < th.Max {
			return th.Status
		}
	}
	return ""
}

// DefaultComponentRules returns the standard set of rules for all KaiVue
// platform components listed in KAI-375.
func DefaultComponentRules() []ComponentRule {
	return []ComponentRule{
		{
			ComponentName:   "Cloud Control Plane",
			PrometheusQuery: `up{job="cloud-apiserver"}`,
			Thresholds: []Threshold{
				{Min: 1, Max: 2, Status: provider.ComponentOperational},
				{Min: 0, Max: 1, Status: provider.ComponentMajorOutage},
			},
		},
		{
			ComponentName:   "Identity",
			PrometheusQuery: `up{job="identity-service"}`,
			Thresholds: []Threshold{
				{Min: 1, Max: 2, Status: provider.ComponentOperational},
				{Min: 0, Max: 1, Status: provider.ComponentMajorOutage},
			},
		},
		{
			ComponentName:   "Cloud Directory",
			PrometheusQuery: `up{job="directory-service"}`,
			Thresholds: []Threshold{
				{Min: 1, Max: 2, Status: provider.ComponentOperational},
				{Min: 0, Max: 1, Status: provider.ComponentMajorOutage},
			},
		},
		{
			ComponentName:   "Integrator Portal",
			PrometheusQuery: `up{job="integrator-portal"}`,
			Thresholds: []Threshold{
				{Min: 1, Max: 2, Status: provider.ComponentOperational},
				{Min: 0, Max: 1, Status: provider.ComponentMajorOutage},
			},
		},
		{
			ComponentName:   "AI Inference",
			PrometheusQuery: `up{job="ai-inference"}`,
			Thresholds: []Threshold{
				{Min: 1, Max: 2, Status: provider.ComponentOperational},
				{Min: 0, Max: 1, Status: provider.ComponentMajorOutage},
			},
		},
		{
			ComponentName:   "Recording Archive",
			PrometheusQuery: `up{job="recording-archive"}`,
			Thresholds: []Threshold{
				{Min: 1, Max: 2, Status: provider.ComponentOperational},
				{Min: 0, Max: 1, Status: provider.ComponentMajorOutage},
			},
		},
		{
			ComponentName:   "Notifications",
			PrometheusQuery: `up{job="notifications"}`,
			Thresholds: []Threshold{
				{Min: 1, Max: 2, Status: provider.ComponentOperational},
				{Min: 0, Max: 1, Status: provider.ComponentMajorOutage},
			},
		},
		{
			ComponentName:   "Cloud Relay",
			PrometheusQuery: `up{job="cloud-relay"}`,
			Thresholds: []Threshold{
				{Min: 1, Max: 2, Status: provider.ComponentOperational},
				{Min: 0, Max: 1, Status: provider.ComponentMajorOutage},
			},
		},
		{
			ComponentName:   "Marketing Site",
			PrometheusQuery: `probe_success{job="blackbox",target="marketing"}`,
			Thresholds: []Threshold{
				{Min: 1, Max: 2, Status: provider.ComponentOperational},
				{Min: 0, Max: 1, Status: provider.ComponentMajorOutage},
			},
		},
		{
			ComponentName:   "Docs",
			PrometheusQuery: `probe_success{job="blackbox",target="docs"}`,
			Thresholds: []Threshold{
				{Min: 1, Max: 2, Status: provider.ComponentOperational},
				{Min: 0, Max: 1, Status: provider.ComponentMajorOutage},
			},
		},
	}
}
