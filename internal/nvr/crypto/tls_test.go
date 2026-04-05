package crypto

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTLSManager_EnsureCertificate(t *testing.T) {
	dir := t.TempDir()
	mgr := NewTLSManager(dir)

	// Should not have a certificate initially.
	require.False(t, mgr.HasCertificate())

	// Generate self-signed.
	generated, err := mgr.EnsureCertificate()
	require.NoError(t, err)
	require.True(t, generated)
	require.True(t, mgr.HasCertificate())

	// Calling again should not regenerate.
	generated2, err := mgr.EnsureCertificate()
	require.NoError(t, err)
	require.False(t, generated2)
}

func TestTLSManager_GetCertificateInfo(t *testing.T) {
	dir := t.TempDir()
	mgr := NewTLSManager(dir)

	_, err := mgr.EnsureCertificate()
	require.NoError(t, err)

	info, err := mgr.GetCertificateInfo()
	require.NoError(t, err)

	require.Equal(t, true, info.SelfSigned)
	require.Contains(t, info.Subject, "MediaMTX NVR")
	require.Contains(t, info.DNSNames, "localhost")
	require.True(t, info.DaysLeft > 360)
	require.NotEmpty(t, info.Fingerprint)
	require.NotEmpty(t, info.SerialNumber)
}

func TestTLSManager_CheckExpiry(t *testing.T) {
	dir := t.TempDir()
	mgr := NewTLSManager(dir)

	_, err := mgr.EnsureCertificate()
	require.NoError(t, err)

	level, daysLeft, err := mgr.CheckExpiry()
	require.NoError(t, err)
	require.Equal(t, "ok", level)
	require.True(t, daysLeft > 360)
}

func TestTLSManager_StoreCertificate_InvalidCert(t *testing.T) {
	dir := t.TempDir()
	mgr := NewTLSManager(dir)

	err := mgr.StoreCertificate([]byte("not a cert"), []byte("not a key"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid certificate PEM")
}

func TestTLSManager_StoreCertificate_RoundTrip(t *testing.T) {
	// Generate a cert in one manager, read it back, and store in another.
	dir1 := t.TempDir()
	mgr1 := NewTLSManager(dir1)
	_, err := mgr1.EnsureCertificate()
	require.NoError(t, err)

	certPEM, err := os.ReadFile(mgr1.CertPath())
	require.NoError(t, err)
	keyPEM, err := os.ReadFile(mgr1.KeyPath())
	require.NoError(t, err)

	dir2 := t.TempDir()
	mgr2 := NewTLSManager(dir2)
	err = mgr2.StoreCertificate(certPEM, keyPEM)
	require.NoError(t, err)

	// Verify stored cert matches.
	info2, err := mgr2.GetCertificateInfo()
	require.NoError(t, err)
	require.Contains(t, info2.Subject, "MediaMTX NVR")
}

func TestTLSManager_Paths(t *testing.T) {
	mgr := NewTLSManager("/tmp/test-tls")
	require.Equal(t, filepath.Join("/tmp/test-tls", "server.crt"), mgr.CertPath())
	require.Equal(t, filepath.Join("/tmp/test-tls", "server.key"), mgr.KeyPath())
}
