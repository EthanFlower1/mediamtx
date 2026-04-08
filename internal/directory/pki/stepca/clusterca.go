package stepca

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/shared/cryptostore"
)

// PEM block types used on disk.
const (
	pemTypeCertificate       = "CERTIFICATE"
	pemTypeEncryptedPrivKey  = "KAIVUE ENCRYPTED PRIVATE KEY"
	pemTypeUnencryptedPrivKey = "PRIVATE KEY"
)

// File names used inside StateDir.
const (
	fileRootCert      = "root.crt"
	fileRootKey       = "root.key.enc"
	fileDirLeafCert   = "directory.crt"
	fileDirLeafKey    = "directory.key.enc"
	fileLeafArchive   = "leaves.archive.enc"
	subjectCommonName = "Kaivue Site Root CA"
)

// Default validity windows.
const (
	rootValidity = 10 * 365 * 24 * time.Hour // ~10 years
	leafValidity = 24 * time.Hour
	clockSkew    = 5 * time.Minute
)

// Logf is a minimal printf-style logger injected by callers. Avoids a hard
// dep on *slog.Logger in this leaf package; callers will typically pass
// logger.Info or a closure wrapping one.
type Logf func(format string, args ...any)

// Config parameterizes a ClusterCA.
type Config struct {
	// StateDir holds the root cert, encrypted root key, leaf artifacts and
	// the encrypted leaf archive. Files are created with 0600 permissions.
	// Default: /var/lib/mediamtx-directory/pki/
	StateDir string

	// MasterKey is the installation master key (nvrJWTSecret). Used solely
	// to derive the cryptostore subkey for sealing the root private key at
	// rest. This package never persists the master key itself.
	MasterKey []byte

	// Cryptostore is an optional pre-built cryptostore bound to the
	// InfoFederationRoot label. When nil, New constructs one from
	// MasterKey. Tests inject this directly to decouple from MasterKey.
	Cryptostore cryptostore.Cryptostore

	// Logf, if non-nil, receives progress messages during bootstrap.
	Logf Logf

	// Clock overrides time.Now for tests. Zero value = time.Now.
	Clock func() time.Time
}

// ClusterCA is the embedded per-site cluster Certificate Authority. It is
// safe for concurrent use after New returns.
type ClusterCA struct {
	cfg     Config
	dir     string
	log     Logf
	now     func() time.Time
	crypto  cryptostore.Cryptostore

	mu          sync.RWMutex
	rootCert    *x509.Certificate
	rootCertPEM []byte
	rootKey     ed25519.PrivateKey
	rootPool    *x509.CertPool
	fingerprint string

	dirLeaf *tls.Certificate
}

// New loads an existing CA from cfg.StateDir or, if none is present, invokes
// Bootstrap to create one.
func New(cfg Config) (*ClusterCA, error) {
	if cfg.StateDir == "" {
		cfg.StateDir = "/var/lib/mediamtx-directory/pki"
	}
	logf := cfg.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	now := cfg.Clock
	if now == nil {
		now = time.Now
	}

	cs := cfg.Cryptostore
	if cs == nil {
		if len(cfg.MasterKey) == 0 {
			return nil, fmt.Errorf("stepca: MasterKey or Cryptostore must be provided")
		}
		var err error
		cs, err = cryptostore.NewFromMaster(cfg.MasterKey, nil, cryptostore.InfoFederationRoot)
		if err != nil {
			return nil, fmt.Errorf("stepca: derive cryptostore: %w", err)
		}
	}

	ca := &ClusterCA{
		cfg:    cfg,
		dir:    cfg.StateDir,
		log:    logf,
		now:    now,
		crypto: cs,
	}

	if err := os.MkdirAll(ca.dir, 0o700); err != nil {
		return nil, fmt.Errorf("stepca: mkdir state dir: %w", err)
	}

	rootCertPath := filepath.Join(ca.dir, fileRootCert)
	rootKeyPath := filepath.Join(ca.dir, fileRootKey)

	_, certErr := os.Stat(rootCertPath)
	_, keyErr := os.Stat(rootKeyPath)
	switch {
	case certErr == nil && keyErr == nil:
		if err := ca.load(rootCertPath, rootKeyPath); err != nil {
			return nil, err
		}
		logf("stepca: loaded existing cluster root fingerprint=%s", ca.fingerprint)
	case errors.Is(certErr, os.ErrNotExist) && errors.Is(keyErr, os.ErrNotExist):
		if err := ca.Bootstrap(context.Background()); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("stepca: inconsistent state dir: cert err=%v key err=%v", certErr, keyErr)
	}

	// Ensure the Directory's own serving leaf exists / is fresh.
	if _, err := ca.IssueDirectoryServingCert(context.Background()); err != nil {
		return nil, fmt.Errorf("stepca: issue directory serving cert: %w", err)
	}
	return ca, nil
}

// Bootstrap generates a fresh root on an empty state dir. It is safe to
// call manually but New invokes it automatically on first start.
func (c *ClusterCA) Bootstrap(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.log("stepca: bootstrapping new cluster root CA in %s", c.dir)

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("stepca: generate root key: %w", err)
	}

	serial, err := randSerial()
	if err != nil {
		return fmt.Errorf("stepca: root serial: %w", err)
	}

	now := c.now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   subjectCommonName,
			Organization: []string{"Kaivue"},
		},
		NotBefore:             now.Add(-clockSkew),
		NotAfter:              now.Add(rootValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
		MaxPathLenZero:        false,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		return fmt.Errorf("stepca: self-sign root: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return fmt.Errorf("stepca: parse root: %w", err)
	}

	if err := c.installRoot(cert, priv); err != nil {
		return err
	}

	certPath := filepath.Join(c.dir, fileRootCert)
	keyPath := filepath.Join(c.dir, fileRootKey)

	if err := writePEMFile(certPath, pemTypeCertificate, der); err != nil {
		return fmt.Errorf("stepca: persist root cert: %w", err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("stepca: marshal root key: %w", err)
	}
	sealed, err := c.crypto.Encrypt(keyDER)
	// zero plaintext buffer
	for i := range keyDER {
		keyDER[i] = 0
	}
	if err != nil {
		return fmt.Errorf("stepca: seal root key: %w", err)
	}
	if err := writePEMFile(keyPath, pemTypeEncryptedPrivKey, sealed); err != nil {
		return fmt.Errorf("stepca: persist root key: %w", err)
	}

	c.log("stepca: root bootstrapped fingerprint=%s notAfter=%s", c.fingerprint, cert.NotAfter.Format(time.RFC3339))
	return nil
}

// load reads root cert + sealed key from disk.
func (c *ClusterCA) load(certPath, keyPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("stepca: read root cert: %w", err)
	}
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil || certBlock.Type != pemTypeCertificate {
		return fmt.Errorf("stepca: malformed root cert PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return fmt.Errorf("stepca: parse root cert: %w", err)
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("stepca: read root key: %w", err)
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil || keyBlock.Type != pemTypeEncryptedPrivKey {
		return fmt.Errorf("stepca: malformed root key PEM")
	}
	opened, err := c.crypto.Decrypt(keyBlock.Bytes)
	if err != nil {
		return fmt.Errorf("stepca: decrypt root key: %w", err)
	}
	defer func() {
		for i := range opened {
			opened[i] = 0
		}
	}()
	rawKey, err := x509.ParsePKCS8PrivateKey(opened)
	if err != nil {
		return fmt.Errorf("stepca: parse root key: %w", err)
	}
	edKey, ok := rawKey.(ed25519.PrivateKey)
	if !ok {
		return fmt.Errorf("stepca: root key is not ed25519 (got %T)", rawKey)
	}
	return c.installRootLocked(cert, edKey)
}

func (c *ClusterCA) installRoot(cert *x509.Certificate, key ed25519.PrivateKey) error {
	// caller already holds c.mu
	return c.installRootLocked(cert, key)
}

func (c *ClusterCA) installRootLocked(cert *x509.Certificate, key ed25519.PrivateKey) error {
	c.rootCert = cert
	c.rootKey = key
	c.rootCertPEM = pem.EncodeToMemory(&pem.Block{Type: pemTypeCertificate, Bytes: cert.Raw})
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	c.rootPool = pool
	sum := sha256.Sum256(cert.Raw)
	c.fingerprint = hex.EncodeToString(sum[:])
	return nil
}

// IssueLeaf signs csr with the root and returns a leaf valid for ttl.
// If ttl is zero or exceeds the default leaf validity it is clamped to
// leafValidity. The leaf is persisted to the state dir under
// leaves/<serial>.crt for audit (best-effort; errors are logged but not
// returned — audit is not on the critical path of cert issuance).
func (c *ClusterCA) IssueLeaf(ctx context.Context, csr *x509.CertificateRequest, ttl time.Duration) (*x509.Certificate, error) {
	if csr == nil {
		return nil, errors.New("stepca: nil CSR")
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("stepca: CSR signature: %w", err)
	}
	if ttl <= 0 || ttl > leafValidity {
		ttl = leafValidity
	}

	c.mu.RLock()
	root := c.rootCert
	key := c.rootKey
	c.mu.RUnlock()
	if root == nil || key == nil {
		return nil, errors.New("stepca: CA not initialized")
	}

	serial, err := randSerial()
	if err != nil {
		return nil, fmt.Errorf("stepca: leaf serial: %w", err)
	}
	now := c.now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber:   serial,
		Subject:        csr.Subject,
		DNSNames:       csr.DNSNames,
		IPAddresses:    csr.IPAddresses,
		URIs:           csr.URIs,
		EmailAddresses: csr.EmailAddresses,
		NotBefore:      now.Add(-clockSkew),
		NotAfter:       now.Add(ttl),
		KeyUsage:       x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:    []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, root, csr.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("stepca: sign leaf: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("stepca: parse leaf: %w", err)
	}

	// Audit-only persistence.
	auditDir := filepath.Join(c.dir, "leaves")
	if err := os.MkdirAll(auditDir, 0o700); err == nil {
		path := filepath.Join(auditDir, fmt.Sprintf("%s.crt", serial.Text(16)))
		if werr := writePEMFile(path, pemTypeCertificate, der); werr != nil {
			c.log("stepca: audit persist leaf failed: %v", werr)
		}
	}
	return leaf, nil
}

// IssueDirectoryServingCert issues (or reuses if still valid) the Directory's
// own TLS listening certificate. The leaf CN is directory-<uuid> and the
// cert is saved unencrypted alongside the sealed private key.
func (c *ClusterCA) IssueDirectoryServingCert(ctx context.Context) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	certPath := filepath.Join(c.dir, fileDirLeafCert)
	keyPath := filepath.Join(c.dir, fileDirLeafKey)

	// Fast path: reuse existing leaf if present and still valid for >1h.
	if c.dirLeaf == nil {
		if leaf, err := c.loadDirectoryLeafLocked(certPath, keyPath); err == nil && leaf != nil {
			c.dirLeaf = leaf
		}
	}
	if c.dirLeaf != nil && c.dirLeaf.Leaf != nil &&
		c.now().Add(1*time.Hour).Before(c.dirLeaf.Leaf.NotAfter) {
		return c.dirLeaf, nil
	}

	// Generate a new ed25519 keypair for the Directory leaf.
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("stepca: directory leaf keygen: %w", err)
	}
	serial, err := randSerial()
	if err != nil {
		return nil, err
	}
	cn := "directory-" + uuid.NewString()
	now := c.now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn, Organization: []string{"Kaivue"}},
		DNSNames:              []string{cn, "directory.kaivue.local", "localhost"},
		NotBefore:             now.Add(-clockSkew),
		NotAfter:              now.Add(leafValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.rootCert, pub, c.rootKey)
	if err != nil {
		return nil, fmt.Errorf("stepca: sign directory leaf: %w", err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}

	// Persist cert (plaintext) + key (sealed).
	if err := writePEMFile(certPath, pemTypeCertificate, der); err != nil {
		return nil, fmt.Errorf("stepca: persist directory cert: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}
	sealed, err := c.crypto.Encrypt(keyDER)
	for i := range keyDER {
		keyDER[i] = 0
	}
	if err != nil {
		return nil, fmt.Errorf("stepca: seal directory key: %w", err)
	}
	if err := writePEMFile(keyPath, pemTypeEncryptedPrivKey, sealed); err != nil {
		return nil, fmt.Errorf("stepca: persist directory key: %w", err)
	}

	tlsCert := &tls.Certificate{
		Certificate: [][]byte{der, c.rootCert.Raw},
		PrivateKey:  priv,
		Leaf:        parsed,
	}
	c.dirLeaf = tlsCert
	return tlsCert, nil
}

// loadDirectoryLeafLocked rebuilds a tls.Certificate from disk.
// Called with c.mu held.
func (c *ClusterCA) loadDirectoryLeafLocked(certPath, keyPath string) (*tls.Certificate, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	cb, _ := pem.Decode(certPEM)
	if cb == nil || cb.Type != pemTypeCertificate {
		return nil, errors.New("stepca: malformed directory cert")
	}
	parsed, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, err
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	kb, _ := pem.Decode(keyPEM)
	if kb == nil || kb.Type != pemTypeEncryptedPrivKey {
		return nil, errors.New("stepca: malformed directory key")
	}
	opened, err := c.crypto.Decrypt(kb.Bytes)
	if err != nil {
		return nil, err
	}
	defer func() {
		for i := range opened {
			opened[i] = 0
		}
	}()
	raw, err := x509.ParsePKCS8PrivateKey(opened)
	if err != nil {
		return nil, err
	}
	priv, ok := raw.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("stepca: directory key not ed25519 (got %T)", raw)
	}
	return &tls.Certificate{
		Certificate: [][]byte{cb.Bytes, c.rootCert.Raw},
		PrivateKey:  priv,
		Leaf:        parsed,
	}, nil
}

// Fingerprint returns the lowercase hex SHA-256 digest of the DER encoding
// of the root certificate. This is what gets embedded in pairing tokens so
// clients can trust-on-first-use the site root.
func (c *ClusterCA) Fingerprint() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.fingerprint
}

// RootPEM returns the root certificate in PEM form, suitable for installing
// in client trust stores.
func (c *ClusterCA) RootPEM() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]byte, len(c.rootCertPEM))
	copy(out, c.rootCertPEM)
	return out
}

// RootPool returns a CertPool containing just the root. Useful for
// x509.Verify calls inside the Directory.
func (c *ClusterCA) RootPool() *x509.CertPool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	// Pool has no public clone — return the shared instance; callers must
	// treat it as read-only.
	return c.rootPool
}

// ArchiveLeaf appends an encrypted, length-prefixed record of cert.Raw to
// the leaf archive file. Used by KAI-242's rotation flow as a secure-delete
// seam: old leaves are moved into the archive instead of being left on
// disk as plaintext PEMs.
func (c *ClusterCA) ArchiveLeaf(ctx context.Context, cert *x509.Certificate) error {
	if cert == nil || len(cert.Raw) == 0 {
		return errors.New("stepca: nil cert")
	}
	sealed, err := c.crypto.Encrypt(cert.Raw)
	if err != nil {
		return fmt.Errorf("stepca: seal archived leaf: %w", err)
	}
	path := filepath.Join(c.dir, fileLeafArchive)

	c.mu.Lock()
	defer c.mu.Unlock()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("stepca: open archive: %w", err)
	}
	defer f.Close()

	// Length prefix: 4-byte big-endian.
	var lenBuf [4]byte
	n := uint32(len(sealed))
	lenBuf[0] = byte(n >> 24)
	lenBuf[1] = byte(n >> 16)
	lenBuf[2] = byte(n >> 8)
	lenBuf[3] = byte(n)
	if _, err := f.Write(lenBuf[:]); err != nil {
		return err
	}
	if _, err := f.Write(sealed); err != nil {
		return err
	}

	// Best-effort remove of the plaintext audit copy so the archive is the
	// sole record after rotation.
	plaintextPath := filepath.Join(c.dir, "leaves", fmt.Sprintf("%s.crt", cert.SerialNumber.Text(16)))
	_ = os.Remove(plaintextPath)
	return nil
}

// Shutdown zeroes in-memory key material. The returned error is reserved
// for future integrations that may need to flush state.
func (c *ClusterCA) Shutdown(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.rootKey {
		c.rootKey[i] = 0
	}
	c.rootKey = nil
	c.rootCert = nil
	c.rootCertPEM = nil
	c.rootPool = nil
	c.dirLeaf = nil
	return nil
}

// --- helpers --------------------------------------------------------------

func randSerial() (*big.Int, error) {
	max := new(big.Int).Lsh(big.NewInt(1), 159) // 159-bit positive serial
	return rand.Int(rand.Reader, max)
}

func writePEMFile(path, blockType string, der []byte) error {
	buf := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// Write atomically via temp file + rename so partial writes never leave
	// a half-baked PEM on disk.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".pem-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if _, err := tmp.Write(buf); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// Ensure crypto.Signer assertion holds at compile time for ed25519 private
// keys — x509.CreateCertificate panics otherwise, and this is cheap insurance.
var _ crypto.Signer = ed25519.PrivateKey(nil)
