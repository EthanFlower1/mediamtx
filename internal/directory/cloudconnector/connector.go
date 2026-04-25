package cloudconnector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// SiteInfo describes the on-prem site that the connector registers with the
// cloud broker.
type SiteInfo struct {
	ID           string
	Alias        string
	Version      string
	LANCIDRs     []string
	Capabilities Capabilities
}

// Config configures a Connector.
type Config struct {
	URL               string                                                       // WSS endpoint
	Token             string                                                       // bearer token
	Site              SiteInfo                                                     // site identity
	HeartbeatInterval time.Duration                                                // default 30s
	MinReconnectDelay time.Duration                                                // default 1s
	MaxReconnectDelay time.Duration                                                // default 60s
	SiteInfoFunc      func() HeartbeatPayload                                     // optional, called before each heartbeat
	CommandHandler    func(ctx context.Context, cmd CommandPayload) (json.RawMessage, error) // optional
	Logger            *slog.Logger
}

// Connector maintains a persistent outbound WebSocket connection to the cloud
// broker. It handles registration, periodic heartbeats, event forwarding, and
// incoming command dispatch.
type Connector struct {
	cfg      Config
	mu       sync.Mutex
	conn     *websocket.Conn
	relayURL string
}

// New creates a Connector with sensible defaults applied to cfg.
func New(cfg Config) *Connector {
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = 30 * time.Second
	}
	if cfg.MinReconnectDelay == 0 {
		cfg.MinReconnectDelay = 1 * time.Second
	}
	if cfg.MaxReconnectDelay == 0 {
		cfg.MaxReconnectDelay = 60 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Connector{cfg: cfg}
}

// RelayURL returns the relay URL received from the cloud during registration.
func (c *Connector) RelayURL() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.relayURL
}

// SendEvent sends an event envelope to the cloud. It is safe for concurrent
// use.
func (c *Connector) SendEvent(event EventPayload) error {
	c.mu.Lock()
	ws := c.conn
	c.mu.Unlock()

	if ws == nil {
		return fmt.Errorf("cloudconnector: not connected")
	}

	env := Envelope{
		Type:  MsgTypeEvent,
		Event: &event,
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(env)
}

// Run connects to the cloud broker and blocks until ctx is cancelled. It
// automatically reconnects with exponential backoff on connection loss.
func (c *Connector) Run(ctx context.Context) {
	var attempt int
	for {
		if ctx.Err() != nil {
			return
		}

		err := c.connectAndServe(ctx)
		if ctx.Err() != nil {
			return
		}

		delay := c.backoff(attempt)
		c.cfg.Logger.Warn("cloud connection lost, reconnecting",
			"error", err, "attempt", attempt, "delay", delay)
		attempt++

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

// connectAndServe dials the cloud, registers, then runs heartbeat and read
// loops until the connection is lost or ctx is cancelled.
func (c *Connector) connectAndServe(ctx context.Context) error {
	header := http.Header{}
	if c.cfg.Token != "" {
		header.Set("Authorization", "Bearer "+c.cfg.Token)
	}

	ws, _, err := websocket.DefaultDialer.DialContext(ctx, c.cfg.URL, header)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	c.mu.Lock()
	c.conn = ws
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
		ws.Close()
	}()

	// Send registration.
	reg := Envelope{
		Type: MsgTypeRegister,
		Register: &RegisterPayload{
			SiteID:       c.cfg.Site.ID,
			SiteAlias:    c.cfg.Site.Alias,
			Version:      c.cfg.Site.Version,
			LANCIDRs:     c.cfg.Site.LANCIDRs,
			Capabilities: c.cfg.Site.Capabilities,
		},
	}
	if err := ws.WriteJSON(reg); err != nil {
		return fmt.Errorf("send register: %w", err)
	}

	// Wait for ack with timeout.
	ws.SetReadDeadline(time.Now().Add(10 * time.Second)) //nolint:errcheck
	var ack Envelope
	if err := ws.ReadJSON(&ack); err != nil {
		return fmt.Errorf("read register ack: %w", err)
	}
	ws.SetReadDeadline(time.Time{}) //nolint:errcheck

	if ack.Type != MsgTypeRegistered || ack.Registered == nil {
		return fmt.Errorf("unexpected ack type: %s", ack.Type)
	}
	if !ack.Registered.OK {
		return fmt.Errorf("registration rejected: %s", ack.Registered.Error)
	}

	c.mu.Lock()
	c.relayURL = ack.Registered.RelayURL
	c.mu.Unlock()

	c.cfg.Logger.Info("registered with cloud",
		"relay_url", ack.Registered.RelayURL)

	// Run heartbeat and read loops concurrently; first error cancels both.
	loopCtx, loopCancel := context.WithCancel(ctx)
	defer loopCancel()

	errCh := make(chan error, 2)

	go func() { errCh <- c.heartbeatLoop(loopCtx, ws) }()
	go func() { errCh <- c.readLoop(loopCtx, ws) }()

	// Wait for the first error (or parent context cancellation).
	select {
	case err := <-errCh:
		loopCancel()
		// Drain the second goroutine.
		<-errCh
		if ctx.Err() != nil {
			// Graceful shutdown.
			ws.WriteMessage( //nolint:errcheck
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseGoingAway, "shutting down"),
			)
		}
		return err
	case <-ctx.Done():
		loopCancel()
		ws.WriteMessage( //nolint:errcheck
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "shutting down"),
		)
		// Drain goroutines.
		<-errCh
		<-errCh
		return ctx.Err()
	}
}

// heartbeatLoop sends periodic heartbeats until ctx is cancelled or an error
// occurs.
func (c *Connector) heartbeatLoop(ctx context.Context, ws *websocket.Conn) error {
	ticker := time.NewTicker(c.cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := c.sendHeartbeat(ws); err != nil {
				return fmt.Errorf("heartbeat: %w", err)
			}
		}
	}
}

// readLoop reads incoming messages (commands) from the cloud.
func (c *Connector) readLoop(ctx context.Context, ws *websocket.Conn) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		var env Envelope
		if err := ws.ReadJSON(&env); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("read: %w", err)
		}

		if env.Type == MsgTypeCommand && env.Command != nil {
			go c.handleCommand(ctx, ws, *env.Command)
		}
	}
}

// handleCommand processes an incoming command and sends the response.
func (c *Connector) handleCommand(ctx context.Context, ws *websocket.Conn, cmd CommandPayload) {
	var resp CommandResponsePayload
	resp.ID = cmd.ID

	if c.cfg.CommandHandler != nil {
		data, err := c.cfg.CommandHandler(ctx, cmd)
		if err != nil {
			resp.Success = false
			resp.Error = err.Error()
		} else {
			resp.Success = true
			resp.Data = data
		}
	} else {
		resp.Success = false
		resp.Error = "no command handler configured"
	}

	env := Envelope{
		Type:            MsgTypeCommandResponse,
		CommandResponse: &resp,
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		if err := c.conn.WriteJSON(env); err != nil {
			c.cfg.Logger.Error("failed to send command response",
				"command_id", cmd.ID, "error", err)
		}
	}
}

// sendHeartbeat sends a single heartbeat message.
func (c *Connector) sendHeartbeat(ws *websocket.Conn) error {
	var hb HeartbeatPayload
	if c.cfg.SiteInfoFunc != nil {
		hb = c.cfg.SiteInfoFunc()
	} else {
		hb = HeartbeatPayload{
			SiteID:    c.cfg.Site.ID,
			Timestamp: time.Now(),
		}
	}

	env := Envelope{
		Type:      MsgTypeHeartbeat,
		Heartbeat: &hb,
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return ws.WriteJSON(env)
}

// backoff returns the delay before the next reconnection attempt using
// exponential backoff with 25% jitter.
func (c *Connector) backoff(attempt int) time.Duration {
	base := float64(c.cfg.MinReconnectDelay) * math.Pow(2, float64(attempt))
	if base > float64(c.cfg.MaxReconnectDelay) {
		base = float64(c.cfg.MaxReconnectDelay)
	}
	// Add 25% jitter.
	jitter := base * 0.25 * rand.Float64() //nolint:gosec
	return time.Duration(base + jitter)
}
