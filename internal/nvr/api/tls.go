package api

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/crypto"
)

// TLSHandler provides HTTP endpoints for TLS certificate management.
type TLSHandler struct {
	Manager *crypto.TLSManager
}

// Status returns the current TLS certificate information including
// subject, issuer, expiry, and whether the certificate is self-signed.
//
//	GET /api/nvr/system/tls/status
func (h *TLSHandler) Status(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	if !h.Manager.HasCertificate() {
		c.JSON(http.StatusOK, gin.H{
			"configured": false,
			"message":    "no TLS certificate is configured",
		})
		return
	}

	info, err := h.Manager.GetCertificateInfo()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to read certificate", err)
		return
	}

	level, daysLeft, _ := h.Manager.CheckExpiry()

	c.JSON(http.StatusOK, gin.H{
		"configured":    true,
		"subject":       info.Subject,
		"issuer":        info.Issuer,
		"not_before":    info.NotBefore,
		"not_after":     info.NotAfter,
		"serial_number": info.SerialNumber,
		"dns_names":     info.DNSNames,
		"ip_addresses":  info.IPAddresses,
		"is_ca":         info.IsCA,
		"self_signed":   info.SelfSigned,
		"days_left":     daysLeft,
		"expiry_level":  level,
		"fingerprint":   info.Fingerprint,
	})
}

// Upload accepts a PEM-encoded certificate and key via multipart form upload
// and stores them, replacing any existing certificate.
//
//	POST /api/nvr/system/tls/upload
//
// Form fields:
//   - cert: PEM-encoded certificate file
//   - key: PEM-encoded private key file
func (h *TLSHandler) Upload(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	certFile, err := c.FormFile("cert")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing 'cert' file"})
		return
	}
	keyFile, err := c.FormFile("key")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing 'key' file"})
		return
	}

	// Read certificate.
	cf, err := certFile.Open()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to open cert file", err)
		return
	}
	defer cf.Close()
	certPEM, err := io.ReadAll(io.LimitReader(cf, 1<<20)) // 1 MB limit
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to read cert file", err)
		return
	}

	// Read key.
	kf, err := keyFile.Open()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to open key file", err)
		return
	}
	defer kf.Close()
	keyPEM, err := io.ReadAll(io.LimitReader(kf, 1<<20)) // 1 MB limit
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to read key file", err)
		return
	}

	if err := h.Manager.StoreCertificate(certPEM, keyPEM); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid certificate or key: %v", err)})
		return
	}

	nvrLogInfo("tls", "TLS certificate uploaded successfully")

	// Return the new certificate info.
	info, err := h.Manager.GetCertificateInfo()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "certificate stored but failed to read back", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "certificate uploaded successfully",
		"subject":     info.Subject,
		"not_after":   info.NotAfter,
		"days_left":   info.DaysLeft,
		"self_signed": info.SelfSigned,
		"fingerprint": info.Fingerprint,
	})
}

// Generate creates a new self-signed certificate, replacing any existing one.
//
//	POST /api/nvr/system/tls/generate
func (h *TLSHandler) Generate(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	// Remove existing cert to force regeneration.
	_ = removeIfExists(h.Manager.CertPath())
	_ = removeIfExists(h.Manager.KeyPath())

	generated, err := h.Manager.EnsureCertificate()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to generate certificate", err)
		return
	}
	if !generated {
		apiError(c, http.StatusInternalServerError, "certificate generation failed unexpectedly", fmt.Errorf("EnsureCertificate returned false"))
		return
	}

	nvrLogInfo("tls", "self-signed TLS certificate generated")

	info, err := h.Manager.GetCertificateInfo()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "certificate generated but failed to read", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "self-signed certificate generated",
		"subject":     info.Subject,
		"not_after":   info.NotAfter,
		"days_left":   info.DaysLeft,
		"fingerprint": info.Fingerprint,
	})
}

// removeIfExists removes a file if it exists, ignoring "not exists" errors.
func removeIfExists(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
