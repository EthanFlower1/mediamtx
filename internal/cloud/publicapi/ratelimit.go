package publicapi

import (
	"net/http"
	"sync"
	"time"
)

// TieredRateLimiter implements per-tenant rate limiting with tier-based
// capacity. Each tenant gets a token bucket sized according to their tier.
type TieredRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tieredBucket
	tiers   map[TenantTier]TierLimits
	now     func() time.Time

	// TierResolver looks up a tenant's tier. If nil, all tenants get
	// TierFree limits. KAI-400 will wire the real resolver.
	TierResolver func(tenantID string) TenantTier
}

type tieredBucket struct {
	tokens   float64
	capacity float64
	rate     float64
	last     time.Time
	daily    int64
	dayStart time.Time
	dayQuota int64
}

func (b *tieredBucket) allow(now time.Time) bool {
	// Check daily quota first.
	if b.dayQuota > 0 {
		if now.Day() != b.dayStart.Day() || now.Month() != b.dayStart.Month() {
			b.daily = 0
			b.dayStart = now
		}
		if b.daily >= b.dayQuota {
			return false
		}
	}

	// Token bucket refill.
	if !b.last.IsZero() {
		elapsed := now.Sub(b.last).Seconds()
		b.tokens += elapsed * b.rate
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
	}
	b.last = now

	if b.tokens >= 1 {
		b.tokens--
		if b.dayQuota > 0 {
			b.daily++
		}
		return true
	}
	return false
}

// NewTieredRateLimiter creates a rate limiter with default tier limits.
func NewTieredRateLimiter() *TieredRateLimiter {
	return &TieredRateLimiter{
		buckets: make(map[string]*tieredBucket),
		tiers:   DefaultTierLimits(),
		now:     time.Now,
	}
}

// Allow checks whether the given tenant is allowed to make a request.
func (rl *TieredRateLimiter) Allow(tenantID string) bool {
	if rl == nil {
		return true
	}

	tier := TierFree
	if rl.TierResolver != nil {
		tier = rl.TierResolver(tenantID)
	}

	limits, ok := rl.tiers[tier]
	if !ok {
		limits = rl.tiers[TierFree]
	}

	rl.mu.Lock()
	b, ok := rl.buckets[tenantID]
	if !ok {
		now := rl.now()
		b = &tieredBucket{
			tokens:   float64(limits.Burst),
			capacity: float64(limits.Burst),
			rate:     limits.RequestsPerSecond,
			dayStart: now,
			dayQuota: limits.DailyQuota,
		}
		rl.buckets[tenantID] = b
	}
	rl.mu.Unlock()

	return b.allow(rl.now())
}

// TierForTenant returns the tier for a tenant. Exported for tests.
func (rl *TieredRateLimiter) TierForTenant(tenantID string) TenantTier {
	if rl.TierResolver != nil {
		return rl.TierResolver(tenantID)
	}
	return TierFree
}

// RateLimitHeaders writes standard rate limit headers on the response.
func RateLimitHeaders(w http.ResponseWriter, tier TenantTier) {
	limits := LimitsForTier(tier)
	w.Header().Set("X-RateLimit-Limit", formatFloat(limits.RequestsPerSecond))
	w.Header().Set("X-RateLimit-Tier", string(tier))
}

func formatFloat(f float64) string {
	if f == float64(int(f)) {
		return itoa(int(f))
	}
	// Simple 1-decimal formatting.
	whole := int(f)
	frac := int((f - float64(whole)) * 10)
	return itoa(whole) + "." + itoa(frac)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 10)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	if neg {
		digits = append(digits, '-')
	}
	// Reverse.
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}
