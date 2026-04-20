package recorderapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// FanoutService queries multiple recorders in parallel and merges results.
type FanoutService struct {
	Store  *Store
	Logger *slog.Logger

	// ServiceTokens maps recorder_id → service token for auth.
	// In production this would be looked up dynamically; for v1 we
	// accept a single shared token passed at boot.
	SharedServiceToken string
}

// recorderResult holds the outcome of a single recorder query.
type recorderResult struct {
	RecorderID string
	Items      []json.RawMessage
	Error      error
}

// FanoutRecordingsHandler handles GET /api/v1/query/recordings.
// Fans out to all healthy recorders and merges.
func (f *FanoutService) FanoutRecordingsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cameraID := r.URL.Query().Get("camera_id")
		startStr := r.URL.Query().Get("start")
		endStr := r.URL.Query().Get("end")

		start, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			http.Error(w, `{"error":"invalid start (RFC3339)"}`, http.StatusBadRequest)
			return
		}
		end, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			http.Error(w, `{"error":"invalid end (RFC3339)"}`, http.StatusBadRequest)
			return
		}

		results := f.fanout(r.Context(), func(ctx context.Context, client *RecorderClient) ([]json.RawMessage, error) {
			return client.QueryRecordings(ctx, cameraID, start, end)
		})

		writeAggregated(w, results)
	}
}

// FanoutEventsHandler handles GET /api/v1/query/events.
func (f *FanoutService) FanoutEventsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cameraID := r.URL.Query().Get("camera_id")
		eventType := r.URL.Query().Get("type")
		startStr := r.URL.Query().Get("start")
		endStr := r.URL.Query().Get("end")

		start, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			http.Error(w, `{"error":"invalid start (RFC3339)"}`, http.StatusBadRequest)
			return
		}
		end, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			http.Error(w, `{"error":"invalid end (RFC3339)"}`, http.StatusBadRequest)
			return
		}

		results := f.fanout(r.Context(), func(ctx context.Context, client *RecorderClient) ([]json.RawMessage, error) {
			return client.QueryEvents(ctx, cameraID, eventType, start, end)
		})

		writeAggregated(w, results)
	}
}

// FanoutTimelineHandler handles GET /api/v1/query/timeline.
func (f *FanoutService) FanoutTimelineHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cameraID := r.URL.Query().Get("camera_id")
		date := r.URL.Query().Get("date")

		if cameraID == "" || date == "" {
			http.Error(w, `{"error":"camera_id and date required"}`, http.StatusBadRequest)
			return
		}

		results := f.fanout(r.Context(), func(ctx context.Context, client *RecorderClient) ([]json.RawMessage, error) {
			return client.QueryTimeline(ctx, cameraID, date)
		})

		// Timeline uses "blocks" key.
		var all []json.RawMessage
		var errors []string
		for _, res := range results {
			if res.Error != nil {
				errors = append(errors, res.RecorderID+": "+res.Error.Error())
				continue
			}
			all = append(all, res.Items...)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"blocks": all,
			"errors": errors,
		})
	}
}

// FanoutHealthHandler handles GET /api/v1/query/health — aggregated recorder health.
func (f *FanoutService) FanoutHealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recs, err := f.Store.ListRecorders(r.Context())
		if err != nil {
			http.Error(w, `{"error":"list recorders failed"}`, http.StatusInternalServerError)
			return
		}

		type recHealth struct {
			RecorderID string          `json:"recorder_id"`
			Health     *RecorderHealth `json:"health,omitempty"`
			Error      string          `json:"error,omitempty"`
		}

		results := make([]recHealth, len(recs))
		var wg sync.WaitGroup
		for i, rec := range recs {
			wg.Add(1)
			go func(idx int, rec RecorderRow) {
				defer wg.Done()
				if rec.InternalAPIAddr == "" {
					results[idx] = recHealth{RecorderID: rec.ID, Error: "no internal API address"}
					return
				}
				client := NewRecorderClient("http://"+resolveAddr(rec.InternalAPIAddr), f.SharedServiceToken)
				h, err := client.Health(r.Context())
				if err != nil {
					results[idx] = recHealth{RecorderID: rec.ID, Error: err.Error()}
					return
				}
				results[idx] = recHealth{RecorderID: rec.ID, Health: h}
			}(i, rec)
		}
		wg.Wait()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"recorders": results})
	}
}

// fanout queries all healthy recorders in parallel using the given query function.
func (f *FanoutService) fanout(ctx context.Context, queryFn func(context.Context, *RecorderClient) ([]json.RawMessage, error)) []recorderResult {
	recs, err := f.Store.ListRecorders(ctx)
	if err != nil {
		f.Logger.Error("fanout: list recorders failed", "error", err)
		return nil
	}

	results := make([]recorderResult, len(recs))
	var wg sync.WaitGroup

	for i, rec := range recs {
		if rec.InternalAPIAddr == "" {
			results[i] = recorderResult{RecorderID: rec.ID, Error: fmt.Errorf("no internal API address")}
			continue
		}

		wg.Add(1)
		go func(idx int, rec RecorderRow) {
			defer wg.Done()

			queryCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()

			client := NewRecorderClient("http://"+resolveAddr(rec.InternalAPIAddr), f.SharedServiceToken)
			items, err := queryFn(queryCtx, client)
			results[idx] = recorderResult{
				RecorderID: rec.ID,
				Items:      items,
				Error:      err,
			}
		}(i, rec)
	}

	wg.Wait()
	return results
}

// resolveAddr normalizes listen addresses like ":8880" to "127.0.0.1:8880".
func resolveAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	if strings.HasPrefix(addr, "[::]:") {
		return "127.0.0.1:" + strings.TrimPrefix(addr, "[::]:")
	}
	return addr
}

func writeAggregated(w http.ResponseWriter, results []recorderResult) {
	var all []json.RawMessage
	var errors []string

	for _, res := range results {
		if res.Error != nil {
			errors = append(errors, res.RecorderID+": "+res.Error.Error())
			continue
		}
		all = append(all, res.Items...)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"items":  all,
		"errors": errors,
	})
}
