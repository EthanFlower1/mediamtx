// Package api — API key management handlers for the integrator portal (KAI-319).
//
// Endpoints:
//
//	POST   /api/nvr/api-keys           — generate a new key
//	GET    /api/nvr/api-keys           — list keys
//	POST   /api/nvr/api-keys/:id/rotate — rotate (creates new key, grace period on old)
//	DELETE /api/nvr/api-keys/:id       — revoke
//	GET    /api/nvr/api-keys/:id/audit — per-key audit log
package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// Default rotation grace period: the old key stays valid for this duration
// after rotation so integrators have time to swap credentials.
const defaultGracePeriod = 24 * time.Hour

// APIKeyHandler handles CRUD + rotate/revoke for integrator API keys.
type APIKeyHandler struct {
	DB    *db.DB
	Audit *AuditLogger
}

// generateRawKey produces a 32-byte random key encoded as hex (64 chars).
// The first 8 chars serve as a non-secret prefix for identification.
func generateRawKey() (raw string, prefix string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("generate key: %w", err)
	}
	raw = hex.EncodeToString(buf)
	prefix = raw[:8]
	h := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(h[:])
	return raw, prefix, hash, nil
}

// hashKey returns the SHA-256 hex digest of a raw key string.
func hashKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// ----- Generate (POST /api-keys) -----

type generateRequest struct {
	Name          string `json:"name" binding:"required"`
	Scope         string `json:"scope" binding:"required,oneof=read-only read-write"`
	CustomerScope string `json:"customer_scope"`
	ExpiresInDays int    `json:"expires_in_days"` // 0 = no expiry
}

type generateResponse struct {
	Key    string      `json:"key"` // one-time display
	APIKey *db.APIKey  `json:"api_key"`
}

// Generate creates a new API key, stores the hash, and returns the raw key
// exactly once. Requires admin role.
func (h *APIKeyHandler) Generate(c *gin.Context) {
	role, _ := c.Get("role")
	if role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin role required"})
		return
	}

	var req generateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	raw, prefix, keyHash, err := generateRawKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate key"})
		return
	}

	userID, _ := c.Get("user_id")
	uid, _ := userID.(string)

	expiresAt := ""
	if req.ExpiresInDays > 0 {
		expiresAt = time.Now().UTC().Add(time.Duration(req.ExpiresInDays) * 24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	}

	k := &db.APIKey{
		Name:          req.Name,
		KeyPrefix:     prefix,
		KeyHash:       keyHash,
		Scope:         req.Scope,
		CustomerScope: req.CustomerScope,
		CreatedBy:     uid,
		ExpiresAt:     expiresAt,
	}

	if err := h.DB.CreateAPIKey(k); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store key"})
		return
	}

	// Audit: key created.
	username, _ := c.Get("username")
	uname, _ := username.(string)
	_ = h.DB.InsertAPIKeyAudit(&db.APIKeyAuditEntry{
		APIKeyID:      k.ID,
		Action:        "created",
		ActorID:       uid,
		ActorUsername: uname,
		IPAddress:     c.ClientIP(),
		Details:       fmt.Sprintf("scope=%s customer_scope=%s", k.Scope, k.CustomerScope),
	})
	h.Audit.logAction(c, "api_key_created", "api_key", k.ID,
		fmt.Sprintf("name=%s scope=%s", k.Name, k.Scope))

	c.JSON(http.StatusCreated, generateResponse{
		Key:    raw,
		APIKey: k,
	})
}

// ----- List (GET /api-keys) -----

// List returns all API keys visible to the caller. Admins see all keys;
// non-admins see only their own.
func (h *APIKeyHandler) List(c *gin.Context) {
	role, _ := c.Get("role")
	createdBy := ""
	if role != "admin" {
		userID, _ := c.Get("user_id")
		createdBy, _ = userID.(string)
	}

	keys, err := h.DB.ListAPIKeys(createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list keys"})
		return
	}
	if keys == nil {
		keys = []*db.APIKey{}
	}
	c.JSON(http.StatusOK, gin.H{"api_keys": keys})
}

// ----- Rotate (POST /api-keys/:id/rotate) -----

type rotateRequest struct {
	GracePeriodHours int `json:"grace_period_hours"` // 0 = default 24h
}

type rotateResponse struct {
	Key    string     `json:"key"` // one-time display of the NEW key
	APIKey *db.APIKey `json:"api_key"`
}

// Rotate generates a new key that replaces the old one. The old key enters
// a grace period (default 24 hours) during which both keys are accepted.
// Requires admin role.
func (h *APIKeyHandler) Rotate(c *gin.Context) {
	role, _ := c.Get("role")
	if role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin role required"})
		return
	}

	oldID := c.Param("id")
	oldKey, err := h.DB.GetAPIKey(oldID)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch key"})
		return
	}
	if oldKey.RevokedAt != "" {
		c.JSON(http.StatusConflict, gin.H{"error": "cannot rotate a revoked key"})
		return
	}

	var req rotateRequest
	// Body is optional — default grace period is used when omitted.
	_ = c.ShouldBindJSON(&req)

	grace := defaultGracePeriod
	if req.GracePeriodHours > 0 {
		grace = time.Duration(req.GracePeriodHours) * time.Hour
	}
	graceExpiry := time.Now().UTC().Add(grace)

	// Mark old key with grace expiry.
	if err := h.DB.SetAPIKeyGraceExpiry(oldID, graceExpiry); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set grace period"})
		return
	}

	// Generate replacement key.
	raw, prefix, keyHash, err := generateRawKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate new key"})
		return
	}

	userID, _ := c.Get("user_id")
	uid, _ := userID.(string)

	newKey := &db.APIKey{
		Name:          oldKey.Name,
		KeyPrefix:     prefix,
		KeyHash:       keyHash,
		Scope:         oldKey.Scope,
		CustomerScope: oldKey.CustomerScope,
		CreatedBy:     uid,
		ExpiresAt:     oldKey.ExpiresAt,
		RotatedFrom:   oldID,
	}
	if err := h.DB.CreateAPIKey(newKey); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store new key"})
		return
	}

	// Audit entries.
	username, _ := c.Get("username")
	uname, _ := username.(string)
	_ = h.DB.InsertAPIKeyAudit(&db.APIKeyAuditEntry{
		APIKeyID:      oldID,
		Action:        "rotated",
		ActorID:       uid,
		ActorUsername: uname,
		IPAddress:     c.ClientIP(),
		Details:       fmt.Sprintf("new_key_id=%s grace_hours=%d", newKey.ID, int(grace.Hours())),
	})
	_ = h.DB.InsertAPIKeyAudit(&db.APIKeyAuditEntry{
		APIKeyID:      newKey.ID,
		Action:        "created_via_rotation",
		ActorID:       uid,
		ActorUsername: uname,
		IPAddress:     c.ClientIP(),
		Details:       fmt.Sprintf("rotated_from=%s", oldID),
	})
	h.Audit.logAction(c, "api_key_rotated", "api_key", oldID,
		fmt.Sprintf("new_key=%s grace=%s", newKey.ID, graceExpiry.Format(time.RFC3339)))

	c.JSON(http.StatusCreated, rotateResponse{
		Key:    raw,
		APIKey: newKey,
	})
}

// ----- Revoke (DELETE /api-keys/:id) -----

// Revoke marks a key as revoked. The key is rejected immediately (within the
// 5-second acceptance criteria window since revoked_at is checked on every
// auth attempt). Requires admin role.
func (h *APIKeyHandler) Revoke(c *gin.Context) {
	role, _ := c.Get("role")
	if role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin role required"})
		return
	}

	id := c.Param("id")
	if err := h.DB.RevokeAPIKey(id); errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found or already revoked"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke key"})
		return
	}

	userID, _ := c.Get("user_id")
	uid, _ := userID.(string)
	username, _ := c.Get("username")
	uname, _ := username.(string)
	_ = h.DB.InsertAPIKeyAudit(&db.APIKeyAuditEntry{
		APIKeyID:      id,
		Action:        "revoked",
		ActorID:       uid,
		ActorUsername: uname,
		IPAddress:     c.ClientIP(),
	})
	h.Audit.logAction(c, "api_key_revoked", "api_key", id, "")

	c.JSON(http.StatusOK, gin.H{"message": "key revoked"})
}

// ----- Per-key audit log (GET /api-keys/:id/audit) -----

// AuditLog returns the audit trail for a specific API key.
func (h *APIKeyHandler) AuditLog(c *gin.Context) {
	role, _ := c.Get("role")
	if role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin role required"})
		return
	}

	id := c.Param("id")
	entries, err := h.DB.ListAPIKeyAudit(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch audit log"})
		return
	}
	if entries == nil {
		entries = []*db.APIKeyAuditEntry{}
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries})
}
