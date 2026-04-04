package api

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// Config keys used for branding settings.
const (
	configKeyProductName = "branding_product_name"
	configKeyAccentColor = "branding_accent_color"
	configKeyLogoURL     = "branding_logo_url"
)

// BrandingHandler implements HTTP endpoints for branding customization.
type BrandingHandler struct {
	DB *db.DB
}

// brandingResponse is the JSON shape returned by GET and PUT branding.
type brandingResponse struct {
	ProductName string `json:"product_name"`
	AccentColor string `json:"accent_color"`
	LogoURL     string `json:"logo_url"`
}

// Get returns the current branding configuration.
//
//	GET /api/nvr/system/branding
func (h *BrandingHandler) Get(c *gin.Context) {
	resp := h.loadBranding()
	c.JSON(http.StatusOK, resp)
}

// Update sets the product name and/or accent color.
//
//	PUT /api/nvr/system/branding
func (h *BrandingHandler) Update(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	var req struct {
		ProductName *string `json:"product_name"`
		AccentColor *string `json:"accent_color"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if req.ProductName != nil {
		name := strings.TrimSpace(*req.ProductName)
		if len(name) > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "product_name must be 100 characters or fewer"})
			return
		}
		if err := h.DB.SetConfig(configKeyProductName, name); err != nil {
			apiError(c, http.StatusInternalServerError, "failed to save product name", err)
			return
		}
	}

	if req.AccentColor != nil {
		color := strings.TrimSpace(*req.AccentColor)
		if color != "" && !isValidHexColor(color) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "accent_color must be a valid hex color (e.g. #3B82F6)"})
			return
		}
		if err := h.DB.SetConfig(configKeyAccentColor, color); err != nil {
			apiError(c, http.StatusInternalServerError, "failed to save accent color", err)
			return
		}
	}

	resp := h.loadBranding()
	c.JSON(http.StatusOK, resp)
}

// UploadLogo accepts a logo image upload and stores it as a base64 data URI
// in the config table. Maximum file size is 512 KB.
//
//	POST /api/nvr/system/branding/logo
func (h *BrandingHandler) UploadLogo(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	const maxLogoBytes = 512 * 1024 // 512 KB

	file, header, err := c.Request.FormFile("logo")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing logo file in form data"})
		return
	}
	defer file.Close()

	if header.Size > maxLogoBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "logo file must be 512 KB or smaller"})
		return
	}

	data, err := io.ReadAll(io.LimitReader(file, maxLogoBytes+1))
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to read logo file", err)
		return
	}
	if len(data) > int(maxLogoBytes) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "logo file must be 512 KB or smaller"})
		return
	}

	// Detect content type.
	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(contentType, "image/") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file must be an image (PNG, JPEG, SVG, etc.)"})
		return
	}

	// For SVG files, http.DetectContentType returns text/xml; override.
	if strings.HasSuffix(strings.ToLower(header.Filename), ".svg") {
		contentType = "image/svg+xml"
	}

	// Encode as data URI.
	b64 := base64.StdEncoding.EncodeToString(data)
	dataURI := fmt.Sprintf("data:%s;base64,%s", contentType, b64)

	if err := h.DB.SetConfig(configKeyLogoURL, dataURI); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to save logo", err)
		return
	}

	nvrLogInfo("branding", "logo uploaded successfully")

	resp := h.loadBranding()
	c.JSON(http.StatusOK, resp)
}

// DeleteLogo removes the custom logo, reverting to the default.
//
//	DELETE /api/nvr/system/branding/logo
func (h *BrandingHandler) DeleteLogo(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	// Setting to empty string effectively removes the custom logo.
	if err := h.DB.SetConfig(configKeyLogoURL, ""); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to remove logo", err)
		return
	}

	nvrLogInfo("branding", "logo removed")

	resp := h.loadBranding()
	c.JSON(http.StatusOK, resp)
}

// loadBranding reads all branding config keys from the database.
func (h *BrandingHandler) loadBranding() brandingResponse {
	resp := brandingResponse{
		ProductName: "MediaMTX NVR",
		AccentColor: "#3B82F6",
	}

	if v, err := h.DB.GetConfig(configKeyProductName); err == nil && v != "" {
		resp.ProductName = v
	}
	if v, err := h.DB.GetConfig(configKeyAccentColor); err == nil && v != "" {
		resp.AccentColor = v
	}
	if v, err := h.DB.GetConfig(configKeyLogoURL); err == nil && v != "" {
		resp.LogoURL = v
	}

	return resp
}

// isValidHexColor checks that a string is a valid 3, 4, 6, or 8-digit hex color.
func isValidHexColor(s string) bool {
	if len(s) == 0 || s[0] != '#' {
		return false
	}
	hex := s[1:]
	switch len(hex) {
	case 3, 4, 6, 8:
		// valid lengths
	default:
		return false
	}
	for _, c := range hex {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
