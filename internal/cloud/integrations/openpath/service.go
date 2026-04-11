package openpath

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// -----------------------------------------------------------------------
// Store interface
// -----------------------------------------------------------------------

// EventStore persists door events and correlation records. The production
// implementation backs onto SQLite; tests use the in-memory fake below.
type EventStore interface {
	// SaveDoorEvent persists a normalised door event. Duplicate IDs
	// (idempotency key) must be silently ignored (upsert semantics).
	SaveDoorEvent(ctx context.Context, ev DoorEvent) error

	// SaveCorrelatedClip persists a door-event → video correlation record.
	SaveCorrelatedClip(ctx context.Context, clip CorrelatedClip) error

	// ListDoorEvents returns door events for a tenant within a time range,
	// ordered by timestamp descending.
	ListDoorEvents(ctx context.Context, tenantID string, from, to time.Time) ([]DoorEvent, error)
}

// -----------------------------------------------------------------------
// Service
// -----------------------------------------------------------------------

// Service orchestrates the OpenPath / Alta integration lifecycle:
//   - Accepts inbound webhook events and normalises them
//   - Correlates door events with NVR camera recordings
//   - Exposes bidirectional actions (lockdown trigger)
//   - Manages per-tenant configuration
type Service struct {
	mu      sync.RWMutex
	configs map[string]*tenantState // key: tenantID

	store  EventStore
	log    *slog.Logger

	// correlationWindow is the time window around a door event that is
	// searched for matching video segments.
	correlationWindow time.Duration
}

// tenantState holds the live runtime state for one tenant's integration.
type tenantState struct {
	cfg    Config
	client *Client
}

// ServiceConfig configures the Service.
type ServiceConfig struct {
	Store             EventStore
	Logger            *slog.Logger
	CorrelationWindow time.Duration // default: 30s before + 10s after
}

// NewService constructs a ready Service.
func NewService(sc ServiceConfig) (*Service, error) {
	if sc.Store == nil {
		return nil, errors.New("openpath: EventStore is required")
	}
	if sc.Logger == nil {
		sc.Logger = slog.Default()
	}
	cw := sc.CorrelationWindow
	if cw == 0 {
		cw = 40 * time.Second // 30s before + 10s after the event
	}
	return &Service{
		configs:           make(map[string]*tenantState),
		store:             sc.Store,
		log:               sc.Logger.With(slog.String("component", "openpath.service")),
		correlationWindow: cw,
	}, nil
}

// -----------------------------------------------------------------------
// Tenant lifecycle
// -----------------------------------------------------------------------

// Register adds or replaces a tenant's Alta integration config. It also
// verifies connectivity by performing an OAuth2 token exchange.
func (s *Service) Register(ctx context.Context, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	client := NewClient(cfg, nil, s.log)

	// Verify connectivity — fail fast if credentials are wrong.
	if _, err := client.Authenticate(ctx); err != nil {
		return fmt.Errorf("openpath: connectivity check failed: %w", err)
	}

	s.mu.Lock()
	s.configs[cfg.TenantID] = &tenantState{cfg: cfg, client: client}
	s.mu.Unlock()

	s.log.InfoContext(ctx, "tenant registered",
		"tenant_id", cfg.TenantID,
		"org_id", cfg.OrgID,
		"doors_mapped", len(cfg.DoorCameraMappings),
	)
	return nil
}

// RegisterWithClient is like Register but accepts a pre-built Client. This
// is used by tests to inject a mock HTTP transport.
func (s *Service) RegisterWithClient(cfg Config, client *Client) {
	s.mu.Lock()
	s.configs[cfg.TenantID] = &tenantState{cfg: cfg, client: client}
	s.mu.Unlock()
}

// Unregister removes a tenant's integration.
func (s *Service) Unregister(tenantID string) {
	s.mu.Lock()
	delete(s.configs, tenantID)
	s.mu.Unlock()
}

// tenant returns the live state for a tenant, or an error if not registered.
func (s *Service) tenant(tenantID string) (*tenantState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ts, ok := s.configs[tenantID]
	if !ok {
		return nil, fmt.Errorf("openpath: tenant %q not registered", tenantID)
	}
	return ts, nil
}

// -----------------------------------------------------------------------
// Webhook handler
// -----------------------------------------------------------------------

// HandleWebhook is the http.HandlerFunc for Alta webhook deliveries.
//
// Path: POST /api/v1/integrations/openpath/webhook/{tenant_id}
//
// The handler:
//  1. Verifies the HMAC-SHA256 signature (X-OpenPath-Signature header)
//  2. Parses the event payload
//  3. Persists the normalised event
//  4. Correlates with camera recordings
func (s *Service) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")
	if tenantID == "" {
		// Fallback for routers that use a different path-param mechanism.
		tenantID = r.URL.Query().Get("tenant_id")
	}
	if tenantID == "" {
		http.Error(w, `{"error":"missing tenant_id"}`, http.StatusBadRequest)
		return
	}

	ts, err := s.tenant(tenantID)
	if err != nil {
		http.Error(w, `{"error":"unknown tenant"}`, http.StatusNotFound)
		return
	}

	// Read body.
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MiB max
	if err != nil {
		http.Error(w, `{"error":"read body failed"}`, http.StatusBadRequest)
		return
	}

	// Verify HMAC signature.
	sig := r.Header.Get("X-OpenPath-Signature")
	if !s.verifySignature(body, sig, ts.cfg.WebhookSecret) {
		s.log.WarnContext(r.Context(), "webhook signature verification failed",
			"tenant_id", tenantID,
		)
		http.Error(w, `{"error":"invalid signature"}`, http.StatusUnauthorized)
		return
	}

	// Parse event.
	ev, err := s.parseWebhookPayload(tenantID, body)
	if err != nil {
		s.log.WarnContext(r.Context(), "webhook parse failed",
			"tenant_id", tenantID,
			"error", err,
		)
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	// Persist event.
	if err := s.store.SaveDoorEvent(r.Context(), ev); err != nil {
		s.log.ErrorContext(r.Context(), "failed to save door event",
			"tenant_id", tenantID,
			"event_id", ev.ID,
			"error", err,
		)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	// Correlate with cameras (best-effort, non-blocking).
	go s.correlateDoorEvent(context.Background(), tenantID, ev)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))
}

// verifySignature checks the HMAC-SHA256 signature of the webhook payload.
func (s *Service) verifySignature(body []byte, signature, secret string) bool {
	if signature == "" || secret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// parseWebhookPayload converts the raw Alta webhook JSON into a DoorEvent.
func (s *Service) parseWebhookPayload(tenantID string, body []byte) (DoorEvent, error) {
	var raw struct {
		EventID   string `json:"event_id"`
		OrgID     string `json:"org_id"`
		DoorID    string `json:"door_id"`
		DoorName  string `json:"door_name"`
		EventType string `json:"event_type"`
		UserName  string `json:"user_name"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return DoorEvent{}, fmt.Errorf("unmarshal webhook: %w", err)
	}
	if raw.EventID == "" || raw.DoorID == "" {
		return DoorEvent{}, errors.New("missing required fields: event_id, door_id")
	}

	ts, err := time.Parse(time.RFC3339, raw.Timestamp)
	if err != nil {
		ts = time.Now().UTC()
	}

	return DoorEvent{
		ID:        raw.EventID,
		TenantID:  tenantID,
		OrgID:     raw.OrgID,
		DoorID:    raw.DoorID,
		DoorName:  raw.DoorName,
		Type:      toDoorEventType(raw.EventType),
		UserName:  raw.UserName,
		Timestamp: ts,
		Raw:       body,
	}, nil
}

func toDoorEventType(s string) DoorEventType {
	switch s {
	case "unlock", "granted":
		return DoorEventUnlock
	case "denied":
		return DoorEventDenied
	case "forced_open":
		return DoorEventForcedOpen
	case "held_open":
		return DoorEventHeldOpen
	case "lockdown":
		return DoorEventLockdown
	default:
		return DoorEventType(s) // pass through unknown types
	}
}

// -----------------------------------------------------------------------
// Correlation
// -----------------------------------------------------------------------

// correlateDoorEvent looks up the door-camera mappings and creates
// CorrelatedClip records spanning the correlation window around the event.
func (s *Service) correlateDoorEvent(ctx context.Context, tenantID string, ev DoorEvent) {
	ts, err := s.tenant(tenantID)
	if err != nil {
		return
	}

	cameras := s.camerasForDoor(ts.cfg.DoorCameraMappings, ev.DoorID)
	if len(cameras) == 0 {
		s.log.DebugContext(ctx, "no camera mapping for door",
			"tenant_id", tenantID,
			"door_id", ev.DoorID,
		)
		return
	}

	// Correlation window: 30s before the event to 10s after.
	start := ev.Timestamp.Add(-30 * time.Second)
	end := ev.Timestamp.Add(10 * time.Second)

	for _, camPath := range cameras {
		clip := CorrelatedClip{
			DoorEventID: ev.ID,
			CameraPath:  camPath,
			StartTime:   start,
			EndTime:     end,
		}
		if err := s.store.SaveCorrelatedClip(ctx, clip); err != nil {
			s.log.ErrorContext(ctx, "failed to save correlated clip",
				"event_id", ev.ID,
				"camera", camPath,
				"error", err,
			)
		}
	}

	s.log.InfoContext(ctx, "door event correlated",
		"event_id", ev.ID,
		"door_id", ev.DoorID,
		"cameras", len(cameras),
	)
}

// camerasForDoor returns all camera paths mapped to the given door ID.
func (s *Service) camerasForDoor(mappings []DoorCameraMapping, doorID string) []string {
	for _, m := range mappings {
		if m.DoorID == doorID {
			return m.CameraPaths
		}
	}
	return nil
}

// -----------------------------------------------------------------------
// Bidirectional: lockdown
// -----------------------------------------------------------------------

// TriggerLockdown sends a lockdown command to an Alta door. This is the
// NVR → Alta direction: triggered when the NVR detects a security event.
func (s *Service) TriggerLockdown(ctx context.Context, req LockdownRequest) error {
	if req.TenantID == "" || req.DoorID == "" {
		return errors.New("openpath: tenant_id and door_id are required")
	}
	ts, err := s.tenant(req.TenantID)
	if err != nil {
		return err
	}
	req.OrgID = ts.cfg.OrgID
	return ts.client.TriggerLockdown(ctx, req)
}

// -----------------------------------------------------------------------
// Query
// -----------------------------------------------------------------------

// ListDoorEvents returns door events for a tenant within a time range.
func (s *Service) ListDoorEvents(ctx context.Context, tenantID string, from, to time.Time) ([]DoorEvent, error) {
	return s.store.ListDoorEvents(ctx, tenantID, from, to)
}

// ListDoors retrieves the door inventory from Alta for a tenant.
func (s *Service) ListDoors(ctx context.Context, tenantID string) ([]AltaDoor, error) {
	ts, err := s.tenant(tenantID)
	if err != nil {
		return nil, err
	}
	return ts.client.ListDoors(ctx)
}

// -----------------------------------------------------------------------
// Door-camera mapping management
// -----------------------------------------------------------------------

// SetDoorCameraMapping creates or updates a door-camera mapping for a tenant.
// If the door already has a mapping it is replaced; otherwise a new one is appended.
func (s *Service) SetDoorCameraMapping(tenantID string, m DoorCameraMapping) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ts, ok := s.configs[tenantID]
	if !ok {
		return fmt.Errorf("openpath: tenant %q not registered", tenantID)
	}

	for i, existing := range ts.cfg.DoorCameraMappings {
		if existing.DoorID == m.DoorID {
			ts.cfg.DoorCameraMappings[i] = m
			return nil
		}
	}
	ts.cfg.DoorCameraMappings = append(ts.cfg.DoorCameraMappings, m)
	return nil
}

// DeleteDoorCameraMapping removes a door-camera mapping by door ID.
func (s *Service) DeleteDoorCameraMapping(tenantID, doorID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ts, ok := s.configs[tenantID]
	if !ok {
		return fmt.Errorf("openpath: tenant %q not registered", tenantID)
	}

	for i, existing := range ts.cfg.DoorCameraMappings {
		if existing.DoorID == doorID {
			ts.cfg.DoorCameraMappings = append(
				ts.cfg.DoorCameraMappings[:i],
				ts.cfg.DoorCameraMappings[i+1:]...,
			)
			return nil
		}
	}
	return nil // not found is a no-op
}
