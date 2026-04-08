package pairing

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	directorydb "github.com/bluenviron/mediamtx/internal/directory/db"
)

// HeadscaleClient is the minimal surface of the Headscale coordinator
// (internal/directory/mesh/headscale.Coordinator) that the pairing service
// requires. Using a local interface keeps the dependency light and testable.
type HeadscaleClient interface {
	// MintPreAuthKey creates a single-use tailnet pre-auth key valid for ttl.
	MintPreAuthKey(ctx context.Context, namespace string, ttl time.Duration) (string, error)
}

// ClusterCAClient is the minimal surface of the step-ca ClusterCA
// (internal/directory/pki/stepca.ClusterCA) that the pairing service requires.
type ClusterCAClient interface {
	// Fingerprint returns the lowercase hex SHA-256 of the root cert DER.
	Fingerprint() string
	// IssueDirectoryServingCert returns the current Directory TLS certificate.
	// The pairing service derives the DirectoryFingerprint from its leaf.
	IssueDirectoryServingCert(ctx context.Context) (*tls.Certificate, error)
	// MintEnrollmentToken produces a short-lived JWK provisioner enrollment token
	// (KAI-244). The Recorder presents this to step-ca's /1.0/sign endpoint to
	// obtain its first mTLS leaf certificate without needing a pre-existing cert.
	//
	// audience is the full CA /sign URL; issuer is the JWK provisioner name;
	// sans lists the DNS names / IPs to embed in the issued cert; ttl clamps the
	// token lifetime (≤ 0 uses the CA default of 10 minutes).
	MintEnrollmentToken(audience, issuer string, sans []string, ttl time.Duration) (string, error)
}

// Config parameterises a Service.
type Config struct {
	// DB is the on-prem Directory SQLite handle (internal/directory/db.DB).
	DB *directorydb.DB

	// Headscale is the embedded tailnet coordinator (KAI-240).
	Headscale HeadscaleClient

	// ClusterCA is the embedded step-ca wrapper (KAI-241).
	ClusterCA ClusterCAClient

	// RootSigningKey is the ed25519 root key of this Directory's site CA.
	// The pairing service derives a domain-scoped sub-key via NewSigningKey.
	// Zero value is invalid.
	RootSigningKey ed25519.PrivateKey

	// DirectoryEndpoint is the base URL Recorders will use to reach this
	// Directory, e.g. "https://dir.acme.local:8443". Required.
	DirectoryEndpoint string

	// HeadscaleNamespace is the tailnet namespace Recorders will join.
	// Defaults to "recorders" if empty.
	HeadscaleNamespace string

	// StepCASignURL is the full URL of step-ca's /1.0/sign endpoint,
	// e.g. "https://ca.site.local:9000/1.0/sign". When non-empty, the service
	// mints a JWK provisioner enrollment token (via ClusterCA.MintEnrollmentToken)
	// and embeds it in PairingToken.StepCAEnrollToken. If empty the field is
	// left blank and the Recorder must obtain the token out-of-band.
	StepCASignURL string

	// StepCAProvisionerName is the JWK provisioner name on the step-ca instance.
	// Defaults to "kaivue-pairing" if empty.
	StepCAProvisionerName string

	// Logger is the slog logger. nil defaults to slog.Default().
	Logger *slog.Logger

	// Metrics is the counter set to increment on each operation.
	// nil means no-op (counters are not incremented).
	Metrics *Metrics

	// Clock overrides time.Now for tests. nil = time.Now.
	Clock func() time.Time
}

// Service generates and manages PairingTokens. It is the single entry point
// for the pairing subsystem and is safe for concurrent use.
type Service struct {
	cfg        Config
	store      *Store
	signingKey ed25519.PrivateKey
	log        *slog.Logger
	now        func() time.Time
}

// NewService constructs a Service. It derives the pairing signing key from
// cfg.RootSigningKey immediately so the root key need not be retained
// beyond construction.
func NewService(cfg Config) (*Service, error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("pairing: Config.DB is required")
	}
	if len(cfg.RootSigningKey) == 0 {
		return nil, fmt.Errorf("pairing: Config.RootSigningKey is required")
	}
	if cfg.DirectoryEndpoint == "" {
		return nil, fmt.Errorf("pairing: Config.DirectoryEndpoint is required")
	}
	if cfg.Headscale == nil {
		return nil, fmt.Errorf("pairing: Config.Headscale is required")
	}
	if cfg.ClusterCA == nil {
		return nil, fmt.Errorf("pairing: Config.ClusterCA is required")
	}
	if cfg.HeadscaleNamespace == "" {
		cfg.HeadscaleNamespace = "recorders"
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	now := cfg.Clock
	if now == nil {
		now = time.Now
	}

	sigKey, err := NewSigningKey(cfg.RootSigningKey)
	if err != nil {
		return nil, fmt.Errorf("pairing: derive signing key: %w", err)
	}

	return &Service{
		cfg:        cfg,
		store:      NewStore(cfg.DB),
		signingKey: sigKey,
		log:        log,
		now:        now,
	}, nil
}

// GenerateResult is what Generate returns on success.
type GenerateResult struct {
	// Encoded is the full opaque string sent to the operator / UI. It is the
	// only form the Recorder will ever see.
	Encoded string
	// TokenID is the human-readable handle (UUID) for audit and revocation.
	TokenID string
}

// Generate mints a new PairingToken signed by this Directory's pairing key,
// persists it, and returns the encoded blob. The caller is responsible for
// delivering the encoded string to the operator (typically via the admin API).
//
// signedBy MUST be the authenticated UserID of the admin requesting the token.
func (svc *Service) Generate(
	ctx context.Context,
	signedBy UserID,
	suggestedRoles []string,
	cloudTenantBinding string,
) (*GenerateResult, error) {
	// 1. Mint a Headscale pre-auth key.
	preAuthKey, err := svc.cfg.Headscale.MintPreAuthKey(ctx, svc.cfg.HeadscaleNamespace, TokenTTL)
	if err != nil {
		return nil, fmt.Errorf("pairing: mint headscale key: %w", err)
	}

	// 2. Obtain step-ca root fingerprint.
	caFingerprint := svc.cfg.ClusterCA.Fingerprint()

	// 3. Derive the Directory TLS fingerprint from its current serving cert.
	dirCert, err := svc.cfg.ClusterCA.IssueDirectoryServingCert(ctx)
	if err != nil {
		return nil, fmt.Errorf("pairing: issue directory serving cert: %w", err)
	}
	dirFingerprint := certFingerprint(dirCert)

	// 3b. Mint a JWK provisioner enrollment token so the Recorder can obtain its
	// first mTLS leaf without a pre-existing cert. Only when StepCASignURL is
	// configured; otherwise leave the field blank (KAI-244 option (a) seam).
	var enrollToken string
	if svc.cfg.StepCASignURL != "" {
		provisioner := svc.cfg.StepCAProvisionerName
		if provisioner == "" {
			provisioner = "kaivue-pairing"
		}
		enrollToken, err = svc.cfg.ClusterCA.MintEnrollmentToken(
			svc.cfg.StepCASignURL,
			provisioner,
			nil, // SANs are chosen by the Recorder when it submits the CSR
			0,   // use CA default TTL
		)
		if err != nil {
			return nil, fmt.Errorf("pairing: mint enrollment token: %w", err)
		}
	}

	// 4. Build the token.
	if suggestedRoles == nil {
		suggestedRoles = []string{"recorder"}
	}
	now := svc.now().UTC()
	pt := &PairingToken{
		TokenID:              uuid.NewString(),
		DirectoryEndpoint:    svc.cfg.DirectoryEndpoint,
		HeadscalePreAuthKey:  preAuthKey,
		StepCAFingerprint:    caFingerprint,
		StepCAEnrollToken:    enrollToken,
		DirectoryFingerprint: dirFingerprint,
		SuggestedRoles:       suggestedRoles,
		ExpiresAt:            now.Add(TokenTTL),
		SignedBy:             signedBy,
		CloudTenantBinding:   cloudTenantBinding,
	}

	// 5. Sign and encode.
	encoded, err := pt.Encode(svc.signingKey)
	if err != nil {
		return nil, fmt.Errorf("pairing: encode: %w", err)
	}

	// 6. Persist.
	if err := svc.store.Insert(ctx, pt, encoded); err != nil {
		return nil, fmt.Errorf("pairing: store insert: %w", err)
	}

	svc.log.Info("pairing: token generated",
		"token_id", pt.TokenID,
		"signed_by", string(signedBy),
		"expires_at", pt.ExpiresAt.Format(time.RFC3339),
		"suggested_roles", suggestedRoles,
	)
	if svc.cfg.Metrics != nil {
		svc.cfg.Metrics.Generated.Add(1)
	}

	return &GenerateResult{Encoded: encoded, TokenID: pt.TokenID}, nil
}

// Redeem atomically transitions a token from pending to redeemed. This is the
// seam for KAI-244's /api/v1/pairing/check-in endpoint — that handler calls
// Redeem after verifying the Recorder's identity.
//
// Returns ErrAlreadyRedeemed, ErrTokenExpired, or ErrNotFound on failure.
func (svc *Service) Redeem(ctx context.Context, tokenID string) error {
	err := svc.store.Redeem(ctx, tokenID)
	if err != nil {
		svc.log.Warn("pairing: redeem failed",
			"token_id", tokenID,
			"error", err,
		)
		return err
	}
	svc.log.Info("pairing: token redeemed", "token_id", tokenID)
	if svc.cfg.Metrics != nil {
		svc.cfg.Metrics.Redeemed.Add(1)
	}
	return nil
}

// VerifyPublicKeyForDecode exposes the public key needed to decode tokens
// issued by this Service. Callers that need to independently verify a token
// (e.g. in tests or in the KAI-244 check-in endpoint) should use this.
func (svc *Service) VerifyPublicKeyForDecode() ed25519.PublicKey {
	return VerifyPublicKey(svc.signingKey)
}

// Store returns the underlying token Store for direct use by the sweeper and
// the check-in endpoint (KAI-244). Callers should prefer the Service methods.
func (svc *Service) Store() *Store { return svc.store }

// certFingerprint returns the lowercase hex SHA-256 of the first certificate
// in tls.Certificate.Certificate (the leaf DER bytes).
func certFingerprint(c *tls.Certificate) string {
	if c == nil || len(c.Certificate) == 0 {
		return ""
	}
	sum := sha256.Sum256(c.Certificate[0])
	return hex.EncodeToString(sum[:])
}
