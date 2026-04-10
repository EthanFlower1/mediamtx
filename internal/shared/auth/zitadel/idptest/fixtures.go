//go:build integration

package idptest

import (
	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// ----- Well-known endpoints exposed by the Docker Compose stack -----

const (
	// OIDCIssuerURL is the mock OIDC discovery issuer served by
	// navikt/mock-oauth2-server under the "kaivue-test" issuer id.
	OIDCIssuerURL = "http://localhost:9090/kaivue-test"

	// OIDCDiscoveryURL is the full .well-known path for the mock OIDC provider.
	OIDCDiscoveryURL = OIDCIssuerURL + "/.well-known/openid-configuration"

	// OIDCClientID is the pre-registered client on the mock OIDC server.
	OIDCClientID = "kaivue-test-client"

	// OIDCClientSecret is the client secret. The mock server accepts any
	// value for client_credentials grants when interactiveLogin=false.
	OIDCClientSecret = "test-secret"

	// SAMLMetadataURL is the SAML IdP metadata endpoint served by
	// kristophjunge/test-saml-idp.
	SAMLMetadataURL = "http://localhost:8080/simplesaml/saml2/idp/metadata.php"

	// SAMLEntityID is the entityID the mock SAML IdP advertises.
	SAMLEntityID = "http://localhost:8080/simplesaml/saml2/idp/metadata.php"

	// LDAPHost is the OpenLDAP server address.
	LDAPHost = "ldap://localhost:389"

	// LDAPBaseDN is the root of the test directory.
	LDAPBaseDN = "dc=test,dc=kaivue,dc=io"

	// LDAPAdminBindDN is the admin bind for the test directory.
	LDAPAdminBindDN = "cn=admin,dc=test,dc=kaivue,dc=io"

	// LDAPAdminPassword is the bind password configured in Docker Compose.
	LDAPAdminPassword = "admin-secret"

	// LDAPUserBaseDN is the OU containing test user entries.
	LDAPUserBaseDN = "ou=People,dc=test,dc=kaivue,dc=io"

	// LDAPGroupBaseDN is the OU containing test group entries.
	LDAPGroupBaseDN = "ou=Groups,dc=test,dc=kaivue,dc=io"
)

// TestTenant returns a deterministic TenantRef for integration tests.
func TestTenant() auth.TenantRef {
	return auth.TenantRef{
		Type: auth.TenantTypeCustomer,
		ID:   "idp-test-org-001",
	}
}

// OIDCProviderConfig returns a ProviderConfig for the mock OIDC server.
func OIDCProviderConfig() auth.ProviderConfig {
	return auth.ProviderConfig{
		Tenant:      TestTenant(),
		Kind:        auth.ProviderKindOIDC,
		DisplayName: "Mock OIDC (CI)",
		Enabled:     true,
		OIDC: &auth.OIDCConfig{
			IssuerURL:    OIDCIssuerURL,
			ClientID:     OIDCClientID,
			ClientSecret: OIDCClientSecret,
			Scopes:       []string{"openid", "profile", "email"},
		},
	}
}

// SAMLProviderConfig returns a ProviderConfig for the mock SAML IdP.
func SAMLProviderConfig() auth.ProviderConfig {
	return auth.ProviderConfig{
		Tenant:      TestTenant(),
		Kind:        auth.ProviderKindSAML,
		DisplayName: "Mock SAML (CI)",
		Enabled:     true,
		SAML: &auth.SAMLConfig{
			MetadataURL: SAMLMetadataURL,
			EntityID:    SAMLEntityID,
		},
	}
}

// LDAPProviderConfig returns a ProviderConfig for the test OpenLDAP server.
func LDAPProviderConfig() auth.ProviderConfig {
	return auth.ProviderConfig{
		Tenant:      TestTenant(),
		Kind:        auth.ProviderKindLDAP,
		DisplayName: "Mock LDAP (CI)",
		Enabled:     true,
		LDAP: &auth.LDAPConfig{
			URL:          LDAPHost,
			BindDN:       LDAPAdminBindDN,
			BindPassword: LDAPAdminPassword,
			UserBaseDN:   LDAPUserBaseDN,
			UserFilter:   "(uid={{username}})",
			GroupBaseDN:  LDAPGroupBaseDN,
			GroupFilter:  "(uniqueMember={{dn}})",
		},
	}
}
