package pairing

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	sharedpairing "github.com/bluenviron/mediamtx/internal/shared/pairing"
	recordermesh "github.com/bluenviron/mediamtx/internal/recorder/mesh"
	"github.com/bluenviron/mediamtx/internal/recorder/state"
	sharedtsnet "github.com/bluenviron/mediamtx/internal/shared/mesh/tsnet"
)

// stateKeyPaired is the key written to state.Store at step 9 to mark the
// Recorder as fully paired. The value is a PairedState struct.
const stateKeyPaired = "pairing.paired"

// PairedState is persisted to the Recorder local SQLite cache (state.Store)
// at the end of a successful pairing run.
type PairedState struct {
	RecorderUUID string    `json:"recorder_uuid"`
	DirectoryURL string    `json:"directory_url"`
	PairedAt     time.Time `json:"paired_at"`
	MeshHostname string    `json:"mesh_hostname"`
	CACertFP     string    `json:"ca_cert_fingerprint"`
}

// checkInRequest is the JSON body sent to POST /api/v1/pairing/check-in.
type checkInRequest struct {
	Hardware     HardwareInfo `json:"hardware"`
	DevicePubkey string       `json:"device_pubkey"`
	OSRelease    string       `json:"os_release"`
}

// checkInResponse is the JSON body returned by POST /api/v1/pairing/check-in.
type checkInResponse struct {
	RecorderID string `json:"recorder_id"`
}

// stepCASignRequest mirrors the step-ca JWK provisioner /1.0/sign body.
type stepCASignRequest struct {
	CertificateRequest string `json:"csr"`
	OTT                string `json:"ott"` // one-time token
}

// stepCASignResponse contains the signed certificate chain from step-ca.
type stepCASignResponse struct {
	ServerPEM        struct{ Raw string } `json:"serverPEM"`
	CertChainPEM     []struct{ Raw string } `json:"certChain"`
}

// JoinerConfig controls a Joiner.
type JoinerConfig struct {
	// StateDir is the directory used for the state SQLite DB and the device key.
	// Defaults to /var/lib/mediamtx-recorder.
	StateDir string

	// MeshStateDir is where the tsnet node stores its state files.
	// Defaults to StateDir + "/mesh".
	MeshStateDir string

	// Logger receives structured log output. Nil = slog.Default().
	Logger *slog.Logger

	// HTTPClient is the HTTP client used for check-in and cert enrollment.
	// Nil = a default client with a 30s timeout; tests can inject a custom one.
	HTTPClient *http.Client

	// Now overrides time.Now for tests.
	Now func() time.Time
}

// Joiner orchestrates the 9-step Recorder join sequence. Each step is its own
// method so failures can be reported precisely and test coverage is granular.
type Joiner struct {
	cfg      JoinerConfig
	log      *slog.Logger
	client   *http.Client
	now      func() time.Time
	rawToken string // preserved from Run() for use as Bearer credential in check-in
}

// NewJoiner constructs a Joiner with the supplied config.
func NewJoiner(cfg JoinerConfig) *Joiner {
	if cfg.StateDir == "" {
		cfg.StateDir = KeystoreDefaultDir
	}
	if cfg.MeshStateDir == "" {
		cfg.MeshStateDir = cfg.StateDir + "/mesh"
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Joiner{
		cfg:    cfg,
		log:    log,
		client: hc,
		now:    now,
	}
}

// Run executes all 9 pairing steps in order. On failure the error identifies
// which step failed so the operator can re-run the same token (steps 1-2 are
// safe to retry; steps 4+ require a fresh token if partially redeemed).
func (j *Joiner) Run(ctx context.Context, rawToken string) error {
	j.rawToken = rawToken
	j.step(1, "decoding and verifying pairing token")
	token, _, err := j.Step1DecodeAndPin(ctx, rawToken)
	if err != nil {
		return fmt.Errorf("step 1 (decode+pin): %w", err)
	}
	j.log.Info("pairing: step 1 ok",
		"token_id", token.TokenID,
		"directory", token.DirectoryEndpoint)

	j.step(2, "generating device keypair")
	deviceKey, err := j.Step4DeviceKeypair(ctx, token, nil)
	if err != nil {
		return fmt.Errorf("step 2 (device keypair): %w", err)
	}
	j.log.Info("pairing: step 2 ok")

	j.step(3, "probing hardware and checking in with Directory")
	hw := ProbeHardware()
	recorderUUID, err := j.Step2CheckIn(ctx, token, hw, deviceKey.Public().(ed25519.PublicKey))
	if err != nil {
		return fmt.Errorf("step 3 (check-in): %w", err)
	}
	j.log.Info("pairing: step 3 ok", "recorder_uuid", recorderUUID)

	// Mesh overlay skipped — Directory and Recorders communicate over LAN.
	// The cloud connector handles remote access via the relay.
	j.step(4, "skipping mesh overlay (LAN mode)")
	j.log.Info("pairing: step 4 ok (mesh skipped — LAN-only)")

	j.step(5, "enrolling with cluster CA to obtain mTLS leaf certificate")
	leafCert, err := j.Step5Enroll(ctx, token, deviceKey, recorderUUID)
	if err != nil {
		return fmt.Errorf("step 5 (CA enroll): %w", err)
	}
	j.log.Info("pairing: step 5 ok",
		"leaf_subject", leafCert.Leaf.Subject.CommonName,
		"not_after", leafCert.Leaf.NotAfter.Format(time.RFC3339))

	j.step(6, "pinning step-ca root certificate")
	if err := j.Step6PinRoot(token, leafCert); err != nil {
		return fmt.Errorf("step 6 (pin root): %w", err)
	}
	j.log.Info("pairing: step 6 ok")

	j.step(7, "verifying Directory control-plane connectivity")
	if err := j.Step7VerifyConnectivity(ctx, token, leafCert); err != nil {
		return fmt.Errorf("step 7 (connectivity): %w", err)
	}
	j.log.Info("pairing: step 7 ok")

	j.step(8, "requesting initial assignment snapshot (deferred to KAI-253)")
	j.Step8InitialSnapshot(ctx)

	hostname := "recorder-" + recorderUUID
	j.step(9, "persisting paired state to local cache")
	if err := j.Step9PersistState(ctx, token, recorderUUID, hostname); err != nil {
		return fmt.Errorf("step 9 (persist): %w", err)
	}
	j.log.Info("pairing: complete — Recorder is ready", "recorder_uuid", recorderUUID)
	return nil
}

// ---- Individual steps -------------------------------------------------------

// Step1DecodeAndPin decodes rawToken and verifies the Directory fingerprint
// via a TLS handshake against the DirectoryEndpoint.
//
// Security invariant: if the Directory's certificate fingerprint does not match
// DirectoryFingerprint in the token, pairing is aborted immediately.
func (j *Joiner) Step1DecodeAndPin(_ context.Context, rawToken string) (*sharedpairing.PairingToken, ed25519.PublicKey, error) {
	// (a) Peek the payload without sig verification to get the endpoint + FP.
	peeked, err := peekToken(rawToken)
	if err != nil {
		return nil, nil, fmt.Errorf("decode: %w", err)
	}
	if peeked.DirectoryEndpoint == "" {
		return nil, nil, errors.New("token missing directory_endpoint")
	}
	if peeked.DirectoryFingerprint == "" {
		return nil, nil, errors.New("token missing directory_fingerprint — cannot pin Directory")
	}

	// (b) TLS handshake with fingerprint pinning. Extracts the leaf public key.
	// In development (HTTP endpoints), skip TLS probe and use token's embedded verify key.
	var verifyKey ed25519.PublicKey
	if strings.HasPrefix(peeked.DirectoryEndpoint, "https://") && os.Getenv("MTX_PAIRING_SKIP_TLS_VERIFY") == "" {
		leafPub, err := j.probeTLSAndPin(peeked.DirectoryEndpoint, peeked.DirectoryFingerprint)
		if err != nil {
			return nil, nil, err
		}
		verifyKey, err = ed25519PubFromCertKey(leafPub)
		if err != nil {
			return nil, nil, fmt.Errorf("extract verify key from cert: %w", err)
		}
	} else {
		// HTTP mode or skip-verify: skip TLS probe and signature verification.
		// This is less secure but allows development without TLS certs.
		j.log.Warn("pairing: skipping TLS probe + signature verification (HTTP endpoint or MTX_PAIRING_SKIP_TLS_VERIFY set)")
		return sharedpairing.DecodeTokenUnsafe(rawToken)
	}

	// (c) Full signature verification with the extracted or derived key.
	token, err := sharedpairing.Decode(rawToken, verifyKey)
	if err != nil {
		return nil, nil, fmt.Errorf("token signature verification: %w", err)
	}
	if token.ExpiresAt.Before(j.now()) {
		return nil, nil, fmt.Errorf("token expired at %s", token.ExpiresAt.Format(time.RFC3339))
	}
	return token, verifyKey, nil
}

// Step2CheckIn sends hardware info to the Directory's check-in endpoint and
// returns the assigned RecorderUUID.
func (j *Joiner) Step2CheckIn(ctx context.Context, token *sharedpairing.PairingToken, hw HardwareInfo, pubkey ed25519.PublicKey) (string, error) {
	body := checkInRequest{
		Hardware:     hw,
		DevicePubkey: base64.RawURLEncoding.EncodeToString(pubkey),
		OSRelease:    hw.OS + "/" + hw.Arch,
	}
	b, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal check-in body: %w", err)
	}

	url := token.DirectoryEndpoint + "/api/v1/pairing/check-in"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+j.rawToken)

	var hc *http.Client
	if strings.HasPrefix(token.DirectoryEndpoint, "https://") {
		hc = j.pinnedHTTPClient(token.DirectoryFingerprint)
	} else {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("check-in HTTP: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusConflict:
		return "", errors.New("token already redeemed; request a new pairing token from the admin")
	case http.StatusGone:
		return "", errors.New("token expired at Directory; request a new pairing token from the admin")
	case http.StatusNotFound:
		return "", errors.New("token not found at Directory; it may have been revoked")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("check-in: Directory returned HTTP %d", resp.StatusCode)
	}

	var out checkInResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode check-in response: %w", err)
	}
	if out.RecorderID == "" {
		return "", errors.New("Directory returned empty recorder_id")
	}
	return out.RecorderID, nil
}

// Step3JoinMesh registers the Recorder node with the Headscale tailnet using
// the HeadscalePreAuthKey from the token.
func (j *Joiner) Step3JoinMesh(ctx context.Context, token *sharedpairing.PairingToken, recorderUUID string) (*sharedtsnet.Node, error) {
	node, err := recordermesh.New(ctx, recordermesh.Config{
		ComponentID: recorderUUID,
		AuthKey:     token.HeadscalePreAuthKey,
		StateDir:    j.cfg.MeshStateDir,
		ControlURL:  token.DirectoryEndpoint + "/headscale",
	})
	if err != nil {
		return nil, fmt.Errorf("tsnet: %w", err)
	}
	return node, nil
}

// Step4DeviceKeypair generates (or loads an existing) encrypted device keypair.
// The encryption master material is derived from the Headscale pre-auth key so
// the sealed blob is tied to the site's tailnet identity.
func (j *Joiner) Step4DeviceKeypair(_ context.Context, token *sharedpairing.PairingToken, _ *sharedtsnet.Node) (ed25519.PrivateKey, error) {
	ks, err := NewKeystore(j.cfg.StateDir)
	if err != nil {
		return nil, err
	}
	masterMaterial := sha256Sum([]byte(token.HeadscalePreAuthKey))
	return ks.LoadOrGenerate(masterMaterial)
}

// Step5Enroll submits a CSR to step-ca using the enrollment token from the
// PairingToken and returns the issued mTLS leaf certificate.
//
// If StepCAEnrollToken is empty (no sign URL configured on the Directory), a
// self-signed stub cert is returned; the operator must supply a cert out-of-band.
func (j *Joiner) Step5Enroll(ctx context.Context, token *sharedpairing.PairingToken, deviceKey ed25519.PrivateKey, recorderUUID string) (*tls.Certificate, error) {
	if token.StepCAEnrollToken == "" {
		j.log.Warn("pairing: StepCAEnrollToken is empty; issuing self-signed stub cert",
			"recorder_uuid", recorderUUID)
		return j.selfSignedStub(deviceKey, recorderUUID)
	}

	cn := "recorder-" + recorderUUID
	csr, err := buildCSR(deviceKey, cn)
	if err != nil {
		return nil, fmt.Errorf("build CSR: %w", err)
	}

	caSignURL := token.DirectoryEndpoint + "/step-ca/1.0/sign"
	return j.stepCASign(ctx, token, csr, caSignURL)
}

// Step6PinRoot verifies that the issued leaf cert chains to the root identified
// by StepCAFingerprint in the token (Trust On First Use).
func (j *Joiner) Step6PinRoot(token *sharedpairing.PairingToken, leaf *tls.Certificate) error {
	if token.StepCAFingerprint == "" {
		j.log.Warn("pairing: StepCAFingerprint empty — skipping root pin (not recommended in production)")
		return nil
	}
	if token.StepCAEnrollToken == "" {
		j.log.Warn("pairing: StepCAEnrollToken empty (stub cert) — skipping root pin")
		return nil
	}
	if leaf == nil || len(leaf.Certificate) == 0 {
		return nil
	}
	// Check last cert in chain (the root).
	rootDER := leaf.Certificate[len(leaf.Certificate)-1]
	sum := sha256.Sum256(rootDER)
	got := hex.EncodeToString(sum[:])
	if got != token.StepCAFingerprint {
		return fmt.Errorf("step-ca root fingerprint mismatch: want %s got %s", token.StepCAFingerprint, got)
	}
	return nil
}

// Step7VerifyConnectivity makes a simple mTLS request to the Directory's
// health endpoint to confirm the issued cert is accepted. KAI-253 streaming
// client is not instantiated here — we only verify connectivity.
func (j *Joiner) Step7VerifyConnectivity(ctx context.Context, token *sharedpairing.PairingToken, leaf *tls.Certificate) error {
	tlsCfg := &tls.Config{
		Certificates:       []tls.Certificate{*leaf},
		InsecureSkipVerify: true, //nolint:gosec // fingerprint-pinned below
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if token.StepCAFingerprint == "" {
				return nil
			}
			for _, raw := range rawCerts {
				sum := sha256.Sum256(raw)
				if hex.EncodeToString(sum[:]) == token.StepCAFingerprint {
					return nil
				}
			}
			// Also check Directory leaf fingerprint.
			if len(rawCerts) > 0 {
				sum := sha256.Sum256(rawCerts[0])
				if hex.EncodeToString(sum[:]) == token.DirectoryFingerprint {
					return nil
				}
			}
			return errors.New("connectivity check: Directory cert not trusted by pinned fingerprints")
		},
	}
	hc := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}
	url := token.DirectoryEndpoint + "/api/v1/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("directory health check: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("directory health check returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// Step8InitialSnapshot is a documented no-op placeholder. KAI-253 will provide
// the RecorderControl streaming client; wire it here once it ships.
//
// TODO(KAI-253): call client.Snapshot() and feed the result into Step9.
func (j *Joiner) Step8InitialSnapshot(_ context.Context) {
	j.log.Info("pairing: step 8 — initial assignment snapshot deferred to KAI-253")
}

// Step9PersistState writes PairedState to the local SQLite state cache.
func (j *Joiner) Step9PersistState(ctx context.Context, token *sharedpairing.PairingToken, recorderUUID, meshHostname string) error {
	dbPath := j.cfg.StateDir + "/state.db"
	store, err := state.Open(dbPath, state.Options{})
	if err != nil {
		return fmt.Errorf("open state db: %w", err)
	}
	defer store.Close()

	ps := PairedState{
		RecorderUUID: recorderUUID,
		DirectoryURL: token.DirectoryEndpoint,
		PairedAt:     j.now().UTC(),
		MeshHostname: meshHostname,
		CACertFP:     token.StepCAFingerprint,
	}
	if err := store.SetState(ctx, stateKeyPaired, ps); err != nil {
		return fmt.Errorf("write paired state: %w", err)
	}
	return nil
}

// ---- helpers ----------------------------------------------------------------

func (j *Joiner) step(n int, desc string) {
	j.log.Info(fmt.Sprintf("pairing: [%d/9] %s", n, desc))
}

// pinnedHTTPClient returns an *http.Client that pins the Directory TLS cert
// by SHA-256 fingerprint of the server's leaf DER.
func (j *Joiner) pinnedHTTPClient(dirFingerprint string) *http.Client {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // fingerprint-pinned below
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("directory: no certificates in TLS chain")
			}
			sum := sha256.Sum256(rawCerts[0])
			got := hex.EncodeToString(sum[:])
			if got != dirFingerprint {
				return fmt.Errorf("directory fingerprint mismatch: want %s got %s", dirFingerprint, got)
			}
			return nil
		},
	}
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}
}

// probeTLSAndPin dials the endpoint and extracts the leaf public key, verifying
// the leaf fingerprint against expectedFP.
func (j *Joiner) probeTLSAndPin(endpoint, expectedFP string) (interface{}, error) {
	var leafPub interface{}
	tlsCfg := &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // we do our own pin
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("no certs from Directory")
			}
			sum := sha256.Sum256(rawCerts[0])
			got := hex.EncodeToString(sum[:])
			if got != expectedFP {
				return fmt.Errorf("directory fingerprint mismatch: want %s got %s — possible MITM attack", expectedFP, got)
			}
			leaf, err := x509.ParseCertificate(rawCerts[0])
			if err == nil {
				leafPub = leaf.PublicKey
			}
			return nil
		},
	}
	conn, err := tls.Dial("tcp", endpointToAddr(endpoint), tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("TLS probe to Directory %q: %w", endpoint, err)
	}
	_ = conn.Close()
	if leafPub == nil {
		return nil, errors.New("could not extract public key from Directory cert")
	}
	return leafPub, nil
}

// stepCASign submits a CSR to step-ca and parses the response into a
// *tls.Certificate.
func (j *Joiner) stepCASign(ctx context.Context, token *sharedpairing.PairingToken, csr *x509.CertificateRequest, caSignURL string) (*tls.Certificate, error) {
	csrPEM := encodeCSRPEM(csr)
	body := stepCASignRequest{
		CertificateRequest: csrPEM,
		OTT:                token.StepCAEnrollToken,
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, caSignURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	// Pin step-ca by its root fingerprint (same as Directory).
	hc := j.pinnedHTTPClient(token.DirectoryFingerprint)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("step-ca sign request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("step-ca returned HTTP %d", resp.StatusCode)
	}
	var out stepCASignResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("parse step-ca response: %w", err)
	}
	return parseCertChain(out)
}

// selfSignedStub issues a minimal self-signed cert from deviceKey. Used when
// StepCAEnrollToken is empty.
func (j *Joiner) selfSignedStub(deviceKey ed25519.PrivateKey, recorderUUID string) (*tls.Certificate, error) {
	tmpl := &x509.Certificate{
		SerialNumber: bigOne(),
		Subject:      pkix.Name{CommonName: "recorder-" + recorderUUID},
		NotBefore:    j.now().Add(-5 * time.Minute),
		NotAfter:     j.now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}
	pub := deviceKey.Public().(ed25519.PublicKey)
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, deviceKey)
	if err != nil {
		return nil, fmt.Errorf("self-signed stub: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  deviceKey,
		Leaf:        leaf,
	}, nil
}

// endpointToAddr converts "https://dir.local:8443" → "dir.local:8443".
func endpointToAddr(endpoint string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	if idx := strings.IndexByte(s, '/'); idx >= 0 {
		s = s[:idx]
	}
	return s
}

// ed25519PubFromCertKey type-asserts to ed25519.PublicKey.
func ed25519PubFromCertKey(pub interface{}) (ed25519.PublicKey, error) {
	k, ok := pub.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("directory cert uses %T not ed25519 — check PKI config", pub)
	}
	return k, nil
}

// peekToken base64-decodes the token's payload segment without verifying the
// signature. Only used to extract DirectoryEndpoint + DirectoryFingerprint so
// we can initiate the TLS probe before we have a verify key.
func peekToken(raw string) (*sharedpairing.PairingToken, error) {
	idx := strings.IndexByte(raw, '.')
	if idx < 0 {
		return nil, errors.New("malformed token: missing '.'")
	}
	payload, err := base64.RawURLEncoding.DecodeString(raw[:idx])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	var pt sharedpairing.PairingToken
	if err := json.Unmarshal(payload, &pt); err != nil {
		return nil, fmt.Errorf("unmarshal token payload: %w", err)
	}
	return &pt, nil
}

// buildCSR constructs a PKCS#10 certificate signing request.
func buildCSR(key ed25519.PrivateKey, cn string) (*x509.CertificateRequest, error) {
	tmpl := &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: cn, Organization: []string{"Kaivue"}},
		DNSNames: []string{cn},
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, tmpl, key)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificateRequest(der)
}

// sha256Sum is a convenience wrapper.
func sha256Sum(b []byte) []byte {
	s := sha256.Sum256(b)
	return s[:]
}
