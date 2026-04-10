package zitadel

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test double ---

type fakeAdminAPI struct {
	defaultOrgID string
	createdOrgs  map[string]string // name → orgID
	saUsers      map[string]string // name → userID
	apps         map[string]string // name → clientID
	keyCounter   int

	createOrgErr error
	createSAErr  error
	createKeyErr error
	createAppErr error
}

func newFakeAdminAPI() *fakeAdminAPI {
	return &fakeAdminAPI{
		createdOrgs: make(map[string]string),
		saUsers:     make(map[string]string),
		apps:        make(map[string]string),
	}
}

func (f *fakeAdminAPI) GetDefaultOrg(_ context.Context) (string, error) {
	return f.defaultOrgID, nil
}

func (f *fakeAdminAPI) CreateOrg(_ context.Context, name string) (string, error) {
	if f.createOrgErr != nil {
		return "", f.createOrgErr
	}
	id := fmt.Sprintf("org-%s", name)
	f.createdOrgs[name] = id
	f.defaultOrgID = id
	return id, nil
}

func (f *fakeAdminAPI) CreateServiceAccount(_ context.Context, orgID, name, description string) (string, error) {
	if f.createSAErr != nil {
		return "", f.createSAErr
	}
	id := fmt.Sprintf("sa-%s", name)
	f.saUsers[name] = id
	return id, nil
}

func (f *fakeAdminAPI) CreateServiceAccountKey(_ context.Context, userID string) (string, []byte, error) {
	if f.createKeyErr != nil {
		return "", nil, f.createKeyErr
	}
	f.keyCounter++
	keyID := fmt.Sprintf("key-%d", f.keyCounter)
	keyJSON := []byte(fmt.Sprintf(`{"userId":"%s","keyId":"%s"}`, userID, keyID))
	return keyID, keyJSON, nil
}

func (f *fakeAdminAPI) CreateOIDCApp(_ context.Context, orgID string, cfg OIDCAppConfig) (string, string, error) {
	if f.createAppErr != nil {
		return "", "", f.createAppErr
	}
	clientID := fmt.Sprintf("client-%s", cfg.Name)
	f.apps[cfg.Name] = clientID
	return clientID, "secret-" + clientID, nil
}

func (f *fakeAdminAPI) GetAppByName(_ context.Context, orgID, name string) (string, error) {
	return f.apps[name], nil
}

func (f *fakeAdminAPI) GetServiceAccountByName(_ context.Context, orgID, name string) (string, error) {
	return f.saUsers[name], nil
}

// --- tests ---

func TestNewBootstrapper_Success(t *testing.T) {
	b, err := NewBootstrapper(newFakeAdminAPI(), BootstrapConfig{})
	require.NoError(t, err)
	assert.NotNil(t, b)
}

func TestNewBootstrapper_NilAPI(t *testing.T) {
	_, err := NewBootstrapper(nil, BootstrapConfig{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "AdminAPI")
}

func TestBootstrap_FullFirstRun(t *testing.T) {
	api := newFakeAdminAPI()
	b, err := NewBootstrapper(api, BootstrapConfig{
		OrgName:            "TestOrg",
		ServiceAccountName: "test-sa",
		DirectoryOIDCApp: OIDCAppConfig{
			Name:         "directory-web",
			RedirectURIs: []string{"http://localhost:8080/callback"},
		},
		FlutterOIDCApp: OIDCAppConfig{
			Name:     "flutter-app",
			AppType:  "NATIVE",
			AuthMethod: "NONE",
		},
	})
	require.NoError(t, err)

	result, err := b.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "org-TestOrg", result.OrgID)
	assert.Equal(t, "sa-test-sa", result.ServiceAccountUserID)
	assert.NotEmpty(t, result.ServiceAccountKeyID)
	assert.NotEmpty(t, result.ServiceAccountKeyJSON)
	assert.Equal(t, "client-directory-web", result.DirectoryClientID)
	assert.Equal(t, "secret-client-directory-web", result.DirectoryClientSecret)
	assert.Equal(t, "client-flutter-app", result.FlutterClientID)
	assert.NotEmpty(t, result.FlutterClientSecret)
}

func TestBootstrap_Idempotent(t *testing.T) {
	api := newFakeAdminAPI()
	// Pre-populate: org, SA, and apps already exist.
	api.defaultOrgID = "existing-org"
	api.saUsers["directory-sa"] = "existing-sa-user"
	api.apps["directory-web"] = "existing-client"

	b, err := NewBootstrapper(api, BootstrapConfig{
		DirectoryOIDCApp: OIDCAppConfig{Name: "directory-web"},
	})
	require.NoError(t, err)

	result, err := b.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "existing-org", result.OrgID)
	assert.Equal(t, "existing-sa-user", result.ServiceAccountUserID)
	assert.Equal(t, "existing-client", result.DirectoryClientID)
	assert.Empty(t, result.DirectoryClientSecret) // not returned for existing apps
}

func TestBootstrap_OrgCreationFailure(t *testing.T) {
	api := newFakeAdminAPI()
	api.createOrgErr = fmt.Errorf("zitadel unavailable")

	b, err := NewBootstrapper(api, BootstrapConfig{})
	require.NoError(t, err)

	_, err = b.Run(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "org")
}

func TestBootstrap_SACreationFailure(t *testing.T) {
	api := newFakeAdminAPI()
	api.createSAErr = fmt.Errorf("permission denied")

	b, err := NewBootstrapper(api, BootstrapConfig{})
	require.NoError(t, err)

	_, err = b.Run(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service account")
}

func TestBootstrap_KeyCreationFailure(t *testing.T) {
	api := newFakeAdminAPI()
	api.createKeyErr = fmt.Errorf("key generation failed")

	b, err := NewBootstrapper(api, BootstrapConfig{})
	require.NoError(t, err)

	_, err = b.Run(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service account")
}

func TestBootstrap_AppCreationFailure(t *testing.T) {
	api := newFakeAdminAPI()
	api.createAppErr = fmt.Errorf("app quota exceeded")

	b, err := NewBootstrapper(api, BootstrapConfig{
		DirectoryOIDCApp: OIDCAppConfig{Name: "test-app"},
	})
	require.NoError(t, err)

	_, err = b.Run(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "directory OIDC")
}

func TestBootstrap_NoApps(t *testing.T) {
	api := newFakeAdminAPI()
	b, err := NewBootstrapper(api, BootstrapConfig{})
	require.NoError(t, err)

	result, err := b.Run(context.Background())
	require.NoError(t, err)

	assert.NotEmpty(t, result.OrgID)
	assert.NotEmpty(t, result.ServiceAccountUserID)
	assert.Empty(t, result.DirectoryClientID) // no app configured
	assert.Empty(t, result.FlutterClientID)
}

func TestBootstrap_DefaultNames(t *testing.T) {
	api := newFakeAdminAPI()
	b, err := NewBootstrapper(api, BootstrapConfig{})
	require.NoError(t, err)

	result, err := b.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "org-Kaivue", result.OrgID)
	assert.Equal(t, "sa-directory-sa", result.ServiceAccountUserID)
}

func TestBootstrap_Result(t *testing.T) {
	api := newFakeAdminAPI()
	b, err := NewBootstrapper(api, BootstrapConfig{})
	require.NoError(t, err)

	assert.Nil(t, b.Result())

	_, err = b.Run(context.Background())
	require.NoError(t, err)

	assert.NotNil(t, b.Result())
	assert.NotEmpty(t, b.Result().OrgID)
}

func TestBootstrapResult_Serialization(t *testing.T) {
	result := &BootstrapResult{
		OrgID:                "org-1",
		ServiceAccountUserID: "sa-1",
		ServiceAccountKeyID:  "key-1",
		ServiceAccountKeyJSON: []byte(`{"test":true}`),
		DirectoryClientID:    "client-1",
	}

	data, err := result.ToJSON()
	require.NoError(t, err)

	parsed, err := ParseBootstrapResult(data)
	require.NoError(t, err)

	assert.Equal(t, result.OrgID, parsed.OrgID)
	assert.Equal(t, result.ServiceAccountUserID, parsed.ServiceAccountUserID)
	assert.Equal(t, result.DirectoryClientID, parsed.DirectoryClientID)
}

func TestBootstrap_ExistingSAGetsNewKey(t *testing.T) {
	api := newFakeAdminAPI()
	api.saUsers["directory-sa"] = "existing-sa"

	b, err := NewBootstrapper(api, BootstrapConfig{})
	require.NoError(t, err)

	result, err := b.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "existing-sa", result.ServiceAccountUserID)
	assert.NotEmpty(t, result.ServiceAccountKeyID) // new key generated
	assert.NotEmpty(t, result.ServiceAccountKeyJSON)
}
