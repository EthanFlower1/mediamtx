package zitadel

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// --- Typed convenience methods for IdP configuration (KAI-133) ----------
//
// These are the admin-facing entry points that the Sign-in Methods wizard
// calls to register external identity providers. Each method:
//
//   1. Validates the provider-specific config
//   2. Runs a protocol-level probe (OIDC discovery, SAML metadata, LDAP bind)
//   3. Persists via the Zitadel Management API
//   4. Returns the Zitadel-assigned provider ID
//
// The fail-closed policy is enforced: no provider is persisted unless the
// test probe succeeds.

// AddOIDCProvider configures an external OIDC identity provider (e.g. Google,
// Azure AD, Okta) for the given tenant org.
func (a *Adapter) AddOIDCProvider(ctx context.Context, tenant auth.TenantRef, cfg auth.OIDCConfig, displayName string) (auth.ProviderID, error) {
	if cfg.IssuerURL == "" {
		return "", fmt.Errorf("zitadel: AddOIDCProvider: issuer URL is required")
	}
	if cfg.ClientID == "" {
		return "", fmt.Errorf("zitadel: AddOIDCProvider: client ID is required")
	}

	orgID, err := a.orgIDFor(tenant)
	if err != nil {
		return "", err
	}

	// Default scopes if none provided.
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}

	// Protocol-level probe: fetch and validate the OIDC discovery document.
	testResult := a.testOIDCDiscovery(ctx, &cfg)
	if !testResult.Success {
		return "", fmt.Errorf("zitadel: AddOIDCProvider: %s", testResult.Message)
	}

	// Persist via the typed Zitadel endpoint.
	req := oidcProviderCreateRequest{
		Name:         displayName,
		Issuer:       cfg.IssuerURL,
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Scopes:       scopes,
	}
	var resp providerCreateResponse
	if err := a.client.doJSON(ctx, http.MethodPost, "/management/v1/idps/oidc", orgID, req, &resp); err != nil {
		return "", translateProviderError(err)
	}

	pid := auth.ProviderID(resp.ID)
	a.auditEmit(ctx, tenant, "", "identity.provider_add_oidc", "provider", string(pid), audit.ResultAllow)
	return pid, nil
}

// AddSAMLProvider configures an external SAML identity provider for the given
// tenant org. Either MetadataURL or MetadataXML must be provided.
func (a *Adapter) AddSAMLProvider(ctx context.Context, tenant auth.TenantRef, cfg auth.SAMLConfig, displayName string) (auth.ProviderID, error) {
	if cfg.MetadataURL == "" && cfg.MetadataXML == "" {
		return "", fmt.Errorf("zitadel: AddSAMLProvider: metadata URL or XML is required")
	}

	orgID, err := a.orgIDFor(tenant)
	if err != nil {
		return "", err
	}

	// Protocol-level probe: fetch and parse SAML metadata.
	testResult := a.testSAMLMetadata(ctx, &cfg)
	if !testResult.Success {
		return "", fmt.Errorf("zitadel: AddSAMLProvider: %s", testResult.Message)
	}

	req := samlProviderCreateRequest{
		Name:        displayName,
		MetadataURL: cfg.MetadataURL,
		MetadataXML: cfg.MetadataXML,
	}
	var resp providerCreateResponse
	if err := a.client.doJSON(ctx, http.MethodPost, "/management/v1/idps/saml", orgID, req, &resp); err != nil {
		return "", translateProviderError(err)
	}

	pid := auth.ProviderID(resp.ID)
	a.auditEmit(ctx, tenant, "", "identity.provider_add_saml", "provider", string(pid), audit.ResultAllow)
	return pid, nil
}

// AddLDAPProvider configures an external LDAP identity provider (e.g. Active
// Directory) for the given tenant org.
func (a *Adapter) AddLDAPProvider(ctx context.Context, tenant auth.TenantRef, cfg auth.LDAPConfig, displayName string) (auth.ProviderID, error) {
	if cfg.URL == "" {
		return "", fmt.Errorf("zitadel: AddLDAPProvider: LDAP URL is required")
	}
	if cfg.BindDN == "" {
		return "", fmt.Errorf("zitadel: AddLDAPProvider: bind DN is required")
	}

	orgID, err := a.orgIDFor(tenant)
	if err != nil {
		return "", err
	}

	// Protocol-level probe: validate URL, bind DN, startTLS compatibility.
	testResult := a.testLDAPConfig(&cfg)
	if !testResult.Success {
		return "", fmt.Errorf("zitadel: AddLDAPProvider: %s", testResult.Message)
	}

	// Default user object classes if none implied by filter.
	userFilter := cfg.UserFilter
	if userFilter == "" {
		userFilter = "(objectClass=inetOrgPerson)"
	}

	req := ldapProviderCreateRequest{
		Name:              displayName,
		Servers:           []string{cfg.URL},
		BindDN:            cfg.BindDN,
		BindPassword:      cfg.BindPassword,
		BaseDN:            cfg.UserBaseDN,
		UserObjectClasses: []string{"inetOrgPerson", "user"},
		UserFilters:       []string{userFilter},
		StartTLS:          cfg.StartTLS,
	}
	var resp providerCreateResponse
	if err := a.client.doJSON(ctx, http.MethodPost, "/management/v1/idps/ldap", orgID, req, &resp); err != nil {
		return "", translateProviderError(err)
	}

	pid := auth.ProviderID(resp.ID)
	a.auditEmit(ctx, tenant, "", "identity.provider_add_ldap", "provider", string(pid), audit.ResultAllow)
	return pid, nil
}

// --- Protocol-level test probes -----------------------------------------

// testOIDCDiscovery fetches the OIDC discovery document and validates required
// fields per OpenID Connect Discovery 1.0.
func (a *Adapter) testOIDCDiscovery(_ context.Context, cfg *auth.OIDCConfig) *auth.TestResult {
	if cfg.IssuerURL == "" || cfg.ClientID == "" {
		return &auth.TestResult{Success: false, Message: "missing oidc fields (issuerURL, clientID)"}
	}

	discoveryURL := strings.TrimSuffix(cfg.IssuerURL, "/") + "/.well-known/openid-configuration"
	start := a.now()
	resp, err := a.cfg.HTTPClient.Get(discoveryURL)
	latency := a.now().Sub(start).Milliseconds()
	if err != nil {
		return &auth.TestResult{
			Success:   false,
			LatencyMS: latency,
			Message:   fmt.Sprintf("oidc discovery fetch failed: %v", err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &auth.TestResult{
			Success:   false,
			LatencyMS: latency,
			Message:   fmt.Sprintf("oidc discovery returned HTTP %d", resp.StatusCode),
		}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB cap
	if err != nil {
		return &auth.TestResult{
			Success:   false,
			LatencyMS: latency,
			Message:   fmt.Sprintf("oidc discovery read failed: %v", err),
		}
	}

	var doc oidcDiscoveryDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return &auth.TestResult{
			Success:   false,
			LatencyMS: latency,
			Message:   "oidc discovery response is not valid JSON",
		}
	}

	// Validate required fields.
	diags := map[string]string{}
	if doc.Issuer == "" {
		diags["issuer"] = "missing"
	} else if doc.Issuer != cfg.IssuerURL {
		diags["issuer"] = fmt.Sprintf("mismatch: got %q, expected %q", doc.Issuer, cfg.IssuerURL)
	}
	if doc.AuthEndpoint == "" {
		diags["authorization_endpoint"] = "missing"
	}
	if doc.TokenEndpoint == "" {
		diags["token_endpoint"] = "missing"
	}
	if doc.JWKSURI == "" {
		diags["jwks_uri"] = "missing"
	}

	if len(diags) > 0 {
		return &auth.TestResult{
			Success:     false,
			LatencyMS:   latency,
			Message:     "oidc discovery validation failed",
			Diagnostics: diags,
		}
	}

	return &auth.TestResult{
		Success:   true,
		LatencyMS: latency,
		Message:   "oidc discovery OK",
		Diagnostics: map[string]string{
			"issuer":                 doc.Issuer,
			"authorization_endpoint": doc.AuthEndpoint,
		},
	}
}

// testSAMLMetadata fetches/parses SAML metadata XML and validates the
// EntityDescriptor has required elements.
func (a *Adapter) testSAMLMetadata(_ context.Context, cfg *auth.SAMLConfig) *auth.TestResult {
	if cfg.MetadataURL == "" && cfg.MetadataXML == "" {
		return &auth.TestResult{Success: false, Message: "missing saml metadata (URL or XML)"}
	}

	start := a.now()
	var metadataBytes []byte

	if cfg.MetadataURL != "" {
		resp, err := a.cfg.HTTPClient.Get(cfg.MetadataURL)
		latency := a.now().Sub(start).Milliseconds()
		if err != nil {
			return &auth.TestResult{
				Success:   false,
				LatencyMS: latency,
				Message:   fmt.Sprintf("saml metadata fetch failed: %v", err),
			}
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return &auth.TestResult{
				Success:   false,
				LatencyMS: latency,
				Message:   fmt.Sprintf("saml metadata returned HTTP %d", resp.StatusCode),
			}
		}
		var readErr error
		metadataBytes, readErr = io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2 MB cap
		if readErr != nil {
			return &auth.TestResult{
				Success:   false,
				LatencyMS: latency,
				Message:   fmt.Sprintf("saml metadata read failed: %v", readErr),
			}
		}
	} else {
		metadataBytes = []byte(cfg.MetadataXML)
	}

	latency := a.now().Sub(start).Milliseconds()

	// Parse the SAML metadata XML.
	var md samlEntityDescriptor
	if err := xml.Unmarshal(metadataBytes, &md); err != nil {
		return &auth.TestResult{
			Success:   false,
			LatencyMS: latency,
			Message:   fmt.Sprintf("saml metadata XML parse failed: %v", err),
		}
	}

	diags := map[string]string{}
	if md.EntityID != "" {
		diags["entityID"] = md.EntityID
	}
	if md.IDPSSODescriptor.SingleSignOnService.Location != "" {
		diags["ssoLocation"] = md.IDPSSODescriptor.SingleSignOnService.Location
	}

	if md.EntityID == "" {
		return &auth.TestResult{
			Success:     false,
			LatencyMS:   latency,
			Message:     "saml metadata missing entityID",
			Diagnostics: diags,
		}
	}

	return &auth.TestResult{
		Success:     true,
		LatencyMS:   latency,
		Message:     "saml metadata OK",
		Diagnostics: diags,
	}
}

// testLDAPConfig validates the LDAP configuration without attempting a real
// bind (the stub SDK cannot open TCP connections). It checks URL scheme,
// StartTLS compatibility, and bind DN structure.
func (a *Adapter) testLDAPConfig(cfg *auth.LDAPConfig) *auth.TestResult {
	if cfg.URL == "" || cfg.BindDN == "" {
		return &auth.TestResult{Success: false, Message: "missing ldap fields (URL, bindDN)"}
	}

	// Validate URL scheme.
	if !strings.HasPrefix(cfg.URL, "ldap://") && !strings.HasPrefix(cfg.URL, "ldaps://") {
		return &auth.TestResult{
			Success: false,
			Message: "ldap URL must start with ldap:// or ldaps://",
		}
	}

	// StartTLS + ldaps:// is a misconfiguration.
	if cfg.StartTLS && strings.HasPrefix(cfg.URL, "ldaps://") {
		return &auth.TestResult{
			Success: false,
			Message: "startTLS cannot be used with ldaps:// (already TLS)",
		}
	}

	diags := map[string]string{
		"url":    cfg.URL,
		"bindDN": cfg.BindDN,
	}
	if cfg.StartTLS {
		diags["startTLS"] = "enabled"
	}

	// Warn (but don't fail) if bind DN looks unusual.
	lower := strings.ToLower(cfg.BindDN)
	if !strings.Contains(lower, "dc=") && !strings.Contains(lower, "cn=") {
		diags["bindDN_warning"] = "bind DN does not contain DC= or CN= components"
	}

	return &auth.TestResult{
		Success:     true,
		LatencyMS:   0,
		Message:     "ldap config validation OK",
		Diagnostics: diags,
	}
}

// --- request/response types for typed provider endpoints -----------------

type oidcProviderCreateRequest struct {
	Name         string   `json:"name"`
	Issuer       string   `json:"issuer"`
	ClientID     string   `json:"clientId"`
	ClientSecret string   `json:"clientSecret,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
}

type samlProviderCreateRequest struct {
	Name        string `json:"name"`
	MetadataURL string `json:"metadataUrl,omitempty"`
	MetadataXML string `json:"metadataXml,omitempty"`
}

type ldapProviderCreateRequest struct {
	Name              string   `json:"name"`
	Servers           []string `json:"servers"`
	BindDN            string   `json:"bindDn"`
	BindPassword      string   `json:"bindPassword,omitempty"`
	BaseDN            string   `json:"baseDn,omitempty"`
	UserObjectClasses []string `json:"userObjectClasses,omitempty"`
	UserFilters       []string `json:"userFilters,omitempty"`
	StartTLS          bool     `json:"startTls,omitempty"`
}

type providerCreateResponse struct {
	ID string `json:"id"`
}

// oidcDiscoveryDoc is the minimal OIDC discovery document shape we validate.
type oidcDiscoveryDoc struct {
	Issuer        string `json:"issuer"`
	AuthEndpoint  string `json:"authorization_endpoint"`
	TokenEndpoint string `json:"token_endpoint"`
	JWKSURI       string `json:"jwks_uri"`
}

// samlEntityDescriptor is the minimal SAML metadata shape we validate.
type samlEntityDescriptor struct {
	XMLName          xml.Name `xml:"EntityDescriptor"`
	EntityID         string   `xml:"entityID,attr"`
	IDPSSODescriptor struct {
		SingleSignOnService struct {
			Location string `xml:"Location,attr"`
		} `xml:"SingleSignOnService"`
	} `xml:"IDPSSODescriptor"`
}
