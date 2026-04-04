package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// SecurityConfig holds configuration for network security middleware.
type SecurityConfig struct {
	// CORS
	CORSAllowedOrigins []string `json:"corsAllowedOrigins"` // e.g. ["https://admin.example.com"]
	CORSAllowedMethods []string `json:"corsAllowedMethods"` // defaults to GET,POST,PUT,DELETE,OPTIONS
	CORSAllowedHeaders []string `json:"corsAllowedHeaders"` // defaults to standard set
	CORSMaxAge         int      `json:"corsMaxAge"`         // preflight cache seconds, default 3600

	// CSP
	ContentSecurityPolicy string `json:"contentSecurityPolicy"` // override CSP header value
	FrameOptions          string `json:"frameOptions"`          // X-Frame-Options, default DENY

	// Rate limiting
	RateLimitEnabled    bool    `json:"rateLimitEnabled"`    // enable global rate limiting
	RateLimitPerSecond  float64 `json:"rateLimitPerSecond"`  // requests per second per IP (default 20)
	RateLimitBurst      int     `json:"rateLimitBurst"`      // burst capacity per IP (default 40)
	RateLimitCleanupSec int     `json:"rateLimitCleanupSec"` // stale entry cleanup interval (default 300)
}

// DefaultSecurityConfig returns a SecurityConfig with sensible defaults.
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		CORSAllowedOrigins: []string{}, // empty = reflect request origin for same-host
		CORSAllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		CORSAllowedHeaders: []string{"Authorization", "Content-Type", "X-Requested-With", "Accept"},
		CORSMaxAge:         3600,

		FrameOptions: "DENY",

		RateLimitEnabled:    true,
		RateLimitPerSecond:  20,
		RateLimitBurst:      40,
		RateLimitCleanupSec: 300,
	}
}

// ----- CORS Middleware -----

// CORSMiddleware returns a gin middleware that handles Cross-Origin Resource Sharing.
func CORSMiddleware(cfg SecurityConfig) gin.HandlerFunc {
	allowedOrigins := make(map[string]struct{}, len(cfg.CORSAllowedOrigins))
	for _, o := range cfg.CORSAllowedOrigins {
		allowedOrigins[o] = struct{}{}
	}

	methods := strings.Join(cfg.CORSAllowedMethods, ", ")
	headers := strings.Join(cfg.CORSAllowedHeaders, ", ")
	maxAge := "3600"
	if cfg.CORSMaxAge > 0 {
		maxAge = intToStr(cfg.CORSMaxAge)
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		allowed := false
		if len(allowedOrigins) == 0 {
			// No explicit list: allow same-host origins (any port).
			reqHost := c.Request.Host
			if idx := strings.LastIndex(reqHost, ":"); idx >= 0 {
				reqHost = reqHost[:idx]
			}
			if strings.Contains(origin, reqHost) {
				allowed = true
			}
		} else {
			_, allowed = allowedOrigins[origin]
		}

		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", methods)
			c.Header("Access-Control-Allow-Headers", headers)
			c.Header("Access-Control-Max-Age", maxAge)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Vary", "Origin")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// ----- CSP / Security Headers Middleware -----

// SecurityHeadersMiddleware adds Content-Security-Policy, X-Frame-Options,
// X-Content-Type-Options, and other security headers to every response.
func SecurityHeadersMiddleware(cfg SecurityConfig) gin.HandlerFunc {
	csp := cfg.ContentSecurityPolicy
	if csp == "" {
		csp = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'self' ws: wss:; font-src 'self'; frame-ancestors 'none'"
	}

	frameOptions := cfg.FrameOptions
	if frameOptions == "" {
		frameOptions = "DENY"
	}

	return func(c *gin.Context) {
		c.Header("Content-Security-Policy", csp)
		c.Header("X-Frame-Options", frameOptions)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Next()
	}
}

// ----- Token Bucket Rate Limiter -----

// ipBucket tracks the token state for a single IP.
type ipBucket struct {
	tokens     float64
	lastRefill time.Time
}

// RateLimiter is a per-IP token bucket rate limiter.
type RateLimiter struct {
	mu          sync.Mutex
	buckets     map[string]*ipBucket
	rate        float64 // tokens per second
	burst       int     // max tokens
	cleanupSec  int
	stopCleanup chan struct{}
}

// NewRateLimiter creates a rate limiter with the given per-second rate and burst.
func NewRateLimiter(rate float64, burst int, cleanupSec int) *RateLimiter {
	if rate <= 0 {
		rate = 20
	}
	if burst <= 0 {
		burst = 40
	}
	if cleanupSec <= 0 {
		cleanupSec = 300
	}
	rl := &RateLimiter{
		buckets:     make(map[string]*ipBucket),
		rate:        rate,
		burst:       burst,
		cleanupSec:  cleanupSec,
		stopCleanup: make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// Allow checks if the request from the given IP should be allowed.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[ip]
	if !ok {
		b = &ipBucket{tokens: float64(rl.burst), lastRefill: now}
		rl.buckets[ip] = b
	}

	// Refill tokens based on elapsed time.
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > float64(rl.burst) {
		b.tokens = float64(rl.burst)
	}
	b.lastRefill = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Stop shuts down the background cleanup goroutine.
func (rl *RateLimiter) Stop() {
	close(rl.stopCleanup)
}

// cleanup removes stale IP entries periodically.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Duration(rl.cleanupSec) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stopCleanup:
			return
		case <-ticker.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-time.Duration(rl.cleanupSec) * time.Second)
			for ip, b := range rl.buckets {
				if b.lastRefill.Before(cutoff) {
					delete(rl.buckets, ip)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// RateLimitMiddleware returns a gin middleware that applies per-IP rate limiting.
func RateLimitMiddleware(rl *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := clientIP(c)
		if !rl.Allow(ip) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
			})
			return
		}
		c.Next()
	}
}

// clientIP extracts the client IP, stripping port if present.
func clientIP(c *gin.Context) string {
	ip := c.ClientIP()
	// Strip port if present.
	if host, _, err := net.SplitHostPort(ip); err == nil {
		return host
	}
	return ip
}

// intToStr converts an int to string without importing strconv for a one-liner.
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}
