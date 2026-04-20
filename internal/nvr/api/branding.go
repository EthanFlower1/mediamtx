package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// BrandingHandler implements HTTP endpoints for branding customization.
type BrandingHandler struct {
	DB       *db.DB
	DataDir  string // base directory for storing uploaded assets (e.g., "./data")
}

// brandingResponse is the JSON shape returned by GET /api/nvr/system/branding.
type brandingResponse struct {
	ProductName string `json:"product_name"`
	AccentColor string `json:"accent_color"`
	LogoURL     string `json:"logo_url"`
}

// GetBranding returns the current branding configuration.
//
//	GET /api/nvr/system/branding
func (h *BrandingHandler) GetBranding(c *gin.Context) {
	resp := brandingResponse{
		ProductName: "Raikada",
		AccentColor: "#FF8C00",
		LogoURL:     "",
	}

	if v, err := h.DB.GetConfig("branding.product_name"); err == nil {
		resp.ProductName = v
	}
	if v, err := h.DB.GetConfig("branding.accent_color"); err == nil {
		resp.AccentColor = v
	}
	if v, err := h.DB.GetConfig("branding.logo_url"); err == nil {
		resp.LogoURL = v
	}

	c.JSON(http.StatusOK, resp)
}

// updateBrandingRequest is the JSON body for PUT /api/nvr/system/branding.
type updateBrandingRequest struct {
	ProductName *string `json:"product_name"`
	AccentColor *string `json:"accent_color"`
}

// UpdateBranding updates the branding configuration (product name and accent color).
//
//	PUT /api/nvr/system/branding (admin only)
func (h *BrandingHandler) UpdateBranding(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	var req updateBrandingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if req.ProductName != nil {
		name := strings.TrimSpace(*req.ProductName)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "product_name cannot be empty"})
			return
		}
		if len(name) > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "product_name must be 100 characters or fewer"})
			return
		}
		if err := h.DB.SetConfig("branding.product_name", name); err != nil {
			apiError(c, http.StatusInternalServerError, "failed to save product name", err)
			return
		}
	}

	if req.AccentColor != nil {
		color := strings.TrimSpace(*req.AccentColor)
		if !isValidHexColor(color) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "accent_color must be a valid hex color (e.g., #6366f1)"})
			return
		}
		if err := h.DB.SetConfig("branding.accent_color", color); err != nil {
			apiError(c, http.StatusInternalServerError, "failed to save accent color", err)
			return
		}
	}

	// Return the updated branding.
	h.GetBranding(c)
}

// UploadLogo handles logo file uploads.
//
//	POST /api/nvr/system/branding/logo (admin only, multipart/form-data)
func (h *BrandingHandler) UploadLogo(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	file, header, err := c.Request.FormFile("logo")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing logo file in form data"})
		return
	}
	defer file.Close()

	// Validate file size (max 2 MB).
	if header.Size > 2*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "logo file must be 2 MB or smaller"})
		return
	}

	// Validate content type.
	contentType := header.Header.Get("Content-Type")
	var ext string
	switch {
	case strings.HasPrefix(contentType, "image/png"):
		ext = ".png"
	case strings.HasPrefix(contentType, "image/jpeg"):
		ext = ".jpg"
	case strings.HasPrefix(contentType, "image/svg+xml"):
		ext = ".svg"
	case strings.HasPrefix(contentType, "image/webp"):
		ext = ".webp"
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported image type; use PNG, JPEG, SVG, or WebP"})
		return
	}

	// Ensure the branding assets directory exists.
	brandingDir := filepath.Join(h.DataDir, "branding")
	if err := os.MkdirAll(brandingDir, 0o755); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create branding directory", err)
		return
	}

	// Delete the old logo file if one exists.
	if oldURL, err := h.DB.GetConfig("branding.logo_url"); err == nil && oldURL != "" {
		// The URL is /api/nvr/system/branding/logo/<filename>, extract filename.
		parts := strings.Split(oldURL, "/")
		if len(parts) > 0 {
			oldFile := filepath.Join(brandingDir, parts[len(parts)-1])
			os.Remove(oldFile) // best effort
		}
	}

	// Save with a unique name.
	filename := fmt.Sprintf("logo-%s%s", uuid.New().String()[:8], ext)
	destPath := filepath.Join(brandingDir, filename)

	out, err := os.Create(destPath)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to save logo file", err)
		return
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to write logo file", err)
		return
	}

	// Store the URL path in the config table.
	logoURL := fmt.Sprintf("/api/nvr/system/branding/logo/%s", filename)
	if err := h.DB.SetConfig("branding.logo_url", logoURL); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to save logo URL", err)
		return
	}

	nvrLogInfo("branding", "logo uploaded: "+filename)

	c.JSON(http.StatusOK, gin.H{
		"logo_url": logoURL,
	})
}

// ServeLogo serves the uploaded logo file.
//
//	GET /api/nvr/system/branding/logo/:filename
func (h *BrandingHandler) ServeLogo(c *gin.Context) {
	filename := c.Param("filename")

	// Sanitize filename to prevent path traversal.
	filename = filepath.Base(filename)
	if filename == "." || filename == "/" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	filePath := filepath.Join(h.DataDir, "branding", filename)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "logo not found"})
		return
	}

	c.File(filePath)
}

// DeleteLogo removes the uploaded logo and resets to default.
//
//	DELETE /api/nvr/system/branding/logo (admin only)
func (h *BrandingHandler) DeleteLogo(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	// Remove old file.
	if oldURL, err := h.DB.GetConfig("branding.logo_url"); err == nil && oldURL != "" {
		parts := strings.Split(oldURL, "/")
		if len(parts) > 0 {
			oldFile := filepath.Join(h.DataDir, "branding", parts[len(parts)-1])
			os.Remove(oldFile) // best effort
		}
	}

	// Clear from DB.
	_ = h.DB.DeleteConfig("branding.logo_url")

	nvrLogInfo("branding", "logo deleted")

	c.JSON(http.StatusOK, gin.H{"logo_url": ""})
}

// isValidHexColor validates a CSS hex color string.
func isValidHexColor(s string) bool {
	if len(s) != 7 && len(s) != 4 {
		return false
	}
	if s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
