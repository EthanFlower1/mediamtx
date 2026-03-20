package crypto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateRSAKeyPair(t *testing.T) {
	priv, pub, err := GenerateRSAKeyPair()
	require.NoError(t, err)
	require.Contains(t, string(priv), "PRIVATE KEY")
	require.Contains(t, string(pub), "PUBLIC KEY")
}

func TestParseRSAPrivateKey(t *testing.T) {
	priv, _, err := GenerateRSAKeyPair()
	require.NoError(t, err)

	key, err := ParseRSAPrivateKey(priv)
	require.NoError(t, err)
	require.NotNil(t, key)
	require.Equal(t, 2048, key.N.BitLen())

	// Invalid PEM should fail.
	_, err = ParseRSAPrivateKey([]byte("not-pem"))
	require.Error(t, err)
}

func TestDeriveKey(t *testing.T) {
	// Same inputs produce the same output.
	k1 := DeriveKey("master", "info1")
	k2 := DeriveKey("master", "info1")
	require.Equal(t, k1, k2)
	require.Len(t, k1, 32)

	// Different info produces a different key.
	k3 := DeriveKey("master", "info2")
	require.NotEqual(t, k1, k3)
}

func TestJWKSFromPublicKey(t *testing.T) {
	_, pub, err := GenerateRSAKeyPair()
	require.NoError(t, err)

	jwksData, err := JWKSFromPublicKey(pub)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(jwksData, &parsed))

	keys, ok := parsed["keys"].([]interface{})
	require.True(t, ok)
	require.Len(t, keys, 1)

	key := keys[0].(map[string]interface{})
	require.Equal(t, "RSA", key["kty"])
	require.Equal(t, "sig", key["use"])
	require.Equal(t, "RS256", key["alg"])
	require.Equal(t, "nvr-signing-key", key["kid"])
	require.NotEmpty(t, key["n"])
	require.NotEmpty(t, key["e"])

	// Invalid PEM should fail.
	_, err = JWKSFromPublicKey([]byte("bad"))
	require.Error(t, err)
}
