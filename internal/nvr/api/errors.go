package api

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// apiError logs the actual error server-side with a request ID and returns
// a generic user-facing message. The request ID ties the client response to
// the server-side log for debugging.
func apiError(c *gin.Context, status int, userMsg string, err error) {
	reqID := uuid.New().String()[:8]
	log.Printf("[NVR] [ERROR] [%s] %s: %v", reqID, userMsg, err)
	c.JSON(status, gin.H{
		"error":      userMsg,
		"request_id": reqID,
	})
}
