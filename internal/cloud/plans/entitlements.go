package plans

import (
	"fmt"
	"sort"
)

// Entitlements is the flattened view of what a tenant is allowed to do. It
// combines the feature flags included in a plan tier with those granted by
// every active add-on, plus any numeric extensions (e.g. retention days) the
// add-ons contribute.
//
// An Entitlements value is immutable after construction. Callers obtain one
// via Resolve and query it through the methods below; direct field access is
// permitted for callers that need to iterate (audit exporters, UIs) but
// should not mutate the slices.
type Entitlements struct {
	// Tier is the base plan tier these entitlements derive from.
	Tier Tier

	// Plan is the full canonical plan row the tier resolves to. Embedded for
	// the convenience of callers that want MaxCameras, CustomPricing, etc.
	// without a second lookup.
	Plan Plan

	// ActiveAddOns is the deterministic (ID-sorted) list of add-ons currently
	// active for this tenant. Each entry is the canonical AddOn row from the
	// catalog, not a reference; safe to read after Resolve returns.
	ActiveAddOns []AddOn

	// features is the combined feature set (tier + add-ons). Stored as a map
	// for O(1) HasFeature lookups; exposed as a sorted slice via Features.
	features map[FeatureFlag]struct{}

	// effectiveRetentionDays is the plan default plus any ExtendsRetention
	// contributions from active add-ons.
	effectiveRetentionDays int
}

// Resolve produces an Entitlements view for (tier, addOnIDs). It enforces:
//
//   - The tier must exist in the catalog (else ErrUnknownTier).
//   - Every add-on ID must exist in the catalog (else ErrUnknownAddOn).
//   - Every add-on's MinTier must be <= the requested tier (else
//     ErrAddOnRequiresHigherTier).
//
// Errors wrap the sentinel values in the plans package so callers can
// distinguish with errors.Is. Duplicate add-on IDs in the input are coalesced
// (activating the same add-on twice is a no-op, not an error — this makes
// callers that merge user selections with system-defaulted add-ons simpler).
func Resolve(tier Tier, addOnIDs []string) (*Entitlements, error) {
	plan, err := LookupPlan(tier)
	if err != nil {
		return nil, err
	}

	// De-duplicate the input while preserving the first-seen order only for
	// error-reporting purposes; the final ActiveAddOns list is re-sorted.
	seen := make(map[string]struct{}, len(addOnIDs))
	dedup := make([]string, 0, len(addOnIDs))
	for _, id := range addOnIDs {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		dedup = append(dedup, id)
	}

	features := make(map[FeatureFlag]struct{}, len(plan.IncludedFeatures)+len(dedup))
	for _, f := range plan.IncludedFeatures {
		features[f] = struct{}{}
	}

	active := make([]AddOn, 0, len(dedup))
	retention := plan.RetentionDays
	for _, id := range dedup {
		addon, lookupErr := LookupAddOn(id)
		if lookupErr != nil {
			return nil, lookupErr
		}
		if tier.Rank() < addon.MinTier.Rank() {
			return nil, fmt.Errorf("%w: add-on %q requires %q, tenant is on %q",
				ErrAddOnRequiresHigherTier, id, addon.MinTier, tier)
		}
		if addon.Feature != "" {
			features[addon.Feature] = struct{}{}
		}
		retention += addon.ExtendsRetention
		active = append(active, addon)
	}

	sort.Slice(active, func(i, j int) bool { return active[i].ID < active[j].ID })

	return &Entitlements{
		Tier:                   tier,
		Plan:                   plan,
		ActiveAddOns:           active,
		features:               features,
		effectiveRetentionDays: retention,
	}, nil
}

// HasFeature reports whether the given feature flag is unlocked for this
// tenant (either included in the tier or granted by an active add-on).
func (e *Entitlements) HasFeature(f FeatureFlag) bool {
	if e == nil {
		return false
	}
	_, ok := e.features[f]
	return ok
}

// Features returns the full feature set as a deterministic (lexicographically
// sorted) slice. Callers can safely mutate the returned slice.
func (e *Entitlements) Features() []FeatureFlag {
	if e == nil {
		return nil
	}
	out := make([]FeatureFlag, 0, len(e.features))
	for f := range e.features {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}

// MaxCameras is a convenience accessor that returns the camera cap from the
// underlying plan. Returns Unlimited (0) for tiers with no cap.
func (e *Entitlements) MaxCameras() int {
	if e == nil {
		return 0
	}
	return e.Plan.MaxCameras
}

// RetentionDays returns the effective cloud retention window for this tenant
// in days, which is the plan default plus any ExtendsRetention contributions
// from active add-ons.
func (e *Entitlements) RetentionDays() int {
	if e == nil {
		return 0
	}
	return e.effectiveRetentionDays
}

// MaxUsers returns the per-tenant user cap from the underlying plan. Returns
// Unlimited (0) for tiers with no cap.
func (e *Entitlements) MaxUsers() int {
	if e == nil {
		return 0
	}
	return e.Plan.MaxUsers
}

// HasAddOn reports whether the given add-on ID is currently active.
func (e *Entitlements) HasAddOn(id string) bool {
	if e == nil {
		return false
	}
	for _, a := range e.ActiveAddOns {
		if a.ID == id {
			return true
		}
	}
	return false
}
