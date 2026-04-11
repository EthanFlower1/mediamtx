package federation_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/directory/federation"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
	"github.com/bluenviron/mediamtx/internal/shared/permissions"
	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
	"github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1/kaivuev1connect"
)

// --- test doubles ---

type memUserStore struct {
	users map[string][]federation.UserRecord // keyed by tenantID
	err   error
}

func (s *memUserStore) ListUsers(_ context.Context, tenantID, search string) ([]federation.UserRecord, error) {
	if s.err != nil {
		return nil, s.err
	}
	all := s.users[tenantID]
	if search == "" {
		return all, nil
	}
	var out []federation.UserRecord
	for _, u := range all {
		if contains(u.Username, search) || contains(u.Email, search) || contains(u.DisplayName, search) {
			out = append(out, u)
		}
	}
	return out, nil
}

type memGroupStore struct {
	groups map[string][]federation.GroupRecord
	err    error
}

func (s *memGroupStore) ListGroups(_ context.Context, tenantID string) ([]federation.GroupRecord, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.groups[tenantID], nil
}

type memCameraStore struct {
	cameras map[string][]federation.CameraRecord
	err     error
}

func (s *memCameraStore) ListCameras(_ context.Context, tenantID, recorderID, search string) ([]federation.CameraRecord, error) {
	if s.err != nil {
		return nil, s.err
	}
	all := s.cameras[tenantID]
	var out []federation.CameraRecord
	for _, c := range all {
		if recorderID != "" && c.RecorderID != recorderID {
			continue
		}
		if search != "" && !contains(c.Name, search) {
			continue
		}
		out = append(out, c)
	}
	if out == nil && search == "" && recorderID == "" {
		return all, nil
	}
	return out, nil
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && containsSubstring(s, sub)
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- test helpers ---

const (
	testPeerID = "peer-dir-001"
	testTenant = "tenant-A"
)

func testTenantRef() *kaivuev1.TenantRef {
	return &kaivuev1.TenantRef{Id: testTenant}
}

func newGrantManager(t *testing.T) *permissions.FederationGrantManager {
	t.Helper()
	store := permissions.NewInMemoryStore()
	enforcer, err := permissions.NewEnforcer(store, nil)
	require.NoError(t, err)
	return permissions.NewFederationGrantManager(enforcer)
}

func grantCamera(t *testing.T, gm *permissions.FederationGrantManager, peerID, tenantID, cameraID, action string) {
	t.Helper()
	err := gm.Grant(context.Background(), permissions.FederationGrant{
		PeerDirectoryID: peerID,
		ReceivingTenant: auth.TenantRef{ID: tenantID},
		ResourceType:    "cameras",
		ResourceID:      cameraID,
		Actions:         []string{action},
	})
	require.NoError(t, err)
}

func grantUser(t *testing.T, gm *permissions.FederationGrantManager, peerID, tenantID, userID string) {
	t.Helper()
	err := gm.Grant(context.Background(), permissions.FederationGrant{
		PeerDirectoryID: peerID,
		ReceivingTenant: auth.TenantRef{ID: tenantID},
		ResourceType:    "users",
		ResourceID:      userID,
		Actions:         []string{permissions.ActionUsersView},
	})
	require.NoError(t, err)
}

func grantGroup(t *testing.T, gm *permissions.FederationGrantManager, peerID, tenantID, groupID string) {
	t.Helper()
	err := gm.Grant(context.Background(), permissions.FederationGrant{
		PeerDirectoryID: peerID,
		ReceivingTenant: auth.TenantRef{ID: tenantID},
		ResourceType:    "groups",
		ResourceID:      groupID,
		Actions:         []string{permissions.ActionUsersView},
	})
	require.NoError(t, err)
}

func newCatalogHandler(t *testing.T, cfg federation.RPCConfig) *federation.RPCHandler {
	t.Helper()
	if cfg.ServerVersion == "" {
		cfg.ServerVersion = testVersion
	}
	if cfg.JWKSProvider == nil {
		cfg.JWKSProvider = &staticJWKSProvider{json: testJWKS, maxAge: 300}
	}
	h, err := federation.NewRPCHandler(cfg)
	require.NoError(t, err)
	return h
}

// peerContextMiddleware injects the peer directory ID into the request context
// by reading the X-Federation-Peer-ID header. This simulates what the mTLS
// middleware does in production.
type peerContextMiddleware struct {
	next http.Handler
}

func (m *peerContextMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	peerID := r.Header.Get("X-Federation-Peer-ID")
	if peerID != "" {
		ctx := context.WithValue(r.Context(), federation.PeerDirectoryIDKey, peerID)
		r = r.WithContext(ctx)
	}
	m.next.ServeHTTP(w, r)
}

func newCatalogClient(t *testing.T, handler *federation.RPCHandler) kaivuev1connect.FederationPeerServiceClient {
	t.Helper()
	_, h := kaivuev1connect.NewFederationPeerServiceHandler(handler)
	ts := httptest.NewServer(&peerContextMiddleware{next: h})
	t.Cleanup(ts.Close)
	return kaivuev1connect.NewFederationPeerServiceClient(http.DefaultClient, ts.URL)
}

func requestWithPeerID[T any](msg *T, peerID string) *connect.Request[T] {
	req := connect.NewRequest(msg)
	req.Header().Set("X-Federation-Peer-ID", peerID)
	return req
}

// --- ListUsers tests ---

func TestListUsers_WithGrants_ReturnsFilteredUsers(t *testing.T) {
	gm := newGrantManager(t)
	userStore := &memUserStore{users: map[string][]federation.UserRecord{
		testTenant: {
			{ID: "u1", TenantID: testTenant, Username: "alice", Email: "alice@example.com"},
			{ID: "u2", TenantID: testTenant, Username: "bob", Email: "bob@example.com"},
			{ID: "u3", TenantID: testTenant, Username: "charlie", Email: "charlie@example.com"},
		},
	}}

	// Grant peer access to u1 and u3 only.
	grantUser(t, gm, testPeerID, testTenant, "u1")
	grantUser(t, gm, testPeerID, testTenant, "u3")

	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		UserStore:    userStore,
	})
	client := newCatalogClient(t, handler)

	resp, err := client.ListUsers(context.Background(), requestWithPeerID(
		&kaivuev1.ListUsersRequest{Tenant: testTenantRef()}, testPeerID,
	))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.GetUsers(), 2)

	ids := make([]string, 0, 2)
	for _, u := range resp.Msg.GetUsers() {
		ids = append(ids, u.GetId())
	}
	assert.Contains(t, ids, "u1")
	assert.Contains(t, ids, "u3")
}

func TestListUsers_NoGrants_ReturnsEmpty(t *testing.T) {
	gm := newGrantManager(t)
	userStore := &memUserStore{users: map[string][]federation.UserRecord{
		testTenant: {
			{ID: "u1", TenantID: testTenant, Username: "alice"},
			{ID: "u2", TenantID: testTenant, Username: "bob"},
		},
	}}

	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		UserStore:    userStore,
	})
	client := newCatalogClient(t, handler)

	resp, err := client.ListUsers(context.Background(), requestWithPeerID(
		&kaivuev1.ListUsersRequest{Tenant: testTenantRef()}, testPeerID,
	))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.GetUsers())
	assert.Empty(t, resp.Msg.GetNextCursor())
}

func TestListUsers_MissingPeerID_ReturnsUnauthenticated(t *testing.T) {
	gm := newGrantManager(t)
	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		UserStore:    &memUserStore{users: map[string][]federation.UserRecord{}},
	})
	client := newCatalogClient(t, handler)

	// No peer ID header.
	_, err := client.ListUsers(context.Background(), connect.NewRequest(
		&kaivuev1.ListUsersRequest{Tenant: testTenantRef()},
	))
	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestListUsers_MissingTenant_ReturnsInvalidArgument(t *testing.T) {
	gm := newGrantManager(t)
	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		UserStore:    &memUserStore{users: map[string][]federation.UserRecord{}},
	})
	client := newCatalogClient(t, handler)

	_, err := client.ListUsers(context.Background(), requestWithPeerID(
		&kaivuev1.ListUsersRequest{}, testPeerID,
	))
	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestListUsers_Pagination(t *testing.T) {
	gm := newGrantManager(t)
	users := make([]federation.UserRecord, 10)
	for i := range users {
		id := fmt.Sprintf("u%02d", i)
		users[i] = federation.UserRecord{ID: id, TenantID: testTenant, Username: id}
		grantUser(t, gm, testPeerID, testTenant, id)
	}

	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		UserStore:    &memUserStore{users: map[string][]federation.UserRecord{testTenant: users}},
	})
	client := newCatalogClient(t, handler)

	// First page: 3 items.
	resp, err := client.ListUsers(context.Background(), requestWithPeerID(
		&kaivuev1.ListUsersRequest{
			Tenant:   testTenantRef(),
			PageSize: 3,
		}, testPeerID,
	))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.GetUsers(), 3)
	assert.NotEmpty(t, resp.Msg.GetNextCursor())

	// Second page.
	resp2, err := client.ListUsers(context.Background(), requestWithPeerID(
		&kaivuev1.ListUsersRequest{
			Tenant:   testTenantRef(),
			PageSize: 3,
			Cursor:   resp.Msg.GetNextCursor(),
		}, testPeerID,
	))
	require.NoError(t, err)
	assert.Len(t, resp2.Msg.GetUsers(), 3)

	// Verify no overlap between pages.
	firstIDs := map[string]bool{}
	for _, u := range resp.Msg.GetUsers() {
		firstIDs[u.GetId()] = true
	}
	for _, u := range resp2.Msg.GetUsers() {
		assert.False(t, firstIDs[u.GetId()], "page 2 should not contain page 1 user %s", u.GetId())
	}
}

func TestListUsers_SearchFilter(t *testing.T) {
	gm := newGrantManager(t)
	userStore := &memUserStore{users: map[string][]federation.UserRecord{
		testTenant: {
			{ID: "u1", TenantID: testTenant, Username: "alice-admin", Email: "alice@example.com"},
			{ID: "u2", TenantID: testTenant, Username: "bob-user", Email: "bob@example.com"},
		},
	}}
	grantUser(t, gm, testPeerID, testTenant, "u1")
	grantUser(t, gm, testPeerID, testTenant, "u2")

	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		UserStore:    userStore,
	})
	client := newCatalogClient(t, handler)

	resp, err := client.ListUsers(context.Background(), requestWithPeerID(
		&kaivuev1.ListUsersRequest{
			Tenant: testTenantRef(),
			Search: "alice",
		}, testPeerID,
	))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.GetUsers(), 1)
	assert.Equal(t, "u1", resp.Msg.GetUsers()[0].GetId())
}

func TestListUsers_StoreError(t *testing.T) {
	gm := newGrantManager(t)
	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		UserStore:    &memUserStore{err: errors.New("db offline")},
	})
	client := newCatalogClient(t, handler)

	_, err := client.ListUsers(context.Background(), requestWithPeerID(
		&kaivuev1.ListUsersRequest{Tenant: testTenantRef()}, testPeerID,
	))
	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeInternal, connectErr.Code())
}

// --- ListGroups tests ---

func TestListGroups_WithGrants_ReturnsFilteredGroups(t *testing.T) {
	gm := newGrantManager(t)
	groupStore := &memGroupStore{groups: map[string][]federation.GroupRecord{
		testTenant: {
			{ID: "g1", TenantID: testTenant, Name: "Admins", Description: "Admin group"},
			{ID: "g2", TenantID: testTenant, Name: "Viewers", Description: "View-only"},
			{ID: "g3", TenantID: testTenant, Name: "Operators", Description: "Ops"},
		},
	}}

	grantGroup(t, gm, testPeerID, testTenant, "g1")
	grantGroup(t, gm, testPeerID, testTenant, "g3")

	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		GroupStore:   groupStore,
	})
	client := newCatalogClient(t, handler)

	resp, err := client.ListGroups(context.Background(), requestWithPeerID(
		&kaivuev1.ListGroupsRequest{Tenant: testTenantRef()}, testPeerID,
	))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.GetGroups(), 2)

	names := make([]string, 0, 2)
	for _, g := range resp.Msg.GetGroups() {
		names = append(names, g.GetName())
	}
	assert.Contains(t, names, "Admins")
	assert.Contains(t, names, "Operators")
}

func TestListGroups_NoGrants_ReturnsEmpty(t *testing.T) {
	gm := newGrantManager(t)
	groupStore := &memGroupStore{groups: map[string][]federation.GroupRecord{
		testTenant: {
			{ID: "g1", TenantID: testTenant, Name: "Admins"},
		},
	}}

	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		GroupStore:   groupStore,
	})
	client := newCatalogClient(t, handler)

	resp, err := client.ListGroups(context.Background(), requestWithPeerID(
		&kaivuev1.ListGroupsRequest{Tenant: testTenantRef()}, testPeerID,
	))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.GetGroups())
}

func TestListGroups_MissingPeerID_ReturnsUnauthenticated(t *testing.T) {
	gm := newGrantManager(t)
	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		GroupStore:   &memGroupStore{groups: map[string][]federation.GroupRecord{}},
	})
	client := newCatalogClient(t, handler)

	_, err := client.ListGroups(context.Background(), connect.NewRequest(
		&kaivuev1.ListGroupsRequest{Tenant: testTenantRef()},
	))
	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestListGroups_Pagination(t *testing.T) {
	gm := newGrantManager(t)
	groups := make([]federation.GroupRecord, 8)
	for i := range groups {
		id := fmt.Sprintf("g%02d", i)
		groups[i] = federation.GroupRecord{ID: id, TenantID: testTenant, Name: id}
		grantGroup(t, gm, testPeerID, testTenant, id)
	}

	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		GroupStore:   &memGroupStore{groups: map[string][]federation.GroupRecord{testTenant: groups}},
	})
	client := newCatalogClient(t, handler)

	resp, err := client.ListGroups(context.Background(), requestWithPeerID(
		&kaivuev1.ListGroupsRequest{
			Tenant:   testTenantRef(),
			PageSize: 3,
		}, testPeerID,
	))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.GetGroups(), 3)
	assert.NotEmpty(t, resp.Msg.GetNextCursor())

	// Last page should have remaining items.
	var allIDs []string
	cursor := ""
	for {
		resp, err := client.ListGroups(context.Background(), requestWithPeerID(
			&kaivuev1.ListGroupsRequest{
				Tenant:   testTenantRef(),
				PageSize: 3,
				Cursor:   cursor,
			}, testPeerID,
		))
		require.NoError(t, err)
		for _, g := range resp.Msg.GetGroups() {
			allIDs = append(allIDs, g.GetId())
		}
		if resp.Msg.GetNextCursor() == "" {
			break
		}
		cursor = resp.Msg.GetNextCursor()
	}
	assert.Len(t, allIDs, 8)
}

// --- ListCameras tests ---

func TestListCameras_WithGrants_ReturnsFilteredCameras(t *testing.T) {
	gm := newGrantManager(t)
	cameraStore := &memCameraStore{cameras: map[string][]federation.CameraRecord{
		testTenant: {
			{ID: "cam1", TenantID: testTenant, RecorderID: "rec1", Name: "Front Door", State: kaivuev1.CameraState_CAMERA_STATE_ONLINE},
			{ID: "cam2", TenantID: testTenant, RecorderID: "rec1", Name: "Back Door", State: kaivuev1.CameraState_CAMERA_STATE_OFFLINE},
			{ID: "cam3", TenantID: testTenant, RecorderID: "rec2", Name: "Lobby", State: kaivuev1.CameraState_CAMERA_STATE_ONLINE},
		},
	}}

	// Grant view.live on cam1 and cam3 only.
	grantCamera(t, gm, testPeerID, testTenant, "cam1", permissions.ActionViewLive)
	grantCamera(t, gm, testPeerID, testTenant, "cam3", permissions.ActionViewPlayback)

	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		CameraStore:  cameraStore,
	})
	client := newCatalogClient(t, handler)

	resp, err := client.ListCameras(context.Background(), requestWithPeerID(
		&kaivuev1.FederationPeerServiceListCamerasRequest{Tenant: testTenantRef()}, testPeerID,
	))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.GetCameras(), 2)

	ids := make([]string, 0, 2)
	for _, c := range resp.Msg.GetCameras() {
		ids = append(ids, c.GetId())
		// Verify credential_ref is never populated.
		assert.Empty(t, c.GetCredentialRef(), "credential_ref MUST be scrubbed for federation peers")
	}
	assert.Contains(t, ids, "cam1")
	assert.Contains(t, ids, "cam3")
}

func TestListCameras_NoGrants_ReturnsEmpty(t *testing.T) {
	gm := newGrantManager(t)
	cameraStore := &memCameraStore{cameras: map[string][]federation.CameraRecord{
		testTenant: {
			{ID: "cam1", TenantID: testTenant, Name: "Front Door"},
		},
	}}

	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		CameraStore:  cameraStore,
	})
	client := newCatalogClient(t, handler)

	resp, err := client.ListCameras(context.Background(), requestWithPeerID(
		&kaivuev1.FederationPeerServiceListCamerasRequest{Tenant: testTenantRef()}, testPeerID,
	))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.GetCameras())
}

func TestListCameras_MissingPeerID_ReturnsUnauthenticated(t *testing.T) {
	gm := newGrantManager(t)
	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		CameraStore:  &memCameraStore{cameras: map[string][]federation.CameraRecord{}},
	})
	client := newCatalogClient(t, handler)

	_, err := client.ListCameras(context.Background(), connect.NewRequest(
		&kaivuev1.FederationPeerServiceListCamerasRequest{Tenant: testTenantRef()},
	))
	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestListCameras_MissingTenant_ReturnsInvalidArgument(t *testing.T) {
	gm := newGrantManager(t)
	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		CameraStore:  &memCameraStore{cameras: map[string][]federation.CameraRecord{}},
	})
	client := newCatalogClient(t, handler)

	_, err := client.ListCameras(context.Background(), requestWithPeerID(
		&kaivuev1.FederationPeerServiceListCamerasRequest{}, testPeerID,
	))
	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestListCameras_RecorderIDFilter(t *testing.T) {
	gm := newGrantManager(t)
	cameraStore := &memCameraStore{cameras: map[string][]federation.CameraRecord{
		testTenant: {
			{ID: "cam1", TenantID: testTenant, RecorderID: "rec1", Name: "Front Door"},
			{ID: "cam2", TenantID: testTenant, RecorderID: "rec2", Name: "Back Door"},
		},
	}}

	grantCamera(t, gm, testPeerID, testTenant, "cam1", permissions.ActionViewLive)
	grantCamera(t, gm, testPeerID, testTenant, "cam2", permissions.ActionViewLive)

	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		CameraStore:  cameraStore,
	})
	client := newCatalogClient(t, handler)

	resp, err := client.ListCameras(context.Background(), requestWithPeerID(
		&kaivuev1.FederationPeerServiceListCamerasRequest{
			Tenant:     testTenantRef(),
			RecorderId: "rec1",
		}, testPeerID,
	))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.GetCameras(), 1)
	assert.Equal(t, "cam1", resp.Msg.GetCameras()[0].GetId())
}

func TestListCameras_Pagination(t *testing.T) {
	gm := newGrantManager(t)
	cameras := make([]federation.CameraRecord, 7)
	for i := range cameras {
		id := fmt.Sprintf("cam%02d", i)
		cameras[i] = federation.CameraRecord{
			ID:         id,
			TenantID:   testTenant,
			RecorderID: "rec1",
			Name:       id,
			State:      kaivuev1.CameraState_CAMERA_STATE_ONLINE,
		}
		grantCamera(t, gm, testPeerID, testTenant, id, permissions.ActionViewLive)
	}

	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		CameraStore:  &memCameraStore{cameras: map[string][]federation.CameraRecord{testTenant: cameras}},
	})
	client := newCatalogClient(t, handler)

	// Iterate all pages.
	var allIDs []string
	cursor := ""
	for {
		resp, err := client.ListCameras(context.Background(), requestWithPeerID(
			&kaivuev1.FederationPeerServiceListCamerasRequest{
				Tenant:   testTenantRef(),
				PageSize: 3,
				Cursor:   cursor,
			}, testPeerID,
		))
		require.NoError(t, err)
		for _, c := range resp.Msg.GetCameras() {
			allIDs = append(allIDs, c.GetId())
		}
		if resp.Msg.GetNextCursor() == "" {
			break
		}
		cursor = resp.Msg.GetNextCursor()
	}
	assert.Len(t, allIDs, 7, "all cameras should be returned across pages")
}

func TestListCameras_CameraMetadataPresent(t *testing.T) {
	gm := newGrantManager(t)
	cameraStore := &memCameraStore{cameras: map[string][]federation.CameraRecord{
		testTenant: {
			{
				ID:          "cam1",
				TenantID:    testTenant,
				RecorderID:  "rec1",
				Name:        "Entrance Camera",
				Description: "Main building entrance",
				State:       kaivuev1.CameraState_CAMERA_STATE_ONLINE,
				Labels:      []string{"outdoor", "entrance"},
			},
		},
	}}
	grantCamera(t, gm, testPeerID, testTenant, "cam1", permissions.ActionViewLive)

	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		CameraStore:  cameraStore,
	})
	client := newCatalogClient(t, handler)

	resp, err := client.ListCameras(context.Background(), requestWithPeerID(
		&kaivuev1.FederationPeerServiceListCamerasRequest{Tenant: testTenantRef()}, testPeerID,
	))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetCameras(), 1)
	cam := resp.Msg.GetCameras()[0]
	assert.Equal(t, "Entrance Camera", cam.GetName())
	assert.Equal(t, "Main building entrance", cam.GetDescription())
	assert.Equal(t, kaivuev1.CameraState_CAMERA_STATE_ONLINE, cam.GetState())
	assert.Equal(t, "rec1", cam.GetRecorderId())
	assert.Equal(t, []string{"outdoor", "entrance"}, cam.GetLabels())
}

func TestListCameras_PartialGrants(t *testing.T) {
	gm := newGrantManager(t)
	cameraStore := &memCameraStore{cameras: map[string][]federation.CameraRecord{
		testTenant: {
			{ID: "cam1", TenantID: testTenant, Name: "Camera 1"},
			{ID: "cam2", TenantID: testTenant, Name: "Camera 2"},
			{ID: "cam3", TenantID: testTenant, Name: "Camera 3"},
			{ID: "cam4", TenantID: testTenant, Name: "Camera 4"},
			{ID: "cam5", TenantID: testTenant, Name: "Camera 5"},
		},
	}}

	// Grant only on cam2 and cam4.
	grantCamera(t, gm, testPeerID, testTenant, "cam2", permissions.ActionViewSnapshot)
	grantCamera(t, gm, testPeerID, testTenant, "cam4", permissions.ActionViewThumbnails)

	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		CameraStore:  cameraStore,
	})
	client := newCatalogClient(t, handler)

	resp, err := client.ListCameras(context.Background(), requestWithPeerID(
		&kaivuev1.FederationPeerServiceListCamerasRequest{Tenant: testTenantRef()}, testPeerID,
	))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.GetCameras(), 2)

	ids := []string{resp.Msg.GetCameras()[0].GetId(), resp.Msg.GetCameras()[1].GetId()}
	assert.Contains(t, ids, "cam2")
	assert.Contains(t, ids, "cam4")
}

func TestListCameras_StoreError(t *testing.T) {
	gm := newGrantManager(t)
	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		CameraStore:  &memCameraStore{err: errors.New("connection refused")},
	})
	client := newCatalogClient(t, handler)

	_, err := client.ListCameras(context.Background(), requestWithPeerID(
		&kaivuev1.FederationPeerServiceListCamerasRequest{Tenant: testTenantRef()}, testPeerID,
	))
	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeInternal, connectErr.Code())
}

// --- Cross-cutting: stores not configured ---

func TestListUsers_NoUserStore_ReturnsUnimplemented(t *testing.T) {
	gm := newGrantManager(t)
	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
		// UserStore intentionally nil.
	})
	client := newCatalogClient(t, handler)

	_, err := client.ListUsers(context.Background(), requestWithPeerID(
		&kaivuev1.ListUsersRequest{Tenant: testTenantRef()}, testPeerID,
	))
	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeUnimplemented, connectErr.Code())
}

func TestListGroups_NoGroupStore_ReturnsUnimplemented(t *testing.T) {
	gm := newGrantManager(t)
	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
	})
	client := newCatalogClient(t, handler)

	_, err := client.ListGroups(context.Background(), requestWithPeerID(
		&kaivuev1.ListGroupsRequest{Tenant: testTenantRef()}, testPeerID,
	))
	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeUnimplemented, connectErr.Code())
}

func TestListCameras_NoCameraStore_ReturnsUnimplemented(t *testing.T) {
	gm := newGrantManager(t)
	handler := newCatalogHandler(t, federation.RPCConfig{
		GrantManager: gm,
	})
	client := newCatalogClient(t, handler)

	_, err := client.ListCameras(context.Background(), requestWithPeerID(
		&kaivuev1.FederationPeerServiceListCamerasRequest{Tenant: testTenantRef()}, testPeerID,
	))
	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeUnimplemented, connectErr.Code())
}
