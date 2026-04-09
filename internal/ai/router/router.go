package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

// Router decides where (edge or cloud) a given inference Feature should run
// for a given tenant.  Implementations MUST be safe for concurrent use.
type Router interface {
	// Route returns a Decision for (feature, tenantID).  The decision is
	// deterministic given the current hardware snapshot and tenant policy.
	// Route always emits a structured slog entry at INFO level.
	Route(ctx context.Context, feature Feature, tenantID string) (Decision, error)

	// Refresh re-probes the underlying HardwareProbe and updates the cached
	// HardwareCapability.  Call this on hardware-change events.
	Refresh(ctx context.Context) error

	// Hardware returns the cached HardwareCapability snapshot.
	Hardware() HardwareCapability

	// SetSaturated marks (or clears) GPU saturation.  When true, the router
	// will fall edge decisions back to cloud without restart.
	SetSaturated(saturated bool)

	// ReportEdgeFailure is called by the runtime when an edge inference
	// attempt fails.  Subsequent Route calls for the same Feature will return
	// a cloud Decision with Fallback=true until the failure is cleared by
	// ClearEdgeFailure.  This is the in-process side of the "no-restart
	// fallback" requirement.
	ReportEdgeFailure(feature Feature)

	// ClearEdgeFailure undoes a previous ReportEdgeFailure.
	ClearEdgeFailure(feature Feature)
}

// matrixRow declares the default routing rule for a single Feature.  The
// rules are evaluated by Router.Route in this order: tenant override ->
// matrix.Eval -> saturation/failure fallback.
type matrixRow struct {
	Feature Feature

	// Eval returns the desired Location and a short reason given the current
	// HardwareCapability.  Eval MUST be a pure function so the matrix is
	// trivially auditable.
	Eval func(hw HardwareCapability) (Location, string)
}

// defaultMatrix encodes § 11.2 of the v1 spec as one declarative table.  Add
// new features here, never as if/else branches inside Route.
var defaultMatrix = []matrixRow{
	{
		Feature: FeatureObjectDetectionLight,
		Eval: func(_ HardwareCapability) (Location, string) {
			return LocationEdge, "matrix: lightweight OD always runs at the edge"
		},
	},
	{
		Feature: FeatureObjectDetectionHeavy,
		Eval: func(hw HardwareCapability) (Location, string) {
			if hw.GPUPresent {
				return LocationEdge, "matrix: heavy OD on edge GPU"
			}
			return LocationCloud, "matrix: heavy OD requires GPU; none present"
		},
	},
	{
		Feature: FeatureFace,
		Eval: func(hw HardwareCapability) (Location, string) {
			if hw.GPUPresent {
				return LocationEdge, "matrix: face on edge GPU"
			}
			return LocationCloud, "matrix: face requires GPU; none present"
		},
	},
	{
		Feature: FeatureLPR,
		Eval: func(hw HardwareCapability) (Location, string) {
			if hw.GPUPresent {
				return LocationEdge, "matrix: LPR on edge GPU"
			}
			return LocationCloud, "matrix: LPR requires GPU; none present"
		},
	},
	{
		Feature: FeatureBehavioral,
		Eval: func(_ HardwareCapability) (Location, string) {
			return LocationEdge, "matrix: behavioral analytics always run at the edge"
		},
	},
	{
		Feature: FeatureAudio,
		Eval: func(_ HardwareCapability) (Location, string) {
			return LocationEdge, "matrix: audio analytics always run at the edge"
		},
	},
	{
		Feature: FeatureCLIPEmbed,
		Eval: func(_ HardwareCapability) (Location, string) {
			return LocationEdge, "matrix: CLIP embeddings computed at the edge"
		},
	},
	{
		Feature: FeatureVectorSearch,
		Eval: func(_ HardwareCapability) (Location, string) {
			return LocationCloud, "matrix: vector search runs in the cloud"
		},
	},
	{
		Feature: FeatureCrossCameraTrack,
		Eval: func(_ HardwareCapability) (Location, string) {
			return LocationCloud, "matrix: cross-camera tracking is cloud-only"
		},
	},
	{
		Feature: FeatureAnomaly,
		Eval: func(_ HardwareCapability) (Location, string) {
			return LocationCloud, "matrix: anomaly detection is cloud-only"
		},
	},
	{
		Feature: FeatureSummary,
		Eval: func(_ HardwareCapability) (Location, string) {
			return LocationCloud, "matrix: summarization is cloud-only"
		},
	},
	{
		Feature: FeatureForensic,
		Eval: func(_ HardwareCapability) (Location, string) {
			return LocationCloud, "matrix: forensic search is cloud-only"
		},
	},
	{
		Feature: FeatureCustom,
		Eval: func(_ HardwareCapability) (Location, string) {
			// Custom uploads default to cloud; tenants override per policy.
			return LocationCloud, "matrix: custom upload defaults to cloud (tenant choice)"
		},
	},
}

// matrixIndex is a Feature -> matrixRow lookup built once at init time.
var matrixIndex = func() map[Feature]matrixRow {
	m := make(map[Feature]matrixRow, len(defaultMatrix))
	for _, row := range defaultMatrix {
		m[row.Feature] = row
	}
	return m
}()

// DefaultMatrix returns a copy of the routing matrix for audit/test use.
func DefaultMatrix() []matrixRow {
	out := make([]matrixRow, len(defaultMatrix))
	copy(out, defaultMatrix)
	return out
}

// ErrUnknownFeature is returned by Route when a Feature is not in the matrix.
var ErrUnknownFeature = errors.New("router: unknown feature")

// Config bundles the dependencies of defaultRouter.
type Config struct {
	// Probe is the hardware probe.  Required.
	Probe HardwareProbe

	// Policies is the per-tenant override store.  If nil, an empty static
	// store is used (no overrides).
	Policies PolicyStore

	// Logger is the slog logger used for routing decisions.  If nil,
	// slog.Default() is used.
	Logger *slog.Logger
}

// defaultRouter is the production Router implementation.
type defaultRouter struct {
	probe    HardwareProbe
	policies PolicyStore
	logger   *slog.Logger

	mu          sync.RWMutex
	hardware    HardwareCapability
	failedEdge  map[Feature]bool
}

// New constructs a Router and performs the initial hardware probe.
func New(ctx context.Context, cfg Config) (Router, error) {
	if cfg.Probe == nil {
		return nil, errors.New("router: Config.Probe is required")
	}
	if cfg.Policies == nil {
		cfg.Policies = NewStaticPolicyStore(nil)
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	r := &defaultRouter{
		probe:      cfg.Probe,
		policies:   cfg.Policies,
		logger:     cfg.Logger.With("component", "ai/router"),
		failedEdge: make(map[Feature]bool),
	}
	if err := r.Refresh(ctx); err != nil {
		return nil, fmt.Errorf("initial hardware probe: %w", err)
	}
	return r, nil
}

// Refresh re-probes hardware and atomically updates the cached snapshot.
func (r *defaultRouter) Refresh(ctx context.Context) error {
	hw, err := r.probe.Probe(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.hardware = hw
	r.mu.Unlock()
	r.logger.Info("hardware probe refreshed", "hw_summary", hw.Summary())
	return nil
}

// Hardware returns the cached HardwareCapability.
func (r *defaultRouter) Hardware() HardwareCapability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.hardware
}

// SetSaturated toggles the GPU saturation flag in the cached snapshot.
func (r *defaultRouter) SetSaturated(saturated bool) {
	r.mu.Lock()
	r.hardware.GPUSaturated = saturated
	r.mu.Unlock()
}

// ReportEdgeFailure marks a feature as currently failing at the edge.
func (r *defaultRouter) ReportEdgeFailure(feature Feature) {
	r.mu.Lock()
	r.failedEdge[feature] = true
	r.mu.Unlock()
}

// ClearEdgeFailure clears a previous ReportEdgeFailure.
func (r *defaultRouter) ClearEdgeFailure(feature Feature) {
	r.mu.Lock()
	delete(r.failedEdge, feature)
	r.mu.Unlock()
}

// Route is the main entry point.  Precedence:
//
//  1. Tenant ForceCloud — always honoured.
//  2. Default matrix evaluated against current hardware.
//  3. If matrix says edge but tenant ForceEdge says edge, edge wins.
//  4. If the chosen Location is edge but the GPU is saturated OR the feature
//     has been reported as failing, fall back to cloud (Fallback=true).
//
// Tenant ForceEdge cannot override a cloud-only matrix row (e.g. forensic);
// the matrix is the source of truth for what is even possible at the edge.
func (r *defaultRouter) Route(ctx context.Context, feature Feature, tenantID string) (Decision, error) {
	row, ok := matrixIndex[feature]
	if !ok {
		return Decision{}, fmt.Errorf("%w: %q", ErrUnknownFeature, feature)
	}

	r.mu.RLock()
	hw := r.hardware
	failed := r.failedEdge[feature]
	r.mu.RUnlock()

	policy, err := r.policies.GetPolicy(tenantID)
	if err != nil {
		return Decision{}, fmt.Errorf("router: load tenant policy: %w", err)
	}

	matrixLoc, matrixReason := row.Eval(hw)

	decision := Decision{
		Feature:  feature,
		TenantID: tenantID,
		Location: matrixLoc,
		Reason:   matrixReason,
	}

	// Tenant ForceCloud short-circuits everything.
	if policy.ForceCloud[feature] {
		decision.Location = LocationCloud
		decision.Reason = "tenant policy: force_cloud"
		decision.Fallback = false
		r.emit(ctx, decision, hw)
		return decision, nil
	}

	// Tenant ForceEdge upgrades a matrix-cloud decision to edge ONLY if the
	// matrix would otherwise allow edge under different hardware.  Cloud-only
	// matrix rows (vector_search, cross_camera_track, anomaly, summary,
	// forensic) cannot be overridden — see canRunOnEdge.
	if policy.ForceEdge[feature] && matrixLoc == LocationCloud && canRunOnEdge(feature) {
		decision.Location = LocationEdge
		decision.Reason = "tenant policy: force_edge"
	}

	// Saturation/failure fallback applies to any edge decision.
	if decision.Location == LocationEdge && (hw.GPUSaturated || failed) {
		reason := "fallback: GPU saturated"
		if failed {
			reason = "fallback: edge inference reported failure"
		}
		decision.Location = LocationCloud
		decision.Reason = reason
		decision.Fallback = true
	}

	r.emit(ctx, decision, hw)
	return decision, nil
}

// canRunOnEdge returns true if a Feature is ever eligible for edge execution
// under any hardware configuration.  This guards tenant ForceEdge against
// cloud-only matrix rows.
func canRunOnEdge(feature Feature) bool {
	switch feature {
	case FeatureVectorSearch,
		FeatureCrossCameraTrack,
		FeatureAnomaly,
		FeatureSummary,
		FeatureForensic:
		return false
	default:
		return true
	}
}

// emit writes the structured log entry mandated by the spec.
func (r *defaultRouter) emit(ctx context.Context, d Decision, hw HardwareCapability) {
	r.logger.LogAttrs(ctx, slog.LevelInfo, "inference routing decision",
		slog.String("feature", string(d.Feature)),
		slog.String("tenant_id", d.TenantID),
		slog.String("decision", string(d.Location)),
		slog.String("reason", d.Reason),
		slog.Bool("fallback", d.Fallback),
		slog.String("hw_summary", hw.Summary()),
	)
}
