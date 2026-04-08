package pairing_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/directory/pairing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestService creates a pairing.Service backed by an in-memory DB.
// It uses stub Headscale + CA so it compiles without live sidecars.
func newTestService(t *testing.T) (*pairing.Service, *pairing.RecorderStore) {
	t.Helper()
	db := newTestDB(t)
	rootKey := newTestRootKey(t)
	svc, err := pairing.NewService(pairing.Config{
		DB:                db,
		Headscale:         &stubHeadscale{key: "hskey-test"},
		ClusterCA:         &stubCA{fp: "aabbcc"},
		RootSigningKey:    rootKey,
		DirectoryEndpoint: "https://dir.test:8443",
	})
	require.NoError(t, err)
	return svc, pairing.NewRecorderStore(db)
}

// mintValidToken generates a PairingToken through the service (writes it to
// the DB) and returns the encoded string.
func mintValidToken(t *testing.T, svc *pairing.Service) string {
	t.Helper()
	result, err := svc.Generate(context.Background(), "admin-1", []string{"recorder"}, "tenant-abc")
	require.NoError(t, err)
	return result.Encoded
}

// validBody builds a CheckInRequest JSON body with all required fields filled.
func validBody(t *testing.T) []byte {
	t.Helper()
	req := pairing.CheckInRequest{
		Hardware: pairing.CheckInHardware{
			CPUModel: "Intel Xeon",
			CPUCores: 8,
			RAMBytes: 16_000_000_000,
			Disks: []pairing.CheckInDisk{
				{Device: "/dev/sda", SizeBytes: 1_000_000_000_000, Model: "Samsung 870"},
			},
			NICs: []pairing.CheckInNIC{
				{Name: "eth0", MAC: "de:ad:be:ef:00:01"},
			},
			GPU: "NVIDIA T400",
		},
		DevicePubkey: "dGVzdC1wdWJsaWMta2V5LWJhc2U2NHVybA", // base64url of "test-public-key-base64url"
		OSRelease:    "Ubuntu 24.04",
	}
	b, err := json.Marshal(req)
	require.NoError(t, err)
	return b
}

// doCheckIn issues a POST /check-in request against the handler and returns
// the recorder.
func doCheckIn(
	t *testing.T,
	handler http.HandlerFunc,
	token string,
	body []byte,
) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/pairing/check-in", bytes.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

// ---------------------------------------------------------------------------
// Happy path
// ---------------------------------------------------------------------------

func TestCheckInHappyPath(t *testing.T) {
	t.Parallel()
	svc, recStore := newTestService(t)
	handler := pairing.CheckInHandler(svc, recStore, nil)

	tok := mintValidToken(t, svc)
	w := doCheckIn(t, handler, tok, validBody(t))

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var resp pairing.CheckInResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.NotEmpty(t, resp.RecorderID)
	assert.Equal(t, "tenant-abc", resp.TenantID)
	assert.Equal(t, "https://dir.test:8443", resp.DirectoryEndpoint)
	assert.Equal(t, "enroll-with-stepca", resp.NextStepHint)
}

// ---------------------------------------------------------------------------
// Expired token
// ---------------------------------------------------------------------------

func TestCheckInExpiredToken(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	rootKey := newTestRootKey(t)
	sk, err := pairing.NewSigningKey(rootKey)
	require.NoError(t, err)

	// Build a token that is already expired.
	tok := newToken(time.Now(), rootKey)
	tok.ExpiresAt = time.Now().UTC().Add(-1 * time.Minute)
	encoded, err := tok.Encode(sk)
	require.NoError(t, err)

	// Insert into store with the past ExpiresAt so the DB also rejects it.
	store := pairing.NewStore(db)
	require.NoError(t, store.Insert(context.Background(), tok, encoded))

	svc, err := pairing.NewService(pairing.Config{
		DB:                db,
		Headscale:         &stubHeadscale{key: "hskey-test"},
		ClusterCA:         &stubCA{fp: "fp"},
		RootSigningKey:    rootKey,
		DirectoryEndpoint: "https://dir.test:8443",
	})
	require.NoError(t, err)
	recStore := pairing.NewRecorderStore(db)

	handler := pairing.CheckInHandler(svc, recStore, nil)
	w := doCheckIn(t, handler, encoded, validBody(t))

	// Decode() will reject it before Redeem() is even called.
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---------------------------------------------------------------------------
// Already-redeemed token — 20-goroutine race, exactly 1 success
// ---------------------------------------------------------------------------

func TestCheckInAlreadyRedeemedRace(t *testing.T) {
	t.Parallel()

	svc, recStore := newTestService(t)
	handler := pairing.CheckInHandler(svc, recStore, nil)

	tok := mintValidToken(t, svc)
	body := validBody(t)

	const n = 20
	var (
		wg         sync.WaitGroup
		successCnt atomic.Int32
		alreadyCnt atomic.Int32
	)
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			w := doCheckIn(t, handler, tok, body)
			switch w.Code {
			case http.StatusOK:
				successCnt.Add(1)
			case http.StatusGone: // 410 ALREADY_REDEEMED
				alreadyCnt.Add(1)
			default:
				t.Logf("unexpected status %d: %s", w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), successCnt.Load(), "exactly one goroutine must succeed")
	assert.Equal(t, int32(n-1), alreadyCnt.Load(), "all others must get 410 Gone")
}

// ---------------------------------------------------------------------------
// Tampered signature — 401
// ---------------------------------------------------------------------------

func TestCheckInTamperedSignature(t *testing.T) {
	t.Parallel()
	svc, recStore := newTestService(t)
	handler := pairing.CheckInHandler(svc, recStore, nil)

	tok := mintValidToken(t, svc)
	// Flip a byte in the payload segment.
	tampered := []byte(tok)
	if len(tampered) > 10 {
		tampered[5] ^= 0xFF
	}

	w := doCheckIn(t, handler, string(tampered), validBody(t))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---------------------------------------------------------------------------
// Missing hardware fields — 400
// ---------------------------------------------------------------------------

func TestCheckInMissingCPUCores(t *testing.T) {
	t.Parallel()
	svc, recStore := newTestService(t)
	handler := pairing.CheckInHandler(svc, recStore, nil)
	tok := mintValidToken(t, svc)

	body, err := json.Marshal(pairing.CheckInRequest{
		Hardware:     pairing.CheckInHardware{CPUCores: 0, RAMBytes: 16_000_000_000},
		DevicePubkey: "dGVzdA",
		OSRelease:    "Ubuntu 24.04",
	})
	require.NoError(t, err)

	w := doCheckIn(t, handler, tok, body)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCheckInMissingRAMBytes(t *testing.T) {
	t.Parallel()
	svc, recStore := newTestService(t)
	handler := pairing.CheckInHandler(svc, recStore, nil)
	tok := mintValidToken(t, svc)

	body, err := json.Marshal(pairing.CheckInRequest{
		Hardware:     pairing.CheckInHardware{CPUCores: 4, RAMBytes: 0},
		DevicePubkey: "dGVzdA",
		OSRelease:    "Ubuntu 24.04",
	})
	require.NoError(t, err)

	w := doCheckIn(t, handler, tok, body)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCheckInMissingDevicePubkey(t *testing.T) {
	t.Parallel()
	svc, recStore := newTestService(t)
	handler := pairing.CheckInHandler(svc, recStore, nil)
	tok := mintValidToken(t, svc)

	body, err := json.Marshal(pairing.CheckInRequest{
		Hardware:     pairing.CheckInHardware{CPUCores: 4, RAMBytes: 8_000_000_000},
		DevicePubkey: "", // missing
		OSRelease:    "Ubuntu 24.04",
	})
	require.NoError(t, err)

	w := doCheckIn(t, handler, tok, body)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---------------------------------------------------------------------------
// Missing Authorization header — 401
// ---------------------------------------------------------------------------

func TestCheckInNoAuthHeader(t *testing.T) {
	t.Parallel()
	svc, recStore := newTestService(t)
	handler := pairing.CheckInHandler(svc, recStore, nil)

	// Pass empty token — doCheckIn skips the header when token is "".
	w := doCheckIn(t, handler, "", validBody(t))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---------------------------------------------------------------------------
// Cross-tenant safety: token signed by Directory A presented to Directory B
// ---------------------------------------------------------------------------

func TestCheckInCrossTenantRejected(t *testing.T) {
	t.Parallel()

	// Directory A mints a token.
	svcA, _ := newTestService(t)
	tokA := mintValidToken(t, svcA)

	// Directory B is a different service (different root key → different signing key).
	svcB, recStoreB := newTestService(t)
	handlerB := pairing.CheckInHandler(svcB, recStoreB, nil)

	// Present Directory A's token to Directory B's handler.
	w := doCheckIn(t, handlerB, tokA, validBody(t))
	// Signature won't verify against B's public key.
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---------------------------------------------------------------------------
// Wrong HTTP method — 405
// ---------------------------------------------------------------------------

func TestCheckInWrongMethod(t *testing.T) {
	t.Parallel()
	svc, recStore := newTestService(t)
	handler := pairing.CheckInHandler(svc, recStore, nil)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/pairing/check-in", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
