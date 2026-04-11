package catalogsync

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	connect "connectrpc.com/connect"
	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
	"github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1/kaivuev1connect"
)

// DefaultInterval is the default catalog sync interval (5 minutes).
const DefaultInterval = 5 * time.Minute

// MinInterval is the minimum configurable sync interval (30 seconds).
const MinInterval = 30 * time.Second

// PeerClientFactory creates a FederationPeerServiceClient for the given peer.
// The syncer calls this for each peer on every sync cycle so the factory can
// handle per-peer mTLS, auth tokens, and connection lifecycle.
type PeerClientFactory func(peer *PeerRecord) (kaivuev1connect.FederationPeerServiceClient, error)

// Config parameterises a CatalogSyncer.
type Config struct {
	// Store is the SQLite-backed catalog cache. Required.
	Store *Store

	// ClientFactory creates a FederationPeerServiceClient per peer. Required.
	ClientFactory PeerClientFactory

	// Interval is the sync interval. Clamped to MinInterval.
	// Zero defaults to DefaultInterval.
	Interval time.Duration

	// StaleMultiplier controls when a peer is marked stale. If the last
	// successful sync is older than StaleMultiplier * Interval, the peer
	// is marked stale. Defaults to 2.
	StaleMultiplier int

	// Logger is the slog logger. nil defaults to slog.Default().
	Logger *slog.Logger

	// Clock overrides time.Now for tests. nil = time.Now.
	Clock func() time.Time

	// PageSize is the number of items to request per page from the peer.
	// Zero defaults to 500.
	PageSize int32
}

// CatalogSyncer runs periodic catalog synchronisation for all registered
// federation peers. Each peer is synced independently; a failure in one
// peer does not block the others.
type CatalogSyncer struct {
	cfg   Config
	store *Store
	log   *slog.Logger
	now   func() time.Time

	mu      sync.Mutex
	cancel  context.CancelFunc
	running bool
}

// New constructs a CatalogSyncer. Call Start to begin the background loop.
func New(cfg Config) (*CatalogSyncer, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("catalogsync: Config.Store is required")
	}
	if cfg.ClientFactory == nil {
		return nil, fmt.Errorf("catalogsync: Config.ClientFactory is required")
	}
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultInterval
	}
	if cfg.Interval < MinInterval {
		cfg.Interval = MinInterval
	}
	if cfg.StaleMultiplier <= 0 {
		cfg.StaleMultiplier = 2
	}
	if cfg.PageSize <= 0 {
		cfg.PageSize = 500
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	now := cfg.Clock
	if now == nil {
		now = time.Now
	}

	return &CatalogSyncer{
		cfg:   cfg,
		store: cfg.Store,
		log:   log.With("component", "catalogsync"),
		now:   now,
	}, nil
}

// Start launches the background sync loop. It is safe to call multiple times;
// only the first call takes effect until Stop is called.
func (cs *CatalogSyncer) Start(ctx context.Context) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.running {
		return
	}
	ctx, cs.cancel = context.WithCancel(ctx)
	cs.running = true
	go cs.loop(ctx)
}

// Stop cancels the background sync loop. Blocks until the loop exits.
func (cs *CatalogSyncer) Stop() {
	cs.mu.Lock()
	if !cs.running {
		cs.mu.Unlock()
		return
	}
	cs.cancel()
	cs.running = false
	cs.mu.Unlock()
}

// SyncOnce performs a single synchronisation cycle for all peers. This is
// exported for tests and manual trigger from admin endpoints.
func (cs *CatalogSyncer) SyncOnce(ctx context.Context) {
	peers, err := cs.store.ListPeers(ctx)
	if err != nil {
		cs.log.Error("failed to list peers", "error", err)
		return
	}

	// Sync each peer independently using a WaitGroup so failures are isolated.
	var wg sync.WaitGroup
	for _, peer := range peers {
		wg.Add(1)
		go func(p *PeerRecord) {
			defer wg.Done()
			cs.syncPeer(ctx, p)
		}(peer)
	}
	wg.Wait()

	// After all syncs, mark stale peers.
	threshold := cs.now().Add(-time.Duration(cs.cfg.StaleMultiplier) * cs.cfg.Interval)
	if n, err := cs.store.MarkStale(ctx, threshold); err != nil {
		cs.log.Error("failed to mark stale peers", "error", err)
	} else if n > 0 {
		cs.log.Warn("marked peers as stale", "count", n)
	}
}

// loop is the background ticker goroutine.
func (cs *CatalogSyncer) loop(ctx context.Context) {
	// Run once immediately on start.
	cs.SyncOnce(ctx)

	ticker := time.NewTicker(cs.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cs.SyncOnce(ctx)
		}
	}
}

// syncPeer synchronises a single peer's catalog. Errors are logged and
// recorded in the peer's sync_status/sync_error columns but do NOT
// propagate to the caller.
func (cs *CatalogSyncer) syncPeer(ctx context.Context, peer *PeerRecord) {
	log := cs.log.With("peer_id", peer.PeerID, "peer_name", peer.Name)
	log.Info("starting catalog sync")

	client, err := cs.cfg.ClientFactory(peer)
	if err != nil {
		cs.recordError(ctx, peer, fmt.Errorf("create client: %w", err))
		return
	}

	now := cs.now()

	// Fetch cameras.
	cameras, err := cs.fetchAllCameras(ctx, client, peer)
	if err != nil {
		cs.recordError(ctx, peer, fmt.Errorf("fetch cameras: %w", err))
		return
	}

	// Fetch users.
	users, err := cs.fetchAllUsers(ctx, client, peer)
	if err != nil {
		cs.recordError(ctx, peer, fmt.Errorf("fetch users: %w", err))
		return
	}

	// Fetch groups.
	groups, err := cs.fetchAllGroups(ctx, client, peer)
	if err != nil {
		cs.recordError(ctx, peer, fmt.Errorf("fetch groups: %w", err))
		return
	}

	// Store results.
	if err := cs.store.ReplaceCameras(ctx, peer.PeerID, cameras); err != nil {
		cs.recordError(ctx, peer, fmt.Errorf("store cameras: %w", err))
		return
	}
	if err := cs.store.ReplaceUsers(ctx, peer.PeerID, users); err != nil {
		cs.recordError(ctx, peer, fmt.Errorf("store users: %w", err))
		return
	}
	if err := cs.store.ReplaceGroups(ctx, peer.PeerID, groups); err != nil {
		cs.recordError(ctx, peer, fmt.Errorf("store groups: %w", err))
		return
	}

	// Update peer status to synced.
	peer.LastSyncAt = &now
	peer.SyncStatus = StatusSynced
	peer.SyncError = ""
	if err := cs.store.UpsertPeer(ctx, peer); err != nil {
		log.Error("failed to update peer status", "error", err)
		return
	}

	log.Info("catalog sync completed",
		"cameras", len(cameras),
		"users", len(users),
		"groups", len(groups),
	)
}

// recordError updates the peer status to error and logs the failure.
func (cs *CatalogSyncer) recordError(ctx context.Context, peer *PeerRecord, syncErr error) {
	cs.log.Error("catalog sync failed",
		"peer_id", peer.PeerID,
		"peer_name", peer.Name,
		"error", syncErr,
	)
	peer.SyncStatus = StatusError
	peer.SyncError = syncErr.Error()
	if err := cs.store.UpsertPeer(ctx, peer); err != nil {
		cs.log.Error("failed to record sync error", "peer_id", peer.PeerID, "error", err)
	}
}

// fetchAllCameras paginates through the peer's ListCameras RPC.
func (cs *CatalogSyncer) fetchAllCameras(
	ctx context.Context,
	client kaivuev1connect.FederationPeerServiceClient,
	peer *PeerRecord,
) ([]CachedCamera, error) {
	now := cs.now()
	var all []CachedCamera
	cursor := ""
	for {
		resp, err := client.ListCameras(ctx, connect.NewRequest(&kaivuev1.FederationPeerServiceListCamerasRequest{
			Tenant: &kaivuev1.TenantRef{
				Type: kaivuev1.TenantType(peer.TenantType),
				Id:   peer.TenantID,
			},
			PageSize: cs.cfg.PageSize,
			Cursor:   cursor,
		}))
		if err != nil {
			return nil, err
		}
		for _, cam := range resp.Msg.Cameras {
			all = append(all, CachedCamera{
				ID:           cam.Id,
				PeerID:       peer.PeerID,
				Name:         cam.Name,
				RecorderID:   cam.RecorderId,
				Manufacturer: cam.Manufacturer,
				Model:        cam.Model,
				IPAddress:    cam.IpAddress,
				State:        int32(cam.State),
				Labels:       cam.Labels,
				SyncedAt:     now,
			})
		}
		cursor = resp.Msg.NextCursor
		if cursor == "" {
			break
		}
	}
	return all, nil
}

// fetchAllUsers paginates through the peer's ListUsers RPC.
func (cs *CatalogSyncer) fetchAllUsers(
	ctx context.Context,
	client kaivuev1connect.FederationPeerServiceClient,
	peer *PeerRecord,
) ([]CachedUser, error) {
	now := cs.now()
	var all []CachedUser
	cursor := ""
	for {
		resp, err := client.ListUsers(ctx, connect.NewRequest(&kaivuev1.ListUsersRequest{
			Tenant: &kaivuev1.TenantRef{
				Type: kaivuev1.TenantType(peer.TenantType),
				Id:   peer.TenantID,
			},
			PageSize: cs.cfg.PageSize,
			Cursor:   cursor,
		}))
		if err != nil {
			return nil, err
		}
		for _, u := range resp.Msg.Users {
			all = append(all, CachedUser{
				ID:          u.Id,
				PeerID:      peer.PeerID,
				Username:    u.Username,
				Email:       u.Email,
				DisplayName: u.DisplayName,
				Groups:      u.Groups,
				Disabled:    u.Disabled,
				SyncedAt:    now,
			})
		}
		cursor = resp.Msg.NextCursor
		if cursor == "" {
			break
		}
	}
	return all, nil
}

// fetchAllGroups paginates through the peer's ListGroups RPC.
func (cs *CatalogSyncer) fetchAllGroups(
	ctx context.Context,
	client kaivuev1connect.FederationPeerServiceClient,
	peer *PeerRecord,
) ([]CachedGroup, error) {
	now := cs.now()
	var all []CachedGroup
	cursor := ""
	for {
		resp, err := client.ListGroups(ctx, connect.NewRequest(&kaivuev1.ListGroupsRequest{
			Tenant: &kaivuev1.TenantRef{
				Type: kaivuev1.TenantType(peer.TenantType),
				Id:   peer.TenantID,
			},
			PageSize: cs.cfg.PageSize,
			Cursor:   cursor,
		}))
		if err != nil {
			return nil, err
		}
		for _, g := range resp.Msg.Groups {
			all = append(all, CachedGroup{
				ID:          g.Id,
				PeerID:      peer.PeerID,
				Name:        g.Name,
				Description: g.Description,
				SyncedAt:    now,
			})
		}
		cursor = resp.Msg.NextCursor
		if cursor == "" {
			break
		}
	}
	return all, nil
}
