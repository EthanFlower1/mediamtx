package pairing_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dirpairing "github.com/bluenviron/mediamtx/internal/directory/pairing"
	"github.com/bluenviron/mediamtx/internal/recorder/pairing"
)

// ----------------------------------------------------------------------------
// Test helpers
// ----------------------------------------------------------------------------

// fakeDirectory bundles a self-signed TLS cert + key and hosts the check-in
// and health endpoints on an httptest.Server.
type fakeDirectory struct {
	server      *httptest.Server
	certDER     []byte
	leafPub     ed25519.PublicKey
	leafPriv    ed25519.PrivateKey
	fingerprint string // SHA-256 hex of leaf cert DER
}

// newFakeDirectory starts a TLS httptest.Server with an ed25519 self-signed
// cert and registers routes for check-in and health.
func newFakeDirectory(t *testing.T, recorderUUID string) *fakeDirectory {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "fake-directory"},
		DNSNames:              []string{"localhost", "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	require.NoError(t, err)

	sum := sha256.Sum256(der)
	fp := hex.EncodeToString(sum[:])

	tlsCert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  priv,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/pairing/check-in", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			TokenID string `json:"token_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"recorder_uuid": recorderUUID})
	})
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewUnstartedServer(mux)
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	return &fakeDirectory{
		server:      srv,
		certDER:     der,
		leafPub:     pub,
		leafPriv:    priv,
		fingerprint: fp,
	}
}

// endpoint returns the base URL of the fake directory.
func (fd *fakeDirectory) endpoint() string { return fd.server.URL }

// makePairingToken builds a fully-formed PairingToken signed by the fake
// directory's leaf key. The Directory uses its own leaf key as the pairing
// signing key so that Step1 can verify the signature from the TLS cert.
func (fd *fakeDirectory) makePairingToken(t *testing.T, signingKey ed25519.PrivateKey, now time.Time) (string, *dirpairing.PairingToken) {
	t.Helper()
	pt := &dirpairing.PairingToken{
		TokenID:              "test-token-id-" + t.Name(),
		DirectoryEndpoint:    fd.endpoint(),
		HeadscalePreAuthKey:  "hskey-test-deadbeef",
		StepCAFingerprint:    fd.fingerprint, // reuse dir FP as "CA" FP for stub
		StepCAEnrollToken:    "",             // empty so Step5 uses self-signed stub
		DirectoryFingerprint: fd.fingerprint,
		SuggestedRoles:       []string{"recorder"},
		ExpiresAt:            now.Add(15 * time.Minute),
		SignedBy:             "admin-test",
	}
	encoded, err := pt.Encode(signingKey)
	require.NoError(t, err)
	return encoded, pt
}

// makeJoiner creates a Joiner with an httptest-compatible pinned HTTP client
// and a temp state dir.
func makeJoiner(t *testing.T, fd *fakeDirectory) *pairing.Joiner {
	t.Helper()
	stateDir := t.TempDir()

	// Build an HTTP client that trusts the fake directory's self-signed cert.
	pinnedTransport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // test only
		},
	}

	return pairing.NewJoiner(pairing.JoinerConfig{
		StateDir:     stateDir,
		MeshStateDir: stateDir + "/mesh",
		HTTPClient:   &http.Client{Timeout: 5 * time.Second, Transport: pinnedTransport},
		Now:          func() time.Time { return time.Now() },
	})
}

// ----------------------------------------------------------------------------
// Tests
// ----------------------------------------------------------------------------

// TestHardwareProbeRuns verifies ProbeHardware succeeds on the test machine
// and returns non-zero RAM.
func TestHardwareProbeRuns(t *testing.T) {
	t.Parallel()
	hw := pairing.ProbeHardware()
	assert.NotEmpty(t, hw.OS, "OS should be populated")
	assert.NotEmpty(t, hw.Arch, "Arch should be populated")
	assert.Greater(t, hw.RAMTotalBytes, uint64(0), "RAM should be detectable")
}

// TestKeystoreRoundTrip verifies that a generated keypair survives encrypt →
// decrypt.
func TestKeystoreRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ks, err := pairing.NewKeystore(dir)
	require.NoError(t, err)

	master := []byte("test-master-material-1234567890ab")
	priv, err := ks.LoadOrGenerate(master)
	require.NoError(t, err)
	require.NotNil(t, priv)

	// Second call should return the same key.
	priv2, err := ks.LoadOrGenerate(master)
	require.NoError(t, err)
	assert.Equal(t, []byte(priv), []byte(priv2), "same key should be returned on reload")
}

// TestStep1DecodeAndPinHappyPath verifies that a valid token signed by the
// fake directory's key is accepted when the fingerprint matches.
func TestStep1DecodeAndPinHappyPath(t *testing.T) {
	t.Parallel()
	fd := newFakeDirectory(t, "recorder-uuid-001")
	joiner := makeJoiner(t, fd)

	// The fake directory uses its leaf key as the pairing signing key.
	encoded, _ := fd.makePairingToken(t, fd.leafPriv, time.Now())

	token, _, err := joiner.Step1DecodeAndPin(context.Background(), encoded)
	require.NoError(t, err)
	assert.Equal(t, "test-token-id-"+t.Name(), token.TokenID)
	assert.Equal(t, fd.endpoint(), token.DirectoryEndpoint)
}

// TestStep1FingerprintMismatch verifies that a tampered DirectoryFingerprint
// causes Step1 to reject the connection.
func TestStep1FingerprintMismatch(t *testing.T) {
	t.Parallel()
	fd := newFakeDirectory(t, "recorder-uuid-002")
	joiner := makeJoiner(t, fd)

	// Build a token with a wrong fingerprint.
	pt := &dirpairing.PairingToken{
		TokenID:              "bad-fp-token",
		DirectoryEndpoint:    fd.endpoint(),
		HeadscalePreAuthKey:  "key",
		StepCAFingerprint:    "aabbcc",
		DirectoryFingerprint: "000000000000000000000000000000000000000000000000000000000000dead",
		ExpiresAt:            time.Now().Add(15 * time.Minute),
		SignedBy:             "admin",
	}
	encoded, err := pt.Encode(fd.leafPriv)
	require.NoError(t, err)

	_, _, err = joiner.Step1DecodeAndPin(context.Background(), encoded)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fingerprint mismatch")
}

// TestStep1ExpiredToken verifies that an expired token is rejected.
func TestStep1ExpiredToken(t *testing.T) {
	t.Parallel()
	fd := newFakeDirectory(t, "recorder-uuid-003")
	joiner := makeJoiner(t, fd)

	past := time.Now().Add(-1 * time.Hour)
	pt := &dirpairing.PairingToken{
		TokenID:              "expired-token",
		DirectoryEndpoint:    fd.endpoint(),
		HeadscalePreAuthKey:  "key",
		StepCAFingerprint:    fd.fingerprint,
		DirectoryFingerprint: fd.fingerprint,
		ExpiresAt:            past,
		SignedBy:             "admin",
	}
	encoded, err := pt.Encode(fd.leafPriv)
	require.NoError(t, err)

	// dirpairing.Decode itself rejects expired tokens.
	_, _, err = joiner.Step1DecodeAndPin(context.Background(), encoded)
	require.Error(t, err)
}

// TestStep2CheckInHappyPath verifies the check-in endpoint interaction.
func TestStep2CheckInHappyPath(t *testing.T) {
	t.Parallel()
	const wantUUID = "recorder-uuid-checkin"
	fd := newFakeDirectory(t, wantUUID)
	joiner := makeJoiner(t, fd)

	encoded, token := fd.makePairingToken(t, fd.leafPriv, time.Now())
	_ = encoded // token is what we use

	hw := pairing.ProbeHardware()
	uuid, err := joiner.Step2CheckIn(context.Background(), token, hw)
	require.NoError(t, err)
	assert.Equal(t, wantUUID, uuid)
}

// TestStep2CheckInAlreadyRedeemed verifies that HTTP 409 maps to a clear error.
func TestStep2CheckInAlreadyRedeemed(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/pairing/check-in", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "conflict", http.StatusConflict)
	})
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	certDER, fp, tlsCert := makeSelfSignedCert(t, pub, priv)
	_ = certDER

	srv := httptest.NewUnstartedServer(mux)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{tlsCert}}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	token := &dirpairing.PairingToken{
		TokenID:              "redeemed-token",
		DirectoryEndpoint:    srv.URL,
		DirectoryFingerprint: fp,
		ExpiresAt:            time.Now().Add(15 * time.Minute),
	}

	stateDir := t.TempDir()
	joiner := pairing.NewJoiner(pairing.JoinerConfig{
		StateDir: stateDir,
		HTTPClient: &http.Client{
			Timeout:   5 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec
		},
	})

	hw := pairing.ProbeHardware()
	_, err = joiner.Step2CheckIn(context.Background(), token, hw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already redeemed")
}

// TestStep4DeviceKeypairEncryptionRoundTrip generates, saves, and reloads the
// device keypair to confirm encryption round-trip.
func TestStep4DeviceKeypairEncryptionRoundTrip(t *testing.T) {
	t.Parallel()
	// Reuse keystore directly — step4 calls it under the hood.
	dir := t.TempDir()
	ks, err := pairing.NewKeystore(dir)
	require.NoError(t, err)

	masterA := []byte("master-material-a")
	privA, err := ks.LoadOrGenerate(masterA)
	require.NoError(t, err)
	require.NotNil(t, privA)

	// Reload with same master — must get same key.
	privB, err := ks.LoadOrGenerate(masterA)
	require.NoError(t, err)
	assert.Equal(t, []byte(privA), []byte(privB))

	// Wrong master material must fail to decrypt.
	ks2, err := pairing.NewKeystore(dir)
	require.NoError(t, err)
	_, err = ks2.LoadOrGenerate([]byte("wrong-master-material"))
	require.Error(t, err, "decryption with wrong key should fail")
}

// TestStep6PinRootMismatch verifies root fingerprint mismatch is caught.
func TestStep6PinRootMismatch(t *testing.T) {
	t.Parallel()
	stateDir := t.TempDir()
	joiner := pairing.NewJoiner(pairing.JoinerConfig{StateDir: stateDir})

	token := &dirpairing.PairingToken{
		StepCAFingerprint: "aaaa",
	}
	// Build a leaf cert with a different root fingerprint.
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	certDER, _, _ := makeSelfSignedCert(t, pub, priv)
	leaf, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)
	tlsCert := &tls.Certificate{
		Certificate: [][]byte{certDER},
		Leaf:        leaf,
	}

	err = joiner.Step6PinRoot(token, tlsCert)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fingerprint mismatch")
}

// TestStep9PersistState verifies the state db write.
func TestStep9PersistState(t *testing.T) {
	t.Parallel()
	stateDir := t.TempDir()
	joiner := pairing.NewJoiner(pairing.JoinerConfig{StateDir: stateDir})

	token := &dirpairing.PairingToken{
		DirectoryEndpoint: "https://dir.test:8443",
		StepCAFingerprint: "cafebabe",
	}
	err := joiner.Step9PersistState(context.Background(), token, "uuid-step9-test", "recorder-uuid-step9-test")
	require.NoError(t, err)

	// Verify file exists.
	_, err = os.Stat(stateDir + "/state.db")
	require.NoError(t, err)
}

// TestHardwareInfoValidate confirms Validate rejects zero RAM.
func TestHardwareInfoValidate(t *testing.T) {
	t.Parallel()
	h := pairing.HardwareInfo{}
	require.Error(t, h.Validate(), "zero RAM should fail validation")

	h.RAMTotalBytes = 1024 * 1024 * 1024
	require.NoError(t, h.Validate())
}

// ----------------------------------------------------------------------------
// Helpers used only by tests
// ----------------------------------------------------------------------------

func makeSelfSignedCert(t *testing.T, pub ed25519.PublicKey, priv ed25519.PrivateKey) ([]byte, string, tls.Certificate) {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "test"},
		DNSNames:              []string{"localhost"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	require.NoError(t, err)
	sum := sha256.Sum256(der)
	fp := hex.EncodeToString(sum[:])
	tlsCert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  priv,
	}
	return der, fp, tlsCert
}
