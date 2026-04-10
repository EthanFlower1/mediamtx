package federation

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	directorydb "github.com/bluenviron/mediamtx/internal/directory/db"
	fedpki "github.com/bluenviron/mediamtx/internal/directory/pki/federation"
)

// TokenTTL is the default lifetime of a federation pairing token (60 minutes).
const TokenTTL = 60 * time.Minute

// FederationCAClient is the minimal interface the pairing service requires
// from the federation CA (internal/directory/pki/federation.FederationCA).
type FederationCAClient interface {
	// MintPeerEnrollmentToken creates a signed enrollment token containing the
	// federation CA fingerprint and root PEM.
	MintPeerEnrollmentToken(foundingEndpoint, peerSiteID, issuedBy string, ttl time.Duration) (string, error)

	// PeerTokenVerifyKey returns the ed25519 public key that can verify
	// peer enrollment tokens issued by this CA.
	PeerTokenVerifyKey() (ed25519.PublicKey, error)

	// Fingerprint returns the lowercase hex SHA-256 of the federation root cert DER.
	Fingerprint() string

	// RootPEM returns the federation root certificate in PEM form.
	RootPEM() []byte
}

// JWKSProvider abstracts the local Directory's JWKS endpoint. The service
// calls LocalJWKS() to obtain the JSON-serialized JWKS that will be exchanged
// with the peer during the handshake.
type JWKSProvider interface {
	// LocalJWKS returns the JSON-encoded JWKS of this Directory's signing keys.
	LocalJWKS() ([]byte, error)
}

// Config parameterizes a Service.
type Config struct {
	// DB is the on-prem Directory SQLite handle.
	DB *directorydb.DB

	// FederationCA is the federation PKI layer (KAI-268).
	FederationCA FederationCAClient

	// JWKS provides this Directory's signing key set for exchange.
	JWKS JWKSProvider

	// SiteID identifies this Directory in the federation.
	SiteID string

	// DirectoryEndpoint is the base URL peers will use to reach this Directory,
	// e.g. "https://dir.site-a.kaivue.local:8443".
	DirectoryEndpoint string

	// Logger is the slog logger. nil defaults to slog.Default().
	Logger *slog.Logger

	// Clock overrides time.Now for tests. nil = time.Now.
	Clock func() time.Time
}

// Service generates and manages federation pairing tokens and the handshake
// lifecycle. It is the single entry point for federation pairing and is safe
// for concurrent use.
type Service struct {
	cfg   Config
	store *Store
	log   *slog.Logger
	now   func() time.Time
}

// NewService constructs a federation pairing Service.
func NewService(cfg Config) (*Service, error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("federation: Config.DB is required")
	}
	if cfg.FederationCA == nil {
		return nil, fmt.Errorf("federation: Config.FederationCA is required")
	}
	if cfg.JWKS == nil {
		return nil, fmt.Errorf("federation: Config.JWKS is required")
	}
	if cfg.SiteID == "" {
		return nil, fmt.Errorf("federation: Config.SiteID is required")
	}
	if cfg.DirectoryEndpoint == "" {
		return nil, fmt.Errorf("federation: Config.DirectoryEndpoint is required")
	}

	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	now := cfg.Clock
	if now == nil {
		now = time.Now
	}

	return &Service{
		cfg:   cfg,
		store: NewStore(cfg.DB),
		log:   log,
		now:   now,
	}, nil
}

// InviteResult is returned by Invite on success.
type InviteResult struct {
	// FEDToken is the full "FED-v1.<payload>.<sig>" string to display in the UI.
	FEDToken string
	// TokenID is the stable identifier for audit and status display.
	TokenID string
	// PeerSiteID is the site ID pre-allocated for the joining peer.
	PeerSiteID string
	// ExpiresAt is the absolute expiry time.
	ExpiresAt time.Time
}

// Invite mints a new federation pairing token. The founding Directory admin
// calls this to generate a token that can be shared with a peer Directory.
//
// issuedBy is the authenticated admin user identifier.
func (svc *Service) Invite(ctx context.Context, issuedBy string) (*InviteResult, error) {
	peerSiteID := uuid.NewString()

	// 1. Mint the low-level enrollment token via the federation CA.
	rawToken, err := svc.cfg.FederationCA.MintPeerEnrollmentToken(
		svc.cfg.DirectoryEndpoint,
		peerSiteID,
		issuedBy,
		TokenTTL,
	)
	if err != nil {
		return nil, fmt.Errorf("federation: mint enrollment token: %w", err)
	}

	// 2. Wrap with FED-v1 prefix.
	fedToken := WrapToken(rawToken)

	// 3. Build a stable token ID.
	tokenID := fmt.Sprintf("fed-%s", peerSiteID)
	expiresAt := svc.now().UTC().Add(TokenTTL)

	// 4. Persist for single-use tracking.
	if err := svc.store.InsertToken(ctx, tokenID, fedToken, peerSiteID, issuedBy, expiresAt); err != nil {
		return nil, fmt.Errorf("federation: store insert: %w", err)
	}

	svc.log.Info("federation: invite token generated",
		"token_id", tokenID,
		"peer_site_id", peerSiteID,
		"issued_by", issuedBy,
		"expires_at", expiresAt.Format(time.RFC3339),
	)

	return &InviteResult{
		FEDToken:   fedToken,
		TokenID:    tokenID,
		PeerSiteID: peerSiteID,
		ExpiresAt:  expiresAt,
	}, nil
}

// JoinRequest is the payload a peer Directory sends to join the federation.
type JoinRequest struct {
	// FEDToken is the full "FED-v1...." string received from the founding admin.
	FEDToken string
	// PeerEndpoint is the joining Directory's reachable endpoint.
	PeerEndpoint string
	// PeerName is the human-readable name of the joining site.
	PeerName string
	// PeerJWKS is the JSON-encoded JWKS of the joining Directory's signing keys.
	PeerJWKS string
	// PeerSiteID is the site ID the joining Directory claims (must match the token).
	PeerSiteID string
}

// JoinResult is returned by Join on success.
type JoinResult struct {
	// FoundingSiteID is the founding Directory's site ID.
	FoundingSiteID string
	// FoundingEndpoint is the founding Directory's endpoint.
	FoundingEndpoint string
	// FoundingJWKS is the JSON-encoded JWKS of the founding Directory.
	FoundingJWKS string
	// CAFingerprint is the federation CA fingerprint for trust verification.
	CAFingerprint string
	// CARootPEM is the federation CA root certificate in PEM form.
	CARootPEM string
}

// Join processes a peer Directory's request to join the federation. This is
// the server-side handler for the handshake:
//
//  1. Unwrap and decode the FED-v1 token
//  2. Verify the enrollment token signature and TTL
//  3. Atomically redeem (single-use enforcement)
//  4. Exchange JWKS
//  5. Write the peer into federation_members
//
// The verifyKey is the ed25519 public key for verifying the enrollment token.
// In production this is derived from the federation CA root key via
// federation.DerivePeerTokenVerifyKey.
func (svc *Service) Join(ctx context.Context, req JoinRequest, verifyKey ed25519.PublicKey) (*JoinResult, error) {
	// 1. Unwrap FED-v1 prefix.
	_, rawToken, err := UnwrapToken(req.FEDToken)
	if err != nil {
		return nil, fmt.Errorf("federation: %w", err)
	}

	// 2. Decode and verify the enrollment token.
	pet, err := fedpki.DecodePeerEnrollmentToken(rawToken, verifyKey)
	if err != nil {
		return nil, fmt.Errorf("federation: invalid token: %w", err)
	}

	// 3. Validate peer site ID matches.
	if req.PeerSiteID != "" && req.PeerSiteID != pet.PeerSiteID {
		return nil, fmt.Errorf("federation: peer site ID mismatch: token has %q, request has %q",
			pet.PeerSiteID, req.PeerSiteID)
	}

	// 4. Validate JWKS is non-empty.
	if req.PeerJWKS == "" {
		return nil, fmt.Errorf("federation: peer JWKS is required")
	}

	// 5. Atomic single-use enforcement.
	tokenID := pet.TokenID
	if err := svc.store.RedeemToken(ctx, tokenID); err != nil {
		return nil, fmt.Errorf("federation: redeem: %w", err)
	}

	// 6. Write the peer into federation_members.
	member := MemberRow{
		SiteID:        pet.PeerSiteID,
		Name:          req.PeerName,
		Endpoint:      req.PeerEndpoint,
		JWKSJson:      req.PeerJWKS,
		CAFingerprint: pet.FederationCAFingerprint,
	}
	if err := svc.store.InsertMember(ctx, member); err != nil {
		return nil, fmt.Errorf("federation: insert member: %w", err)
	}

	// 7. Build the response with the founding Directory's JWKS.
	foundingJWKS, err := svc.cfg.JWKS.LocalJWKS()
	if err != nil {
		return nil, fmt.Errorf("federation: get local JWKS: %w", err)
	}

	svc.log.Info("federation: peer joined",
		"peer_site_id", pet.PeerSiteID,
		"peer_name", req.PeerName,
		"peer_endpoint", req.PeerEndpoint,
		"token_id", tokenID,
	)

	return &JoinResult{
		FoundingSiteID:   svc.cfg.SiteID,
		FoundingEndpoint: svc.cfg.DirectoryEndpoint,
		FoundingJWKS:     string(foundingJWKS),
		CAFingerprint:    svc.cfg.FederationCA.Fingerprint(),
		CARootPEM:        string(svc.cfg.FederationCA.RootPEM()),
	}, nil
}

// Store returns the underlying Store for direct use by the sweeper.
func (svc *Service) Store() *Store { return svc.store }
