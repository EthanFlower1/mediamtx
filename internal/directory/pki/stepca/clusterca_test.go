package stepca

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
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

// newTestCA constructs a ClusterCA in t.TempDir with a stub logger.
func newTestCA(t *testing.T) (*ClusterCA, string) {
	t.Helper()
	dir := t.TempDir()
	ca, err := New(Config{
		StateDir:  dir,
		MasterKey: testMaster(),
		Logf:      func(string, ...any) {},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return ca, dir
}

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

	// Sealed key must not contain "PRIVATE KEY" ascii of plaintext PKCS8 header.
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(keyBytes), pemTypeEncryptedPrivKey) {
		t.Errorf("expected sealed PEM type %q in file, got:\n%s", pemTypeEncryptedPrivKey, keyBytes)
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
	if cert.Subject.CommonName != subjectCommonName {
		t.Errorf("CN = %q, want %q", cert.Subject.CommonName, subjectCommonName)
	}
	if !cert.IsCA {
		t.Errorf("root cert IsCA = false")
	}
}

func TestBootstrapIsIdempotentAcrossReload(t *testing.T) {
	dir := t.TempDir()
	master := testMaster()
	ca1, err := New(Config{StateDir: dir, MasterKey: master})
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	fp1 := ca1.Fingerprint()
	rootPEM1 := ca1.RootPEM()
	_ = ca1.Shutdown(context.Background())

	ca2, err := New(Config{StateDir: dir, MasterKey: master})
	if err != nil {
		t.Fatalf("reload New: %v", err)
	}
	defer ca2.Shutdown(context.Background())

	if ca2.Fingerprint() != fp1 {
		t.Errorf("fingerprint changed on reload: %s -> %s", fp1, ca2.Fingerprint())
	}
	if string(ca2.RootPEM()) != string(rootPEM1) {
		t.Errorf("root PEM changed on reload")
	}
}

func TestFingerprintStableAcrossReloads(t *testing.T) {
	dir := t.TempDir()
	master := testMaster()
	fingerprints := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		ca, err := New(Config{StateDir: dir, MasterKey: master})
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

func TestIssueLeafSignedByRoot(t *testing.T) {
	ca, _ := newTestCA(t)
	defer ca.Shutdown(context.Background())

	leaf, leafKey := issueTestLeaf(t, ca, "recorder-01.kaivue.local")
	_ = leafKey

	// Signed by the root we know about.
	if err := leaf.CheckSignatureFrom(ca.rootCert); err != nil {
		t.Fatalf("leaf not signed by root: %v", err)
	}
}

func TestIssueLeafVerifiesAgainstRootPool(t *testing.T) {
	ca, _ := newTestCA(t)
	defer ca.Shutdown(context.Background())

	leaf, _ := issueTestLeaf(t, ca, "gateway-01.kaivue.local")

	opts := x509.VerifyOptions{
		Roots:     ca.RootPool(),
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSName:   "gateway-01.kaivue.local",
	}
	if _, err := leaf.Verify(opts); err != nil {
		t.Fatalf("leaf verify: %v", err)
	}
}

func TestIssueLeafClampsTTL(t *testing.T) {
	ca, _ := newTestCA(t)
	defer ca.Shutdown(context.Background())

	leaf, _ := issueTestLeafTTL(t, ca, "node.local", 365*24*time.Hour)
	windowSeconds := leaf.NotAfter.Sub(leaf.NotBefore).Seconds()
	want := (leafValidity + clockSkew).Seconds()
	if windowSeconds > want+1 {
		t.Errorf("leaf validity window = %.0fs, expected clamped to ~%.0fs", windowSeconds, want)
	}
}

func TestIssueLeafRejectsNilCSR(t *testing.T) {
	ca, _ := newTestCA(t)
	defer ca.Shutdown(context.Background())

	if _, err := ca.IssueLeaf(context.Background(), nil, 0); err == nil {
		t.Fatal("expected error on nil CSR")
	}
}

func TestDirectoryServingCertChainsToRoot(t *testing.T) {
	ca, _ := newTestCA(t)
	defer ca.Shutdown(context.Background())

	tlsCert, err := ca.IssueDirectoryServingCert(context.Background())
	if err != nil {
		t.Fatalf("IssueDirectoryServingCert: %v", err)
	}
	if tlsCert.Leaf == nil {
		t.Fatal("tlsCert.Leaf nil")
	}
	if !strings.HasPrefix(tlsCert.Leaf.Subject.CommonName, "directory-") {
		t.Errorf("CN = %q, want directory-*", tlsCert.Leaf.Subject.CommonName)
	}

	opts := x509.VerifyOptions{
		Roots: ca.RootPool(),
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if _, err := tlsCert.Leaf.Verify(opts); err != nil {
		t.Fatalf("directory leaf verify: %v", err)
	}
}

func TestTamperedRootKeyFailsOnReload(t *testing.T) {
	dir := t.TempDir()
	master := testMaster()
	ca, err := New(Config{StateDir: dir, MasterKey: master})
	if err != nil {
		t.Fatalf("initial New: %v", err)
	}
	_ = ca.Shutdown(context.Background())

	// Flip a byte inside the sealed key's GCM ciphertext body.
	keyPath := filepath.Join(dir, fileRootKey)
	raw, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		t.Fatal("cannot decode sealed key PEM")
	}
	// Corrupt a byte inside the GCM payload (after the 13-byte cryptostore header).
	if len(block.Bytes) < 20 {
		t.Fatalf("sealed blob too short: %d", len(block.Bytes))
	}
	block.Bytes[len(block.Bytes)-5] ^= 0xFF
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}

	// Reload must fail on decrypt.
	_, err = New(Config{StateDir: dir, MasterKey: master})
	if err == nil {
		t.Fatal("expected error on tampered sealed root key")
	}
	if !strings.Contains(err.Error(), "decrypt") && !errors.Is(err, cryptostore.ErrAuthFailed) {
		t.Errorf("expected decrypt/auth-failure error, got: %v", err)
	}
}

func TestWrongMasterKeyFailsToLoad(t *testing.T) {
	dir := t.TempDir()
	_, err := New(Config{StateDir: dir, MasterKey: testMaster()})
	if err != nil {
		t.Fatal(err)
	}

	// Load with a different master key.
	wrong := []byte("WRONGMASTERKEY____________________xx")
	_, err = New(Config{StateDir: dir, MasterKey: wrong})
	if err == nil {
		t.Fatal("expected error loading with wrong master key")
	}
}

func TestArchiveLeafAppendsEncryptedRecord(t *testing.T) {
	ca, dir := newTestCA(t)
	defer ca.Shutdown(context.Background())

	// Issue a leaf to have something to archive.
	leaf, _ := issueTestLeaf(t, ca, "archive-target.local")

	if err := ca.ArchiveLeaf(context.Background(), leaf); err != nil {
		t.Fatalf("ArchiveLeaf: %v", err)
	}
	archivePath := filepath.Join(dir, fileLeafArchive)
	info, err := os.Stat(archivePath)
	if err != nil {
		t.Fatalf("archive missing: %v", err)
	}
	if info.Size() < int64(len(leaf.Raw)) {
		t.Errorf("archive size %d smaller than expected", info.Size())
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("archive perms = %o, want 0600", perm)
	}

	// Second archive call should grow the file (append semantics).
	before := info.Size()
	if err := ca.ArchiveLeaf(context.Background(), leaf); err != nil {
		t.Fatalf("ArchiveLeaf #2: %v", err)
	}
	info2, _ := os.Stat(archivePath)
	if info2.Size() <= before {
		t.Errorf("archive did not grow: %d -> %d", before, info2.Size())
	}

	// Plaintext audit copy should have been removed.
	pt := filepath.Join(dir, "leaves", leaf.SerialNumber.Text(16)+".crt")
	if _, err := os.Stat(pt); !os.IsNotExist(err) {
		t.Errorf("expected plaintext audit leaf to be removed, stat err = %v", err)
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
		t.Errorf("RootPEM returned aliased buffer — caller mutation affected internal state")
	}
}

func TestCryptostoreInjectionSkipsMasterKey(t *testing.T) {
	dir := t.TempDir()
	cs, err := cryptostore.NewFromMaster(testMaster(), nil, cryptostore.InfoFederationRoot)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := New(Config{StateDir: dir, Cryptostore: cs})
	if err != nil {
		t.Fatalf("New with injected cryptostore: %v", err)
	}
	defer ca.Shutdown(context.Background())

	if ca.Fingerprint() == "" {
		t.Error("fingerprint empty after injected cryptostore bootstrap")
	}
}

func TestShutdownZeroesRootKey(t *testing.T) {
	ca, _ := newTestCA(t)
	_ = ca.Shutdown(context.Background())
	if ca.rootKey != nil || ca.rootCert != nil {
		t.Errorf("shutdown left rootKey or rootCert populated")
	}
}

// --- helpers --------------------------------------------------------------

func issueTestLeaf(t *testing.T, ca *ClusterCA, dns string) (*x509.Certificate, ed25519.PrivateKey) {
	t.Helper()
	return issueTestLeafTTL(t, ca, dns, 1*time.Hour)
}

func issueTestLeafTTL(t *testing.T, ca *ClusterCA, dns string, ttl time.Duration) (*x509.Certificate, ed25519.PrivateKey) {
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
	leaf, err := ca.IssueLeaf(context.Background(), csr, ttl)
	if err != nil {
		t.Fatalf("IssueLeaf: %v", err)
	}
	return leaf, priv
}
