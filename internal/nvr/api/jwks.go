package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// JWKSHandler serves the JSON Web Key Set for token verification.
type JWKSHandler struct {
	JWKSJSON []byte
}

// ServeJWKS serves the JWKS JSON document.
func (h *JWKSHandler) ServeJWKS(c *gin.Context) {
	c.Data(http.StatusOK, "application/json", h.JWKSJSON)
}
