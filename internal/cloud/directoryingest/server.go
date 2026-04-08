// Package directoryingest implements the server side of the Recorder →
// Directory ingest channel defined in
// internal/shared/proto/v1/directory_ingest.proto (KAI-238).
//
// Three HTTP long-poll endpoints mirror the three client-streaming RPCs:
//
//	POST /kaivue.v1.DirectoryIngest/StreamCameraState
//	POST /kaivue.v1.DirectoryIngest/PublishSegmentIndex
//	POST /kaivue.v1.DirectoryIngest/PublishAIEvents
//
// Wire format: NDJSON with proto-field-name keys — intentionally compatible
// with the proto shapes so the migration to generated Connect-Go (KAI-431)
// is mechanical. The recorder keeps the HTTP body open and streams batches;
// the server accumulates counters and replies with a single summary JSON
// line when the body is exhausted (EOF).
//
// Authentication: every RPC verifies that the recorder_id in the payload
// belongs to the authenticated tenant extracted from context. A mismatch
// returns 401. This mirrors the multi-tenant guard in recordercontrol.
//
// Proto-Lock-Holder: KAI-254
package directoryingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// -----------------------------------------------------------------------
// Store interfaces
// -----------------------------------------------------------------------

// CameraStateStore persists per-camera health snapshots.
type CameraStateStore interface {
	// UpsertCameraState writes (or overwrites) the latest state for a
	// camera. Must be tenant-scoped — implementations MUST include
	// tenantID in the upsert predicate.
	UpsertCameraState(ctx context.Context, s CameraState) error
}

// SegmentIndexStore persists recording segment metadata.
//
// TODO(KAI-249): when the camera_segment_index table lands, this should be
// backed by a real SQL upsert. The stub below writes to segment_index_stub.
type SegmentIndexStore interface {
	// UpsertSegmentEntries persists a batch of segment index entries.
	// Duplicate (segment_id, recorder_id) pairs are upserted idempotently.
	UpsertSegmentEntries(ctx context.Context, entries []SegmentIndexEntry) (accepted, rejectedDuplicate int64, err error)
}

// AIEventStore persists AI detection events.
type AIEventStore interface {
	// InsertAIEvents writes a batch of AI events to the ai_events table.
	// Unknown cameras are counted as rejected rather than erroring the
	// whole batch.
	InsertAIEvents(ctx context.Context, events []AIEvent) (accepted, rejectedUnknown int64, err error)
}

// RecorderAuthStore resolves the owning tenant of a recorder.
type RecorderAuthStore interface {
	// GetRecorderTenantID returns the tenant that owns this recorder.
	// Used to verify the recorder_id claim in the request payload against
	// the authenticated tenant carried in context.
	GetRecorderTenantID(ctx context.Context, recorderID string) (string, error)
}

// -----------------------------------------------------------------------
// Domain types
// -----------------------------------------------------------------------

// CameraState is the in-process representation of a CameraStateUpdate proto
// message. Field names match the proto for easy migration.
type CameraState struct {
	RecorderID          string
	TenantID            string
	CameraID            string
	State               string // "online"|"degraded"|"offline"|"error"|"unknown"
	ErrorMessage        string
	CurrentBitrateKbps  int32
	CurrentFramerate    int32
	LastFrameAt         time.Time
	ConfigVersion       int64
	ObservedAt          time.Time
}

// SegmentIndexEntry mirrors SegmentIndexEntry in the proto.
type SegmentIndexEntry struct {
	RecorderID   string
	TenantID     string
	CameraID     string
	SegmentID    string
	StartTime    time.Time
	EndTime      time.Time
	Bytes        int64
	Codec        string
	HasAudio     bool
	IsEventClip  bool
	StorageTier  string
	Sequence     int64
}

// AIEvent mirrors AIEvent in the proto.
type AIEvent struct {
	EventID      string
	TenantID     string
	RecorderID   string
	CameraID     string
	Kind         string
	KindLabel    string
	ObservedAt   time.Time
	Confidence   float32
	BboxX        float32
	BboxY        float32
	BboxWidth    float32
	BboxHeight   float32
	TrackID      string
	SegmentID    string
	ThumbnailRef string
	Attributes   map[string]string
}

// -----------------------------------------------------------------------
// Wire types (NDJSON, field names match proto)
// -----------------------------------------------------------------------

// wireCameraStateUpdate matches the NDJSON payload from the Recorder.
type wireCameraStateUpdate struct {
	CameraID           string  `json:"camera_id"`
	State              string  `json:"state"`
	ObservedAt         string  `json:"observed_at"` // RFC3339
	ErrorMessage       string  `json:"error_message,omitempty"`
	CurrentBitrateKbps int32   `json:"current_bitrate_kbps,omitempty"`
	CurrentFramerate   int32   `json:"current_framerate,omitempty"`
	LastFrameAt        string  `json:"last_frame_at,omitempty"` // RFC3339
	ConfigVersion      int64   `json:"config_version,omitempty"`
}

// wireCameraStateRequest is the NDJSON batch sent over the stream body.
type wireCameraStateRequest struct {
	RecorderID string                  `json:"recorder_id"`
	Updates    []wireCameraStateUpdate `json:"updates"`
}

// wireCameraStateResponse is the single summary line at stream close.
type wireCameraStateResponse struct {
	Accepted        int64 `json:"accepted"`
	RejectedUnknown int64 `json:"rejected_unknown"`
	RejectedStale   int64 `json:"rejected_stale"`
}

// wireSegmentIndexEntry matches SegmentIndexEntry proto.
type wireSegmentIndexEntry struct {
	CameraID    string `json:"camera_id"`
	SegmentID   string `json:"segment_id"`
	StartTime   string `json:"start_time"`  // RFC3339
	EndTime     string `json:"end_time"`    // RFC3339
	Bytes       int64  `json:"bytes,omitempty"`
	Codec       string `json:"codec,omitempty"`
	HasAudio    bool   `json:"has_audio,omitempty"`
	IsEventClip bool   `json:"is_event_clip,omitempty"`
	StorageTier string `json:"storage_tier,omitempty"`
	Sequence    int64  `json:"sequence,omitempty"`
}

// wireSegmentIndexRequest is the NDJSON batch sent over the stream body.
type wireSegmentIndexRequest struct {
	RecorderID string                  `json:"recorder_id"`
	Entries    []wireSegmentIndexEntry `json:"entries"`
}

// wireSegmentIndexResponse is the single summary line at stream close.
type wireSegmentIndexResponse struct {
	Accepted          int64 `json:"accepted"`
	RejectedUnknown   int64 `json:"rejected_unknown"`
	RejectedDuplicate int64 `json:"rejected_duplicate"`
}

// wireBoundingBox matches BoundingBox proto.
type wireBoundingBox struct {
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Width  float32 `json:"width"`
	Height float32 `json:"height"`
}

// wireAIEvent matches AIEvent proto.
type wireAIEvent struct {
	EventID      string            `json:"event_id"`
	CameraID     string            `json:"camera_id"`
	Kind         string            `json:"kind"`
	KindLabel    string            `json:"kind_label,omitempty"`
	ObservedAt   string            `json:"observed_at"` // RFC3339
	Confidence   float32           `json:"confidence,omitempty"`
	Bbox         wireBoundingBox   `json:"bbox"`
	TrackID      string            `json:"track_id,omitempty"`
	SegmentID    string            `json:"segment_id,omitempty"`
	ThumbnailRef string            `json:"thumbnail_ref,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

// wireAIEventsRequest is the NDJSON batch.
type wireAIEventsRequest struct {
	RecorderID string        `json:"recorder_id"`
	TenantID   string        `json:"tenant_id,omitempty"` // echoed for cross-check
	Events     []wireAIEvent `json:"events"`
}

// wireAIEventsResponse is the single summary line at stream close.
type wireAIEventsResponse struct {
	Accepted        int64 `json:"accepted"`
	RejectedUnknown int64 `json:"rejected_unknown"`
}

// -----------------------------------------------------------------------
// Config + Handler
// -----------------------------------------------------------------------

// Config configures the DirectoryIngest HTTP handlers.
// MetricsProvider is the narrow surface directoryingest needs from the
// shared metrics registry (KAI-422). A nil value disables metrics (safe).
type MetricsProvider interface {
	// IngestMessagesTotal increments the message counter with the given
	// stream name and result label.
	IngestMessagesTotal(stream, result string)
	// BackpressureDropsTotal increments the backpressure drop counter.
	BackpressureDropsTotal()
}

type Config struct {
	// CameraState persists camera health updates. Required.
	CameraState CameraStateStore
	// SegmentIndex persists segment metadata. Required.
	SegmentIndex SegmentIndexStore
	// AIEvents persists AI detection events. Required.
	AIEvents AIEventStore
	// Auth verifies recorder identity. Required.
	Auth RecorderAuthStore
	// Logger. Nil defaults to slog.Default().
	Logger *slog.Logger
	// MaxBatchBytes is the maximum body size for a single streamed batch
	// line. Zero defaults to 4 MiB.
	MaxBatchBytes int64
	// Metrics is the optional Prometheus metrics provider (KAI-422). Nil
	// means metrics are silently disabled (fail-open policy).
	Metrics MetricsProvider
}

func (c *Config) validate() error {
	if c.CameraState == nil {
		return errors.New("directoryingest: CameraState store is required")
	}
	if c.SegmentIndex == nil {
		return errors.New("directoryingest: SegmentIndex store is required")
	}
	if c.AIEvents == nil {
		return errors.New("directoryingest: AIEvents store is required")
	}
	if c.Auth == nil {
		return errors.New("directoryingest: Auth store is required")
	}
	return nil
}

func (c *Config) maxBatchBytes() int64 {
	if c.MaxBatchBytes > 0 {
		return c.MaxBatchBytes
	}
	return 4 * 1024 * 1024 // 4 MiB
}

// Handler holds the three ingest HTTP handlers. Mount via:
//
//	mux.Handle(StreamCameraStatePath, h.StreamCameraState())
//	mux.Handle(PublishSegmentIndexPath, h.PublishSegmentIndex())
//	mux.Handle(PublishAIEventsPath, h.PublishAIEvents())
type Handler struct {
	cfg Config
	log *slog.Logger
}

// HTTP path constants — mirror the proto package + service + method name so
// the Connect-Go migration is a rename only.
const (
	StreamCameraStatePath  = "/kaivue.v1.DirectoryIngest/StreamCameraState"
	PublishSegmentIndexPath = "/kaivue.v1.DirectoryIngest/PublishSegmentIndex"
	PublishAIEventsPath    = "/kaivue.v1.DirectoryIngest/PublishAIEvents"
)

// NewHandler constructs a validated Handler. Returns error if any required
// Config field is missing.
func NewHandler(cfg Config) (*Handler, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	l := cfg.Logger
	if l == nil {
		l = slog.Default()
	}
	return &Handler{
		cfg: cfg,
		log: l.With(slog.String("component", "directoryingest")),
	}, nil
}

// -----------------------------------------------------------------------
// StreamCameraState
// -----------------------------------------------------------------------

// StreamCameraState handles POST /kaivue.v1.DirectoryIngest/StreamCameraState.
//
// The recorder keeps the request body open and streams NDJSON batches. Each
// line is a wireCameraStateRequest. The handler drains lines until EOF, then
// writes a single wireCameraStateResponse line and closes.
func (h *Handler) StreamCameraState() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		tenantID, ok := tenantIDFromContext(ctx)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "missing tenant context")
			return
		}

		var (
			accepted        int64
			rejectedUnknown int64
			rejectedStale   int64
			recorderID      string // set on first batch
		)

		dec := json.NewDecoder(io.LimitReader(r.Body, h.cfg.maxBatchBytes()*100))
		for {
			var req wireCameraStateRequest
			if err := dec.Decode(&req); err != nil {
				if err == io.EOF || err == io.ErrUnexpectedEOF {
					break
				}
				h.log.WarnContext(ctx, "StreamCameraState: decode error", slog.String("error", err.Error()))
				break
			}

			// First batch: verify recorder identity.
			if recorderID == "" {
				recorderID = req.RecorderID
				if recorderID == "" {
					writeErr(w, http.StatusBadRequest, "recorder_id is required in first batch")
					return
				}
				owningTenantID, err := h.cfg.Auth.GetRecorderTenantID(ctx, recorderID)
				if err != nil {
					writeErr(w, http.StatusNotFound, "recorder not found")
					return
				}
				if owningTenantID != tenantID {
					h.log.WarnContext(ctx, "StreamCameraState: cross-tenant attempt",
						"claimed_tenant", tenantID,
						"owning_tenant", owningTenantID,
						"recorder_id", recorderID,
					)
					writeErr(w, http.StatusUnauthorized, "recorder does not belong to tenant")
					return
				}
			}

			for _, u := range req.Updates {
				observedAt := parseTime(u.ObservedAt)
				lastFrameAt := parseTime(u.LastFrameAt)
				cs := CameraState{
					RecorderID:         recorderID,
					TenantID:           tenantID,
					CameraID:           u.CameraID,
					State:              u.State,
					ErrorMessage:       u.ErrorMessage,
					CurrentBitrateKbps: u.CurrentBitrateKbps,
					CurrentFramerate:   u.CurrentFramerate,
					LastFrameAt:        lastFrameAt,
					ConfigVersion:      u.ConfigVersion,
					ObservedAt:         observedAt,
				}
				if err := h.cfg.CameraState.UpsertCameraState(ctx, cs); err != nil {
					h.log.WarnContext(ctx, "StreamCameraState: upsert failed",
						"camera_id", u.CameraID, "error", err)
					rejectedUnknown++
					continue
				}
				accepted++
			}
		}

		h.log.InfoContext(ctx, "StreamCameraState complete",
			"recorder_id", recorderID,
			"accepted", accepted,
			"rejected_unknown", rejectedUnknown,
		)

		w.Header().Set("Content-Type", "application/x-ndjson")
		resp := wireCameraStateResponse{
			Accepted:        accepted,
			RejectedUnknown: rejectedUnknown,
			RejectedStale:   rejectedStale,
		}
		writeJSON(w, resp)
	})
}

// -----------------------------------------------------------------------
// PublishSegmentIndex
// -----------------------------------------------------------------------

// PublishSegmentIndex handles POST /kaivue.v1.DirectoryIngest/PublishSegmentIndex.
func (h *Handler) PublishSegmentIndex() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		tenantID, ok := tenantIDFromContext(ctx)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "missing tenant context")
			return
		}

		var (
			accepted          int64
			rejectedUnknown   int64
			rejectedDuplicate int64
			recorderID        string
		)

		dec := json.NewDecoder(io.LimitReader(r.Body, h.cfg.maxBatchBytes()*100))
		for {
			var req wireSegmentIndexRequest
			if err := dec.Decode(&req); err != nil {
				if err == io.EOF || err == io.ErrUnexpectedEOF {
					break
				}
				h.log.WarnContext(ctx, "PublishSegmentIndex: decode error", slog.String("error", err.Error()))
				break
			}

			if recorderID == "" {
				recorderID = req.RecorderID
				if recorderID == "" {
					writeErr(w, http.StatusBadRequest, "recorder_id is required in first batch")
					return
				}
				owningTenantID, err := h.cfg.Auth.GetRecorderTenantID(ctx, recorderID)
				if err != nil {
					writeErr(w, http.StatusNotFound, "recorder not found")
					return
				}
				if owningTenantID != tenantID {
					h.log.WarnContext(ctx, "PublishSegmentIndex: cross-tenant attempt",
						"claimed_tenant", tenantID,
						"owning_tenant", owningTenantID,
						"recorder_id", recorderID,
					)
					writeErr(w, http.StatusUnauthorized, "recorder does not belong to tenant")
					return
				}
			}

			entries := make([]SegmentIndexEntry, 0, len(req.Entries))
			for _, e := range req.Entries {
				entries = append(entries, SegmentIndexEntry{
					RecorderID:  recorderID,
					TenantID:    tenantID,
					CameraID:    e.CameraID,
					SegmentID:   e.SegmentID,
					StartTime:   parseTime(e.StartTime),
					EndTime:     parseTime(e.EndTime),
					Bytes:       e.Bytes,
					Codec:       e.Codec,
					HasAudio:    e.HasAudio,
					IsEventClip: e.IsEventClip,
					StorageTier: e.StorageTier,
					Sequence:    e.Sequence,
				})
			}

			if len(entries) == 0 {
				continue
			}

			a, rd, err := h.cfg.SegmentIndex.UpsertSegmentEntries(ctx, entries)
			if err != nil {
				h.log.WarnContext(ctx, "PublishSegmentIndex: upsert error",
					"recorder_id", recorderID, "error", err)
				rejectedUnknown += int64(len(entries))
				continue
			}
			accepted += a
			rejectedDuplicate += rd
		}

		h.log.InfoContext(ctx, "PublishSegmentIndex complete",
			"recorder_id", recorderID,
			"accepted", accepted,
			"rejected_duplicate", rejectedDuplicate,
		)

		w.Header().Set("Content-Type", "application/x-ndjson")
		resp := wireSegmentIndexResponse{
			Accepted:          accepted,
			RejectedUnknown:   rejectedUnknown,
			RejectedDuplicate: rejectedDuplicate,
		}
		writeJSON(w, resp)
	})
}

// -----------------------------------------------------------------------
// PublishAIEvents
// -----------------------------------------------------------------------

// PublishAIEvents handles POST /kaivue.v1.DirectoryIngest/PublishAIEvents.
//
// This is the cloud receiver for AI detection events pushed by the Recorder.
// It resolves the publisher TODOs from KAI-281 (object detection),
// KAI-283 (LPR), and KAI-284 (behavioral analysis).
func (h *Handler) PublishAIEvents() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		tenantID, ok := tenantIDFromContext(ctx)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "missing tenant context")
			return
		}

		var (
			accepted        int64
			rejectedUnknown int64
			recorderID      string
		)

		dec := json.NewDecoder(io.LimitReader(r.Body, h.cfg.maxBatchBytes()*100))
		for {
			var req wireAIEventsRequest
			if err := dec.Decode(&req); err != nil {
				if err == io.EOF || err == io.ErrUnexpectedEOF {
					break
				}
				h.log.WarnContext(ctx, "PublishAIEvents: decode error", slog.String("error", err.Error()))
				break
			}

			if recorderID == "" {
				recorderID = req.RecorderID
				if recorderID == "" {
					writeErr(w, http.StatusBadRequest, "recorder_id is required in first batch")
					return
				}
				owningTenantID, err := h.cfg.Auth.GetRecorderTenantID(ctx, recorderID)
				if err != nil {
					writeErr(w, http.StatusNotFound, "recorder not found")
					return
				}
				if owningTenantID != tenantID {
					h.log.WarnContext(ctx, "PublishAIEvents: cross-tenant attempt",
						"claimed_tenant", tenantID,
						"owning_tenant", owningTenantID,
						"recorder_id", recorderID,
					)
					writeErr(w, http.StatusUnauthorized, "recorder does not belong to tenant")
					return
				}
			}

			events := make([]AIEvent, 0, len(req.Events))
			for _, e := range req.Events {
				events = append(events, AIEvent{
					EventID:      e.EventID,
					TenantID:     tenantID,
					RecorderID:   recorderID,
					CameraID:     e.CameraID,
					Kind:         e.Kind,
					KindLabel:    e.KindLabel,
					ObservedAt:   parseTime(e.ObservedAt),
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

			if len(events) == 0 {
				continue
			}

			a, ru, err := h.cfg.AIEvents.InsertAIEvents(ctx, events)
			if err != nil {
				h.log.WarnContext(ctx, "PublishAIEvents: insert error",
					"recorder_id", recorderID, "error", err)
				rejectedUnknown += int64(len(events))
				continue
			}
			accepted += a
			rejectedUnknown += ru
		}

		h.log.InfoContext(ctx, "PublishAIEvents complete",
			"recorder_id", recorderID,
			"accepted", accepted,
			"rejected_unknown", rejectedUnknown,
		)

		w.Header().Set("Content-Type", "application/x-ndjson")
		resp := wireAIEventsResponse{
			Accepted:        accepted,
			RejectedUnknown: rejectedUnknown,
		}
		writeJSON(w, resp)
	})
}

// -----------------------------------------------------------------------
// Context helpers (mirrors recordercontrol — no cross-package import)
// -----------------------------------------------------------------------

type ctxTenantKey struct{}

// WithTenantID returns a child context carrying tenantID. Used by tests
// and by the integration shim in apiserver.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, ctxTenantKey{}, tenantID)
}

func tenantIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ctxTenantKey{}).(string)
	return v, ok && v != ""
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, _ = time.Parse(time.RFC3339, s)
	}
	return t
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":%q}`+"\n", msg)
}

func writeJSON(w http.ResponseWriter, v any) {
	b, _ := json.Marshal(v)
	b = append(b, '\n')
	_, _ = w.Write(b)
}
