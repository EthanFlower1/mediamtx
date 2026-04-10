package catalogsync

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	directorydb "github.com/bluenviron/mediamtx/internal/directory/db"
	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
	"github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1/kaivuev1connect"
)

// --- helpers ----------------------------------------------------------------

func openTestDB(t *testing.T) *directorydb.DB {
	t.Helper()
	db, err := directorydb.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedPeer(t *testing.T, store *Store, peerID, name string) *PeerRecord {
	t.Helper()
	p := &PeerRecord{
		PeerID:     peerID,
		Name:       name,
		Endpoint:   "https://" + peerID + ".example.com",
		TenantID:   "tenant-" + peerID,
		TenantType: int32(kaivuev1.TenantType_TENANT_TYPE_CUSTOMER),
		SyncStatus: StatusPending,
	}
	if err := store.UpsertPeer(context.Background(), p); err != nil {
		t.Fatalf("seed peer: %v", err)
	}
	return p
}

// fakePeerClient implements kaivuev1connect.FederationPeerServiceClient
// with configurable responses for the catalog RPCs.
type fakePeerClient struct {
	kaivuev1connect.UnimplementedFederationPeerServiceHandler
	cameras []*kaivuev1.Camera
	users   []*kaivuev1.FederatedUser
	groups  []*kaivuev1.FederatedGroup
	err     error
}

func (f *fakePeerClient) Ping(_ context.Context, _ *connect.Request[kaivuev1.PingRequest]) (*connect.Response[kaivuev1.PingResponse], error) {
	return connect.NewResponse(&kaivuev1.PingResponse{}), nil
}

func (f *fakePeerClient) GetJWKS(_ context.Context, _ *connect.Request[kaivuev1.GetJWKSRequest]) (*connect.Response[kaivuev1.GetJWKSResponse], error) {
	return connect.NewResponse(&kaivuev1.GetJWKSResponse{}), nil
}

func (f *fakePeerClient) ListCameras(_ context.Context, _ *connect.Request[kaivuev1.FederationPeerServiceListCamerasRequest]) (*connect.Response[kaivuev1.FederationPeerServiceListCamerasResponse], error) {
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&kaivuev1.FederationPeerServiceListCamerasResponse{
		Cameras: f.cameras,
	}), nil
}

func (f *fakePeerClient) ListUsers(_ context.Context, _ *connect.Request[kaivuev1.ListUsersRequest]) (*connect.Response[kaivuev1.ListUsersResponse], error) {
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&kaivuev1.ListUsersResponse{
		Users: f.users,
	}), nil
}

func (f *fakePeerClient) ListGroups(_ context.Context, _ *connect.Request[kaivuev1.ListGroupsRequest]) (*connect.Response[kaivuev1.ListGroupsResponse], error) {
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&kaivuev1.ListGroupsResponse{
		Groups: f.groups,
	}), nil
}

func (f *fakePeerClient) SearchRecordings(_ context.Context, _ *connect.Request[kaivuev1.SearchRecordingsRequest]) (*connect.ServerStreamForClient[kaivuev1.SearchRecordingsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

func (f *fakePeerClient) MintStreamURL(_ context.Context, _ *connect.Request[kaivuev1.MintStreamURLRequest]) (*connect.Response[kaivuev1.MintStreamURLResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

// --- tests ------------------------------------------------------------------

func TestSyncPopulatesCache(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	peer := seedPeer(t, store, "peer-1", "Site Alpha")

	fakeClient := &fakePeerClient{
		cameras: []*kaivuev1.Camera{
			{Id: "cam-1", Name: "Front Door", RecorderId: "rec-1", Manufacturer: "Axis", Model: "P1375", IpAddress: "10.0.0.10", State: kaivuev1.CameraState_CAMERA_STATE_ONLINE, Labels: []string{"entrance"}},
			{Id: "cam-2", Name: "Back Gate", RecorderId: "rec-1", Manufacturer: "Hikvision", Model: "DS-2CD2143", IpAddress: "10.0.0.11"},
		},
		users: []*kaivuev1.FederatedUser{
			{Id: "user-1", Username: "alice", Email: "alice@example.com", DisplayName: "Alice", Groups: []string{"admins"}},
		},
		groups: []*kaivuev1.FederatedGroup{
			{Id: "group-1", Name: "admins", Description: "Administrators"},
		},
	}

	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)

	syncer, err := New(Config{
		Store: store,
		ClientFactory: func(_ *PeerRecord) (kaivuev1connect.FederationPeerServiceClient, error) {
			return fakeClient, nil
		},
		Interval: MinInterval,
		Clock:    func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new syncer: %v", err)
	}

	syncer.SyncOnce(ctx)

	// Verify cameras cached.
	cameras, err := store.ListCamerasByPeer(ctx, peer.PeerID)
	if err != nil {
		t.Fatalf("list cameras: %v", err)
	}
	if len(cameras) != 2 {
		t.Fatalf("expected 2 cameras, got %d", len(cameras))
	}
	if cameras[1].Name != "Front Door" { // sorted by name: "Back Gate" < "Front Door"
		t.Errorf("expected Front Door, got %s", cameras[1].Name)
	}
	if cameras[0].Manufacturer != "Hikvision" {
		t.Errorf("expected Hikvision, got %s", cameras[0].Manufacturer)
	}
	if len(cameras[1].Labels) != 1 || cameras[1].Labels[0] != "entrance" {
		t.Errorf("expected labels [entrance], got %v", cameras[1].Labels)
	}

	// Verify users cached.
	users, err := store.ListUsersByPeer(ctx, peer.PeerID)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0].Username != "alice" {
		t.Errorf("expected alice, got %s", users[0].Username)
	}

	// Verify groups cached.
	groups, err := store.ListGroupsByPeer(ctx, peer.PeerID)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Name != "admins" {
		t.Errorf("expected admins, got %s", groups[0].Name)
	}

	// Verify peer status updated.
	peerRec, err := store.GetPeer(ctx, peer.PeerID)
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if peerRec.SyncStatus != StatusSynced {
		t.Errorf("expected synced status, got %s", peerRec.SyncStatus)
	}
	if peerRec.LastSyncAt == nil {
		t.Fatal("expected last_sync_at to be set")
	}
	if !peerRec.LastSyncAt.Equal(now) {
		t.Errorf("expected last_sync_at=%v, got %v", now, *peerRec.LastSyncAt)
	}
}

func TestPeerOfflineMarksError(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	// Seed a peer with an old last_sync_at.
	oldTime := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	p := &PeerRecord{
		PeerID:     "peer-offline",
		Name:       "Offline Site",
		Endpoint:   "https://offline.example.com",
		TenantID:   "tenant-offline",
		TenantType: int32(kaivuev1.TenantType_TENANT_TYPE_CUSTOMER),
		SyncStatus: StatusSynced,
		LastSyncAt: &oldTime,
	}
	if err := store.UpsertPeer(ctx, p); err != nil {
		t.Fatalf("seed peer: %v", err)
	}

	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC) // 2 hours later

	// The error from the client means the peer is unreachable.
	syncer, err := New(Config{
		Store: store,
		ClientFactory: func(_ *PeerRecord) (kaivuev1connect.FederationPeerServiceClient, error) {
			return &fakePeerClient{err: connect.NewError(connect.CodeUnavailable, errors.New("connection refused"))}, nil
		},
		Interval:        5 * time.Minute,
		StaleMultiplier: 2,
		Clock:           func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new syncer: %v", err)
	}

	syncer.SyncOnce(ctx)

	// Peer should be in error state from the failed sync. MarkStale only
	// promotes "synced" peers to "stale"; an "error" peer is already known-bad.
	peer, err := store.GetPeer(ctx, "peer-offline")
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if peer.SyncStatus != StatusError {
		t.Errorf("expected error status, got %s", peer.SyncStatus)
	}
	if peer.SyncError == "" {
		t.Error("expected non-empty sync error for offline peer")
	}
}

func TestIndependentPeerSync(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	seedPeer(t, store, "peer-ok", "Working Site")
	seedPeer(t, store, "peer-fail", "Broken Site")

	var okSynced atomic.Bool
	var failCalled atomic.Bool

	okClient := &fakePeerClient{
		cameras: []*kaivuev1.Camera{{Id: "cam-ok", Name: "OK Camera"}},
		users:   []*kaivuev1.FederatedUser{},
		groups:  []*kaivuev1.FederatedGroup{},
	}
	failClient := &fakePeerClient{
		err: connect.NewError(connect.CodeUnavailable, errors.New("peer down")),
	}

	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)

	syncer, err := New(Config{
		Store: store,
		ClientFactory: func(p *PeerRecord) (kaivuev1connect.FederationPeerServiceClient, error) {
			if p.PeerID == "peer-ok" {
				okSynced.Store(true)
				return okClient, nil
			}
			failCalled.Store(true)
			return failClient, nil
		},
		Interval: MinInterval,
		Clock:    func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new syncer: %v", err)
	}

	syncer.SyncOnce(ctx)

	// Both peers should have been attempted.
	if !okSynced.Load() {
		t.Error("ok peer was not synced")
	}
	if !failCalled.Load() {
		t.Error("fail peer was not attempted")
	}

	// OK peer should have cameras cached.
	cameras, err := store.ListCamerasByPeer(ctx, "peer-ok")
	if err != nil {
		t.Fatalf("list cameras: %v", err)
	}
	if len(cameras) != 1 {
		t.Errorf("expected 1 camera for ok peer, got %d", len(cameras))
	}

	// OK peer should be synced.
	peerOK, _ := store.GetPeer(ctx, "peer-ok")
	if peerOK.SyncStatus != StatusSynced {
		t.Errorf("expected synced for ok peer, got %s", peerOK.SyncStatus)
	}

	// Fail peer should be in error state.
	peerFail, _ := store.GetPeer(ctx, "peer-fail")
	if peerFail.SyncStatus != StatusError {
		t.Errorf("expected error for fail peer, got %s", peerFail.SyncStatus)
	}
	if peerFail.SyncError == "" {
		t.Error("expected sync_error to be non-empty for fail peer")
	}
}

func TestSyncReplacesOldData(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	seedPeer(t, store, "peer-replace", "Replace Site")

	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)

	// First sync: 2 cameras.
	client1 := &fakePeerClient{
		cameras: []*kaivuev1.Camera{
			{Id: "cam-1", Name: "Camera 1"},
			{Id: "cam-2", Name: "Camera 2"},
		},
		users:  []*kaivuev1.FederatedUser{},
		groups: []*kaivuev1.FederatedGroup{},
	}

	syncer, _ := New(Config{
		Store: store,
		ClientFactory: func(_ *PeerRecord) (kaivuev1connect.FederationPeerServiceClient, error) {
			return client1, nil
		},
		Interval: MinInterval,
		Clock:    func() time.Time { return now },
	})
	syncer.SyncOnce(ctx)

	cams, _ := store.ListCamerasByPeer(ctx, "peer-replace")
	if len(cams) != 2 {
		t.Fatalf("expected 2 cameras after first sync, got %d", len(cams))
	}

	// Second sync: only 1 camera (cam-2 removed from peer).
	client2 := &fakePeerClient{
		cameras: []*kaivuev1.Camera{
			{Id: "cam-1", Name: "Camera 1 Updated"},
		},
		users:  []*kaivuev1.FederatedUser{},
		groups: []*kaivuev1.FederatedGroup{},
	}

	syncer2, _ := New(Config{
		Store: store,
		ClientFactory: func(_ *PeerRecord) (kaivuev1connect.FederationPeerServiceClient, error) {
			return client2, nil
		},
		Interval: MinInterval,
		Clock:    func() time.Time { return now },
	})
	syncer2.SyncOnce(ctx)

	cams, _ = store.ListCamerasByPeer(ctx, "peer-replace")
	if len(cams) != 1 {
		t.Fatalf("expected 1 camera after second sync, got %d", len(cams))
	}
	if cams[0].Name != "Camera 1 Updated" {
		t.Errorf("expected updated name, got %s", cams[0].Name)
	}
}

func TestStartStopLifecycle(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)

	seedPeer(t, store, "peer-lifecycle", "Lifecycle Site")

	var syncCount atomic.Int32
	fakeClient := &fakePeerClient{
		cameras: []*kaivuev1.Camera{{Id: "cam-lc", Name: "LC Camera"}},
		users:   []*kaivuev1.FederatedUser{},
		groups:  []*kaivuev1.FederatedGroup{},
	}

	syncer, err := New(Config{
		Store: store,
		ClientFactory: func(_ *PeerRecord) (kaivuev1connect.FederationPeerServiceClient, error) {
			syncCount.Add(1)
			return fakeClient, nil
		},
		Interval: 50 * time.Millisecond, // fast for test, will be clamped to MinInterval in prod but we test the ticker
		Clock:    time.Now,
	})
	if err != nil {
		t.Fatalf("new syncer: %v", err)
	}

	ctx := context.Background()
	syncer.Start(ctx)

	// Wait enough for at least the initial sync.
	time.Sleep(200 * time.Millisecond)

	syncer.Stop()

	// Should have synced at least once (the immediate sync on start).
	if syncCount.Load() < 1 {
		t.Errorf("expected at least 1 sync, got %d", syncCount.Load())
	}
}

func TestNewValidation(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)

	// Missing store.
	_, err := New(Config{ClientFactory: func(_ *PeerRecord) (kaivuev1connect.FederationPeerServiceClient, error) { return nil, nil }})
	if err == nil {
		t.Error("expected error for missing store")
	}

	// Missing client factory.
	_, err = New(Config{Store: store})
	if err == nil {
		t.Error("expected error for missing client factory")
	}

	// Valid config.
	_, err = New(Config{
		Store:         store,
		ClientFactory: func(_ *PeerRecord) (kaivuev1connect.FederationPeerServiceClient, error) { return nil, nil },
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIntervalClamping(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)

	// Below minimum should be clamped.
	syncer, err := New(Config{
		Store:         store,
		ClientFactory: func(_ *PeerRecord) (kaivuev1connect.FederationPeerServiceClient, error) { return nil, nil },
		Interval:      1 * time.Second,
	})
	if err != nil {
		t.Fatalf("new syncer: %v", err)
	}
	if syncer.cfg.Interval != MinInterval {
		t.Errorf("expected interval clamped to %v, got %v", MinInterval, syncer.cfg.Interval)
	}

	// Zero should default.
	syncer2, _ := New(Config{
		Store:         store,
		ClientFactory: func(_ *PeerRecord) (kaivuev1connect.FederationPeerServiceClient, error) { return nil, nil },
	})
	if syncer2.cfg.Interval != DefaultInterval {
		t.Errorf("expected default interval %v, got %v", DefaultInterval, syncer2.cfg.Interval)
	}
}

func TestClientFactoryError(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	seedPeer(t, store, "peer-factory-err", "Factory Error Site")

	syncer, _ := New(Config{
		Store: store,
		ClientFactory: func(_ *PeerRecord) (kaivuev1connect.FederationPeerServiceClient, error) {
			return nil, errors.New("mTLS handshake failed")
		},
		Interval: MinInterval,
		Clock:    time.Now,
	})

	syncer.SyncOnce(ctx)

	peer, _ := store.GetPeer(ctx, "peer-factory-err")
	if peer.SyncStatus != StatusError {
		t.Errorf("expected error status, got %s", peer.SyncStatus)
	}
	if peer.SyncError == "" {
		t.Error("expected non-empty sync error")
	}
}

func TestStaleDetectionForSyncedPeer(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	// Seed a peer that was successfully synced 2 hours ago.
	oldSync := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	p := &PeerRecord{
		PeerID:     "peer-stale",
		Name:       "Aging Site",
		Endpoint:   "https://aging.example.com",
		TenantID:   "tenant-aging",
		TenantType: int32(kaivuev1.TenantType_TENANT_TYPE_CUSTOMER),
		SyncStatus: StatusSynced,
		LastSyncAt: &oldSync,
	}
	if err := store.UpsertPeer(ctx, p); err != nil {
		t.Fatalf("seed peer: %v", err)
	}

	// Mark stale with a threshold in the future of the old sync time.
	// Threshold = now - 2*interval. If last_sync_at < threshold, mark stale.
	threshold := time.Date(2026, 4, 10, 11, 50, 0, 0, time.UTC)
	n, err := store.MarkStale(ctx, threshold)
	if err != nil {
		t.Fatalf("mark stale: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 peer marked stale, got %d", n)
	}

	peer, err := store.GetPeer(ctx, "peer-stale")
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if peer.SyncStatus != StatusStale {
		t.Errorf("expected stale status, got %s", peer.SyncStatus)
	}

	// Cached data should still be available even when stale.
	// (We didn't store any data for this peer, but the peer record persists.)
	if peer.LastSyncAt == nil {
		t.Error("expected last_sync_at to be preserved")
	}
}
