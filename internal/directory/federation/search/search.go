// Package search implements cross-site federated recording search.
//
// FederatedSearch fans out a SearchRecordings RPC to every known peer in
// parallel, enforces a per-peer timeout (default 10 s), merges results by
// timestamp, and reports partial=true when any peer is unreachable or slow.
package search

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"connectrpc.com/connect"

	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
	"github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1/kaivuev1connect"
)

// DefaultPeerTimeout is the per-peer deadline for SearchRecordings RPCs.
const DefaultPeerTimeout = 10 * time.Second

// Peer is the minimal abstraction a caller must provide for each federation
// peer. PeerID is an opaque identifier used in logs and latency metrics.
type Peer struct {
	ID     string
	Client kaivuev1connect.FederationPeerServiceClient
}

// Result is the merged, sorted output of a federated search.
type Result struct {
	// Hits contains every RecordingHit received, sorted ascending by StartTime.
	Hits []*kaivuev1.RecordingHit

	// Partial is true when at least one peer failed or timed out.
	Partial bool

	// PeerErrors maps peer ID to the error encountered (timeout, connection
	// refused, etc.). Only contains entries for peers that failed.
	PeerErrors map[string]error

	// PeerLatencies maps peer ID to the wall-clock duration of its RPC.
	PeerLatencies map[string]time.Duration
}

// Config tunes the fan-out behaviour.
type Config struct {
	// PeerTimeout is the per-peer context deadline. Zero means DefaultPeerTimeout.
	PeerTimeout time.Duration

	// Logger receives structured diagnostics. Nil defaults to slog.Default().
	Logger *slog.Logger
}

func (c *Config) peerTimeout() time.Duration {
	if c.PeerTimeout > 0 {
		return c.PeerTimeout
	}
	return DefaultPeerTimeout
}

func (c *Config) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// peerResult holds the outcome of a single peer's SearchRecordings RPC.
type peerResult struct {
	peerID  string
	hits    []*kaivuev1.RecordingHit
	err     error
	latency time.Duration
}

// Search fans out req to every peer in parallel, each with its own timeout,
// collects results, and returns the merged Result. The parent ctx is respected
// as an overall deadline on top of the per-peer timeouts.
func Search(ctx context.Context, cfg Config, peers []Peer, req *kaivuev1.SearchRecordingsRequest) *Result {
	log := cfg.logger().With(slog.String("component", "federation/search"))

	res := &Result{
		PeerErrors:    make(map[string]error, len(peers)),
		PeerLatencies: make(map[string]time.Duration, len(peers)),
	}

	if len(peers) == 0 {
		return res
	}

	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		collected []peerResult
	)

	for _, p := range peers {
		wg.Add(1)
		go func(peer Peer) {
			defer wg.Done()

			pr := queryPeer(ctx, cfg, peer, req, log)

			mu.Lock()
			collected = append(collected, pr)
			mu.Unlock()
		}(p)
	}

	wg.Wait()

	// Merge.
	for _, pr := range collected {
		res.PeerLatencies[pr.peerID] = pr.latency
		if pr.err != nil {
			res.Partial = true
			res.PeerErrors[pr.peerID] = pr.err
			log.WarnContext(ctx, "peer search failed",
				slog.String("peer", pr.peerID),
				slog.Duration("latency", pr.latency),
				slog.String("error", pr.err.Error()),
			)
		} else {
			res.Hits = append(res.Hits, pr.hits...)
		}
	}

	// Sort merged hits ascending by start_time.
	sort.Slice(res.Hits, func(i, j int) bool {
		ti := res.Hits[i].GetStartTime().AsTime()
		tj := res.Hits[j].GetStartTime().AsTime()
		return ti.Before(tj)
	})

	log.InfoContext(ctx, "federated search complete",
		slog.Int("total_hits", len(res.Hits)),
		slog.Bool("partial", res.Partial),
		slog.Int("peers_queried", len(peers)),
		slog.Int("peers_failed", len(res.PeerErrors)),
	)

	return res
}

// queryPeer issues the SearchRecordings server-streaming RPC to a single peer
// with an isolated per-peer timeout. It drains the stream and returns all hits.
func queryPeer(
	parentCtx context.Context,
	cfg Config,
	peer Peer,
	req *kaivuev1.SearchRecordingsRequest,
	log *slog.Logger,
) peerResult {
	start := time.Now()

	peerCtx, cancel := context.WithTimeout(parentCtx, cfg.peerTimeout())
	defer cancel()

	connectReq := connect.NewRequest(req)

	stream, err := peer.Client.SearchRecordings(peerCtx, connectReq)
	if err != nil {
		return peerResult{
			peerID:  peer.ID,
			err:     err,
			latency: time.Since(start),
		}
	}
	defer stream.Close()

	var hits []*kaivuev1.RecordingHit
	for stream.Receive() {
		if hit := stream.Msg().GetHit(); hit != nil {
			hits = append(hits, hit)
		}
	}

	if err := stream.Err(); err != nil {
		return peerResult{
			peerID:  peer.ID,
			hits:    hits, // return whatever we got before the error
			err:     err,
			latency: time.Since(start),
		}
	}

	log.DebugContext(parentCtx, "peer search complete",
		slog.String("peer", peer.ID),
		slog.Int("hits", len(hits)),
		slog.Duration("latency", time.Since(start)),
	)

	return peerResult{
		peerID:  peer.ID,
		hits:    hits,
		latency: time.Since(start),
	}
}

