package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// FederationHandler exposes REST endpoints for federation management in the
// admin UI. This is a local-NVR-side handler; full Directory ↔ Directory
// federation uses the gRPC FederationPeerService defined in federation_peer.proto.
type FederationHandler struct {
	DB    *db.DB
	Audit *AuditLogger

	mu     sync.RWMutex
	tokens map[string]federationInvite // tokenValue -> invite metadata
}

type federationInvite struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ---------- GET /federation ----------
// Returns the current federation (if any) and its list of peers.

func (h *FederationHandler) Get(c *gin.Context) {
	fed, err := h.DB.GetFederation()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query federation"})
		return
	}
	if fed == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no federation configured"})
		return
	}

	peers, err := h.DB.ListFederationPeers(fed.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list peers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"federation": fed,
		"peers":      peers,
	})
}

// ---------- POST /federation ----------
// Create a new federation.

type createFederationRequest struct {
	Name string `json:"name" binding:"required"`
}

func (h *FederationHandler) Create(c *gin.Context) {
	var req createFederationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if len(name) > 128 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name must be 128 characters or fewer"})
		return
	}

	fed, err := h.DB.CreateFederation(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create federation"})
		return
	}

	if h.Audit != nil {
		h.Audit.Log(c, "federation.create", "federation", fed.ID, fmt.Sprintf("created federation %q", name))
	}

	c.JSON(http.StatusCreated, fed)
}

// ---------- DELETE /federation ----------
// Disband (delete) the current federation.

func (h *FederationHandler) Delete(c *gin.Context) {
	fed, err := h.DB.GetFederation()
	if err != nil || fed == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no federation configured"})
		return
	}

	if err := h.DB.DeleteFederation(fed.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete federation"})
		return
	}

	if h.Audit != nil {
		h.Audit.Log(c, "federation.delete", "federation", fed.ID, fmt.Sprintf("disbanded federation %q", fed.Name))
	}

	c.Status(http.StatusNoContent)
}

// ---------- POST /federation/invite ----------
// Generate an invite token for a peer to join.

func (h *FederationHandler) GenerateInvite(c *gin.Context) {
	fed, err := h.DB.GetFederation()
	if err != nil || fed == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no federation configured"})
		return
	}

	// Generate a FED- prefixed token with 32 random hex bytes.
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}
	tokenValue := "FED-" + hex.EncodeToString(tokenBytes)
	expiresAt := time.Now().Add(10 * time.Minute)

	h.mu.Lock()
	if h.tokens == nil {
		h.tokens = make(map[string]federationInvite)
	}
	h.tokens[tokenValue] = federationInvite{
		Token:     tokenValue,
		ExpiresAt: expiresAt,
	}
	h.mu.Unlock()

	if h.Audit != nil {
		h.Audit.Log(c, "federation.invite", "federation", fed.ID, "generated invite token")
	}

	c.JSON(http.StatusOK, gin.H{
		"token":      tokenValue,
		"expires_at": expiresAt.Format(time.RFC3339),
	})
}

// ---------- POST /federation/join ----------
// Join a federation using an invite token.

type joinFederationRequest struct {
	Token string `json:"token" binding:"required"`
}

func (h *FederationHandler) Join(c *gin.Context) {
	var req joinFederationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
		return
	}
	token := strings.TrimSpace(req.Token)
	if token == "" || !strings.HasPrefix(token, "FED-") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid token format"})
		return
	}

	// In a production deployment, this would validate the token against the
	// remote Directory and perform the mTLS handshake. For now, we record the
	// join attempt and create a peer entry.
	fed, err := h.DB.GetFederation()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query federation"})
		return
	}

	// If no federation exists yet, create one from the join flow.
	if fed == nil {
		fed, err = h.DB.CreateFederation("Joined Federation")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create federation on join"})
			return
		}
	}

	peer, err := h.DB.AddFederationPeer(fed.ID, token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add peer"})
		return
	}

	if h.Audit != nil {
		h.Audit.Log(c, "federation.join", "federation", fed.ID, fmt.Sprintf("joined federation via token, peer %s", peer.ID))
	}

	c.JSON(http.StatusOK, gin.H{
		"federation": fed,
		"peer":       peer,
	})
}

// ---------- DELETE /federation/peers/:id ----------
// Remove a peer from the federation.

func (h *FederationHandler) RemovePeer(c *gin.Context) {
	peerID := c.Param("id")
	if peerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "peer id is required"})
		return
	}

	if err := h.DB.RemoveFederationPeer(peerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove peer"})
		return
	}

	if h.Audit != nil {
		h.Audit.Log(c, "federation.remove_peer", "federation", peerID, fmt.Sprintf("removed peer %s", peerID))
	}

	c.Status(http.StatusNoContent)
}
