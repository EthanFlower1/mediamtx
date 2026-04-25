package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// TLSCertInfo contains parsed information about a TLS certificate.
type TLSCertInfo struct {
	Subject      string    `json:"subject"`
	Issuer       string    `json:"issuer"`
	NotBefore    time.Time `json:"not_before"`
	NotAfter     time.Time `json:"not_after"`
	SerialNumber string    `json:"serial_number"`
	DNSNames     []string  `json:"dns_names,omitempty"`
	IPAddresses  []string  `json:"ip_addresses,omitempty"`
	IsCA         bool      `json:"is_ca"`
	SelfSigned   bool      `json:"self_signed"`
	DaysLeft     int       `json:"days_left"`
	Fingerprint  string    `json:"fingerprint"`
}

// TLSManager handles TLS certificate generation, storage, and monitoring.
type TLSManager struct {
	CertDir string // directory to store cert and key files
}

// NewTLSManager creates a TLSManager that stores certificates in the given directory.
func NewTLSManager(certDir string) *TLSManager {
	return &TLSManager{CertDir: certDir}
}

// CertPath returns the path to the certificate file.
func (m *TLSManager) CertPath() string {
	return filepath.Join(m.CertDir, "server.crt")
}

// KeyPath returns the path to the private key file.
func (m *TLSManager) KeyPath() string {
	return filepath.Join(m.CertDir, "server.key")
}

// HasCertificate checks whether both certificate and key files exist.
func (m *TLSManager) HasCertificate() bool {
	_, certErr := os.Stat(m.CertPath())
	_, keyErr := os.Stat(m.KeyPath())
	return certErr == nil && keyErr == nil
}

// EnsureCertificate generates a self-signed certificate if none exists.
// It returns true if a new certificate was generated.
func (m *TLSManager) EnsureCertificate() (bool, error) {
	if m.HasCertificate() {
		return false, nil
	}

	if err := os.MkdirAll(m.CertDir, 0o700); err != nil {
		return false, fmt.Errorf("create cert directory: %w", err)
	}

	if err := m.generateSelfSigned(); err != nil {
		return false, err
	}
	return true, nil
}

// generateSelfSigned creates a self-signed ECDSA P-256 certificate valid for
// 365 days. It includes localhost, 127.0.0.1, and ::1 as SANs.
func (m *TLSManager) generateSelfSigned() error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate ECDSA key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generate serial number: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Raikada"},
			CommonName:   "Raikada",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}

	// Write certificate.
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err := os.WriteFile(m.CertPath(), certPEM, 0o644); err != nil {
		return fmt.Errorf("write certificate: %w", err)
	}

	// Write private key.
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(m.KeyPath(), keyPEM, 0o600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}

	return nil
}

// StoreCertificate writes the given PEM-encoded certificate and key to disk,
// replacing any existing files. It validates the cert and key before writing.
func (m *TLSManager) StoreCertificate(certPEM, keyPEM []byte) error {
	// Validate the certificate.
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return fmt.Errorf("invalid certificate PEM: no CERTIFICATE block found")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse certificate: %w", err)
	}

	// Validate the private key.
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return fmt.Errorf("invalid key PEM: no PEM block found")
	}

	privKey, err := parsePrivateKey(keyBlock)
	if err != nil {
		return fmt.Errorf("parse private key: %w", err)
	}

	// Verify the key matches the certificate.
	if err := verifyKeyMatchesCert(privKey, cert); err != nil {
		return fmt.Errorf("key does not match certificate: %w", err)
	}

	if err := os.MkdirAll(m.CertDir, 0o700); err != nil {
		return fmt.Errorf("create cert directory: %w", err)
	}

	if err := os.WriteFile(m.CertPath(), certPEM, 0o644); err != nil {
		return fmt.Errorf("write certificate: %w", err)
	}
	if err := os.WriteFile(m.KeyPath(), keyPEM, 0o600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}

	return nil
}

// GetCertificateInfo reads and parses the current certificate, returning its details.
func (m *TLSManager) GetCertificateInfo() (*TLSCertInfo, error) {
	certPEM, err := os.ReadFile(m.CertPath())
	if err != nil {
		return nil, fmt.Errorf("read certificate: %w", err)
	}
	return ParseCertificateInfo(certPEM)
}

// ParseCertificateInfo extracts information from PEM-encoded certificate data.
func ParseCertificateInfo(certPEM []byte) (*TLSCertInfo, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("invalid certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	ips := make([]string, 0, len(cert.IPAddresses))
	for _, ip := range cert.IPAddresses {
		ips = append(ips, ip.String())
	}

	selfSigned := cert.Issuer.CommonName == cert.Subject.CommonName &&
		cert.AuthorityKeyId == nil

	daysLeft := int(time.Until(cert.NotAfter).Hours() / 24)

	fingerprint := fmt.Sprintf("%X", sha256Fingerprint(cert.Raw))

	return &TLSCertInfo{
		Subject:      cert.Subject.String(),
		Issuer:       cert.Issuer.String(),
		NotBefore:    cert.NotBefore,
		NotAfter:     cert.NotAfter,
		SerialNumber: cert.SerialNumber.String(),
		DNSNames:     cert.DNSNames,
		IPAddresses:  ips,
		IsCA:         cert.IsCA,
		SelfSigned:   selfSigned,
		DaysLeft:     daysLeft,
		Fingerprint:  fingerprint,
	}, nil
}

// CheckExpiry returns a warning level based on certificate expiry.
// Returns "expired", "critical" (<7 days), "warning" (<30 days), or "ok".
func (m *TLSManager) CheckExpiry() (string, int, error) {
	info, err := m.GetCertificateInfo()
	if err != nil {
		return "", 0, err
	}

	if info.DaysLeft <= 0 {
		return "expired", info.DaysLeft, nil
	}
	if info.DaysLeft <= 7 {
		return "critical", info.DaysLeft, nil
	}
	if info.DaysLeft <= 30 {
		return "warning", info.DaysLeft, nil
	}
	return "ok", info.DaysLeft, nil
}

// parsePrivateKey tries to parse a PEM block as PKCS8, PKCS1, or EC private key.
func parsePrivateKey(block *pem.Block) (any, error) {
	// Try PKCS8 first (most common modern format).
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	// Try PKCS1 RSA.
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	// Try EC.
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, fmt.Errorf("unsupported private key type %q", block.Type)
}

// verifyKeyMatchesCert checks that the private key corresponds to the certificate's public key.
func verifyKeyMatchesCert(privKey any, cert *x509.Certificate) error {
	switch k := privKey.(type) {
	case *ecdsa.PrivateKey:
		pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
		if !ok {
			return fmt.Errorf("certificate has non-ECDSA public key but key is ECDSA")
		}
		if k.PublicKey.X.Cmp(pub.X) != 0 || k.PublicKey.Y.Cmp(pub.Y) != 0 {
			return fmt.Errorf("ECDSA key does not match certificate")
		}
	case *rsa.PrivateKey:
		pub, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf("certificate has non-RSA public key but key is RSA")
		}
		if k.PublicKey.N.Cmp(pub.N) != 0 || k.PublicKey.E != pub.E {
			return fmt.Errorf("RSA key does not match certificate")
		}
	default:
		return fmt.Errorf("unsupported private key type %T", privKey)
	}
	return nil
}

// sha256Fingerprint computes a SHA-256 hash of the raw certificate bytes.
func sha256Fingerprint(raw []byte) [32]byte {
	return sha256.Sum256(raw)
}
