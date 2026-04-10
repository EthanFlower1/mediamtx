package federation

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/bluenviron/mediamtx/internal/shared/permissions"
	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
	"github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1/kaivuev1connect"
)

// --- stubs -------------------------------------------------------------------

type stubPeerExtractor struct {
	id  string
	err error
}

func (s *stubPeerExtractor) Extract(_ context.Context) (PeerIdentity, error) {
	if s.err != nil {
		return PeerIdentity{}, s.err
	}
	return PeerIdentity{PeerDirectoryID: s.id}, nil
}

type stubRecordingIndex struct {
	entries []RecordingEntry
	err     error
}

func (s *stubRecordingIndex) Search(
	_ context.Context, _ string, _ []string,
	_, _ time.Time, _ []kaivuev1.AIEventKind, _ string, _ int,
) ([]RecordingEntry, error) {
	return s.entries, s.err
}

type stubCameraRegistry struct {
	cameras map[string]string // cameraID -> tenantID
	urls    map[string]string // cameraID -> base URL
	recIDs  map[string]string // cameraID -> recorder ID
}

func (s *stubCameraRegistry) CameraTenantID(_ context.Context, id string) (string, error) {
	t, ok := s.cameras[id]
	if !ok {
		return "", fmt.Errorf("unknown camera %q", id)
	}
	return t, nil
}

func (s *stubCameraRegistry) CameraRecorderBaseURL(_ context.Context, id string) (string, string, error) {
	u, ok := s.urls[id]
	if !ok {
		return "", "", fmt.Errorf("unknown camera %q", id)
	}
	return u, s.recIDs[id], nil
}

type stubSigner struct {
	token string
	err   error
}

func (s *stubSigner) SignStreamToken(_ StreamTokenClaims) (string, error) {
	return s.token, s.err
}

// --- helpers -----------------------------------------------------------------

const (
	peerID   = "peer-dir-1"
	tenantID = "tenant-A"
	camID    = "cam-1"
	recID    = "rec-1"
	recURL   = "https://recorder.example.com"
)

var testTenant = &kaivuev1.TenantRef{
	Type: kaivuev1.TenantType_TENANT_TYPE_CUSTOMER,
	Id:   tenantID,
}

var fixedNow = time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)

func newTestEnforcer(t *testing.T, policies ...permissions.PolicyRule) *permissions.Enforcer {
	t.Helper()
	store := permissions.NewInMemoryStore()
	for _, p := range policies {
		if err := store.AddPolicy(p); err != nil {
			t.Fatal(err)
		}
	}
	e, err := permissions.NewEnforcer(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func grantPolicy(sub, obj, act string) permissions.PolicyRule {
	return permissions.PolicyRule{Sub: sub, Obj: obj, Act: act, Eft: "allow"}
}

func federationSub() string {
	return fmt.Sprintf("federation:%s", peerID)
}

func newTestHandler(t *testing.T, enforcer *permissions.Enforcer, index RecordingIndex) *Handler {
	t.Helper()
	h, err := NewHandler(HandlerConfig{
		Enforcer: enforcer,
		Index:    index,
		Cameras: &stubCameraRegistry{
			cameras: map[string]string{camID: tenantID},
			urls:    map[string]string{camID: recURL},
			recIDs:  map[string]string{camID: recID},
		},
		Signer:  &stubSigner{token: "signed-test-token"},
		PeerID:  &stubPeerExtractor{id: peerID},
		BaseURL: "https://directory.example.com",
		NowFunc: func() time.Time { return fixedNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// newTestClient sets up an httptest server with the handler and returns a
// connect client pointed at it.
func newTestClient(t *testing.T, h *Handler) (kaivuev1connect.FederationPeerServiceClient, func()) {
	t.Helper()
	path, handler := kaivuev1connect.NewFederationPeerServiceHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	client := kaivuev1connect.NewFederationPeerServiceClient(srv.Client(), srv.URL)
	return client, srv.Close
}

// --- SearchRecordings tests --------------------------------------------------

func TestSearchRecordings_WithGrants_ReturnsResults(t *testing.T) {
	enforcer := newTestEnforcer(t,
		grantPolicy(federationSub(), tenantID+"/recordings/*", permissions.ActionViewPlayback),
	)

	now := time.Now()
	entries := []RecordingEntry{
		{
			TenantID:    tenantID,
			RecorderID:  recID,
			CameraID:    camID,
			SegmentID:   "seg-1",
			StartTime:   now.Add(-1 * time.Hour),
			EndTime:     now.Add(-30 * time.Minute),
			Bytes:       1024000,
			IsEventClip: false,
		},
		{
			TenantID:    tenantID,
			RecorderID:  recID,
			CameraID:    camID,
			SegmentID:   "seg-2",
			StartTime:   now.Add(-30 * time.Minute),
			EndTime:     now,
			Bytes:       2048000,
			IsEventClip: true,
			EventIDs:    []string{"ev-1"},
		},
	}

	h := newTestHandler(t, enforcer, &stubRecordingIndex{entries: entries})
	client, cleanup := newTestClient(t, h)
	defer cleanup()

	stream, err := client.SearchRecordings(context.Background(), connect.NewRequest(&kaivuev1.SearchRecordingsRequest{
		Tenant:   testTenant,
		PageSize: 50,
	}))
	if err != nil {
		t.Fatalf("SearchRecordings: %v", err)
	}

	var hits []*kaivuev1.RecordingHit
	for stream.Receive() {
		hits = append(hits, stream.Msg().Hit)
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}

	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].SegmentId != "seg-1" {
		t.Errorf("hit[0].SegmentId = %q, want %q", hits[0].SegmentId, "seg-1")
	}
	if hits[1].IsEventClip != true {
		t.Errorf("hit[1].IsEventClip = false, want true")
	}
	if len(hits[1].MatchingEventIds) != 1 || hits[1].MatchingEventIds[0] != "ev-1" {
		t.Errorf("hit[1].MatchingEventIds = %v, want [ev-1]", hits[1].MatchingEventIds)
	}
}

func TestSearchRecordings_WithoutGrants_PermissionDenied(t *testing.T) {
	enforcer := newTestEnforcer(t) // no policies

	h := newTestHandler(t, enforcer, &stubRecordingIndex{})
	client, cleanup := newTestClient(t, h)
	defer cleanup()

	stream, err := client.SearchRecordings(context.Background(), connect.NewRequest(&kaivuev1.SearchRecordingsRequest{
		Tenant: testTenant,
	}))
	if err != nil {
		t.Fatalf("SearchRecordings: %v", err)
	}

	// Drain the stream -- should get an error.
	for stream.Receive() {
	}
	err = stream.Err()
	if err == nil {
		t.Fatal("expected permission denied error, got nil")
	}
	if !strings.Contains(err.Error(), "permission_denied") && !strings.Contains(err.Error(), "lacks playback") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSearchRecordings_CameraFilterDenied(t *testing.T) {
	enforcer := newTestEnforcer(t,
		// Grant recordings access but deny specific camera.
		grantPolicy(federationSub(), tenantID+"/recordings/*", permissions.ActionViewPlayback),
		// No grant for cameras/cam-1.
	)

	h := newTestHandler(t, enforcer, &stubRecordingIndex{})
	client, cleanup := newTestClient(t, h)
	defer cleanup()

	stream, err := client.SearchRecordings(context.Background(), connect.NewRequest(&kaivuev1.SearchRecordingsRequest{
		Tenant:    testTenant,
		CameraIds: []string{camID},
	}))
	if err != nil {
		t.Fatalf("SearchRecordings: %v", err)
	}

	for stream.Receive() {
	}
	err = stream.Err()
	if err == nil {
		t.Fatal("expected permission denied for camera filter, got nil")
	}
	if !strings.Contains(err.Error(), "lacks access to camera") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSearchRecordings_CameraFilterGranted(t *testing.T) {
	enforcer := newTestEnforcer(t,
		grantPolicy(federationSub(), tenantID+"/recordings/*", permissions.ActionViewPlayback),
		grantPolicy(federationSub(), tenantID+"/cameras/"+camID, permissions.ActionViewPlayback),
	)

	entries := []RecordingEntry{
		{
			TenantID:   tenantID,
			RecorderID: recID,
			CameraID:   camID,
			SegmentID:  "seg-filtered",
			StartTime:  fixedNow.Add(-10 * time.Minute),
			EndTime:    fixedNow,
			Bytes:      500,
		},
	}

	h := newTestHandler(t, enforcer, &stubRecordingIndex{entries: entries})
	client, cleanup := newTestClient(t, h)
	defer cleanup()

	stream, err := client.SearchRecordings(context.Background(), connect.NewRequest(&kaivuev1.SearchRecordingsRequest{
		Tenant:    testTenant,
		CameraIds: []string{camID},
	}))
	if err != nil {
		t.Fatalf("SearchRecordings: %v", err)
	}

	var hits []*kaivuev1.RecordingHit
	for stream.Receive() {
		hits = append(hits, stream.Msg().Hit)
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].SegmentId != "seg-filtered" {
		t.Errorf("hit[0].SegmentId = %q, want %q", hits[0].SegmentId, "seg-filtered")
	}
}

func TestSearchRecordings_TimeRangePassedThrough(t *testing.T) {
	enforcer := newTestEnforcer(t,
		grantPolicy(federationSub(), tenantID+"/recordings/*", permissions.ActionViewPlayback),
	)

	// Use a capturing index to verify time range is forwarded.
	capIdx := &capturingIndex{}
	h := newTestHandler(t, enforcer, capIdx)
	client, cleanup := newTestClient(t, h)
	defer cleanup()

	start := fixedNow.Add(-2 * time.Hour)
	end := fixedNow.Add(-1 * time.Hour)

	stream, err := client.SearchRecordings(context.Background(), connect.NewRequest(&kaivuev1.SearchRecordingsRequest{
		Tenant:    testTenant,
		StartTime: timestamppb.New(start),
		EndTime:   timestamppb.New(end),
	}))
	if err != nil {
		t.Fatalf("SearchRecordings: %v", err)
	}

	for stream.Receive() {
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}

	// Verify the index received the correct time range. Allow 1s of timestamp
	// rounding from proto conversion.
	if capIdx.lastStart.Sub(start).Abs() > time.Second {
		t.Errorf("index startTime = %v, want ~%v", capIdx.lastStart, start)
	}
	if capIdx.lastEnd.Sub(end).Abs() > time.Second {
		t.Errorf("index endTime = %v, want ~%v", capIdx.lastEnd, end)
	}
}

func TestSearchRecordings_MissingTenant(t *testing.T) {
	enforcer := newTestEnforcer(t)
	h := newTestHandler(t, enforcer, &stubRecordingIndex{})
	client, cleanup := newTestClient(t, h)
	defer cleanup()

	stream, err := client.SearchRecordings(context.Background(), connect.NewRequest(&kaivuev1.SearchRecordingsRequest{}))
	if err != nil {
		t.Fatalf("SearchRecordings: %v", err)
	}

	for stream.Receive() {
	}
	err = stream.Err()
	if err == nil {
		t.Fatal("expected invalid argument error, got nil")
	}
	if !strings.Contains(err.Error(), "tenant is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSearchRecordings_Unauthenticated(t *testing.T) {
	enforcer := newTestEnforcer(t)
	h, err := NewHandler(HandlerConfig{
		Enforcer: enforcer,
		Index:    &stubRecordingIndex{},
		Cameras: &stubCameraRegistry{
			cameras: map[string]string{},
			urls:    map[string]string{},
			recIDs:  map[string]string{},
		},
		Signer:  &stubSigner{token: "x"},
		PeerID:  &stubPeerExtractor{err: fmt.Errorf("no peer identity")},
		BaseURL: "https://dir.example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	client, cleanup := newTestClient(t, h)
	defer cleanup()

	stream, err := client.SearchRecordings(context.Background(), connect.NewRequest(&kaivuev1.SearchRecordingsRequest{
		Tenant: testTenant,
	}))
	if err != nil {
		t.Fatalf("SearchRecordings: %v", err)
	}

	for stream.Receive() {
	}
	err = stream.Err()
	if err == nil {
		t.Fatal("expected unauthenticated error, got nil")
	}
	if !strings.Contains(err.Error(), "no peer identity") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSearchRecordings_EmptyResults(t *testing.T) {
	enforcer := newTestEnforcer(t,
		grantPolicy(federationSub(), tenantID+"/recordings/*", permissions.ActionViewPlayback),
	)

	h := newTestHandler(t, enforcer, &stubRecordingIndex{entries: nil})
	client, cleanup := newTestClient(t, h)
	defer cleanup()

	stream, err := client.SearchRecordings(context.Background(), connect.NewRequest(&kaivuev1.SearchRecordingsRequest{
		Tenant: testTenant,
	}))
	if err != nil {
		t.Fatalf("SearchRecordings: %v", err)
	}

	var count int
	for stream.Receive() {
		count++
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 results, got %d", count)
	}
}

// --- MintStreamURL tests -----------------------------------------------------

func TestMintStreamURL_WithGrants_ReturnsSignedURL(t *testing.T) {
	enforcer := newTestEnforcer(t,
		grantPolicy(federationSub(), tenantID+"/cameras/"+camID, permissions.ActionViewLive),
	)

	h := newTestHandler(t, enforcer, &stubRecordingIndex{})
	client, cleanup := newTestClient(t, h)
	defer cleanup()

	resp, err := client.MintStreamURL(context.Background(), connect.NewRequest(&kaivuev1.MintStreamURLRequest{
		CameraId:          camID,
		RequestedKind:     uint32(kaivuev1.StreamKindBit_STREAM_KIND_BIT_LIVE),
		PreferredProtocol: kaivuev1.StreamProtocol_STREAM_PROTOCOL_WEBRTC,
	}))
	if err != nil {
		t.Fatalf("MintStreamURL: %v", err)
	}

	msg := resp.Msg
	if msg.Url == "" {
		t.Fatal("expected non-empty URL")
	}
	if !strings.Contains(msg.Url, "signed-test-token") {
		t.Errorf("URL does not contain signed token: %s", msg.Url)
	}
	if !strings.HasPrefix(msg.Url, recURL+"/webrtc/") {
		t.Errorf("URL does not start with recorder WebRTC base: %s", msg.Url)
	}
	if msg.GrantedKind != uint32(kaivuev1.StreamKindBit_STREAM_KIND_BIT_LIVE) {
		t.Errorf("GrantedKind = %d, want %d", msg.GrantedKind, kaivuev1.StreamKindBit_STREAM_KIND_BIT_LIVE)
	}
	if msg.Claims == nil {
		t.Fatal("expected non-nil Claims")
	}
	if msg.Claims.CameraId != camID {
		t.Errorf("Claims.CameraId = %q, want %q", msg.Claims.CameraId, camID)
	}
	if msg.Claims.RecorderId != recID {
		t.Errorf("Claims.RecorderId = %q, want %q", msg.Claims.RecorderId, recID)
	}
	if msg.Claims.TenantRef == nil || msg.Claims.TenantRef.Id != tenantID {
		t.Errorf("Claims.TenantRef.Id = %q, want %q", msg.Claims.TenantRef.GetId(), tenantID)
	}
}

func TestMintStreamURL_WithoutGrants_PermissionDenied(t *testing.T) {
	enforcer := newTestEnforcer(t) // no policies

	h := newTestHandler(t, enforcer, &stubRecordingIndex{})
	client, cleanup := newTestClient(t, h)
	defer cleanup()

	_, err := client.MintStreamURL(context.Background(), connect.NewRequest(&kaivuev1.MintStreamURLRequest{
		CameraId:      camID,
		RequestedKind: uint32(kaivuev1.StreamKindBit_STREAM_KIND_BIT_LIVE),
	}))
	if err == nil {
		t.Fatal("expected permission denied error, got nil")
	}
	if !strings.Contains(err.Error(), "permission_denied") && !strings.Contains(err.Error(), "lacks") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMintStreamURL_UnknownCamera_NotFound(t *testing.T) {
	enforcer := newTestEnforcer(t,
		grantPolicy(federationSub(), tenantID+"/cameras/*", permissions.ActionViewLive),
	)

	h, err := NewHandler(HandlerConfig{
		Enforcer: enforcer,
		Index:    &stubRecordingIndex{},
		Cameras: &stubCameraRegistry{
			cameras: map[string]string{}, // empty: no cameras known
			urls:    map[string]string{},
			recIDs:  map[string]string{},
		},
		Signer:  &stubSigner{token: "x"},
		PeerID:  &stubPeerExtractor{id: peerID},
		BaseURL: "https://dir.example.com",
		NowFunc: func() time.Time { return fixedNow },
	})
	if err != nil {
		t.Fatal(err)
	}

	client, cleanup := newTestClient(t, h)
	defer cleanup()

	_, err = client.MintStreamURL(context.Background(), connect.NewRequest(&kaivuev1.MintStreamURLRequest{
		CameraId:      "nonexistent-cam",
		RequestedKind: uint32(kaivuev1.StreamKindBit_STREAM_KIND_BIT_LIVE),
	}))
	if err == nil {
		t.Fatal("expected not found error, got nil")
	}
	if !strings.Contains(err.Error(), "not_found") && !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMintStreamURL_MissingCameraID(t *testing.T) {
	enforcer := newTestEnforcer(t)
	h := newTestHandler(t, enforcer, &stubRecordingIndex{})
	client, cleanup := newTestClient(t, h)
	defer cleanup()

	_, err := client.MintStreamURL(context.Background(), connect.NewRequest(&kaivuev1.MintStreamURLRequest{}))
	if err == nil {
		t.Fatal("expected invalid argument error, got nil")
	}
	if !strings.Contains(err.Error(), "camera_id is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMintStreamURL_PlaybackKind_RequiresPlaybackGrant(t *testing.T) {
	// Grant only live, not playback.
	enforcer := newTestEnforcer(t,
		grantPolicy(federationSub(), tenantID+"/cameras/"+camID, permissions.ActionViewLive),
	)

	h := newTestHandler(t, enforcer, &stubRecordingIndex{})
	client, cleanup := newTestClient(t, h)
	defer cleanup()

	_, err := client.MintStreamURL(context.Background(), connect.NewRequest(&kaivuev1.MintStreamURLRequest{
		CameraId:      camID,
		RequestedKind: uint32(kaivuev1.StreamKindBit_STREAM_KIND_BIT_PLAYBACK),
	}))
	if err == nil {
		t.Fatal("expected permission denied for playback, got nil")
	}
	if !strings.Contains(err.Error(), "lacks") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMintStreamURL_PlaybackKind_Granted(t *testing.T) {
	enforcer := newTestEnforcer(t,
		grantPolicy(federationSub(), tenantID+"/cameras/"+camID, permissions.ActionViewPlayback),
	)

	h := newTestHandler(t, enforcer, &stubRecordingIndex{})
	client, cleanup := newTestClient(t, h)
	defer cleanup()

	start := fixedNow.Add(-1 * time.Hour)
	end := fixedNow

	resp, err := client.MintStreamURL(context.Background(), connect.NewRequest(&kaivuev1.MintStreamURLRequest{
		CameraId:          camID,
		RequestedKind:     uint32(kaivuev1.StreamKindBit_STREAM_KIND_BIT_PLAYBACK),
		PreferredProtocol: kaivuev1.StreamProtocol_STREAM_PROTOCOL_MP4,
		PlaybackRange: &kaivuev1.PlaybackRange{
			Start: timestamppb.New(start),
			End:   timestamppb.New(end),
		},
	}))
	if err != nil {
		t.Fatalf("MintStreamURL: %v", err)
	}

	if !strings.Contains(resp.Msg.Url, "/playback/") {
		t.Errorf("expected MP4 playback URL, got: %s", resp.Msg.Url)
	}
	if resp.Msg.Claims.PlaybackRange == nil {
		t.Fatal("expected PlaybackRange in claims")
	}
}

func TestMintStreamURL_TTLCappedByServer(t *testing.T) {
	enforcer := newTestEnforcer(t,
		grantPolicy(federationSub(), tenantID+"/cameras/"+camID, permissions.ActionViewLive),
	)

	h := newTestHandler(t, enforcer, &stubRecordingIndex{})
	client, cleanup := newTestClient(t, h)
	defer cleanup()

	resp, err := client.MintStreamURL(context.Background(), connect.NewRequest(&kaivuev1.MintStreamURLRequest{
		CameraId:       camID,
		RequestedKind:  uint32(kaivuev1.StreamKindBit_STREAM_KIND_BIT_LIVE),
		MaxTtlSeconds:  600, // 10 min -- server max is 5 min
	}))
	if err != nil {
		t.Fatalf("MintStreamURL: %v", err)
	}

	// Server should cap at its own maxTTL (5 min).
	expiresAt := resp.Msg.Claims.ExpiresAt.AsTime()
	expectedExpiry := fixedNow.Add(5 * time.Minute)
	if expiresAt.Sub(expectedExpiry).Abs() > time.Second {
		t.Errorf("ExpiresAt = %v, want ~%v (capped by server)", expiresAt, expectedExpiry)
	}
}

func TestMintStreamURL_TTLHonoursClientShorter(t *testing.T) {
	enforcer := newTestEnforcer(t,
		grantPolicy(federationSub(), tenantID+"/cameras/"+camID, permissions.ActionViewLive),
	)

	h := newTestHandler(t, enforcer, &stubRecordingIndex{})
	client, cleanup := newTestClient(t, h)
	defer cleanup()

	resp, err := client.MintStreamURL(context.Background(), connect.NewRequest(&kaivuev1.MintStreamURLRequest{
		CameraId:       camID,
		RequestedKind:  uint32(kaivuev1.StreamKindBit_STREAM_KIND_BIT_LIVE),
		MaxTtlSeconds:  60, // 1 min -- shorter than server max
	}))
	if err != nil {
		t.Fatalf("MintStreamURL: %v", err)
	}

	expiresAt := resp.Msg.Claims.ExpiresAt.AsTime()
	expectedExpiry := fixedNow.Add(1 * time.Minute)
	if expiresAt.Sub(expectedExpiry).Abs() > time.Second {
		t.Errorf("ExpiresAt = %v, want ~%v (client's shorter TTL)", expiresAt, expectedExpiry)
	}
}

func TestMintStreamURL_ProtocolURLFormats(t *testing.T) {
	enforcer := newTestEnforcer(t,
		grantPolicy(federationSub(), tenantID+"/cameras/"+camID, "*"),
	)

	tests := []struct {
		protocol kaivuev1.StreamProtocol
		contains string
	}{
		{kaivuev1.StreamProtocol_STREAM_PROTOCOL_WEBRTC, "/webrtc/"},
		{kaivuev1.StreamProtocol_STREAM_PROTOCOL_HLS, "/hls/"},
		{kaivuev1.StreamProtocol_STREAM_PROTOCOL_RTSP, "rtsp://"},
		{kaivuev1.StreamProtocol_STREAM_PROTOCOL_MP4, "/playback/"},
		{kaivuev1.StreamProtocol_STREAM_PROTOCOL_JPEG, "/snapshot/"},
	}

	for _, tc := range tests {
		t.Run(tc.protocol.String(), func(t *testing.T) {
			h := newTestHandler(t, enforcer, &stubRecordingIndex{})
			client, cleanup := newTestClient(t, h)
			defer cleanup()

			resp, err := client.MintStreamURL(context.Background(), connect.NewRequest(&kaivuev1.MintStreamURLRequest{
				CameraId:          camID,
				RequestedKind:     uint32(kaivuev1.StreamKindBit_STREAM_KIND_BIT_LIVE),
				PreferredProtocol: tc.protocol,
			}))
			if err != nil {
				t.Fatalf("MintStreamURL: %v", err)
			}
			if !strings.Contains(resp.Msg.Url, tc.contains) {
				t.Errorf("URL %q does not contain %q", resp.Msg.Url, tc.contains)
			}
		})
	}
}

func TestMintStreamURL_Unauthenticated(t *testing.T) {
	enforcer := newTestEnforcer(t)
	h, err := NewHandler(HandlerConfig{
		Enforcer: enforcer,
		Index:    &stubRecordingIndex{},
		Cameras: &stubCameraRegistry{
			cameras: map[string]string{},
			urls:    map[string]string{},
			recIDs:  map[string]string{},
		},
		Signer:  &stubSigner{token: "x"},
		PeerID:  &stubPeerExtractor{err: fmt.Errorf("unauthenticated")},
		BaseURL: "https://dir.example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	client, cleanup := newTestClient(t, h)
	defer cleanup()

	_, err = client.MintStreamURL(context.Background(), connect.NewRequest(&kaivuev1.MintStreamURLRequest{
		CameraId: camID,
	}))
	if err == nil {
		t.Fatal("expected unauthenticated error, got nil")
	}
	if !strings.Contains(err.Error(), "unauthenticated") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- helpers for capture tests -----------------------------------------------

type capturingIndex struct {
	lastStart time.Time
	lastEnd   time.Time
}

func (c *capturingIndex) Search(
	_ context.Context, _ string, _ []string,
	start, end time.Time, _ []kaivuev1.AIEventKind, _ string, _ int,
) ([]RecordingEntry, error) {
	c.lastStart = start
	c.lastEnd = end
	return nil, nil
}

// --- NewHandler validation tests ---------------------------------------------

func TestNewHandler_MissingDeps(t *testing.T) {
	enforcer := newTestEnforcer(t)
	base := HandlerConfig{
		Enforcer: enforcer,
		Index:    &stubRecordingIndex{},
		Cameras:  &stubCameraRegistry{cameras: map[string]string{}, urls: map[string]string{}, recIDs: map[string]string{}},
		Signer:   &stubSigner{token: "x"},
		PeerID:   &stubPeerExtractor{id: "p"},
	}

	tests := []struct {
		name string
		mut  func(*HandlerConfig)
		want string
	}{
		{"no enforcer", func(c *HandlerConfig) { c.Enforcer = nil }, "enforcer is required"},
		{"no index", func(c *HandlerConfig) { c.Index = nil }, "recording index is required"},
		{"no cameras", func(c *HandlerConfig) { c.Cameras = nil }, "camera registry is required"},
		{"no signer", func(c *HandlerConfig) { c.Signer = nil }, "stream signer is required"},
		{"no peerID", func(c *HandlerConfig) { c.PeerID = nil }, "peer identity extractor is required"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.mut(&cfg)
			_, err := NewHandler(cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("expected error containing %q, got: %v", tc.want, err)
			}
		})
	}
}

// --- unit tests for helpers --------------------------------------------------

func TestActionForKind(t *testing.T) {
	tests := []struct {
		kind uint32
		want string
	}{
		{uint32(kaivuev1.StreamKindBit_STREAM_KIND_BIT_LIVE), permissions.ActionViewLive},
		{uint32(kaivuev1.StreamKindBit_STREAM_KIND_BIT_PLAYBACK), permissions.ActionViewPlayback},
		{uint32(kaivuev1.StreamKindBit_STREAM_KIND_BIT_LIVE) | uint32(kaivuev1.StreamKindBit_STREAM_KIND_BIT_PLAYBACK), permissions.ActionViewPlayback},
		{0, permissions.ActionViewLive},
	}

	for _, tc := range tests {
		got := actionForKind(tc.kind)
		if got != tc.want {
			t.Errorf("actionForKind(%d) = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

func TestBuildStreamURL(t *testing.T) {
	tests := []struct {
		protocol kaivuev1.StreamProtocol
		contains string
	}{
		{kaivuev1.StreamProtocol_STREAM_PROTOCOL_WEBRTC, "/webrtc/cam-1?token=tok"},
		{kaivuev1.StreamProtocol_STREAM_PROTOCOL_HLS, "/hls/cam-1/index.m3u8?token=tok"},
		{kaivuev1.StreamProtocol_STREAM_PROTOCOL_RTSP, "rtsp://recorder.example.com/cam-1?token=tok"},
		{kaivuev1.StreamProtocol_STREAM_PROTOCOL_MP4, "/playback/cam-1.mp4?token=tok"},
		{kaivuev1.StreamProtocol_STREAM_PROTOCOL_JPEG, "/snapshot/cam-1.jpg?token=tok"},
		{kaivuev1.StreamProtocol_STREAM_PROTOCOL_UNSPECIFIED, "/stream/cam-1?token=tok"},
	}

	for _, tc := range tests {
		url := buildStreamURL("https://recorder.example.com", "cam-1", tc.protocol, "tok")
		if !strings.Contains(url, tc.contains) {
			t.Errorf("buildStreamURL(%v) = %q, want to contain %q", tc.protocol, url, tc.contains)
		}
	}
}
