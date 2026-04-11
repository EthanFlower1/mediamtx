// Package validation contains cross-cutting acceptance tests for KAI-275:
// federation failure semantics. These tests prove that local-site operations
// are never affected by federation state and that offline peers degrade
// gracefully.
package validation

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	directorydb "github.com/bluenviron/mediamtx/internal/directory/db"
	"github.com/bluenviron/mediamtx/internal/directory/federation/catalogsync"
	"github.com/bluenviron/mediamtx/internal/directory/federation/playback"
	"github.com/bluenviron/mediamtx/internal/directory/federation/search"
	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
	"github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1/kaivuev1connect"
)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

var discardLog = slog.New(slog.NewTextHandler(io.Discard, nil))

func openTestDB(t *testing.T) *directorydb.DB {
	t.Helper()
	db, err := directorydb.Open(context.Background(), ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedPeer(t *testing.T, store *catalogsync.Store, peerID, name string) *catalogsync.PeerRecord {
	t.Helper()
	p := &catalogsync.PeerRecord{
		PeerID:     peerID,
		Name:       name,
		Endpoint:   "https://" + peerID + ".example.com",
		TenantID:   "tenant-" + peerID,
		TenantType: int32(kaivuev1.TenantType_TENANT_TYPE_CUSTOMER),
		SyncStatus: catalogsync.StatusPending,
	}
	require.NoError(t, store.UpsertPeer(context.Background(), p))
	return p
}

// ---------------------------------------------------------------------------
// Fake catalog-sync client (for CatalogSyncer tests)
// ---------------------------------------------------------------------------

type fakeSyncClient struct {
	kaivuev1connect.UnimplementedFederationPeerServiceHandler
	mu      sync.Mutex
	cameras []*kaivuev1.Camera
	users   []*kaivuev1.FederatedUser
	groups  []*kaivuev1.FederatedGroup
	err     error
}

func (f *fakeSyncClient) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeSyncClient) clearErr() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = nil
}

func (f *fakeSyncClient) getErr() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}

func (f *fakeSyncClient) Ping(_ context.Context, _ *connect.Request[kaivuev1.PingRequest]) (*connect.Response[kaivuev1.PingResponse], error) {
	return connect.NewResponse(&kaivuev1.PingResponse{}), nil
}

func (f *fakeSyncClient) GetJWKS(_ context.Context, _ *connect.Request[kaivuev1.GetJWKSRequest]) (*connect.Response[kaivuev1.GetJWKSResponse], error) {
	return connect.NewResponse(&kaivuev1.GetJWKSResponse{}), nil
}

func (f *fakeSyncClient) ListCameras(_ context.Context, _ *connect.Request[kaivuev1.FederationPeerServiceListCamerasRequest]) (*connect.Response[kaivuev1.FederationPeerServiceListCamerasResponse], error) {
	if err := f.getErr(); err != nil {
		return nil, err
	}
	return connect.NewResponse(&kaivuev1.FederationPeerServiceListCamerasResponse{
		Cameras: f.cameras,
	}), nil
}

func (f *fakeSyncClient) ListUsers(_ context.Context, _ *connect.Request[kaivuev1.ListUsersRequest]) (*connect.Response[kaivuev1.ListUsersResponse], error) {
	if err := f.getErr(); err != nil {
		return nil, err
	}
	return connect.NewResponse(&kaivuev1.ListUsersResponse{
		Users: f.users,
	}), nil
}

func (f *fakeSyncClient) ListGroups(_ context.Context, _ *connect.Request[kaivuev1.ListGroupsRequest]) (*connect.Response[kaivuev1.ListGroupsResponse], error) {
	if err := f.getErr(); err != nil {
		return nil, err
	}
	return connect.NewResponse(&kaivuev1.ListGroupsResponse{
		Groups: f.groups,
	}), nil
}

func (f *fakeSyncClient) SearchRecordings(_ context.Context, _ *connect.Request[kaivuev1.SearchRecordingsRequest]) (*connect.ServerStreamForClient[kaivuev1.SearchRecordingsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

func (f *fakeSyncClient) MintStreamURL(_ context.Context, _ *connect.Request[kaivuev1.MintStreamURLRequest]) (*connect.Response[kaivuev1.MintStreamURLResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

// ---------------------------------------------------------------------------
// Fake search peer server (for httptest-based search tests)
// ---------------------------------------------------------------------------

type fakePeerHandler struct {
	kaivuev1connect.UnimplementedFederationPeerServiceHandler
	mu     sync.Mutex
	hits   []*kaivuev1.RecordingHit
	delay  time.Duration
	errMsg string
}

func (h *fakePeerHandler) SearchRecordings(
	ctx context.Context,
	_ *connect.Request[kaivuev1.SearchRecordingsRequest],
	stream *connect.ServerStream[kaivuev1.SearchRecordingsResponse],
) error {
	h.mu.Lock()
	delay := h.delay
	errMsg := h.errMsg
	hits := h.hits
	h.mu.Unlock()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if errMsg != "" {
		return connect.NewError(connect.CodeInternal, errors.New(errMsg))
	}
	for _, hit := range hits {
		if err := stream.Send(&kaivuev1.SearchRecordingsResponse{Hit: hit}); err != nil {
			return err
		}
	}
	return nil
}

func fakePeerServer(hits []*kaivuev1.RecordingHit, delay time.Duration, errMsg string) *httptest.Server {
	_, handler := kaivuev1connect.NewFederationPeerServiceHandler(&fakePeerHandler{
		hits:   hits,
		delay:  delay,
		errMsg: errMsg,
	})
	mux := http.NewServeMux()
	mux.Handle("/", handler)
	return httptest.NewServer(mux)
}

func makeHit(cameraID string, startSec int64) *kaivuev1.RecordingHit {
	return &kaivuev1.RecordingHit{
		CameraId:  cameraID,
		SegmentId: cameraID + "-seg",
		StartTime: timestamppb.New(time.Unix(startSec, 0)),
		EndTime:   timestamppb.New(time.Unix(startSec+60, 0)),
	}
}

func makePeerFromServer(id string, srv *httptest.Server) search.Peer {
	return search.Peer{
		ID:     id,
		Client: kaivuev1connect.NewFederationPeerServiceClient(srv.Client(), srv.URL),
	}
}

// ---------------------------------------------------------------------------
// Fake playback doubles
// ---------------------------------------------------------------------------

type fakeCatalog struct {
	entries map[string]playback.CatalogEntry
}

func (f *fakeCatalog) ResolveCamera(_ context.Context, cameraID string) (playback.CatalogEntry, error) {
	e, ok := f.entries[cameraID]
	if !ok {
		return playback.CatalogEntry{}, errors.New("not found")
	}
	return e, nil
}

type fakePlaybackClient struct {
	kaivuev1connect.UnimplementedFederationPeerServiceHandler
	mu       sync.Mutex
	mintResp *kaivuev1.MintStreamURLResponse
	mintErr  error
}

func (f *fakePlaybackClient) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mintErr = err
}

func (f *fakePlaybackClient) Ping(_ context.Context, _ *connect.Request[kaivuev1.PingRequest]) (*connect.Response[kaivuev1.PingResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

func (f *fakePlaybackClient) GetJWKS(_ context.Context, _ *connect.Request[kaivuev1.GetJWKSRequest]) (*connect.Response[kaivuev1.GetJWKSResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

func (f *fakePlaybackClient) ListUsers(_ context.Context, _ *connect.Request[kaivuev1.ListUsersRequest]) (*connect.Response[kaivuev1.ListUsersResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

func (f *fakePlaybackClient) ListGroups(_ context.Context, _ *connect.Request[kaivuev1.ListGroupsRequest]) (*connect.Response[kaivuev1.ListGroupsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

func (f *fakePlaybackClient) ListCameras(_ context.Context, _ *connect.Request[kaivuev1.FederationPeerServiceListCamerasRequest]) (*connect.Response[kaivuev1.FederationPeerServiceListCamerasResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

func (f *fakePlaybackClient) SearchRecordings(_ context.Context, _ *connect.Request[kaivuev1.SearchRecordingsRequest]) (*connect.ServerStreamForClient[kaivuev1.SearchRecordingsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

func (f *fakePlaybackClient) MintStreamURL(_ context.Context, _ *connect.Request[kaivuev1.MintStreamURLRequest]) (*connect.Response[kaivuev1.MintStreamURLResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.mintErr != nil {
		return nil, f.mintErr
	}
	return connect.NewResponse(f.mintResp), nil
}

type fakePeerFactory struct {
	clients map[string]*fakePlaybackClient
}

func (f *fakePeerFactory) ClientForPeer(_ context.Context, peerID string) (kaivuev1connect.FederationPeerServiceClient, error) {
	c, ok := f.clients[peerID]
	if !ok {
		return nil, errors.New("unknown peer")
	}
	return c, nil
}

// ===========================================================================
// AC-1: Chaos test — kill peer connection mid-operation, verify no
//        local-site impact.
// ===========================================================================

func TestChaos_PeerFailureMidSync_NoLocalImpact(t *testing.T) {
	// Setup: two peers. peer-local represents local cameras (always online),
	// peer-remote goes offline mid-sync.
	db := openTestDB(t)
	store := catalogsync.NewStore(db)
	ctx := context.Background()

	seedPeer(t, store, "peer-local", "Local Site")
	seedPeer(t, store, "peer-remote", "Remote Site")

	localClient := &fakeSyncClient{
		cameras: []*kaivuev1.Camera{
			{Id: "local-cam-1", Name: "Lobby", Manufacturer: "Axis"},
			{Id: "local-cam-2", Name: "Parking", Manufacturer: "Hikvision"},
		},
		users:  []*kaivuev1.FederatedUser{{Id: "u1", Username: "admin"}},
		groups: []*kaivuev1.FederatedGroup{{Id: "g1", Name: "ops"}},
	}

	// Remote peer starts healthy, then goes down.
	remoteClient := &fakeSyncClient{
		cameras: []*kaivuev1.Camera{
			{Id: "remote-cam-1", Name: "Warehouse"},
		},
		users:  []*kaivuev1.FederatedUser{},
		groups: []*kaivuev1.FederatedGroup{},
	}

	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)

	syncer, err := catalogsync.New(catalogsync.Config{
		Store: store,
		ClientFactory: func(p *catalogsync.PeerRecord) (kaivuev1connect.FederationPeerServiceClient, error) {
			switch p.PeerID {
			case "peer-local":
				return localClient, nil
			case "peer-remote":
				return remoteClient, nil
			default:
				return nil, errors.New("unknown peer")
			}
		},
		Interval: catalogsync.MinInterval,
		Clock:    func() time.Time { return now },
		Logger:   discardLog,
	})
	require.NoError(t, err)

	// First sync: both peers online.
	syncer.SyncOnce(ctx)

	localCams, err := store.ListCamerasByPeer(ctx, "peer-local")
	require.NoError(t, err)
	assert.Len(t, localCams, 2, "local cameras should be cached")

	remoteCams, err := store.ListCamerasByPeer(ctx, "peer-remote")
	require.NoError(t, err)
	assert.Len(t, remoteCams, 1, "remote cameras should be cached")

	// Kill remote peer.
	remoteClient.setErr(connect.NewError(connect.CodeUnavailable, errors.New("connection reset by peer")))

	// Second sync: remote peer is down.
	syncer.SyncOnce(ctx)

	// Local peer's data must be unaffected.
	localCams, err = store.ListCamerasByPeer(ctx, "peer-local")
	require.NoError(t, err)
	assert.Len(t, localCams, 2, "local cameras must survive remote peer failure")

	localUsers, err := store.ListUsersByPeer(ctx, "peer-local")
	require.NoError(t, err)
	assert.Len(t, localUsers, 1, "local users must survive remote peer failure")

	// Local peer should still be synced.
	peerLocal, err := store.GetPeer(ctx, "peer-local")
	require.NoError(t, err)
	assert.Equal(t, catalogsync.StatusSynced, peerLocal.SyncStatus, "local peer status must remain synced")

	// Remote peer should be in error state.
	peerRemote, err := store.GetPeer(ctx, "peer-remote")
	require.NoError(t, err)
	assert.Equal(t, catalogsync.StatusError, peerRemote.SyncStatus, "remote peer should be in error state")
	assert.NotEmpty(t, peerRemote.SyncError, "remote peer should have error message")
}

// ===========================================================================
// AC-2: Cached catalog survives full peer outage, marked stale.
// ===========================================================================

func TestCachedCatalog_SurvivesPeerOutage_MarkedStale(t *testing.T) {
	db := openTestDB(t)
	store := catalogsync.NewStore(db)
	ctx := context.Background()

	seedPeer(t, store, "peer-alpha", "Alpha Site")

	alphaClient := &fakeSyncClient{
		cameras: []*kaivuev1.Camera{
			{Id: "alpha-cam-1", Name: "Reception", Manufacturer: "Dahua", Model: "IPC-HDW5442TM"},
			{Id: "alpha-cam-2", Name: "Server Room", Manufacturer: "Axis", Model: "M3106-L"},
			{Id: "alpha-cam-3", Name: "Loading Dock", Manufacturer: "Hikvision"},
		},
		users: []*kaivuev1.FederatedUser{
			{Id: "u1", Username: "operator1", Email: "op1@alpha.com"},
		},
		groups: []*kaivuev1.FederatedGroup{
			{Id: "g1", Name: "security"},
		},
	}

	t0 := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	clock := &t0
	interval := catalogsync.MinInterval
	staleMultiplier := 2

	syncer, err := catalogsync.New(catalogsync.Config{
		Store: store,
		ClientFactory: func(_ *catalogsync.PeerRecord) (kaivuev1connect.FederationPeerServiceClient, error) {
			return alphaClient, nil
		},
		Interval:        interval,
		StaleMultiplier: staleMultiplier,
		Clock:           func() time.Time { return *clock },
		Logger:          discardLog,
	})
	require.NoError(t, err)

	// Sync at t0: populate cache.
	syncer.SyncOnce(ctx)

	cams, err := store.ListCamerasByPeer(ctx, "peer-alpha")
	require.NoError(t, err)
	assert.Len(t, cams, 3, "all cameras should be cached after initial sync")

	peer, err := store.GetPeer(ctx, "peer-alpha")
	require.NoError(t, err)
	assert.Equal(t, catalogsync.StatusSynced, peer.SyncStatus)

	// Peer goes fully offline.
	alphaClient.setErr(connect.NewError(connect.CodeUnavailable, errors.New("host unreachable")))

	// Advance clock past stale threshold (interval * staleMultiplier).
	t1 := t0.Add(time.Duration(staleMultiplier+1) * interval)
	clock = &t1

	syncer.SyncOnce(ctx)

	// Peer record: sync failed so it becomes error (not stale, because
	// MarkStale only transitions "synced" -> "stale", and the sync failure
	// already set it to "error").
	peer, err = store.GetPeer(ctx, "peer-alpha")
	require.NoError(t, err)
	assert.Equal(t, catalogsync.StatusError, peer.SyncStatus,
		"peer should be in error state after outage")

	// Critical: cached camera data MUST still be present even after the peer
	// is in error state. The ReplaceCameras is only called on SUCCESS, so the
	// old data persists.
	cams, err = store.ListCamerasByPeer(ctx, "peer-alpha")
	require.NoError(t, err)
	assert.Len(t, cams, 3, "cached cameras must survive peer outage")

	users, err := store.ListUsersByPeer(ctx, "peer-alpha")
	require.NoError(t, err)
	assert.Len(t, users, 1, "cached users must survive peer outage")

	groups, err := store.ListGroupsByPeer(ctx, "peer-alpha")
	require.NoError(t, err)
	assert.Len(t, groups, 1, "cached groups must survive peer outage")

	// Verify the last_sync_at is the old (stale) time, not the current time.
	assert.NotNil(t, peer.LastSyncAt, "last_sync_at should still reference the last successful sync")
	assert.True(t, peer.LastSyncAt.Before(t1),
		"last_sync_at (%v) should be before current clock (%v)", peer.LastSyncAt, t1)
}

// ===========================================================================
// AC-2 variant: MarkStale transitions synced -> stale when sync does not
// fail but the peer simply hasn't been synced recently.
// ===========================================================================

func TestMarkStale_SyncedPeerBeyondThreshold_BecomesStale(t *testing.T) {
	db := openTestDB(t)
	store := catalogsync.NewStore(db)
	ctx := context.Background()

	// Manually create a peer that was synced 2 hours ago.
	oldSync := time.Date(2026, 4, 10, 8, 0, 0, 0, time.UTC)
	p := &catalogsync.PeerRecord{
		PeerID:     "peer-aging",
		Name:       "Aging Site",
		Endpoint:   "https://aging.example.com",
		TenantID:   "t1",
		TenantType: int32(kaivuev1.TenantType_TENANT_TYPE_CUSTOMER),
		SyncStatus: catalogsync.StatusSynced,
		LastSyncAt: &oldSync,
	}
	require.NoError(t, store.UpsertPeer(ctx, p))

	// Pre-populate some cameras so we can verify they persist.
	require.NoError(t, store.ReplaceCameras(ctx, "peer-aging", []catalogsync.CachedCamera{
		{ID: "cam-1", PeerID: "peer-aging", Name: "Old Camera", SyncedAt: oldSync},
	}))

	// Threshold is newer than last_sync_at, so it gets marked stale.
	threshold := time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)
	n, err := store.MarkStale(ctx, threshold)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n, "one peer should be marked stale")

	peer, err := store.GetPeer(ctx, "peer-aging")
	require.NoError(t, err)
	assert.Equal(t, catalogsync.StatusStale, peer.SyncStatus, "peer should be stale")

	// Cached data must survive.
	cams, err := store.ListCamerasByPeer(ctx, "peer-aging")
	require.NoError(t, err)
	assert.Len(t, cams, 1, "cached cameras must survive stale marking")
}

// ===========================================================================
// AC-3: Reconnect triggers automatic resync within one interval.
// ===========================================================================

func TestReconnect_TriggersResync_WithinOneInterval(t *testing.T) {
	db := openTestDB(t)
	store := catalogsync.NewStore(db)
	ctx := context.Background()

	seedPeer(t, store, "peer-flaky", "Flaky Site")

	flakyClient := &fakeSyncClient{
		cameras: []*kaivuev1.Camera{
			{Id: "flaky-cam-1", Name: "Gate Camera"},
		},
		users:  []*kaivuev1.FederatedUser{},
		groups: []*kaivuev1.FederatedGroup{},
	}

	var syncCount atomic.Int32

	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)

	syncer, err := catalogsync.New(catalogsync.Config{
		Store: store,
		ClientFactory: func(_ *catalogsync.PeerRecord) (kaivuev1connect.FederationPeerServiceClient, error) {
			syncCount.Add(1)
			return flakyClient, nil
		},
		Interval: catalogsync.MinInterval,
		Clock:    func() time.Time { return now },
		Logger:   discardLog,
	})
	require.NoError(t, err)

	// Phase 1: Sync works.
	syncer.SyncOnce(ctx)
	assert.Equal(t, int32(1), syncCount.Load())

	peer, _ := store.GetPeer(ctx, "peer-flaky")
	assert.Equal(t, catalogsync.StatusSynced, peer.SyncStatus)

	// Phase 2: Peer goes offline.
	flakyClient.setErr(connect.NewError(connect.CodeUnavailable, errors.New("network partition")))
	syncer.SyncOnce(ctx)

	peer, _ = store.GetPeer(ctx, "peer-flaky")
	assert.Equal(t, catalogsync.StatusError, peer.SyncStatus)

	// Phase 3: Peer comes back. Next SyncOnce should recover it.
	flakyClient.clearErr()
	flakyClient.cameras = []*kaivuev1.Camera{
		{Id: "flaky-cam-1", Name: "Gate Camera"},
		{Id: "flaky-cam-2", Name: "New Camera Added While Offline"},
	}

	syncer.SyncOnce(ctx)

	// Peer should be back to synced.
	peer, _ = store.GetPeer(ctx, "peer-flaky")
	assert.Equal(t, catalogsync.StatusSynced, peer.SyncStatus,
		"peer should recover to synced status after reconnect")

	// New camera data should be present (proving full resync).
	cams, err := store.ListCamerasByPeer(ctx, "peer-flaky")
	require.NoError(t, err)
	assert.Len(t, cams, 2, "both cameras should be present after resync")

	// Total sync attempts: 3 (initial + offline + reconnect).
	assert.Equal(t, int32(3), syncCount.Load(),
		"syncer must attempt every interval, including during outage")
}

// ===========================================================================
// AC-3 variant: Reconnect via background Start/Stop lifecycle.
// ===========================================================================

func TestReconnect_BackgroundLoop_AutoResync(t *testing.T) {
	db := openTestDB(t)
	store := catalogsync.NewStore(db)

	seedPeer(t, store, "peer-bg", "Background Site")

	flakyClient := &fakeSyncClient{
		cameras: []*kaivuev1.Camera{{Id: "bg-cam", Name: "BG Camera"}},
		users:   []*kaivuev1.FederatedUser{},
		groups:  []*kaivuev1.FederatedGroup{},
	}

	var syncCount atomic.Int32

	syncer, err := catalogsync.New(catalogsync.Config{
		Store: store,
		ClientFactory: func(_ *catalogsync.PeerRecord) (kaivuev1connect.FederationPeerServiceClient, error) {
			syncCount.Add(1)
			return flakyClient, nil
		},
		// Use MinInterval (30s) but we rely on the immediate sync-on-start.
		Interval: catalogsync.MinInterval,
		Clock:    time.Now,
		Logger:   discardLog,
	})
	require.NoError(t, err)

	ctx := context.Background()
	syncer.Start(ctx)

	// Wait for the immediate sync.
	time.Sleep(200 * time.Millisecond)
	syncer.Stop()

	assert.GreaterOrEqual(t, syncCount.Load(), int32(1),
		"background loop must run at least one sync immediately on start")

	peer, _ := store.GetPeer(ctx, "peer-bg")
	assert.Equal(t, catalogsync.StatusSynced, peer.SyncStatus)
}

// ===========================================================================
// AC-4: Cross-site search returns partial=true with online peer results only.
// ===========================================================================

func TestFederatedSearch_PeerOffline_ReturnsPartialWithOnlineResults(t *testing.T) {
	// Two online peers + one offline peer.
	srvA := fakePeerServer([]*kaivuev1.RecordingHit{
		makeHit("site-a-cam1", 1000),
		makeHit("site-a-cam2", 1200),
	}, 0, "")
	defer srvA.Close()

	srvB := fakePeerServer([]*kaivuev1.RecordingHit{
		makeHit("site-b-cam1", 1100),
	}, 0, "")
	defer srvB.Close()

	// Create and immediately close to simulate offline.
	srvOffline := fakePeerServer(nil, 0, "")
	offlineURL := srvOffline.URL
	offlineHTTP := srvOffline.Client()
	srvOffline.Close()

	peers := []search.Peer{
		makePeerFromServer("site-a", srvA),
		makePeerFromServer("site-b", srvB),
		{
			ID:     "site-offline",
			Client: kaivuev1connect.NewFederationPeerServiceClient(offlineHTTP, offlineURL),
		},
	}

	cfg := search.Config{
		PeerTimeout: 2 * time.Second,
		Logger:      discardLog,
	}

	result := search.Search(context.Background(), cfg, peers, &kaivuev1.SearchRecordingsRequest{})

	// Must be partial because one peer is offline.
	assert.True(t, result.Partial, "result must be partial when a peer is offline")

	// Only online peer results should be present.
	assert.Len(t, result.Hits, 3, "should have 3 hits from 2 online peers")

	// Verify hits are from online peers only.
	for _, hit := range result.Hits {
		camID := hit.GetCameraId()
		assert.NotContains(t, camID, "offline",
			"no hits should come from the offline peer")
	}

	// Verify the offline peer has an error entry.
	assert.Contains(t, result.PeerErrors, "site-offline",
		"offline peer must appear in PeerErrors")
	assert.NotContains(t, result.PeerErrors, "site-a",
		"online peers must not appear in PeerErrors")
	assert.NotContains(t, result.PeerErrors, "site-b",
		"online peers must not appear in PeerErrors")

	// Latencies should be tracked for all peers.
	assert.Len(t, result.PeerLatencies, 3, "latencies should be tracked for all peers")

	// Verify sorted ascending by start_time.
	for i := 1; i < len(result.Hits); i++ {
		prev := result.Hits[i-1].GetStartTime().AsTime()
		curr := result.Hits[i].GetStartTime().AsTime()
		assert.False(t, curr.Before(prev),
			"hits must be sorted ascending: hit[%d]=%v >= hit[%d]=%v", i-1, prev, i, curr)
	}
}

func TestFederatedSearch_AllPeersOffline_PartialWithNoHits(t *testing.T) {
	srvOffline1 := fakePeerServer(nil, 0, "")
	off1URL := srvOffline1.URL
	off1HTTP := srvOffline1.Client()
	srvOffline1.Close()

	srvOffline2 := fakePeerServer(nil, 0, "")
	off2URL := srvOffline2.URL
	off2HTTP := srvOffline2.Client()
	srvOffline2.Close()

	peers := []search.Peer{
		{ID: "peer-1", Client: kaivuev1connect.NewFederationPeerServiceClient(off1HTTP, off1URL)},
		{ID: "peer-2", Client: kaivuev1connect.NewFederationPeerServiceClient(off2HTTP, off2URL)},
	}

	cfg := search.Config{
		PeerTimeout: 1 * time.Second,
		Logger:      discardLog,
	}

	result := search.Search(context.Background(), cfg, peers, &kaivuev1.SearchRecordingsRequest{})

	assert.True(t, result.Partial, "must be partial when all peers are offline")
	assert.Empty(t, result.Hits, "no hits when all peers are offline")
	assert.Len(t, result.PeerErrors, 2, "both peers should have errors")
}

// ===========================================================================
// AC-5: Cross-site playback for offline peer returns ErrPeerUnreachable.
// ===========================================================================

func TestPlaybackDelegation_PeerOffline_ReturnsErrPeerUnreachable(t *testing.T) {
	catalog := &fakeCatalog{
		entries: map[string]playback.CatalogEntry{
			"remote-cam-1": {CameraID: "remote-cam-1", PeerID: "peer-down", RecorderID: "rec-1"},
		},
	}

	// Peer is unreachable — factory returns error.
	factory := &fakePeerFactory{clients: map[string]*fakePlaybackClient{}}

	d := playback.NewDelegator(catalog, factory, discardLog)

	_, err := d.Delegate(context.Background(), playback.DelegateRequest{
		CameraID:      "remote-cam-1",
		RequestedKind: 1,
		UserID:        "user-1",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, playback.ErrPeerUnreachable),
		"expected ErrPeerUnreachable, got: %v", err)
}

func TestPlaybackDelegation_PeerReturnsUnavailable_ErrPeerUnreachable(t *testing.T) {
	catalog := &fakeCatalog{
		entries: map[string]playback.CatalogEntry{
			"remote-cam-1": {CameraID: "remote-cam-1", PeerID: "peer-b", RecorderID: "rec-1"},
		},
	}

	peerClient := &fakePlaybackClient{
		mintErr: connect.NewError(connect.CodeUnavailable, errors.New("connection refused")),
	}
	factory := &fakePeerFactory{
		clients: map[string]*fakePlaybackClient{"peer-b": peerClient},
	}

	d := playback.NewDelegator(catalog, factory, discardLog)

	_, err := d.Delegate(context.Background(), playback.DelegateRequest{
		CameraID:      "remote-cam-1",
		RequestedKind: 2,
		UserID:        "user-1",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, playback.ErrPeerUnreachable),
		"expected ErrPeerUnreachable for unavailable, got: %v", err)
}

func TestPlaybackDelegation_PeerTimeout_ErrPeerUnreachable(t *testing.T) {
	catalog := &fakeCatalog{
		entries: map[string]playback.CatalogEntry{
			"remote-cam-1": {CameraID: "remote-cam-1", PeerID: "peer-b", RecorderID: "rec-1"},
		},
	}

	peerClient := &fakePlaybackClient{
		mintErr: connect.NewError(connect.CodeDeadlineExceeded, errors.New("deadline exceeded")),
	}
	factory := &fakePeerFactory{
		clients: map[string]*fakePlaybackClient{"peer-b": peerClient},
	}

	d := playback.NewDelegator(catalog, factory, discardLog,
		playback.WithTimeout(100*time.Millisecond))

	_, err := d.Delegate(context.Background(), playback.DelegateRequest{
		CameraID:      "remote-cam-1",
		RequestedKind: 1,
		UserID:        "user-1",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, playback.ErrPeerUnreachable),
		"expected ErrPeerUnreachable for deadline exceeded, got: %v", err)
}

// ===========================================================================
// AC-6: Local operations unaffected when a remote peer is offline.
// ===========================================================================

func TestLocalOps_Unaffected_WhenPeerOffline(t *testing.T) {
	// This test verifies the broader invariant: local-site camera list,
	// local search, and local playback all work when a federation peer is
	// down.

	t.Run("local_catalog_sync_succeeds_when_remote_down", func(t *testing.T) {
		db := openTestDB(t)
		store := catalogsync.NewStore(db)
		ctx := context.Background()

		seedPeer(t, store, "local", "Local Site")
		seedPeer(t, store, "remote", "Remote Site")

		now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)

		syncer, err := catalogsync.New(catalogsync.Config{
			Store: store,
			ClientFactory: func(p *catalogsync.PeerRecord) (kaivuev1connect.FederationPeerServiceClient, error) {
				if p.PeerID == "local" {
					return &fakeSyncClient{
						cameras: []*kaivuev1.Camera{
							{Id: "l1", Name: "Lobby"},
							{Id: "l2", Name: "Entrance"},
						},
						users:  []*kaivuev1.FederatedUser{{Id: "u1", Username: "admin"}},
						groups: []*kaivuev1.FederatedGroup{{Id: "g1", Name: "admins"}},
					}, nil
				}
				return nil, errors.New("host unreachable")
			},
			Interval: catalogsync.MinInterval,
			Clock:    func() time.Time { return now },
			Logger:   discardLog,
		})
		require.NoError(t, err)

		syncer.SyncOnce(ctx)

		// Local peer fully synced.
		localPeer, _ := store.GetPeer(ctx, "local")
		assert.Equal(t, catalogsync.StatusSynced, localPeer.SyncStatus)

		localCams, _ := store.ListCamerasByPeer(ctx, "local")
		assert.Len(t, localCams, 2)

		localUsers, _ := store.ListUsersByPeer(ctx, "local")
		assert.Len(t, localUsers, 1)

		// Remote peer in error — but this did not block local.
		remotePeer, _ := store.GetPeer(ctx, "remote")
		assert.Equal(t, catalogsync.StatusError, remotePeer.SyncStatus)
	})

	t.Run("local_search_returns_results_when_remote_down", func(t *testing.T) {
		srvLocal := fakePeerServer([]*kaivuev1.RecordingHit{
			makeHit("local-cam", 500),
			makeHit("local-cam", 600),
		}, 0, "")
		defer srvLocal.Close()

		// Offline peer.
		srvOff := fakePeerServer(nil, 0, "")
		offURL := srvOff.URL
		offHTTP := srvOff.Client()
		srvOff.Close()

		peers := []search.Peer{
			makePeerFromServer("local-site", srvLocal),
			{ID: "remote-site", Client: kaivuev1connect.NewFederationPeerServiceClient(offHTTP, offURL)},
		}

		result := search.Search(context.Background(), search.Config{
			PeerTimeout: 1 * time.Second,
			Logger:      discardLog,
		}, peers, &kaivuev1.SearchRecordingsRequest{})

		assert.True(t, result.Partial, "partial because remote is down")
		assert.Len(t, result.Hits, 2, "local search hits must be present")
		for _, h := range result.Hits {
			assert.Equal(t, "local-cam", h.GetCameraId())
		}
	})

	t.Run("local_playback_works_when_remote_peer_down", func(t *testing.T) {
		// Local camera resolves to a local peer that is always reachable.
		catalog := &fakeCatalog{
			entries: map[string]playback.CatalogEntry{
				"local-cam-1": {CameraID: "local-cam-1", PeerID: "local-peer", RecorderID: "rec-local"},
			},
		}

		localPeerClient := &fakePlaybackClient{
			mintResp: &kaivuev1.MintStreamURLResponse{
				Url: "https://local-peer.example.com/webrtc/local-cam-1?token=abc",
				Claims: &kaivuev1.StreamClaims{
					CameraId:   "local-cam-1",
					RecorderId: "rec-local",
					Kind:       1,
				},
				GrantedKind: 1,
			},
		}

		factory := &fakePeerFactory{
			clients: map[string]*fakePlaybackClient{
				"local-peer": localPeerClient,
				// "remote-peer" is NOT in the map — simulating it being down.
			},
		}

		d := playback.NewDelegator(catalog, factory, discardLog)

		resp, err := d.Delegate(context.Background(), playback.DelegateRequest{
			CameraID:      "local-cam-1",
			RequestedKind: 1,
			UserID:        "user-1",
		})

		require.NoError(t, err, "local playback must succeed")
		assert.Contains(t, resp.URL, "local-cam-1")
		assert.Equal(t, "local-peer", resp.PeerID)
	})
}

// ===========================================================================
// AC-7: Mid-operation peer failure — operation degrades gracefully,
//        no crash/hang.
// ===========================================================================

func TestMidOperation_SearchPeerDies_NoHangNoCrash(t *testing.T) {
	// Peer that hangs (simulating mid-operation freeze), with a short timeout.
	srvHanging := fakePeerServer(nil, 30*time.Second, "")
	defer srvHanging.Close()

	srvHealthy := fakePeerServer([]*kaivuev1.RecordingHit{
		makeHit("healthy-cam", 100),
	}, 0, "")
	defer srvHealthy.Close()

	peers := []search.Peer{
		makePeerFromServer("hanging-peer", srvHanging),
		makePeerFromServer("healthy-peer", srvHealthy),
	}

	cfg := search.Config{
		PeerTimeout: 500 * time.Millisecond,
		Logger:      discardLog,
	}

	start := time.Now()
	result := search.Search(context.Background(), cfg, peers, &kaivuev1.SearchRecordingsRequest{})
	elapsed := time.Since(start)

	// Must complete within the timeout window, not hang for 30s.
	assert.Less(t, elapsed, 3*time.Second,
		"search must not hang; elapsed: %v", elapsed)

	assert.True(t, result.Partial, "must be partial when peer hangs")
	assert.Len(t, result.Hits, 1, "healthy peer's hits must be returned")
	assert.Contains(t, result.PeerErrors, "hanging-peer")
}

func TestMidOperation_PlaybackPeerDies_CleanError(t *testing.T) {
	catalog := &fakeCatalog{
		entries: map[string]playback.CatalogEntry{
			"cam-x": {CameraID: "cam-x", PeerID: "dying-peer", RecorderID: "rec-1"},
		},
	}

	dyingClient := &fakePlaybackClient{
		mintErr: errors.New("broken pipe: connection reset"),
	}
	factory := &fakePeerFactory{
		clients: map[string]*fakePlaybackClient{"dying-peer": dyingClient},
	}

	d := playback.NewDelegator(catalog, factory, discardLog,
		playback.WithTimeout(500*time.Millisecond))

	start := time.Now()
	_, err := d.Delegate(context.Background(), playback.DelegateRequest{
		CameraID:      "cam-x",
		RequestedKind: 1,
		UserID:        "user-1",
	})
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.True(t, errors.Is(err, playback.ErrPeerUnreachable),
		"must return ErrPeerUnreachable for network error, got: %v", err)
	assert.Less(t, elapsed, 2*time.Second,
		"must not hang; elapsed: %v", elapsed)
}

func TestMidOperation_CatalogSync_PeerDiesMidSync_OtherPeersUnaffected(t *testing.T) {
	db := openTestDB(t)
	store := catalogsync.NewStore(db)
	ctx := context.Background()

	seedPeer(t, store, "peer-ok", "OK Site")
	seedPeer(t, store, "peer-dying", "Dying Site")

	var okCompleted atomic.Bool

	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)

	syncer, err := catalogsync.New(catalogsync.Config{
		Store: store,
		ClientFactory: func(p *catalogsync.PeerRecord) (kaivuev1connect.FederationPeerServiceClient, error) {
			switch p.PeerID {
			case "peer-ok":
				okCompleted.Store(true)
				return &fakeSyncClient{
					cameras: []*kaivuev1.Camera{{Id: "ok-cam", Name: "OK Camera"}},
					users:   []*kaivuev1.FederatedUser{},
					groups:  []*kaivuev1.FederatedGroup{},
				}, nil
			case "peer-dying":
				// Simulate mid-operation failure: client creation succeeds but
				// the RPC fails.
				return &fakeSyncClient{
					err: connect.NewError(connect.CodeUnavailable, errors.New("connection reset")),
				}, nil
			}
			return nil, errors.New("unknown")
		},
		Interval: catalogsync.MinInterval,
		Clock:    func() time.Time { return now },
		Logger:   discardLog,
	})
	require.NoError(t, err)

	// SyncOnce must complete without panic or deadlock.
	done := make(chan struct{})
	go func() {
		syncer.SyncOnce(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Good — completed.
	case <-time.After(10 * time.Second):
		t.Fatal("SyncOnce hung — deadlock detected")
	}

	assert.True(t, okCompleted.Load(), "OK peer should have been synced")

	okPeer, _ := store.GetPeer(ctx, "peer-ok")
	assert.Equal(t, catalogsync.StatusSynced, okPeer.SyncStatus)

	dyingPeer, _ := store.GetPeer(ctx, "peer-dying")
	assert.Equal(t, catalogsync.StatusError, dyingPeer.SyncStatus)
}
