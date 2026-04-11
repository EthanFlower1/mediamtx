package brivo

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"time"
)

// OAuthService manages the OAuth 2.0 authorization-code flow with PKCE
// for connecting a tenant to Brivo.
type OAuthService struct {
	cfg        OAuthConfig
	tokens     TokenStore
	api        BrivoAPIClient
	auditHook  AuditHook
	clock      func() time.Time

	// pkce stores in-flight PKCE sessions keyed by state. In production
	// this should be backed by Redis/DynamoDB with a short TTL. For now
	// we use an in-memory map (sufficient for single-instance + tests).
	pkce map[string]*PKCESession
}

// OAuthServiceConfig bundles dependencies for OAuthService.
type OAuthServiceConfig struct {
	OAuth     OAuthConfig
	Tokens    TokenStore
	API       BrivoAPIClient
	AuditHook AuditHook
	Clock     func() time.Time
}

// NewOAuthService constructs an OAuthService.
func NewOAuthService(cfg OAuthServiceConfig) *OAuthService {
	clock := cfg.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	auditHook := cfg.AuditHook
	if auditHook == nil {
		auditHook = func(context.Context, AuditEvent) {}
	}
	return &OAuthService{
		cfg:       cfg.OAuth,
		tokens:    cfg.Tokens,
		api:       cfg.API,
		auditHook: auditHook,
		clock:     clock,
		pkce:      make(map[string]*PKCESession),
	}
}

// BeginAuthorize generates the Brivo OAuth authorization URL with PKCE.
// The caller should redirect the tenant admin's browser to this URL.
func (s *OAuthService) BeginAuthorize(ctx context.Context, tenantID string) (authorizeURL string, err error) {
	state, err := randomString(32)
	if err != nil {
		return "", fmt.Errorf("brivo: generate state: %w", err)
	}
	verifier, err := randomString(64)
	if err != nil {
		return "", fmt.Errorf("brivo: generate verifier: %w", err)
	}

	challenge := s256Challenge(verifier)

	s.pkce[state] = &PKCESession{
		TenantID:     tenantID,
		State:        state,
		CodeVerifier: verifier,
		CreatedAt:    s.clock(),
	}

	authURL := s.cfg.AuthURL
	if authURL == "" {
		authURL = "https://auth.brivo.com/oauth/authorize"
	}

	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {s.cfg.ClientID},
		"redirect_uri":          {s.cfg.RedirectURL},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"scope":                 {"read write"},
	}

	return authURL + "?" + params.Encode(), nil
}

// CompleteAuthorize handles the OAuth callback, exchanges the code for tokens,
// and stores them in the token store.
func (s *OAuthService) CompleteAuthorize(ctx context.Context, state, code string) (TokenPair, error) {
	session, ok := s.pkce[state]
	if !ok {
		return TokenPair{}, ErrInvalidState
	}
	delete(s.pkce, state)

	// Reject sessions older than 10 minutes.
	if s.clock().Sub(session.CreatedAt) > 10*time.Minute {
		return TokenPair{}, ErrInvalidState
	}

	tokens, err := s.api.ExchangeCode(ctx, code, session.CodeVerifier)
	if err != nil {
		return TokenPair{}, fmt.Errorf("brivo: exchange code: %w", err)
	}

	if err := s.tokens.StoreToken(ctx, session.TenantID, tokens); err != nil {
		return TokenPair{}, fmt.Errorf("brivo: store token: %w", err)
	}

	s.auditHook(ctx, AuditEvent{
		Action:   "connect",
		TenantID: session.TenantID,
		Detail:   "OAuth flow completed",
	})

	return tokens, nil
}

// EnsureValidToken retrieves the stored token for a tenant, refreshing it
// if expired. Returns the valid access token string.
func (s *OAuthService) EnsureValidToken(ctx context.Context, tenantID string) (string, error) {
	tok, err := s.tokens.GetToken(ctx, tenantID)
	if err != nil {
		return "", err
	}

	if !tok.IsExpired() {
		return tok.AccessToken, nil
	}

	// Attempt refresh with retry.
	var refreshErr error
	for attempt := 0; attempt < 3; attempt++ {
		newTok, err := s.api.RefreshToken(ctx, tok.RefreshToken)
		if err == nil {
			if storeErr := s.tokens.StoreToken(ctx, tenantID, newTok); storeErr != nil {
				return "", fmt.Errorf("brivo: store refreshed token: %w", storeErr)
			}
			s.auditHook(ctx, AuditEvent{
				Action:   "token_refresh",
				TenantID: tenantID,
				Detail:   fmt.Sprintf("refreshed on attempt %d", attempt+1),
			})
			return newTok.AccessToken, nil
		}
		refreshErr = err
		// Exponential-ish backoff handled by caller if needed; here we just
		// retry immediately since the Brivo API is typically fast.
	}

	return "", fmt.Errorf("%w: %v", ErrTokenExpired, refreshErr)
}

// Disconnect removes the stored token and marks the connection as
// disconnected.
func (s *OAuthService) Disconnect(ctx context.Context, tenantID string) error {
	if err := s.tokens.DeleteToken(ctx, tenantID); err != nil {
		return fmt.Errorf("brivo: delete token: %w", err)
	}
	s.auditHook(ctx, AuditEvent{
		Action:   "disconnect",
		TenantID: tenantID,
	})
	return nil
}

// ---------------------------------------------------------------------------
// PKCE helpers
// ---------------------------------------------------------------------------

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func s256Challenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
