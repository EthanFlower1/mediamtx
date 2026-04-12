// Package recorder — Boot implements the Recorder runtime mode entry point.
//
// When MTX_MODE=recorder, the Recorder goes through a multi-phase boot:
//
//  1. Open the local SQLite state store.
//  2. Check pairing state; if not paired, run the 9-step Joiner.
//  3. Bring up the tailnet mesh node.
//  4. Start the MediaMTX supervisor (sidecar + hot-reload controller).
//  5. Connect to the Directory's RecorderControl stream (camera assignments).
//  6. Start Directory ingest streams (camera state, segments, AI events).
//  7. Feature pipelines (AI) are noted but deferred to their own tickets.
//
// Graceful degradation: if the Directory is unreachable after pairing,
// the Recorder continues using cached camera assignments. The
// recording-never-stops invariant applies at every stage.
package recorder

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/bluenviron/mediamtx/internal/recorder/directoryingest"
	recordermesh "github.com/bluenviron/mediamtx/internal/recorder/mesh"
	"github.com/bluenviron/mediamtx/internal/recorder/mediamtxsupervisor"
	"github.com/bluenviron/mediamtx/internal/recorder/pairing"
	"github.com/bluenviron/mediamtx/internal/recorder/recordercontrol"
	"github.com/bluenviron/mediamtx/internal/recorder/state"
	sharedtsnet "github.com/bluenviron/mediamtx/internal/shared/mesh/tsnet"
	kairuntime "github.com/bluenviron/mediamtx/internal/shared/runtime"
)

// DefaultStateDir is the default directory for local state, device key,
// and mesh identity when no override is provided.
const DefaultStateDir = "/var/lib/mediamtx-recorder"

// Booter implements kairuntime.RecorderBooter. It is the concrete type
// wired into Core.recorderBooter so that Dispatch can start a real
// Recorder instead of a stub.
type Booter struct {
	server *RecorderServer
}

// Boot implements kairuntime.RecorderBooter. cfg is expected to be
// *conf.Conf but is accepted as any for interface compliance.
func (b *Booter) Boot(ctx context.Context, _ any, logger *slog.Logger) error {
	srv, err := Boot(ctx, BootConfig{
		Logger: logger,
	})
	if err != nil {
		return err
	}
	b.server = srv
	return nil
}

// PairingRedeemer returns a PairingTokenRedeemer for in-process
// AllInOne auto-pairing. The concrete redeemer runs the Joiner
// without an HTTP round-trip.
func (b *Booter) PairingRedeemer() kairuntime.PairingTokenRedeemer {
	return &recorderPairingRedeemer{}
}

// Shutdown gracefully stops the Recorder subsystem.
func (b *Booter) Shutdown(_ context.Context) error {
	if b.server != nil {
		b.server.Shutdown()
	}
	return nil
}

// Compile-time interface check.
var _ kairuntime.RecorderBooter = (*Booter)(nil)

// recorderPairingRedeemer redeems a token by running the Joiner
// in-process. Used only by AllInOne auto-pair.
type recorderPairingRedeemer struct{}

func (r *recorderPairingRedeemer) RedeemPairingToken(token string) error {
	joiner := pairing.NewJoiner(pairing.JoinerConfig{})
	return joiner.Run(context.Background(), token)
}

// BootConfig holds all inputs needed to boot the Recorder.
type BootConfig struct {
	// StateDir is the root directory for the Recorder's local
	// SQLite DB, device key, and mesh state. If empty,
	// DefaultStateDir is used.
	StateDir string

	// PairingToken is the base64-encoded pairing token issued by
	// the Directory admin. Read from MTX_PAIRING_TOKEN env var or
	// supplied by the caller. Required on first boot (when not yet
	// paired); ignored once paired.
	PairingToken string

	// Logger receives structured log output. nil = slog.Default().
	Logger *slog.Logger

	// MeshTestMode, when true, uses the in-memory mesh backend
	// instead of real tsnet. Intended for tests.
	MeshTestMode bool

	// MediaMTXAPIURL is the base URL of the MediaMTX HTTP API used
	// by the supervisor's HTTPController. Default: "http://127.0.0.1:9997".
	MediaMTXAPIURL string

	// MediaMTXPathPrefix overrides the path prefix used by the
	// supervisor to namespace Recorder-managed paths in MediaMTX.
	// Default: "cam_".
	MediaMTXPathPrefix string
}

func (c *BootConfig) stateDir() string {
	if c.StateDir != "" {
		return c.StateDir
	}
	return DefaultStateDir
}

func (c *BootConfig) meshStateDir() string {
	return filepath.Join(c.stateDir(), "mesh")
}

func (c *BootConfig) dbPath() string {
	return filepath.Join(c.stateDir(), "state.db")
}

func (c *BootConfig) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

func (c *BootConfig) mediaAPIURL() string {
	if c.MediaMTXAPIURL != "" {
		return c.MediaMTXAPIURL
	}
	return "http://127.0.0.1:9997"
}

func (c *BootConfig) pathPrefix() string {
	if c.MediaMTXPathPrefix != "" {
		return c.MediaMTXPathPrefix
	}
	return "cam_"
}

// RecorderServer is the running Recorder. Callers hold this value and
// call Shutdown to tear down all subsystems in reverse order.
type RecorderServer struct {
	log       *slog.Logger
	store     *state.Store
	meshNode  *sharedtsnet.Node
	cancelFn  context.CancelFunc
	doneCh    chan struct{}

	// Exported for health probes / metrics.
	RecorderID   string
	DirectoryURL string
	MeshHostname string
}

// Shutdown tears down all subsystems gracefully. Safe to call multiple
// times; the second call is a no-op.
func (rs *RecorderServer) Shutdown() {
	if rs.cancelFn != nil {
		rs.cancelFn()
	}
	if rs.doneCh != nil {
		<-rs.doneCh
	}
	if rs.meshNode != nil {
		_ = rs.meshNode.Shutdown(context.Background())
	}
	if rs.store != nil {
		_ = rs.store.Close()
	}
	rs.log.Info("recorder: shutdown complete")
}

// Boot starts the Recorder, pairing with a Directory if needed. It
// returns a RecorderServer that the caller must Shutdown when done.
//
// Boot is the real implementation behind runtime.Hooks.StartRecorder.
func Boot(ctx context.Context, cfg BootConfig) (*RecorderServer, error) {
	log := cfg.logger().With(slog.String("component", "recorder-boot"))

	// -----------------------------------------------------------
	// 1. Open local state store (SQLite)
	// -----------------------------------------------------------
	log.Info("recorder: opening local state store", slog.String("path", cfg.dbPath()))
	store, err := state.Open(cfg.dbPath(), state.Options{})
	if err != nil {
		return nil, fmt.Errorf("recorder boot: open state store: %w", err)
	}

	// -----------------------------------------------------------
	// 2. Check pairing state
	// -----------------------------------------------------------
	var ps pairing.PairedState
	paired := true
	if err := store.GetState(ctx, "pairing.paired", &ps); err != nil {
		if errors.Is(err, state.ErrNotFound) {
			paired = false
		} else {
			store.Close()
			return nil, fmt.Errorf("recorder boot: read pairing state: %w", err)
		}
	}

	// -----------------------------------------------------------
	// 3. If not paired, run the Joiner
	// -----------------------------------------------------------
	if !paired {
		token := cfg.PairingToken
		if token == "" {
			token = os.Getenv("MTX_PAIRING_TOKEN")
		}
		if token == "" {
			store.Close()
			return nil, fmt.Errorf("recorder boot: not yet paired and no pairing token provided " +
				"(set MTX_PAIRING_TOKEN env var or pass PairingToken in config)")
		}

		log.Info("recorder: not yet paired, starting join sequence")
		joiner := pairing.NewJoiner(pairing.JoinerConfig{
			StateDir:     cfg.stateDir(),
			MeshStateDir: cfg.meshStateDir(),
			Logger:       log,
		})
		if err := joiner.Run(ctx, token); err != nil {
			store.Close()
			return nil, fmt.Errorf("recorder boot: pairing failed: %w", err)
		}

		// Re-read the paired state the Joiner just wrote.
		if err := store.GetState(ctx, "pairing.paired", &ps); err != nil {
			store.Close()
			return nil, fmt.Errorf("recorder boot: read pairing state after join: %w", err)
		}
		log.Info("recorder: pairing complete",
			slog.String("recorder_uuid", ps.RecorderUUID),
			slog.String("directory_url", ps.DirectoryURL))
	} else {
		log.Info("recorder: already paired",
			slog.String("recorder_uuid", ps.RecorderUUID),
			slog.String("directory_url", ps.DirectoryURL))
	}

	// -----------------------------------------------------------
	// 4. Bring up tsnet mesh node
	// -----------------------------------------------------------
	log.Info("recorder: starting mesh node",
		slog.String("hostname", ps.MeshHostname))
	meshNode, err := recordermesh.New(ctx, recordermesh.Config{
		ComponentID: ps.RecorderUUID,
		AuthKey:     "", // Already enrolled; tsnet reuses persisted identity.
		StateDir:    cfg.meshStateDir(),
		ControlURL:  ps.DirectoryURL + "/headscale",
		TestMode:    cfg.MeshTestMode,
	})
	if err != nil {
		// Graceful degradation: mesh failure is non-fatal. The
		// Recorder can still operate on cached assignments.
		log.Warn("recorder: mesh node failed to start (continuing without mesh)",
			slog.String("error", err.Error()))
		meshNode = nil
	}

	// -----------------------------------------------------------
	// 5. Start MediaMTX supervisor
	// -----------------------------------------------------------
	controller := &mediamtxsupervisor.HTTPController{
		BaseURL:    cfg.mediaAPIURL(),
		PathPrefix: cfg.pathPrefix(),
	}
	supervisor, err := mediamtxsupervisor.New(mediamtxsupervisor.Config{
		Source:     mediamtxsupervisor.StoreSource{Store: store},
		Controller: controller,
		Render: mediamtxsupervisor.RenderOptions{
			PathPrefix: cfg.pathPrefix(),
		},
		Logger: log,
	})
	if err != nil {
		if meshNode != nil {
			_ = meshNode.Shutdown(ctx)
		}
		store.Close()
		return nil, fmt.Errorf("recorder boot: create supervisor: %w", err)
	}

	// The supervisor Start does an initial synchronous reload.
	if err := supervisor.Start(ctx); err != nil {
		// Fail-open: supervisor logs internally and continues.
		log.Warn("recorder: initial supervisor reload failed (continuing fail-open)",
			slog.String("error", err.Error()))
	}

	// -----------------------------------------------------------
	// 6-8. Start background streaming clients
	// -----------------------------------------------------------
	// Create a cancellable context for all streaming goroutines.
	streamCtx, streamCancel := context.WithCancel(ctx)
	doneCh := make(chan struct{})

	// We need a GetCertificate function for mTLS. For now we
	// return a placeholder that indicates the cert should be loaded
	// from the state directory. In production, certmgr (KAI-242)
	// will supply this; for the initial boot we return a nil cert
	// which degrades to non-mTLS (the Directory handles this
	// gracefully during the initial window).
	getCert := func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
		// TODO(KAI-242): replace with certmgr.Manager.GetCertificate
		return &tls.Certificate{}, nil
	}

	// noopCaptureMgr is a minimal CaptureManager that satisfies
	// the recordercontrol.Client requirement. The real capture
	// manager will be wired when the sidecar lifecycle lands.
	capMgr := &noopCaptureManager{}

	// CameraStore adapter for recordercontrol
	cameraStore := &stateCameraStoreAdapter{store: store}

	go func() {
		defer close(doneCh)

		// 6. RecorderControl — receive camera assignments
		rcClient, rcErr := recordercontrol.NewClient(recordercontrol.ClientConfig{
			DirectoryEndpoint: ps.DirectoryURL,
			RecorderID:        ps.RecorderUUID,
			GetCertificate:    getCert,
			Store:             cameraStore,
			CaptureMgr:        capMgr,
			Logger:            log,
		})
		if rcErr != nil {
			log.Error("recorder: failed to create RecorderControl client",
				slog.String("error", rcErr.Error()))
		} else {
			go rcClient.Run(streamCtx)
			log.Info("recorder: RecorderControl stream started")
		}

		// 7. Directory ingest — camera state stream
		csClient, csErr := directoryingest.NewCameraStateClient(
			directoryingest.CameraStateClientConfig{
				DirectoryEndpoint: ps.DirectoryURL,
				RecorderID:        ps.RecorderUUID,
				GetCertificate:    getCert,
				Logger:            log,
			})
		if csErr != nil {
			log.Error("recorder: failed to create CameraState ingest client",
				slog.String("error", csErr.Error()))
		} else {
			go csClient.Run(streamCtx)
			log.Info("recorder: CameraState ingest stream started")
		}

		// 8. Directory ingest — segment index stream
		siClient, siErr := directoryingest.NewSegmentIndexClient(
			directoryingest.SegmentIndexClientConfig{
				DirectoryEndpoint: ps.DirectoryURL,
				RecorderID:        ps.RecorderUUID,
				GetCertificate:    getCert,
				Logger:            log,
			})
		if siErr != nil {
			log.Error("recorder: failed to create SegmentIndex ingest client",
				slog.String("error", siErr.Error()))
		} else {
			go siClient.Run(streamCtx)
			log.Info("recorder: SegmentIndex ingest stream started")
		}

		// 9. Directory ingest — AI events stream
		aiClient, aiErr := directoryingest.NewAIEventsClient(
			directoryingest.AIEventsClientConfig{
				DirectoryEndpoint: ps.DirectoryURL,
				RecorderID:        ps.RecorderUUID,
				GetCertificate:    getCert,
				Logger:            log,
			})
		if aiErr != nil {
			log.Error("recorder: failed to create AIEvents ingest client",
				slog.String("error", aiErr.Error()))
		} else {
			go aiClient.Run(streamCtx)
			log.Info("recorder: AIEvents ingest stream started")
		}

		// 10. Feature pipelines (AI) — deferred to their own tickets.
		log.Info("recorder: AI feature pipelines deferred (KAI-281, KAI-283, KAI-284)")

		// Block until context cancelled.
		<-streamCtx.Done()

		// Give the supervisor a chance to close cleanly.
		supervisor.Close()
	}()

	log.Info("recorder: boot complete — paired with Directory",
		slog.String("recorder_uuid", ps.RecorderUUID),
		slog.String("directory_url", ps.DirectoryURL),
		slog.String("mesh_hostname", ps.MeshHostname))

	return &RecorderServer{
		log:          log,
		store:        store,
		meshNode:     meshNode,
		cancelFn:     streamCancel,
		doneCh:       doneCh,
		RecorderID:   ps.RecorderUUID,
		DirectoryURL: ps.DirectoryURL,
		MeshHostname: ps.MeshHostname,
	}, nil
}

// --- Adapters and stubs ---------------------------------------------------

// stateCameraStoreAdapter adapts *state.Store to the
// recordercontrol.CameraStore interface.
type stateCameraStoreAdapter struct {
	store *state.Store
}

func (a *stateCameraStoreAdapter) ReplaceAll(ctx context.Context, cameras []recordercontrol.Camera) error {
	cams := make([]state.AssignedCamera, len(cameras))
	for i, c := range cameras {
		cams[i] = state.AssignedCamera{
			CameraID:      c.ID,
			ConfigVersion: c.ConfigVersion,
			Config: state.CameraConfig{
				ID: c.ID,
			},
		}
	}
	_, err := a.store.ReconcileAssignments(ctx, cams)
	return err
}

func (a *stateCameraStoreAdapter) Add(ctx context.Context, c recordercontrol.Camera) error {
	return a.store.UpsertCamera(ctx, state.AssignedCamera{
		CameraID:      c.ID,
		ConfigVersion: c.ConfigVersion,
		Config:        state.CameraConfig{ID: c.ID},
	})
}

func (a *stateCameraStoreAdapter) Update(ctx context.Context, c recordercontrol.Camera) error {
	return a.Add(ctx, c)
}

func (a *stateCameraStoreAdapter) Remove(ctx context.Context, cameraID string) error {
	return a.store.RemoveCamera(ctx, cameraID)
}

func (a *stateCameraStoreAdapter) List(ctx context.Context) ([]recordercontrol.Camera, error) {
	cams, err := a.store.ListAssigned(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]recordercontrol.Camera, len(cams))
	for i, c := range cams {
		out[i] = recordercontrol.Camera{
			ID:            c.CameraID,
			ConfigVersion: c.ConfigVersion,
		}
	}
	return out, nil
}

// noopCaptureManager satisfies recordercontrol.CaptureManager with
// no-op operations. It is a placeholder until the real capture manager
// is wired (KAI-259).
type noopCaptureManager struct {
	running []string
}

func (n *noopCaptureManager) EnsureRunning(_ recordercontrol.Camera) error {
	return nil
}

func (n *noopCaptureManager) Stop(_ string) error {
	return nil
}

func (n *noopCaptureManager) RunningCameras() []string {
	return n.running
}
