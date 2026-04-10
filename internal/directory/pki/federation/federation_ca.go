package federation

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
	pemTypeCertificate      = "CERTIFICATE"
	pemTypeEncryptedPrivKey = "KAIVUE ENCRYPTED PRIVATE KEY"
)

// File names used inside StateDir.
const (
	fileRootCert    = "federation-root.crt"
	fileRootKey     = "federation-root.key.enc"
	filePeerCert    = "federation-peer.crt"
	filePeerKey     = "federation-peer.key.enc"
	subjectRootCN   = "Kaivue Federation Root CA"
	subjectPeerCN   = "federation-peer"
)

// Default validity windows.
const (
	rootValidity = 10 * 365 * 24 * time.Hour // ~10 years
	peerValidity = 24 * time.Hour
	clockSkew    = 5 * time.Minute
)

// CryptostoreInfoLabel is the HKDF info string for the federation CA key
// encryption. This is distinct from the cluster CA's label to ensure
// complete key separation.
const CryptostoreInfoLabel = "federation-ca-root"

// Mode selects between air-gapped and cloud-connected federation CA operation.
type Mode int

const (
	// ModeAirGapped means the founding Directory self-signs the federation root.
	ModeAirGapped Mode = iota
	// ModeCloud means a cloud identity service provisions federation credentials.
	ModeCloud
)

// Logf is a minimal printf-style logger injected by callers.
type Logf func(format string, args ...any)

// CloudCAProvider abstracts a cloud identity service that can provision
// federation credentials. This is the extension point for cloud-connected mode.
type CloudCAProvider interface {
	// ProvisionRoot requests the cloud to provision a federation root CA for
	// the given site. Returns the root cert and private key. The cloud may
	// act as an intermediate CA or as a full root CA depending on deployment.
	ProvisionRoot(ctx context.Context, siteID string) (*x509.Certificate, ed25519.PrivateKey, error)

	// IssuePeerCert asks the cloud to issue a leaf cert for a peer Directory.
	// The cloud may delegate this to the founding Directory or handle it centrally.
	IssuePeerCert(ctx context.Context, csr *x509.CertificateRequest) (*x509.Certificate, error)

	// RootPool returns the trust pool containing the cloud-provisioned root(s).
	RootPool(ctx context.Context) (*x509.CertPool, error)
}

// Config parameterizes a FederationCA.
type Config struct {
	// StateDir holds the federation root cert, encrypted root key, and peer
	// leaf artifacts. Files are created with 0600 permissions.
	StateDir string

	// MasterKey is the installation master key (nvrJWTSecret). Used to derive
	// the cryptostore subkey for sealing the federation root private key.
	MasterKey []byte

	// Cryptostore is an optional pre-built cryptostore. When nil, New
	// constructs one from MasterKey using CryptostoreInfoLabel.
	Cryptostore cryptostore.Cryptostore

	// Mode selects air-gapped vs cloud-connected operation.
	Mode Mode

	// SiteID identifies this site in the federation. Used as the subject
	// identifier in the peer enrollment token and cert CN suffix.
	SiteID string

	// CloudProvider is required when Mode == ModeCloud. Ignored in air-gapped mode.
	CloudProvider CloudCAProvider

	// Logf receives progress messages. Nil = discard.
	Logf Logf

	// Clock overrides time.Now for tests. Nil = time.Now.
	Clock func() time.Time
}

// FederationCA is the federation-domain Certificate Authority. It manages a
// separate PKI root from the per-site cluster CA, exclusively for Directory
// to Directory mTLS across federated sites.
//
// Safe for concurrent use after New returns.
type FederationCA struct {
	cfg    Config
	dir    string
	log    Logf
	now    func() time.Time
	cs     cryptostore.Cryptostore

	mu          sync.RWMutex
	rootCert    *x509.Certificate
	rootCertPEM []byte
	rootKey     ed25519.PrivateKey
	rootPool    *x509.CertPool
	fingerprint string

	peerLeaf *tls.Certificate
}

// New loads an existing federation CA from cfg.StateDir or, if none is
// present, bootstraps a new one.
func New(cfg Config) (*FederationCA, error) {
	if cfg.StateDir == "" {
		cfg.StateDir = "/var/lib/mediamtx-directory/pki/federation"
	}
	if cfg.SiteID == "" {
		cfg.SiteID = uuid.NewString()
	}
	logf := cfg.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	now := cfg.Clock
	if now == nil {
		now = time.Now
	}
	if cfg.Mode == ModeCloud && cfg.CloudProvider == nil {
		return nil, fmt.Errorf("federation: CloudProvider required in cloud mode")
	}

	cs := cfg.Cryptostore
	if cs == nil {
		if len(cfg.MasterKey) == 0 {
			return nil, fmt.Errorf("federation: MasterKey or Cryptostore must be provided")
		}
		var err error
		cs, err = cryptostore.NewFromMaster(cfg.MasterKey, nil, CryptostoreInfoLabel)
		if err != nil {
			return nil, fmt.Errorf("federation: derive cryptostore: %w", err)
		}
	}

	ca := &FederationCA{
		cfg: cfg,
		dir: cfg.StateDir,
		log: logf,
		now: now,
		cs:  cs,
	}

	if err := os.MkdirAll(ca.dir, 0o700); err != nil {
		return nil, fmt.Errorf("federation: mkdir state dir: %w", err)
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
		logf("federation: loaded existing federation root fingerprint=%s", ca.fingerprint)
	case errors.Is(certErr, os.ErrNotExist) && errors.Is(keyErr, os.ErrNotExist):
		if err := ca.bootstrap(context.Background()); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("federation: inconsistent state dir: cert err=%v key err=%v", certErr, keyErr)
	}

	// Ensure this Directory's own federation peer leaf exists and is fresh.
	if _, err := ca.IssueSelfPeerCert(context.Background()); err != nil {
		return nil, fmt.Errorf("federation: issue self peer cert: %w", err)
	}

	return ca, nil
}

// bootstrap creates the federation root CA. In air-gapped mode this is a
// self-signed Ed25519 root. In cloud mode it delegates to the CloudProvider.
func (c *FederationCA) bootstrap(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.cfg.Mode {
	case ModeAirGapped:
		return c.bootstrapAirGapped(ctx)
	case ModeCloud:
		return c.bootstrapCloud(ctx)
	default:
		return fmt.Errorf("federation: unknown mode %d", c.cfg.Mode)
	}
}

func (c *FederationCA) bootstrapAirGapped(_ context.Context) error {
	c.log("federation: bootstrapping air-gapped federation root CA in %s", c.dir)

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("federation: generate root key: %w", err)
	}

	serial, err := randSerial()
	if err != nil {
		return fmt.Errorf("federation: root serial: %w", err)
	}

	now := c.now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   subjectRootCN,
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
		return fmt.Errorf("federation: self-sign root: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return fmt.Errorf("federation: parse root: %w", err)
	}

	c.installRootLocked(cert, priv)

	if err := c.persistRoot(cert.Raw, priv); err != nil {
		return err
	}

	c.log("federation: root bootstrapped fingerprint=%s notAfter=%s",
		c.fingerprint, cert.NotAfter.Format(time.RFC3339))
	return nil
}

func (c *FederationCA) bootstrapCloud(ctx context.Context) error {
	c.log("federation: bootstrapping cloud-provisioned federation root CA")

	cert, priv, err := c.cfg.CloudProvider.ProvisionRoot(ctx, c.cfg.SiteID)
	if err != nil {
		return fmt.Errorf("federation: cloud provision root: %w", err)
	}

	c.installRootLocked(cert, priv)

	if err := c.persistRoot(cert.Raw, priv); err != nil {
		return err
	}

	c.log("federation: cloud root provisioned fingerprint=%s notAfter=%s",
		c.fingerprint, cert.NotAfter.Format(time.RFC3339))
	return nil
}

func (c *FederationCA) persistRoot(certDER []byte, priv ed25519.PrivateKey) error {
	certPath := filepath.Join(c.dir, fileRootCert)
	keyPath := filepath.Join(c.dir, fileRootKey)

	if err := writePEMFile(certPath, pemTypeCertificate, certDER); err != nil {
		return fmt.Errorf("federation: persist root cert: %w", err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("federation: marshal root key: %w", err)
	}
	sealed, err := c.cs.Encrypt(keyDER)
	for i := range keyDER {
		keyDER[i] = 0
	}
	if err != nil {
		return fmt.Errorf("federation: seal root key: %w", err)
	}
	if err := writePEMFile(keyPath, pemTypeEncryptedPrivKey, sealed); err != nil {
		return fmt.Errorf("federation: persist root key: %w", err)
	}
	return nil
}

func (c *FederationCA) load(certPath, keyPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("federation: read root cert: %w", err)
	}
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil || certBlock.Type != pemTypeCertificate {
		return fmt.Errorf("federation: malformed root cert PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return fmt.Errorf("federation: parse root cert: %w", err)
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("federation: read root key: %w", err)
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil || keyBlock.Type != pemTypeEncryptedPrivKey {
		return fmt.Errorf("federation: malformed root key PEM")
	}
	opened, err := c.cs.Decrypt(keyBlock.Bytes)
	if err != nil {
		return fmt.Errorf("federation: decrypt root key: %w", err)
	}
	defer func() {
		for i := range opened {
			opened[i] = 0
		}
	}()
	rawKey, err := x509.ParsePKCS8PrivateKey(opened)
	if err != nil {
		return fmt.Errorf("federation: parse root key: %w", err)
	}
	edKey, ok := rawKey.(ed25519.PrivateKey)
	if !ok {
		return fmt.Errorf("federation: root key is not ed25519 (got %T)", rawKey)
	}
	c.installRootLocked(cert, edKey)
	return nil
}

func (c *FederationCA) installRootLocked(cert *x509.Certificate, key ed25519.PrivateKey) {
	c.rootCert = cert
	c.rootKey = key
	c.rootCertPEM = pem.EncodeToMemory(&pem.Block{Type: pemTypeCertificate, Bytes: cert.Raw})
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	c.rootPool = pool
	sum := sha256.Sum256(cert.Raw)
	c.fingerprint = hex.EncodeToString(sum[:])
}

// Fingerprint returns the lowercase hex SHA-256 digest of the DER encoding
// of the federation root certificate. This is embedded in peer enrollment
// tokens so joining Directories can trust-on-first-use.
func (c *FederationCA) Fingerprint() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.fingerprint
}

// RootPEM returns the federation root certificate in PEM form, suitable for
// installing in peer trust stores.
func (c *FederationCA) RootPEM() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]byte, len(c.rootCertPEM))
	copy(out, c.rootCertPEM)
	return out
}

// RootPool returns a CertPool containing just the federation root. Used for
// peer mTLS verification.
func (c *FederationCA) RootPool() *x509.CertPool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rootPool
}

// RootCert returns the parsed federation root certificate.
func (c *FederationCA) RootCert() *x509.Certificate {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rootCert
}

// IssuePeerCert signs a CSR to produce a leaf certificate for a peer
// Directory. The leaf is valid for the given TTL (clamped to peerValidity
// maximum). This is used when a remote Directory joins the federation.
func (c *FederationCA) IssuePeerCert(_ context.Context, csr *x509.CertificateRequest, ttl time.Duration) (*x509.Certificate, error) {
	if csr == nil {
		return nil, errors.New("federation: nil CSR")
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("federation: CSR signature: %w", err)
	}
	if ttl <= 0 || ttl > peerValidity {
		ttl = peerValidity
	}

	// In cloud mode, delegate to the cloud provider if available.
	if c.cfg.Mode == ModeCloud && c.cfg.CloudProvider != nil {
		return c.cfg.CloudProvider.IssuePeerCert(context.Background(), csr)
	}

	c.mu.RLock()
	root := c.rootCert
	key := c.rootKey
	c.mu.RUnlock()
	if root == nil || key == nil {
		return nil, errors.New("federation: CA not initialized")
	}

	serial, err := randSerial()
	if err != nil {
		return nil, fmt.Errorf("federation: leaf serial: %w", err)
	}
	now := c.now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               csr.Subject,
		DNSNames:              csr.DNSNames,
		IPAddresses:           csr.IPAddresses,
		URIs:                  csr.URIs,
		NotBefore:             now.Add(-clockSkew),
		NotAfter:              now.Add(ttl),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, root, csr.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("federation: sign peer leaf: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("federation: parse peer leaf: %w", err)
	}
	return leaf, nil
}

// IssueSelfPeerCert issues (or reuses if still valid) this Directory's own
// federation peer certificate. Used for mTLS client auth when connecting to
// other federated Directories.
func (c *FederationCA) IssueSelfPeerCert(_ context.Context) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	certPath := filepath.Join(c.dir, filePeerCert)
	keyPath := filepath.Join(c.dir, filePeerKey)

	// Fast path: reuse existing leaf if present and still valid for >1h.
	if c.peerLeaf == nil {
		if leaf, err := c.loadPeerLeafLocked(certPath, keyPath); err == nil && leaf != nil {
			c.peerLeaf = leaf
		}
	}
	if c.peerLeaf != nil && c.peerLeaf.Leaf != nil &&
		c.now().Add(1*time.Hour).Before(c.peerLeaf.Leaf.NotAfter) {
		return c.peerLeaf, nil
	}

	// Generate a new ed25519 keypair for the peer leaf.
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("federation: peer leaf keygen: %w", err)
	}
	serial, err := randSerial()
	if err != nil {
		return nil, err
	}
	cn := fmt.Sprintf("%s-%s", subjectPeerCN, c.cfg.SiteID)
	now := c.now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   cn,
			Organization: []string{"Kaivue"},
		},
		DNSNames:              []string{cn, "directory.kaivue.local", "localhost"},
		NotBefore:             now.Add(-clockSkew),
		NotAfter:              now.Add(peerValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.rootCert, pub, c.rootKey)
	if err != nil {
		return nil, fmt.Errorf("federation: sign peer leaf: %w", err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}

	// Persist cert (plaintext) + key (sealed).
	if err := writePEMFile(certPath, pemTypeCertificate, der); err != nil {
		return nil, fmt.Errorf("federation: persist peer cert: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}
	sealed, err := c.cs.Encrypt(keyDER)
	for i := range keyDER {
		keyDER[i] = 0
	}
	if err != nil {
		return nil, fmt.Errorf("federation: seal peer key: %w", err)
	}
	if err := writePEMFile(keyPath, pemTypeEncryptedPrivKey, sealed); err != nil {
		return nil, fmt.Errorf("federation: persist peer key: %w", err)
	}

	tlsCert := &tls.Certificate{
		Certificate: [][]byte{der, c.rootCert.Raw},
		PrivateKey:  priv,
		Leaf:        parsed,
	}
	c.peerLeaf = tlsCert
	return tlsCert, nil
}

func (c *FederationCA) loadPeerLeafLocked(certPath, keyPath string) (*tls.Certificate, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	cb, _ := pem.Decode(certPEM)
	if cb == nil || cb.Type != pemTypeCertificate {
		return nil, errors.New("federation: malformed peer cert")
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
		return nil, errors.New("federation: malformed peer key")
	}
	opened, err := c.cs.Decrypt(kb.Bytes)
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
		return nil, fmt.Errorf("federation: peer key not ed25519 (got %T)", raw)
	}
	return &tls.Certificate{
		Certificate: [][]byte{cb.Bytes, c.rootCert.Raw},
		PrivateKey:  priv,
		Leaf:        parsed,
	}, nil
}

// PeerTLSConfig returns a tls.Config configured for federation mTLS using
// this Directory's peer certificate and the federation trust pool.
// This config can be used for both client and server sides of Directory
// to Directory connections.
func (c *FederationCA) PeerTLSConfig() (*tls.Config, error) {
	c.mu.RLock()
	leaf := c.peerLeaf
	pool := c.rootPool
	c.mu.RUnlock()

	if leaf == nil {
		return nil, errors.New("federation: no peer leaf certificate")
	}
	if pool == nil {
		return nil, errors.New("federation: no root pool")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{*leaf},
		RootCAs:      pool,
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// Shutdown zeroes in-memory key material.
func (c *FederationCA) Shutdown(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.rootKey {
		c.rootKey[i] = 0
	}
	c.rootKey = nil
	c.rootCert = nil
	c.rootCertPEM = nil
	c.rootPool = nil
	c.peerLeaf = nil
	return nil
}

// --- helpers --------------------------------------------------------------

func randSerial() (*big.Int, error) {
	max := new(big.Int).Lsh(big.NewInt(1), 159)
	return rand.Int(rand.Reader, max)
}

func writePEMFile(path, blockType string, der []byte) error {
	buf := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
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

var _ crypto.Signer = ed25519.PrivateKey(nil)
