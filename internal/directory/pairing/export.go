package pairing

// This file re-exports the shared PairingToken contract so that callers
// who import internal/directory/pairing can still reference these types
// and functions directly, without importing internal/shared/pairing.
//
// The canonical definitions live in internal/shared/pairing.

import (
	"crypto"
	"crypto/ed25519"

	sharedpairing "github.com/bluenviron/mediamtx/internal/shared/pairing"
)

// TokenTTL re-exports sharedpairing.TokenTTL.
const TokenTTL = sharedpairing.TokenTTL

// UserID re-exports sharedpairing.UserID.
type UserID = sharedpairing.UserID

// PairingToken re-exports sharedpairing.PairingToken.
type PairingToken = sharedpairing.PairingToken

// Decode re-exports sharedpairing.Decode.
func Decode(token string, verifyKey crypto.PublicKey) (*PairingToken, error) {
	return sharedpairing.Decode(token, verifyKey)
}

// DecodeTokenUnsafe re-exports sharedpairing.DecodeTokenUnsafe.
func DecodeTokenUnsafe(token string) (*PairingToken, ed25519.PublicKey, error) {
	return sharedpairing.DecodeTokenUnsafe(token)
}

// NewSigningKey re-exports sharedpairing.NewSigningKey.
func NewSigningKey(rootKey ed25519.PrivateKey) (ed25519.PrivateKey, error) {
	return sharedpairing.NewSigningKey(rootKey)
}

// VerifyPublicKey re-exports sharedpairing.VerifyPublicKey.
func VerifyPublicKey(signingKey ed25519.PrivateKey) ed25519.PublicKey {
	return sharedpairing.VerifyPublicKey(signingKey)
}
