package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"

	"golang.org/x/crypto/hkdf"
)

// GenerateRSAKeyPair generates a 2048-bit RSA key pair and returns
// the private and public keys in PEM-encoded form.
func GenerateRSAKeyPair() (privPEM []byte, pubPEM []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate RSA key: %w", err)
	}

	privDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal private key: %w", err)
	}
	privPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privDER,
	})

	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal public key: %w", err)
	}
	pubPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDER,
	})

	return privPEM, pubPEM, nil
}

// ParseRSAPrivateKey parses a PEM-encoded RSA private key.
func ParseRSAPrivateKey(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA")
	}

	return rsaKey, nil
}

// DeriveKey derives a 32-byte key from masterSecret and info using
// HKDF-SHA256 with an empty salt.
func DeriveKey(masterSecret, info string) []byte {
	reader := hkdf.New(sha256.New, []byte(masterSecret), nil, []byte(info))
	key := make([]byte, 32)
	// hkdf.New reader will not return an error for valid inputs.
	_, _ = reader.Read(key)
	return key
}

// jwksKey represents a single RSA public key in JWKS format.
type jwksKey struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// jwks represents a JSON Web Key Set.
type jwks struct {
	Keys []jwksKey `json:"keys"`
}

// base64URLUint encodes a big.Int as unpadded base64url.
func base64URLUint(n *big.Int) string {
	return base64.RawURLEncoding.EncodeToString(n.Bytes())
}

// JWKSFromPublicKey generates a JWKS JSON document from a PEM-encoded
// RSA public key. The key ID is set to "nvr-signing-key".
func JWKSFromPublicKey(pubPEM []byte) ([]byte, error) {
	block, _ := pem.Decode(pubPEM)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA")
	}

	ks := jwks{
		Keys: []jwksKey{
			{
				Kty: "RSA",
				Use: "sig",
				Alg: "RS256",
				Kid: "nvr-signing-key",
				N:   base64URLUint(rsaPub.N),
				E:   base64URLUint(big.NewInt(int64(rsaPub.E))),
			},
		},
	}

	return json.Marshal(ks)
}
