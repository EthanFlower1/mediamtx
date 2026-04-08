package recordercontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// -----------------------------------------------------------------------
// Domain types
// -----------------------------------------------------------------------

// RecorderStatus is the lifecycle state persisted to on_prem_directories.status
// by the disconnect-detection logic.
type RecorderStatus string

const (
	RecorderStatusOnline   RecorderStatus = "online"
	RecorderStatusDegraded RecorderStatus = "degraded"
	RecorderStatusOffline  RecorderStatus = "offline"
)

// -----------------------------------------------------------------------
// Store interfaces (injected dependencies — easy to stub in tests)
// -----------------------------------------------------------------------

// CameraStore is the query seam the Handler uses to load cameras.
//
// TODO(KAI-249): when the cloud Directory schema (KAI-249) lands, this
// interface should be backed by a real SQL query against the cameras table
// scoped to (tenantID, recorderID). Until then the Handler is wired to
// an in-process stub.
type CameraStore interface {
	// ListCamerasForRecorder returns all cameras assigned to the given
	// recorder within the given tenant. MUST return an empty slice (not
	// nil) when no cameras are assigned.
	//
	// Multi-tenant invariant: implementations MUST include tenantID in
	// the query predicate. Returning cameras from another tenant is the
	// canonical multi-tenant isolation bug.
	ListCamerasForRecorder(ctx context.Context, tenantID, recorderID string) ([]CameraPayload, error)
}

// RecorderStore is the seam for persisting recorder health state.
//
// Today this maps to on_prem_directories.status (KAI-218). When
// KAI-249 lands a dedicated recorders table may supersede this.
type RecorderStore interface {
	// UpdateRecorderStatus sets the recorder's operational status.
	// Must be tenant-scoped — implementations MUST include tenantID.
	UpdateRecorderStatus(ctx context.Context, tenantID, recorderID string, status RecorderStatus) error
	// GetRecorderTenantID returns the tenant that owns this recorder.
	// The Handler calls this to verify the recorder_id in the request
	// matches the mTLS identity before streaming any data.
	GetRecorderTenantID(ctx context.Context, recorderID string) (string, error)
}

// -----------------------------------------------------------------------
// Wire-format types (hand-rolled until buf generate / KAI-310)
// -----------------------------------------------------------------------

// assignmentEventKind is the discriminant tag written into the JSON stream.
type assignmentEventKind string

const (
	kindSnapshot     assignmentEventKind = "snapshot"
	kindCameraAdded  assignmentEventKind = "camera_added"
	kindCameraUpdated assignmentEventKind = "camera_updated"
	kindCameraRemoved assignmentEventKind = "camera_removed"
	kindHeartbeat    assignmentEventKind = "heartbeat"
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
	Snapshot *wireSnapshot       `json:"snapshot,omitempty"`
	Added    *wireCameraAdded    `json:"camera_added,omitempty"`
	Updated  *wireCameraUpdated  `json:"camera_updated,omitempty"`
	Removed  *wireCameraRemoved  `json:"camera_removed,omitempty"`
	// Heartbeat carries no payload beyond the envelope timestamps.
}

type wireSnapshot struct {
	Cameras []wireCamera `json:"cameras"`
}

type wireCamera struct {
	ID            string `json:"id"`
	TenantID      string `json:"tenant_id"`
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
	heartbeatInterval = 30 * time.Second

	// degradedAfter is the duration of missed heartbeats before the
	// recorder is marked DEGRADED.
	degradedAfter = 30 * time.Second

	// offlineAfter is the duration of missed heartbeats before the
	// recorder is marked OFFLINE.
	offlineAfter = 90 * time.Second
)

// Config configures the streaming handler.
type Config struct {
	// Bus is the in-process pub/sub hub. Required.
	Bus *EventBus
	// Cameras is the camera query seam. Required.
	Cameras CameraStore
	// Recorders is the recorder state seam. Required.
	Recorders RecorderStore
	// Logger is the structured logger. Nil defaults to slog.Default().
	Logger *slog.Logger
	// Region is this server's region tag (e.g. "us-east-2"). Required for
	// multi-region audit fields.
	Region string
	// HeartbeatInterval overrides heartbeatInterval. Zero uses the default.
	HeartbeatInterval time.Duration
}

func (c *Config) validate() error {
	if c.Bus == nil {
		return errors.New("recordercontrol: Bus is required")
	}
	if c.Cameras == nil {
		return errors.New("recordercontrol: Cameras is required")
	}
	if c.Recorders == nil {
		return errors.New("recordercontrol: Recorders is required")
	}
	return nil
}

func (c *Config) heartbeat() time.Duration {
	if c.HeartbeatInterval > 0 {
		return c.HeartbeatInterval
	}
	return heartbeatInterval
}

// -----------------------------------------------------------------------
// Handler
// -----------------------------------------------------------------------

// Handler is the http.Handler for the RecorderControl.StreamAssignments RPC.
//
// It is intentionally a plain net/http handler so it can be mounted in the
// existing apiserver mux today and swapped for a connect-go handler when
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
		log: cfg.Logger.With(slog.String("component", "recordercontrol")),
	}, nil
}

// ServeHTTP implements http.Handler.
//
// Authentication: the caller must have passed the standard auth middleware
// chain (KAI-226) before reaching this handler. The handler additionally
// verifies that the recorder_id in the request body belongs to the tenant
// derived from the bearer token — this is the mTLS identity check that
// guards against cross-tenant recorder spoofing.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

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

	// --- Multi-tenant identity verification ---
	// The recorder_id in the request MUST belong to the same tenant as
	// the authenticated session. We look up the owning tenant from the
	// recorder store and compare against the authenticated tenant from ctx.
	claimedTenantID, ok := tenantIDFromContext(ctx)
	if !ok || claimedTenantID == "" {
		h.writeError(w, http.StatusUnauthorized, "missing tenant context")
		return
	}
	owningTenantID, err := h.cfg.Recorders.GetRecorderTenantID(ctx, req.RecorderID)
	if err != nil {
		h.log.WarnContext(ctx, "recorder tenant lookup failed",
			"recorder_id", req.RecorderID,
			"error", err,
		)
		h.writeError(w, http.StatusNotFound, "recorder not found")
		return
	}
	if owningTenantID != claimedTenantID {
		// Cross-tenant recorder spoof attempt — log at warn for security
		// visibility, return 404 to avoid confirming the recorder exists.
		h.log.WarnContext(ctx, "cross-tenant recorder access rejected",
			"claimed_tenant", claimedTenantID,
			"owning_tenant", owningTenantID,
			"recorder_id", req.RecorderID,
		)
		h.writeError(w, http.StatusNotFound, "recorder not found")
		return
	}
	tenantID := owningTenantID

	h.log.InfoContext(ctx, "StreamAssignments stream opened",
		"recorder_id", req.RecorderID,
		"tenant_id", tenantID,
		"known_version", req.KnownVersion,
		"software_version", req.RecorderSoftwareVersion,
	)

	// --- Mark recorder online ---
	if err := h.cfg.Recorders.UpdateRecorderStatus(ctx, tenantID, req.RecorderID, RecorderStatusOnline); err != nil {
		h.log.WarnContext(ctx, "failed to mark recorder online",
			"recorder_id", req.RecorderID,
			"error", err,
		)
		// Non-fatal: continue streaming.
	}

	// --- Set streaming headers ---
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Accel-Buffering", "no") // nginx: disable proxy buffering
	w.WriteHeader(http.StatusOK)

	flusher, canFlush := w.(http.Flusher)

	// --- Load and send initial Snapshot ---
	cameras, err := h.cfg.Cameras.ListCamerasForRecorder(ctx, tenantID, req.RecorderID)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to load cameras for snapshot",
			"recorder_id", req.RecorderID,
			"tenant_id", tenantID,
			"error", err,
		)
		// We've already written 200 — write an error event on the stream
		// and close. The recorder will reconnect.
		_ = h.writeEvent(w, wireEvent{
			Kind:      kindHeartbeat, // reuse heartbeat as a ping-before-close
			EmittedAt: time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	snapshotWire := cameraPayloadsToWire(cameras)
	snapshotEvent := wireEvent{
		Kind:      kindSnapshot,
		Version:   h.cfg.Bus.currentVersion(),
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
	eventCh, unsubscribe := h.cfg.Bus.Subscribe(tenantID, req.RecorderID)
	defer unsubscribe()

	heartbeatTicker := time.NewTicker(h.cfg.heartbeat())
	defer heartbeatTicker.Stop()

	// degradedTimer fires degradedAfter after the last received heartbeat
	// acknowledgement. Since this is server-push only (no client ACK in
	// this wire format), we use connection liveness as the proxy: if we
	// cannot write to the stream, the Recorder has gone away.
	var (
		lastSuccessfulWrite = time.Now()
		healthMu            sync.Mutex
	)
	markLastWrite := func() {
		healthMu.Lock()
		lastSuccessfulWrite = time.Now()
		healthMu.Unlock()
	}
	healthChecker := time.NewTicker(10 * time.Second)
	defer healthChecker.Stop()

	needsFullResync := false

	for {
		select {
		case <-ctx.Done():
			// Client disconnected normally.
			h.onDisconnect(ctx, tenantID, req.RecorderID, lastSuccessfulWrite)
			return

		case <-heartbeatTicker.C:
			ev := wireEvent{
				Kind:      kindHeartbeat,
				EmittedAt: time.Now().UTC().Format(time.RFC3339),
			}
			if err := h.writeEvent(w, ev); err != nil {
				h.onDisconnect(ctx, tenantID, req.RecorderID, lastSuccessfulWrite)
				return
			}
			markLastWrite()
			if canFlush {
				flusher.Flush()
			}

		case <-healthChecker.C:
			healthMu.Lock()
			age := time.Since(lastSuccessfulWrite)
			healthMu.Unlock()

			if age >= offlineAfter {
				_ = h.cfg.Recorders.UpdateRecorderStatus(ctx, tenantID, req.RecorderID, RecorderStatusOffline)
				return
			} else if age >= degradedAfter {
				_ = h.cfg.Recorders.UpdateRecorderStatus(ctx, tenantID, req.RecorderID, RecorderStatusDegraded)
			}

		case busEv, ok := <-eventCh:
			if !ok {
				// Channel closed by Unsubscribe — stream is done.
				return
			}

			if busEv.Kind == EventKindForceResync || needsFullResync {
				// Send a fresh Snapshot rather than delivering the
				// partial event sequence the recorder missed.
				cameras, err := h.cfg.Cameras.ListCamerasForRecorder(ctx, tenantID, req.RecorderID)
				if err != nil {
					h.log.ErrorContext(ctx, "resync: failed to reload cameras",
						"recorder_id", req.RecorderID, "error", err)
					return
				}
				ev := wireEvent{
					Kind:      kindSnapshot,
					Version:   busEv.Version,
					EmittedAt: time.Now().UTC().Format(time.RFC3339),
					Snapshot:  &wireSnapshot{Cameras: cameraPayloadsToWire(cameras)},
				}
				if err := h.writeEvent(w, ev); err != nil {
					return
				}
				needsFullResync = false
				markLastWrite()
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
					Added:     &wireCameraAdded{Camera: cameraPayloadToWire(busEv.Camera)},
				}
			case EventKindCameraUpdated:
				if busEv.Camera == nil {
					continue
				}
				ev = wireEvent{
					Kind:      kindCameraUpdated,
					Version:   busEv.Version,
					EmittedAt: busEv.EmittedAt.Format(time.RFC3339),
					Updated:   &wireCameraUpdated{Camera: cameraPayloadToWire(busEv.Camera)},
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
				needsFullResync = true
				h.onDisconnect(ctx, tenantID, req.RecorderID, lastSuccessfulWrite)
				return
			}
			markLastWrite()
			if canFlush {
				flusher.Flush()
			}
		}
	}
}

// onDisconnect is called whenever the streaming loop exits — clean disconnect
// or write error. It transitions the recorder to DEGRADED initially; the
// health-checker goroutine / next connect call will update further.
func (h *Handler) onDisconnect(ctx context.Context, tenantID, recorderID string, lastWrite time.Time) {
	age := time.Since(lastWrite)
	status := RecorderStatusDegraded
	if age >= offlineAfter {
		status = RecorderStatusOffline
	}
	if err := h.cfg.Recorders.UpdateRecorderStatus(
		context.Background(), // ctx may already be cancelled
		tenantID, recorderID, status,
	); err != nil {
		h.log.Warn("onDisconnect: failed to update recorder status",
			"recorder_id", recorderID,
			"status", status,
			"error", err,
		)
	}
	h.log.InfoContext(ctx, "StreamAssignments stream closed",
		"recorder_id", recorderID,
		"tenant_id", tenantID,
		"final_status", status,
	)
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

func cameraPayloadsToWire(cameras []CameraPayload) []wireCamera {
	out := make([]wireCamera, len(cameras))
	for i, c := range cameras {
		out[i] = cameraPayloadToWire(&c)
	}
	return out
}

func cameraPayloadToWire(c *CameraPayload) wireCamera {
	return wireCamera{
		ID:            c.ID,
		TenantID:      c.TenantID,
		RecorderID:    c.RecorderID,
		Name:          c.Name,
		CredentialRef: c.CredentialRef,
		ConfigJSON:    c.ConfigJSON,
		ConfigVersion: c.ConfigVersion,
	}
}

// -----------------------------------------------------------------------
// Context plumbing — thin helpers so the package doesn't import apiserver
// -----------------------------------------------------------------------

type ctxTenantKey struct{}

// WithTenantID returns a child context carrying tenantID. Used by tests
// and by the integration shim in apiserver.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, ctxTenantKey{}, tenantID)
}

// tenantIDFromContext extracts the tenantID from ctx.
func tenantIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ctxTenantKey{}).(string)
	return v, ok && v != ""
}

// -----------------------------------------------------------------------
// EventBus helper — expose current version without locking externally
// -----------------------------------------------------------------------

// currentVersion returns the latest published version counter. Used when
// constructing Snapshot events so the version is consistent with what
// the bus last emitted.
func (b *EventBus) currentVersion() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.version
}
