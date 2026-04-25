package connect

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/directory/cloudconnector"
	"github.com/gorilla/websocket"
)

// BrokerConfig configures the WebSocket broker that accepts connections
// from on-prem Directory instances.
type BrokerConfig struct {
	Registry     *Registry
	Authenticate func(token string) (tenantID string, ok bool)
	RelayURL     string // e.g. "wss://relay.raikada.com"
	Logger       *slog.Logger
}

// Broker implements http.Handler to accept and manage Directory WebSocket
// connections from on-prem sites.
type Broker struct {
	cfg      BrokerConfig
	upgrader websocket.Upgrader
}

// NewBroker creates a new Broker with the given configuration.
func NewBroker(cfg BrokerConfig) *Broker {
	return &Broker{
		cfg: cfg,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// ServeHTTP handles incoming WebSocket connections from Directory instances.
func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Extract bearer token.
	tenantID, ok := b.authenticateRequest(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. Upgrade to WebSocket.
	conn, err := b.upgrader.Upgrade(w, r, nil)
	if err != nil {
		b.cfg.Logger.Error("websocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	// 3. Read register message (10s timeout).
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	var env cloudconnector.Envelope
	if err := conn.ReadJSON(&env); err != nil {
		b.cfg.Logger.Error("read register message failed", "error", err)
		return
	}

	if env.Type != cloudconnector.MsgTypeRegister || env.Register == nil {
		b.cfg.Logger.Error("expected register message", "type", env.Type)
		return
	}

	reg := env.Register
	siteID := reg.SiteID

	// 4. Add session to Registry with the WSS connection for tunneling.
	siteConn := NewSiteConn(conn)
	caps := map[string]bool{
		"streams":  reg.Capabilities.Streams,
		"playback": reg.Capabilities.Playback,
		"ai":       reg.Capabilities.AI,
	}

	b.cfg.Registry.Add(Session{
		SiteID:       siteID,
		TenantID:     tenantID,
		SiteAlias:    reg.SiteAlias,
		PublicIP:     reg.PublicIP,
		LANCIDRs:     reg.LANCIDRs,
		Capabilities: caps,
		Status:       StatusOnline,
		Conn:         siteConn,
	})

	// 5. On disconnect, remove from registry.
	defer b.cfg.Registry.Remove(siteID)

	b.cfg.Logger.Info("directory connected",
		"site_id", siteID,
		"tenant_id", tenantID,
		"alias", reg.SiteAlias,
	)

	// 6. Send RegisteredPayload ack with RelayURL.
	ack := cloudconnector.Envelope{
		Type: cloudconnector.MsgTypeRegistered,
		Registered: &cloudconnector.RegisteredPayload{
			OK:       true,
			RelayURL: b.cfg.RelayURL,
		},
	}
	if err := conn.WriteJSON(ack); err != nil {
		b.cfg.Logger.Error("write ack failed", "error", err, "site_id", siteID)
		return
	}

	// 7. Read loop: process heartbeats and events.
	conn.SetReadDeadline(time.Time{}) // clear deadline for ongoing reads
	for {
		var msg cloudconnector.Envelope
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseNormalClosure,
				websocket.CloseNoStatusReceived,
			) {
				b.cfg.Logger.Error("read error", "error", err, "site_id", siteID)
			}
			return
		}

		switch msg.Type {
		case cloudconnector.MsgTypeHeartbeat:
			if msg.Heartbeat != nil {
				b.cfg.Registry.UpdateHeartbeat(siteID, HeartbeatUpdate{
					CameraCount:   msg.Heartbeat.CameraCount,
					RecorderCount: msg.Heartbeat.RecorderCount,
					DiskUsedPct:   msg.Heartbeat.DiskUsedPct,
					PublicIP:      msg.Heartbeat.PublicIP,
				})
				b.cfg.Logger.Debug("heartbeat received", "site_id", siteID)
			}

		case cloudconnector.MsgTypeEvent:
			if msg.Event != nil {
				b.cfg.Logger.Info("event received",
					"site_id", siteID,
					"kind", msg.Event.Kind,
					"camera_id", msg.Event.CameraID,
				)
			}

		case cloudconnector.MsgTypeCommandResponse:
			if msg.CommandResponse != nil {
				siteConn.DeliverResponse(*msg.CommandResponse)
			}

		default:
			b.cfg.Logger.Warn("unknown message type", "type", msg.Type, "site_id", siteID)
		}
	}
}

// authenticateRequest extracts the bearer token and validates it.
func (b *Broker) authenticateRequest(r *http.Request) (string, bool) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "", false
	}

	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return "", false
	}

	token := strings.TrimPrefix(auth, prefix)
	return b.cfg.Authenticate(token)
}
