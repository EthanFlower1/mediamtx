package crypto

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testSecret = []byte("test-master-secret-for-unit-tests-at-least-32-bytes-long!")

func TestColumnEncryptor_RoundTrip(t *testing.T) {
	enc, err := NewColumnEncryptor(testSecret, "rtsp_password")
	require.NoError(t, err)

	original := "super-secret-rtsp-password"
	ciphertext, err := enc.Encrypt(original)
	require.NoError(t, err)

	// Ciphertext should be valid base64 and different from plaintext.
	assert.NotEqual(t, original, ciphertext)
	_, err = base64.StdEncoding.DecodeString(ciphertext)
	assert.NoError(t, err)

	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	assert.Equal(t, original, decrypted)
}

func TestColumnEncryptor_EmptyString(t *testing.T) {
	enc, err := NewColumnEncryptor(testSecret, "rtsp_username")
	require.NoError(t, err)

	ciphertext, err := enc.Encrypt("")
	require.NoError(t, err)
	assert.NotEmpty(t, ciphertext) // even empty string produces ciphertext

	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	assert.Equal(t, "", decrypted)
}

func TestColumnEncryptor_DifferentNoncesPerEncrypt(t *testing.T) {
	enc, err := NewColumnEncryptor(testSecret, "rtsp_password")
	require.NoError(t, err)

	ct1, err := enc.Encrypt("same-plaintext")
	require.NoError(t, err)
	ct2, err := enc.Encrypt("same-plaintext")
	require.NoError(t, err)

	// Two encryptions of the same plaintext should produce different ciphertexts
	// because each uses a random nonce.
	assert.NotEqual(t, ct1, ct2)

	// Both should decrypt to the same value.
	d1, err := enc.Decrypt(ct1)
	require.NoError(t, err)
	d2, err := enc.Decrypt(ct2)
	require.NoError(t, err)
	assert.Equal(t, d1, d2)
}

func TestColumnEncryptor_DifferentColumnsProduceDifferentKeys(t *testing.T) {
	encUser, err := NewColumnEncryptor(testSecret, "rtsp_username")
	require.NoError(t, err)
	encPass, err := NewColumnEncryptor(testSecret, "rtsp_password")
	require.NoError(t, err)

	ct, err := encUser.Encrypt("admin")
	require.NoError(t, err)

	// Decrypting with the password encryptor should fail (different derived key).
	_, err = encPass.Decrypt(ct)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decrypt")
}

func TestColumnEncryptor_TamperedCiphertextFails(t *testing.T) {
	enc, err := NewColumnEncryptor(testSecret, "rtsp_password")
	require.NoError(t, err)

	ct, err := enc.Encrypt("legit-password")
	require.NoError(t, err)

	// Tamper with a byte in the middle of the ciphertext.
	raw, err := base64.StdEncoding.DecodeString(ct)
	require.NoError(t, err)
	raw[len(raw)/2] ^= 0xFF
	tampered := base64.StdEncoding.EncodeToString(raw)

	_, err = enc.Decrypt(tampered)
	assert.Error(t, err)
}

func TestColumnEncryptor_TruncatedCiphertextFails(t *testing.T) {
	enc, err := NewColumnEncryptor(testSecret, "rtsp_password")
	require.NoError(t, err)

	// Too-short ciphertext (less than nonce + GCM overhead).
	_, err = enc.Decrypt(base64.StdEncoding.EncodeToString([]byte("short")))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
}

func TestColumnEncryptor_InvalidBase64Fails(t *testing.T) {
	enc, err := NewColumnEncryptor(testSecret, "rtsp_password")
	require.NoError(t, err)

	_, err = enc.Decrypt("not-valid-base64!!!")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "base64")
}

func TestNewColumnEncryptor_EmptySecretFails(t *testing.T) {
	_, err := NewColumnEncryptor(nil, "rtsp_password")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "master secret is empty")

	_, err = NewColumnEncryptor([]byte{}, "rtsp_password")
	assert.Error(t, err)
}

func TestNewColumnEncryptor_EmptyColumnInfoFails(t *testing.T) {
	_, err := NewColumnEncryptor(testSecret, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "column info is empty")
}

func TestCredentialEncryptor_RoundTrip(t *testing.T) {
	ce, err := NewCredentialEncryptor(testSecret)
	require.NoError(t, err)

	encU, encP, err := ce.EncryptCredentials("admin", "p@ssw0rd!")
	require.NoError(t, err)

	username, password, err := ce.DecryptCredentials(encU, encP)
	require.NoError(t, err)
	assert.Equal(t, "admin", username)
	assert.Equal(t, "p@ssw0rd!", password)
}

func TestCredentialEncryptor_CrossColumnDecryptFails(t *testing.T) {
	ce, err := NewCredentialEncryptor(testSecret)
	require.NoError(t, err)

	encU, _, err := ce.EncryptCredentials("admin", "password")
	require.NoError(t, err)

	// Try to decrypt username ciphertext with password decryptor.
	_, err = ce.Password.Decrypt(encU)
	assert.Error(t, err)
}

func TestColumnEncryptor_DifferentMasterSecrets(t *testing.T) {
	enc1, err := NewColumnEncryptor([]byte("secret-one-at-least-16-bytes!!"), "rtsp_password")
	require.NoError(t, err)
	enc2, err := NewColumnEncryptor([]byte("secret-two-at-least-16-bytes!!"), "rtsp_password")
	require.NoError(t, err)

	ct, err := enc1.Encrypt("password123")
	require.NoError(t, err)

	// Different master secret can't decrypt.
	_, err = enc2.Decrypt(ct)
	assert.Error(t, err)
}
