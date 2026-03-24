package playback

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// HandleStream returns a gin handler that streams fMP4 data for a single
// camera in a playback session as a chunked HTTP response.
func HandleStream(manager *SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("session")
		cameraID := c.Param("camera")

		session := manager.GetSession(sessionID)
		if session == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}

		ch := session.StreamChannel(cameraID)
		if ch == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "camera not in session"})
			return
		}

		c.Header("Content-Type", "video/mp4")
		c.Header("Transfer-Encoding", "chunked")
		c.Header("Cache-Control", "no-cache, no-store")
		c.Header("Connection", "keep-alive")
		c.Header("Accept-Ranges", "none")
		c.Status(http.StatusOK)
		c.Writer.Flush()

		keepAlive := time.NewTicker(10 * time.Second)
		defer keepAlive.Stop()

		for {
			select {
			case data, ok := <-ch:
				if !ok {
					return
				}
				if _, err := c.Writer.Write(data); err != nil {
					return
				}
				c.Writer.Flush()

			case <-keepAlive.C:
				c.Writer.Flush()

			case <-c.Request.Context().Done():
				return
			}
		}
	}
}
