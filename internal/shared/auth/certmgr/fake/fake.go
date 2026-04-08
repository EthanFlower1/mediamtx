// Package fake provides test doubles for the certmgr package interfaces.
// These are exported so integration-test packages outside this module can
// wire them in without duplicating the implementation.
package fake

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// CAClient is a fake implementation of certmgr.CAClient for unit and
// integration tests. It generates real x509 certificates using an in-memory
// ECDSA root, so tests exercise the full cert-verification path without
// depending on a running step-ca instance.
type CAClient struct {
	mu sync.Mutex

	rootKey  *ecdsa.PrivateKey
	rootCert *x509.Certificate
	rootPool *x509.CertPool

	// RenewErr, if non-nil, is returned by Renew instead of issuing a cert.
	RenewErr error

	// ReEnrollErr, if non-nil, is returned by ReEnroll instead of issuing.
	ReEnrollErr error

	// RenewCalls counts how many times Renew has been called.
	RenewCalls atomic.Uint64

	// ReEnrollCalls counts how many times ReEnroll has been called.
	ReEnrollCalls atomic.Uint64

	// LeafLifetime is the validity window for issued leaves.
	// Default: 24h.
	LeafLifetime time.Duration

	// Now overrides time.Now for issued cert NotBefore/NotAfter.
	// Nil = time.Now.
	Now func() time.Time
}

// NewCAClient creates a new fake CAClient with a self-signed in-memory root.
// The root is generated with ECDSA P-256 so it is cheap to create in tests.
func NewCAClient() (*CAClient, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "fake-root-ca"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	root, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}

	pool := x509.NewCertPool()
	pool.AddCert(root)

	return &CAClient{
		rootKey:      key,
		rootCert:     root,
		rootPool:     pool,
		LeafLifetime: 24 * time.Hour,
	}, nil
}

// Renew implements certmgr.CAClient. If RenewErr is set it is returned
// immediately. Otherwise, a fresh leaf is signed by the in-memory root,
// mirroring all SANs (DNS names + IP addresses) from the presented cert.
func (c *CAClient) Renew(ctx context.Context, current *tls.Certificate) (*tls.Certificate, error) {
	c.RenewCalls.Add(1)
	if c.RenewErr != nil {
		return nil, c.RenewErr
	}
	// Collect all SANs from the existing leaf — DNS names and IP addresses
	// (in string form so issueLeaf's splitter handles them uniformly).
	var sans []string
	if current != nil && current.Leaf != nil {
		sans = append(sans, current.Leaf.DNSNames...)
		for _, ip := range current.Leaf.IPAddresses {
			sans = append(sans, ip.String())
		}
	}
	if len(sans) == 0 {
		sans = []string{"component.kaivue.local"}
	}
	return c.issueLeaf(sans, nil)
}

// ReEnroll implements certmgr.CAClient. If ReEnrollErr is set it is returned
// immediately. Otherwise, a fresh leaf is signed by the in-memory root using
// the provided sans.
func (c *CAClient) ReEnroll(ctx context.Context, deviceKey crypto.PrivateKey, sans []string) (*tls.Certificate, error) {
	c.ReEnrollCalls.Add(1)
	if c.ReEnrollErr != nil {
		return nil, c.ReEnrollErr
	}
	if len(sans) == 0 {
		sans = []string{"component.kaivue.local"}
	}
	return c.issueLeaf(sans, deviceKey)
}

// RootPool implements certmgr.CAClient.
func (c *CAClient) RootPool() *x509.CertPool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rootPool
}

// issueLeaf creates a fresh ECDSA leaf cert signed by the in-memory root.
// If leafKey is non-nil it is used as the leaf private key (for re-enroll);
// otherwise a fresh keypair is generated (for renew).
func (c *CAClient) issueLeaf(dnsNames []string, leafKey crypto.PrivateKey) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().UTC()
	if c.Now != nil {
		now = c.Now().UTC()
	}
	lifetime := c.LeafLifetime
	if lifetime <= 0 {
		lifetime = 24 * time.Hour
	}

	var priv *ecdsa.PrivateKey
	if ek, ok := leafKey.(*ecdsa.PrivateKey); ok && ek != nil {
		priv = ek
	} else {
		var err error
		priv, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	// Split SANs into DNS names and IP addresses.
	var dnsOnly []string
	var ipAddrs []net.IP
	for _, san := range dnsNames {
		if ip := net.ParseIP(san); ip != nil {
			ipAddrs = append(ipAddrs, ip)
		} else {
			dnsOnly = append(dnsOnly, san)
		}
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "fake-leaf"},
		DNSNames:     dnsOnly,
		IPAddresses:  ipAddrs,
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(lifetime),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.rootCert, &priv.PublicKey, c.rootKey)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}

	return &tls.Certificate{
		Certificate: [][]byte{der, c.rootCert.Raw},
		PrivateKey:  priv,
		Leaf:        leaf,
	}, nil
}

// MemKeyStore is an in-memory implementation of certmgr.KeyStore for tests.
type MemKeyStore struct {
	mu   sync.Mutex
	cert *tls.Certificate

	// SaveErr, if non-nil, is returned by Save.
	SaveErr error
	// LoadErr, if non-nil, is returned by Load.
	LoadErr error

	// SaveCalls counts Save invocations.
	SaveCalls atomic.Uint64
	// LoadCalls counts Load invocations.
	LoadCalls atomic.Uint64
}

// Load implements certmgr.KeyStore.
func (s *MemKeyStore) Load(_ context.Context) (*tls.Certificate, error) {
	s.LoadCalls.Add(1)
	if s.LoadErr != nil {
		return nil, s.LoadErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cert, nil
}

// Save implements certmgr.KeyStore.
func (s *MemKeyStore) Save(_ context.Context, cert *tls.Certificate) error {
	s.SaveCalls.Add(1)
	if s.SaveErr != nil {
		return s.SaveErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cert = cert
	return nil
}
