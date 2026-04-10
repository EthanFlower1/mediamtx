package zitadel

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test doubles ---

type fakeClient struct {
	// Auth
	verifyPasswordErr error
	verifyPasswordUID string
	createSessionSess *auth.Session
	createSessionErr  error
	refreshSess       *auth.Session
	refreshErr        error
	introspectClaims  *auth.Claims
	introspectErr     error
	revokeErr         error

	// SSO
	beginAuthURL   string
	beginState     string
	beginErr       error
	exchangeSess   *auth.Session
	exchangeErr    error

	// User CRUD
	listUsersResult []*auth.User
	listUsersErr    error
	getUserResult   *auth.User
	getUserErr      error
	createUserResult *auth.User
	createUserErr   error
	updateUserResult *auth.User
	updateUserErr   error
	deleteUserErr   error

	// Groups
	listGroupsResult []*auth.Group
	listGroupsErr    error
	addGroupErr      error
	removeGroupErr   error

	// Provider
	configureErr error
	testResult   *auth.TestResult
	testErr      error
}

func (f *fakeClient) VerifyPassword(_ context.Context, _, _, _ string) (string, string, error) {
	if f.verifyPasswordErr != nil {
		return "", "", f.verifyPasswordErr
	}
	return f.verifyPasswordUID, "session-token", nil
}
func (f *fakeClient) CreateSession(_ context.Context, _ string) (*auth.Session, error) {
	return f.createSessionSess, f.createSessionErr
}
func (f *fakeClient) RefreshSession(_ context.Context, _ string) (*auth.Session, error) {
	return f.refreshSess, f.refreshErr
}
func (f *fakeClient) IntrospectToken(_ context.Context, _ string) (*auth.Claims, error) {
	return f.introspectClaims, f.introspectErr
}
func (f *fakeClient) RevokeSession(_ context.Context, _ string) error { return f.revokeErr }
func (f *fakeClient) BeginAuthFlow(_ context.Context, _, _, _ string) (string, string, error) {
	return f.beginAuthURL, f.beginState, f.beginErr
}
func (f *fakeClient) ExchangeCode(_ context.Context, _, _, _ string) (*auth.Session, error) {
	return f.exchangeSess, f.exchangeErr
}
func (f *fakeClient) ListUsers(_ context.Context, _, _ string, _ int, _ string) ([]*auth.User, error) {
	return f.listUsersResult, f.listUsersErr
}
func (f *fakeClient) GetUser(_ context.Context, _, _ string) (*auth.User, error) {
	return f.getUserResult, f.getUserErr
}
func (f *fakeClient) CreateUser(_ context.Context, _ string, _ auth.UserSpec) (*auth.User, error) {
	return f.createUserResult, f.createUserErr
}
func (f *fakeClient) UpdateUser(_ context.Context, _, _ string, _ auth.UserUpdate) (*auth.User, error) {
	return f.updateUserResult, f.updateUserErr
}
func (f *fakeClient) DeleteUser(_ context.Context, _, _ string) error { return f.deleteUserErr }
func (f *fakeClient) ListGroups(_ context.Context, _ string) ([]*auth.Group, error) {
	return f.listGroupsResult, f.listGroupsErr
}
func (f *fakeClient) AddUserToGroup(_ context.Context, _, _, _ string) error { return f.addGroupErr }
func (f *fakeClient) RemoveUserFromGroup(_ context.Context, _, _, _ string) error {
	return f.removeGroupErr
}
func (f *fakeClient) ConfigureProvider(_ context.Context, _ string, _ auth.ProviderConfig) error {
	return f.configureErr
}
func (f *fakeClient) TestProvider(_ context.Context, _ string, _ auth.ProviderConfig) (*auth.TestResult, error) {
	return f.testResult, f.testErr
}

var testTenant = auth.TenantRef{Type: auth.TenantTypeCustomer, ID: "tenant-1"}
var testOrgs = &StaticOrgResolver{OrgID: "org-1"}

func testSession() *auth.Session {
	return &auth.Session{
		ID:           "sess-1",
		UserID:       "user-1",
		Tenant:       testTenant,
		AccessToken:  "at-1",
		RefreshToken: "rt-1",
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(time.Hour),
	}
}

// --- tests ---

func TestNewAdapter_Success(t *testing.T) {
	a, err := NewAdapter(&fakeClient{}, testOrgs)
	require.NoError(t, err)
	assert.NotNil(t, a)
}

func TestNewAdapter_NilClient(t *testing.T) {
	_, err := NewAdapter(nil, testOrgs)
	assert.Error(t, err)
}

func TestNewAdapter_NilOrgs(t *testing.T) {
	_, err := NewAdapter(&fakeClient{}, nil)
	assert.Error(t, err)
}

func TestAuthenticateLocal_Success(t *testing.T) {
	client := &fakeClient{
		verifyPasswordUID: "user-1",
		createSessionSess: testSession(),
	}
	a, _ := NewAdapter(client, testOrgs)

	sess, err := a.AuthenticateLocal(context.Background(), testTenant, "admin", "password")
	require.NoError(t, err)
	assert.Equal(t, auth.UserID("user-1"), sess.UserID)
}

func TestAuthenticateLocal_BadPassword(t *testing.T) {
	client := &fakeClient{verifyPasswordErr: fmt.Errorf("wrong password")}
	a, _ := NewAdapter(client, testOrgs)

	_, err := a.AuthenticateLocal(context.Background(), testTenant, "admin", "wrong")
	assert.ErrorIs(t, err, auth.ErrInvalidCredentials)
}

func TestAuthenticateLocal_BadOrg(t *testing.T) {
	a, _ := NewAdapter(&fakeClient{}, &StaticOrgResolver{})

	_, err := a.AuthenticateLocal(context.Background(), testTenant, "admin", "pass")
	assert.ErrorIs(t, err, auth.ErrInvalidCredentials)
}

func TestVerifyToken_Success(t *testing.T) {
	claims := &auth.Claims{UserID: "user-1", TenantRef: testTenant}
	client := &fakeClient{introspectClaims: claims}
	a, _ := NewAdapter(client, testOrgs)

	result, err := a.VerifyToken(context.Background(), "valid-token")
	require.NoError(t, err)
	assert.Equal(t, auth.UserID("user-1"), result.UserID)
}

func TestVerifyToken_Invalid(t *testing.T) {
	client := &fakeClient{introspectErr: fmt.Errorf("expired")}
	a, _ := NewAdapter(client, testOrgs)

	_, err := a.VerifyToken(context.Background(), "bad-token")
	assert.ErrorIs(t, err, auth.ErrTokenInvalid)
}

func TestRefreshSession_Success(t *testing.T) {
	sess := testSession()
	client := &fakeClient{refreshSess: sess}
	a, _ := NewAdapter(client, testOrgs)

	result, err := a.RefreshSession(context.Background(), "rt-1")
	require.NoError(t, err)
	assert.Equal(t, sess.AccessToken, result.AccessToken)
}

func TestRefreshSession_NotFound(t *testing.T) {
	client := &fakeClient{refreshErr: fmt.Errorf("not found")}
	a, _ := NewAdapter(client, testOrgs)

	_, err := a.RefreshSession(context.Background(), "bad-rt")
	assert.ErrorIs(t, err, auth.ErrSessionNotFound)
}

func TestBeginSSOFlow(t *testing.T) {
	client := &fakeClient{beginAuthURL: "https://zitadel/auth", beginState: "state-abc"}
	a, _ := NewAdapter(client, testOrgs)

	result, err := a.BeginSSOFlow(context.Background(), testTenant, "oidc-provider", "http://localhost/callback")
	require.NoError(t, err)
	assert.Equal(t, "https://zitadel/auth", result.AuthURL)
	assert.Equal(t, "state-abc", result.State)
}

func TestCompleteSSOFlow_Success(t *testing.T) {
	sess := testSession()
	client := &fakeClient{exchangeSess: sess}
	a, _ := NewAdapter(client, testOrgs)

	result, err := a.CompleteSSOFlow(context.Background(), testTenant, "state-abc", "code-123")
	require.NoError(t, err)
	assert.Equal(t, sess.UserID, result.UserID)
}

func TestCompleteSSOFlow_InvalidState(t *testing.T) {
	client := &fakeClient{exchangeErr: fmt.Errorf("invalid state")}
	a, _ := NewAdapter(client, testOrgs)

	_, err := a.CompleteSSOFlow(context.Background(), testTenant, "bad-state", "code")
	assert.ErrorIs(t, err, auth.ErrSSOStateInvalid)
}

func TestGetUser_Success(t *testing.T) {
	user := &auth.User{ID: "user-1", Tenant: testTenant, Username: "admin"}
	client := &fakeClient{getUserResult: user}
	a, _ := NewAdapter(client, testOrgs)

	result, err := a.GetUser(context.Background(), testTenant, "user-1")
	require.NoError(t, err)
	assert.Equal(t, "admin", result.Username)
}

func TestGetUser_NotFound(t *testing.T) {
	client := &fakeClient{getUserErr: fmt.Errorf("not found")}
	a, _ := NewAdapter(client, testOrgs)

	_, err := a.GetUser(context.Background(), testTenant, "user-999")
	assert.ErrorIs(t, err, auth.ErrUserNotFound)
}

func TestGetUser_TenantMismatch(t *testing.T) {
	otherTenant := auth.TenantRef{Type: auth.TenantTypeCustomer, ID: "other-tenant"}
	user := &auth.User{ID: "user-1", Tenant: otherTenant}
	client := &fakeClient{getUserResult: user}
	a, _ := NewAdapter(client, testOrgs)

	_, err := a.GetUser(context.Background(), testTenant, "user-1")
	assert.ErrorIs(t, err, auth.ErrTenantMismatch)
}

func TestCreateUser_Success(t *testing.T) {
	user := &auth.User{ID: "user-new", Tenant: testTenant, Username: "newuser"}
	client := &fakeClient{createUserResult: user}
	a, _ := NewAdapter(client, testOrgs)

	result, err := a.CreateUser(context.Background(), testTenant, auth.UserSpec{Username: "newuser"})
	require.NoError(t, err)
	assert.Equal(t, "newuser", result.Username)
}

func TestCreateUser_Exists(t *testing.T) {
	client := &fakeClient{createUserErr: fmt.Errorf("already exists")}
	a, _ := NewAdapter(client, testOrgs)

	_, err := a.CreateUser(context.Background(), testTenant, auth.UserSpec{Username: "dup"})
	assert.ErrorIs(t, err, auth.ErrUserExists)
}

func TestListUsers(t *testing.T) {
	users := []*auth.User{{ID: "u1"}, {ID: "u2"}}
	client := &fakeClient{listUsersResult: users}
	a, _ := NewAdapter(client, testOrgs)

	result, err := a.ListUsers(context.Background(), testTenant, auth.ListOptions{})
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestListGroups(t *testing.T) {
	groups := []*auth.Group{{ID: "admin"}, {ID: "operator"}}
	client := &fakeClient{listGroupsResult: groups}
	a, _ := NewAdapter(client, testOrgs)

	result, err := a.ListGroups(context.Background(), testTenant)
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestAddUserToGroup(t *testing.T) {
	a, _ := NewAdapter(&fakeClient{}, testOrgs)
	err := a.AddUserToGroup(context.Background(), testTenant, "user-1", "admin")
	assert.NoError(t, err)
}

func TestRemoveUserFromGroup(t *testing.T) {
	a, _ := NewAdapter(&fakeClient{}, testOrgs)
	err := a.RemoveUserFromGroup(context.Background(), testTenant, "user-1", "admin")
	assert.NoError(t, err)
}

func TestConfigureProvider(t *testing.T) {
	a, _ := NewAdapter(&fakeClient{}, testOrgs)
	err := a.ConfigureProvider(context.Background(), testTenant, auth.ProviderConfig{
		Kind: auth.ProviderKindOIDC,
	})
	assert.NoError(t, err)
}

func TestTestProvider(t *testing.T) {
	client := &fakeClient{testResult: &auth.TestResult{Success: true, LatencyMS: 42}}
	a, _ := NewAdapter(client, testOrgs)

	result, err := a.TestProvider(context.Background(), testTenant, auth.ProviderConfig{})
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, int64(42), result.LatencyMS)
}

func TestStaticOrgResolver_Success(t *testing.T) {
	r := &StaticOrgResolver{OrgID: "org-1"}
	id, err := r.ResolveOrg(context.Background(), testTenant)
	require.NoError(t, err)
	assert.Equal(t, "org-1", id)
}

func TestStaticOrgResolver_Empty(t *testing.T) {
	r := &StaticOrgResolver{}
	_, err := r.ResolveOrg(context.Background(), testTenant)
	assert.Error(t, err)
}

func TestRevokeSession(t *testing.T) {
	a, _ := NewAdapter(&fakeClient{}, testOrgs)
	err := a.RevokeSession(context.Background(), "sess-1")
	assert.NoError(t, err)
}

func TestDeleteUser(t *testing.T) {
	a, _ := NewAdapter(&fakeClient{}, testOrgs)
	err := a.DeleteUser(context.Background(), testTenant, "user-1")
	assert.NoError(t, err)
}

func TestUpdateUser(t *testing.T) {
	user := &auth.User{ID: "user-1", Tenant: testTenant, Username: "updated"}
	client := &fakeClient{updateUserResult: user}
	a, _ := NewAdapter(client, testOrgs)

	name := "New Name"
	result, err := a.UpdateUser(context.Background(), testTenant, "user-1", auth.UserUpdate{DisplayName: &name})
	require.NoError(t, err)
	assert.Equal(t, "updated", result.Username)
}
