package zitadel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// --- test plumbing --------------------------------------------------------

// fixture loads a JSON file from testdata/ and returns its bytes.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("load fixture %s: %v", name, err)
	}
	return b
}

// recordedRequest captures a single request for assertions.
type recordedRequest struct {
	Method string
	Path   string
	OrgID  string
	Body   []byte
}

// fakeResponse is one canned response the fake round-tripper returns in order.
type fakeResponse struct {
	Status int
	Body   []byte
}

// fakeRoundTripper returns canned responses in FIFO order and records
// every request it sees for later assertion.
type fakeRoundTripper struct {
	mu        sync.Mutex
	responses []fakeResponse
	requests  []recordedRequest
}

func (f *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	f.requests = append(f.requests, recordedRequest{
		Method: req.Method,
		Path:   req.URL.Path,
		OrgID:  req.Header.Get("x-zitadel-orgid"),
		Body:   body,
	})
	if len(f.responses) == 0 {
		return nil, errors.New("fakeRoundTripper: no more canned responses")
	}
	resp := f.responses[0]
	f.responses = f.responses[1:]
	return &http.Response{
		StatusCode: resp.Status,
		Body:       io.NopCloser(bytes.NewReader(resp.Body)),
		Header:     make(http.Header),
	}, nil
}

// memRecorder is a minimal audit.Recorder for tests.
type memRecorder struct {
	mu      sync.Mutex
	entries []audit.Entry
}

func (m *memRecorder) Record(_ context.Context, e audit.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, e)
	return nil
}
func (m *memRecorder) Query(context.Context, audit.QueryFilter) ([]audit.Entry, error) {
	return nil, nil
}
func (m *memRecorder) Export(context.Context, audit.QueryFilter, audit.ExportFormat, io.Writer) error {
	return nil
}

// newTestAdapter constructs an Adapter wired to a fakeRoundTripper with
// the given canned responses. It primes the tenant-org cache with two
// tenants so tests don't have to call Bootstrap helpers first.
func newTestAdapter(t *testing.T, responses ...fakeResponse) (*Adapter, *fakeRoundTripper, *memRecorder) {
	t.Helper()
	rt := &fakeRoundTripper{responses: responses}
	rec := &memRecorder{}
	cfg := Config{
		Domain:            "zitadel.test",
		ServiceAccountKey: "testdata/REPLACE_ME.json",
		PlatformOrgID:     "org_platform",
		HTTPClient:        &http.Client{Transport: rt, Timeout: 5 * time.Second},
		AuditRecorder:     rec,
		Now:               func() time.Time { return time.Unix(1712500000, 0) },
	}
	a, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Prime two tenants: customer org_cust_1, integrator org_int_1.
	a.RegisterTenantMapping(auth.TenantRef{Type: auth.TenantTypeCustomer, ID: "org_cust_1"}, "org_cust_1")
	a.RegisterTenantMapping(auth.TenantRef{Type: auth.TenantTypeIntegrator, ID: "org_int_1"}, "org_int_1")
	return a, rt, rec
}

func okResp(body []byte) fakeResponse    { return fakeResponse{Status: 200, Body: body} }
func errResp(code int, msg string) fakeResponse {
	b, _ := json.Marshal(map[string]string{"code": "ERR", "message": msg})
	return fakeResponse{Status: code, Body: b}
}

func custTenant() auth.TenantRef {
	return auth.TenantRef{Type: auth.TenantTypeCustomer, ID: "org_cust_1"}
}
func otherTenant() auth.TenantRef {
	return auth.TenantRef{Type: auth.TenantTypeCustomer, ID: "org_other"}
}

// --- tests ---------------------------------------------------------------

func TestNew_RejectsEmptyConfig(t *testing.T) {
	if _, err := New(context.Background(), Config{}); err == nil {
		t.Fatal("expected error for empty config")
	}
	if _, err := New(context.Background(), Config{Domain: "x"}); err == nil {
		t.Fatal("expected error for missing ServiceAccountKey")
	}
	if _, err := New(context.Background(), Config{Domain: "x", ServiceAccountKey: "k"}); err == nil {
		t.Fatal("expected error for missing PlatformOrgID")
	}
}

func TestAuthenticateLocal_Happy(t *testing.T) {
	a, rt, rec := newTestAdapter(t, okResp(fixture(t, "session_ok.json")))
	sess, err := a.AuthenticateLocal(context.Background(), custTenant(), "alice", "hunter2")
	if err != nil {
		t.Fatalf("AuthenticateLocal: %v", err)
	}
	if sess.AccessToken != "tok_abc" || sess.UserID != "user_alice" {
		t.Fatalf("unexpected session: %+v", sess)
	}
	// Assert the request was scoped to the right org.
	if got := rt.requests[0].OrgID; got != "org_cust_1" {
		t.Errorf("expected x-zitadel-orgid=org_cust_1, got %q", got)
	}
	if !strings.Contains(string(rt.requests[0].Body), `"loginName":"alice"`) {
		t.Errorf("request body missing loginName: %s", rt.requests[0].Body)
	}
	// Audit emitted exactly one allow entry.
	if len(rec.entries) != 1 || rec.entries[0].Result != audit.ResultAllow {
		t.Errorf("expected 1 allow audit entry, got %+v", rec.entries)
	}
}

func TestAuthenticateLocal_InvalidCreds_NoEnumeration(t *testing.T) {
	// Both "unknown user" (404) and "wrong password" (401) must map to
	// the same sentinel — the adapter MUST NOT leak which check failed.
	a, _, _ := newTestAdapter(t,
		errResp(http.StatusNotFound, "user not found"),
	)
	_, err := a.AuthenticateLocal(context.Background(), custTenant(), "ghost", "x")
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}

	a2, _, _ := newTestAdapter(t,
		errResp(http.StatusUnauthorized, "wrong password"),
	)
	_, err = a2.AuthenticateLocal(context.Background(), custTenant(), "alice", "wrong")
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthenticateLocal_ZeroTenantRejected(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	_, err := a.AuthenticateLocal(context.Background(), auth.TenantRef{}, "alice", "x")
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials for zero tenant, got %v", err)
	}
}

func TestSSOFlow_RoundTrip(t *testing.T) {
	a, _, rec := newTestAdapter(t, okResp(fixture(t, "session_ok.json")))
	begin, err := a.BeginSSOFlow(context.Background(), custTenant(), "provider_google", "https://app/cb")
	if err != nil {
		t.Fatalf("BeginSSOFlow: %v", err)
	}
	if begin.State == "" || !strings.Contains(begin.AuthURL, "state=") {
		t.Fatalf("bad begin: %+v", begin)
	}

	sess, err := a.CompleteSSOFlow(context.Background(), custTenant(), begin.State, "code_xyz")
	if err != nil {
		t.Fatalf("CompleteSSOFlow: %v", err)
	}
	if sess.UserID != "user_alice" {
		t.Fatalf("unexpected user: %s", sess.UserID)
	}
	// Replay must fail.
	if _, err := a.CompleteSSOFlow(context.Background(), custTenant(), begin.State, "code_xyz"); !errors.Is(err, auth.ErrSSOStateInvalid) {
		t.Fatal("expected replay to fail with ErrSSOStateInvalid")
	}
	// Cross-tenant must fail even before replay (but here state is
	// already consumed, so still ErrSSOStateInvalid).
	if len(rec.entries) < 2 {
		t.Errorf("expected >=2 audit entries, got %d", len(rec.entries))
	}
}

func TestSSOFlow_CrossTenantRejected(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	begin, err := a.BeginSSOFlow(context.Background(), custTenant(), "p", "https://app/cb")
	if err != nil {
		t.Fatal(err)
	}
	// Attempt to complete with a different tenant.
	wrong := auth.TenantRef{Type: auth.TenantTypeCustomer, ID: "org_int_1"}
	if _, err := a.CompleteSSOFlow(context.Background(), wrong, begin.State, "code"); !errors.Is(err, auth.ErrSSOStateInvalid) {
		t.Fatalf("want ErrSSOStateInvalid, got %v", err)
	}
}

func TestRefreshSession_UnknownToken(t *testing.T) {
	a, _, _ := newTestAdapter(t, errResp(http.StatusUnauthorized, "expired"))
	if _, err := a.RefreshSession(context.Background(), "rt_bad"); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("want ErrSessionNotFound, got %v", err)
	}
}

func TestVerifyToken_Happy(t *testing.T) {
	a, _, _ := newTestAdapter(t, okResp(fixture(t, "introspect_active.json")))
	claims, err := a.VerifyToken(context.Background(), "tok")
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if claims.UserID != "user_alice" || claims.TenantRef.ID != "org_cust_1" {
		t.Fatalf("bad claims: %+v", claims)
	}
	if len(claims.Groups) != 2 {
		t.Errorf("expected 2 groups, got %v", claims.Groups)
	}
}

func TestVerifyToken_InactiveRejected(t *testing.T) {
	a, _, _ := newTestAdapter(t, okResp(fixture(t, "introspect_inactive.json")))
	if _, err := a.VerifyToken(context.Background(), "tok"); !errors.Is(err, auth.ErrTokenInvalid) {
		t.Fatalf("want ErrTokenInvalid, got %v", err)
	}
}

func TestVerifyToken_UnknownOrgRejected(t *testing.T) {
	// Introspect returns an active token but for an org we don't have
	// a tenant mapping for — fail closed.
	body := []byte(`{"active":true,"sub":"u1","urn:zitadel:iam:user:resourceowner:id":"org_stranger","exp":1,"iat":0,"sid":"s"}`)
	a, _, _ := newTestAdapter(t, okResp(body))
	if _, err := a.VerifyToken(context.Background(), "tok"); !errors.Is(err, auth.ErrTokenInvalid) {
		t.Fatalf("want ErrTokenInvalid for unknown org, got %v", err)
	}
}

func TestRevokeSession_Idempotent(t *testing.T) {
	a, _, _ := newTestAdapter(t, errResp(http.StatusNotFound, "gone"))
	if err := a.RevokeSession(context.Background(), "sess_gone"); err != nil {
		t.Fatalf("expected idempotent nil, got %v", err)
	}
}

func TestListUsers_TenantIsolation(t *testing.T) {
	// The fixture includes a cross-org user the adapter must filter out.
	a, _, _ := newTestAdapter(t, okResp(fixture(t, "users_list.json")))
	users, err := a.ListUsers(context.Background(), custTenant(), auth.ListOptions{})
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user after seam #4 filter, got %d (%+v)", len(users), users)
	}
	if users[0].Username != "alice" {
		t.Errorf("expected alice, got %s", users[0].Username)
	}
}

func TestCreateUser_Happy(t *testing.T) {
	a, rt, rec := newTestAdapter(t, okResp(fixture(t, "create_user_ok.json")))
	u, err := a.CreateUser(context.Background(), custTenant(), auth.UserSpec{
		Username:    "newbie",
		Email:       "n@x.com",
		DisplayName: "New Bie",
		Password:    "pw",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID != "user_new" {
		t.Fatalf("unexpected id: %s", u.ID)
	}
	if rt.requests[0].OrgID != "org_cust_1" {
		t.Errorf("expected org scope, got %q", rt.requests[0].OrgID)
	}
	if len(rec.entries) == 0 {
		t.Error("expected audit entry for user_create")
	}
}

func TestCreateUser_Conflict(t *testing.T) {
	a, _, _ := newTestAdapter(t, errResp(http.StatusConflict, "exists"))
	_, err := a.CreateUser(context.Background(), custTenant(), auth.UserSpec{Username: "dup"})
	if !errors.Is(err, auth.ErrUserExists) {
		t.Fatalf("want ErrUserExists, got %v", err)
	}
}

func TestUpdateUser_CrossTenantRejected(t *testing.T) {
	// GetUser returns a user whose resourceOwner is a different org.
	crossOrg := []byte(`{"userId":"user_x","details":{"resourceOwner":"org_stranger"},"human":{"username":"x","profile":{"displayName":"X"},"email":{"email":"x@x"}}}`)
	a, _, _ := newTestAdapter(t, okResp(crossOrg))
	_, err := a.UpdateUser(context.Background(), custTenant(), "user_x", auth.UserUpdate{})
	if !errors.Is(err, auth.ErrTenantMismatch) {
		t.Fatalf("want ErrTenantMismatch, got %v", err)
	}
}

func TestDeleteUser_NotFound(t *testing.T) {
	a, _, _ := newTestAdapter(t, errResp(http.StatusNotFound, "nope"))
	err := a.DeleteUser(context.Background(), custTenant(), "ghost")
	if !errors.Is(err, auth.ErrUserNotFound) {
		t.Fatalf("want ErrUserNotFound, got %v", err)
	}
}

func TestConfigureProvider_RequiresTestFirst(t *testing.T) {
	// TestProvider hits /idps/_test first; then ConfigureProvider hits
	// /idps. Both OK.
	a, rt, _ := newTestAdapter(t,
		okResp([]byte(`{}`)), // test probe
		okResp([]byte(`{}`)), // configure
	)
	cfg := auth.ProviderConfig{
		Kind:        auth.ProviderKindOIDC,
		DisplayName: "Google",
		OIDC: &auth.OIDCConfig{
			IssuerURL: "https://accounts.google.com",
			ClientID:  "cid",
		},
	}
	if err := a.ConfigureProvider(context.Background(), custTenant(), cfg); err != nil {
		t.Fatalf("ConfigureProvider: %v", err)
	}
	if len(rt.requests) != 2 {
		t.Errorf("expected 2 requests (test + configure), got %d", len(rt.requests))
	}
	if !strings.Contains(rt.requests[0].Path, "_test") {
		t.Errorf("expected first request to be the _test probe, got %s", rt.requests[0].Path)
	}
}

func TestTestProvider_MissingFieldsFails(t *testing.T) {
	a, _, _ := newTestAdapter(t)
	res, err := a.TestProvider(context.Background(), custTenant(), auth.ProviderConfig{
		Kind: auth.ProviderKindOIDC,
		OIDC: &auth.OIDCConfig{}, // empty
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Success {
		t.Fatal("expected Success=false for empty OIDC")
	}
}

func TestBootstrapIntegrator_Happy(t *testing.T) {
	a, rt, _ := newTestAdapter(t, okResp(fixture(t, "org_created.json")))
	orgID, err := a.BootstrapIntegrator(context.Background(), Integrator{
		TenantID:    "tenant_int_42",
		DisplayName: "ACME Resellers",
	})
	if err != nil {
		t.Fatalf("BootstrapIntegrator: %v", err)
	}
	if orgID != "org_new_12345" {
		t.Fatalf("unexpected org id: %s", orgID)
	}
	// Verify the create call was scoped to the platform org.
	if rt.requests[0].OrgID != "org_platform" {
		t.Errorf("expected platform-org scope, got %q", rt.requests[0].OrgID)
	}
	// Cache primed for later AuthenticateLocal calls.
	got, _ := a.orgIDFor(auth.TenantRef{Type: auth.TenantTypeIntegrator, ID: "tenant_int_42"})
	if got != "org_new_12345" {
		t.Errorf("cache not primed: got %q", got)
	}
}

func TestBootstrapCustomerTenant_Happy(t *testing.T) {
	a, _, _ := newTestAdapter(t, okResp(fixture(t, "org_created.json")))
	orgID, err := a.BootstrapCustomerTenant(context.Background(), CustomerTenant{
		TenantID:              "tenant_cust_7",
		DisplayName:           "Acme Hospital",
		ParentIntegratorOrgID: "org_int_1",
	})
	if err != nil {
		t.Fatalf("BootstrapCustomerTenant: %v", err)
	}
	if orgID == "" {
		t.Fatal("empty orgID")
	}
}

func TestNilAuditRecorder_DoesNotPanic(t *testing.T) {
	rt := &fakeRoundTripper{responses: []fakeResponse{okResp(fixture(t, "session_ok.json"))}}
	cfg := Config{
		Domain:            "zitadel.test",
		ServiceAccountKey: "k",
		PlatformOrgID:     "p",
		HTTPClient:        &http.Client{Transport: rt},
		Now:               func() time.Time { return time.Unix(0, 0) },
	}
	a, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	a.RegisterTenantMapping(custTenant(), "org_cust_1")
	if _, err := a.AuthenticateLocal(context.Background(), custTenant(), "alice", "pw"); err != nil {
		t.Fatalf("AuthenticateLocal: %v", err)
	}
}
