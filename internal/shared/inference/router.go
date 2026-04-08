package inference

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Feature is a stable identifier for an AI capability that the router knows
// how to place. The values are referenced from feature tickets (KAI-281..291)
// so keep them string-stable; renames require a migration.
type Feature string

const (
	FeatureLightweightObjectDetection Feature = "lightweight_object_detection"
	FeatureHeavyObjectDetection       Feature = "heavy_object_detection"
	FeatureFaceRecognition            Feature = "face_recognition"
	FeatureLicensePlateRecognition    Feature = "license_plate_recognition"
	FeatureBehavioralAnalysis         Feature = "behavioral_analysis"
	FeatureAudioEventDetection        Feature = "audio_event_detection"
	FeatureCLIPEmbedding              Feature = "clip_embedding"
	FeatureForensicSearch             Feature = "forensic_search"
)

// Location is where an inference should run.
type Location string

const (
	LocationEdge  Location = "edge"
	LocationCloud Location = "cloud"
)

// HardwareCapability is a caller-supplied snapshot of what the local
// machine can do. Real probing (NVIDIA GPU presence, Jetson model detection,
// Apple Silicon Neural Engine, DirectML adapter enumeration) lands in a
// later ticket; for now the Router trusts the caller.
type HardwareCapability struct {
	// HasGPU is true if a general-purpose GPU (CUDA, Metal, DirectML) is
	// available for inference.
	HasGPU bool

	// HasNPU is true if a dedicated neural accelerator is present
	// (Jetson DLA, Apple Neural Engine, Hailo, Coral, etc.).
	HasNPU bool

	// GPUMemoryMB is the free VRAM in megabytes at probe time. Zero
	// means "unknown", not "no GPU".
	GPUMemoryMB int

	// Backends lists the BackendKinds the local process can actually
	// instantiate. An empty slice forces cloud routing for everything.
	Backends []BackendKind
}

// Supports reports whether the given backend is in the capability list.
func (h HardwareCapability) Supports(b BackendKind) bool {
	for _, x := range h.Backends {
		if x == b {
			return true
		}
	}
	return false
}

// FeaturePolicy is the rule the Router applies for a single feature. It is
// intentionally declarative so the matrix can be inspected in tests and
// documented in the README.
type FeaturePolicy struct {
	// PreferredLocation is where the feature SHOULD run if the hardware
	// allows it.
	PreferredLocation Location

	// RequireGPU, when true, forces cloud routing if the edge has no
	// GPU (even if PreferredLocation is edge).
	RequireGPU bool

	// AllowCloudFallback, when true, permits the router to fall back to
	// cloud if the preferred edge placement is unavailable.
	AllowCloudFallback bool

	// PreferredBackends is an ordered preference list. The router picks
	// the first available backend from this list.
	PreferredBackends []BackendKind
}

// defaultFeatureRoutes encodes the routing matrix from v1-roadmap §11.2.
// Keep this map in sync with the README in this package.
var defaultFeatureRoutes = map[Feature]FeaturePolicy{
	FeatureLightweightObjectDetection: {
		PreferredLocation:  LocationEdge,
		RequireGPU:         false,
		AllowCloudFallback: true,
		PreferredBackends:  []BackendKind{BackendCoreML, BackendDirectML, BackendONNXRuntime, BackendTensorRT},
	},
	FeatureHeavyObjectDetection: {
		PreferredLocation:  LocationEdge,
		RequireGPU:         true,
		AllowCloudFallback: true,
		PreferredBackends:  []BackendKind{BackendTensorRT, BackendONNXRuntime, BackendDirectML, BackendCoreML},
	},
	FeatureFaceRecognition: {
		PreferredLocation:  LocationEdge,
		RequireGPU:         true,
		AllowCloudFallback: true,
		PreferredBackends:  []BackendKind{BackendTensorRT, BackendONNXRuntime, BackendCoreML, BackendDirectML},
	},
	FeatureLicensePlateRecognition: {
		PreferredLocation:  LocationEdge,
		RequireGPU:         false,
		AllowCloudFallback: true,
		PreferredBackends:  []BackendKind{BackendONNXRuntime, BackendCoreML, BackendDirectML, BackendTensorRT},
	},
	FeatureBehavioralAnalysis: {
		PreferredLocation:  LocationEdge,
		RequireGPU:         true,
		AllowCloudFallback: true,
		PreferredBackends:  []BackendKind{BackendTensorRT, BackendONNXRuntime},
	},
	FeatureAudioEventDetection: {
		PreferredLocation:  LocationEdge,
		RequireGPU:         false,
		AllowCloudFallback: true,
		PreferredBackends:  []BackendKind{BackendONNXRuntime, BackendCoreML, BackendDirectML},
	},
	FeatureCLIPEmbedding: {
		PreferredLocation:  LocationCloud,
		RequireGPU:         true,
		AllowCloudFallback: true,
		PreferredBackends:  []BackendKind{BackendTensorRT, BackendONNXRuntime},
	},
	FeatureForensicSearch: {
		PreferredLocation:  LocationCloud,
		RequireGPU:         true,
		AllowCloudFallback: true,
		PreferredBackends:  []BackendKind{BackendTensorRT, BackendONNXRuntime},
	},
}

// DefaultFeaturePolicies returns a copy of the built-in routing matrix.
// Callers that want to customise it should copy, mutate, then pass into
// NewRouter.
func DefaultFeaturePolicies() map[Feature]FeaturePolicy {
	out := make(map[Feature]FeaturePolicy, len(defaultFeatureRoutes))
	for k, v := range defaultFeatureRoutes {
		out[k] = v
	}
	return out
}

// Decision is the Router's answer for a single Pick call. It is logged
// verbatim so operators can understand why a request went where it did.
type Decision struct {
	Feature  Feature
	Location Location
	Backend  BackendKind
	// EdgeInferencer is non-nil iff Location == LocationEdge.
	EdgeInferencer Inferencer
	// Reason is a short human-readable explanation ("edge:preferred",
	// "cloud:fallback:no-gpu", "cloud:preferred", …).
	Reason string
}

// Router selects an Inferencer per request based on feature policy and
// caller-supplied hardware capability. It does not own the inferencers —
// callers register them and are responsible for their lifecycle.
type Router struct {
	mu          sync.RWMutex
	edgeByKind  map[BackendKind]Inferencer
	policies    map[Feature]FeaturePolicy
	logger      *slog.Logger
}

// RouterOption customises a Router at construction time.
type RouterOption func(*Router)

// WithFeaturePolicies overrides the built-in routing matrix. The supplied
// map is copied defensively.
func WithFeaturePolicies(p map[Feature]FeaturePolicy) RouterOption {
	return func(r *Router) {
		r.policies = make(map[Feature]FeaturePolicy, len(p))
		for k, v := range p {
			r.policies[k] = v
		}
	}
}

// WithLogger sets the slog.Logger used for decision logging. A nil logger
// disables decision logging.
func WithLogger(l *slog.Logger) RouterOption {
	return func(r *Router) {
		r.logger = l
	}
}

// NewRouter constructs a Router with the default feature policies.
func NewRouter(opts ...RouterOption) *Router {
	r := &Router{
		edgeByKind: make(map[BackendKind]Inferencer),
		policies:   DefaultFeaturePolicies(),
		logger:     slog.Default(),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// RegisterEdge adds an edge-local Inferencer to the router, keyed by its
// BackendKind. Registering twice with the same kind replaces the previous
// entry.
func (r *Router) RegisterEdge(inf Inferencer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.edgeByKind[inf.Backend()] = inf
}

// SetPolicy overrides the policy for a single feature at runtime. This is
// primarily used by tests; production deployments should pass a full map
// via WithFeaturePolicies.
func (r *Router) SetPolicy(f Feature, p FeaturePolicy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policies[f] = p
}

// Pick returns the routing Decision for a single inference request. The
// decision is logged via the router's slog.Logger (if any) at Debug level.
func (r *Router) Pick(ctx context.Context, feature Feature, hw HardwareCapability) (Decision, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	policy, ok := r.policies[feature]
	if !ok {
		return Decision{}, fmt.Errorf("%w: %s", ErrUnsupportedFeature, feature)
	}

	// Try edge first if that's the preference.
	if policy.PreferredLocation == LocationEdge {
		if policy.RequireGPU && !hw.HasGPU && !hw.HasNPU {
			if !policy.AllowCloudFallback {
				return Decision{}, fmt.Errorf("%w: %s needs GPU/NPU", ErrUnsupportedFeature, feature)
			}
			return r.cloudDecision(feature, "cloud:fallback:no-gpu"), nil
		}
		if inf, kind, ok := r.pickEdgeBackend(policy.PreferredBackends, hw); ok {
			dec := Decision{
				Feature:        feature,
				Location:       LocationEdge,
				Backend:        kind,
				EdgeInferencer: inf,
				Reason:         "edge:preferred",
			}
			r.log(ctx, dec)
			return dec, nil
		}
		if !policy.AllowCloudFallback {
			return Decision{}, fmt.Errorf("%w: %s no edge backend", ErrUnsupportedFeature, feature)
		}
		dec := r.cloudDecision(feature, "cloud:fallback:no-edge-backend")
		r.log(ctx, dec)
		return dec, nil
	}

	// Cloud preferred. Still allow opportunistic edge if the hardware
	// happens to support a preferred backend AND the feature doesn't
	// require GPU (to avoid bouncing expensive models to underpowered
	// edge boxes).
	if !policy.RequireGPU {
		if inf, kind, ok := r.pickEdgeBackend(policy.PreferredBackends, hw); ok {
			dec := Decision{
				Feature:        feature,
				Location:       LocationEdge,
				Backend:        kind,
				EdgeInferencer: inf,
				Reason:         "edge:opportunistic",
			}
			r.log(ctx, dec)
			return dec, nil
		}
	}
	dec := r.cloudDecision(feature, "cloud:preferred")
	r.log(ctx, dec)
	return dec, nil
}

// pickEdgeBackend walks the preferred-backends list and returns the first
// one the hardware supports AND that has a registered Inferencer. Caller
// must hold r.mu (read lock OK).
func (r *Router) pickEdgeBackend(prefs []BackendKind, hw HardwareCapability) (Inferencer, BackendKind, bool) {
	for _, kind := range prefs {
		if !hw.Supports(kind) {
			continue
		}
		if inf, ok := r.edgeByKind[kind]; ok {
			return inf, kind, true
		}
	}
	return nil, "", false
}

func (r *Router) cloudDecision(f Feature, reason string) Decision {
	return Decision{
		Feature:  f,
		Location: LocationCloud,
		Backend:  "", // cloud backend is selected by the cloud-side router
		Reason:   reason,
	}
}

func (r *Router) log(ctx context.Context, d Decision) {
	if r.logger == nil {
		return
	}
	r.logger.DebugContext(ctx, "inference routing decision",
		slog.String("feature", string(d.Feature)),
		slog.String("location", string(d.Location)),
		slog.String("backend", string(d.Backend)),
		slog.String("reason", d.Reason),
	)
}
