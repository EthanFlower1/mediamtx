package search

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
	"github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1/kaivuev1connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ---------------------------------------------------------------------------
// httptest-based fake peer server
// ---------------------------------------------------------------------------

// fakePeerHandler implements the server-side SearchRecordings handler.
type fakePeerHandler struct {
	kaivuev1connect.UnimplementedFederationPeerServiceHandler

	hits   []*kaivuev1.RecordingHit
	delay  time.Duration
	errMsg string
}

func (h *fakePeerHandler) SearchRecordings(
	ctx context.Context,
	_ *connect.Request[kaivuev1.SearchRecordingsRequest],
	stream *connect.ServerStream[kaivuev1.SearchRecordingsResponse],
) error {
	if h.delay > 0 {
		select {
		case <-time.After(h.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if h.errMsg != "" {
		return connect.NewError(connect.CodeInternal, errors.New(h.errMsg))
	}
	for _, hit := range h.hits {
		if err := stream.Send(&kaivuev1.SearchRecordingsResponse{Hit: hit}); err != nil {
			return err
		}
	}
	return nil
}

// fakePeerServer creates an httptest server that serves the FederationPeerService.
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func makeHit(cameraID string, startSec int64) *kaivuev1.RecordingHit {
	return &kaivuev1.RecordingHit{
		CameraId:  cameraID,
		SegmentId: cameraID + "-seg",
		StartTime: timestamppb.New(time.Unix(startSec, 0)),
		EndTime:   timestamppb.New(time.Unix(startSec+60, 0)),
	}
}

func makePeerFromServer(id string, srv *httptest.Server) Peer {
	return Peer{
		ID:     id,
		Client: kaivuev1connect.NewFederationPeerServiceClient(srv.Client(), srv.URL),
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestSearch_AllPeersRespond(t *testing.T) {
	hits1 := []*kaivuev1.RecordingHit{
		makeHit("cam-a", 100),
		makeHit("cam-a", 300),
	}
	hits2 := []*kaivuev1.RecordingHit{
		makeHit("cam-b", 200),
		makeHit("cam-b", 400),
	}

	srv1 := fakePeerServer(hits1, 0, "")
	defer srv1.Close()
	srv2 := fakePeerServer(hits2, 0, "")
	defer srv2.Close()

	peers := []Peer{
		makePeerFromServer("peer-1", srv1),
		makePeerFromServer("peer-2", srv2),
	}

	cfg := Config{
		PeerTimeout: 5 * time.Second,
		Logger:      discardLogger(),
	}

	res := Search(context.Background(), cfg, peers, &kaivuev1.SearchRecordingsRequest{})

	if res.Partial {
		t.Fatal("expected partial=false when all peers respond")
	}
	if len(res.Hits) != 4 {
		t.Fatalf("expected 4 hits, got %d", len(res.Hits))
	}
	// Verify sorted order.
	for i := 1; i < len(res.Hits); i++ {
		prev := res.Hits[i-1].GetStartTime().AsTime()
		curr := res.Hits[i].GetStartTime().AsTime()
		if curr.Before(prev) {
			t.Errorf("hit[%d] (%v) is before hit[%d] (%v)", i, curr, i-1, prev)
		}
	}
	if len(res.PeerErrors) != 0 {
		t.Errorf("expected no peer errors, got %v", res.PeerErrors)
	}
	for _, pid := range []string{"peer-1", "peer-2"} {
		if _, ok := res.PeerLatencies[pid]; !ok {
			t.Errorf("missing latency for %s", pid)
		}
	}
}

func TestSearch_OnePeerOffline_ReturnsPartial(t *testing.T) {
	hits1 := []*kaivuev1.RecordingHit{makeHit("cam-a", 100)}

	srv1 := fakePeerServer(hits1, 0, "")
	defer srv1.Close()

	// Create and immediately close to simulate offline.
	srvOffline := fakePeerServer(nil, 0, "")
	offlineURL := srvOffline.URL
	offlineClient := srvOffline.Client()
	srvOffline.Close()

	peers := []Peer{
		makePeerFromServer("peer-1", srv1),
		{
			ID:     "peer-offline",
			Client: kaivuev1connect.NewFederationPeerServiceClient(offlineClient, offlineURL),
		},
	}

	cfg := Config{
		PeerTimeout: 2 * time.Second,
		Logger:      discardLogger(),
	}

	res := Search(context.Background(), cfg, peers, &kaivuev1.SearchRecordingsRequest{})

	if !res.Partial {
		t.Fatal("expected partial=true when one peer is offline")
	}
	if len(res.Hits) != 1 {
		t.Fatalf("expected 1 hit from online peer, got %d", len(res.Hits))
	}
	if res.Hits[0].GetCameraId() != "cam-a" {
		t.Errorf("expected cam-a hit, got %s", res.Hits[0].GetCameraId())
	}
	if _, ok := res.PeerErrors["peer-offline"]; !ok {
		t.Error("expected error entry for peer-offline")
	}
}

func TestSearch_PeerTimeout_EnforcedRegardlessOfBehavior(t *testing.T) {
	// peer-slow delays 5s, but timeout is 500ms.
	srvSlow := fakePeerServer(nil, 5*time.Second, "")
	defer srvSlow.Close()

	peers := []Peer{makePeerFromServer("peer-slow", srvSlow)}

	cfg := Config{
		PeerTimeout: 500 * time.Millisecond,
		Logger:      discardLogger(),
	}

	start := time.Now()
	res := Search(context.Background(), cfg, peers, &kaivuev1.SearchRecordingsRequest{})
	elapsed := time.Since(start)

	if !res.Partial {
		t.Fatal("expected partial=true when peer times out")
	}
	if _, ok := res.PeerErrors["peer-slow"]; !ok {
		t.Error("expected error entry for peer-slow")
	}
	if elapsed > 2*time.Second {
		t.Errorf("timeout not enforced: took %v", elapsed)
	}
}

func TestSearch_PeerReturnsError_Partial(t *testing.T) {
	srvErr := fakePeerServer(nil, 0, "internal storage failure")
	defer srvErr.Close()

	peers := []Peer{makePeerFromServer("peer-err", srvErr)}

	cfg := Config{
		PeerTimeout: 5 * time.Second,
		Logger:      discardLogger(),
	}

	res := Search(context.Background(), cfg, peers, &kaivuev1.SearchRecordingsRequest{})

	if !res.Partial {
		t.Fatal("expected partial=true when peer returns error")
	}
	if len(res.Hits) != 0 {
		t.Errorf("expected 0 hits, got %d", len(res.Hits))
	}
}

func TestSearch_NoPeers(t *testing.T) {
	cfg := Config{Logger: discardLogger()}
	res := Search(context.Background(), cfg, nil, &kaivuev1.SearchRecordingsRequest{})

	if res.Partial {
		t.Error("expected partial=false with no peers")
	}
	if len(res.Hits) != 0 {
		t.Errorf("expected 0 hits, got %d", len(res.Hits))
	}
}

func TestSearch_TwoPeersOnlineOneOffline_Integration(t *testing.T) {
	// Main acceptance-criteria test: two peers + one offline.
	hits1 := []*kaivuev1.RecordingHit{
		makeHit("site-a-cam1", 1000),
		makeHit("site-a-cam2", 1100),
	}
	hits2 := []*kaivuev1.RecordingHit{
		makeHit("site-b-cam1", 1050),
	}

	srv1 := fakePeerServer(hits1, 0, "")
	defer srv1.Close()
	srv2 := fakePeerServer(hits2, 0, "")
	defer srv2.Close()

	srvOffline := fakePeerServer(nil, 0, "")
	offlineURL := srvOffline.URL
	offlineClient := srvOffline.Client()
	srvOffline.Close()

	peers := []Peer{
		makePeerFromServer("site-a", srv1),
		makePeerFromServer("site-b", srv2),
		{
			ID:     "site-offline",
			Client: kaivuev1connect.NewFederationPeerServiceClient(offlineClient, offlineURL),
		},
	}

	cfg := Config{
		PeerTimeout: 2 * time.Second,
		Logger:      discardLogger(),
	}

	res := Search(context.Background(), cfg, peers, &kaivuev1.SearchRecordingsRequest{})

	if !res.Partial {
		t.Fatal("expected partial=true with one offline peer")
	}
	if len(res.Hits) != 3 {
		t.Fatalf("expected 3 hits, got %d", len(res.Hits))
	}

	// Verify sorted ascending by start_time.
	expectedOrder := []string{"site-a-cam1", "site-b-cam1", "site-a-cam2"}
	for i, expected := range expectedOrder {
		if res.Hits[i].GetCameraId() != expected {
			t.Errorf("hit[%d]: expected camera %s, got %s", i, expected, res.Hits[i].GetCameraId())
		}
	}

	// Verify latencies for all peers.
	for _, pid := range []string{"site-a", "site-b", "site-offline"} {
		lat, ok := res.PeerLatencies[pid]
		if !ok {
			t.Errorf("missing latency for %s", pid)
		}
		if lat <= 0 {
			t.Errorf("latency for %s should be positive, got %v", pid, lat)
		}
	}

	if len(res.PeerErrors) != 1 {
		t.Errorf("expected 1 peer error, got %d", len(res.PeerErrors))
	}
	if _, ok := res.PeerErrors["site-offline"]; !ok {
		t.Error("expected error for site-offline")
	}
}

func TestSearch_ResultMergingSortOrder(t *testing.T) {
	srv1 := fakePeerServer([]*kaivuev1.RecordingHit{
		makeHit("c1", 500),
		makeHit("c1", 100),
	}, 0, "")
	defer srv1.Close()

	srv2 := fakePeerServer([]*kaivuev1.RecordingHit{
		makeHit("c2", 300),
	}, 0, "")
	defer srv2.Close()

	srv3 := fakePeerServer([]*kaivuev1.RecordingHit{
		makeHit("c3", 200),
		makeHit("c3", 400),
	}, 0, "")
	defer srv3.Close()

	peers := []Peer{
		makePeerFromServer("p1", srv1),
		makePeerFromServer("p2", srv2),
		makePeerFromServer("p3", srv3),
	}

	cfg := Config{
		PeerTimeout: 5 * time.Second,
		Logger:      discardLogger(),
	}

	res := Search(context.Background(), cfg, peers, &kaivuev1.SearchRecordingsRequest{})

	if res.Partial {
		t.Fatal("expected partial=false")
	}
	if len(res.Hits) != 5 {
		t.Fatalf("expected 5 hits, got %d", len(res.Hits))
	}

	expectedTimes := []int64{100, 200, 300, 400, 500}
	for i, expected := range expectedTimes {
		got := res.Hits[i].GetStartTime().AsTime().Unix()
		if got != expected {
			t.Errorf("hit[%d]: expected start_time %d, got %d", i, expected, got)
		}
	}
}

func TestSearch_DefaultTimeout(t *testing.T) {
	cfg := Config{}
	if cfg.peerTimeout() != DefaultPeerTimeout {
		t.Errorf("expected default timeout %v, got %v", DefaultPeerTimeout, cfg.peerTimeout())
	}
}

func TestSearch_PerPeerLatencyInstrumented(t *testing.T) {
	srvFast := fakePeerServer([]*kaivuev1.RecordingHit{makeHit("fast", 1)}, 0, "")
	defer srvFast.Close()

	srvSlow := fakePeerServer([]*kaivuev1.RecordingHit{makeHit("slow", 2)}, 200*time.Millisecond, "")
	defer srvSlow.Close()

	peers := []Peer{
		makePeerFromServer("fast-peer", srvFast),
		makePeerFromServer("slow-peer", srvSlow),
	}

	cfg := Config{
		PeerTimeout: 5 * time.Second,
		Logger:      discardLogger(),
	}

	res := Search(context.Background(), cfg, peers, &kaivuev1.SearchRecordingsRequest{})

	slowLat := res.PeerLatencies["slow-peer"]
	if slowLat < 150*time.Millisecond {
		t.Errorf("slow peer latency should be >= 150ms, got %v", slowLat)
	}
}
