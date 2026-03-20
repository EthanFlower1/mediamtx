package crypto

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncryptDecrypt(t *testing.T) {
	key := DeriveKey("test-secret", "encryption")
	plaintext := []byte("hello, world!")

	ciphertext, err := Encrypt(key, plaintext)
	require.NoError(t, err)
	require.NotEqual(t, plaintext, ciphertext)

	decrypted, err := Decrypt(key, ciphertext)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
}

func TestEncryptDifferentOutputs(t *testing.T) {
	key := DeriveKey("test-secret", "encryption")
	plaintext := []byte("same input")

	ct1, err := Encrypt(key, plaintext)
	require.NoError(t, err)

	ct2, err := Encrypt(key, plaintext)
	require.NoError(t, err)

	// Random nonce means different ciphertexts.
	require.False(t, bytes.Equal(ct1, ct2))
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := DeriveKey("secret-1", "encryption")
	key2 := DeriveKey("secret-2", "encryption")

	ciphertext, err := Encrypt(key1, []byte("secret data"))
	require.NoError(t, err)

	_, err = Decrypt(key2, ciphertext)
	require.Error(t, err)
}
