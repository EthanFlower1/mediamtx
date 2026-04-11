package brivo

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Service is the top-level Brivo integration service. It orchestrates OAuth,
// event ingestion, camera correlation, and bidirectional event flow.
type Service struct {
	oauth      *OAuthService
	conns      ConnectionStore
	mappings   MappingStore
	events     EventLog
	snapshots  SnapshotService
	api        BrivoAPIClient
	auditHook  AuditHook
	clock      func() time.Time
	webhookKey []byte
}

// Config bundles the dependencies for Service.
type Config struct {
	OAuth      OAuthConfig
	Tokens     TokenStore
	Conns      ConnectionStore
	Mappings   MappingStore
	Events     EventLog
	Snapshots  SnapshotService
	API        BrivoAPIClient
	AuditHook  AuditHook
	Clock      func() time.Time
}

// NewService constructs a Brivo integration Service.
func NewService(cfg Config) (*Service, error) {
	if cfg.Tokens == nil {
		return nil, fmt.Errorf("brivo: token store is required")
	}
	if cfg.API == nil {
		return nil, fmt.Errorf("brivo: API client is required")
	}

	clock := cfg.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	auditHook := cfg.AuditHook
	if auditHook == nil {
		auditHook = func(context.Context, AuditEvent) {}
	}

	oauth := NewOAuthService(OAuthServiceConfig{
		OAuth:     cfg.OAuth,
		Tokens:    cfg.Tokens,
		API:       cfg.API,
		AuditHook: auditHook,
		Clock:     clock,
	})

	return &Service{
		oauth:      oauth,
		conns:      cfg.Conns,
		mappings:   cfg.Mappings,
		events:     cfg.Events,
		snapshots:  cfg.Snapshots,
		api:        cfg.API,
		auditHook:  auditHook,
		clock:      clock,
		webhookKey: []byte(cfg.OAuth.WebhookSecret),
	}, nil
}

// OAuth returns the OAuth sub-service for direct access to auth flows.
func (s *Service) OAuth() *OAuthService {
	return s.oauth
}

// ---------------------------------------------------------------------------
// Connection management
// ---------------------------------------------------------------------------

// Connect completes the OAuth flow, fetches the site list, and persists
// the connection record.
func (s *Service) Connect(ctx context.Context, tenantID, state, code, siteID, siteName string) error {
	if _, err := s.oauth.CompleteAuthorize(ctx, state, code); err != nil {
		return err
	}

	conn := Connection{
		ID:          fmt.Sprintf("brivo_%s", tenantID),
		TenantID:    tenantID,
		BrivoSiteID: siteID,
		SiteName:    siteName,
		Status:      ConnStatusActive,
		CreatedAt:   s.clock(),
		UpdatedAt:   s.clock(),
	}
	if s.conns != nil {
		if err := s.conns.Upsert(ctx, conn); err != nil {
			return fmt.Errorf("brivo: store connection: %w", err)
		}
	}
	return nil
}

// Disconnect removes both the token and connection record.
func (s *Service) Disconnect(ctx context.Context, tenantID string) error {
	if err := s.oauth.Disconnect(ctx, tenantID); err != nil {
		return err
	}
	if s.conns != nil {
		if err := s.conns.Delete(ctx, tenantID); err != nil {
			return fmt.Errorf("brivo: delete connection: %w", err)
		}
	}
	s.auditHook(ctx, AuditEvent{
		Action:   "disconnect",
		TenantID: tenantID,
		Detail:   "connection and tokens removed",
	})
	return nil
}

// GetConnection returns the current connection for a tenant.
func (s *Service) GetConnection(ctx context.Context, tenantID string) (*Connection, error) {
	if s.conns == nil {
		return nil, ErrNotConnected
	}
	return s.conns.Get(ctx, tenantID)
}

// ---------------------------------------------------------------------------
// Door-camera mapping
// ---------------------------------------------------------------------------

// SetDoorCameraMapping creates or updates a mapping between a Brivo door
// and a KaiVue camera.
func (s *Service) SetDoorCameraMapping(ctx context.Context, m DoorCameraMapping) error {
	if s.mappings == nil {
		return fmt.Errorf("brivo: mapping store not configured")
	}
	return s.mappings.Set(ctx, m)
}

// ListDoorCameraMappings returns all mappings for a tenant.
func (s *Service) ListDoorCameraMappings(ctx context.Context, tenantID string) ([]DoorCameraMapping, error) {
	if s.mappings == nil {
		return nil, nil
	}
	return s.mappings.ListByTenant(ctx, tenantID)
}

// DeleteDoorCameraMapping removes a mapping.
func (s *Service) DeleteDoorCameraMapping(ctx context.Context, tenantID, mappingID string) error {
	if s.mappings == nil {
		return fmt.Errorf("brivo: mapping store not configured")
	}
	return s.mappings.Delete(ctx, tenantID, mappingID)
}

// ---------------------------------------------------------------------------
// Event ingestion (Brivo -> KaiVue)
// ---------------------------------------------------------------------------

// VerifyWebhookSignature checks the HMAC-SHA256 signature on an inbound
// Brivo webhook payload.
func (s *Service) VerifyWebhookSignature(payload []byte, signature string) error {
	if len(s.webhookKey) == 0 {
		// No webhook secret configured; skip verification (dev mode).
		return nil
	}
	mac := hmac.New(sha256.New, s.webhookKey)
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return ErrWebhookSignatureInvalid
	}
	return nil
}

// HandleDoorEvent processes an inbound door event: logs it, correlates to
// cameras, captures snapshots, and updates the connection sync time.
func (s *Service) HandleDoorEvent(ctx context.Context, event DoorEvent) (*DoorEvent, error) {
	event.ReceivedAt = s.clock()

	// Look up camera mappings for this door.
	if s.mappings != nil {
		mappings, err := s.mappings.ListByDoor(ctx, event.TenantID, event.BrivoDoorID)
		if err != nil {
			return nil, fmt.Errorf("brivo: lookup door mappings: %w", err)
		}

		// Capture snapshots from each mapped camera.
		if s.snapshots != nil {
			for _, m := range mappings {
				snapURL, err := s.snapshots.Capture(ctx, event.TenantID, m.CameraID, event.OccurredAt)
				if err != nil {
					// Log but don't fail the entire event on snapshot error.
					continue
				}
				event.SnapshotURLs = append(event.SnapshotURLs, snapURL)
			}
		}
	}

	// Persist the event.
	if s.events != nil {
		if err := s.events.Append(ctx, event); err != nil {
			return nil, fmt.Errorf("brivo: append event: %w", err)
		}
	}

	// Update last sync time.
	if s.conns != nil {
		now := s.clock()
		_ = s.conns.UpdateSyncTime(ctx, event.TenantID, now)
	}

	s.auditHook(ctx, AuditEvent{
		Action:   "event_received",
		TenantID: event.TenantID,
		Detail:   fmt.Sprintf("door=%s type=%s", event.BrivoDoorID, event.EventType),
	})

	return &event, nil
}

// ListEvents returns recent door events for a tenant.
func (s *Service) ListEvents(ctx context.Context, tenantID string, from, to time.Time, limit int) ([]DoorEvent, error) {
	if s.events == nil {
		return nil, nil
	}
	return s.events.ListByTenant(ctx, tenantID, from, to, limit)
}

// ---------------------------------------------------------------------------
// Bidirectional event flow (KaiVue -> Brivo)
// ---------------------------------------------------------------------------

// SendNVREvent pushes an NVR event to the Brivo API. For example, a motion
// alert near a controlled door can trigger a lockdown command.
func (s *Service) SendNVREvent(ctx context.Context, event NVREvent) error {
	accessToken, err := s.oauth.EnsureValidToken(ctx, event.TenantID)
	if err != nil {
		return fmt.Errorf("brivo: get token for send: %w", err)
	}

	if err := s.api.SendEvent(ctx, accessToken, event); err != nil {
		return fmt.Errorf("brivo: send event to brivo: %w", err)
	}

	s.auditHook(ctx, AuditEvent{
		Action:   "event_sent",
		TenantID: event.TenantID,
		Detail:   fmt.Sprintf("camera=%s door=%s action=%s", event.CameraID, event.DoorID, event.Action),
	})

	return nil
}

// ---------------------------------------------------------------------------
// Config UI helpers
// ---------------------------------------------------------------------------

// ListSites returns the Brivo sites visible to the authenticated tenant.
// Used by the config UI to populate the site selector.
func (s *Service) ListSites(ctx context.Context, tenantID string) ([]BrivoSite, error) {
	accessToken, err := s.oauth.EnsureValidToken(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return s.api.ListSites(ctx, accessToken)
}

// ListDoors returns the doors for a Brivo site. Used by the config UI to
// populate the door-camera mapping form.
func (s *Service) ListDoors(ctx context.Context, tenantID, siteID string) ([]BrivoDoor, error) {
	accessToken, err := s.oauth.EnsureValidToken(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return s.api.ListDoors(ctx, accessToken, siteID)
}

// TestConnection verifies that the stored credentials are valid by making
// a lightweight API call to Brivo. Returns nil if the connection is healthy.
func (s *Service) TestConnection(ctx context.Context, tenantID string) error {
	accessToken, err := s.oauth.EnsureValidToken(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("brivo: test connection: %w", err)
	}

	// A successful ListSites call confirms the token works.
	_, err = s.api.ListSites(ctx, accessToken)
	if err != nil {
		return fmt.Errorf("brivo: test connection: API call failed: %w", err)
	}

	s.auditHook(ctx, AuditEvent{
		Action:   "test",
		TenantID: tenantID,
		Detail:   "connection test passed",
	})

	return nil
}
