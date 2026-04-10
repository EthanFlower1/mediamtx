package email

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

// DKIMKeySize is the RSA key size used for all new DKIM keys. 2048 is
// the DKIM2 / DKIM-EH baseline; most providers still reject 4096
// because TXT records exceed 255-byte chunks with ugly results.
const DKIMKeySize = 2048

// GeneratedKeypair holds a freshly-generated DKIM keypair. The caller
// is expected to immediately hand off PrivateKeyPEM to the KAI-251
// cryptostore and persist only the returned [DKIMKey].
type GeneratedKeypair struct {
	PrivateKeyPEM string
	PublicKeyPEM  string
	KeySizeBits   int
}

// GenerateDKIMKeypair creates a fresh RSA keypair suitable for DKIM
// signing. This function DOES NOT persist anything — the caller must
// hand PrivateKeyPEM to the cryptostore and only store the public key
// + cryptostore key id in the email package's tables.
func GenerateDKIMKeypair() (GeneratedKeypair, error) {
	key, err := rsa.GenerateKey(rand.Reader, DKIMKeySize)
	if err != nil {
		return GeneratedKeypair{}, fmt.Errorf("email: generate RSA key: %w", err)
	}
	return keypairFromRSA(key)
}

// keypairFromRSA encodes an already-generated RSA key to PEM. Split
// out so tests can drive it with a deterministic key and avoid paying
// for RSA-2048 generation in every test run.
func keypairFromRSA(key *rsa.PrivateKey) (GeneratedKeypair, error) {
	privDER := x509.MarshalPKCS1PrivateKey(key)
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privDER,
	})

	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return GeneratedKeypair{}, fmt.Errorf("email: marshal public key: %w", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDER,
	})

	return GeneratedKeypair{
		PrivateKeyPEM: string(privPEM),
		PublicKeyPEM:  string(pubPEM),
		KeySizeBits:   key.Size() * 8,
	}, nil
}
