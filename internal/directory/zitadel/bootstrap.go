// Package zitadel implements the Zitadel identity provider integration for
// the on-prem Directory. This file provides the first-run bootstrap logic
// that provisions Zitadel with the required org, service account, and OIDC
// client configuration.
//
// Bootstrap is idempotent: it checks for existing resources before creating
// them, so it is safe to call on every Directory startup.
package zitadel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// AdminAPI abstracts the Zitadel Admin/Management API calls needed for
// bootstrap. In production this wraps the gRPC client; in tests it's a fake.
type AdminAPI interface {
	// GetDefaultOrg returns the default org ID, or empty string if none exists.
	GetDefaultOrg(ctx context.Context) (orgID string, err error)

	// CreateOrg creates an organization and returns its ID.
	CreateOrg(ctx context.Context, name string) (orgID string, err error)

	// CreateServiceAccount creates a machine user (service account) in the
	// given org and returns the user ID.
	CreateServiceAccount(ctx context.Context, orgID, name, description string) (userID string, err error)

	// CreateServiceAccountKey generates an API key (JWT or PAT) for the
	// service account. Returns the key ID and the key material (JSON or token).
	CreateServiceAccountKey(ctx context.Context, userID string) (keyID string, keyJSON []byte, err error)

	// CreateOIDCApp creates an OIDC application (client) in the given org
	// and returns the client ID and secret.
	CreateOIDCApp(ctx context.Context, orgID string, cfg OIDCAppConfig) (clientID, clientSecret string, err error)

	// GetAppByName looks up an OIDC application by name within the org.
	// Returns the client ID if found, or empty string if not.
	GetAppByName(ctx context.Context, orgID, name string) (clientID string, err error)

	// GetServiceAccountByName looks up a service account by name within the org.
	// Returns the user ID if found, or empty string if not.
	GetServiceAccountByName(ctx context.Context, orgID, name string) (userID string, err error)
}

// OIDCAppConfig describes the OIDC client to create during bootstrap.
type OIDCAppConfig struct {
	Name         string
	RedirectURIs []string
	PostLogoutRedirectURIs []string
	ResponseTypes []string // "CODE"
	GrantTypes    []string // "AUTHORIZATION_CODE", "REFRESH_TOKEN"
	AppType       string   // "WEB" or "NATIVE"
	AuthMethod    string   // "BASIC" or "NONE" (for PKCE)
}

// BootstrapConfig holds the parameters for first-run provisioning.
type BootstrapConfig struct {
	// OrgName is the name of the default organization. Default: "Kaivue".
	OrgName string

	// ServiceAccountName is the machine user name. Default: "directory-sa".
	ServiceAccountName string

	// DirectoryOIDCApp configures the OIDC client for the Directory login flow.
	DirectoryOIDCApp OIDCAppConfig

	// FlutterOIDCApp configures the OIDC client for the Flutter mobile app.
	FlutterOIDCApp OIDCAppConfig

	// Logger is the base logger.
	Logger *slog.Logger
}

func (c *BootstrapConfig) orgName() string {
	if c.OrgName != "" {
		return c.OrgName
	}
	return "Kaivue"
}

func (c *BootstrapConfig) saName() string {
	if c.ServiceAccountName != "" {
		return c.ServiceAccountName
	}
	return "directory-sa"
}

func (c *BootstrapConfig) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// BootstrapResult contains the IDs and credentials provisioned during bootstrap.
type BootstrapResult struct {
	OrgID                  string
	ServiceAccountUserID   string
	ServiceAccountKeyID    string
	ServiceAccountKeyJSON  []byte
	DirectoryClientID      string
	DirectoryClientSecret  string
	FlutterClientID        string
	FlutterClientSecret    string
	CreatedAt              time.Time
}

// Bootstrapper performs idempotent first-run provisioning of Zitadel.
type Bootstrapper struct {
	api AdminAPI
	cfg BootstrapConfig
	log *slog.Logger

	mu     sync.Mutex
	result *BootstrapResult
}

// NewBootstrapper creates a Bootstrapper with the given Zitadel admin API
// and configuration.
func NewBootstrapper(api AdminAPI, cfg BootstrapConfig) (*Bootstrapper, error) {
	if api == nil {
		return nil, fmt.Errorf("zitadel/bootstrap: AdminAPI is required")
	}
	return &Bootstrapper{
		api: api,
		cfg: cfg,
		log: cfg.logger().With("component", "zitadel.bootstrap"),
	}, nil
}

// Run performs the idempotent bootstrap sequence:
//  1. Ensure default organization exists
//  2. Ensure service account exists with API key
//  3. Ensure Directory OIDC client exists
//  4. Ensure Flutter OIDC client exists
//
// Run is safe to call multiple times; existing resources are reused.
func (b *Bootstrapper) Run(ctx context.Context) (*BootstrapResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	result := &BootstrapResult{CreatedAt: time.Now().UTC()}

	// Step 1: Organization.
	orgID, err := b.ensureOrg(ctx)
	if err != nil {
		return nil, fmt.Errorf("zitadel/bootstrap: org: %w", err)
	}
	result.OrgID = orgID

	// Step 2: Service account.
	saUserID, keyID, keyJSON, err := b.ensureServiceAccount(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("zitadel/bootstrap: service account: %w", err)
	}
	result.ServiceAccountUserID = saUserID
	result.ServiceAccountKeyID = keyID
	result.ServiceAccountKeyJSON = keyJSON

	// Step 3: Directory OIDC client.
	if b.cfg.DirectoryOIDCApp.Name != "" {
		clientID, clientSecret, err := b.ensureOIDCApp(ctx, orgID, b.cfg.DirectoryOIDCApp)
		if err != nil {
			return nil, fmt.Errorf("zitadel/bootstrap: directory OIDC: %w", err)
		}
		result.DirectoryClientID = clientID
		result.DirectoryClientSecret = clientSecret
	}

	// Step 4: Flutter OIDC client.
	if b.cfg.FlutterOIDCApp.Name != "" {
		clientID, clientSecret, err := b.ensureOIDCApp(ctx, orgID, b.cfg.FlutterOIDCApp)
		if err != nil {
			return nil, fmt.Errorf("zitadel/bootstrap: flutter OIDC: %w", err)
		}
		result.FlutterClientID = clientID
		result.FlutterClientSecret = clientSecret
	}

	b.result = result
	b.log.Info("bootstrap complete",
		slog.String("org_id", result.OrgID),
		slog.String("sa_user_id", result.ServiceAccountUserID))

	return result, nil
}

// Result returns the last successful bootstrap result, or nil if Run has
// not completed.
func (b *Bootstrapper) Result() *BootstrapResult {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.result
}

func (b *Bootstrapper) ensureOrg(ctx context.Context) (string, error) {
	orgID, err := b.api.GetDefaultOrg(ctx)
	if err != nil {
		return "", err
	}
	if orgID != "" {
		b.log.Info("org already exists", slog.String("org_id", orgID))
		return orgID, nil
	}

	orgID, err = b.api.CreateOrg(ctx, b.cfg.orgName())
	if err != nil {
		return "", err
	}
	b.log.Info("org created", slog.String("org_id", orgID), slog.String("name", b.cfg.orgName()))
	return orgID, nil
}

func (b *Bootstrapper) ensureServiceAccount(ctx context.Context, orgID string) (string, string, []byte, error) {
	saName := b.cfg.saName()

	userID, err := b.api.GetServiceAccountByName(ctx, orgID, saName)
	if err != nil {
		return "", "", nil, err
	}

	if userID != "" {
		b.log.Info("service account already exists", slog.String("user_id", userID))
		// Generate a new key even if the SA exists (keys are additive).
		keyID, keyJSON, err := b.api.CreateServiceAccountKey(ctx, userID)
		if err != nil {
			return "", "", nil, fmt.Errorf("create key for existing SA: %w", err)
		}
		return userID, keyID, keyJSON, nil
	}

	userID, err = b.api.CreateServiceAccount(ctx, orgID, saName, "Directory service account for API access")
	if err != nil {
		return "", "", nil, err
	}
	b.log.Info("service account created", slog.String("user_id", userID), slog.String("name", saName))

	keyID, keyJSON, err := b.api.CreateServiceAccountKey(ctx, userID)
	if err != nil {
		return "", "", nil, fmt.Errorf("create key: %w", err)
	}
	b.log.Info("service account key created", slog.String("key_id", keyID))

	return userID, keyID, keyJSON, nil
}

func (b *Bootstrapper) ensureOIDCApp(ctx context.Context, orgID string, appCfg OIDCAppConfig) (string, string, error) {
	clientID, err := b.api.GetAppByName(ctx, orgID, appCfg.Name)
	if err != nil {
		return "", "", err
	}
	if clientID != "" {
		b.log.Info("OIDC app already exists",
			slog.String("name", appCfg.Name),
			slog.String("client_id", clientID))
		return clientID, "", nil // secret not returned for existing apps
	}

	clientID, clientSecret, err := b.api.CreateOIDCApp(ctx, orgID, appCfg)
	if err != nil {
		return "", "", err
	}
	b.log.Info("OIDC app created",
		slog.String("name", appCfg.Name),
		slog.String("client_id", clientID))

	return clientID, clientSecret, nil
}

// BootstrapResultJSON serializes the result for encrypted storage.
func (r *BootstrapResult) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

// ParseBootstrapResult deserializes a previously stored result.
func ParseBootstrapResult(data []byte) (*BootstrapResult, error) {
	var r BootstrapResult
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("zitadel/bootstrap: parse result: %w", err)
	}
	return &r, nil
}
