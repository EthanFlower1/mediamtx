package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/ai"
	"github.com/bluenviron/mediamtx/internal/nvr/ai/forensic"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// ForensicSearchHandler implements the forensic multi-faceted search API.
type ForensicSearchHandler struct {
	DB       *db.DB
	Embedder *ai.Embedder
}

// forensicSearchRequest is the JSON request body for POST /api/nvr/forensic-search.
type forensicSearchRequest struct {
	Query  *forensic.Query `json:"query"`
	Limit  int             `json:"limit,omitempty"`
	Offset int             `json:"offset,omitempty"`
}

// Search handles POST /api/nvr/forensic-search
//
// Accepts a JSON query DSL body with AND/OR/NOT composition of:
//   - clip: CLIP text similarity search
//   - object: object detection class matching
//   - lpr: license plate text matching
//   - time: absolute time range
//   - camera: camera ID filtering
//   - time_of_day: recurring daily time window
//   - day_of_week: day-of-week filtering
//   - confidence: minimum confidence threshold
//
// Example request body:
//
//	{
//	  "query": {
//	    "op": "AND",
//	    "children": [
//	      {"type": "clip", "clip_text": "red truck"},
//	      {"type": "time_of_day", "time_of_day_start": "18:00", "time_of_day_end": "08:00"},
//	      {"type": "day_of_week", "days_of_week": [2, 3, 4]},
//	      {"type": "camera", "camera_ids": ["loading-dock-cam"]}
//	    ]
//	  },
//	  "limit": 50,
//	  "offset": 0
//	}
func (h *ForensicSearchHandler) Search(c *gin.Context) {
	var req forensicSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	if req.Query == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}

	executor := &forensic.Executor{
		DB:       h.DB,
		Embedder: h.Embedder,
	}

	result, err := executor.Execute(&forensic.ExecuteRequest{
		Query:  req.Query,
		Limit:  req.Limit,
		Offset: req.Offset,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// SampleQueries handles GET /api/nvr/forensic-search/samples
// Returns a set of example queries demonstrating the DSL capabilities.
func (h *ForensicSearchHandler) SampleQueries(c *gin.Context) {
	samples := []gin.H{
		{
			"description": "Find red trucks at the loading dock between 6pm and 8am on weekdays",
			"query": gin.H{
				"op": "AND",
				"children": []gin.H{
					{"type": "clip", "clip_text": "red truck"},
					{"type": "time_of_day", "time_of_day_start": "18:00", "time_of_day_end": "08:00"},
					{"type": "day_of_week", "days_of_week": []int{1, 2, 3, 4, 5}},
					{"type": "camera", "camera_ids": []string{"loading-dock"}},
				},
			},
		},
		{
			"description": "Find any person or vehicle with plate ABC* in the parking lot",
			"query": gin.H{
				"op": "AND",
				"children": []gin.H{
					{
						"op": "OR",
						"children": []gin.H{
							{"type": "object", "object_class": "person"},
							{"type": "lpr", "plate_text": "ABC*"},
						},
					},
					{"type": "camera", "camera_ids": []string{"parking-lot"}},
				},
			},
		},
		{
			"description": "Find cars but not trucks last Tuesday with high confidence",
			"query": gin.H{
				"op": "AND",
				"children": []gin.H{
					{"type": "object", "object_class": "car"},
					{
						"op": "NOT",
						"children": []gin.H{
							{"type": "object", "object_class": "truck"},
						},
					},
					{"type": "day_of_week", "days_of_week": []int{2}},
					{"type": "confidence", "min_confidence": 0.85},
				},
			},
		},
		{
			"description": "Find anything resembling a delivery at entrance cameras during business hours",
			"query": gin.H{
				"op": "AND",
				"children": []gin.H{
					{"type": "clip", "clip_text": "delivery van package"},
					{"type": "time_of_day", "time_of_day_start": "08:00", "time_of_day_end": "18:00"},
					{"type": "camera", "camera_ids": []string{"entrance-1", "entrance-2"}},
				},
			},
		},
		{
			"description": "Find vehicles with specific plate OR matching CLIP description",
			"query": gin.H{
				"op": "OR",
				"children": []gin.H{
					{"type": "lpr", "plate_text": "XYZ789"},
					{
						"op": "AND",
						"children": []gin.H{
							{"type": "clip", "clip_text": "white sedan"},
							{"type": "object", "object_class": "car"},
						},
					},
				},
			},
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"count":   len(samples),
		"samples": samples,
	})
}
