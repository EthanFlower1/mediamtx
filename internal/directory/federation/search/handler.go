package search

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// PeerProvider returns the current set of federated peers. Implementations
// typically query the directory's peer registry.
type PeerProvider func() []Peer

// HandlerConfig configures the Gin handler.
type HandlerConfig struct {
	// Peers returns the active set of federated peers.
	Peers PeerProvider

	// SearchConfig is forwarded to Search.
	SearchConfig Config

	// Logger for the handler. Nil defaults to slog.Default().
	Logger *slog.Logger
}

// searchRequest is the JSON body accepted by the API endpoint.
type searchRequest struct {
	CameraIDs  []string `json:"camera_ids,omitempty"`
	StartTime  string   `json:"start_time"` // RFC 3339
	EndTime    string   `json:"end_time"`   // RFC 3339
	EventKinds []int32  `json:"event_kinds,omitempty"`
	Query      string   `json:"query,omitempty"`
	PageSize   int32    `json:"page_size,omitempty"`
}

// searchResponseHit is a single recording hit in the JSON response.
type searchResponseHit struct {
	RecorderID string `json:"recorder_id"`
	CameraID   string `json:"camera_id"`
	SegmentID  string `json:"segment_id"`
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
	Bytes      int64  `json:"bytes"`
	IsEventClip bool  `json:"is_event_clip"`
}

// searchResponse is the JSON response body.
type searchResponse struct {
	Hits          []searchResponseHit    `json:"hits"`
	Partial       bool                   `json:"partial"`
	PeerErrors    map[string]string      `json:"peer_errors,omitempty"`
	PeerLatencies map[string]string      `json:"peer_latencies,omitempty"`
}

// RegisterRoutes adds the federated search endpoint to the given router group.
func RegisterRoutes(rg *gin.RouterGroup, hcfg HandlerConfig) {
	rg.POST("/federation/search/recordings", newSearchHandler(hcfg))
}

func newSearchHandler(hcfg HandlerConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body searchRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Build proto request.
		req := &kaivuev1.SearchRecordingsRequest{
			CameraIds: body.CameraIDs,
			Query:     body.Query,
			PageSize:  body.PageSize,
		}
		if body.StartTime != "" {
			if t, err := time.Parse(time.RFC3339, body.StartTime); err == nil {
				req.StartTime = timestamppb.New(t)
			}
		}
		if body.EndTime != "" {
			if t, err := time.Parse(time.RFC3339, body.EndTime); err == nil {
				req.EndTime = timestamppb.New(t)
			}
		}
		for _, ek := range body.EventKinds {
			req.EventKinds = append(req.EventKinds, kaivuev1.AIEventKind(ek))
		}

		peers := hcfg.Peers()
		result := Search(c.Request.Context(), hcfg.SearchConfig, peers, req)

		// Map to JSON response.
		resp := searchResponse{
			Hits:    make([]searchResponseHit, 0, len(result.Hits)),
			Partial: result.Partial,
		}
		for _, h := range result.Hits {
			resp.Hits = append(resp.Hits, searchResponseHit{
				RecorderID:  h.GetRecorderId(),
				CameraID:    h.GetCameraId(),
				SegmentID:   h.GetSegmentId(),
				StartTime:   h.GetStartTime().AsTime().Format(time.RFC3339),
				EndTime:     h.GetEndTime().AsTime().Format(time.RFC3339),
				Bytes:       h.GetBytes(),
				IsEventClip: h.GetIsEventClip(),
			})
		}
		if len(result.PeerErrors) > 0 {
			resp.PeerErrors = make(map[string]string, len(result.PeerErrors))
			for pid, err := range result.PeerErrors {
				resp.PeerErrors[pid] = err.Error()
			}
		}
		if len(result.PeerLatencies) > 0 {
			resp.PeerLatencies = make(map[string]string, len(result.PeerLatencies))
			for pid, lat := range result.PeerLatencies {
				resp.PeerLatencies[pid] = lat.String()
			}
		}

		c.JSON(http.StatusOK, resp)
	}
}
