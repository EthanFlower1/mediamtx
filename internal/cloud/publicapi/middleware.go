package publicapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// Middleware is the standard http middleware shape.
type Middleware func(http.Handler) http.Handler

// chain composes middlewares so the first argument runs first (outermost).
func chain(mws ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for i := len(mws) - 1; i >= 0; i-- {
			next = mws[i](next)
		}
		return next
	}
}

// APIVersionMiddleware injects the API version header on every response.
func APIVersionMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-API-Version", "v1")
			next.ServeHTTP(w, r)
		})
	}
}

// APIKeyAuthMiddleware authenticates requests via the X-API-Key header.
// If the header is present, it validates the key and attaches claims to
// the context. If the header is absent, it falls through to the next
// middleware (which should be the OAuth bearer token middleware).
//
// This middleware MUST run before the OAuth middleware so API keys take
// priority when both are present.
func APIKeyAuthMiddleware(store APIKeyStore) Middleware {
	if store == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get(APIKeyHeader)
			if apiKey == "" {
				// No API key; fall through to bearer token auth.
				next.ServeHTTP(w, r)
				return
			}

			key, err := store.Validate(r.Context(), apiKey)
			if err != nil {
				if errors.Is(err, ErrInvalidAPIKey) {
					writePublicError(w, http.StatusUnauthorized, "unauthenticated", "invalid API key")
					return
				}
				if errors.Is(err, ErrAPIKeyExpired) {
					writePublicError(w, http.StatusUnauthorized, "unauthenticated", "API key expired")
					return
				}
				if errors.Is(err, ErrAPIKeyRevoked) {
					writePublicError(w, http.StatusUnauthorized, "unauthenticated", "API key revoked")
					return
				}
				writePublicError(w, http.StatusInternalServerError, "internal", "API key validation error")
				return
			}

			if !key.IsActive() {
				writePublicError(w, http.StatusUnauthorized, "unauthenticated", "API key inactive")
				return
			}

			// Attach claims derived from the API key.
			claims := APIKeyToClaims(key)
			ctx := withPublicClaims(r.Context(), claims)
			ctx = withAPIKeyID(ctx, key.ID)
			ctx = withTenantTier(ctx, key.Tier)

			// Touch last-used timestamp (best-effort).
			go func() { _ = store.TouchLastUsed(r.Context(), key.ID) }()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OAuthAuthMiddleware authenticates requests via the Authorization: Bearer
// header using the standard IdentityProvider. This is the fallback when
// no X-API-Key header is present.
func OAuthAuthMiddleware(idp auth.IdentityProvider) Middleware {
	if idp == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If claims are already set (by API key middleware), skip.
			if _, ok := PublicClaimsFromContext(r.Context()); ok {
				next.ServeHTTP(w, r)
				return
			}

			authz := r.Header.Get("Authorization")
			if !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
				writePublicError(w, http.StatusUnauthorized, "unauthenticated",
					"missing authentication: provide X-API-Key or Authorization: Bearer <token>")
				return
			}

			token := strings.TrimSpace(authz[len("Bearer "):])
			claims, err := idp.VerifyToken(r.Context(), token)
			if err != nil || claims == nil {
				writePublicError(w, http.StatusUnauthorized, "unauthenticated", "invalid bearer token")
				return
			}

			ctx := withPublicClaims(r.Context(), claims)
			// Bearer token users default to their plan tier. The tier
			// resolver will be wired by KAI-400 to look up the actual
			// subscription; for now we default to TierStarter for
			// authenticated OAuth users.
			ctx = withTenantTier(ctx, TierStarter)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// TieredRateLimitMiddleware enforces per-tenant rate limits based on the
// tenant's tier. Must run after authentication so the tenant ID and tier
// are available in the context.
func TieredRateLimitMiddleware(rl *TieredRateLimiter) Middleware {
	if rl == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := PublicClaimsFromContext(r.Context())
			if !ok {
				// Unauthenticated: should have been caught by auth middleware.
				next.ServeHTTP(w, r)
				return
			}

			tenantID := claims.TenantRef.ID
			if tenantID == "" {
				tenantID = "__anon__"
			}

			tier, _ := TenantTierFromContext(r.Context())
			RateLimitHeaders(w, tier)

			if !rl.Allow(tenantID) {
				writePublicError(w, http.StatusTooManyRequests, "resource_exhausted",
					"rate limit exceeded for tier "+string(tier))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ScopeEnforcementMiddleware checks that an API key has the required scope
// for the requested endpoint. Bearer token users bypass this check (their
// access is controlled by Casbin permissions).
func ScopeEnforcementMiddleware(routes map[string]RouteAuth) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			keyID, hasKey := APIKeyIDFromContext(r.Context())
			if !hasKey || keyID == "" {
				// Not an API key request; skip scope check.
				next.ServeHTTP(w, r)
				return
			}

			route, ok := routes[r.URL.Path]
			if !ok {
				// No route auth entry; fall through to handler.
				next.ServeHTTP(w, r)
				return
			}

			claims, ok := PublicClaimsFromContext(r.Context())
			if !ok {
				writePublicError(w, http.StatusUnauthorized, "unauthenticated", "missing claims")
				return
			}

			// The claims.Groups field carries the API key scopes.
			scope := route.Resource + ":" + route.Action
			hasScope := len(claims.Groups) == 0 // empty = full access
			if !hasScope {
				for _, s := range claims.Groups {
					if string(s) == scope || string(s) == route.Resource+":*" {
						hasScope = true
						break
					}
				}
			}
			if !hasScope {
				writePublicError(w, http.StatusForbidden, "permission_denied",
					"API key lacks scope: "+scope)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
