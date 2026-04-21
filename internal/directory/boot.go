package directory

import (
	"context"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/hkdf"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/directory/adminapi"
	"github.com/bluenviron/mediamtx/internal/directory/authapi"
	"github.com/bluenviron/mediamtx/internal/directory/cameraapi"
	directorydb "github.com/bluenviron/mediamtx/internal/directory/db"
	"github.com/bluenviron/mediamtx/internal/directory/ingest"
	"github.com/bluenviron/mediamtx/internal/directory/mdns"
	"github.com/bluenviron/mediamtx/internal/directory/mesh/headscale"
	"github.com/bluenviron/mediamtx/internal/directory/pairing"
	"github.com/bluenviron/mediamtx/internal/directory/pki/stepca"
	"github.com/bluenviron/mediamtx/internal/directory/recorderapi"
	"github.com/bluenviron/mediamtx/internal/directory/recordercontrol"
	"github.com/bluenviron/mediamtx/internal/directory/streams"
	"github.com/bluenviron/mediamtx/internal/directory/timeline"
	"github.com/bluenviron/mediamtx/internal/directory/webui"
	kairuntime "github.com/bluenviron/mediamtx/internal/shared/runtime"
	"github.com/bluenviron/mediamtx/internal/shared/systemapi"
)

// BootConfig holds the parameters for booting the Directory subsystem.
// Callers (core.go or tests) populate this from mediamtx.yml / conf.Conf.
type BootConfig struct {
	// DataDir is the root directory for all Directory state
	// (SQLite DB, PKI material, mesh state). Defaults to
	// /var/lib/mediamtx-directory if empty.
	DataDir string

	// ListenAddr is the HTTP listen address, e.g. ":9997".
	ListenAddr string

	// MasterKey is the nvrJWTSecret bytes used for PKI encryption
	// and Headscale state encryption. Required.
	MasterKey []byte

	// Logger is the structured logger. Nil defaults to slog.Default().
	Logger *slog.Logger

	// MDNSEnabled controls whether the mDNS broadcaster starts.
	// Defaults to true.
	MDNSEnabled *bool

	// MDNSInstanceName overrides the mDNS service instance name.
	// Empty defaults to the system hostname.
	MDNSInstanceName string

	// RecorderServiceToken is the shared bearer token the Directory uses
	// when querying recorders' internal APIs. Must match the nvrServiceToken
	// configured on the recorders.
	RecorderServiceToken string

	// DirectoryEndpoint is the base URL Recorders use to reach this
	// Directory, e.g. "https://dir.acme.local:8443". When empty the
	// boot sequence constructs one from ListenAddr.
	DirectoryEndpoint string

	// NVRDBPath is the path to the NVR SQLite database used by the new
	// camera and system API handlers. Defaults to <DataDir>/nvr.db.
	// When empty the handlers are not registered.
	NVRDBPath string
}

func (c *BootConfig) withDefaults() {
	if c.DataDir == "" {
		// Use a local data directory relative to the working directory
		// for development. Production deployments should set DataDir
		// explicitly via nvrDirectoryDataDir config.
		c.DataDir = "data/directory"
	}
	if c.ListenAddr == "" {
		c.ListenAddr = ":9996"
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.MDNSEnabled == nil {
		t := true
		c.MDNSEnabled = &t
	}
	if c.DirectoryEndpoint == "" {
		if env := os.Getenv("MTX_DIRECTORY_ENDPOINT"); env != "" {
			c.DirectoryEndpoint = env
		} else {
			c.DirectoryEndpoint = "http://localhost" + c.ListenAddr
		}
	}
}

// DirectoryServer holds all running Directory subsystems and provides a
// clean Shutdown method.
type DirectoryServer struct {
	DB          *directorydb.DB
	NVRDB       *directorydb.DB
	CA          *stepca.ClusterCA
	Headscale   *headscale.Coordinator
	PairingSvc  *pairing.Service
	HTTPServer  *http.Server
	Broadcaster *mdns.Broadcaster
	logger      *slog.Logger
}

// Shutdown gracefully stops all Directory subsystems in reverse boot order.
func (ds *DirectoryServer) Shutdown(ctx context.Context) error {
	var errs []string

	if ds.Broadcaster != nil {
		ds.Broadcaster.Stop()
	}

	if ds.HTTPServer != nil {
		if err := ds.HTTPServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Sprintf("http: %v", err))
		}
	}

	if ds.Headscale != nil {
		if err := ds.Headscale.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Sprintf("headscale: %v", err))
		}
	}

	if ds.CA != nil {
		if err := ds.CA.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Sprintf("pki: %v", err))
		}
	}

	if ds.NVRDB != nil {
		if err := ds.NVRDB.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("nvrdb: %v", err))
		}
	}

	if ds.DB != nil {
		if err := ds.DB.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("db: %v", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("directory shutdown errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Boot starts all Directory subsystems in dependency order and returns a
// running DirectoryServer. The caller should defer srv.Shutdown(ctx) to
// clean up resources.
//
// Boot sequence:
//  1. Open Directory SQLite DB (migrations auto-applied)
//  2. Bootstrap PKI (embedded step-ca cluster CA)
//  3. Start Headscale mesh coordinator
//  4. Start pairing service
//  5. Create recorder-control, ingest, streams, timeline handlers
//  6. Build and start HTTP server with all handlers
//  7. Optionally start mDNS broadcaster
func Boot(ctx context.Context, cfg BootConfig) (*DirectoryServer, error) {
	cfg.withDefaults()
	log := cfg.Logger

	srv := &DirectoryServer{logger: log}

	// ---------------------------------------------------------------
	// 1. Open Directory SQLite DB (migrations auto-applied)
	// ---------------------------------------------------------------
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("directory: mkdir data dir: %w", err)
	}
	dbPath := filepath.Join(cfg.DataDir, "directory.db")
	log.Info("directory: opening database", "path", dbPath)

	ddb, err := directorydb.Open(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("directory: open db: %w", err)
	}
	srv.DB = ddb

	// ---------------------------------------------------------------
	// 1b. Open NVR SQLite DB (optional — for cameraapi / systemapi)
	// ---------------------------------------------------------------
	nvrDBPath := cfg.NVRDBPath
	if nvrDBPath == "" {
		nvrDBPath = filepath.Join(cfg.DataDir, "nvr.db")
	}
	log.Info("directory: opening NVR database", "path", nvrDBPath)
	nDB, nvrDBErr := directorydb.Open(ctx, nvrDBPath)
	if nvrDBErr != nil {
		// Non-fatal: log and continue without the new camera/system API.
		log.Warn("directory: failed to open NVR database; cameraapi/systemapi will not be registered",
			"error", nvrDBErr)
		nDB = nil
	} else {
		srv.NVRDB = nDB
	}

	// ---------------------------------------------------------------
	// 2. Bootstrap PKI — embedded step-ca cluster CA
	// ---------------------------------------------------------------
	pkiDir := filepath.Join(cfg.DataDir, "pki")
	log.Info("directory: bootstrapping PKI", "dir", pkiDir)

	ca, err := stepca.New(stepca.Config{
		StateDir:  pkiDir,
		MasterKey: cfg.MasterKey,
		Logf: func(format string, args ...any) {
			log.Info(fmt.Sprintf(format, args...))
		},
	})
	if err != nil {
		_ = ddb.Close()
		return nil, fmt.Errorf("directory: pki: %w", err)
	}
	srv.CA = ca

	// ---------------------------------------------------------------
	// 3. Start Headscale mesh coordinator
	// ---------------------------------------------------------------
	meshDir := filepath.Join(cfg.DataDir, "mesh")
	log.Info("directory: starting Headscale coordinator", "dir", meshDir)

	hs, err := headscale.New(headscale.Config{
		StateDir:  meshDir,
		MasterKey: cfg.MasterKey,
		Logf: func(format string, args ...any) {
			log.Info(fmt.Sprintf(format, args...))
		},
	})
	if err != nil {
		_ = ca.Shutdown(ctx)
		_ = ddb.Close()
		return nil, fmt.Errorf("directory: headscale new: %w", err)
	}
	if err := hs.Start(ctx); err != nil {
		_ = ca.Shutdown(ctx)
		_ = ddb.Close()
		return nil, fmt.Errorf("directory: headscale start: %w", err)
	}
	srv.Headscale = hs

	// ---------------------------------------------------------------
	// 4. Start pairing service
	// ---------------------------------------------------------------
	log.Info("directory: starting pairing service")

	// The pairing service needs an ed25519 root key. We derive a
	// deterministic key from the master key for signing pairing tokens.
	// This keeps the pairing signing key stable across restarts.
	pairingRootKey := derivePairingKey(cfg.MasterKey)

	pairingSvc, err := pairing.NewService(pairing.Config{
		DB:                 ddb,
		Headscale:          hs,
		ClusterCA:          ca,
		RootSigningKey:     pairingRootKey,
		DirectoryEndpoint:  cfg.DirectoryEndpoint,
		HeadscaleNamespace: headscale.DefaultNamespace,
		Logger:             log.With(slog.String("component", "pairing")),
	})
	if err != nil {
		_ = hs.Shutdown(ctx)
		_ = ca.Shutdown(ctx)
		_ = ddb.Close()
		return nil, fmt.Errorf("directory: pairing: %w", err)
	}
	srv.PairingSvc = pairingSvc

	// ---------------------------------------------------------------
	// 5. Create recorder-control, ingest, streams, timeline handlers
	// ---------------------------------------------------------------
	log.Info("directory: creating API handlers")

	// Recorder control
	eventBus := recordercontrol.NewEventBus()
	rcStore := recordercontrol.NewStore(ddb.DB)
	recCtrl, err := recordercontrol.NewHandler(recordercontrol.Config{
		Bus:   eventBus,
		Store: rcStore,
		RecorderAuthenticator: func(r *http.Request) (string, bool) {
			// In production this extracts from mTLS or bearer token.
			// For now accept the X-Recorder-ID header.
			id := r.Header.Get("X-Recorder-ID")
			return id, id != ""
		},
		Logger: log.With(slog.String("component", "recordercontrol")),
	})
	if err != nil {
		_ = srv.cleanup(ctx)
		return nil, fmt.Errorf("directory: recordercontrol: %w", err)
	}

	// Recorder API (registration, heartbeats, camera CRUD, fan-out queries)
	recorderStore := recorderapi.NewStore(ddb.DB)
	recorderHandlers := &recorderapi.Handlers{
		Store:    recorderStore,
		RCStore:  rcStore,
		EventBus: eventBus,
		Logger:   log.With(slog.String("component", "recorderapi")),
	}
	fanoutSvc := &recorderapi.FanoutService{
		Store:              recorderStore,
		Logger:             log.With(slog.String("component", "fanout")),
		SharedServiceToken: cfg.RecorderServiceToken,
	}

	// Recorder auth — tries bearer token first, falls back to X-Recorder-ID header.
	recorderAuth := recorderapi.BearerOrHeaderAuth(recorderStore)

	// Ingest
	ingestStore := ingest.NewStore(ddb.DB)
	ingestLog := log.With(slog.String("component", "ingest"))

	// Streams
	urlSigner := streams.NewURLSigner(cfg.MasterKey, 5*time.Minute)
	cameraResolver := &dbCameraResolver{db: ddb.DB}

	// Timeline
	segmentStore := &dbSegmentStore{db: ddb.DB}
	assembler := timeline.NewAssembler(segmentStore)

	// Recorder store (for pairing check-in)
	pairingRecorderStore := pairing.NewRecorderStore(ddb)

	// Pending store (for pending pairing requests)
	pendingStore := pairing.NewPendingStore(ddb)

	// ---------------------------------------------------------------
	// 6. Build HTTP mux and start server
	// ---------------------------------------------------------------
	mux := http.NewServeMux()

	// Health check — unauthenticated
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"mode":   "directory",
		})
	})

	// Pairing endpoints — unauthenticated
	mux.HandleFunc("/api/v1/pairing/tokens", pairing.GenerateHandler(pairingSvc, func(r *http.Request) (pairing.UserID, bool) {
		// In production this extracts from the JWT claims.
		// For now accept the X-User-ID header.
		uid := r.Header.Get("X-User-ID")
		return pairing.UserID(uid), uid != ""
	}))
	mux.HandleFunc("/api/v1/pairing/check-in", pairing.CheckInHandler(pairingSvc, pairingRecorderStore, nil, log))
	mux.HandleFunc("/api/v1/pairing/pending", pairing.ListPendingHandler(pendingStore))

	// Recorder control — streaming endpoint
	mux.Handle("/kaivue.v1.RecorderControlService/StreamAssignments", recCtrl)

	// Ingest endpoints
	mux.HandleFunc("/kaivue.v1.DirectoryIngest/StreamCameraState",
		ingest.StreamCameraStateHandler(ingestStore, recorderAuth, ingestLog))
	mux.HandleFunc("/kaivue.v1.DirectoryIngest/PublishSegmentIndex",
		ingest.PublishSegmentIndexHandler(ingestStore, recorderAuth, ingestLog))
	mux.HandleFunc("/kaivue.v1.DirectoryIngest/PublishAIEvents",
		ingest.PublishAIEventsHandler(ingestStore, recorderAuth, ingestLog))

	// Streams
	mux.HandleFunc("/api/v1/streams/request", streams.Handler(cameraResolver, urlSigner))

	// Timeline
	mux.HandleFunc("/api/v1/timeline", timeline.Handler(assembler))

	// Recorder management API — registration, heartbeats, camera CRUD
	mux.HandleFunc("/api/v1/recorders/register", recorderHandlers.RegisterHandler())
	mux.HandleFunc("/api/v1/recorders/heartbeat", recorderHandlers.HeartbeatHandler())
	mux.HandleFunc("/api/v1/recorders", recorderHandlers.ListRecordersHandler())
	// Camera CRUD — registered on a Gin engine so cameraapi (which uses
	// gin.IRouter) can be wired in directly. The Gin engine is mounted as
	// an http.Handler on /api/v1/cameras and /api/v1/cameras/ so that both
	// the collection and item routes are served. The legacy per-method
	// switch is superseded by the Gin router.
	if nDB != nil {
		gin.SetMode(gin.ReleaseMode)
		ginRouter := gin.New()
		ginRouter.Use(gin.Recovery())
		cameraapi.NewHandler(nDB).Register(ginRouter)
		systemapi.NewHandler("directory").Register(ginRouter)
		mux.Handle("/api/v1/cameras", ginRouter)
		mux.Handle("/api/v1/cameras/", ginRouter)
		mux.Handle("/system/", ginRouter)
	} else {
		// Fallback: legacy ad-hoc handler when NVR DB is unavailable.
		mux.HandleFunc("/api/v1/cameras", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				recorderHandlers.CreateCameraHandler()(w, r)
			case http.MethodGet:
				recorderHandlers.ListCamerasHandler()(w, r)
			case http.MethodDelete:
				recorderHandlers.DeleteCameraHandler()(w, r)
			default:
				http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			}
		})
	}

	// Fan-out query endpoints — query all recorders and merge results
	mux.HandleFunc("/api/v1/query/recordings", fanoutSvc.FanoutRecordingsHandler())
	mux.HandleFunc("/api/v1/query/events", fanoutSvc.FanoutEventsHandler())
	mux.HandleFunc("/api/v1/query/timeline", fanoutSvc.FanoutTimelineHandler())
	mux.HandleFunc("/api/v1/query/health", fanoutSvc.FanoutHealthHandler())

	// Admin API — users, roles, schedules, retention, alerts, audit, exports
	adminStore := adminapi.NewStore(ddb.DB)
	adminHandlers := &adminapi.Handlers{
		Store:  adminStore,
		Logger: log.With(slog.String("component", "adminapi")),
	}
	mux.HandleFunc("/api/v1/admin/users", adminHandlers.UsersHandler())
	mux.HandleFunc("/api/v1/admin/users/by-id", adminHandlers.UserByIDHandler())
	mux.HandleFunc("/api/v1/admin/roles", adminHandlers.RolesHandler())
	mux.HandleFunc("/api/v1/admin/schedules", adminHandlers.SchedulesHandler())
	mux.HandleFunc("/api/v1/admin/retention", adminHandlers.RetentionHandler())
	mux.HandleFunc("/api/v1/admin/alert-rules", adminHandlers.AlertRulesHandler())
	mux.HandleFunc("/api/v1/admin/audit", adminHandlers.AuditHandler())
	mux.HandleFunc("/api/v1/admin/exports", adminHandlers.ExportJobsHandler())
	mux.HandleFunc("/api/v1/admin/exports/by-id", adminHandlers.ExportJobByIDHandler())

	// Auth API — login, refresh, logout
	// Derive a stable RSA-2048 signing key from the master key so JWTs survive
	// restarts without needing to store a separate key file.
	jwtKey, jwtKeyErr := deriveJWTSigningKey(cfg.MasterKey)
	if jwtKeyErr != nil {
		_ = srv.cleanup(ctx)
		return nil, fmt.Errorf("directory: derive JWT signing key: %w", jwtKeyErr)
	}
	localProvider := authapi.NewLocalAuthProvider(ddb, jwtKey)
	// defaultTenant is the single on-prem tenant; the value is not validated
	// server-side for local auth so any constant works.
	defaultTenant := authapi.TenantRef{Type: "onprem", ID: "default"}
	authHandlers := authapi.NewHandlers(
		localProvider,
		func(_ *http.Request) authapi.TenantRef { return defaultTenant },
		func(_ context.Context) (authapi.SessionID, bool) { return "", false }, // logout not context-based in this path
		log.With(slog.String("component", "authapi")),
	)
	mux.HandleFunc("/api/v1/auth/login", authHandlers.Login())
	mux.HandleFunc("/api/v1/auth/refresh", authHandlers.Refresh())
	mux.HandleFunc("/api/v1/auth/logout", authHandlers.Logout())

	// Web UI — SPA fallback at /admin
	mux.Handle("/admin/", webui.Handler("/admin"))
	mux.Handle("/admin", http.RedirectHandler("/admin/", http.StatusMovedPermanently))

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	srv.HTTPServer = httpServer

	// Start the HTTP server in a goroutine.
	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		_ = srv.cleanup(ctx)
		return nil, fmt.Errorf("directory: listen %s: %w", cfg.ListenAddr, err)
	}

	// Update the server address to the actual bound address (useful when port is 0).
	srv.HTTPServer.Addr = ln.Addr().String()

	go func() {
		log.Info("directory: HTTP server listening", "addr", ln.Addr().String())
		if serveErr := httpServer.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			log.Error("directory: HTTP server error", "error", serveErr)
		}
	}()

	// ---------------------------------------------------------------
	// 7. Start mDNS broadcaster (optional)
	// ---------------------------------------------------------------
	if *cfg.MDNSEnabled {
		_, portStr, _ := net.SplitHostPort(ln.Addr().String())
		port := 9997
		if portStr != "" {
			fmt.Sscanf(portStr, "%d", &port) //nolint:errcheck
		}

		broadcaster, mdnsErr := mdns.NewBroadcaster(mdns.BroadcasterConfig{
			InstanceName: cfg.MDNSInstanceName,
			Port:         port,
			TXTRecords:   map[string]string{"fingerprint": ca.Fingerprint()},
			Logger:       log.With(slog.String("component", "mdns")),
		})
		if mdnsErr != nil {
			// mDNS is best-effort; log but do not fail boot.
			log.Warn("directory: mDNS broadcaster failed to start", "error", mdnsErr)
		} else {
			srv.Broadcaster = broadcaster
		}
	}

	log.Info("directory: Directory mode started successfully",
		"addr", srv.HTTPServer.Addr,
		"data_dir", cfg.DataDir,
		"ca_fingerprint", ca.Fingerprint(),
	)

	return srv, nil
}

// Addr returns the HTTP server's bound address. Useful in tests when
// ListenAddr was ":0".
func (ds *DirectoryServer) Addr() string {
	if ds.HTTPServer != nil {
		return ds.HTTPServer.Addr
	}
	return ""
}

// cleanup is used during boot to release partially-initialized resources.
func (ds *DirectoryServer) cleanup(ctx context.Context) error {
	return ds.Shutdown(ctx)
}

// -----------------------------------------------------------------------
// Adapters — bridge Directory subsystem interfaces to the SQLite DB
// -----------------------------------------------------------------------

// dbCameraResolver implements streams.CameraResolver by looking up the
// recorder that owns a camera in the assigned_cameras table.
type dbCameraResolver struct {
	db *sql.DB
}

func (r *dbCameraResolver) Resolve(cameraID string) (recorderBaseURL, streamPath string, err error) {
	var recorderID, name string
	err = r.db.QueryRow(
		`SELECT recorder_id, name FROM assigned_cameras WHERE camera_id = ?`,
		cameraID,
	).Scan(&recorderID, &name)
	if err != nil {
		return "", "", fmt.Errorf("camera %s not found: %w", cameraID, err)
	}
	// The recorder base URL is looked up from the recorders table.
	// For now we use the recorder_id as a placeholder host.
	var recorderAddr string
	addrErr := r.db.QueryRow(
		`SELECT COALESCE(
			(SELECT json_extract(hardware_json, '$.address') FROM recorders WHERE id = ?),
			?
		)`,
		recorderID, recorderID+".kaivue.local:8554",
	).Scan(&recorderAddr)
	if addrErr != nil {
		recorderAddr = recorderID + ".kaivue.local:8554"
	}
	return recorderAddr, name, nil
}

// dbSegmentStore implements timeline.SegmentStore by querying the
// segment_index table in the Directory SQLite DB.
type dbSegmentStore struct {
	db *sql.DB
}

func (s *dbSegmentStore) QuerySegments(cameraIDs []string, start, end time.Time) ([]timeline.Segment, error) {
	if len(cameraIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(cameraIDs))
	args := make([]any, 0, len(cameraIDs)+2)
	for i, id := range cameraIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339))

	query := fmt.Sprintf(`
		SELECT camera_id, recorder_id, start_time, end_time
		FROM segment_index
		WHERE camera_id IN (%s)
		  AND end_time >= ?
		  AND start_time <= ?
		ORDER BY start_time
	`, strings.Join(placeholders, ","))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("timeline: query segments: %w", err)
	}
	defer rows.Close()

	var result []timeline.Segment
	for rows.Next() {
		var seg timeline.Segment
		var startStr, endStr string
		if err := rows.Scan(&seg.CameraID, &seg.RecorderID, &startStr, &endStr); err != nil {
			return nil, fmt.Errorf("timeline: scan segment: %w", err)
		}
		seg.Start, _ = time.Parse(time.RFC3339, startStr)
		seg.End, _ = time.Parse(time.RFC3339, endStr)
		result = append(result, seg)
	}
	return result, rows.Err()
}

// derivePairingKey deterministically derives an ed25519 key from the master
// key for pairing token signing. This uses SHA-256 as a KDF (acceptable for
// this use case since the master key is already high-entropy).
func derivePairingKey(masterKey []byte) ed25519.PrivateKey {
	// Use HKDF-like expansion: SHA-256(masterKey || "pairing-signing-key-v1")
	// to get 32 bytes of seed, then use ed25519.NewKeyFromSeed.
	h := sha256.New()
	h.Write(masterKey)
	h.Write([]byte("pairing-signing-key-v1"))
	seed := h.Sum(nil)
	return ed25519.NewKeyFromSeed(seed)
}

// deriveJWTSigningKey deterministically derives an RSA-2048 private key from
// the master key using HKDF-SHA256 as a deterministic source of randomness.
// This means the same nvrJWTSecret always produces the same RSA key, so tokens
// remain verifiable across restarts without storing a key file.
func deriveJWTSigningKey(masterKey []byte) (*rsa.PrivateKey, error) {
	reader := hkdf.New(sha256.New, masterKey, []byte("jwt-signing-key-v1"), nil)
	// io.Reader that produces deterministic bytes from the master key.
	limitedReader := io.LimitReader(reader, 1<<20) // 1 MiB upper bound — RSA gen needs ~1 KiB
	return rsa.GenerateKey(limitedReader, 2048)
}

// -----------------------------------------------------------------------
// Booter — implements kairuntime.DirectoryBooter
// -----------------------------------------------------------------------

// Booter implements kairuntime.DirectoryBooter so Core can wire it in
// and the dispatch shim can call Boot() without knowing the concrete type.
type Booter struct {
	srv *DirectoryServer
}

// NewBooter returns a DirectoryBooter ready to be set on Core.
func NewBooter() kairuntime.DirectoryBooter {
	return &Booter{}
}

// Boot starts the Directory subsystem. cfg must be *conf.Conf.
func (b *Booter) Boot(ctx context.Context, cfg any, logger *slog.Logger) error {
	c, ok := cfg.(*conf.Conf)
	if !ok {
		return fmt.Errorf("directory/booter: expected *conf.Conf, got %T", cfg)
	}

	if logger == nil {
		logger = slog.Default()
	}

	masterKey := []byte(c.NVRJWTSecret)
	if len(masterKey) == 0 {
		return fmt.Errorf("directory/booter: nvrJWTSecret must be set for directory mode")
	}

	bootCfg := BootConfig{
		DataDir:              c.NVRDirectoryDataDir,
		ListenAddr:           c.APIAddress,
		MasterKey:            masterKey,
		RecorderServiceToken: c.NVRServiceToken,
		Logger:               logger,
	}

	srv, err := Boot(ctx, bootCfg)
	if err != nil {
		return err
	}
	b.srv = srv
	return nil
}

// PairingService returns the in-process pairing token generator for
// AllInOne auto-pairing.
func (b *Booter) PairingService() kairuntime.PairingTokenGenerator {
	if b.srv == nil || b.srv.PairingSvc == nil {
		return nil
	}
	return &booterPairingAdapter{svc: b.srv.PairingSvc}
}

// Shutdown gracefully stops the Directory subsystem.
func (b *Booter) Shutdown(ctx context.Context) error {
	if b.srv == nil {
		return nil
	}
	return b.srv.Shutdown(ctx)
}

// booterPairingAdapter wraps *pairing.Service to implement
// kairuntime.PairingTokenGenerator.
type booterPairingAdapter struct {
	svc *pairing.Service
}

func (a *booterPairingAdapter) GeneratePairingToken() (string, error) {
	result, err := a.svc.Generate(
		context.Background(),
		pairing.UserID("system:auto-pair"),
		[]string{"recorder"},
		"",
	)
	if err != nil {
		return "", err
	}
	return result.Encoded, nil
}
