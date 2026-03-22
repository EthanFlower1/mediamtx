package api

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// apiError logs the actual error server-side with a request ID and returns
// a generic user-facing message. The request ID ties the client response to
// the server-side log for debugging.
func apiError(c *gin.Context, status int, userMsg string, err error) {
	reqID := uuid.New().String()[:8]
	log.Printf("[NVR] [ERROR] [%s] %s: %v", reqID, userMsg, err)

	code := "internal_error"
	switch status {
	case http.StatusBadRequest:
		code = "bad_request"
	case http.StatusUnauthorized:
		code = "unauthorized"
	case http.StatusForbidden:
		code = "forbidden"
	case http.StatusNotFound:
		code = "not_found"
	case http.StatusConflict:
		code = "conflict"
	case http.StatusServiceUnavailable:
		code = "service_unavailable"
	case http.StatusNotImplemented:
		code = "not_implemented"
	case http.StatusTooManyRequests:
		code = "rate_limited"
	}

	c.JSON(status, gin.H{
		"error":      userMsg,
		"code":       code,
		"request_id": reqID,
	})
}
