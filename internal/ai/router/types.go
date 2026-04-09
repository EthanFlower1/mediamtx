// Package router implements the KAI-280 edge-vs-cloud inference routing
// engine.  Given a Feature, a tenant identifier, and the local hardware
// capability, Router.Route returns a deterministic Decision describing where
// the inference workload should run (edge or cloud) and why.
//
// The default routing matrix is encoded as a single declarative table in
// router.go so it can be audited at a glance.  Per-tenant policy overrides
// and a runtime fallback path (edge -> cloud) are supported without restart.
//
// This package is intentionally standalone: it has no dependency on the
// recorder, the inferencer, or any HTTP/CLI surface.  Wiring it into the
// Recorder pipeline is tracked as a follow-up ticket.
package router

import (
	"time"
)

// Feature is the catalogue of inference workloads the router knows how to
// place.  Each feature corresponds to a row of § 11.2 of the v1 spec.
type Feature string

// Feature constants.  String values are stable and used in structured logs.
const (
	FeatureObjectDetectionLight Feature = "object_detection_light"
	FeatureObjectDetectionHeavy Feature = "object_detection_heavy"
	FeatureFace                 Feature = "face"
	FeatureLPR                  Feature = "lpr"
	FeatureBehavioral           Feature = "behavioral"
	FeatureAudio                Feature = "audio"
	FeatureCLIPEmbed            Feature = "clip_embed"
	FeatureVectorSearch         Feature = "vector_search"
	FeatureCrossCameraTrack     Feature = "cross_camera_track"
	FeatureAnomaly              Feature = "anomaly"
	FeatureSummary              Feature = "summary"
	FeatureForensic             Feature = "forensic"
	FeatureCustom               Feature = "custom"
)

// AllFeatures returns every Feature constant in declaration order.  Used by
// tests and audits to ensure the routing matrix covers the full enum.
func AllFeatures() []Feature {
	return []Feature{
		FeatureObjectDetectionLight,
		FeatureObjectDetectionHeavy,
		FeatureFace,
		FeatureLPR,
		FeatureBehavioral,
		FeatureAudio,
		FeatureCLIPEmbed,
		FeatureVectorSearch,
		FeatureCrossCameraTrack,
		FeatureAnomaly,
		FeatureSummary,
		FeatureForensic,
		FeatureCustom,
	}
}

// Location is where an inference workload runs.
type Location string

// Location constants.
const (
	LocationEdge  Location = "edge"
	LocationCloud Location = "cloud"
)

// Decision is the structured output of Router.Route.  It is deterministic
// for a given (feature, tenant, hardware, policy) tuple and safe to log.
type Decision struct {
	// Feature is the workload that was routed.
	Feature Feature `json:"feature"`

	// TenantID is the tenant the decision was made for.
	TenantID string `json:"tenant_id"`

	// Location is the chosen execution location.
	Location Location `json:"location"`

	// Reason is a short human-readable explanation of why this Location was
	// chosen (matrix default, hardware fallback, tenant override, etc.).
	Reason string `json:"reason"`

	// Fallback is true when the decision is the result of an edge->cloud
	// fallback (e.g. inference failure or GPU saturation) rather than the
	// default matrix.
	Fallback bool `json:"fallback,omitempty"`
}

// HardwareCapability summarises the inference-relevant hardware on the local
// system.  Probed at startup and on hardware-change events by HardwareProbe.
type HardwareCapability struct {
	// GPUPresent is true when at least one inference-capable GPU is available.
	GPUPresent bool `json:"gpu_present"`

	// GPUCount is the number of inference-capable GPUs detected.
	GPUCount int `json:"gpu_count"`

	// GPUMemoryMB is the total VRAM (sum across GPUs) in megabytes.
	GPUMemoryMB int `json:"gpu_memory_mb"`

	// GPUSaturated is set by the runtime when the GPU is at capacity and
	// further edge inference should fall back to cloud.  The router does not
	// own this flag; it is updated externally via Router.SetSaturated.
	GPUSaturated bool `json:"gpu_saturated,omitempty"`

	// CPUClass is a coarse label for CPU performance ("low", "mid", "high").
	CPUClass string `json:"cpu_class"`

	// ProbedAt is when this snapshot was last refreshed.
	ProbedAt time.Time `json:"probed_at"`
}

// Summary returns a compact one-line representation suitable for log fields.
func (h HardwareCapability) Summary() string {
	if !h.GPUPresent {
		return "no_gpu cpu=" + h.CPUClass
	}
	return "gpu=" + itoa(h.GPUCount) + " vram_mb=" + itoa(h.GPUMemoryMB) +
		" cpu=" + h.CPUClass
}

// TenantPolicy lets a tenant override the default matrix on a per-feature
// basis.  An empty TenantPolicy disables all overrides.
type TenantPolicy struct {
	// TenantID is the tenant this policy applies to.
	TenantID string `json:"tenant_id"`

	// ForceCloud lists features the tenant has chosen to always run in the
	// cloud, regardless of edge hardware availability.
	ForceCloud map[Feature]bool `json:"force_cloud,omitempty"`

	// ForceEdge lists features the tenant has chosen to always attempt at the
	// edge.  This may still fall back to cloud if the matrix marks the feature
	// as cloud-only or if the edge runtime fails — see Router.Route for
	// precedence rules.
	ForceEdge map[Feature]bool `json:"force_edge,omitempty"`
}

// PolicyStore returns the active TenantPolicy for a tenant ID.  An
// implementation that has no record for a tenant MUST return the zero
// TenantPolicy and a nil error (the router treats that as "no overrides").
type PolicyStore interface {
	GetPolicy(tenantID string) (TenantPolicy, error)
}

// staticPolicyStore is a trivial in-memory PolicyStore used by tests and as
// the default when no real store is wired in.
type staticPolicyStore struct {
	policies map[string]TenantPolicy
}

// NewStaticPolicyStore returns a PolicyStore backed by the given map.
// The map is not copied; do not mutate it after construction.
func NewStaticPolicyStore(policies map[string]TenantPolicy) PolicyStore {
	if policies == nil {
		policies = map[string]TenantPolicy{}
	}
	return &staticPolicyStore{policies: policies}
}

// GetPolicy implements PolicyStore.
func (s *staticPolicyStore) GetPolicy(tenantID string) (TenantPolicy, error) {
	return s.policies[tenantID], nil
}

// itoa is a tiny stdlib-free helper to keep Summary allocation-light.
// (We deliberately avoid pulling strconv into types.go to keep this file
// self-contained; the cost is negligible at log frequency.)
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
