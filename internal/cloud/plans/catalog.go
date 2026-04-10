package plans

import (
	"errors"
	"fmt"
	"sort"
)

// Tier identifies one of the four v1 plan tiers. Values are stable strings so
// they are safe to persist (billing snapshots, audit log) and to expose over
// JSON APIs.
type Tier string

// Tier constants. Order in source = order of escalation (Free is the cheapest,
// Enterprise the most expensive). Do not renumber casually — downstream
// snapshot tests assert against these literals.
const (
	TierFree       Tier = "free"
	TierStarter    Tier = "starter"
	TierPro        Tier = "professional"
	TierEnterprise Tier = "enterprise"
)

// allTiersInOrder is the canonical low-to-high ordering used for monotonic
// pricing assertions and rank comparisons.
var allTiersInOrder = []Tier{TierFree, TierStarter, TierPro, TierEnterprise}

// Rank returns the 0-indexed position of t in the canonical order. Returns -1
// for unknown tiers. Used by the entitlements resolver to enforce
// tier-compatibility rules ("add-on requires >= Starter").
func (t Tier) Rank() int {
	for i, x := range allTiersInOrder {
		if x == t {
			return i
		}
	}
	return -1
}

// Unlimited is the sentinel used in any "max N" field to mean "no cap".
const Unlimited = 0

// Plan is the immutable description of a single tier in the catalog.
type Plan struct {
	// Tier is the stable identifier (also the catalog key).
	Tier Tier
	// DisplayName is a human-readable name suitable for UIs and invoices.
	DisplayName string
	// MaxCameras is the per-tenant camera cap. Unlimited (0) means no cap.
	MaxCameras int
	// RetentionDays is the default cloud retention window for recordings.
	// May be extended at runtime by the cloud_archive_extended add-on.
	RetentionDays int
	// RetailPricePerCameraCents is the public list price (USD cents) per
	// camera per month. Zero for the Free tier; the Enterprise tier carries
	// the floor price ("$45+") and is treated as a starting point for custom
	// quotes.
	RetailPricePerCameraCents int64
	// WholesalePricePerCameraCents is the integrator/partner price (USD cents)
	// per camera per month. Must be <= retail. Zero for Free.
	WholesalePricePerCameraCents int64
	// IncludedFeatures is the deterministic list of feature flags this tier
	// unlocks at no extra cost. Sorted at construction time so callers can
	// rely on stable ordering.
	IncludedFeatures []FeatureFlag
	// MinUsers is the minimum number of seats provisioned with the tier.
	MinUsers int
	// MaxUsers is the seat cap. Unlimited (0) means no cap.
	MaxUsers int
	// CustomPricing is true for tiers whose published price is a starting
	// point for negotiation rather than the final invoice (Enterprise).
	CustomPricing bool
}

// PriceUnit describes how an add-on is billed. Stable strings so they survive
// JSON round-tripping.
type PriceUnit string

const (
	// PriceUnitPerCameraMonth: charged per active camera per month.
	PriceUnitPerCameraMonth PriceUnit = "per_camera_month"
	// PriceUnitPerTenantMonth: flat per-tenant per month.
	PriceUnitPerTenantMonth PriceUnit = "per_tenant_month"
	// PriceUnitPerGBMonth: storage-style metered, charged per GB per month.
	PriceUnitPerGBMonth PriceUnit = "per_gb_month"
	// PriceUnitPerGPUHour: GPU compute, charged per GPU-hour consumed.
	PriceUnitPerGPUHour PriceUnit = "per_gpu_hour"
)

// AddOn is a boolean entitlement that may be layered on top of a plan tier
// for an additional charge. Pricing is intentionally optional: some add-ons
// (e.g. federation_unlimited bundled into Enterprise) may resolve to a zero
// price for certain tiers but the entitlement still flows through Resolve.
type AddOn struct {
	// ID is the stable lookup key.
	ID string
	// DisplayName is human-readable.
	DisplayName string
	// Feature is the feature flag this add-on grants when active.
	Feature FeatureFlag
	// PriceCents is the unit price in USD cents. May be zero for grandfathered
	// or bundled cases.
	PriceCents int64
	// Unit describes how PriceCents is metered.
	Unit PriceUnit
	// MinTier is the lowest tier on which this add-on may be activated.
	// A Resolve call against a lower tier returns ErrAddOnRequiresHigherTier.
	MinTier Tier
	// ExtendsRetention, if non-zero, is the additional number of retention
	// days this add-on grants. Used by Entitlements.RetentionDays so a single
	// add-on can express both a feature flag and a numeric extension.
	ExtendsRetention int
}

// Sentinel errors. Callers use errors.Is to distinguish them.
var (
	// ErrUnknownTier is returned by LookupPlan for an unrecognised tier value.
	ErrUnknownTier = errors.New("plans: unknown tier")
	// ErrUnknownAddOn is returned by LookupAddOn / Resolve for an unrecognised
	// add-on ID.
	ErrUnknownAddOn = errors.New("plans: unknown add-on")
	// ErrAddOnRequiresHigherTier is returned by Resolve when an add-on is
	// activated against a tier below its MinTier.
	ErrAddOnRequiresHigherTier = errors.New("plans: add-on requires a higher plan tier")
)

// catalog is the in-memory plan table. It is built once at package init and
// is read-only thereafter. Tests assert that the catalog satisfies the
// well-formedness invariants in validateCatalog.
var catalog map[Tier]Plan

// addOnCatalog is the in-memory add-on table, keyed by AddOn.ID.
var addOnCatalog map[string]AddOn

// Add-on ID constants. Stable strings — persisted in billing/Stripe
// metadata, so changing a value is a breaking change.
const (
	AddOnCloudArchiveExtended  = "cloud_archive_extended"
	AddOnFaceRecognition       = "face_recognition"
	AddOnLPR                   = "lpr"
	AddOnBehavioralAnalytics   = "behavioral_analytics"
	AddOnCustomAIModelUpload   = "custom_ai_model_upload"
	AddOnFederationUnlimited   = "federation_unlimited"
	AddOnPrioritySupport       = "priority_support"
	AddOnDedicatedInferencePool = "dedicated_inference_pool"
)

func init() {
	catalog = buildPlanCatalog()
	addOnCatalog = buildAddOnCatalog()
	if err := validateCatalog(catalog, addOnCatalog); err != nil {
		// Catalog data is compile-time-constant; an invalid catalog is a bug
		// the build must fail loud-and-early on.
		panic(fmt.Sprintf("plans: catalog invariant violated: %v", err))
	}
}

// buildPlanCatalog returns the canonical plan map. Source of truth: the
// pricing table approved on KAI-363.
func buildPlanCatalog() map[Tier]Plan {
	plans := []Plan{
		{
			Tier:                         TierFree,
			DisplayName:                  "Free",
			MaxCameras:                   4,
			RetentionDays:                7,
			RetailPricePerCameraCents:    0,
			WholesalePricePerCameraCents: 0,
			IncludedFeatures: []FeatureFlag{
				FeatureBasicDetection,
			},
			MinUsers: 1,
			MaxUsers: 1,
		},
		{
			Tier:                         TierStarter,
			DisplayName:                  "Starter",
			MaxCameras:                   32,
			RetentionDays:                30,
			RetailPricePerCameraCents:    1500, // $15.00 retail per camera/month
			WholesalePricePerCameraCents: 900,  // $9.00 wholesale per camera/month
			IncludedFeatures: []FeatureFlag{
				FeatureBasicDetection,
				FeatureFullObjectDetection,
				FeatureSSO,
				FeatureUnlimitedUsers,
			},
			MinUsers: 1,
			MaxUsers: Unlimited,
		},
		{
			Tier:                         TierPro,
			DisplayName:                  "Professional",
			MaxCameras:                   256,
			RetentionDays:                90,
			RetailPricePerCameraCents:    3000, // $30.00 retail per camera/month
			WholesalePricePerCameraCents: 1800, // $18.00 wholesale per camera/month
			IncludedFeatures: []FeatureFlag{
				FeatureBasicDetection,
				FeatureFullObjectDetection,
				FeatureSSO,
				FeatureUnlimitedUsers,
				FeatureFaceRecognition,
				FeatureLPR,
				FeatureBehavioralAnalytics,
				FeatureFederation,
				FeatureIntegrations,
			},
			MinUsers: 1,
			MaxUsers: Unlimited,
		},
		{
			Tier:                         TierEnterprise,
			DisplayName:                  "Enterprise",
			MaxCameras:                   Unlimited,
			RetentionDays:                365, // default starting point; custom quotes may extend
			RetailPricePerCameraCents:    4500, // $45.00 floor; "$45+" custom
			WholesalePricePerCameraCents: 2700, // 60% of retail floor; partners negotiate
			IncludedFeatures: []FeatureFlag{
				FeatureBasicDetection,
				FeatureFullObjectDetection,
				FeatureSSO,
				FeatureUnlimitedUsers,
				FeatureFaceRecognition,
				FeatureLPR,
				FeatureBehavioralAnalytics,
				FeatureFederation,
				FeatureFederationUnlimited,
				FeatureIntegrations,
				FeatureCustomAIModelUpload,
				FeaturePrioritySupport,
				FeatureDedicatedInferencePool,
				FeatureFedRAMP,
				FeatureOnPremDeployment,
			},
			MinUsers:      1,
			MaxUsers:      Unlimited,
			CustomPricing: true,
		},
	}

	out := make(map[Tier]Plan, len(plans))
	for _, p := range plans {
		// Sort included features for deterministic output.
		sortFeatures(p.IncludedFeatures)
		out[p.Tier] = p
	}
	return out
}

// buildAddOnCatalog returns the canonical add-on map.
func buildAddOnCatalog() map[string]AddOn {
	addons := []AddOn{
		{
			ID:               AddOnCloudArchiveExtended,
			DisplayName:      "Cloud Archive (Extended Retention)",
			Feature:          FeatureCloudArchiveExtended,
			PriceCents:       5, // $0.05 per GB-month
			Unit:             PriceUnitPerGBMonth,
			MinTier:          TierStarter,
			ExtendsRetention: 365, // grants up to one extra year on top of the plan default
		},
		{
			ID:          AddOnFaceRecognition,
			DisplayName: "Face Recognition",
			Feature:     FeatureFaceRecognition,
			PriceCents:  500, // $5.00 per camera/month
			Unit:        PriceUnitPerCameraMonth,
			MinTier:     TierStarter,
		},
		{
			ID:          AddOnLPR,
			DisplayName: "License Plate Recognition",
			Feature:     FeatureLPR,
			PriceCents:  500, // $5.00 per camera/month
			Unit:        PriceUnitPerCameraMonth,
			MinTier:     TierStarter,
		},
		{
			ID:          AddOnBehavioralAnalytics,
			DisplayName: "Behavioral Analytics",
			Feature:     FeatureBehavioralAnalytics,
			PriceCents:  700, // $7.00 per camera/month
			Unit:        PriceUnitPerCameraMonth,
			MinTier:     TierStarter,
		},
		{
			ID:          AddOnCustomAIModelUpload,
			DisplayName: "Custom AI Model Upload",
			Feature:     FeatureCustomAIModelUpload,
			PriceCents:  25000, // $250.00 per tenant/month
			Unit:        PriceUnitPerTenantMonth,
			MinTier:     TierPro,
		},
		{
			ID:          AddOnFederationUnlimited,
			DisplayName: "Unlimited Federation",
			Feature:     FeatureFederationUnlimited,
			PriceCents:  20000, // $200.00 per tenant/month
			Unit:        PriceUnitPerTenantMonth,
			MinTier:     TierPro,
		},
		{
			ID:          AddOnPrioritySupport,
			DisplayName: "Priority Support",
			Feature:     FeaturePrioritySupport,
			PriceCents:  50000, // $500.00 per tenant/month
			Unit:        PriceUnitPerTenantMonth,
			MinTier:     TierStarter,
		},
		{
			ID:          AddOnDedicatedInferencePool,
			DisplayName: "Dedicated GPU Inference Pool",
			Feature:     FeatureDedicatedInferencePool,
			PriceCents:  250, // $2.50 per GPU-hour
			Unit:        PriceUnitPerGPUHour,
			MinTier:     TierPro,
		},
	}

	out := make(map[string]AddOn, len(addons))
	for _, a := range addons {
		out[a.ID] = a
	}
	return out
}

// validateCatalog enforces the structural invariants the rest of the package
// (and the rest of the codebase) is allowed to assume. Called from init.
func validateCatalog(plans map[Tier]Plan, addOns map[string]AddOn) error {
	// Every tier in canonical order must have an entry.
	for _, t := range allTiersInOrder {
		if _, ok := plans[t]; !ok {
			return fmt.Errorf("missing plan for tier %q", t)
		}
	}
	// No extras.
	if len(plans) != len(allTiersInOrder) {
		return fmt.Errorf("catalog has %d plans, expected %d", len(plans), len(allTiersInOrder))
	}

	// Tier field of each entry must match its key.
	for k, p := range plans {
		if p.Tier != k {
			return fmt.Errorf("plan key %q has mismatched Tier field %q", k, p.Tier)
		}
		if p.DisplayName == "" {
			return fmt.Errorf("plan %q has empty DisplayName", k)
		}
		if p.RetailPricePerCameraCents < 0 || p.WholesalePricePerCameraCents < 0 {
			return fmt.Errorf("plan %q has negative price", k)
		}
		if p.WholesalePricePerCameraCents > p.RetailPricePerCameraCents {
			return fmt.Errorf("plan %q wholesale > retail", k)
		}
	}

	// Retail price must be monotonically non-decreasing along the canonical
	// order. (Free=0, Starter, Pro, Enterprise floor.)
	var prev int64 = -1
	for _, t := range allTiersInOrder {
		p := plans[t]
		if p.RetailPricePerCameraCents < prev {
			return fmt.Errorf("retail price not monotonic at tier %q (%d < %d)", t, p.RetailPricePerCameraCents, prev)
		}
		prev = p.RetailPricePerCameraCents
	}

	// Camera caps must be monotonic non-decreasing, treating Unlimited as +∞.
	prevCams := -1
	for _, t := range allTiersInOrder {
		p := plans[t]
		eff := p.MaxCameras
		if eff == Unlimited {
			eff = 1<<31 - 1
		}
		if eff < prevCams {
			return fmt.Errorf("camera cap not monotonic at tier %q", t)
		}
		prevCams = eff
	}

	// Add-on validation.
	for id, a := range addOns {
		if a.ID != id {
			return fmt.Errorf("add-on key %q has mismatched ID field %q", id, a.ID)
		}
		if a.DisplayName == "" {
			return fmt.Errorf("add-on %q has empty DisplayName", id)
		}
		if a.Feature == "" {
			return fmt.Errorf("add-on %q has empty Feature", id)
		}
		if a.PriceCents < 0 {
			return fmt.Errorf("add-on %q has negative price", id)
		}
		if a.Unit == "" {
			return fmt.Errorf("add-on %q has empty Unit", id)
		}
		if a.MinTier.Rank() < 0 {
			return fmt.Errorf("add-on %q has unknown MinTier %q", id, a.MinTier)
		}
	}
	return nil
}

// LookupPlan returns the canonical plan definition for the given tier.
// Returns ErrUnknownTier wrapped with the offending value if t is not in the
// catalog.
func LookupPlan(t Tier) (Plan, error) {
	p, ok := catalog[t]
	if !ok {
		return Plan{}, fmt.Errorf("%w: %q", ErrUnknownTier, t)
	}
	return p, nil
}

// LookupAddOn returns the canonical add-on definition for the given ID.
// Returns ErrUnknownAddOn wrapped with the offending value if id is not in
// the catalog.
func LookupAddOn(id string) (AddOn, error) {
	a, ok := addOnCatalog[id]
	if !ok {
		return AddOn{}, fmt.Errorf("%w: %q", ErrUnknownAddOn, id)
	}
	return a, nil
}

// AllPlans returns every plan in the canonical low-to-high tier order.
// Safe to mutate the returned slice — it is freshly allocated.
func AllPlans() []Plan {
	out := make([]Plan, 0, len(allTiersInOrder))
	for _, t := range allTiersInOrder {
		out = append(out, catalog[t])
	}
	return out
}

// AllAddOns returns every add-on sorted by ID. Safe to mutate the returned
// slice — it is freshly allocated.
func AllAddOns() []AddOn {
	out := make([]AddOn, 0, len(addOnCatalog))
	for _, a := range addOnCatalog {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// sortFeatures sorts a feature flag slice in place lexicographically. Used at
// catalog construction time so callers see deterministic output.
func sortFeatures(f []FeatureFlag) {
	sort.Slice(f, func(i, j int) bool { return string(f[i]) < string(f[j]) })
}
