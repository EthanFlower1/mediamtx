package zitadel

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"strings"
	"testing"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// --- OIDC provider tests ------------------------------------------------

func TestAddOIDCProvider_Happy(t *testing.T) {
	// Two HTTP calls: discovery fetch (via HTTPClient.Get) + Zitadel create.
	// The fakeRoundTripper handles both in order.
	discoveryDoc := mustJSON(t, oidcDiscoveryDoc{
		Issuer:        "https://accounts.google.com",
		AuthEndpoint:  "https://accounts.google.com/o/oauth2/v2/auth",
		TokenEndpoint: "https://oauth2.googleapis.com/token",
		JWKSURI:       "https://www.googleapis.com/oauth2/v3/certs",
	})
	createResp := mustJSON(t, providerCreateResponse{ID: "idp_oidc_123"})

	a, rt, rec := newTestAdapter(t,
		okResp(discoveryDoc), // OIDC discovery fetch
		okResp(createResp),   // Zitadel create
	)

	pid, err := a.AddOIDCProvider(context.Background(), custTenant(), auth.OIDCConfig{
		IssuerURL:    "https://accounts.google.com",
		ClientID:     "my-client-id",
		ClientSecret: "my-secret",
	}, "Google")
	if err != nil {
		t.Fatalf("AddOIDCProvider: %v", err)
	}
	if pid != "idp_oidc_123" {
		t.Fatalf("expected provider ID idp_oidc_123, got %s", pid)
	}

	// Verify the discovery URL was fetched.
	if len(rt.requests) < 1 {
		t.Fatal("expected at least 1 request")
	}
	if !strings.Contains(rt.requests[0].Path, ".well-known/openid-configuration") {
		t.Errorf("expected discovery URL in first request, got %s", rt.requests[0].Path)
	}

	// Verify the create request went to the right endpoint.
	if len(rt.requests) < 2 {
		t.Fatal("expected 2 requests")
	}
	if rt.requests[1].Path != "/management/v1/idps/oidc" {
		t.Errorf("expected create path /management/v1/idps/oidc, got %s", rt.requests[1].Path)
	}
	if rt.requests[1].OrgID != "org_cust_1" {
		t.Errorf("expected org scope org_cust_1, got %s", rt.requests[1].OrgID)
	}

	// Verify the request body contains the right fields.
	var body oidcProviderCreateRequest
	if err := json.Unmarshal(rt.requests[1].Body, &body); err != nil {
		t.Fatalf("unmarshal create body: %v", err)
	}
	if body.Issuer != "https://accounts.google.com" {
		t.Errorf("wrong issuer in body: %s", body.Issuer)
	}
	if body.ClientID != "my-client-id" {
		t.Errorf("wrong clientId in body: %s", body.ClientID)
	}
	// Default scopes should be applied.
	if len(body.Scopes) != 3 {
		t.Errorf("expected 3 default scopes, got %v", body.Scopes)
	}

	// Audit entry emitted.
	if len(rec.entries) == 0 {
		t.Error("expected audit entry for provider_add_oidc")
	}
	if rec.entries[0].Action != "identity.provider_add_oidc" {
		t.Errorf("expected audit action identity.provider_add_oidc, got %s", rec.entries[0].Action)
	}
}

func TestAddOIDCProvider_MissingIssuer(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	_, err := a.AddOIDCProvider(context.Background(), custTenant(), auth.OIDCConfig{
		ClientID: "cid",
	}, "Bad")
	if err == nil {
		t.Fatal("expected error for missing issuer")
	}
	if !strings.Contains(err.Error(), "issuer URL is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAddOIDCProvider_MissingClientID(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	_, err := a.AddOIDCProvider(context.Background(), custTenant(), auth.OIDCConfig{
		IssuerURL: "https://example.com",
	}, "Bad")
	if err == nil {
		t.Fatal("expected error for missing clientID")
	}
}

func TestAddOIDCProvider_DiscoveryFailure(t *testing.T) {
	// Discovery returns 404.
	a, _, _ := newTestAdapter(t, errResp(404, "not found"))
	_, err := a.AddOIDCProvider(context.Background(), custTenant(), auth.OIDCConfig{
		IssuerURL: "https://bad-issuer.example.com",
		ClientID:  "cid",
	}, "Bad IDP")
	if err == nil {
		t.Fatal("expected error when discovery fails")
	}
	if !strings.Contains(err.Error(), "oidc discovery returned HTTP 404") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAddOIDCProvider_DiscoveryIssuerMismatch(t *testing.T) {
	// Discovery returns a document with a different issuer.
	doc := mustJSON(t, oidcDiscoveryDoc{
		Issuer:        "https://wrong-issuer.example.com",
		AuthEndpoint:  "https://example.com/auth",
		TokenEndpoint: "https://example.com/token",
		JWKSURI:       "https://example.com/jwks",
	})
	a, _, _ := newTestAdapter(t, okResp(doc))
	_, err := a.AddOIDCProvider(context.Background(), custTenant(), auth.OIDCConfig{
		IssuerURL: "https://expected-issuer.example.com",
		ClientID:  "cid",
	}, "Mismatch")
	if err == nil {
		t.Fatal("expected error for issuer mismatch")
	}
	if !strings.Contains(err.Error(), "oidc discovery validation failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAddOIDCProvider_ZeroTenantRejected(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	_, err := a.AddOIDCProvider(context.Background(), auth.TenantRef{}, auth.OIDCConfig{
		IssuerURL: "https://example.com",
		ClientID:  "cid",
	}, "X")
	if !errors.Is(err, auth.ErrTenantMismatch) {
		t.Fatalf("want ErrTenantMismatch, got %v", err)
	}
}

func TestAddOIDCProvider_CustomScopes(t *testing.T) {
	doc := mustJSON(t, oidcDiscoveryDoc{
		Issuer:        "https://example.com",
		AuthEndpoint:  "https://example.com/auth",
		TokenEndpoint: "https://example.com/token",
		JWKSURI:       "https://example.com/jwks",
	})
	a, rt, _ := newTestAdapter(t,
		okResp(doc),
		okResp(mustJSON(t, providerCreateResponse{ID: "idp_1"})),
	)
	_, err := a.AddOIDCProvider(context.Background(), custTenant(), auth.OIDCConfig{
		IssuerURL: "https://example.com",
		ClientID:  "cid",
		Scopes:    []string{"openid", "groups"},
	}, "Custom")
	if err != nil {
		t.Fatalf("AddOIDCProvider: %v", err)
	}
	var body oidcProviderCreateRequest
	json.Unmarshal(rt.requests[1].Body, &body)
	if len(body.Scopes) != 2 || body.Scopes[1] != "groups" {
		t.Errorf("expected custom scopes [openid groups], got %v", body.Scopes)
	}
}

func TestAddOIDCProvider_ZitadelCreateError(t *testing.T) {
	doc := mustJSON(t, oidcDiscoveryDoc{
		Issuer:        "https://example.com",
		AuthEndpoint:  "https://example.com/auth",
		TokenEndpoint: "https://example.com/token",
		JWKSURI:       "https://example.com/jwks",
	})
	a, _, _ := newTestAdapter(t,
		okResp(doc),
		errResp(409, "already exists"),
	)
	_, err := a.AddOIDCProvider(context.Background(), custTenant(), auth.OIDCConfig{
		IssuerURL: "https://example.com",
		ClientID:  "cid",
	}, "Dup")
	if err == nil {
		t.Fatal("expected error on Zitadel 409")
	}
}

// --- SAML provider tests ------------------------------------------------

func TestAddSAMLProvider_HappyURL(t *testing.T) {
	samlMeta := validSAMLMetadata("https://idp.example.com", "https://idp.example.com/sso")
	createResp := mustJSON(t, providerCreateResponse{ID: "idp_saml_456"})
	a, rt, rec := newTestAdapter(t,
		okResp(samlMeta),   // metadata fetch
		okResp(createResp), // Zitadel create
	)

	pid, err := a.AddSAMLProvider(context.Background(), custTenant(), auth.SAMLConfig{
		MetadataURL: "https://idp.example.com/metadata",
	}, "Enterprise SAML")
	if err != nil {
		t.Fatalf("AddSAMLProvider: %v", err)
	}
	if pid != "idp_saml_456" {
		t.Fatalf("expected idp_saml_456, got %s", pid)
	}
	if rt.requests[1].Path != "/management/v1/idps/saml" {
		t.Errorf("expected saml create path, got %s", rt.requests[1].Path)
	}
	if len(rec.entries) == 0 || rec.entries[0].Action != "identity.provider_add_saml" {
		t.Error("expected audit entry for provider_add_saml")
	}
}

func TestAddSAMLProvider_HappyXML(t *testing.T) {
	// Inline XML — no HTTP fetch needed, just the Zitadel create call.
	createResp := mustJSON(t, providerCreateResponse{ID: "idp_saml_789"})
	a, _, _ := newTestAdapter(t,
		okResp(createResp), // Zitadel create
	)

	pid, err := a.AddSAMLProvider(context.Background(), custTenant(), auth.SAMLConfig{
		MetadataXML: string(validSAMLMetadata("https://inline.example.com", "https://inline.example.com/sso")),
	}, "Inline SAML")
	if err != nil {
		t.Fatalf("AddSAMLProvider: %v", err)
	}
	if pid != "idp_saml_789" {
		t.Fatalf("expected idp_saml_789, got %s", pid)
	}
}

func TestAddSAMLProvider_MissingMetadata(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	_, err := a.AddSAMLProvider(context.Background(), custTenant(), auth.SAMLConfig{}, "Bad")
	if err == nil {
		t.Fatal("expected error for missing metadata")
	}
}

func TestAddSAMLProvider_InvalidXML(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	_, err := a.AddSAMLProvider(context.Background(), custTenant(), auth.SAMLConfig{
		MetadataXML: "<not-valid-saml/>",
	}, "Bad XML")
	if err == nil {
		t.Fatal("expected error for invalid SAML XML")
	}
}

func TestAddSAMLProvider_MissingEntityID(t *testing.T) {
	// Valid XML structure but missing entityID attribute.
	noEntityID := `<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata">
		<IDPSSODescriptor>
			<SingleSignOnService Location="https://sso.example.com"/>
		</IDPSSODescriptor>
	</EntityDescriptor>`
	a, _, _ := newTestAdapter(t)
	_, err := a.AddSAMLProvider(context.Background(), custTenant(), auth.SAMLConfig{
		MetadataXML: noEntityID,
	}, "No Entity")
	if err == nil {
		t.Fatal("expected error for missing entityID")
	}
	if !strings.Contains(err.Error(), "missing entityID") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAddSAMLProvider_MetadataFetchFails(t *testing.T) {
	a, _, _ := newTestAdapter(t, errResp(500, "server error"))
	_, err := a.AddSAMLProvider(context.Background(), custTenant(), auth.SAMLConfig{
		MetadataURL: "https://broken.example.com/metadata",
	}, "Broken")
	if err == nil {
		t.Fatal("expected error on metadata fetch failure")
	}
}

// --- LDAP provider tests ------------------------------------------------

func TestAddLDAPProvider_Happy(t *testing.T) {
	createResp := mustJSON(t, providerCreateResponse{ID: "idp_ldap_321"})
	a, rt, rec := newTestAdapter(t, okResp(createResp))

	pid, err := a.AddLDAPProvider(context.Background(), custTenant(), auth.LDAPConfig{
		URL:          "ldaps://ad.corp.example.com:636",
		BindDN:       "cn=svc,dc=corp,dc=example,dc=com",
		BindPassword: "secret",
		UserBaseDN:   "ou=users,dc=corp,dc=example,dc=com",
		UserFilter:   "(sAMAccountName={0})",
	}, "Corp AD")
	if err != nil {
		t.Fatalf("AddLDAPProvider: %v", err)
	}
	if pid != "idp_ldap_321" {
		t.Fatalf("expected idp_ldap_321, got %s", pid)
	}
	if rt.requests[0].Path != "/management/v1/idps/ldap" {
		t.Errorf("expected ldap create path, got %s", rt.requests[0].Path)
	}

	var body ldapProviderCreateRequest
	json.Unmarshal(rt.requests[0].Body, &body)
	if body.BindDN != "cn=svc,dc=corp,dc=example,dc=com" {
		t.Errorf("wrong bindDN: %s", body.BindDN)
	}
	if len(body.Servers) != 1 || body.Servers[0] != "ldaps://ad.corp.example.com:636" {
		t.Errorf("wrong servers: %v", body.Servers)
	}

	if len(rec.entries) == 0 || rec.entries[0].Action != "identity.provider_add_ldap" {
		t.Error("expected audit entry for provider_add_ldap")
	}
}

func TestAddLDAPProvider_MissingURL(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	_, err := a.AddLDAPProvider(context.Background(), custTenant(), auth.LDAPConfig{
		BindDN: "cn=x",
	}, "Bad")
	if err == nil {
		t.Fatal("expected error for missing URL")
	}
}

func TestAddLDAPProvider_MissingBindDN(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	_, err := a.AddLDAPProvider(context.Background(), custTenant(), auth.LDAPConfig{
		URL: "ldap://host",
	}, "Bad")
	if err == nil {
		t.Fatal("expected error for missing bindDN")
	}
}

func TestAddLDAPProvider_BadURLScheme(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	_, err := a.AddLDAPProvider(context.Background(), custTenant(), auth.LDAPConfig{
		URL:    "http://not-ldap:389",
		BindDN: "cn=svc,dc=example,dc=com",
	}, "Bad Scheme")
	if err == nil {
		t.Fatal("expected error for non-ldap URL scheme")
	}
	if !strings.Contains(err.Error(), "ldap:// or ldaps://") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAddLDAPProvider_StartTLSWithLDAPS(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	_, err := a.AddLDAPProvider(context.Background(), custTenant(), auth.LDAPConfig{
		URL:      "ldaps://host:636",
		BindDN:   "cn=svc,dc=example,dc=com",
		StartTLS: true,
	}, "Bad TLS")
	if err == nil {
		t.Fatal("expected error for startTLS+ldaps")
	}
	if !strings.Contains(err.Error(), "startTLS cannot be used with ldaps://") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAddLDAPProvider_StartTLSWithLDAP(t *testing.T) {
	// StartTLS with ldap:// is valid.
	createResp := mustJSON(t, providerCreateResponse{ID: "idp_ldap_tls"})
	a, _, _ := newTestAdapter(t, okResp(createResp))
	pid, err := a.AddLDAPProvider(context.Background(), custTenant(), auth.LDAPConfig{
		URL:      "ldap://host:389",
		BindDN:   "cn=svc,dc=example,dc=com",
		StartTLS: true,
	}, "TLS OK")
	if err != nil {
		t.Fatalf("AddLDAPProvider: %v", err)
	}
	if pid != "idp_ldap_tls" {
		t.Fatalf("unexpected pid: %s", pid)
	}
}

func TestAddLDAPProvider_ZeroTenantRejected(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	_, err := a.AddLDAPProvider(context.Background(), auth.TenantRef{}, auth.LDAPConfig{
		URL:    "ldap://host",
		BindDN: "cn=x",
	}, "X")
	if !errors.Is(err, auth.ErrTenantMismatch) {
		t.Fatalf("want ErrTenantMismatch, got %v", err)
	}
}

func TestAddLDAPProvider_DefaultUserFilter(t *testing.T) {
	createResp := mustJSON(t, providerCreateResponse{ID: "idp_ldap_df"})
	a, rt, _ := newTestAdapter(t, okResp(createResp))
	_, err := a.AddLDAPProvider(context.Background(), custTenant(), auth.LDAPConfig{
		URL:    "ldap://host:389",
		BindDN: "cn=svc,dc=example,dc=com",
		// No UserFilter — should default.
	}, "Default Filter")
	if err != nil {
		t.Fatalf("AddLDAPProvider: %v", err)
	}
	var body ldapProviderCreateRequest
	json.Unmarshal(rt.requests[0].Body, &body)
	if len(body.UserFilters) != 1 || body.UserFilters[0] != "(objectClass=inetOrgPerson)" {
		t.Errorf("expected default user filter, got %v", body.UserFilters)
	}
}

// --- Protocol-level test probe tests ------------------------------------

func TestTestOIDCDiscovery_InvalidJSON(t *testing.T) {
	a, _, _ := newTestAdapter(t, okResp([]byte("not json")))
	result := a.testOIDCDiscovery(context.Background(), &auth.OIDCConfig{
		IssuerURL: "https://example.com",
		ClientID:  "cid",
	})
	if result.Success {
		t.Fatal("expected failure for invalid JSON")
	}
	if !strings.Contains(result.Message, "not valid JSON") {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

func TestTestOIDCDiscovery_MissingFields(t *testing.T) {
	// Incomplete discovery doc.
	doc := mustJSON(t, map[string]string{"issuer": "https://example.com"})
	a, _, _ := newTestAdapter(t, okResp(doc))
	result := a.testOIDCDiscovery(context.Background(), &auth.OIDCConfig{
		IssuerURL: "https://example.com",
		ClientID:  "cid",
	})
	if result.Success {
		t.Fatal("expected failure for missing fields")
	}
	if result.Diagnostics == nil {
		t.Fatal("expected diagnostics")
	}
}

func TestTestSAMLMetadata_InlineXML(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	result := a.testSAMLMetadata(context.Background(), &auth.SAMLConfig{
		MetadataXML: string(validSAMLMetadata("https://idp.example.com", "https://idp.example.com/sso")),
	})
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Message)
	}
	if result.Diagnostics["entityID"] != "https://idp.example.com" {
		t.Errorf("expected entityID in diagnostics, got %v", result.Diagnostics)
	}
}

func TestTestLDAPConfig_ValidLDAPS(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	result := a.testLDAPConfig(&auth.LDAPConfig{
		URL:    "ldaps://ad.example.com:636",
		BindDN: "cn=svc,dc=example,dc=com",
	})
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Message)
	}
}

func TestTestLDAPConfig_InvalidScheme(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	result := a.testLDAPConfig(&auth.LDAPConfig{
		URL:    "https://not-ldap",
		BindDN: "cn=svc",
	})
	if result.Success {
		t.Fatal("expected failure for https scheme")
	}
}

// --- Existing TestProvider integration tests ----------------------------

func TestTestProvider_OIDC_EnhancedProbe(t *testing.T) {
	// The existing TestProvider method should still work with OIDC configs.
	// It validates input and delegates to Zitadel _test endpoint.
	a, _, _ := newTestAdapter(t, okResp([]byte(`{}`)))
	res, err := a.TestProvider(context.Background(), custTenant(), auth.ProviderConfig{
		Kind:        auth.ProviderKindOIDC,
		DisplayName: "Test OIDC",
		OIDC: &auth.OIDCConfig{
			IssuerURL: "https://example.com",
			ClientID:  "cid",
		},
	})
	if err != nil {
		t.Fatalf("TestProvider: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Message)
	}
}

func TestTestProvider_SAML_MissingFields(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	res, err := a.TestProvider(context.Background(), custTenant(), auth.ProviderConfig{
		Kind: auth.ProviderKindSAML,
		SAML: &auth.SAMLConfig{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatal("expected failure for empty SAML config")
	}
}

func TestTestProvider_LDAP_MissingFields(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	res, err := a.TestProvider(context.Background(), custTenant(), auth.ProviderConfig{
		Kind: auth.ProviderKindLDAP,
		LDAP: &auth.LDAPConfig{URL: "ldap://host"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatal("expected failure for missing bindDN")
	}
}

func TestTestProvider_UnknownKind(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	res, err := a.TestProvider(context.Background(), custTenant(), auth.ProviderConfig{
		Kind: "kerberos",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatal("expected failure for unknown provider kind")
	}
}

func TestTestProvider_TenantMismatch(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	res, err := a.TestProvider(context.Background(), custTenant(), auth.ProviderConfig{
		Tenant: otherTenant(),
		Kind:   auth.ProviderKindOIDC,
		OIDC:   &auth.OIDCConfig{IssuerURL: "https://x.com", ClientID: "cid"},
	})
	if !errors.Is(err, auth.ErrTenantMismatch) {
		t.Fatalf("want ErrTenantMismatch, got err=%v res=%+v", err, res)
	}
}

// --- helpers ------------------------------------------------------------

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustJSON: %v", err)
	}
	return b
}

// validSAMLMetadata returns a minimal valid SAML EntityDescriptor XML.
func validSAMLMetadata(entityID, ssoLocation string) []byte {
	md := struct {
		XMLName  xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:metadata EntityDescriptor"`
		EntityID string   `xml:"entityID,attr"`
		IDPSSO   struct {
			SSO struct {
				Binding  string `xml:"Binding,attr"`
				Location string `xml:"Location,attr"`
			} `xml:"urn:oasis:names:tc:SAML:2.0:metadata SingleSignOnService"`
		} `xml:"urn:oasis:names:tc:SAML:2.0:metadata IDPSSODescriptor"`
	}{
		EntityID: entityID,
	}
	md.IDPSSO.SSO.Binding = "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect"
	md.IDPSSO.SSO.Location = ssoLocation
	b, _ := xml.Marshal(md)
	return b
}
