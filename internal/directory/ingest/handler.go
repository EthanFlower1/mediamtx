package ingest

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// RecorderAuthenticator verifies that the request comes from a paired Recorder
// and returns its recorder_id. The Directory wires this to mTLS cert validation
// or a bearer token check.
type RecorderAuthenticator func(r *http.Request) (recorderID string, ok bool)

// --- Wire types (match internal/recorder/directoryingest wire format) ---

type wireBoundingBox struct {
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Width  float32 `json:"width"`
	Height float32 `json:"height"`
}

type wireCameraStatePayload struct {
	RecorderID string                  `json:"recorder_id"`
	Updates    []wireCameraStateUpdate `json:"updates"`
}

type wireCameraStateUpdate struct {
	CameraID           string `json:"camera_id"`
	State              string `json:"state"`
	ObservedAt         string `json:"observed_at"`
	ErrorMessage       string `json:"error_message,omitempty"`
	CurrentBitrateKbps int32  `json:"current_bitrate_kbps,omitempty"`
	CurrentFramerate   int32  `json:"current_framerate,omitempty"`
	LastFrameAt        string `json:"last_frame_at,omitempty"`
	ConfigVersion      int64  `json:"config_version,omitempty"`
}

type wireSegmentIndexPayload struct {
	RecorderID string                  `json:"recorder_id"`
	Entries    []wireSegmentIndexEntry `json:"entries"`
}

type wireSegmentIndexEntry struct {
	CameraID    string `json:"camera_id"`
	SegmentID   string `json:"segment_id"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	Bytes       int64  `json:"bytes,omitempty"`
	Codec       string `json:"codec,omitempty"`
	HasAudio    bool   `json:"has_audio,omitempty"`
	IsEventClip bool   `json:"is_event_clip,omitempty"`
	StorageTier string `json:"storage_tier,omitempty"`
	Sequence    int64  `json:"sequence,omitempty"`
}

type wireAIEventsPayload struct {
	RecorderID string        `json:"recorder_id"`
	Events     []wireAIEvent `json:"events"`
}

type wireAIEvent struct {
	EventID      string            `json:"event_id"`
	CameraID     string            `json:"camera_id"`
	Kind         string            `json:"kind"`
	KindLabel    string            `json:"kind_label,omitempty"`
	ObservedAt   string            `json:"observed_at"`
	Confidence   float32           `json:"confidence,omitempty"`
	Bbox         wireBoundingBox   `json:"bbox"`
	TrackID      string            `json:"track_id,omitempty"`
	SegmentID    string            `json:"segment_id,omitempty"`
	ThumbnailRef string            `json:"thumbnail_ref,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

// StreamCameraStateHandler returns an http.HandlerFunc for
// POST /kaivue.v1.DirectoryIngest/StreamCameraState.
func StreamCameraStateHandler(store *Store, auth RecorderAuthenticator, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}

		recorderID, ok := auth(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "invalid recorder credentials")
			return
		}

		var payload wireCameraStatePayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
			return
		}

		if payload.RecorderID != "" && payload.RecorderID != recorderID {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "recorder_id mismatch")
			return
		}

		rows := make([]CameraStateRow, 0, len(payload.Updates))
		for _, u := range payload.Updates {
			if u.CameraID == "" || u.State == "" {
				continue
			}
			observedAt, err := time.Parse(time.RFC3339, u.ObservedAt)
			if err != nil {
				observedAt = time.Now().UTC()
			}
			row := CameraStateRow{
				CameraID:           u.CameraID,
				RecorderID:         recorderID,
				State:              u.State,
				ErrorMessage:       u.ErrorMessage,
				CurrentBitrateKbps: u.CurrentBitrateKbps,
				CurrentFramerate:   u.CurrentFramerate,
				ConfigVersion:      u.ConfigVersion,
				ObservedAt:         observedAt,
			}
			if u.LastFrameAt != "" {
				if t, err := time.Parse(time.RFC3339, u.LastFrameAt); err == nil {
					row.LastFrameAt = &t
				}
			}
			rows = append(rows, row)
		}

		if err := store.UpsertCameraStates(r.Context(), rows); err != nil {
			log.ErrorContext(r.Context(), "camera state upsert failed",
				slog.String("recorder", recorderID),
				slog.Int("count", len(rows)),
				slog.Any("error", err))
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to persist camera states")
			return
		}

		log.DebugContext(r.Context(), "camera states ingested",
			slog.String("recorder", recorderID),
			slog.Int("count", len(rows)))
		w.WriteHeader(http.StatusOK)
	}
}

// PublishSegmentIndexHandler returns an http.HandlerFunc for
// POST /kaivue.v1.DirectoryIngest/PublishSegmentIndex.
func PublishSegmentIndexHandler(store *Store, auth RecorderAuthenticator, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}

		recorderID, ok := auth(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "invalid recorder credentials")
			return
		}

		var payload wireSegmentIndexPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
			return
		}

		if payload.RecorderID != "" && payload.RecorderID != recorderID {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "recorder_id mismatch")
			return
		}

		rows := make([]SegmentIndexRow, 0, len(payload.Entries))
		for _, e := range payload.Entries {
			if e.SegmentID == "" || e.CameraID == "" {
				continue
			}
			startTime, err := time.Parse(time.RFC3339, e.StartTime)
			if err != nil {
				continue
			}
			endTime, err := time.Parse(time.RFC3339, e.EndTime)
			if err != nil {
				continue
			}
			rows = append(rows, SegmentIndexRow{
				SegmentID:   e.SegmentID,
				CameraID:    e.CameraID,
				RecorderID:  recorderID,
				StartTime:   startTime,
				EndTime:     endTime,
				Bytes:       e.Bytes,
				Codec:       e.Codec,
				HasAudio:    e.HasAudio,
				IsEventClip: e.IsEventClip,
				StorageTier: e.StorageTier,
				Sequence:    e.Sequence,
			})
		}

		if err := store.InsertSegments(r.Context(), rows); err != nil {
			log.ErrorContext(r.Context(), "segment index insert failed",
				slog.String("recorder", recorderID),
				slog.Int("count", len(rows)),
				slog.Any("error", err))
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to persist segments")
			return
		}

		log.DebugContext(r.Context(), "segments ingested",
			slog.String("recorder", recorderID),
			slog.Int("count", len(rows)))
		w.WriteHeader(http.StatusOK)
	}
}

// PublishAIEventsHandler returns an http.HandlerFunc for
// POST /kaivue.v1.DirectoryIngest/PublishAIEvents.
func PublishAIEventsHandler(store *Store, auth RecorderAuthenticator, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}

		recorderID, ok := auth(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "invalid recorder credentials")
			return
		}

		var payload wireAIEventsPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
			return
		}

		if payload.RecorderID != "" && payload.RecorderID != recorderID {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "recorder_id mismatch")
			return
		}

		rows := make([]AIEventRow, 0, len(payload.Events))
		for _, e := range payload.Events {
			if e.EventID == "" || e.CameraID == "" || e.Kind == "" {
				continue
			}
			observedAt, err := time.Parse(time.RFC3339, e.ObservedAt)
			if err != nil {
				observedAt = time.Now().UTC()
			}
			rows = append(rows, AIEventRow{
				EventID:      e.EventID,
				CameraID:     e.CameraID,
				RecorderID:   recorderID,
				Kind:         e.Kind,
				KindLabel:    e.KindLabel,
				ObservedAt:   observedAt,
				Confidence:   e.Confidence,
				BboxX:        e.Bbox.X,
				BboxY:        e.Bbox.Y,
				BboxWidth:    e.Bbox.Width,
				BboxHeight:   e.Bbox.Height,
				TrackID:      e.TrackID,
				SegmentID:    e.SegmentID,
				ThumbnailRef: e.ThumbnailRef,
				Attributes:   e.Attributes,
			})
		}

		if err := store.InsertAIEvents(r.Context(), rows); err != nil {
			log.ErrorContext(r.Context(), "ai events insert failed",
				slog.String("recorder", recorderID),
				slog.Int("count", len(rows)),
				slog.Any("error", err))
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to persist AI events")
			return
		}

		log.DebugContext(r.Context(), "ai events ingested",
			slog.String("recorder", recorderID),
			slog.Int("count", len(rows)))
		w.WriteHeader(http.StatusOK)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}
