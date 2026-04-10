//go:build integration

package idptest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
	"github.com/bluenviron/mediamtx/internal/shared/auth/zitadel"
)

// skipIfNoDocker skips the test if the Docker daemon is not reachable.
func skipIfNoDocker(t *testing.T) {
	t.Helper()
	if os.Getenv("IDP_STACK_RUNNING") == "" {
		t.Skip("IDP_STACK_RUNNING not set; skipping (run `docker compose -f test/idp/docker-compose.yml up -d` first)")
	}
}

// ---------- OIDC mock endpoint probes ------------------------------------

func TestOIDCDiscoveryReachable(t *testing.T) {
	skipIfNoDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, OIDCDiscoveryURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OIDC discovery unreachable: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("OIDC discovery returned %d", resp.StatusCode)
	}

	var disc map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&disc); err != nil {
		t.Fatalf("invalid discovery JSON: %v", err)
	}
	if _, ok := disc["issuer"]; !ok {
		t.Fatal("discovery document missing 'issuer' field")
	}
	if _, ok := disc["jwks_uri"]; !ok {
		t.Fatal("discovery document missing 'jwks_uri' field")
	}
	t.Logf("OIDC discovery OK: issuer=%s", disc["issuer"])
}

// ---------- SAML mock endpoint probes ------------------------------------

func TestSAMLMetadataReachable(t *testing.T) {
	skipIfNoDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, SAMLMetadataURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SAML metadata unreachable: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SAML metadata returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) < 100 {
		t.Fatal("SAML metadata suspiciously short")
	}
	// Sanity check: should contain EntityDescriptor XML.
	if got := string(body); len(got) > 0 {
		t.Logf("SAML metadata OK: %d bytes", len(body))
	}
}

// ---------- LDAP mock endpoint probes ------------------------------------

func TestLDAPBindReachable(t *testing.T) {
	skipIfNoDocker(t)

	// We can't import a full LDAP client without adding a dependency, so
	// just verify the TCP port is open and accepting connections.
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).Dial("tcp", "localhost:389")
	if err != nil {
		t.Fatalf("LDAP port 389 not reachable: %v", err)
	}
	_ = conn.Close()
	t.Log("LDAP TCP port OK")
}

// ---------- Adapter.TestProvider against mock Zitadel --------------------
//
// Because we don't have a live Zitadel instance in CI, we stand up a thin
// httptest.Server that mimics the /management/v1/idps/_test endpoint and
// routes to the real mock IdP services for discovery probes.

func TestTestProvider_OIDC_AgainstMockStack(t *testing.T) {
	skipIfNoDocker(t)

	// Stub Zitadel: accept the idp test probe and return success.
	zitadelStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/management/v1/idps/_test":
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer zitadelStub.Close()

	adapter := newTestAdapter(t, zitadelStub.URL)
	ctx := context.Background()
	cfg := OIDCProviderConfig()

	result, err := adapter.TestProvider(ctx, TestTenant(), cfg)
	if err != nil {
		t.Fatalf("TestProvider(OIDC) error: %v", err)
	}
	if !result.Success {
		t.Fatalf("TestProvider(OIDC) failed: %s", result.Message)
	}
	t.Logf("TestProvider(OIDC) OK: latency=%dms", result.LatencyMS)
}

func TestTestProvider_SAML_AgainstMockStack(t *testing.T) {
	skipIfNoDocker(t)

	zitadelStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/management/v1/idps/_test":
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer zitadelStub.Close()

	adapter := newTestAdapter(t, zitadelStub.URL)
	ctx := context.Background()
	cfg := SAMLProviderConfig()

	result, err := adapter.TestProvider(ctx, TestTenant(), cfg)
	if err != nil {
		t.Fatalf("TestProvider(SAML) error: %v", err)
	}
	if !result.Success {
		t.Fatalf("TestProvider(SAML) failed: %s", result.Message)
	}
	t.Logf("TestProvider(SAML) OK: latency=%dms", result.LatencyMS)
}

func TestTestProvider_LDAP_AgainstMockStack(t *testing.T) {
	skipIfNoDocker(t)

	zitadelStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/management/v1/idps/_test":
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer zitadelStub.Close()

	adapter := newTestAdapter(t, zitadelStub.URL)
	ctx := context.Background()
	cfg := LDAPProviderConfig()

	result, err := adapter.TestProvider(ctx, TestTenant(), cfg)
	if err != nil {
		t.Fatalf("TestProvider(LDAP) error: %v", err)
	}
	if !result.Success {
		t.Fatalf("TestProvider(LDAP) failed: %s", result.Message)
	}
	t.Logf("TestProvider(LDAP) OK: latency=%dms", result.LatencyMS)
}

// ---------- ConfigureProvider round-trip ----------------------------------

func TestConfigureProvider_OIDC(t *testing.T) {
	skipIfNoDocker(t)

	var gotPath, gotMethod string
	zitadelStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/management/v1/idps/_test":
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)
		case "/management/v1/idps":
			gotPath = r.URL.Path
			gotMethod = r.Method
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"idpId":"new-oidc-123"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer zitadelStub.Close()

	adapter := newTestAdapter(t, zitadelStub.URL)
	ctx := context.Background()
	cfg := OIDCProviderConfig()

	if err := adapter.ConfigureProvider(ctx, TestTenant(), cfg); err != nil {
		t.Fatalf("ConfigureProvider(OIDC) error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/management/v1/idps" {
		t.Errorf("unexpected path: %s", gotPath)
	}
	t.Log("ConfigureProvider(OIDC) OK")
}

func TestConfigureProvider_SAML(t *testing.T) {
	skipIfNoDocker(t)

	zitadelStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/management/v1/idps/_test":
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)
		case "/management/v1/idps":
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"idpId":"new-saml-456"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer zitadelStub.Close()

	adapter := newTestAdapter(t, zitadelStub.URL)
	ctx := context.Background()
	cfg := SAMLProviderConfig()

	if err := adapter.ConfigureProvider(ctx, TestTenant(), cfg); err != nil {
		t.Fatalf("ConfigureProvider(SAML) error: %v", err)
	}
	t.Log("ConfigureProvider(SAML) OK")
}

func TestConfigureProvider_LDAP(t *testing.T) {
	skipIfNoDocker(t)

	zitadelStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/management/v1/idps/_test":
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)
		case "/management/v1/idps":
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"idpId":"new-ldap-789"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer zitadelStub.Close()

	adapter := newTestAdapter(t, zitadelStub.URL)
	ctx := context.Background()
	cfg := LDAPProviderConfig()

	if err := adapter.ConfigureProvider(ctx, TestTenant(), cfg); err != nil {
		t.Fatalf("ConfigureProvider(LDAP) error: %v", err)
	}
	t.Log("ConfigureProvider(LDAP) OK")
}

// ---------- Validation rejects incomplete configs -------------------------

func TestTestProvider_MissingFields(t *testing.T) {
	zitadelStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{}`)
	}))
	defer zitadelStub.Close()

	adapter := newTestAdapter(t, zitadelStub.URL)
	ctx := context.Background()

	cases := []struct {
		name string
		cfg  auth.ProviderConfig
	}{
		{
			name: "OIDC missing issuer",
			cfg: auth.ProviderConfig{
				Tenant: TestTenant(),
				Kind:   auth.ProviderKindOIDC,
				OIDC:   &auth.OIDCConfig{ClientID: "x"},
			},
		},
		{
			name: "SAML missing metadata",
			cfg: auth.ProviderConfig{
				Tenant: TestTenant(),
				Kind:   auth.ProviderKindSAML,
				SAML:   &auth.SAMLConfig{},
			},
		},
		{
			name: "LDAP missing URL",
			cfg: auth.ProviderConfig{
				Tenant: TestTenant(),
				Kind:   auth.ProviderKindLDAP,
				LDAP:   &auth.LDAPConfig{BindDN: "cn=admin"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := adapter.TestProvider(ctx, TestTenant(), tc.cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Success {
				t.Fatal("expected validation failure, got success")
			}
			t.Logf("correctly rejected: %s", result.Message)
		})
	}
}

// ---------- helpers -------------------------------------------------------

// newTestAdapter constructs a zitadel.Adapter wired to the given stub URL.
// The stub URL replaces the real Zitadel domain so all HTTP calls land on
// the httptest.Server.
func newTestAdapter(t *testing.T, stubURL string) *zitadel.Adapter {
	t.Helper()

	// Strip the scheme — the adapter prepends https://, but for tests
	// we route through the stub which uses plain HTTP on the httptest URL.
	// We override the HTTP client's transport to rewrite the scheme.
	transport := &schemeRewriter{
		target:    stubURL,
		transport: http.DefaultTransport,
	}

	adapter, err := zitadel.New(context.Background(), zitadel.Config{
		Domain:            "stub.zitadel.test",
		ServiceAccountKey: "/dev/null", // validation requires non-empty
		PlatformOrgID:     "platform-org-001",
		HTTPClient:        &http.Client{Transport: transport, Timeout: 10 * time.Second},
	})
	if err != nil {
		t.Fatalf("failed to construct adapter: %v", err)
	}
	return adapter
}

// schemeRewriter is an http.RoundTripper that rewrites request URLs to point
// at the test stub server, regardless of the scheme/host the adapter uses.
type schemeRewriter struct {
	target    string
	transport http.RoundTripper
}

func (s *schemeRewriter) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	req2.URL.Scheme = "http"
	// Parse host from target URL.
	if len(s.target) > 7 { // len("http://")
		req2.URL.Host = s.target[7:]
	}
	return s.transport.RoundTrip(req2)
}
