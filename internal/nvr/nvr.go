// Package nvr implements the NVR subsystem for Raikada.
package nvr

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/bluenviron/mediamtx/internal/recordstore"

	"github.com/bluenviron/mediamtx/internal/nvr/ai"
	"github.com/bluenviron/mediamtx/internal/nvr/alerts"
	"github.com/bluenviron/mediamtx/internal/nvr/api"
	"github.com/bluenviron/mediamtx/internal/nvr/backchannel"
	"github.com/bluenviron/mediamtx/internal/nvr/backup"
	"github.com/bluenviron/mediamtx/internal/nvr/connmgr"
	"github.com/bluenviron/mediamtx/internal/nvr/crypto"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/integrity"
	"github.com/bluenviron/mediamtx/internal/nvr/managed"
	"github.com/bluenviron/mediamtx/internal/nvr/metrics"
	"github.com/bluenviron/mediamtx/internal/nvr/recovery"
	"github.com/bluenviron/mediamtx/internal/nvr/onvif"
	"github.com/bluenviron/mediamtx/internal/nvr/scheduler"
	"github.com/bluenviron/mediamtx/internal/nvr/storage"
	"github.com/bluenviron/mediamtx/internal/nvr/updater"
	"github.com/bluenviron/mediamtx/internal/nvr/webhook"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

// NVR is the main NVR subsystem struct.
type NVR struct {
	DatabasePath   string
	JWTSecret      string
	ConfigPath     string
	APIAddress     string
	RecordingsPath string

	// Managed mode fields (set when a Directory URL is configured).
	DirectoryURL    string
	ServiceToken    string
	InternalAPIAddr string
	RecorderID      string

	database   *db.DB
	yamlWriter *yamlwriter.Writer
	sched      *scheduler.Scheduler
	privateKey *rsa.PrivateKey
	jwksJSON   []byte
	discovery   *onvif.Discovery
	events      *api.EventBroadcaster
	callbackMgr *onvif.CallbackManager
	wsServer    *http.Server // separate WebSocket server for notifications

	ctx       context.Context
	ctxCancel context.CancelFunc

	aiDetector      *ai.Detector
	aiEmbedder      *ai.Embedder
	aiPipelines     map[string]*ai.Pipeline // camera ID -> pipeline
	aiModelManager  *ai.ModelManager

	hlsHandler *api.HLSHandler
	storageMgr *storage.Manager

	metricsCollector *metrics.Collector

	cameraStatusDone chan struct{} // closed to stop the camera status monitor

	integrityScanner  *integrity.Scanner
	connMgr           *connmgr.Manager
	maintenanceRunner *db.MaintenanceRunner

	backchannelMgr      *backchannel.Manager
	exportHandler       *api.ExportHandler
	emailSender         *alerts.EmailSender
	alertEvaluator      *alerts.Evaluator
	backupSvc           *backup.Service
	tlsManager          *crypto.TLSManager
	detectionEvaluator  *scheduler.DetectionEvaluator
	webhookDispatcher   *webhook.Dispatcher

	managedClient      *managed.Client
	managedInternalAPI *managed.InternalAPI

	firstBoot bool // true when the DB was freshly created (no prior state)
}

// StartOptions controls which building blocks are activated during initialization.
// In legacy mode (empty mode field), all options default to true. In directory
// or recorder mode, only the relevant blocks are started.
type StartOptions struct {
	Camera    bool // ONVIF discovery, connection manager, camera status monitor
	Recording bool // Scheduler, storage manager, recovery, integrity, fragment backfill
	AI        bool // ONNX detection, CLIP embedder, AI pipelines
	Auth      bool // RSA key generation, TLS certificates
	Alerts    bool // Alert evaluator, email sender, webhooks
	Managed   bool // Directory client, internal query API
	Metrics   bool // System metrics collector
	Backup    bool // Backup service
	Events    bool // WebSocket notification server, event broadcaster
}

// AllOptions returns StartOptions with every block enabled (legacy mode).
func AllOptions() StartOptions {
	return StartOptions{
		Camera:    true,
		Recording: true,
		AI:        true,
		Auth:      true,
		Alerts:    true,
		Managed:   false, // only when DirectoryURL is set
		Metrics:   true,
		Backup:    true,
		Events:    true,
	}
}

// DirectoryOptions returns StartOptions for directory mode — camera management,
// auth, alerts, events, but no recording or AI (those run on recorders).
func DirectoryOptions() StartOptions {
	return StartOptions{
		Camera:  true,
		Auth:    true,
		Alerts:  true,
		Metrics: true,
		Events:  true,
		Backup:  true,
	}
}

// RecorderOptions returns StartOptions for recorder mode — recording, AI, and
// managed mode, but no camera management or auth (Directory handles those).
func RecorderOptions() StartOptions {
	return StartOptions{
		Recording: true,
		AI:        true,
		Managed:   true,
		Metrics:   true,
		Events:    true,
	}
}

// Initialize sets up the NVR subsystem with the given options. Call with
// AllOptions() for legacy mode, DirectoryOptions() for directory mode,
// or RecorderOptions() for recorder mode.
func (n *NVR) Initialize() error {
	return n.InitializeWithOptions(AllOptions())
}

// InitializeWithOptions sets up the NVR subsystem, starting only the building
// blocks enabled in opts.
func (n *NVR) InitializeWithOptions(opts StartOptions) error {
	n.ctx, n.ctxCancel = context.WithCancel(context.Background())

	// --- Core: always runs (database, config, encryption key) ---------------

	if err := n.initCore(); err != nil {
		return err
	}

	// --- Events: broadcaster + notification server --------------------------

	n.events = api.NewEventBroadcaster()
	if opts.Events {
		n.startNotificationServer()
	}

	// --- Camera: ONVIF, connection manager, status monitor ------------------

	if opts.Camera {
		n.initCamera()
	}

	// --- Recording: scheduler, storage, recovery, integrity -----------------

	if opts.Recording {
		n.initRecording()
	}

	// --- Metrics: system metrics collector ----------------------------------

	if opts.Metrics {
		n.metricsCollector = metrics.New(360, 10*time.Second)
		n.metricsCollector.Start()
	}

	// --- Backup: backup service ---------------------------------------------

	if opts.Backup {
		backupDir := filepath.Join(filepath.Dir(n.DatabasePath), "backups")
		n.backupSvc = backup.New(n.DatabasePath, n.ConfigPath, backupDir)
		if err := n.backupSvc.Init(); err != nil {
			log.Printf("[NVR] [WARN] backup service init: %v", err)
		}
	}

	// --- Auth: RSA keys, TLS certificates -----------------------------------

	if opts.Auth {
		if err := n.initAuth(); err != nil {
			return err
		}
	}

	// --- AI: detection pipelines, embedder ----------------------------------

	if opts.AI {
		n.initAI()
	}

	// --- Alerts: evaluator, email sender ------------------------------------

	if opts.Alerts {
		n.initAlerts()
	}

	// --- Managed: Directory client, internal API ----------------------------

	if opts.Managed || n.DirectoryURL != "" {
		if err := n.startManagedMode(); err != nil {
			log.Printf("[NVR] [WARN] managed mode failed to start: %v", err)
		}
	}

	return nil
}

// initCore sets up the database, JWT secret, YAML writer, and encryption key.
// This always runs regardless of mode.
func (n *NVR) initCore() error {
	if n.JWTSecret == "" {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return fmt.Errorf("generate JWT secret: %w", err)
		}
		n.JWTSecret = hex.EncodeToString(secret)

		if n.ConfigPath != "" {
			w := yamlwriter.New(n.ConfigPath)
			if err := w.SetTopLevelValue("nvrJWTSecret", n.JWTSecret); err != nil {
				return fmt.Errorf("persist JWT secret: %w", err)
			}
		}
	} else {
		if len(n.JWTSecret) < 32 {
			return fmt.Errorf("JWT secret must be at least 32 characters (got %d); set a stronger nvrJWTSecret in config", len(n.JWTSecret))
		}
	}

	if strings.HasPrefix(n.DatabasePath, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			n.DatabasePath = filepath.Join(home, n.DatabasePath[2:])
		}
	}

	dbDir := filepath.Dir(n.DatabasePath)
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}

	_, statErr := os.Stat(n.DatabasePath)
	n.firstBoot = os.IsNotExist(statErr)

	var err error
	n.database, err = db.Open(n.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	if n.firstBoot {
		if err := n.bootstrapFirstRun(); err != nil {
			n.database.Close()
			return fmt.Errorf("first-boot setup: %w", err)
		}
	}

	_ = n.database.CloseOrphanedMotionEvents()

	n.maintenanceRunner = n.database.StartMaintenance(db.DefaultMaintenanceConfig(), func(alertType, message string) {
		log.Printf("[NVR] [db-maintenance] ALERT [%s]: %s", alertType, message)
		if n.events != nil {
			n.events.Publish(api.Event{
				Type:    alertType,
				Message: message,
			})
		}
	})

	if err := n.database.SeedDefaultTemplates(); err != nil {
		log.Printf("nvr: failed to seed default templates: %v", err)
	}

	n.yamlWriter = yamlwriter.New(n.ConfigPath)
	n.migrateMediaMTXPaths()

	return nil
}

// initCamera starts ONVIF discovery, connection manager, backchannel manager,
// and camera status monitor.
func (n *NVR) initCamera() {
	n.discovery = onvif.NewDiscovery()
	n.callbackMgr = onvif.NewCallbackManager()

	encKey := crypto.DeriveKey(n.JWTSecret, "nvr-credential-encryption")

	n.backchannelMgr = backchannel.NewManager(func(cameraID string) (string, string, string, error) {
		cam, err := n.database.GetCamera(cameraID)
		if err != nil {
			return "", "", "", err
		}
		password := cam.ONVIFPassword
		if len(encKey) > 0 && strings.HasPrefix(password, "enc:") {
			ct, decErr := base64.StdEncoding.DecodeString(strings.TrimPrefix(password, "enc:"))
			if decErr == nil {
				if pt, decErr2 := crypto.Decrypt(encKey, ct); decErr2 == nil {
					password = string(pt)
				}
			}
		}
		return cam.ONVIFEndpoint, cam.ONVIFUsername, password, nil
	})

	n.cameraStatusDone = make(chan struct{})
	go n.runCameraStatusMonitor(n.cameraStatusDone)

	n.connMgr = connmgr.New(n.database)
	n.connMgr.OnStateChange = func(cameraID, oldState, newState, errMsg string) {
		if n.events != nil {
			msg := fmt.Sprintf("%s → %s", oldState, newState)
			if errMsg != "" {
				msg += ": " + errMsg
			}
			n.events.Publish(api.Event{
				Type:    "connection_state_change",
				Camera:  cameraID,
				Message: msg,
			})
		}
	}
	if err := n.connMgr.Start(); err != nil {
		log.Printf("[NVR] connection manager start error: %v", err)
	}
}

// initRecording starts the scheduler, storage manager, recovery, integrity
// scanner, and fragment backfill.
func (n *NVR) initRecording() {
	encKey := crypto.DeriveKey(n.JWTSecret, "nvr-credential-encryption")

	if n.callbackMgr == nil {
		n.callbackMgr = onvif.NewCallbackManager()
	}

	n.sched = scheduler.New(n.database, n.yamlWriter, encKey, n.callbackMgr, n.APIAddress, n.RecordingsPath)
	n.sched.SetEventBroadcaster(n.events)
	n.sched.Start()

	n.storageMgr = storage.New(n.database, n.yamlWriter, n.RecordingsPath, n.APIAddress)
	n.storageMgr.SetEventPublisher(n.events)
	n.storageMgr.Start()

	n.syncAudioTranscodeState()

	if n.RecordingsPath != "" {
		recoveryCfg := recovery.RunConfig{
			RecordDirs: []string{n.RecordingsPath},
			DB:         &recoveryDBAdapter{db: n.database},
			Reconciler: &recoveryReconcileAdapter{db: n.database, nvr: n},
		}
		if result, err := recovery.Run(recoveryCfg); err != nil {
			log.Printf("NVR: recovery scan failed: %v", err)
		} else if result.Scanned > 0 {
			log.Printf("NVR: recovery complete — scanned=%d repaired=%d unrecoverable=%d",
				result.Scanned, result.Repaired, result.Unrecoverable)
		}
	}

	n.startFragmentBackfill()

	n.integrityScanner = &integrity.Scanner{
		Interval:  1 * time.Hour,
		BatchSize: 100,
		FetchFunc: func(cutoff time.Time, batchSize int) ([]integrity.ScanItem, error) {
			recs, err := n.database.GetRecordingsNeedingVerification(cutoff, batchSize)
			if err != nil {
				return nil, err
			}
			items := make([]integrity.ScanItem, 0, len(recs))
			for _, rec := range recs {
				fragCount := 0
				if frags, err := n.database.GetFragments(rec.ID); err == nil {
					fragCount = len(frags)
				}
				items = append(items, integrity.ScanItem{
					RecordingID: rec.ID,
					CameraID:    rec.CameraID,
					Info: integrity.RecordingInfo{
						FilePath:      rec.FilePath,
						FileSize:      rec.FileSize,
						InitSize:      rec.InitSize,
						FragmentCount: fragCount,
						DurationMs:    rec.DurationMs,
					},
				})
			}
			return items, nil
		},
		OnResult: func(recordingID int64, result integrity.VerificationResult) {
			now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			var detail *string
			if result.Detail != "" {
				detail = &result.Detail
			}
			n.database.UpdateRecordingStatus(recordingID, result.Status, detail, now)

			if result.Status == integrity.StatusCorrupted && n.events != nil {
				rec, err := n.database.GetRecording(recordingID)
				if err == nil {
					n.events.PublishSegmentCorrupted(rec.CameraID, recordingID, rec.FilePath, result.Detail)
				}
			}
		},
	}
	go n.integrityScanner.Run(n.ctx)
}

// initAuth loads or generates RSA keys and TLS certificates.
func (n *NVR) initAuth() error {
	if err := n.loadOrGenerateKeys(); err != nil {
		n.database.Close()
		return fmt.Errorf("load or generate keys: %w", err)
	}

	certDir := filepath.Join(filepath.Dir(n.DatabasePath), "tls")
	n.tlsManager = crypto.NewTLSManager(certDir)
	generated, err := n.tlsManager.EnsureCertificate()
	if err != nil {
		log.Printf("[NVR] [WARN] TLS certificate auto-generation failed: %v", err)
	} else if generated {
		log.Printf("[NVR] [INFO] auto-generated self-signed TLS certificate in %s", certDir)
	}

	go n.runCertExpiryMonitor()
	return nil
}

// initAI initializes AI detection pipelines and embedders.
func (n *NVR) initAI() {
	n.aiPipelines = make(map[string]*ai.Pipeline)
	if err := ai.InitONNXRuntime(); err != nil {
		log.Printf("AI: ONNX Runtime not available: %v", err)
		return
	}

	modelsDir := "./models"
	nanoPath := filepath.Join(modelsDir, "yolov8n.onnx")
	if _, err := os.Stat(nanoPath); err == nil {
		det, err := ai.NewDetector(nanoPath)
		if err != nil {
			log.Printf("AI: failed to load YOLOv8n: %v", err)
		} else {
			n.aiDetector = det
			log.Printf("AI: YOLOv8n detector loaded from %s", nanoPath)
		}
	} else {
		log.Printf("AI: YOLO model not found at %s, detection disabled", nanoPath)
	}

	n.aiModelManager = ai.NewModelManager(modelsDir, n.aiDetector, nanoPath)
	log.Printf("AI: model manager initialized (models dir: %s)", modelsDir)

	visualPath := filepath.Join(modelsDir, "clip-vit-b32-visual.onnx")
	textPath := filepath.Join(modelsDir, "clip-vit-b32-text.onnx")
	vocabPath := filepath.Join(modelsDir, "clip-vocab.json")
	projPath := filepath.Join(modelsDir, "clip-visual-projection.bin")
	if _, err := os.Stat(visualPath); err == nil {
		if _, err := os.Stat(textPath); err == nil {
			if _, err := os.Stat(vocabPath); err == nil {
				emb, err := ai.NewEmbedder(visualPath, textPath, vocabPath, projPath)
				if err != nil {
					log.Printf("AI: failed to load CLIP embedder: %v", err)
				} else {
					n.aiEmbedder = emb
					log.Printf("AI: CLIP embedder loaded (with visual projection)")
				}
			}
		}
	}

	n.startAIPipelines()

	n.detectionEvaluator = scheduler.NewDetectionEvaluator(n.database, n)
	n.detectionEvaluator.Start()
}

// initAlerts starts the alert evaluator and email sender.
func (n *NVR) initAlerts() {
	n.emailSender = &alerts.EmailSender{DB: n.database}
	n.alertEvaluator = &alerts.Evaluator{
		DB:             n.database,
		RecordingsPath: n.RecordingsPath,
		EmailSender:    n.emailSender,
	}
	n.alertEvaluator.Start(n.ctx)
}

// IsManagedMode reports whether this recorder is operating under Directory control.
func (n *NVR) IsManagedMode() bool {
	return n.DirectoryURL != ""
}

// startManagedMode initializes the Directory client and internal API.
func (n *NVR) startManagedMode() error {
	cfg := managed.Config{
		DirectoryURL:   n.DirectoryURL,
		ServiceToken:   n.ServiceToken,
		InternalListenAddr: n.InternalAPIAddr,
		RecorderID:     n.RecorderID,
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	// Ensure a stable recorder ID.
	if cfg.RecorderID == "" {
		id, err := n.database.GetConfig("recorder_id")
		if err != nil || id == "" {
			newID := fmt.Sprintf("rec-%s", generateShortID())
			_ = n.database.SetConfig("recorder_id", newID)
			cfg.RecorderID = newID
		} else {
			cfg.RecorderID = id
		}
		n.RecorderID = cfg.RecorderID
	}

	recordingsPath := n.RecordingsPath
	if recordingsPath == "" {
		recordingsPath = "./recordings/"
	}

	// Start the internal API for Directory queries.
	n.managedInternalAPI = &managed.InternalAPI{
		DB:             n.database,
		Scheduler:      n.sched,
		StorageManager: n.storageMgr,
		RecordingsPath: recordingsPath,
		ServiceToken:   n.ServiceToken,
		RecorderID:     cfg.RecorderID,
	}
	if err := n.managedInternalAPI.Start(cfg.ListenAddr()); err != nil {
		return fmt.Errorf("start internal API: %w", err)
	}

	// Start the Directory client (register + heartbeat).
	n.managedClient = managed.NewClient(cfg, n, n.version())
	go n.managedClient.Run(n.ctx)

	log.Printf("[NVR] managed mode active — Directory: %s, Recorder ID: %s, Internal API: %s",
		n.DirectoryURL, cfg.RecorderID, n.managedInternalAPI.Addr())
	return nil
}

// CameraCount implements managed.HealthProvider.
func (n *NVR) CameraCount() int {
	cams, err := n.database.ListCameras()
	if err != nil {
		return 0
	}
	return len(cams)
}

// GetRecordingsPath implements managed.HealthProvider.
func (n *NVR) GetRecordingsPath() string {
	if n.RecordingsPath != "" {
		return n.RecordingsPath
	}
	return "./recordings/"
}

// version returns the NVR version string for registration.
func (n *NVR) version() string {
	return "dev" // TODO: wire build-time version
}

func generateShortID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// runCameraStatusMonitor polls the Raikada /v3/paths/list endpoint every 5
// seconds and publishes camera_online/camera_offline events on transitions.
func (n *NVR) runCameraStatusMonitor(done <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Map of Raikada path name → ready state from the previous poll.
	prevReady := make(map[string]bool)
	firstPoll := true

	addr := n.APIAddress
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	listURL := fmt.Sprintf("http://%s/v3/paths/list", addr)
	client := &http.Client{Timeout: 5 * time.Second}

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			resp, err := client.Get(listURL)
			if err != nil {
				// Raikada not yet ready — skip this tick.
				continue
			}

			var result struct {
				Items []struct {
					Name  string `json:"name"`
					Ready bool   `json:"ready"`
				} `json:"items"`
			}
			decodeErr := func() error {
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					return fmt.Errorf("status %d", resp.StatusCode)
				}
				return json.NewDecoder(resp.Body).Decode(&result)
			}()
			if decodeErr != nil {
				continue
			}

			// Build a name → ready map for this tick.
			currReady := make(map[string]bool, len(result.Items))
			for _, item := range result.Items {
				currReady[item.Name] = item.Ready
			}

			if firstPoll {
				// On the first successful poll, seed state without firing events.
				prevReady = currReady
				firstPoll = false
				continue
			}

			// Detect transitions and look up camera names from the DB.
			cameras, dbErr := n.database.ListCameras()
			if dbErr != nil {
				prevReady = currReady
				continue
			}

			// Build a Raikada path → camera name index.
			pathToName := make(map[string]string, len(cameras))
			for _, cam := range cameras {
				if cam.MediaMTXPath != "" {
					pathToName[cam.MediaMTXPath] = cam.Name
				}
			}

			for path, ready := range currReady {
				wasReady, known := prevReady[path]
				if !known {
					wasReady = false
				}
				if ready == wasReady {
					continue
				}
				cameraName, ok := pathToName[path]
				if !ok {
					continue // not a NVR-managed path
				}
				if ready {
					n.events.PublishCameraOnline(cameraName)
					log.Printf("[NVR] camera online: %s (%s)", cameraName, path)
					// Notify connection manager to trigger immediate reconnect.
					if n.connMgr != nil {
						for _, cam := range cameras {
							if cam.MediaMTXPath == path {
								n.connMgr.NotifyOnline(cam.ID)
								break
							}
						}
					}
				} else {
					n.events.PublishCameraOffline(cameraName)
					log.Printf("[NVR] camera offline: %s (%s)", cameraName, path)
				}
			}

			// Check sub-stream paths (format: <main_path>~<prefix>)
			for _, cam := range cameras {
				if cam.MediaMTXPath == "" {
					continue
				}
				subPrefix := cam.MediaMTXPath + "~"
				for pathName, ready := range currReady {
					if !strings.HasPrefix(pathName, subPrefix) {
						continue
					}
					prevReady2, existed := prevReady[pathName]
					if !existed {
						continue // first observation, skip event
					}
					if prevReady2 && !ready {
						log.Printf("[NVR] sub-stream offline: %s (camera %s)", pathName, cam.Name)
						n.events.PublishCameraOffline(cam.Name + " (" + strings.TrimPrefix(pathName, subPrefix) + ")")
					} else if !prevReady2 && ready {
						log.Printf("[NVR] sub-stream online: %s (camera %s)", pathName, cam.Name)
						n.events.PublishCameraOnline(cam.Name + " (" + strings.TrimPrefix(pathName, subPrefix) + ")")
					}
				}
			}

			prevReady = currReady
		}
	}
}

// startNotificationServer starts a WebSocket server on port 9998.
func (n *NVR) startNotificationServer() {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true // Allow non-browser clients
			}
			// Allow same-host connections (any port)
			host := r.Host
			if idx := strings.LastIndex(host, ":"); idx >= 0 {
				host = host[:idx]
			}
			return strings.Contains(origin, host)
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		// Allow CORS preflight — reflect the request origin instead of wildcard.
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		conn.WriteJSON(map[string]string{"type": "connected"})

		// Active-event replay removed: the new modular ai.Pipeline does not
		// expose HasActiveEvent/CameraName. Clients will receive new events
		// via the event broadcaster as they occur.

		ch := n.events.Subscribe()
		defer n.events.Unsubscribe(ch)

		// Detect client disconnect.
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()

		pingTicker := time.NewTicker(30 * time.Second)
		defer pingTicker.Stop()

		for {
			select {
			case <-done:
				return
			case <-pingTicker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
			case evt, ok := <-ch:
				if !ok {
					return
				}
				if err := conn.WriteJSON(evt); err != nil {
					return
				}
			}
		}
	})

	wsAddr := n.wsPort()
	n.wsServer = &http.Server{
		Addr:    wsAddr,
		Handler: mux,
	}

	go func() {
		if err := n.wsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("notification server error: %v\n", err)
		}
	}()
	fmt.Printf("NVR notification WebSocket server listening on %s\n", wsAddr)
}

// wsPort derives the WebSocket server port from the API address (API port + 1).
func (n *NVR) wsPort() string {
	port := strings.TrimPrefix(n.APIAddress, ":")
	p, err := strconv.Atoi(port)
	if err != nil {
		return ":9998" // fallback
	}
	return fmt.Sprintf(":%d", p+1)
}

// Close closes the NVR subsystem.
func (n *NVR) Close() {
	// Shut down managed mode components first.
	if n.managedInternalAPI != nil {
		n.managedInternalAPI.Shutdown()
	}

	if n.metricsCollector != nil {
		n.metricsCollector.Stop()
	}

	// Cancel the NVR lifecycle context so all pipeline goroutines exit.
	if n.ctxCancel != nil {
		n.ctxCancel()
	}

	// Stop detection evaluator before pipelines.
	if n.detectionEvaluator != nil {
		n.detectionEvaluator.Stop()
	}

	// Stop AI pipelines first so they don't write to the DB after it's closed.
	for id, p := range n.aiPipelines {
		p.Stop()
		log.Printf("AI: stopped pipeline for camera %s", id)
	}
	if n.aiModelManager != nil {
		n.aiModelManager.Close()
	} else if n.aiDetector != nil {
		n.aiDetector.Close()
	}
	if n.aiEmbedder != nil {
		n.aiEmbedder.Close()
	}

	if n.backchannelMgr != nil {
		n.backchannelMgr.CloseAll()
	}

	if n.backupSvc != nil {
		n.backupSvc.StopSchedule()
	}
	if n.exportHandler != nil {
		n.exportHandler.Stop()
	}
	if n.alertEvaluator != nil {
		n.alertEvaluator.Stop()
	}
	if n.connMgr != nil {
		n.connMgr.Stop()
	}
	if n.cameraStatusDone != nil {
		close(n.cameraStatusDone)
	}
	if n.wsServer != nil {
		n.wsServer.Close()
	}
	if n.storageMgr != nil {
		n.storageMgr.Stop()
	}
	if n.sched != nil {
		n.sched.Stop()
	}
	if n.maintenanceRunner != nil {
		n.maintenanceRunner.Stop()
	}
	if n.database != nil {
		n.database.Close()
	}
}

// syncAudioTranscodeState checks the YAML config for existing -live paths
// and updates the database to match. This handles the case where the
// audio_transcode column was added after -live paths already existed.
func (n *NVR) syncAudioTranscodeState() {
	paths, err := n.yamlWriter.GetNVRPaths()
	if err != nil {
		log.Printf("NVR: failed to read YAML paths for audio sync: %v", err)
		return
	}

	// Build a set of base paths that have a -live companion.
	liveSet := make(map[string]bool)
	for _, p := range paths {
		if strings.HasSuffix(p, "-live") {
			liveSet[strings.TrimSuffix(p, "-live")] = true
		}
	}
	if len(liveSet) == 0 {
		return
	}

	cameras, err := n.database.ListCameras()
	if err != nil {
		log.Printf("NVR: failed to list cameras for audio sync: %v", err)
		return
	}

	for _, cam := range cameras {
		if liveSet[cam.MediaMTXPath] && !cam.AudioTranscode {
			cam.AudioTranscode = true
			if err := n.database.UpdateCamera(cam); err != nil {
				log.Printf("NVR: failed to sync audio_transcode for %s: %v", cam.Name, err)
			} else {
				log.Printf("NVR: synced audio_transcode=true for %s", cam.Name)
			}
		}
	}
}

// migrateMediaMTXPaths updates camera MediaMTX paths from the old naming
// convention (nvr/<sanitized-name>) to the new convention (nvr/<camera-id>/main).
// It also verifies that every camera's Raikada path exists in the YAML config.
func (n *NVR) migrateMediaMTXPaths() {
	cameras, err := n.database.ListCameras()
	if err != nil {
		log.Printf("[NVR] [migration] failed to list cameras: %v", err)
		return
	}

	for _, cam := range cameras {
		expectedPath := "nvr/" + cam.ID + "/main"
		if cam.MediaMTXPath == expectedPath {
			continue // Already migrated.
		}

		oldPath := cam.MediaMTXPath
		cam.MediaMTXPath = expectedPath

		if err := n.database.UpdateCamera(cam); err != nil {
			log.Printf("[NVR] [migration] failed to update path for camera %s: %v", cam.ID, err)
			continue
		}

		// Resolve recording stream URL (prefer camera_streams, fall back to legacy rtsp_url).
		sourceURL, err := n.database.ResolveStreamURL(cam.ID, db.StreamRoleRecording)
		if err != nil || sourceURL == "" {
			sourceURL = cam.RTSPURL
		}

		yamlConfig := map[string]interface{}{
			"source": sourceURL,
			"record": true,
		}
		storagePath := cam.StoragePath
		if storagePath == "" {
			storagePath = n.RecordingsPath
		}
		yamlConfig["recordPath"] = storagePath + "/%path/%Y-%m/%d/%H-%M-%S-%f"

		if err := n.yamlWriter.AddPath(expectedPath, yamlConfig); err != nil {
			log.Printf("[NVR] [migration] failed to add new path for camera %s: %v", cam.ID, err)
			continue
		}

		if oldPath != "" {
			_ = n.yamlWriter.RemovePath(oldPath)
		}

		log.Printf("[NVR] [migration] migrated camera %s path: %s -> %s", cam.ID, oldPath, expectedPath)
	}
}

// startAIPipelines starts AI detection pipelines for all cameras that have
// ai_enabled set. Each pipeline decodes an RTSP stream and runs YOLO detection.
func (n *NVR) startAIPipelines() {
	if n.aiDetector == nil {
		return
	}
	n.aiPipelines = make(map[string]*ai.Pipeline)

	cameras, err := n.database.ListCameras()
	if err != nil {
		log.Printf("ai: failed to list cameras: %v", err)
		return
	}

	for _, cam := range cameras {
		if !cam.AIEnabled {
			continue
		}
		n.startSinglePipeline(cam)
	}

	log.Printf("ai: started %d pipelines", len(n.aiPipelines))
}

// startSinglePipeline resolves the best stream URL for a camera and starts
// an ai.Pipeline for it.
func (n *NVR) startSinglePipeline(cam *db.Camera) {
	// Resolve stream URL: explicit ai_stream_id > ai_detection role > legacy sub_stream_url > main rtsp_url
	streamURL := ""
	var streamWidth, streamHeight int

	if cam.AIStreamID != "" {
		stream, err := n.database.GetCameraStream(cam.AIStreamID)
		if err == nil {
			streamURL = stream.RTSPURL
			streamWidth = stream.Width
			streamHeight = stream.Height
		}
	}
	if streamURL == "" {
		resolved, err := n.database.ResolveStreamURL(cam.ID, db.StreamRoleAIDetection)
		if err == nil && resolved != "" {
			streamURL = resolved
		}
	}
	if streamURL == "" && cam.SubStreamURL != "" {
		streamURL = cam.SubStreamURL
	}
	if streamURL == "" && cam.RTSPURL != "" {
		streamURL = cam.RTSPURL
	}
	if streamURL == "" {
		log.Printf("ai: camera %s (%s) has no stream URL for AI", cam.ID, cam.Name)
		return
	}

	// Embed credentials into RTSP URL if needed.
	streamURL = n.embedCredentials(cam, streamURL)

	config := ai.PipelineConfig{
		CameraID:         cam.ID,
		CameraName:       cam.Name,
		StreamURL:        streamURL,
		StreamWidth:      streamWidth,
		StreamHeight:     streamHeight,
		ConfidenceThresh:         float32(cam.AIConfidence),
		TrackTimeout:             cam.AITrackTimeout,
		ConfidenceThresholdsJSON: cam.ConfidenceThresholds,
	}

	pipeline := ai.NewPipeline(config, n.aiDetector, n.aiEmbedder, n.database, n.events)
	if n.webhookDispatcher != nil {
		pipeline.SetDetectionCallback(n.webhookDispatcher.OnDetection)
	}
	pipeline.Start(n.ctx)
	n.aiPipelines[cam.ID] = pipeline
}

// embedCredentials embeds ONVIF credentials into an RTSP URL if they are not
// already present. The stored password is decrypted before embedding.
func (n *NVR) embedCredentials(cam *db.Camera, rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if u.User != nil && u.User.Username() != "" {
		return rawURL // already has credentials
	}
	username := cam.ONVIFUsername
	password := cam.ONVIFPassword
	if password != "" {
		encKey := crypto.DeriveKey(n.JWTSecret, "nvr-credential-encryption")
		password = n.decryptPassword(encKey, password)
	}
	if username != "" {
		u.User = url.UserPassword(username, password)
	}
	return u.String()
}

// RestartAIPipeline stops and restarts the AI pipeline for the given camera ID.
// Called by the API after camera settings change (credentials, AI toggle, etc.).
func (n *NVR) RestartAIPipeline(cameraID string) {
	if p, ok := n.aiPipelines[cameraID]; ok {
		p.Stop()
		delete(n.aiPipelines, cameraID)
	}

	if n.aiDetector == nil {
		return
	}

	cam, err := n.database.GetCamera(cameraID)
	if err != nil {
		log.Printf("ai: restart pipeline: get camera %s: %v", cameraID, err)
		return
	}

	if !cam.AIEnabled {
		return
	}

	n.startSinglePipeline(cam)
}

// StartDetectionPipeline starts the AI detection pipeline for a camera.
// Implements scheduler.DetectionPipelineController.
func (n *NVR) StartDetectionPipeline(cameraID string) {
	if n.aiDetector == nil {
		return
	}
	if _, running := n.aiPipelines[cameraID]; running {
		return // already running
	}

	cam, err := n.database.GetCamera(cameraID)
	if err != nil {
		log.Printf("ai: start detection pipeline: get camera %s: %v", cameraID, err)
		return
	}
	if !cam.AIEnabled {
		return
	}
	n.startSinglePipeline(cam)
}

// StopDetectionPipeline stops the AI detection pipeline for a camera.
// Implements scheduler.DetectionPipelineController.
func (n *NVR) StopDetectionPipeline(cameraID string) {
	if p, ok := n.aiPipelines[cameraID]; ok {
		p.Stop()
		delete(n.aiPipelines, cameraID)
	}
}

// IsDetectionPipelineRunning returns true if the AI pipeline is running for a camera.
// Implements scheduler.DetectionPipelineController.
func (n *NVR) IsDetectionPipelineRunning(cameraID string) bool {
	_, ok := n.aiPipelines[cameraID]
	return ok
}

// decryptPassword decrypts an ONVIF password from the DB if it was encrypted
// with the "enc:" prefix.
func (n *NVR) decryptPassword(encKey []byte, encrypted string) string {
	if len(encKey) == 0 || !strings.HasPrefix(encrypted, "enc:") {
		return encrypted
	}
	ciphertext, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encrypted, "enc:"))
	if err != nil {
		return encrypted
	}
	plain, err := crypto.Decrypt(encKey, ciphertext)
	if err != nil {
		return encrypted
	}
	return string(plain)
}

// IsSetupRequired returns true if no users exist in the database.
func (n *NVR) IsSetupRequired() bool {
	count, err := n.database.CountUsers()
	if err != nil {
		return true
	}
	return count == 0
}

// IsFirstBoot returns true when this is the first time the NVR has started
// (the database was freshly created during this Initialize call).
func (n *NVR) IsFirstBoot() bool {
	return n.firstBoot
}

// bootstrapFirstRun performs one-time setup on the very first launch:
//   - Creates default directories (recordings, backups, tls)
//   - Stores a first-boot timestamp in the config table
//   - Logs the first-boot event
//
// RSA key generation and encryption key derivation are handled by
// loadOrGenerateKeys which runs unconditionally after this.
func (n *NVR) bootstrapFirstRun() error {
	log.Printf("[NVR] first boot detected -- running initial setup")

	// Create standard data directories relative to the database location.
	dataRoot := filepath.Dir(n.DatabasePath)
	defaultDirs := []string{
		filepath.Join(dataRoot, "backups"),
		filepath.Join(dataRoot, "tls"),
	}
	// Also create recordings directory if configured.
	if n.RecordingsPath != "" {
		recPath := n.RecordingsPath
		if strings.HasPrefix(recPath, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				recPath = filepath.Join(home, recPath[2:])
			}
		}
		defaultDirs = append(defaultDirs, recPath)
	}

	for _, dir := range defaultDirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
		log.Printf("[NVR] created directory: %s", dir)
	}

	// Record the first-boot timestamp so subsequent starts know setup was done.
	now := time.Now().UTC().Format(time.RFC3339)
	if err := n.database.SetConfig("first_boot_at", now); err != nil {
		return fmt.Errorf("record first-boot timestamp: %w", err)
	}

	log.Printf("[NVR] first-boot setup complete -- setup wizard required")
	return nil
}

// DB returns the database handle.
func (n *NVR) DB() *db.DB {
	return n.database
}

// JWKSJSON returns the JWKS JSON document.
func (n *NVR) JWKSJSON() []byte {
	return n.jwksJSON
}

// PrivateKey returns the RSA private key.
func (n *NVR) PrivateKey() *rsa.PrivateKey {
	return n.privateKey
}

// EventBroadcaster returns the event broadcaster for publishing system events.
func (n *NVR) EventBroadcaster() *api.EventBroadcaster {
	return n.events
}

// RegisterRoutes registers NVR API routes on the given gin engine.
func (n *NVR) RegisterRoutes(engine *gin.Engine, version string) {
	recordingsPath := n.RecordingsPath
	if recordingsPath == "" {
		recordingsPath = "./recordings/"
	}

	credKey := crypto.DeriveKey(n.JWTSecret, "nvr-credential-encryption")

	if n.hlsHandler == nil {
		n.hlsHandler = &api.HLSHandler{
			DB:             n.database,
			RecordingsPath: recordingsPath,
		}
	}

	n.exportHandler = api.RegisterRoutes(engine, &api.RouterConfig{
		DB:             n.database,
		PrivateKey:     n.privateKey,
		JWKSJSON:       n.jwksJSON,
		YAMLWriter:     n.yamlWriter,
		Version:        version,
		Discovery:      n.discovery,
		APIAddress:     n.APIAddress,
		Scheduler:      n.sched,
		SetupChecker:   n,
		RecordingsPath: recordingsPath,
		Events:          n.events,
		CallbackManager: n.callbackMgr,
		EncryptionKey:   credKey,
		ConfigPath:      n.ConfigPath,
		Embedder:        n.aiEmbedder,
		AIRestarter:     n,
		HLSHandler:      n.hlsHandler,
		StorageManager:  n.storageMgr,
		Collector:       n.metricsCollector,
		BackchannelMgr:  n.backchannelMgr,
		ConnManager:     n.connMgr,
		EmailSender:     n.emailSender,
		BackupService:   n.backupSvc,
		SecurityConfig:  api.DefaultSecurityConfig(),
		UpdateManager:   updater.New(n.database, version),
		TLSManager:          n.tlsManager,
		DetectionEvaluator:  n.detectionEvaluator,
	})
}

// buildDBFragments converts scanned fragment info into DB fragment records.
func buildDBFragments(recordingID int64, fragments []api.FragmentInfo) []db.RecordingFragment {
	dbFrags := make([]db.RecordingFragment, len(fragments))
	var cumulativeMs float64
	for i, f := range fragments {
		dbFrags[i] = db.RecordingFragment{
			RecordingID:   recordingID,
			FragmentIndex: i,
			ByteOffset:    f.Offset,
			Size:          f.Size,
			DurationMs:    f.DurationMs,
			IsKeyframe:    true,
			TimestampMs:   int64(cumulativeMs),
		}
		cumulativeMs += f.DurationMs
	}
	return dbFrags
}

// indexRecordingFragments scans an fMP4 file and stores fragment metadata in the DB.
// It also extracts the NTP timestamp from the mtxi box to correct DB timestamps.
func (n *NVR) indexRecordingFragments(rec *db.Recording) {
	initSize, fragments, err := api.ScanFragments(rec.FilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "NVR: failed to scan fragments for %s: %v\n", rec.FilePath, err)
		return
	}

	if err := n.database.UpdateRecordingInitSize(rec.ID, initSize); err != nil {
		fmt.Fprintf(os.Stderr, "NVR: failed to update init_size for recording %d: %v\n", rec.ID, err)
	}

	dbFrags := buildDBFragments(rec.ID, fragments)

	if err := n.database.InsertFragments(rec.ID, dbFrags); err != nil {
		fmt.Fprintf(os.Stderr, "NVR: failed to insert fragments for recording %d: %v\n", rec.ID, err)
	}

	// Extract NTP timestamp from the mtxi box and correct DB timestamps.
	ntpTime, ntpErr := recordstore.ReadNTPFromFile(rec.FilePath)
	if ntpErr == nil && !ntpTime.IsZero() {
		mediaDuration := time.Duration(rec.DurationMs) * time.Millisecond
		mediaStart := ntpTime.UTC().Format("2006-01-02T15:04:05.000Z")
		mediaEnd := ntpTime.Add(mediaDuration).UTC().Format("2006-01-02T15:04:05.000Z")

		if err := n.database.UpdateMediaTimestamps(rec.ID, mediaStart, mediaStart, mediaEnd); err != nil {
			fmt.Fprintf(os.Stderr, "NVR: failed to update media timestamps for recording %d: %v\n", rec.ID, err)
		}
	}
}

// resolveCameraFromPath extracts the camera and optional stream prefix from a
// recording file path. Returns nil camera if no match is found.
func (n *NVR) resolveCameraFromPath(filePath string) (cam *db.Camera, streamPrefix string) {
	// Try to extract camera ID from path convention: .../nvr/<camera-id>/main/...
	// Non-default stream paths use: .../nvr/<camera-id>~<stream-prefix>/...
	if idx := strings.Index(filePath, "nvr/"); idx >= 0 {
		rest := filePath[idx+4:] // after "nvr/"
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) >= 1 {
			candidate := parts[0]
			// Capture ~streamID prefix if present (per-stream recording paths).
			if tildeIdx := strings.Index(candidate, "~"); tildeIdx > 0 {
				streamPrefix = candidate[tildeIdx+1:]
				candidate = candidate[:tildeIdx]
			}
			if c, err := n.database.GetCamera(candidate); err == nil {
				cam = c
				return
			}
		}
	}

	// Fallback: legacy substring match for pre-migration recordings.
	cameras, err := n.database.ListCameras()
	if err != nil {
		return
	}
	for _, c := range cameras {
		if c.MediaMTXPath != "" && strings.Contains(filePath, c.MediaMTXPath) {
			cam = c
			return
		}
	}
	return
}

// OnSegmentCreate is called when a new recording segment file is created on disk.
// It inserts an in-progress recording row so playback can discover it immediately.
func (n *NVR) OnSegmentCreate(filePath string) {
	cam, streamPrefix := n.resolveCameraFromPath(filePath)
	if cam == nil {
		return
	}

	format := "fmp4"
	if strings.HasSuffix(filePath, ".ts") {
		format = "mpegts"
	}

	now := time.Now().UTC()
	// Set end_time far ahead so the in-progress recording appears in time-range
	// queries. OnSegmentComplete will update it to the real end time.
	farFuture := now.Add(24 * time.Hour)
	rec := &db.Recording{
		CameraID:  cam.ID,
		StartTime: now.Format("2006-01-02T15:04:05.000Z"),
		EndTime:   farFuture.Format("2006-01-02T15:04:05.000Z"),
		FilePath:  filePath,
		Format:    format,
	}

	if streamPrefix != "" {
		if sid, err := n.database.ResolveStreamByPathPrefix(cam.ID, streamPrefix); err == nil {
			rec.StreamID = sid
		}
	}

	if err := n.database.InsertRecording(rec); err != nil {
		fmt.Fprintf(os.Stderr, "NVR: failed to insert in-progress recording for %s: %v\n", filePath, err)
	}
}

// OnSegmentComplete is called when a recording segment finishes writing.
// It matches recorder.OnSegmentCompleteFunc: func(path string, duration time.Duration).
func (n *NVR) OnSegmentComplete(filePath string, duration time.Duration) {
	cam, streamPrefix := n.resolveCameraFromPath(filePath)
	if cam == nil {
		return
	}

	var fileSize int64
	if info, err := os.Stat(filePath); err == nil {
		fileSize = info.Size()
	}

	format := "fmp4"
	if strings.HasSuffix(filePath, ".ts") {
		format = "mpegts"
	}

	now := time.Now().UTC()
	start := now.Add(-duration)

	// Check if this recording was already inserted by OnSegmentCreate.
	existing, _ := n.database.GetRecordingByFilePath(filePath)

	var rec *db.Recording
	if existing != nil {
		// Update the in-progress recording with final metadata.
		err := n.database.CompleteRecording(filePath,
			now.Format("2006-01-02T15:04:05.000Z"),
			duration.Milliseconds(),
			fileSize,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "NVR: failed to complete recording for %s: %v\n", filePath, err)
			return
		}
		existing.EndTime = now.Format("2006-01-02T15:04:05.000Z")
		existing.DurationMs = duration.Milliseconds()
		existing.FileSize = fileSize
		rec = existing
	} else {
		// Fallback: insert fresh (e.g. recordings created before this change).
		rec = &db.Recording{
			CameraID:   cam.ID,
			StartTime:  start.Format("2006-01-02T15:04:05.000Z"),
			EndTime:    now.Format("2006-01-02T15:04:05.000Z"),
			DurationMs: duration.Milliseconds(),
			FilePath:   filePath,
			FileSize:   fileSize,
			Format:     format,
		}

		if streamPrefix != "" {
			if sid, err := n.database.ResolveStreamByPathPrefix(cam.ID, streamPrefix); err == nil {
				rec.StreamID = sid
			}
		}

		var insertErr error
		for attempt := 0; attempt < 3; attempt++ {
			insertErr = n.database.InsertRecording(rec)
			if insertErr == nil {
				break
			}
			fmt.Fprintf(os.Stderr, "NVR: recording insert attempt %d/3 failed: %v\n", attempt+1, insertErr)
			if attempt < 2 {
				time.Sleep(1 * time.Second)
			}
		}
		if insertErr != nil {
			fmt.Fprintf(os.Stderr, "NVR: failed to insert recording after 3 attempts for %s: %v\n", filePath, insertErr)
			return
		}
	}

	// Notify the scheduler for health tracking.
	if n.sched != nil {
		n.sched.NotifySegmentForCamera(cam.ID)
	}

	detectAndInsertPendingSync(n.database, rec, cam)

	if n.hlsHandler != nil {
		dateStr := start.Format("2006-01-02")
		n.hlsHandler.InvalidateCache(cam.ID, dateStr)
	}

	// Index fragments for fMP4 files.
	if format == "fmp4" {
		go n.indexRecordingFragments(rec)
	}

	// Verify segment integrity inline (file is still in page cache).
	go func() {
		fragCount := 0
		if frags, err := n.database.GetFragments(rec.ID); err == nil {
			fragCount = len(frags)
		}

		info := integrity.RecordingInfo{
			FilePath:      rec.FilePath,
			FileSize:      rec.FileSize,
			InitSize:      rec.InitSize,
			FragmentCount: fragCount,
			DurationMs:    rec.DurationMs,
		}
		result := integrity.VerifySegment(info)

		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		var detail *string
		if result.Detail != "" {
			detail = &result.Detail
		}
		n.database.UpdateRecordingStatus(rec.ID, result.Status, detail, now)

		if result.Status == integrity.StatusCorrupted && n.events != nil {
			n.events.PublishSegmentCorrupted(cam.ID, rec.ID, rec.FilePath, result.Detail)
		}
	}()
}

// OnSegmentDelete is called when a recording segment is deleted by the cleaner.
func (n *NVR) OnSegmentDelete(filePath string) {
	n.database.DeleteRecordingByPath(filePath)
}

// detectAndInsertPendingSync checks if a recording landed in local fallback
// storage instead of the camera's configured storage path, and if so inserts
// a pending_syncs record.
func detectAndInsertPendingSync(database *db.DB, rec *db.Recording, cam *db.Camera) {
	if cam.StoragePath == "" {
		return // No custom storage, nothing to sync.
	}

	// If the file is already under the camera's storage path, no sync needed.
	if strings.HasPrefix(rec.FilePath, cam.StoragePath) {
		return
	}

	// Build target path by replacing the local prefix with the NAS path.
	// Local: ./recordings/nvr/<id>/main/2026-03/25/file.mp4
	// Target: /mnt/nas1/recordings/nvr/<id>/main/2026-03/25/file.mp4
	relPath := rec.FilePath
	if idx := strings.Index(relPath, "nvr/"); idx >= 0 {
		relPath = relPath[idx:]
	}
	targetPath := filepath.Join(cam.StoragePath, relPath)

	ps := &db.PendingSync{
		RecordingID: rec.ID,
		CameraID:    cam.ID,
		LocalPath:   rec.FilePath,
		TargetPath:  targetPath,
	}
	if err := database.InsertPendingSync(ps); err != nil {
		log.Printf("[NVR] [storage] failed to create pending sync for recording %d: %v", rec.ID, err)
	}
}

// loadOrGenerateKeys derives an encryption key, then loads or generates
// RSA keys from the database config table.
func (n *NVR) loadOrGenerateKeys() error {
	encKey := crypto.DeriveKey(n.JWTSecret, "nvr-rsa-key-encryption")

	encPrivB64, err := n.database.GetConfig("rsa_private_key")
	if errors.Is(err, db.ErrNotFound) {
		// Generate new RSA key pair.
		privPEM, pubPEM, err := crypto.GenerateRSAKeyPair()
		if err != nil {
			return fmt.Errorf("generate RSA key pair: %w", err)
		}

		// Encrypt private key and store as base64.
		encPriv, err := crypto.Encrypt(encKey, privPEM)
		if err != nil {
			return fmt.Errorf("encrypt private key: %w", err)
		}
		encPrivB64 = base64.StdEncoding.EncodeToString(encPriv)

		if err := n.database.SetConfig("rsa_private_key", encPrivB64); err != nil {
			return fmt.Errorf("store private key: %w", err)
		}
		if err := n.database.SetConfig("rsa_public_key", base64.StdEncoding.EncodeToString(pubPEM)); err != nil {
			return fmt.Errorf("store public key: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("load private key: %w", err)
	}

	// Decrypt private key.
	encPriv, err := base64.StdEncoding.DecodeString(encPrivB64)
	if err != nil {
		return fmt.Errorf("decode private key: %w", err)
	}
	privPEM, err := crypto.Decrypt(encKey, encPriv)
	if err != nil {
		return fmt.Errorf("decrypt private key: %w", err)
	}
	n.privateKey, err = crypto.ParseRSAPrivateKey(privPEM)
	if err != nil {
		return fmt.Errorf("parse private key: %w", err)
	}

	// Load public key and generate JWKS.
	pubB64, err := n.database.GetConfig("rsa_public_key")
	if err != nil {
		return fmt.Errorf("load public key: %w", err)
	}
	pubPEM, err := base64.StdEncoding.DecodeString(pubB64)
	if err != nil {
		return fmt.Errorf("decode public key: %w", err)
	}
	n.jwksJSON, err = crypto.JWKSFromPublicKey(pubPEM)
	if err != nil {
		return fmt.Errorf("generate JWKS: %w", err)
	}

	return nil
}

// runCertExpiryMonitor checks TLS certificate expiry every 12 hours and
// publishes warning events via the notification system when nearing expiry.
func (n *NVR) runCertExpiryMonitor() {
	if n.tlsManager == nil {
		return
	}

	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()

	check := func() {
		if !n.tlsManager.HasCertificate() {
			return
		}
		level, daysLeft, err := n.tlsManager.CheckExpiry()
		if err != nil {
			log.Printf("[NVR] [WARN] [tls] failed to check certificate expiry: %v", err)
			return
		}

		switch level {
		case "expired":
			log.Printf("[NVR] [ERROR] [tls] TLS certificate has expired")
			if n.events != nil {
				n.events.Publish(api.Event{
					Type:    "tls_cert_expired",
					Message: "TLS certificate has expired",
				})
			}
		case "critical":
			log.Printf("[NVR] [WARN] [tls] TLS certificate expires in %d days", daysLeft)
			if n.events != nil {
				n.events.Publish(api.Event{
					Type:    "tls_cert_expiring",
					Message: fmt.Sprintf("TLS certificate expires in %d days", daysLeft),
				})
			}
		case "warning":
			log.Printf("[NVR] [INFO] [tls] TLS certificate expires in %d days", daysLeft)
			if n.events != nil {
				n.events.Publish(api.Event{
					Type:    "tls_cert_expiring",
					Message: fmt.Sprintf("TLS certificate expires in %d days", daysLeft),
				})
			}
		}
	}

	// Check once at startup.
	check()

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}
