package federation

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/cryptostore"
)

// testMaster returns a deterministic 32-byte master key for test use.
func testMaster() []byte {
	return []byte("0123456789abcdef0123456789abcdef")
}

// newTestCA constructs a FederationCA in t.TempDir with air-gapped mode.
func newTestCA(t *testing.T) (*FederationCA, string) {
	t.Helper()
	dir := t.TempDir()
	ca, err := New(Config{
		StateDir:  dir,
		MasterKey: testMaster(),
		SiteID:    "test-site-001",
		Mode:      ModeAirGapped,
		Logf:      func(string, ...any) {},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return ca, dir
}

// --- Bootstrap tests --------------------------------------------------------

func TestBootstrapCreatesRootAndEncryptsKey(t *testing.T) {
	ca, dir := newTestCA(t)
	t.Cleanup(func() { _ = ca.Shutdown(context.Background()) })

	// Root cert file present.
	certPath := filepath.Join(dir, fileRootCert)
	if _, err := os.Stat(certPath); err != nil {
		t.Fatalf("root cert missing: %v", err)
	}

	// Key file present with 0600 perms.
	keyPath := filepath.Join(dir, fileRootKey)
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("root key missing: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("root key perms = %o, want 0600", perm)
	}

	// Sealed key must contain encrypted PEM type, not plaintext.
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(keyBytes), pemTypeEncryptedPrivKey) {
		t.Errorf("expected sealed PEM type %q in file", pemTypeEncryptedPrivKey)
	}

	// Fingerprint populated.
	if fp := ca.Fingerprint(); len(fp) != 64 {
		t.Errorf("fingerprint length = %d, want 64 hex chars", len(fp))
	}

	// RootPEM parses as a CERTIFICATE block.
	block, _ := pem.Decode(ca.RootPEM())
	if block == nil || block.Type != pemTypeCertificate {
		t.Fatalf("RootPEM did not decode as CERTIFICATE")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if cert.Subject.CommonName != subjectRootCN {
		t.Errorf("CN = %q, want %q", cert.Subject.CommonName, subjectRootCN)
	}
	if !cert.IsCA {
		t.Errorf("root cert IsCA = false")
	}
}

func TestBootstrapIsDistinctFromClusterCA(t *testing.T) {
	ca, _ := newTestCA(t)
	defer ca.Shutdown(context.Background())

	// Verify the CN is federation-specific, not cluster.
	block, _ := pem.Decode(ca.RootPEM())
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if cert.Subject.CommonName == "Kaivue Site Root CA" {
		t.Error("federation root CN should not match cluster CA CN")
	}
	if cert.Subject.CommonName != subjectRootCN {
		t.Errorf("CN = %q, want %q", cert.Subject.CommonName, subjectRootCN)
	}
}

func TestBootstrapUsesEd25519Root(t *testing.T) {
	ca, _ := newTestCA(t)
	defer ca.Shutdown(context.Background())

	rootCert := ca.RootCert()
	if rootCert.PublicKeyAlgorithm != x509.Ed25519 {
		t.Errorf("root public key algorithm = %v, want Ed25519", rootCert.PublicKeyAlgorithm)
	}
}

func TestBootstrapIdempotentAcrossReload(t *testing.T) {
	dir := t.TempDir()
	master := testMaster()
	ca1, err := New(Config{StateDir: dir, MasterKey: master, SiteID: "test", Mode: ModeAirGapped})
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	fp1 := ca1.Fingerprint()
	rootPEM1 := ca1.RootPEM()
	_ = ca1.Shutdown(context.Background())

	ca2, err := New(Config{StateDir: dir, MasterKey: master, SiteID: "test", Mode: ModeAirGapped})
	if err != nil {
		t.Fatalf("reload New: %v", err)
	}
	defer ca2.Shutdown(context.Background())

	if ca2.Fingerprint() != fp1 {
		t.Errorf("fingerprint changed on reload: %s -> %s", fp1, ca2.Fingerprint())
	}
	if string(ca2.RootPEM()) != string(rootPEM1) {
		t.Error("root PEM changed on reload")
	}
}

func TestFingerprintStableAcrossReloads(t *testing.T) {
	dir := t.TempDir()
	master := testMaster()
	fingerprints := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		ca, err := New(Config{StateDir: dir, MasterKey: master, SiteID: "test", Mode: ModeAirGapped})
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		fingerprints = append(fingerprints, ca.Fingerprint())
		_ = ca.Shutdown(context.Background())
	}
	for i := 1; i < len(fingerprints); i++ {
		if fingerprints[i] != fingerprints[0] {
			t.Errorf("fingerprint drifted on iteration %d: %s vs %s", i, fingerprints[i], fingerprints[0])
		}
	}
}

func TestRootPEMIsDefensiveCopy(t *testing.T) {
	ca, _ := newTestCA(t)
	defer ca.Shutdown(context.Background())

	p1 := ca.RootPEM()
	for i := range p1 {
		p1[i] = 0
	}
	p2 := ca.RootPEM()
	if len(p2) == 0 || p2[0] == 0 {
		t.Error("RootPEM returned aliased buffer")
	}
}

// --- Peer cert issuance tests -----------------------------------------------

func TestIssuePeerCertSignedByFederationRoot(t *testing.T) {
	ca, _ := newTestCA(t)
	defer ca.Shutdown(context.Background())

	leaf, _ := issueTestPeerLeaf(t, ca, "peer-dir-002.kaivue.local")

	if err := leaf.CheckSignatureFrom(ca.RootCert()); err != nil {
		t.Fatalf("peer leaf not signed by federation root: %v", err)
	}
}

func TestIssuePeerCertVerifiesAgainstRootPool(t *testing.T) {
	ca, _ := newTestCA(t)
	defer ca.Shutdown(context.Background())

	leaf, _ := issueTestPeerLeaf(t, ca, "peer-dir-002.kaivue.local")

	opts := x509.VerifyOptions{
		Roots:     ca.RootPool(),
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		DNSName:   "peer-dir-002.kaivue.local",
	}
	if _, err := leaf.Verify(opts); err != nil {
		t.Fatalf("peer leaf verify: %v", err)
	}
}

func TestIssuePeerCertClampsTTL(t *testing.T) {
	ca, _ := newTestCA(t)
	defer ca.Shutdown(context.Background())

	leaf, _ := issueTestPeerLeafTTL(t, ca, "peer.local", 365*24*time.Hour)
	windowSeconds := leaf.NotAfter.Sub(leaf.NotBefore).Seconds()
	want := (peerValidity + clockSkew).Seconds()
	if windowSeconds > want+1 {
		t.Errorf("peer leaf validity window = %.0fs, expected clamped to ~%.0fs", windowSeconds, want)
	}
}

func TestIssuePeerCertRejectsNilCSR(t *testing.T) {
	ca, _ := newTestCA(t)
	defer ca.Shutdown(context.Background())

	if _, err := ca.IssuePeerCert(context.Background(), nil, 0); err == nil {
		t.Fatal("expected error on nil CSR")
	}
}

func TestSelfPeerCertChainsToRoot(t *testing.T) {
	ca, _ := newTestCA(t)
	defer ca.Shutdown(context.Background())

	tlsCert, err := ca.IssueSelfPeerCert(context.Background())
	if err != nil {
		t.Fatalf("IssueSelfPeerCert: %v", err)
	}
	if tlsCert.Leaf == nil {
		t.Fatal("tlsCert.Leaf nil")
	}
	if !strings.HasPrefix(tlsCert.Leaf.Subject.CommonName, "federation-peer-") {
		t.Errorf("CN = %q, want federation-peer-*", tlsCert.Leaf.Subject.CommonName)
	}

	opts := x509.VerifyOptions{
		Roots:     ca.RootPool(),
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if _, err := tlsCert.Leaf.Verify(opts); err != nil {
		t.Fatalf("self peer leaf verify: %v", err)
	}
}

// --- Peer enrollment token tests -------------------------------------------

func TestMintPeerEnrollmentToken(t *testing.T) {
	ca, _ := newTestCA(t)
	defer ca.Shutdown(context.Background())

	tokenStr, err := ca.MintPeerEnrollmentToken(
		"https://dir.site-a.kaivue.local:8443",
		"peer-site-002",
		"admin@site-a",
		0,
	)
	if err != nil {
		t.Fatalf("MintPeerEnrollmentToken: %v", err)
	}
	if !strings.Contains(tokenStr, ".") {
		t.Error("token missing signature separator")
	}
}

func TestPeerEnrollmentTokenContainsFingerprint(t *testing.T) {
	ca, _ := newTestCA(t)
	defer ca.Shutdown(context.Background())

	tokenStr, err := ca.MintPeerEnrollmentToken(
		"https://dir.site-a.kaivue.local:8443",
		"peer-site-002",
		"admin@site-a",
		0,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Derive verify key from the root key for decoding.
	ca.mu.RLock()
	rootKey := ca.rootKey
	ca.mu.RUnlock()

	verifyKey, err := DerivePeerTokenVerifyKey(rootKey)
	if err != nil {
		t.Fatal(err)
	}

	pet, err := DecodePeerEnrollmentToken(tokenStr, verifyKey)
	if err != nil {
		t.Fatalf("DecodePeerEnrollmentToken: %v", err)
	}

	if pet.FederationCAFingerprint != ca.Fingerprint() {
		t.Errorf("token fingerprint = %s, want %s", pet.FederationCAFingerprint, ca.Fingerprint())
	}
	if pet.FederationCARootPEM == "" {
		t.Error("token missing federation CA root PEM")
	}
	if pet.PeerSiteID != "peer-site-002" {
		t.Errorf("peer site ID = %q, want peer-site-002", pet.PeerSiteID)
	}
	if pet.FoundingDirectoryEndpoint != "https://dir.site-a.kaivue.local:8443" {
		t.Errorf("endpoint = %q", pet.FoundingDirectoryEndpoint)
	}
}

func TestPeerEnrollmentTokenRejectsExpired(t *testing.T) {
	dir := t.TempDir()
	// Use a clock that is in the past.
	pastTime := time.Now().Add(-1 * time.Hour)
	ca, err := New(Config{
		StateDir:  dir,
		MasterKey: testMaster(),
		SiteID:    "test",
		Mode:      ModeAirGapped,
		Clock:     func() time.Time { return pastTime },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ca.Shutdown(context.Background())

	// Mint with very short TTL — the token expires in the past.
	tokenStr, err := ca.MintPeerEnrollmentToken(
		"https://dir.local:8443",
		"peer",
		"admin",
		1*time.Millisecond,
	)
	if err != nil {
		t.Fatal(err)
	}

	ca.mu.RLock()
	rootKey := ca.rootKey
	ca.mu.RUnlock()

	verifyKey, err := DerivePeerTokenVerifyKey(rootKey)
	if err != nil {
		t.Fatal(err)
	}

	_, err = DecodePeerEnrollmentToken(tokenStr, verifyKey)
	if err == nil {
		t.Fatal("expected expired token error")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("expected 'expired' in error, got: %v", err)
	}
}

func TestPeerEnrollmentTokenRejectsWrongKey(t *testing.T) {
	ca, _ := newTestCA(t)
	defer ca.Shutdown(context.Background())

	tokenStr, err := ca.MintPeerEnrollmentToken(
		"https://dir.local:8443",
		"peer",
		"admin",
		0,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Use a completely different key to verify.
	_, wrongKey, _ := ed25519.GenerateKey(rand.Reader)
	wrongVerify, _ := DerivePeerTokenVerifyKey(wrongKey)

	_, err = DecodePeerEnrollmentToken(tokenStr, wrongVerify)
	if err == nil {
		t.Fatal("expected signature verification failure")
	}
	if !strings.Contains(err.Error(), "signature") {
		t.Errorf("expected 'signature' in error, got: %v", err)
	}
}

func TestMintPeerEnrollmentTokenRejectsEmptyEndpoint(t *testing.T) {
	ca, _ := newTestCA(t)
	defer ca.Shutdown(context.Background())

	_, err := ca.MintPeerEnrollmentToken("", "peer", "admin", 0)
	if err == nil {
		t.Fatal("expected error on empty endpoint")
	}
}

func TestMintPeerEnrollmentTokenRejectsEmptyPeerID(t *testing.T) {
	ca, _ := newTestCA(t)
	defer ca.Shutdown(context.Background())

	_, err := ca.MintPeerEnrollmentToken("https://dir.local:8443", "", "admin", 0)
	if err == nil {
		t.Fatal("expected error on empty peer ID")
	}
}

// --- mTLS handshake test ---------------------------------------------------

func TestMTLSHandshakeBetweenTwoDirectories(t *testing.T) {
	// Simulate two Directories in the same federation sharing the same root CA.
	// Directory A is the founding site; Directory B joins and gets a peer cert.
	caA, _ := newTestCA(t)
	defer caA.Shutdown(context.Background())

	// Issue a peer cert for Directory B by creating a CSR.
	_, privB, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	csrTmpl := &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: "federation-peer-site-b"},
		DNSNames: []string{"federation-peer-site-b", "localhost"},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTmpl, privB)
	if err != nil {
		t.Fatal(err)
	}
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		t.Fatal(err)
	}
	leafB, err := caA.IssuePeerCert(context.Background(), csr, 0)
	if err != nil {
		t.Fatalf("IssuePeerCert for B: %v", err)
	}

	// Build TLS configs for both sides.
	rootPool := caA.RootPool()

	// Directory A's TLS config (server side with its self peer cert).
	selfCertA, err := caA.IssueSelfPeerCert(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	serverConf := &tls.Config{
		Certificates: []tls.Certificate{*selfCertA},
		ClientCAs:    rootPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}

	// Directory B's TLS config (client side).
	certB := &tls.Certificate{
		Certificate: [][]byte{leafB.Raw, caA.RootCert().Raw},
		PrivateKey:  privB,
		Leaf:        leafB,
	}
	clientConf := &tls.Config{
		Certificates: []tls.Certificate{*certB},
		RootCAs:      rootPool,
		MinVersion:   tls.VersionTLS13,
		// Skip hostname verification for this test since we're using a pipe.
		InsecureSkipVerify: false,
		ServerName:         "localhost",
	}

	// Perform the TLS handshake over an in-memory pipe.
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	errCh := make(chan error, 2)

	go func() {
		srv := tls.Server(serverConn, serverConf)
		err := srv.Handshake()
		if err == nil {
			// Verify the peer presented a valid cert.
			state := srv.ConnectionState()
			if len(state.PeerCertificates) == 0 {
				err = errors.New("server: no peer certificates")
			}
		}
		errCh <- err
	}()

	go func() {
		cli := tls.Client(clientConn, clientConf)
		err := cli.Handshake()
		if err == nil {
			state := cli.ConnectionState()
			if len(state.PeerCertificates) == 0 {
				err = errors.New("client: no peer certificates")
			}
		}
		errCh <- err
	}()

	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("TLS handshake failed: %v", err)
		}
	}
}

// --- Cloud mode tests -------------------------------------------------------

type fakeCloudProvider struct {
	rootCert *x509.Certificate
	rootKey  ed25519.PrivateKey

	provisionRootCalls int
	issuePeerCertCalls int
	provisionErr       error
	issuePeerErr       error
}

func newFakeCloudProvider(t *testing.T) *fakeCloudProvider {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, _ := randSerial()
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "Cloud Federation Root",
			Organization: []string{"Kaivue Cloud"},
		},
		NotBefore:             now.Add(-clockSkew),
		NotAfter:              now.Add(rootValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeCloudProvider{rootCert: cert, rootKey: priv}
}

func (f *fakeCloudProvider) ProvisionRoot(_ context.Context, _ string) (*x509.Certificate, ed25519.PrivateKey, error) {
	f.provisionRootCalls++
	if f.provisionErr != nil {
		return nil, nil, f.provisionErr
	}
	return f.rootCert, f.rootKey, nil
}

func (f *fakeCloudProvider) IssuePeerCert(_ context.Context, csr *x509.CertificateRequest) (*x509.Certificate, error) {
	f.issuePeerCertCalls++
	if f.issuePeerErr != nil {
		return nil, f.issuePeerErr
	}
	serial, _ := randSerial()
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               csr.Subject,
		DNSNames:              csr.DNSNames,
		NotBefore:             now.Add(-clockSkew),
		NotAfter:              now.Add(peerValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, f.rootCert, csr.PublicKey, f.rootKey)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(der)
}

func (f *fakeCloudProvider) RootPool(_ context.Context) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	pool.AddCert(f.rootCert)
	return pool, nil
}

func TestCloudModeBootstrap(t *testing.T) {
	cloud := newFakeCloudProvider(t)
	dir := t.TempDir()

	ca, err := New(Config{
		StateDir:      dir,
		MasterKey:     testMaster(),
		SiteID:        "cloud-site-001",
		Mode:          ModeCloud,
		CloudProvider: cloud,
		Logf:          func(string, ...any) {},
	})
	if err != nil {
		t.Fatalf("New cloud mode: %v", err)
	}
	defer ca.Shutdown(context.Background())

	if cloud.provisionRootCalls != 1 {
		t.Errorf("expected 1 ProvisionRoot call, got %d", cloud.provisionRootCalls)
	}
	if ca.Fingerprint() == "" {
		t.Error("fingerprint empty after cloud bootstrap")
	}
	if ca.RootCert().Subject.CommonName != "Cloud Federation Root" {
		t.Errorf("CN = %q, expected cloud root CN", ca.RootCert().Subject.CommonName)
	}
}

func TestCloudModeRejectsNilProvider(t *testing.T) {
	dir := t.TempDir()
	_, err := New(Config{
		StateDir:  dir,
		MasterKey: testMaster(),
		SiteID:    "test",
		Mode:      ModeCloud,
	})
	if err == nil {
		t.Fatal("expected error when CloudProvider is nil in cloud mode")
	}
}

func TestCloudModeIssuePeerCertDelegatesToProvider(t *testing.T) {
	cloud := newFakeCloudProvider(t)
	dir := t.TempDir()

	ca, err := New(Config{
		StateDir:      dir,
		MasterKey:     testMaster(),
		SiteID:        "cloud-site-001",
		Mode:          ModeCloud,
		CloudProvider: cloud,
		Logf:          func(string, ...any) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ca.Shutdown(context.Background())

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	csrTmpl := &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: "peer-cloud"},
		DNSNames: []string{"peer-cloud.kaivue.local"},
	}
	csrDER, _ := x509.CreateCertificateRequest(rand.Reader, csrTmpl, priv)
	csr, _ := x509.ParseCertificateRequest(csrDER)

	leaf, err := ca.IssuePeerCert(context.Background(), csr, 0)
	if err != nil {
		t.Fatalf("IssuePeerCert: %v", err)
	}
	if cloud.issuePeerCertCalls != 1 {
		t.Errorf("expected 1 IssuePeerCert cloud call, got %d", cloud.issuePeerCertCalls)
	}
	if leaf.Subject.CommonName != "peer-cloud" {
		t.Errorf("CN = %q", leaf.Subject.CommonName)
	}
}

// --- Security tests --------------------------------------------------------

func TestTamperedRootKeyFailsOnReload(t *testing.T) {
	dir := t.TempDir()
	master := testMaster()
	ca, err := New(Config{StateDir: dir, MasterKey: master, SiteID: "test", Mode: ModeAirGapped})
	if err != nil {
		t.Fatal(err)
	}
	_ = ca.Shutdown(context.Background())

	// Corrupt the sealed key.
	keyPath := filepath.Join(dir, fileRootKey)
	raw, _ := os.ReadFile(keyPath)
	block, _ := pem.Decode(raw)
	if block == nil || len(block.Bytes) < 20 {
		t.Fatal("cannot decode sealed key PEM")
	}
	block.Bytes[len(block.Bytes)-5] ^= 0xFF
	os.WriteFile(keyPath, pem.EncodeToMemory(block), 0o600)

	_, err = New(Config{StateDir: dir, MasterKey: master, SiteID: "test", Mode: ModeAirGapped})
	if err == nil {
		t.Fatal("expected error on tampered sealed root key")
	}
	if !strings.Contains(err.Error(), "decrypt") && !errors.Is(err, cryptostore.ErrAuthFailed) {
		t.Errorf("expected decrypt/auth-failure error, got: %v", err)
	}
}

func TestWrongMasterKeyFailsToLoad(t *testing.T) {
	dir := t.TempDir()
	_, err := New(Config{StateDir: dir, MasterKey: testMaster(), SiteID: "test", Mode: ModeAirGapped})
	if err != nil {
		t.Fatal(err)
	}

	wrong := []byte("WRONGMASTERKEY____________________xx")
	_, err = New(Config{StateDir: dir, MasterKey: wrong, SiteID: "test", Mode: ModeAirGapped})
	if err == nil {
		t.Fatal("expected error loading with wrong master key")
	}
}

func TestShutdownZeroesRootKey(t *testing.T) {
	ca, _ := newTestCA(t)
	_ = ca.Shutdown(context.Background())
	if ca.rootKey != nil || ca.rootCert != nil {
		t.Error("shutdown left rootKey or rootCert populated")
	}
}

func TestCryptostoreInjectionSkipsMasterKey(t *testing.T) {
	dir := t.TempDir()
	cs, err := cryptostore.NewFromMaster(testMaster(), nil, CryptostoreInfoLabel)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := New(Config{StateDir: dir, Cryptostore: cs, SiteID: "test", Mode: ModeAirGapped})
	if err != nil {
		t.Fatalf("New with injected cryptostore: %v", err)
	}
	defer ca.Shutdown(context.Background())

	if ca.Fingerprint() == "" {
		t.Error("fingerprint empty after injected cryptostore bootstrap")
	}
}

// --- PeerTLSConfig test ----------------------------------------------------

func TestPeerTLSConfigEnforcesTLS13(t *testing.T) {
	ca, _ := newTestCA(t)
	defer ca.Shutdown(context.Background())

	tlsConf, err := ca.PeerTLSConfig()
	if err != nil {
		t.Fatalf("PeerTLSConfig: %v", err)
	}
	if tlsConf.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = %d, want TLS 1.3 (%d)", tlsConf.MinVersion, tls.VersionTLS13)
	}
	if tlsConf.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("ClientAuth = %d, want RequireAndVerifyClientCert", tlsConf.ClientAuth)
	}
	if len(tlsConf.Certificates) == 0 {
		t.Error("no certificates in TLS config")
	}
}

// --- Cryptostore label separation test -------------------------------------

func TestCryptostoreLabelSeparation(t *testing.T) {
	// Verify that the federation CA uses a different HKDF label than the
	// cluster CA, ensuring key separation.
	if CryptostoreInfoLabel == cryptostore.InfoFederationRoot {
		t.Errorf("federation CA label %q must differ from cluster CA label %q",
			CryptostoreInfoLabel, cryptostore.InfoFederationRoot)
	}
}

// --- helpers ----------------------------------------------------------------

func issueTestPeerLeaf(t *testing.T, ca *FederationCA, dns string) (*x509.Certificate, ed25519.PrivateKey) {
	t.Helper()
	return issueTestPeerLeafTTL(t, ca, dns, 1*time.Hour)
}

func issueTestPeerLeafTTL(t *testing.T, ca *FederationCA, dns string, ttl time.Duration) (*x509.Certificate, ed25519.PrivateKey) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: dns},
		DNSNames: []string{dns},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, tmpl, priv)
	if err != nil {
		t.Fatal(err)
	}
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := ca.IssuePeerCert(context.Background(), csr, ttl)
	if err != nil {
		t.Fatalf("IssuePeerCert: %v", err)
	}
	return leaf, priv
}
