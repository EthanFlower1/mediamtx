package publicapi

import (
	"testing"
	"time"
)

func TestTieredRateLimiterDefaultTier(t *testing.T) {
	rl := NewTieredRateLimiter()
	// No tier resolver; defaults to TierFree (burst=5).
	for i := 0; i < 5; i++ {
		if !rl.Allow("tenant-1") {
			t.Fatalf("request %d rejected; want allowed (burst=5)", i)
		}
	}
	if rl.Allow("tenant-1") {
		t.Error("6th request allowed; want rejected")
	}
}

func TestTieredRateLimiterEnterpriseTier(t *testing.T) {
	rl := NewTieredRateLimiter()
	rl.TierResolver = func(string) TenantTier { return TierEnterprise }

	// Enterprise burst=500; first 500 should all pass.
	for i := 0; i < 500; i++ {
		if !rl.Allow("enterprise-tenant") {
			t.Fatalf("request %d rejected; want allowed (enterprise burst=500)", i)
		}
	}
	if rl.Allow("enterprise-tenant") {
		t.Error("501st request allowed; want rejected")
	}
}

func TestTieredRateLimiterSeparateTenants(t *testing.T) {
	rl := NewTieredRateLimiter()
	// Each tenant gets their own bucket.
	for i := 0; i < 5; i++ {
		if !rl.Allow("tenant-a") {
			t.Fatalf("tenant-a request %d rejected", i)
		}
	}
	// tenant-b should still have its full burst.
	if !rl.Allow("tenant-b") {
		t.Error("tenant-b first request rejected; should have its own bucket")
	}
}

func TestTieredRateLimiterDailyQuota(t *testing.T) {
	rl := NewTieredRateLimiter()
	// Override the internal limits to make daily quota testable.
	rl.tiers = map[TenantTier]TierLimits{
		TierFree: {
			RequestsPerSecond: 1000, // high RPS so bucket never blocks
			Burst:             1000,
			DailyQuota:        3,    // very low daily quota
		},
	}

	for i := 0; i < 3; i++ {
		if !rl.Allow("quota-tenant") {
			t.Fatalf("request %d rejected; should be within daily quota", i)
		}
	}
	if rl.Allow("quota-tenant") {
		t.Error("4th request allowed; should exceed daily quota of 3")
	}
}

func TestTieredRateLimiterRefill(t *testing.T) {
	now := time.Now()
	rl := NewTieredRateLimiter()
	rl.now = func() time.Time { return now }
	rl.tiers = map[TenantTier]TierLimits{
		TierFree: {RequestsPerSecond: 1, Burst: 1, DailyQuota: 0},
	}

	// Drain the bucket.
	if !rl.Allow("refill-tenant") {
		t.Fatal("first request rejected")
	}
	if rl.Allow("refill-tenant") {
		t.Fatal("second request should be rejected")
	}

	// Advance time by 2 seconds; at 1 RPS that refills 2 tokens (capped at burst=1).
	now = now.Add(2 * time.Second)
	if !rl.Allow("refill-tenant") {
		t.Error("request after 2s should be allowed (token refilled)")
	}
}

func TestLimitsForTierUnknown(t *testing.T) {
	lim := LimitsForTier("nonexistent")
	free := DefaultTierLimits()[TierFree]
	if lim.Burst != free.Burst {
		t.Errorf("unknown tier burst = %d; want %d (free default)", lim.Burst, free.Burst)
	}
}

func TestTierLimitsContract(t *testing.T) {
	// Contract assertion: tier limits must satisfy the ordering constraint.
	tiers := DefaultTierLimits()
	if tiers[TierFree].Burst >= tiers[TierStarter].Burst {
		t.Error("free burst should be less than starter burst")
	}
	if tiers[TierStarter].Burst >= tiers[TierPro].Burst {
		t.Error("starter burst should be less than pro burst")
	}
	if tiers[TierPro].Burst >= tiers[TierEnterprise].Burst {
		t.Error("pro burst should be less than enterprise burst")
	}
}
