package recordercontrol

import (
	"bufio"
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
	"time"
)

const (
	// backoffBase is the initial reconnect delay.
	backoffBase = 500 * time.Millisecond
	// backoffMax is the ceiling for the exponential back-off.
	backoffMax = 30 * time.Second
	// backoffJitter is the ±jitter fraction applied to each delay.
	backoffJitter = 0.20
	// periodicReconcileInterval is the safety-net full reconcile period.
	periodicReconcileInterval = 5 * time.Minute
	// softwareVersion is sent in the StreamAssignments request.
	softwareVersion = "dev"
	// streamAssignmentsPath is the HTTP path for StreamAssignments.
	streamAssignmentsPath = "/kaivue.v1.RecorderControlService/StreamAssignments"
)

// GetCertificateFunc is the type for certmgr.Manager.GetCertificate. Injected
// so the client never holds a static cert — renewal happens transparently.
type GetCertificateFunc func(*tls.ClientHelloInfo) (*tls.Certificate, error)

// ClientConfig configures the streaming client.
type ClientConfig struct {
	// DirectoryEndpoint is the base URL of the Directory, e.g.
	// "https://directory.kaivue.internal:443". Required.
	DirectoryEndpoint string

	// RecorderID is the stable Recorder UUID. Required.
	RecorderID string

	// GetCertificate is called before each dial to get the current mTLS
	// cert from certmgr (KAI-242). Required.
	GetCertificate GetCertificateFunc

	// Store is the local SQLite camera cache (KAI-250). Required.
	Store CameraStore

	// CaptureMgr controls capture loops. Required.
	CaptureMgr CaptureManager

	// Logger. Nil defaults to slog.Default().
	Logger *slog.Logger

	// PeriodicReconcileInterval overrides the 5-min safety-net. Zero uses
	// the default. Tests set this to a small value.
	PeriodicReconcileInterval time.Duration

	// nowFn is injectable for tests; defaults to time.Now.
	nowFn func() time.Time
}

func (c *ClientConfig) validate() error {
	if c.DirectoryEndpoint == "" {
		return fmt.Errorf("recordercontrol: DirectoryEndpoint is required")
	}
	if c.RecorderID == "" {
		return fmt.Errorf("recordercontrol: RecorderID is required")
	}
	if c.GetCertificate == nil {
		return fmt.Errorf("recordercontrol: GetCertificate is required")
	}
	if c.Store == nil {
		return fmt.Errorf("recordercontrol: Store is required")
	}
	if c.CaptureMgr == nil {
		return fmt.Errorf("recordercontrol: CaptureMgr is required")
	}
	return nil
}

func (c *ClientConfig) reconcileInterval() time.Duration {
	if c.PeriodicReconcileInterval > 0 {
		return c.PeriodicReconcileInterval
	}
	return periodicReconcileInterval
}

func (c *ClientConfig) now() time.Time {
	if c.nowFn != nil {
		return c.nowFn()
	}
	return time.Now()
}

// Client is the Recorder-side streaming client for RecorderControl.StreamAssignments.
//
// It maintains a long-lived subscription to the Directory, applies incoming
// events to the local camera cache, and reconciles capture loops. On stream
// drop it reconnects with exponential back-off + ±20% jitter.
//
// Construct via NewClient; call Run to start. Run blocks until ctx is cancelled.
type Client struct {
	cfg     ClientConfig
	log     *slog.Logger
	metrics clientMetrics
	rec     reconciler
}

// NewClient constructs a validated Client.
func NewClient(cfg ClientConfig) (*Client, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	c := &Client{
		cfg: cfg,
		log: cfg.Logger.With(
			slog.String("component", "recordercontrol"),
			slog.String("recorder_id", cfg.RecorderID),
		),
	}
	c.rec = reconciler{
		cap:     cfg.CaptureMgr,
		store:   cfg.Store,
		log:     c.log,
		metrics: &c.metrics,
	}
	return c, nil
}

// Run blocks, streaming events from the Directory and reconciling capture
// loops. It returns when ctx is cancelled.
func (c *Client) Run(ctx context.Context) {
	bo := newBackoff(backoffBase, backoffMax)

	// Periodic reconcile runs in a separate goroutine so it fires even
	// when the scanner is blocked waiting for the next event from the server.
	reconcileTicker := time.NewTicker(c.cfg.reconcileInterval())
	defer reconcileTicker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-reconcileTicker.C:
				c.periodicReconcile(ctx)
			}
		}
	}()

	// lastKnownVersion is the config_version the Recorder last applied.
	// It is sent on reconnect so the server can decide delta vs snapshot.
	var lastKnownVersion int64

	for {
		// Attempt one streaming session.
		reconnectReason, nextVersion := c.runStream(ctx, lastKnownVersion)
		if nextVersion > lastKnownVersion {
			lastKnownVersion = nextVersion
		}

		if ctx.Err() != nil {
			c.metrics.setConnected(false)
			return
		}

		c.metrics.setConnected(false)
		c.metrics.incReconnect(reconnectReason)

		// Compute jittered back-off delay.
		delay := applyJitter(bo.next(), backoffJitter)
		c.log.WarnContext(ctx, "recordercontrol: stream disconnected; will reconnect",
			slog.String("reason", reconnectReason),
			slog.Duration("backoff", delay),
		)

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}

		bo.reset()
	}
}

// Metrics returns a snapshot of the client's in-process metrics.
func (c *Client) Metrics() MetricsSnapshot {
	return c.metrics.Snapshot()
}

// runStream opens one streaming session to the Directory. It returns when
// the stream ends (clean disconnect, server reset, or ForceResync), with a
// reconnect reason and the highest version received.
func (c *Client) runStream(
	ctx context.Context,
	knownVersion int64,
) (reason string, maxVersion int64) {
	tlsCfg := &tls.Config{
		GetClientCertificate: func(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return c.cfg.GetCertificate(nil)
		},
		MinVersion: tls.VersionTLS13,
	}
	httpClient := &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}

	reqBody, _ := json.Marshal(map[string]any{
		"recorder_id":               c.cfg.RecorderID,
		"known_version":             knownVersion,
		"recorder_software_version": softwareVersion,
	})

	endpoint := strings.TrimRight(c.cfg.DirectoryEndpoint, "/") + streamAssignmentsPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		c.log.ErrorContext(ctx, "recordercontrol: failed to build request", slog.String("error", err.Error()))
		return "stream_drop", knownVersion
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "stream_drop", knownVersion
		}
		c.log.WarnContext(ctx, "recordercontrol: HTTP dial failed", slog.String("error", err.Error()))
		return "stream_drop", knownVersion
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.log.WarnContext(ctx, "recordercontrol: unexpected status from Directory",
			slog.Int("status", resp.StatusCode))
		return "stream_drop", knownVersion
	}

	c.metrics.setConnected(true)
	c.log.InfoContext(ctx, "recordercontrol: stream connected",
		slog.Int64("known_version", knownVersion))

	scanner := bufio.NewScanner(resp.Body)
	maxVersion = knownVersion

	for scanner.Scan() {
		if ctx.Err() != nil {
			return "stream_drop", maxVersion
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var ev wireEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			c.log.WarnContext(ctx, "recordercontrol: failed to unmarshal event",
				slog.String("error", err.Error()),
				slog.String("line", string(line)))
			continue
		}

		if ev.Version > maxVersion {
			maxVersion = ev.Version
		}

		c.metrics.incEvent(ev.Kind)

		switch ev.Kind {
		case kindSnapshot:
			c.applySnapshot(ctx, ev)
		case kindCameraAdded:
			c.applyAdded(ctx, ev)
		case kindCameraUpdated:
			c.applyUpdated(ctx, ev)
		case kindCameraRemoved:
			c.applyRemoved(ctx, ev)
		case kindHeartbeat:
			// heartbeat is a liveness signal; no action required
		default:
			// unknown kind — server is newer than client, ignore gracefully
			c.log.DebugContext(ctx, "recordercontrol: unknown event kind",
				slog.String("kind", ev.Kind))
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		c.log.WarnContext(ctx, "recordercontrol: stream scan error",
			slog.String("error", err.Error()))
	}
	return "stream_drop", maxVersion
}

// applySnapshot atomically replaces the local camera cache and reconciles
// capture loops from the authoritative list.
//
// Cross-recorder filtering: any camera whose recorder_id does not match this
// Recorder's ID is rejected. The server enforces tenant isolation but the
// client adds a second layer of defence (defence-in-depth).
func (c *Client) applySnapshot(ctx context.Context, ev wireEvent) {
	if ev.Snapshot == nil {
		return
	}
	cameras := make([]Camera, 0, len(ev.Snapshot.Cameras))
	for _, wc := range ev.Snapshot.Cameras {
		if wc.RecorderID != "" && wc.RecorderID != c.cfg.RecorderID {
			c.log.WarnContext(ctx, "recordercontrol: snapshot contains foreign camera; skipping",
				slog.String("camera_id", wc.ID),
				slog.String("camera_recorder_id", wc.RecorderID),
				slog.String("our_recorder_id", c.cfg.RecorderID),
			)
			continue
		}
		cameras = append(cameras, wireCameraToCamera(wc))
	}
	result := c.rec.reconcileSnapshot(ctx, cameras)
	c.metrics.incReconcile(result)
	c.log.InfoContext(ctx, "recordercontrol: applied snapshot",
		slog.Int("cameras", len(cameras)),
		slog.Int64("version", ev.Version),
		slog.String("result", result),
	)
}

// applyAdded handles a CameraAdded incremental event.
func (c *Client) applyAdded(ctx context.Context, ev wireEvent) {
	if ev.Added == nil {
		return
	}
	wc := ev.Added.Camera
	if wc.RecorderID != "" && wc.RecorderID != c.cfg.RecorderID {
		c.log.WarnContext(ctx, "recordercontrol: camera_added for foreign recorder; skipping",
			slog.String("camera_id", wc.ID),
			slog.String("camera_recorder_id", wc.RecorderID))
		return
	}
	cam := wireCameraToCamera(wc)
	c.rec.reconcileAdd(ctx, cam)
	c.log.InfoContext(ctx, "recordercontrol: camera added",
		slog.String("camera_id", cam.ID),
		slog.Int64("version", ev.Version),
	)
}

// applyUpdated handles a CameraUpdated incremental event.
func (c *Client) applyUpdated(ctx context.Context, ev wireEvent) {
	if ev.Updated == nil {
		return
	}
	wc := ev.Updated.Camera
	if wc.RecorderID != "" && wc.RecorderID != c.cfg.RecorderID {
		c.log.WarnContext(ctx, "recordercontrol: camera_updated for foreign recorder; skipping",
			slog.String("camera_id", wc.ID),
			slog.String("camera_recorder_id", wc.RecorderID))
		return
	}
	cam := wireCameraToCamera(wc)
	c.rec.reconcileUpdate(ctx, cam)
	c.log.InfoContext(ctx, "recordercontrol: camera updated",
		slog.String("camera_id", cam.ID),
		slog.Int64("version", ev.Version),
	)
}

// applyRemoved handles a CameraRemoved incremental event.
func (c *Client) applyRemoved(ctx context.Context, ev wireEvent) {
	if ev.Removed == nil {
		return
	}
	c.rec.reconcileRemove(ctx, ev.Removed.CameraID)
	c.log.InfoContext(ctx, "recordercontrol: camera removed",
		slog.String("camera_id", ev.Removed.CameraID),
		slog.Bool("purge_recordings", ev.Removed.PurgeRecordings),
		slog.String("reason", ev.Removed.Reason),
		slog.Int64("version", ev.Version),
	)
}

// periodicReconcile is the 5-minute safety-net: reads the authoritative
// list from the store and reconciles capture loops even while the stream
// is healthy.
func (c *Client) periodicReconcile(ctx context.Context) {
	cameras, err := c.cfg.Store.List(ctx)
	if err != nil {
		c.log.WarnContext(ctx, "recordercontrol: periodic reconcile: list failed",
			slog.String("error", err.Error()))
		c.metrics.incReconcile(reconcileResultError)
		return
	}
	result := c.rec.applyDiff(ctx, cameras)
	c.metrics.incReconcile(result)
	c.log.DebugContext(ctx, "recordercontrol: periodic reconcile complete",
		slog.String("result", result),
		slog.Int("cameras", len(cameras)),
	)
}

// applyJitter applies ±jitter fraction to d using a uniform random offset.
// For example, jitter=0.20 means the result is in [d*0.80, d*1.20].
func applyJitter(d time.Duration, jitter float64) time.Duration {
	// rand.Float64() in [0,1); shift to [-jitter, +jitter)
	factor := 1 + jitter*(2*rand.Float64()-1) //nolint:gosec // non-crypto jitter
	return time.Duration(float64(d) * factor)
}
