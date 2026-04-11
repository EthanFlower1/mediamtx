package publicapi

// TenantTier identifies the rate-limit and feature tier for a tenant.
// Each tier has different rate limits and feature access.
type TenantTier string

const (
	// TierFree is the default tier for new/unverified tenants.
	TierFree TenantTier = "free"
	// TierStarter is the entry-level paid tier.
	TierStarter TenantTier = "starter"
	// TierPro is the mid-tier for growing deployments.
	TierPro TenantTier = "pro"
	// TierEnterprise is the top tier with highest limits.
	TierEnterprise TenantTier = "enterprise"
)

// TierLimits defines the rate limits for a tier.
type TierLimits struct {
	// RequestsPerSecond is the steady-state token refill rate.
	RequestsPerSecond float64
	// Burst is the maximum bucket capacity.
	Burst int
	// DailyQuota is the maximum requests per calendar day. 0 = unlimited.
	DailyQuota int64
}

// DefaultTierLimits returns the canonical rate limits for each tier.
// These are the contract values downstream tickets (KAI-400, integrations)
// depend on.
func DefaultTierLimits() map[TenantTier]TierLimits {
	return map[TenantTier]TierLimits{
		TierFree: {
			RequestsPerSecond: 1,
			Burst:             5,
			DailyQuota:        1000,
		},
		TierStarter: {
			RequestsPerSecond: 10,
			Burst:             20,
			DailyQuota:        50000,
		},
		TierPro: {
			RequestsPerSecond: 50,
			Burst:             100,
			DailyQuota:        500000,
		},
		TierEnterprise: {
			RequestsPerSecond: 200,
			Burst:             500,
			DailyQuota:        0, // unlimited
		},
	}
}

// LimitsForTier returns the limits for the given tier. Unknown tiers
// default to TierFree limits.
func LimitsForTier(tier TenantTier) TierLimits {
	defaults := DefaultTierLimits()
	if lim, ok := defaults[tier]; ok {
		return lim
	}
	return defaults[TierFree]
}
