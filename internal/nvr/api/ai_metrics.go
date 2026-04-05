package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/ai"
)

// AIMetricsHandler serves AI detection performance metrics.
type AIMetricsHandler struct {
	Collector *ai.DetectionMetrics
}

// GetMetrics returns all AI metrics as JSON.
// GET /ai/metrics
func (h *AIMetricsHandler) GetMetrics(c *gin.Context) {
	if h.Collector == nil {
		c.JSON(http.StatusOK, ai.DetectionMetricsSnapshot{
			Models:  []ai.ModelMetrics{},
			Cameras: []ai.CameraMetrics{},
		})
		return
	}
	c.JSON(http.StatusOK, h.Collector.Snapshot())
}

// GetMetricsPrometheus returns all AI metrics in Prometheus text exposition format.
// GET /ai/metrics/prometheus
func (h *AIMetricsHandler) GetMetricsPrometheus(c *gin.Context) {
	if h.Collector == nil {
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(""))
		return
	}
	c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8",
		[]byte(h.Collector.PrometheusExport()))
}
