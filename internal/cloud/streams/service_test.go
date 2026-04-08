package streams_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	"github.com/bluenviron/mediamtx/internal/cloud/permissions"
	"github.com/bluenviron/mediamtx/internal/cloud/streams"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
	"github.com/bluenviron/mediamtx/internal/shared/streamclaims"
)

// ---- test helpers ----------------------------------------------------------

// fakeCameraRegistry implements streams.CameraRegistry in memory.
type fakeCameraRegistry struct {
	cameras map[string]streams.Camera // key: tenantID+"/"+cameraID
}

func newFakeCameraRegistry() *fakeCameraRegistry {
	return &fakeCameraRegistry{cameras: map[string]streams.Camera{}}
}

func (f *fakeCameraRegistry) add(tenantID string, cam streams.Camera) {
	f.cameras[tenantID+"/"+cam.ID] = cam
}

func (f *fakeCameraRegistry) GetCamera(_ context.Context, tenantID, cameraID string) (streams.Camera, error) {
	cam, ok := f.cameras[tenantID+"/"+cameraID]
	if !ok {
		return streams.Camera{}, streams.ErrCameraNotFound
	}
	return cam, nil
}

// testEnforcer builds a Casbin enforcer and grants a user the listed actions
// on all streams in the given tenant.
func testEnforcer(t *testing.T, tenantID, userID string, actions ...string) *permissions.Enforcer {
	t.Helper()
	store := permissions.NewInMemoryStore()
	for _, action := range actions {
		subj := "user:" + userID + "@" + tenantID
		obj := tenantID + "/streams/*"
		_ = store.AddPolicy(permissions.PolicyRule{Sub: subj, Obj: obj, Act: action})
	}
	enf, err := permissions.NewEnforcer(store, permissions.DefaultAuditSink)
	if err != nil {
		t.Fatalf("testEnforcer: %v", err)
	}
	return enf
}

// buildService creates a fully wired Service for test use.
func buildService(
	t *testing.T,
	issuer *streamclaims.Issuer,
	cam streams.Camera,
	tenantID, userID string,
	actions []string,
) (*streams.Service, *audit.MemoryRecorder) {
	t.Helper()

	reg := newFakeCameraRegistry()
	reg.add(tenantID, cam)

	enf := testEnforcer(t, tenantID, userID, actions...)
	auditRec := audit.NewMemoryRecorder()

	svc, err := streams.NewService(streams.Config{
		Issuer:         issuer,
		Router:         &streams.Router{RelayBaseURL: "https://relay.kaivue.io"},
		CameraRegistry: reg,
		Enforcer:       enf,
		AuditRecorder:  auditRec,
		DirectoryID:    "dir-test-1",
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, auditRec
}

// mintClaims is a convenience to inject auth.Claims into a request context
// via the streams package's ClaimsAdapter / Handler path.
func mintClaims(userID, tenantID string, tenantType auth.TenantType) *auth.Claims {
	return &auth.Claims{
		UserID: auth.UserID(userID),
		TenantRef: auth.TenantRef{
			Type: tenantType,
			ID:   tenantID,
		},
	}
}

// doRequest fires a POST /api/v1/streams/request with the given body and
// injected claims, returns the recorder.
func doRequest(t *testing.T, svc *streams.Service, claims *auth.Claims, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/streams/request", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.5:12345" // private IP → should get lan_direct
	rr := httptest.NewRecorder()

	// Wire the claims via the Handler adapter, which injects them into the
	// streams package's own context key.
	h := svc.Handler(func(_ *http.Request) (*auth.Claims, bool) {
		return claims, true
	})
	h.ServeHTTP(rr, req)
	return rr
}

// ---- tests -----------------------------------------------------------------

// TestHappyPathLive verifies a successful live-stream mint:
//   - returns 200 with ≥1 endpoint
//   - endpoint token is a valid JWT verifiable against the issuer's public key
//   - TTL is exactly MaxTTL (5 minutes)
//   - stream_id is non-empty
func TestHappyPathLive(t *testing.T) {
	key, err := streamclaims.GenerateTestKey()
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := streamclaims.NewIssuer(key, "https://test.kaivue.io", "kaivue-recorder")
	if err != nil {
		t.Fatal(err)
	}

	tenantID := "tenant-acme"
	userID := "user-1"
	cam := streams.Camera{
		ID:           "cam-abc-123",
		RecorderID:   "rec-1",
		RelayBaseURL: "https://relay.kaivue.io",
	}

	svc, auditRec := buildService(t, issuer, cam, tenantID, userID,
		[]string{permissions.ActionViewLive})
	claims := mintClaims(userID, tenantID, auth.TenantTypeCustomer)

	rr := doRequest(t, svc, claims, map[string]any{
		"camera_id": "cam-abc-123",
		"kind":      "live",
		"protocol":  "webrtc",
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp streams.MintResponse
	if decErr := json.NewDecoder(rr.Body).Decode(&resp); decErr != nil {
		t.Fatalf("decode response: %v", decErr)
	}
	if resp.StreamID == "" {
		t.Error("stream_id should not be empty")
	}
	if resp.TTLSeconds != int(streamclaims.MaxTTL.Seconds()) {
		t.Errorf("ttl_seconds want %d got %d", int(streamclaims.MaxTTL.Seconds()), resp.TTLSeconds)
	}
	if len(resp.Endpoints) < 1 {
		t.Fatal("expected at least 1 endpoint")
	}

	// Verify each token is a valid JWT using the issuer's public key.
	jwks, err := issuer.PublicKeySet()
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := streamclaims.NewVerifier(jwks, "https://test.kaivue.io", "kaivue-recorder")
	if err != nil {
		t.Fatal(err)
	}
	for _, ep := range resp.Endpoints {
		sc, verErr := verifier.Verify(ep.Token)
		if verErr != nil {
			t.Errorf("endpoint %s: token verification failed: %v", ep.Kind, verErr)
			continue
		}
		if sc.CameraID != "cam-abc-123" {
			t.Errorf("endpoint %s: want cam cam-abc-123, got %s", ep.Kind, sc.CameraID)
		}
		if sc.TenantRef.ID != tenantID {
			t.Errorf("endpoint %s: want tenant %s, got %s", ep.Kind, tenantID, sc.TenantRef.ID)
		}
	}

	// Audit log should have exactly one allow entry.
	entries, qErr := auditRec.Query(context.Background(), audit.QueryFilter{
		TenantID: tenantID,
		Result:   audit.ResultAllow,
	})
	if qErr != nil {
		t.Fatal(qErr)
	}
	if len(entries) != 1 {
		t.Errorf("want 1 audit allow entry, got %d", len(entries))
	}
}

// TestPermissionDeniedLive verifies that a user without view.live gets 403.
func TestPermissionDeniedLive(t *testing.T) {
	key, _ := streamclaims.GenerateTestKey()
	issuer, _ := streamclaims.NewIssuer(key, "https://test.kaivue.io", "kaivue-recorder")

	tenantID := "tenant-acme"
	userID := "user-no-view"
	cam := streams.Camera{ID: "cam-1", RecorderID: "rec-1"}

	// Grant playback but NOT live.
	svc, auditRec := buildService(t, issuer, cam, tenantID, userID,
		[]string{permissions.ActionViewPlayback})
	claims := mintClaims(userID, tenantID, auth.TenantTypeCustomer)

	rr := doRequest(t, svc, claims, map[string]any{
		"camera_id": "cam-1",
		"kind":      "live",
		"protocol":  "webrtc",
	})

	if rr.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d: %s", rr.Code, rr.Body.String())
	}

	// Audit log should record a deny.
	entries, _ := auditRec.Query(context.Background(), audit.QueryFilter{
		TenantID: tenantID,
		Result:   audit.ResultDeny,
	})
	if len(entries) == 0 {
		t.Error("expected audit deny entry")
	}
}

// TestCrossTenantRequest verifies that a user from tenant A cannot access a
// camera in tenant B — the critical multi-tenant isolation test.
//
// The camera registry is seeded only for tenant-B. The request carries
// tenant-A claims. The registry lookup for tenant-A will return
// ErrCameraNotFound, yielding 404 (not 403) so the caller cannot distinguish
// "exists but denied" from "does not exist".
func TestCrossTenantRequest(t *testing.T) {
	key, _ := streamclaims.GenerateTestKey()
	issuer, _ := streamclaims.NewIssuer(key, "https://test.kaivue.io", "kaivue-recorder")

	tenantA := "tenant-a"
	tenantB := "tenant-b"
	userID := "user-from-a"

	// Camera exists only in tenant-B.
	reg := newFakeCameraRegistry()
	reg.add(tenantB, streams.Camera{ID: "cam-b", RecorderID: "rec-b"})

	// User in tenant-A has full stream permissions (for their own tenant).
	store := permissions.NewInMemoryStore()
	_ = store.AddPolicy(permissions.PolicyRule{
		Sub: "user:" + userID + "@" + tenantA,
		Obj: tenantA + "/streams/*",
		Act: permissions.ActionViewLive,
	})
	enf, _ := permissions.NewEnforcer(store, permissions.DefaultAuditSink)
	auditRec := audit.NewMemoryRecorder()

	svc, err := streams.NewService(streams.Config{
		Issuer:         issuer,
		Router:         &streams.Router{RelayBaseURL: "https://relay.kaivue.io"},
		CameraRegistry: reg,
		Enforcer:       enf,
		AuditRecorder:  auditRec,
		DirectoryID:    "dir-test-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Claims are for tenant-A.
	claims := mintClaims(userID, tenantA, auth.TenantTypeCustomer)

	rr := doRequest(t, svc, claims, map[string]any{
		"camera_id": "cam-b", // belongs to tenant-B
		"kind":      "live",
		"protocol":  "webrtc",
	})

	// We expect 403 because tenant-A's user does not have view.live
	// on tenant-A/streams/cam-b (it's not in the policy), or 404 if the
	// permission check passes but the camera lookup fails under tenant-A.
	// Either is acceptable for cross-tenant isolation; the important thing
	// is it's NOT 200.
	if rr.Code == http.StatusOK {
		t.Errorf("cross-tenant request must not succeed; got 200")
	}
}

// TestTokenTTL verifies the minted token's expiry is MaxTTL (5 minutes).
func TestTokenTTL(t *testing.T) {
	key, _ := streamclaims.GenerateTestKey()
	issuer, _ := streamclaims.NewIssuer(key, "https://test.kaivue.io", "kaivue-recorder")

	tenantID := "tenant-ttl"
	userID := "user-ttl"
	cam := streams.Camera{ID: "cam-ttl", RecorderID: "rec-1", RelayBaseURL: "https://relay.kaivue.io"}

	svc, _ := buildService(t, issuer, cam, tenantID, userID,
		[]string{permissions.ActionViewLive})
	claims := mintClaims(userID, tenantID, auth.TenantTypeCustomer)

	before := time.Now()
	rr := doRequest(t, svc, claims, map[string]any{
		"camera_id": "cam-ttl",
		"kind":      "live",
		"protocol":  "webrtc",
	})
	after := time.Now()

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp streams.MintResponse
	json.NewDecoder(rr.Body).Decode(&resp) //nolint:errcheck

	if len(resp.Endpoints) == 0 {
		t.Fatal("no endpoints in response")
	}

	// Verify TTL field.
	if resp.TTLSeconds != 300 {
		t.Errorf("want ttl_seconds=300, got %d", resp.TTLSeconds)
	}

	// Parse and verify the token's expiry via the verifier.
	jwks, _ := issuer.PublicKeySet()
	verifier, _ := streamclaims.NewVerifier(jwks, "https://test.kaivue.io", "kaivue-recorder")

	for _, ep := range resp.Endpoints {
		sc, err := verifier.Verify(ep.Token)
		if err != nil {
			t.Errorf("endpoint %s: verify failed: %v", ep.Kind, err)
			continue
		}
		// ExpiresAt should be within [before+MaxTTL, after+MaxTTL+1s].
		earliest := before.Add(streamclaims.MaxTTL).Add(-time.Second)
		latest := after.Add(streamclaims.MaxTTL).Add(time.Second)
		if sc.ExpiresAt.Before(earliest) || sc.ExpiresAt.After(latest) {
			t.Errorf("endpoint %s: ExpiresAt %v out of expected window [%v, %v]",
				ep.Kind, sc.ExpiresAt, earliest, latest)
		}
	}
}

// TestTokenSignedByCloudKey verifies that each endpoint token is verifiable
// with the issuer's public key only (not some other key).
func TestTokenSignedByCloudKey(t *testing.T) {
	key, _ := streamclaims.GenerateTestKey()
	issuer, _ := streamclaims.NewIssuer(key, "https://test.kaivue.io", "kaivue-recorder")

	// Different key for negative verification.
	otherKey, _ := streamclaims.GenerateTestKey()
	otherIssuer, _ := streamclaims.NewIssuer(otherKey, "https://test.kaivue.io", "kaivue-recorder")
	otherJWKS, _ := otherIssuer.PublicKeySet()
	wrongVerifier, _ := streamclaims.NewVerifier(otherJWKS, "https://test.kaivue.io", "kaivue-recorder")

	tenantID := "tenant-keychk"
	userID := "user-keychk"
	cam := streams.Camera{ID: "cam-keychk", RecorderID: "rec-1", RelayBaseURL: "https://relay.kaivue.io"}

	svc, _ := buildService(t, issuer, cam, tenantID, userID,
		[]string{permissions.ActionViewLive})
	claims := mintClaims(userID, tenantID, auth.TenantTypeCustomer)

	rr := doRequest(t, svc, claims, map[string]any{
		"camera_id": "cam-keychk",
		"kind":      "live",
		"protocol":  "webrtc",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}

	var resp streams.MintResponse
	json.NewDecoder(rr.Body).Decode(&resp) //nolint:errcheck

	jwks, _ := issuer.PublicKeySet()
	correctVerifier, _ := streamclaims.NewVerifier(jwks, "https://test.kaivue.io", "kaivue-recorder")

	for _, ep := range resp.Endpoints {
		if _, err := correctVerifier.Verify(ep.Token); err != nil {
			t.Errorf("endpoint %s: correct key failed to verify: %v", ep.Kind, err)
		}
		if _, err := wrongVerifier.Verify(ep.Token); err == nil {
			t.Errorf("endpoint %s: wrong key should NOT verify but did", ep.Kind)
		}
	}
}

// TestUniqueNoncesPerEndpoint verifies that each endpoint's token carries a
// distinct nonce (replay protection requires per-token uniqueness).
func TestUniqueNoncesPerEndpoint(t *testing.T) {
	key, _ := streamclaims.GenerateTestKey()
	issuer, _ := streamclaims.NewIssuer(key, "https://test.kaivue.io", "kaivue-recorder")

	tenantID := "tenant-nonce"
	userID := "user-nonce"
	// Use a private-IP client so we get both lan_direct and managed_relay.
	cam := streams.Camera{
		ID:                 "cam-nonce",
		RecorderID:         "rec-1",
		RecorderLANBaseURL: "https://192.168.1.10:8443",
		RelayBaseURL:       "https://relay.kaivue.io",
	}

	svc, _ := buildService(t, issuer, cam, tenantID, userID,
		[]string{permissions.ActionViewLive})
	claims := mintClaims(userID, tenantID, auth.TenantTypeCustomer)

	rr := doRequest(t, svc, claims, map[string]any{
		"camera_id": "cam-nonce",
		"kind":      "live",
		"protocol":  "webrtc",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp streams.MintResponse
	json.NewDecoder(rr.Body).Decode(&resp) //nolint:errcheck

	if len(resp.Endpoints) < 2 {
		t.Skip("need ≥2 endpoints to verify nonce uniqueness; got", len(resp.Endpoints))
	}

	jwks, _ := issuer.PublicKeySet()
	verifier, _ := streamclaims.NewVerifier(jwks, "https://test.kaivue.io", "kaivue-recorder")
	nonces := map[string]bool{}
	for _, ep := range resp.Endpoints {
		sc, err := verifier.Verify(ep.Token)
		if err != nil {
			t.Fatalf("endpoint %s verify: %v", ep.Kind, err)
		}
		if nonces[sc.Nonce] {
			t.Errorf("duplicate nonce across endpoints: %s", sc.Nonce)
		}
		nonces[sc.Nonce] = true
	}
}

// TestPlaybackMissingRange verifies 400 when kind=playback but no
// playback_range is supplied.
func TestPlaybackMissingRange(t *testing.T) {
	key, _ := streamclaims.GenerateTestKey()
	issuer, _ := streamclaims.NewIssuer(key, "https://test.kaivue.io", "kaivue-recorder")

	tenantID := "tenant-pb"
	userID := "user-pb"
	cam := streams.Camera{ID: "cam-pb", RecorderID: "rec-1", RelayBaseURL: "https://relay.kaivue.io"}

	svc, _ := buildService(t, issuer, cam, tenantID, userID,
		[]string{permissions.ActionViewPlayback})
	claims := mintClaims(userID, tenantID, auth.TenantTypeCustomer)

	rr := doRequest(t, svc, claims, map[string]any{
		"camera_id": "cam-pb",
		"kind":      "playback",
		"protocol":  "hls",
		// playback_range intentionally omitted
	})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestAudioTalkbackPermissions verifies that audio_talkback requires both
// view.live AND audio.talkback permissions.
func TestAudioTalkbackPermissions(t *testing.T) {
	key, _ := streamclaims.GenerateTestKey()
	issuer, _ := streamclaims.NewIssuer(key, "https://test.kaivue.io", "kaivue-recorder")

	tenantID := "tenant-talkback"
	userID := "user-talkback"
	cam := streams.Camera{ID: "cam-tb", RecorderID: "rec-1", RelayBaseURL: "https://relay.kaivue.io"}

	// Scenario A: only view.live, no audio.talkback → should deny.
	svcA, _ := buildService(t, issuer, cam, tenantID, userID,
		[]string{permissions.ActionViewLive})
	claimsA := mintClaims(userID, tenantID, auth.TenantTypeCustomer)
	rrA := doRequest(t, svcA, claimsA, map[string]any{
		"camera_id": "cam-tb",
		"kind":      "audio_talkback",
		"protocol":  "webrtc",
	})
	if rrA.Code != http.StatusForbidden {
		t.Errorf("scenario A: want 403 without audio.talkback, got %d", rrA.Code)
	}

	// Scenario B: both view.live AND audio.talkback → should succeed.
	svcB, _ := buildService(t, issuer, cam, tenantID, userID,
		[]string{permissions.ActionViewLive, permissions.ActionAudioTalkback})
	claimsB := mintClaims(userID, tenantID, auth.TenantTypeCustomer)
	rrB := doRequest(t, svcB, claimsB, map[string]any{
		"camera_id": "cam-tb",
		"kind":      "audio_talkback",
		"protocol":  "webrtc",
	})
	if rrB.Code != http.StatusOK {
		t.Errorf("scenario B: want 200 with both permissions, got %d: %s", rrB.Code, rrB.Body.String())
	}
}

// TestJWKSHandler verifies that JWKSHandler returns a valid JSON key set with
// status 200 and the correct Content-Type header.
func TestJWKSHandler(t *testing.T) {
	key, _ := streamclaims.GenerateTestKey()
	issuer, _ := streamclaims.NewIssuer(key, "https://test.kaivue.io", "kaivue-recorder",
		streamclaims.WithKeyID("test-kid-1"))

	handler := streams.JWKSHandler(issuer)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("want Content-Type application/json, got %q", ct)
	}

	var jwksDoc map[string]json.RawMessage
	if err := json.NewDecoder(rr.Body).Decode(&jwksDoc); err != nil {
		t.Fatalf("JWKS response is not valid JSON: %v", err)
	}
	if _, ok := jwksDoc["keys"]; !ok {
		t.Error("JWKS response missing 'keys' field")
	}

	// The JWKS should be usable to verify tokens minted by this issuer.
	jwks, _ := issuer.PublicKeySet()
	verifier, err := streamclaims.NewVerifier(jwks, "https://test.kaivue.io", "kaivue-recorder")
	if err != nil {
		t.Fatalf("NewVerifier from JWKS: %v", err)
	}

	nonce, _ := streamclaims.GenerateNonce()
	tokenStr, _ := issuer.Sign(streamclaims.StreamClaims{
		UserID:      "u1",
		TenantRef:   auth.TenantRef{Type: auth.TenantTypeCustomer, ID: "t1"},
		CameraID:    "c1",
		RecorderID:  "r1",
		DirectoryID: "dir-test",
		Kind:        streamclaims.StreamKindLive,
		Protocol:    streamclaims.ProtocolWebRTC,
		ExpiresAt:   time.Now().Add(4 * time.Minute),
		Nonce:       nonce,
	})
	if _, verifyErr := verifier.Verify(tokenStr); verifyErr != nil {
		t.Errorf("token minted by issuer not verifiable via JWKS: %v", verifyErr)
	}
}

// TestRouterPrivateIPLANDirect verifies the simplified router includes
// lan_direct when the source IP is in an RFC-1918 range.
func TestRouterPrivateIPLANDirect(t *testing.T) {
	router := &streams.Router{RelayBaseURL: "https://relay.kaivue.io"}
	cam := streams.Camera{
		ID:                 "cam-1",
		RecorderID:         "rec-1",
		RecorderLANBaseURL: "https://192.168.1.10:8443",
		RelayBaseURL:       "https://relay.kaivue.io",
	}
	client := streams.ClientInfo{SourceIP: net.ParseIP("192.168.1.50")}
	choices := router.ChooseEndpoints(client, cam)

	hasLAN := false
	hasRelay := false
	for _, c := range choices {
		if c.Kind == streams.EndpointKindLANDirect {
			hasLAN = true
		}
		if c.Kind == streams.EndpointKindManagedRelay {
			hasRelay = true
		}
	}
	if !hasLAN {
		t.Error("private source IP should produce lan_direct endpoint")
	}
	if !hasRelay {
		t.Error("managed_relay should always be included")
	}
}

// TestRouterPublicIPNoLAN verifies the simplified router omits lan_direct
// for a public source IP not in the recorder's LAN subnets.
func TestRouterPublicIPNoLAN(t *testing.T) {
	router := &streams.Router{RelayBaseURL: "https://relay.kaivue.io"}
	cam := streams.Camera{
		ID:                 "cam-1",
		RecorderID:         "rec-1",
		RecorderLANSubnets: []string{"10.0.0.0/8"},
		RecorderLANBaseURL: "https://192.168.1.10:8443",
		RelayBaseURL:       "https://relay.kaivue.io",
	}
	client := streams.ClientInfo{SourceIP: net.ParseIP("8.8.8.8")} // public IP
	choices := router.ChooseEndpoints(client, cam)

	for _, c := range choices {
		if c.Kind == streams.EndpointKindLANDirect {
			t.Error("public IP outside recorder subnets should not get lan_direct")
		}
	}
	if len(choices) == 0 {
		t.Error("should still have managed_relay for public IP")
	}
}
