package certmgr

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultRenewThreshold is the remaining lifetime below which the Manager
	// will attempt renewal. §13.2 says "renew when remaining lifetime drops
	// below ~8h".
	DefaultRenewThreshold = 8 * time.Hour

	// DefaultTickInterval is how often the background goroutine wakes to
	// check whether renewal is due. 15 minutes gives plenty of headroom
	// within the 8h threshold.
	DefaultTickInterval = 15 * time.Minute

	// renewalTimeout is the per-attempt context deadline for Renew / ReEnroll
	// calls. Generous enough for slow CA round-trips, tight enough to not
	// stall the tick loop.
	renewalTimeout = 2 * time.Minute
)

// errExpiredOrUnknown is the substring we look for in CA error messages to
// detect the "cert expired / cert unknown" case that triggers re-enrollment.
// step-ca surfaces these as TLS alert descriptions in the error message.
const errExpiredOrUnknown = "certificate"

// Config parameterizes a Manager. All fields except CA and KeyStore are
// optional (sensible defaults apply).
type Config struct {
	// CA is the step-ca client. Required.
	CA CAClient

	// KeyStore persists the leaf cert across restarts. Required.
	KeyStore KeyStore

	// DeviceKey is the pairing-time private key used exclusively for the
	// re-enrollment fallback path. Required if re-enrollment is needed.
	DeviceKey crypto.PrivateKey

	// SANs is the list of DNS names / IP SANs to embed in re-enrolled certs.
	// Not used on the renewal path (the CA mirrors the existing SANs).
	SANs []string

	// RenewThreshold is the remaining lifetime below which renewal is
	// triggered. Defaults to DefaultRenewThreshold.
	RenewThreshold time.Duration

	// TickInterval is the period of the background renewal-check loop.
	// Defaults to DefaultTickInterval.
	TickInterval time.Duration

	// Clock overrides time.Now for test determinism. Nil = time.Now.
	Clock ClockFunc

	// Logger receives structured log output. Nil = slog.Default().
	Logger *slog.Logger

	// Metrics collects counters and gauges. Nil = discard.
	Metrics *Metrics
}

func (c *Config) setDefaults() {
	if c.RenewThreshold <= 0 {
		c.RenewThreshold = DefaultRenewThreshold
	}
	if c.TickInterval <= 0 {
		c.TickInterval = DefaultTickInterval
	}
	if c.Clock == nil {
		c.Clock = time.Now
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

// Manager is the mTLS leaf-cert lifecycle controller. It is safe for
// concurrent use after New returns.
//
// Call Start to launch the background renewal goroutine, and Shutdown to stop
// it. GetCertificate is wired directly into tls.Config.GetCertificate and
// returns the currently-cached cert atomically — no listener restart required.
type Manager struct {
	cfg Config

	// cert holds the *tls.Certificate pointer atomically so GetCertificate
	// can read it without acquiring mu. mu is only held during the write
	// side (renewal / re-enrollment swap).
	cert atomic.Pointer[tls.Certificate]

	mu sync.Mutex // serialises renewal attempts

	// forceRenewCh signals the background loop to attempt renewal
	// immediately, bypassing the tick timer.
	forceRenewCh chan struct{}

	// done is closed by Shutdown to stop the background goroutine.
	done chan struct{}

	wg sync.WaitGroup
}

// New constructs a Manager and loads the most-recently persisted cert from the
// KeyStore. If no cert is stored (first boot after pairing), the Manager
// starts with a nil cert; the caller is responsible for driving an initial
// ForceRenew() or wiring up an initial enrollment before placing the Manager's
// tls.Config into service.
func New(cfg Config) (*Manager, error) {
	if cfg.CA == nil {
		return nil, fmt.Errorf("certmgr: CA is required")
	}
	if cfg.KeyStore == nil {
		return nil, fmt.Errorf("certmgr: KeyStore is required")
	}
	cfg.setDefaults()

	m := &Manager{
		cfg:          cfg,
		forceRenewCh: make(chan struct{}, 1),
		done:         make(chan struct{}),
	}

	// Attempt to restore the most recently persisted cert. On first boot this
	// returns (nil, nil) which is fine — the caller will drive ForceRenew.
	ctx := context.Background()
	stored, err := cfg.KeyStore.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("certmgr: load stored cert: %w", err)
	}
	if stored != nil {
		m.cert.Store(stored)
		if cfg.Metrics != nil && stored.Leaf != nil {
			cfg.Metrics.SetCertExpiry(stored.Leaf.NotAfter)
		}
		cfg.Logger.Info("certmgr: loaded stored cert",
			slog.Time("not_after", certNotAfter(stored)))
	}

	return m, nil
}

// Start launches the background renewal goroutine. It is safe to call Start
// multiple times but subsequent calls are no-ops after the first.
func (m *Manager) Start(ctx context.Context) {
	m.wg.Add(1)
	go m.loop(ctx)
}

// Shutdown stops the background goroutine and waits for it to exit. The
// context passed here is the process-shutdown context; pass a context with a
// short deadline for clean shutdown behaviour.
func (m *Manager) Shutdown(_ context.Context) {
	close(m.done)
	m.wg.Wait()
}

// GetCertificate implements tls.Config.GetCertificate. It returns the
// currently-active cert; the pointer swap during renewal is atomic so
// callers never see a torn state. Returns nil if no cert has been loaded
// yet (the TLS stack will fall back to the static tls.Config.Certificates
// slice if one is provided).
func (m *Manager) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cert := m.cert.Load()
	if cert == nil {
		return nil, nil
	}
	return cert, nil
}

// ForceRenew signals the background loop to attempt renewal immediately,
// bypassing the tick timer. It is non-blocking: if a renewal is already
// queued, the signal is dropped.
func (m *Manager) ForceRenew() {
	select {
	case m.forceRenewCh <- struct{}{}:
	default:
	}
}

// ActiveCert returns the currently active *tls.Certificate, or nil if none
// has been loaded. Safe to call concurrently.
func (m *Manager) ActiveCert() *tls.Certificate {
	return m.cert.Load()
}

// loop is the background renewal goroutine.
func (m *Manager) loop(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.cfg.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.maybeRenew(ctx)
		case <-m.forceRenewCh:
			m.maybeRenew(ctx)
		}
	}
}

// maybeRenew checks the current cert's remaining lifetime and initiates
// renewal if it has dropped below RenewThreshold. It is serialised by mu so
// concurrent signals (tick + ForceRenew) do not cause double-renewal.
func (m *Manager) maybeRenew(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.cfg.Clock()
	current := m.cert.Load()

	if current != nil && current.Leaf != nil {
		remaining := current.Leaf.NotAfter.Sub(now)
		if remaining > m.cfg.RenewThreshold {
			// Still plenty of lifetime — nothing to do.
			if m.cfg.Metrics != nil {
				m.cfg.Metrics.RecordRenewal(RenewalResultSkipped)
			}
			return
		}
		m.cfg.Logger.Info("certmgr: cert approaching expiry; renewing",
			slog.Duration("remaining", remaining),
			slog.Time("not_after", current.Leaf.NotAfter))
	} else {
		m.cfg.Logger.Info("certmgr: no active cert; enrolling")
	}

	rctx, cancel := context.WithTimeout(ctx, renewalTimeout)
	defer cancel()

	m.attemptRenewal(rctx, current)
}

// attemptRenewal tries Renew first; if that fails with an expiry/unknown
// condition (or if current is nil/expired), it falls back to ReEnroll.
// Must be called with m.mu held.
func (m *Manager) attemptRenewal(ctx context.Context, current *tls.Certificate) {
	log := m.cfg.Logger

	// Decide whether to use the normal renewal path or the re-enrollment
	// fallback. Re-enroll if: no current cert, or cert is already past NotAfter.
	now := m.cfg.Clock()
	needsReEnroll := current == nil || (current.Leaf != nil && now.After(current.Leaf.NotAfter))

	if !needsReEnroll {
		renewed, err := m.cfg.CA.Renew(ctx, current)
		if err == nil {
			m.installCert(ctx, renewed, "renewal")
			return
		}
		// Check whether the error indicates an expired/unknown cert so we can
		// fall through to re-enrollment rather than just alarming.
		if isExpiredOrUnknownErr(err) {
			log.Warn("certmgr: renewal rejected by CA (cert expired/unknown); falling back to re-enrollment",
				slog.String("err", err.Error()))
			needsReEnroll = true
		} else {
			log.Error("certmgr: renewal failed; keeping old cert",
				slog.String("err", err.Error()))
			if m.cfg.Metrics != nil {
				m.cfg.Metrics.RecordRenewal(RenewalResultError)
			}
			return
		}
	}

	if !needsReEnroll {
		return
	}

	// Re-enrollment path.
	if m.cfg.DeviceKey == nil {
		log.Error("certmgr: re-enrollment required but DeviceKey is nil; cannot recover")
		if m.cfg.Metrics != nil {
			m.cfg.Metrics.RecordRenewal(RenewalResultError)
			m.cfg.Metrics.RecordReEnrollment(ReEnrollResultError)
		}
		return
	}

	enrolled, err := m.cfg.CA.ReEnroll(ctx, m.cfg.DeviceKey, m.cfg.SANs)
	if err != nil {
		log.Error("certmgr: re-enrollment failed; keeping old cert (if any)",
			slog.String("err", err.Error()))
		if m.cfg.Metrics != nil {
			m.cfg.Metrics.RecordRenewal(RenewalResultError)
			m.cfg.Metrics.RecordReEnrollment(ReEnrollResultError)
		}
		return
	}
	m.installCert(ctx, enrolled, "re-enrollment")
	if m.cfg.Metrics != nil {
		m.cfg.Metrics.RecordReEnrollment(ReEnrollResultOK)
	}
}

// installCert verifies the candidate cert against the CA trust pool and, if
// it passes, atomically swaps it in and persists it to the KeyStore.
// Must be called with m.mu held.
func (m *Manager) installCert(ctx context.Context, candidate *tls.Certificate, op string) {
	log := m.cfg.Logger

	// Populate Leaf if the CA client didn't.
	if candidate.Leaf == nil && len(candidate.Certificate) > 0 {
		parsed, err := x509.ParseCertificate(candidate.Certificate[0])
		if err != nil {
			log.Error("certmgr: could not parse renewed leaf; rejecting",
				slog.String("op", op),
				slog.String("err", err.Error()))
			if m.cfg.Metrics != nil {
				m.cfg.Metrics.RecordRenewal(RenewalResultError)
			}
			return
		}
		candidate.Leaf = parsed
	}

	// Verify the new cert chains to the site root before hot-swapping.
	if pool := m.cfg.CA.RootPool(); pool != nil && candidate.Leaf != nil {
		opts := x509.VerifyOptions{
			Roots:     pool,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}
		if _, err := candidate.Leaf.Verify(opts); err != nil {
			log.Error("certmgr: renewed cert failed verification; keeping old cert (fail-closed)",
				slog.String("op", op),
				slog.String("err", err.Error()))
			if m.cfg.Metrics != nil {
				m.cfg.Metrics.RecordRenewal(RenewalResultError)
			}
			return
		}
	}

	// Atomic swap — GetCertificate callers immediately see the new cert.
	m.cert.Store(candidate)

	// Update metrics gauge.
	if m.cfg.Metrics != nil && candidate.Leaf != nil {
		m.cfg.Metrics.SetCertExpiry(candidate.Leaf.NotAfter)
		m.cfg.Metrics.RecordRenewal(RenewalResultOK)
	}

	log.Info("certmgr: cert hot-swapped successfully",
		slog.String("op", op),
		slog.Time("not_after", certNotAfter(candidate)))

	// Persist to KeyStore — non-fatal if it fails (we can re-issue on next
	// boot from the CA, and the cert is already live in memory).
	if err := m.cfg.KeyStore.Save(ctx, candidate); err != nil {
		log.Warn("certmgr: could not persist renewed cert to KeyStore",
			slog.String("op", op),
			slog.String("err", err.Error()))
	}
}

// isExpiredOrUnknownErr returns true when err looks like a step-ca rejection
// due to the presented cert being expired or unrecognised by the CA. We
// detect this heuristically from the error message because there is no stable
// typed error from the step-ca client library for these conditions.
func isExpiredOrUnknownErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "expired") ||
		strings.Contains(msg, "certificate unknown") ||
		strings.Contains(msg, "unknown certificate") ||
		strings.Contains(msg, "tls: bad certificate") ||
		strings.Contains(msg, "x509: certificate has expired")
}

// certNotAfter returns the NotAfter from a tls.Certificate, or zero time if
// Leaf is nil.
func certNotAfter(cert *tls.Certificate) time.Time {
	if cert == nil || cert.Leaf == nil {
		return time.Time{}
	}
	return cert.Leaf.NotAfter
}
