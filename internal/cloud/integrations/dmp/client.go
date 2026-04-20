package dmp

import (
	"context"
	"log"

	"github.com/bluenviron/mediamtx/internal/recorder/db"
)

// ClientConfig configures the DMP XR-Series integration.
type ClientConfig struct {
	// Enabled controls whether the integration is active.
	Enabled bool `json:"enabled"`

	// Receiver configures the SIA TCP receiver.
	Receiver ReceiverConfig `json:"receiver"`

	// ZoneMappings defines the initial zone-to-camera mappings.
	ZoneMappings []*ZoneMapping `json:"zone_mappings,omitempty"`
}

// DefaultClientConfig returns a default configuration.
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		Enabled:  false,
		Receiver: DefaultReceiverConfig(),
	}
}

// Client is the top-level DMP XR-Series integration. It manages the SIA
// receiver, zone mapper, and timeline integrator.
type Client struct {
	Config     ClientConfig
	DB         *db.DB
	ZoneMapper *ZoneMapper
	Timeline   *TimelineIntegrator
	Receiver   *Receiver

	ctx    context.Context
	cancel context.CancelFunc
}

// NewClient creates a new DMP integration client.
func NewClient(cfg ClientConfig, database *db.DB) *Client {
	zm := NewZoneMapper()
	if len(cfg.ZoneMappings) > 0 {
		zm.LoadMappings(cfg.ZoneMappings)
	}

	ti := &TimelineIntegrator{
		DB:         database,
		ZoneMapper: zm,
	}

	c := &Client{
		Config:     cfg,
		DB:         database,
		ZoneMapper: zm,
		Timeline:   ti,
	}

	c.Receiver = NewReceiver(cfg.Receiver, c.handleEvent)

	return c
}

// Start begins the DMP integration: starts the SIA receiver and begins
// processing alarm events.
func (c *Client) Start(ctx context.Context) error {
	if !c.Config.Enabled {
		log.Printf("[DMP] [INFO] integration disabled, skipping start")
		return nil
	}

	c.ctx, c.cancel = context.WithCancel(ctx)

	if err := c.Receiver.Start(c.ctx); err != nil {
		return err
	}

	log.Printf("[DMP] [INFO] XR-Series integration started (zones=%d)", c.ZoneMapper.Count())
	return nil
}

// Stop shuts down the DMP integration.
func (c *Client) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	if c.Receiver != nil {
		c.Receiver.Stop()
	}
	log.Printf("[DMP] [INFO] XR-Series integration stopped")
}

// handleEvent is called by the SIA receiver for each alarm event.
func (c *Client) handleEvent(event *AlarmEvent) {
	// Inject into the video timeline.
	c.Timeline.IngestAlarmEvent(event)
}

// AddZoneMapping adds a zone-to-camera mapping at runtime.
func (c *Client) AddZoneMapping(m *ZoneMapping) {
	c.ZoneMapper.Add(m)
	log.Printf("[DMP] [INFO] zone mapping added: account=%s zone=%d area=%d -> camera=%s",
		m.AccountID, m.Zone, m.Area, m.CameraID)
}

// RemoveZoneMapping removes a zone mapping at runtime.
func (c *Client) RemoveZoneMapping(accountID string, zone, area int) {
	c.ZoneMapper.Remove(accountID, zone, area)
	log.Printf("[DMP] [INFO] zone mapping removed: account=%s zone=%d area=%d",
		accountID, zone, area)
}

// ListZoneMappings returns all current zone mappings.
func (c *Client) ListZoneMappings() []*ZoneMapping {
	return c.ZoneMapper.List()
}
