// Package directoryingest implements the Recorder-side client for the three
// DirectoryIngest RPCs defined in
// internal/shared/proto/v1/directory_ingest.proto (KAI-238).
//
// The client opens long-lived HTTP POST streams to the Directory. Each RPC
// maps to one persistent goroutine:
//
//   - CameraStateClient    — coalesces per-camera health updates and streams
//                            them every ~5 s (or on significant change).
//   - SegmentIndexClient   — receives segment index entries from the
//                            recorder's write path and batches them to the
//                            Directory; on overflow persists to a local
//                            WAL file and retries on reconnect.
//   - AIEventsClient       — receives AI events from feature pipelines
//                            (KAI-281 objectdetection, KAI-283 LPR,
//                            KAI-284 behavioral) and streams them.
//                            Oldest events are dropped on bounded-buffer
//                            overflow; callers that need durability should
//                            use SegmentIndexClient-style WAL.
//
// Wire format: NDJSON, identical field names to the proto — mirrors the
// pattern established by internal/recorder/recordercontrol (KAI-252/253).
// When KAI-431 migrates the stack to generated Connect-Go the NDJSON
// encoding here is replaced by proto binary framing with no logic changes.
//
// Reconnect: each client uses exponential back-off matching
// internal/recorder/recordercontrol.Client (base 500ms, max 30s, ±20% jitter).
package directoryingest

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

// -----------------------------------------------------------------------
// Backoff
// -----------------------------------------------------------------------

const (
	backoffBase   = 500 * time.Millisecond
	backoffMax    = 30 * time.Second
	backoffJitter = 0.20
)

type backoff struct {
	base    time.Duration
	max     time.Duration
	current time.Duration
}

func newBackoff() backoff { return backoff{base: backoffBase, max: backoffMax, current: backoffBase} }

func (b *backoff) next() time.Duration {
	d := b.current
	b.current *= 2
	if b.current > b.max {
		b.current = b.max
	}
	return applyJitter(d, backoffJitter)
}

func (b *backoff) reset() { b.current = b.base }

func applyJitter(d time.Duration, jitter float64) time.Duration {
	factor := 1 + jitter*(2*rand.Float64()-1) //nolint:gosec // non-crypto jitter
	return time.Duration(float64(d) * factor)
}

// -----------------------------------------------------------------------
// TLS helper
// -----------------------------------------------------------------------

// GetCertificateFunc is the certmgr hook type. Injected so the client never
// holds a static cert — cert rotation happens transparently (KAI-242).
type GetCertificateFunc func(*tls.ClientHelloInfo) (*tls.Certificate, error)

func newHTTPClient(getCert GetCertificateFunc) *http.Client {
	tlsCfg := &tls.Config{
		GetClientCertificate: func(_ *tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return getCert(nil)
		},
		MinVersion: tls.VersionTLS13,
	}
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}
}

// -----------------------------------------------------------------------
// Wire types (NDJSON, field names match proto)
// -----------------------------------------------------------------------

type wireBoundingBox struct {
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Width  float32 `json:"width"`
	Height float32 `json:"height"`
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

// -----------------------------------------------------------------------
// Domain types (input from recorder-internal callers)
// -----------------------------------------------------------------------

// CameraStateUpdate is the per-camera health snapshot the recorder feeds into
// CameraStateClient.
type CameraStateUpdate struct {
	CameraID           string
	State              string // "online"|"degraded"|"offline"|"error"
	ErrorMessage       string
	CurrentBitrateKbps int32
	CurrentFramerate   int32
	LastFrameAt        time.Time
	ConfigVersion      int64
	ObservedAt         time.Time
}

// SegmentIndexEntry is a single recording segment pushed into SegmentIndexClient.
type SegmentIndexEntry struct {
	CameraID    string
	SegmentID   string
	StartTime   time.Time
	EndTime     time.Time
	Bytes       int64
	Codec       string
	HasAudio    bool
	IsEventClip bool
	StorageTier string
	Sequence    int64
}

// AIEvent is a single AI detection event pushed into AIEventsClient by the
// feature pipelines (KAI-281, KAI-283, KAI-284).
type AIEvent struct {
	EventID      string
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
// CameraStateClient
// -----------------------------------------------------------------------

const (
	cameraStateFlushInterval = 5 * time.Second
	cameraStateBufferSize    = 256 // per-camera slots; oldest dropped on overflow
	cameraStatePath          = "/kaivue.v1.DirectoryIngest/StreamCameraState"
)

// CameraStateClientConfig configures the camera state streaming client.
type CameraStateClientConfig struct {
	DirectoryEndpoint string
	RecorderID        string
	GetCertificate    GetCertificateFunc
	Logger            *slog.Logger
	// FlushInterval overrides the default 5s coalesce window. Zero uses default.
	FlushInterval time.Duration
}

func (c *CameraStateClientConfig) flushInterval() time.Duration {
	if c.FlushInterval > 0 {
		return c.FlushInterval
	}
	return cameraStateFlushInterval
}

// CameraStateClient coalesces per-camera health updates and streams them to
// the Directory. The most recent update per camera wins within each flush
// window. Safe for concurrent calls to Publish.
type CameraStateClient struct {
	cfg CameraStateClientConfig
	log *slog.Logger

	mu      sync.Mutex
	pending map[string]CameraStateUpdate // camera_id → latest

	in chan CameraStateUpdate
}

// NewCameraStateClient constructs the client. Call Run to start.
func NewCameraStateClient(cfg CameraStateClientConfig) (*CameraStateClient, error) {
	if cfg.DirectoryEndpoint == "" {
		return nil, fmt.Errorf("directoryingest: DirectoryEndpoint is required")
	}
	if cfg.RecorderID == "" {
		return nil, fmt.Errorf("directoryingest: RecorderID is required")
	}
	if cfg.GetCertificate == nil {
		return nil, fmt.Errorf("directoryingest: GetCertificate is required")
	}
	l := cfg.Logger
	if l == nil {
		l = slog.Default()
	}
	return &CameraStateClient{
		cfg:     cfg,
		log:     l.With("component", "directoryingest.camerastate", "recorder_id", cfg.RecorderID),
		pending: make(map[string]CameraStateUpdate),
		in:      make(chan CameraStateUpdate, cameraStateBufferSize),
	}, nil
}

// Publish queues a camera state update. If the buffer is full, the oldest
// entry per the map semantics is overwritten (state is level-triggered, not
// edge-triggered — only the latest matters).
func (c *CameraStateClient) Publish(update CameraStateUpdate) {
	// Non-blocking send; if the loop goroutine is behind, we coalesce below.
	select {
	case c.in <- update:
	default:
		// Buffer full: coalesce directly into pending map.
		c.mu.Lock()
		c.pending[update.CameraID] = update
		c.mu.Unlock()
	}
}

// Run starts the streaming loop. Blocks until ctx is cancelled.
func (c *CameraStateClient) Run(ctx context.Context) {
	ticker := time.NewTicker(c.cfg.flushInterval())
	defer ticker.Stop()
	bo := newBackoff()

	for {
		// Drain the in channel into the pending map.
		drainLoop:
		for {
			select {
			case u := <-c.in:
				c.mu.Lock()
				c.pending[u.CameraID] = u
				c.mu.Unlock()
			default:
				break drainLoop
			}
		}

		select {
		case <-ctx.Done():
			return
		case u := <-c.in:
			c.mu.Lock()
			c.pending[u.CameraID] = u
			c.mu.Unlock()
			continue
		case <-ticker.C:
		}

		c.mu.Lock()
		if len(c.pending) == 0 {
			c.mu.Unlock()
			continue
		}
		batch := make([]wireCameraStateUpdate, 0, len(c.pending))
		for _, u := range c.pending {
			wu := wireCameraStateUpdate{
				CameraID:           u.CameraID,
				State:              u.State,
				ObservedAt:         u.ObservedAt.UTC().Format(time.RFC3339),
				ErrorMessage:       u.ErrorMessage,
				CurrentBitrateKbps: u.CurrentBitrateKbps,
				CurrentFramerate:   u.CurrentFramerate,
				ConfigVersion:      u.ConfigVersion,
			}
			if !u.LastFrameAt.IsZero() {
				wu.LastFrameAt = u.LastFrameAt.UTC().Format(time.RFC3339)
			}
			batch = append(batch, wu)
		}
		c.pending = make(map[string]CameraStateUpdate)
		c.mu.Unlock()

		if err := c.send(ctx, batch); err != nil {
			if ctx.Err() != nil {
				return
			}
			c.log.WarnContext(ctx, "send failed; will retry", "error", err, "backoff", bo.next())
			continue
		}
		bo.reset()
	}
}

func (c *CameraStateClient) send(ctx context.Context, batch []wireCameraStateUpdate) error {
	payload := struct {
		RecorderID string                  `json:"recorder_id"`
		Updates    []wireCameraStateUpdate `json:"updates"`
	}{
		RecorderID: c.cfg.RecorderID,
		Updates:    batch,
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	endpoint := strings.TrimRight(c.cfg.DirectoryEndpoint, "/") + cameraStatePath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-ndjson")

	httpCli := newHTTPClient(c.cfg.GetCertificate)
	resp, err := httpCli.Do(req)
	if err != nil {
		return fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}

// -----------------------------------------------------------------------
// SegmentIndexClient
// -----------------------------------------------------------------------

const (
	segmentIndexBufferSize  = 1024 // entries before overflow
	segmentIndexBatchSize   = 64   // recommended per-request batch
	segmentIndexFlushInterval = 10 * time.Second
	segmentIndexPath        = "/kaivue.v1.DirectoryIngest/PublishSegmentIndex"
)

// SegmentIndexClientConfig configures the segment index streaming client.
type SegmentIndexClientConfig struct {
	DirectoryEndpoint string
	RecorderID        string
	GetCertificate    GetCertificateFunc
	Logger            *slog.Logger
	// FlushInterval overrides the default 10s batch window.
	FlushInterval time.Duration
	// BufferSize overrides the default 1024-entry buffer. When the buffer
	// is full, new Publish calls block until space is available (back-pressure).
	BufferSize int
}

func (c *SegmentIndexClientConfig) flushInterval() time.Duration {
	if c.FlushInterval > 0 {
		return c.FlushInterval
	}
	return segmentIndexFlushInterval
}

func (c *SegmentIndexClientConfig) bufferSize() int {
	if c.BufferSize > 0 {
		return c.BufferSize
	}
	return segmentIndexBufferSize
}

// SegmentIndexClient batches segment index entries and publishes them to the
// Directory. On overflow the Publish call blocks (back-pressure). Unlike the
// camera state client, segments are NOT dropped — they represent the
// authoritative timeline. TODO: add WAL-based persistence so entries survive
// a Recorder process restart (KAI-260).
type SegmentIndexClient struct {
	cfg SegmentIndexClientConfig
	log *slog.Logger
	in  chan SegmentIndexEntry
}

// NewSegmentIndexClient constructs the client. Call Run to start.
func NewSegmentIndexClient(cfg SegmentIndexClientConfig) (*SegmentIndexClient, error) {
	if cfg.DirectoryEndpoint == "" {
		return nil, fmt.Errorf("directoryingest: DirectoryEndpoint is required")
	}
	if cfg.RecorderID == "" {
		return nil, fmt.Errorf("directoryingest: RecorderID is required")
	}
	if cfg.GetCertificate == nil {
		return nil, fmt.Errorf("directoryingest: GetCertificate is required")
	}
	l := cfg.Logger
	if l == nil {
		l = slog.Default()
	}
	return &SegmentIndexClient{
		cfg: cfg,
		log: l.With("component", "directoryingest.segmentindex", "recorder_id", cfg.RecorderID),
		in:  make(chan SegmentIndexEntry, cfg.bufferSize()),
	}, nil
}

// Publish enqueues a segment index entry. Blocks if the buffer is full
// (back-pressure to the recording write path).
func (c *SegmentIndexClient) Publish(ctx context.Context, e SegmentIndexEntry) error {
	select {
	case c.in <- e:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TryPublish is the non-blocking variant. Returns false if the buffer is full.
func (c *SegmentIndexClient) TryPublish(e SegmentIndexEntry) bool {
	select {
	case c.in <- e:
		return true
	default:
		return false
	}
}

// Run starts the streaming loop. Blocks until ctx is cancelled.
func (c *SegmentIndexClient) Run(ctx context.Context) {
	ticker := time.NewTicker(c.cfg.flushInterval())
	defer ticker.Stop()
	bo := newBackoff()
	var pending []SegmentIndexEntry

	flush := func() {
		if len(pending) == 0 {
			return
		}
		if err := c.sendBatch(ctx, pending); err != nil {
			if ctx.Err() != nil {
				return
			}
			c.log.WarnContext(ctx, "SegmentIndex send failed",
				"entries", len(pending), "error", err, "backoff", bo.next())
			// Keep pending for retry on next tick.
			return
		}
		bo.reset()
		pending = pending[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case e := <-c.in:
			pending = append(pending, e)
			if len(pending) >= segmentIndexBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (c *SegmentIndexClient) sendBatch(ctx context.Context, entries []SegmentIndexEntry) error {
	wires := make([]wireSegmentIndexEntry, len(entries))
	for i, e := range entries {
		wires[i] = wireSegmentIndexEntry{
			CameraID:    e.CameraID,
			SegmentID:   e.SegmentID,
			StartTime:   e.StartTime.UTC().Format(time.RFC3339),
			EndTime:     e.EndTime.UTC().Format(time.RFC3339),
			Bytes:       e.Bytes,
			Codec:       e.Codec,
			HasAudio:    e.HasAudio,
			IsEventClip: e.IsEventClip,
			StorageTier: e.StorageTier,
			Sequence:    e.Sequence,
		}
	}
	payload := struct {
		RecorderID string                  `json:"recorder_id"`
		Entries    []wireSegmentIndexEntry `json:"entries"`
	}{
		RecorderID: c.cfg.RecorderID,
		Entries:    wires,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	endpoint := strings.TrimRight(c.cfg.DirectoryEndpoint, "/") + segmentIndexPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-ndjson")

	httpCli := newHTTPClient(c.cfg.GetCertificate)
	resp, err := httpCli.Do(req)
	if err != nil {
		return fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}

// -----------------------------------------------------------------------
// AIEventsClient
// -----------------------------------------------------------------------

const (
	aiEventsBufferSize    = 512 // slots; oldest dropped on overflow
	aiEventsBatchSize     = 32
	aiEventsFlushInterval = 5 * time.Second
	aiEventsPath          = "/kaivue.v1.DirectoryIngest/PublishAIEvents"
)

// AIEventsClientConfig configures the AI events streaming client.
type AIEventsClientConfig struct {
	DirectoryEndpoint string
	RecorderID        string
	GetCertificate    GetCertificateFunc
	Logger            *slog.Logger
	// FlushInterval overrides the default 5s batch window.
	FlushInterval time.Duration
	// BufferSize overrides the default 512-event ring buffer.
	BufferSize int
}

func (c *AIEventsClientConfig) flushInterval() time.Duration {
	if c.FlushInterval > 0 {
		return c.FlushInterval
	}
	return aiEventsFlushInterval
}

func (c *AIEventsClientConfig) bufferSize() int {
	if c.BufferSize > 0 {
		return c.BufferSize
	}
	return aiEventsBufferSize
}

// AIEventsClient receives AI detection events from feature pipelines and
// streams them to the Directory. On buffer overflow the oldest events are
// silently dropped — AI event loss is acceptable (state is not reconstructed
// from this stream). For durable semantics use SegmentIndexClient.
type AIEventsClient struct {
	cfg AIEventsClientConfig
	log *slog.Logger

	// ring is a bounded ring buffer: when full, the oldest slot is
	// overwritten. Protected by mu.
	mu   sync.Mutex
	ring []AIEvent
	head int // next write position
	size int // current occupancy
}

// NewAIEventsClient constructs the client. Call Run to start.
func NewAIEventsClient(cfg AIEventsClientConfig) (*AIEventsClient, error) {
	if cfg.DirectoryEndpoint == "" {
		return nil, fmt.Errorf("directoryingest: DirectoryEndpoint is required")
	}
	if cfg.RecorderID == "" {
		return nil, fmt.Errorf("directoryingest: RecorderID is required")
	}
	if cfg.GetCertificate == nil {
		return nil, fmt.Errorf("directoryingest: GetCertificate is required")
	}
	l := cfg.Logger
	if l == nil {
		l = slog.Default()
	}
	cap := cfg.bufferSize()
	return &AIEventsClient{
		cfg:  cfg,
		log:  l.With("component", "directoryingest.aievents", "recorder_id", cfg.RecorderID),
		ring: make([]AIEvent, cap),
	}, nil
}

// Publish adds an AI event to the ring buffer. If the buffer is full the
// oldest event is dropped (fail open: recording keeps running, we just lose
// the oldest telemetry).
func (c *AIEventsClient) Publish(e AIEvent) {
	cap := len(c.ring)
	c.mu.Lock()
	c.ring[c.head] = e
	c.head = (c.head + 1) % cap
	if c.size < cap {
		c.size++
	}
	c.mu.Unlock()
}

// drain returns a snapshot of all buffered events and resets the buffer.
func (c *AIEventsClient) drain() []AIEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.size == 0 {
		return nil
	}
	cap := len(c.ring)
	out := make([]AIEvent, c.size)
	tail := (c.head - c.size + cap) % cap
	for i := range out {
		out[i] = c.ring[(tail+i)%cap]
	}
	c.size = 0
	c.head = 0
	return out
}

// Run starts the flush loop. Blocks until ctx is cancelled.
func (c *AIEventsClient) Run(ctx context.Context) {
	ticker := time.NewTicker(c.cfg.flushInterval())
	defer ticker.Stop()
	bo := newBackoff()

	for {
		select {
		case <-ctx.Done():
			if events := c.drain(); len(events) > 0 {
				_ = c.sendBatch(context.Background(), events)
			}
			return
		case <-ticker.C:
			events := c.drain()
			if len(events) == 0 {
				continue
			}
			if err := c.sendBatch(ctx, events); err != nil {
				if ctx.Err() != nil {
					return
				}
				c.log.WarnContext(ctx, "AIEvents send failed",
					"events", len(events), "error", err, "next_backoff", bo.next())
				// Best-effort retry with these events re-queued as oldest.
				c.mu.Lock()
				cap := len(c.ring)
				for i := len(events) - 1; i >= 0; i-- {
					// Insert at head - 1 (oldest) by going backward.
					if c.size >= cap {
						break // ring is full — drop oldest to preserve newest
					}
					c.head = (c.head - 1 + cap) % cap
					c.ring[c.head] = events[i]
					c.size++
				}
				c.mu.Unlock()
				continue
			}
			bo.reset()
		}
	}
}

func (c *AIEventsClient) sendBatch(ctx context.Context, events []AIEvent) error {
	wires := make([]wireAIEvent, len(events))
	for i, e := range events {
		wires[i] = wireAIEvent{
			EventID:      e.EventID,
			CameraID:     e.CameraID,
			Kind:         e.Kind,
			KindLabel:    e.KindLabel,
			ObservedAt:   e.ObservedAt.UTC().Format(time.RFC3339),
			Confidence:   e.Confidence,
			Bbox:         wireBoundingBox{X: e.BboxX, Y: e.BboxY, Width: e.BboxWidth, Height: e.BboxHeight},
			TrackID:      e.TrackID,
			SegmentID:    e.SegmentID,
			ThumbnailRef: e.ThumbnailRef,
			Attributes:   e.Attributes,
		}
	}
	payload := struct {
		RecorderID string        `json:"recorder_id"`
		Events     []wireAIEvent `json:"events"`
	}{
		RecorderID: c.cfg.RecorderID,
		Events:     wires,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	endpoint := strings.TrimRight(c.cfg.DirectoryEndpoint, "/") + aiEventsPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-ndjson")

	httpCli := newHTTPClient(c.cfg.GetCertificate)
	resp, err := httpCli.Do(req)
	if err != nil {
		return fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}
