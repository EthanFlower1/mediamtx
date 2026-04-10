package recordercontrol

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// -----------------------------------------------------------------------
// Wire-format types (hand-rolled until buf generate / KAI-310)
// -----------------------------------------------------------------------

// assignmentEventKind is the discriminant tag written into the JSON stream.
type assignmentEventKind string

const (
	kindSnapshot      assignmentEventKind = "snapshot"
	kindCameraAdded   assignmentEventKind = "camera_added"
	kindCameraUpdated assignmentEventKind = "camera_updated"
	kindCameraRemoved assignmentEventKind = "camera_removed"
	kindHeartbeat     assignmentEventKind = "heartbeat"
)

// wireEvent is the JSON envelope streamed over the HTTP/1.1 long-poll body.
// When Connect-Go generated code lands (KAI-310) this is replaced by
// proto-over-connect binary framing; the JSON shape here is intentionally
// compatible with the proto field names so the migration is mechanical.
type wireEvent struct {
	Kind      assignmentEventKind `json:"kind"`
	Version   int64               `json:"version"`
	EmittedAt string              `json:"emitted_at"`
	// Exactly one of the following fields is non-nil per event.
	Snapshot *wireSnapshot      `json:"snapshot,omitempty"`
	Added    *wireCameraAdded   `json:"camera_added,omitempty"`
	Updated  *wireCameraUpdated `json:"camera_updated,omitempty"`
	Removed  *wireCameraRemoved `json:"camera_removed,omitempty"`
	// Heartbeat carries no payload beyond the envelope timestamps.
}

type wireSnapshot struct {
	Cameras []wireCamera `json:"cameras"`
}

type wireCamera struct {
	ID            string `json:"id"`
	RecorderID    string `json:"recorder_id"`
	Name          string `json:"name"`
	CredentialRef string `json:"credential_ref"`
	ConfigJSON    string `json:"config_json"`
	ConfigVersion int64  `json:"config_version"`
}

type wireCameraAdded struct {
	Camera wireCamera `json:"camera"`
}

type wireCameraUpdated struct {
	Camera wireCamera `json:"camera"`
}

type wireCameraRemoved struct {
	CameraID        string `json:"camera_id"`
	PurgeRecordings bool   `json:"purge_recordings"`
	Reason          string `json:"reason,omitempty"`
}

// -----------------------------------------------------------------------
// Handler config
// -----------------------------------------------------------------------

const (
	// heartbeatInterval is how frequently the server sends a Heartbeat
	// event while the stream is otherwise idle.
	defaultHeartbeatInterval = 30 * time.Second
)

// Config configures the streaming handler.
type Config struct {
	// Bus is the in-process pub/sub hub. Required.
	Bus *EventBus
	// Store is the SQLite-backed camera assignment store. Required.
	Store *Store
	// Logger is the structured logger. Nil defaults to slog.Default().
	Logger *slog.Logger
	// HeartbeatInterval overrides the default heartbeat interval. Zero uses
	// the default (30s).
	HeartbeatInterval time.Duration
	// RecorderAuthenticator extracts and validates the recorder_id from the
	// HTTP request. In production this is backed by mTLS cert validation or
	// a bearer token check. For tests a simple header extractor suffices.
	RecorderAuthenticator func(r *http.Request) (recorderID string, ok bool)
}

func (c *Config) validate() error {
	if c.Bus == nil {
		return errors.New("recordercontrol: Bus is required")
	}
	if c.Store == nil {
		return errors.New("recordercontrol: Store is required")
	}
	if c.RecorderAuthenticator == nil {
		return errors.New("recordercontrol: RecorderAuthenticator is required")
	}
	return nil
}

func (c *Config) heartbeat() time.Duration {
	if c.HeartbeatInterval > 0 {
		return c.HeartbeatInterval
	}
	return defaultHeartbeatInterval
}

// -----------------------------------------------------------------------
// Handler
// -----------------------------------------------------------------------

// Handler is the http.Handler for the on-prem Directory's
// RecorderControl.StreamAssignments RPC.
//
// It is intentionally a plain net/http handler so it can be mounted in the
// existing NVR Gin mux today and swapped for a connect-go handler when
// KAI-310 lands without touching any business logic.
//
// Path: /kaivue.v1.RecorderControlService/StreamAssignments
type Handler struct {
	cfg Config
	log *slog.Logger
}

// NewHandler constructs a validated Handler. Returns an error if any
// required Config field is missing.
func NewHandler(cfg Config) (*Handler, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Handler{
		cfg: cfg,
		log: cfg.Logger.With(slog.String("component", "directory/recordercontrol")),
	}, nil
}

// ServeHTTP implements http.Handler.
//
// The stream request flow:
//  1. Authenticate the recorder via RecorderAuthenticator.
//  2. Parse the request body for recorder_id + known_version.
//  3. Verify the recorder is enrolled.
//  4. Send initial Snapshot of all assigned cameras.
//  5. Stream incremental events (added/updated/removed) as they arrive on the bus.
//  6. Send heartbeat pings at the configured interval.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// --- Authenticate ---
	authRecorderID, ok := h.cfg.RecorderAuthenticator(r)
	if !ok || authRecorderID == "" {
		h.writeError(w, http.StatusUnauthorized, "recorder authentication failed")
		return
	}

	// --- Parse request ---
	var req struct {
		RecorderID              string `json:"recorder_id"`
		KnownVersion            int64  `json:"known_version"`
		RecorderSoftwareVersion string `json:"recorder_software_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.RecorderID == "" {
		h.writeError(w, http.StatusBadRequest, "recorder_id is required")
		return
	}

	// Verify the authenticated recorder matches the request.
	if authRecorderID != req.RecorderID {
		h.log.WarnContext(ctx, "recorder ID mismatch",
			"auth_recorder", authRecorderID,
			"request_recorder", req.RecorderID,
		)
		h.writeError(w, http.StatusForbidden, "recorder_id mismatch")
		return
	}

	// --- Verify recorder is enrolled ---
	exists, err := h.cfg.Store.RecorderExists(ctx, req.RecorderID)
	if err != nil {
		h.log.ErrorContext(ctx, "recorder existence check failed",
			"recorder_id", req.RecorderID, "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !exists {
		h.writeError(w, http.StatusNotFound, "recorder not found")
		return
	}

	h.log.InfoContext(ctx, "StreamAssignments stream opened",
		"recorder_id", req.RecorderID,
		"known_version", req.KnownVersion,
		"software_version", req.RecorderSoftwareVersion,
	)

	// --- Set streaming headers ---
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Accel-Buffering", "no") // nginx: disable proxy buffering
	w.WriteHeader(http.StatusOK)

	flusher, canFlush := w.(http.Flusher)

	// --- Load and send initial Snapshot ---
	cameras, err := h.cfg.Store.ListCamerasForRecorder(ctx, req.RecorderID)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to load cameras for snapshot",
			"recorder_id", req.RecorderID, "error", err)
		// Already wrote 200 — write a heartbeat and close. Recorder will reconnect.
		_ = h.writeEvent(w, wireEvent{
			Kind:      kindHeartbeat,
			EmittedAt: time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	snapshotWire := cameraRowsToWire(cameras)
	snapshotEvent := wireEvent{
		Kind:      kindSnapshot,
		Version:   h.cfg.Bus.CurrentVersion(),
		EmittedAt: time.Now().UTC().Format(time.RFC3339),
		Snapshot:  &wireSnapshot{Cameras: snapshotWire},
	}
	if err := h.writeEvent(w, snapshotEvent); err != nil {
		return // client gone
	}
	if canFlush {
		flusher.Flush()
	}

	// --- Subscribe to incremental events ---
	eventCh, unsubscribe := h.cfg.Bus.Subscribe(req.RecorderID)
	defer unsubscribe()

	heartbeatTicker := time.NewTicker(h.cfg.heartbeat())
	defer heartbeatTicker.Stop()

	needsFullResync := false

	for {
		select {
		case <-ctx.Done():
			// Client disconnected.
			h.log.InfoContext(ctx, "StreamAssignments stream closed",
				"recorder_id", req.RecorderID)
			return

		case <-heartbeatTicker.C:
			ev := wireEvent{
				Kind:      kindHeartbeat,
				EmittedAt: time.Now().UTC().Format(time.RFC3339),
			}
			if err := h.writeEvent(w, ev); err != nil {
				return
			}

			if canFlush {
				flusher.Flush()
			}

		case busEv, ok := <-eventCh:
			if !ok {
				// Channel closed by Unsubscribe.
				return
			}

			if busEv.Kind == EventKindForceResync || needsFullResync {
				cameras, err := h.cfg.Store.ListCamerasForRecorder(ctx, req.RecorderID)
				if err != nil {
					h.log.ErrorContext(ctx, "resync: failed to reload cameras",
						"recorder_id", req.RecorderID, "error", err)
					return
				}
				ev := wireEvent{
					Kind:      kindSnapshot,
					Version:   busEv.Version,
					EmittedAt: time.Now().UTC().Format(time.RFC3339),
					Snapshot:  &wireSnapshot{Cameras: cameraRowsToWire(cameras)},
				}
				if err := h.writeEvent(w, ev); err != nil {
					return
				}
				needsFullResync = false
	
				if canFlush {
					flusher.Flush()
				}
				continue
			}

			var ev wireEvent
			switch busEv.Kind {
			case EventKindCameraAdded:
				if busEv.Camera == nil {
					continue
				}
				ev = wireEvent{
					Kind:      kindCameraAdded,
					Version:   busEv.Version,
					EmittedAt: busEv.EmittedAt.Format(time.RFC3339),
					Added:     &wireCameraAdded{Camera: cameraRowToWire(busEv.Camera)},
				}
			case EventKindCameraUpdated:
				if busEv.Camera == nil {
					continue
				}
				ev = wireEvent{
					Kind:      kindCameraUpdated,
					Version:   busEv.Version,
					EmittedAt: busEv.EmittedAt.Format(time.RFC3339),
					Updated:   &wireCameraUpdated{Camera: cameraRowToWire(busEv.Camera)},
				}
			case EventKindCameraRemoved:
				if busEv.Removal == nil {
					continue
				}
				ev = wireEvent{
					Kind:    kindCameraRemoved,
					Version: busEv.Version,
					EmittedAt: busEv.EmittedAt.Format(time.RFC3339),
					Removed: &wireCameraRemoved{
						CameraID:        busEv.Removal.CameraID,
						PurgeRecordings: busEv.Removal.PurgeRecordings,
						Reason:          busEv.Removal.Reason,
					},
				}
			default:
				continue
			}

			if err := h.writeEvent(w, ev); err != nil {
				return
			}

			if canFlush {
				flusher.Flush()
			}
		}
	}
}

// writeEvent serialises ev as a newline-delimited JSON record.
func (h *Handler) writeEvent(w http.ResponseWriter, ev wireEvent) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

// writeError writes a compact JSON error body and status.
// Safe to call before the 200 is written; must NOT be called after.
func (h *Handler) writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":%q}`+"\n", msg)
}

// -----------------------------------------------------------------------
// Conversion helpers
// -----------------------------------------------------------------------

func cameraRowsToWire(cameras []CameraRow) []wireCamera {
	out := make([]wireCamera, len(cameras))
	for i, c := range cameras {
		out[i] = cameraRowToWire(&c)
	}
	return out
}

func cameraRowToWire(c *CameraRow) wireCamera {
	return wireCamera{
		ID:            c.CameraID,
		RecorderID:    c.RecorderID,
		Name:          c.Name,
		CredentialRef: c.CredentialRef,
		ConfigJSON:    c.ConfigJSON,
		ConfigVersion: c.ConfigVersion,
	}
}

