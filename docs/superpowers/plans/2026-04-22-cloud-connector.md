# Cloud Connector (QuickConnect-Style Remote Access) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable seamless remote access to on-prem NVR recordings and live streams via an outbound-only WebSocket connection to the cloud, with automatic NAT traversal and relay fallback — modeled after Synology QuickConnect.

**Architecture:** The on-prem Directory maintains an outbound WSS control channel to the cloud. The cloud acts as a connection broker and relay — never the source of truth. Two modes: air-gapped (default, no outbound connections) and cloud-connected (WSS control + HTTPS data plane, all cloud features). The Flutter client resolves a site alias via the cloud broker, then tries LAN direct → STUN direct → cloud relay in order.

**Tech Stack:** Go 1.22+, `gorilla/websocket` (already in go.mod), SQLite on-prem, PostgreSQL cloud, `http.NewServeMux()` routing, NDJSON wire format

---

## File Structure

### On-Prem (Directory side)

| File | Responsibility |
|------|----------------|
| `internal/directory/cloudconnector/connector.go` | WSS client lifecycle — connect, register, heartbeat, reconnect |
| `internal/directory/cloudconnector/connector_test.go` | Unit tests for connector |
| `internal/directory/cloudconnector/messages.go` | Wire message types (register, heartbeat, command, event) |
| `internal/directory/cloudconnector/messages_test.go` | Message serialization tests |
| `internal/directory/cloudconnector/eventforwarder.go` | Subscribes to local events, forwards via WSS |
| `internal/directory/cloudconnector/eventforwarder_test.go` | Event forwarder tests |
| `internal/conf/conf.go` | Add `CloudConnect*` config fields |

### Cloud Side

| File | Responsibility |
|------|----------------|
| `internal/cloud/connect/broker.go` | Accept Directory WSS connections, manage sessions |
| `internal/cloud/connect/broker_test.go` | Broker unit tests |
| `internal/cloud/connect/registry.go` | Site alias → active session lookup |
| `internal/cloud/connect/registry_test.go` | Registry unit tests |
| `internal/cloud/connect/resolve.go` | Client-facing endpoint: resolve site alias → connection plan |
| `internal/cloud/connect/resolve_test.go` | Resolve endpoint tests |
| `internal/cloud/relay/relay.go` | Bidirectional WebSocket proxy between client and Directory |
| `internal/cloud/relay/relay_test.go` | Relay unit tests |
| `internal/cloud/relay/session.go` | Relay session state and lifecycle |
| `internal/cloud/relay/session_test.go` | Session tests |
| `internal/cloud/db/migrations/0035_cloud_connect.up.sql` | Site aliases + relay sessions tables |
| `internal/cloud/db/migrations/0035_cloud_connect.down.sql` | Rollback migration |

---

## Phase 1: Wire Protocol & Message Types

### Task 1: Define wire message types

**Files:**
- Create: `internal/directory/cloudconnector/messages.go`
- Create: `internal/directory/cloudconnector/messages_test.go`

These are the JSON messages exchanged over the WSS control channel between the on-prem Directory and the cloud broker.

- [ ] **Step 1: Write the failing test**

```go
// internal/directory/cloudconnector/messages_test.go
package cloudconnector

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMarshalRegisterMessage(t *testing.T) {
	msg := Envelope{
		Type: MsgTypeRegister,
		Register: &RegisterPayload{
			SiteID:    "my-home",
			SiteAlias: "my-home",
			Version:   "1.0.0",
			PublicIP:  "73.42.100.5",
			LANCIDRs:  []string{"192.168.1.0/24"},
			Capabilities: Capabilities{
				Streams:  true,
				Playback: true,
				AI:       true,
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Type != MsgTypeRegister {
		t.Fatalf("type = %q, want %q", decoded.Type, MsgTypeRegister)
	}
	if decoded.Register.SiteID != "my-home" {
		t.Fatalf("site_id = %q, want %q", decoded.Register.SiteID, "my-home")
	}
	if !decoded.Register.Capabilities.AI {
		t.Fatal("capabilities.ai should be true")
	}
}

func TestMarshalHeartbeatMessage(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	msg := Envelope{
		Type: MsgTypeHeartbeat,
		Heartbeat: &HeartbeatPayload{
			SiteID:        "my-home",
			Timestamp:     now,
			UptimeSec:     3600,
			CameraCount:   4,
			RecorderCount: 1,
			DiskUsedPct:   42.5,
			PublicIP:      "73.42.100.5",
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Heartbeat.CameraCount != 4 {
		t.Fatalf("camera_count = %d, want 4", decoded.Heartbeat.CameraCount)
	}
	if decoded.Heartbeat.DiskUsedPct != 42.5 {
		t.Fatalf("disk_used_pct = %f, want 42.5", decoded.Heartbeat.DiskUsedPct)
	}
}

func TestMarshalEventMessage(t *testing.T) {
	msg := Envelope{
		Type: MsgTypeEvent,
		Event: &EventPayload{
			Kind:       "alert",
			CameraID:   "cam-01",
			RecorderID: "rec-abc",
			Timestamp:  time.Now().UTC().Truncate(time.Second),
			Data:       json.RawMessage(`{"rule_id":"motion-1","severity":"high"}`),
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Event.Kind != "alert" {
		t.Fatalf("kind = %q, want %q", decoded.Event.Kind, "alert")
	}
	if decoded.Event.CameraID != "cam-01" {
		t.Fatalf("camera_id = %q, want %q", decoded.Event.CameraID, "cam-01")
	}
}

func TestMarshalCommandMessage(t *testing.T) {
	msg := Envelope{
		Type: MsgTypeCommand,
		Command: &CommandPayload{
			ID:   "cmd-123",
			Kind: "relay_request",
			Data: json.RawMessage(`{"client_id":"cl-456","stream_id":"s-789"}`),
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Command.Kind != "relay_request" {
		t.Fatalf("kind = %q, want %q", decoded.Command.Kind, "relay_request")
	}
}

func TestMarshalCommandResponseMessage(t *testing.T) {
	msg := Envelope{
		Type: MsgTypeCommandResponse,
		CommandResponse: &CommandResponsePayload{
			ID:      "cmd-123",
			Success: true,
			Data:    json.RawMessage(`{"relay_token":"tok-abc"}`),
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !decoded.CommandResponse.Success {
		t.Fatal("success should be true")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/directory/cloudconnector/... -v -run TestMarshal
```

Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/directory/cloudconnector/messages.go
package cloudconnector

import (
	"encoding/json"
	"time"
)

// Message types for the WSS control channel.
const (
	// On-prem → Cloud
	MsgTypeRegister        = "register"
	MsgTypeHeartbeat       = "heartbeat"
	MsgTypeEvent           = "event"
	MsgTypeCommandResponse = "command_response"

	// Cloud → On-prem
	MsgTypeCommand    = "command"
	MsgTypeRegistered = "registered" // ack for register
)

// Envelope is the top-level wire message. Exactly one payload field is
// non-nil, determined by Type.
type Envelope struct {
	Type            string                  `json:"type"`
	Register        *RegisterPayload        `json:"register,omitempty"`
	Registered      *RegisteredPayload      `json:"registered,omitempty"`
	Heartbeat       *HeartbeatPayload       `json:"heartbeat,omitempty"`
	Event           *EventPayload           `json:"event,omitempty"`
	Command         *CommandPayload         `json:"command,omitempty"`
	CommandResponse *CommandResponsePayload `json:"command_response,omitempty"`
}

// RegisterPayload is sent once on WSS connect. Announces the site to the cloud.
type RegisterPayload struct {
	SiteID       string       `json:"site_id"`
	SiteAlias    string       `json:"site_alias"`
	Version      string       `json:"version"`
	PublicIP     string       `json:"public_ip"`
	LANCIDRs     []string     `json:"lan_cidrs"`
	Capabilities Capabilities `json:"capabilities"`
}

// Capabilities declares what this site supports.
type Capabilities struct {
	Streams  bool `json:"streams"`
	Playback bool `json:"playback"`
	AI       bool `json:"ai"`
}

// RegisteredPayload is the cloud's ack to a register message.
type RegisteredPayload struct {
	OK         bool   `json:"ok"`
	RelayURL   string `json:"relay_url,omitempty"`
	Error      string `json:"error,omitempty"`
}

// HeartbeatPayload is sent periodically (every 30s) to maintain the session.
type HeartbeatPayload struct {
	SiteID        string    `json:"site_id"`
	Timestamp     time.Time `json:"timestamp"`
	UptimeSec     int64     `json:"uptime_sec"`
	CameraCount   int       `json:"camera_count"`
	RecorderCount int       `json:"recorder_count"`
	DiskUsedPct   float64   `json:"disk_used_pct"`
	PublicIP      string    `json:"public_ip"`
}

// EventPayload carries an event from on-prem to cloud (alerts, AI detections,
// camera status changes, telemetry).
type EventPayload struct {
	Kind       string          `json:"kind"`
	CameraID   string          `json:"camera_id,omitempty"`
	RecorderID string          `json:"recorder_id,omitempty"`
	Timestamp  time.Time       `json:"timestamp"`
	Data       json.RawMessage `json:"data,omitempty"`
}

// CommandPayload is sent from cloud to on-prem (relay requests, config pushes).
type CommandPayload struct {
	ID   string          `json:"id"`
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data,omitempty"`
}

// CommandResponsePayload is the on-prem's response to a command.
type CommandResponsePayload struct {
	ID      string          `json:"id"`
	Success bool            `json:"success"`
	Error   string          `json:"error,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/directory/cloudconnector/... -v -run TestMarshal
```

Expected: all 5 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/directory/cloudconnector/messages.go internal/directory/cloudconnector/messages_test.go
git commit -m "feat(cloudconnector): define wire message types for WSS control channel"
```

---

## Phase 2: Cloud Connector (On-Prem WSS Client)

### Task 2: Add config fields

**Files:**
- Modify: `internal/conf/conf.go:401-414` (add fields after NVR block)
- Modify: `internal/directory/boot.go:46-84` (add to BootConfig)

- [ ] **Step 1: Add fields to Conf struct**

In `internal/conf/conf.go`, after the `NVRRecorderID` field (line 414), add:

```go
	// Cloud Connector — outbound WSS to cloud broker for remote access
	// and cloud services. Empty = air-gapped mode (default).
	CloudConnectURL   string `json:"cloudConnectURL"`
	CloudConnectToken string `json:"cloudConnectToken"`
	CloudSiteAlias    string `json:"cloudSiteAlias"`
```

- [ ] **Step 2: Add fields to BootConfig**

In `internal/directory/boot.go`, add to the `BootConfig` struct after the `NVRDBPath` field (line 83):

```go
	// CloudConnectURL is the cloud broker WebSocket endpoint, e.g.
	// "wss://connect.raikada.com/ws/directory". Empty disables the
	// cloud connector (air-gapped mode).
	CloudConnectURL string

	// CloudConnectToken is the bearer token used to authenticate
	// with the cloud broker. Issued during cloud account setup.
	CloudConnectToken string

	// CloudSiteAlias is the human-readable alias for this site,
	// e.g. "my-home". Used as the QuickConnect-style identifier.
	CloudSiteAlias string
```

- [ ] **Step 3: Verify compilation**

```bash
go build ./internal/conf/...
go build ./internal/directory/...
```

Expected: clean compilation.

- [ ] **Step 4: Commit**

```bash
git add internal/conf/conf.go internal/directory/boot.go
git commit -m "feat(conf): add CloudConnect config fields for cloud connector"
```

---

### Task 3: Implement the Connector (WSS client with reconnect)

**Files:**
- Create: `internal/directory/cloudconnector/connector.go`
- Create: `internal/directory/cloudconnector/connector_test.go`

This is the core component: a persistent outbound WebSocket client that registers with the cloud, sends heartbeats, forwards events, and handles incoming commands.

- [ ] **Step 1: Write the failing test**

```go
// internal/directory/cloudconnector/connector_test.go
package cloudconnector

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var testUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

func TestConnectorRegisters(t *testing.T) {
	var received Envelope
	var mu sync.Mutex
	done := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade: %v", err)
		}
		defer ws.Close()

		// Read the register message.
		_, data, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		mu.Lock()
		if err := json.Unmarshal(data, &received); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		mu.Unlock()

		// Send registered ack.
		ack := Envelope{
			Type: MsgTypeRegistered,
			Registered: &RegisteredPayload{
				OK:       true,
				RelayURL: "wss://relay.raikada.com",
			},
		}
		ackData, _ := json.Marshal(ack)
		_ = ws.WriteMessage(websocket.TextMessage, ackData)

		close(done)
		// Keep connection open briefly so connector doesn't reconnect.
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/directory"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c := New(Config{
		URL:   wsURL,
		Token: "test-token",
		Site: SiteInfo{
			ID:    "site-123",
			Alias: "my-home",
		},
		HeartbeatInterval: 30 * time.Second,
		Logger:            slog.Default(),
	})

	go c.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for register message")
	}

	mu.Lock()
	defer mu.Unlock()

	if received.Type != MsgTypeRegister {
		t.Fatalf("type = %q, want %q", received.Type, MsgTypeRegister)
	}
	if received.Register.SiteAlias != "my-home" {
		t.Fatalf("alias = %q, want %q", received.Register.SiteAlias, "my-home")
	}
}

func TestConnectorSendsHeartbeats(t *testing.T) {
	heartbeatCount := 0
	var mu sync.Mutex
	done := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade: %v", err)
		}
		defer ws.Close()

		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			var env Envelope
			if err := json.Unmarshal(data, &env); err != nil {
				continue
			}

			if env.Type == MsgTypeRegister {
				ack := Envelope{
					Type:       MsgTypeRegistered,
					Registered: &RegisteredPayload{OK: true},
				}
				ackData, _ := json.Marshal(ack)
				_ = ws.WriteMessage(websocket.TextMessage, ackData)
				continue
			}

			if env.Type == MsgTypeHeartbeat {
				mu.Lock()
				heartbeatCount++
				if heartbeatCount >= 2 {
					mu.Unlock()
					close(done)
					return
				}
				mu.Unlock()
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/directory"

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c := New(Config{
		URL:   wsURL,
		Token: "test-token",
		Site: SiteInfo{
			ID:    "site-123",
			Alias: "my-home",
		},
		HeartbeatInterval: 100 * time.Millisecond, // fast for testing
		Logger:            slog.Default(),
	})

	go c.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for 2 heartbeats")
	}

	mu.Lock()
	defer mu.Unlock()
	if heartbeatCount < 2 {
		t.Fatalf("heartbeat_count = %d, want >= 2", heartbeatCount)
	}
}

func TestConnectorReconnectsOnClose(t *testing.T) {
	connectCount := 0
	var mu sync.Mutex
	done := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		mu.Lock()
		connectCount++
		count := connectCount
		mu.Unlock()

		if count == 1 {
			// First connection: read register, ack, then close immediately.
			_, _, _ = ws.ReadMessage()
			ack := Envelope{
				Type:       MsgTypeRegistered,
				Registered: &RegisteredPayload{OK: true},
			}
			ackData, _ := json.Marshal(ack)
			_ = ws.WriteMessage(websocket.TextMessage, ackData)
			ws.Close()
			return
		}

		// Second connection: success — we reconnected.
		_, _, _ = ws.ReadMessage()
		ack := Envelope{
			Type:       MsgTypeRegistered,
			Registered: &RegisteredPayload{OK: true},
		}
		ackData, _ := json.Marshal(ack)
		_ = ws.WriteMessage(websocket.TextMessage, ackData)
		close(done)
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/directory"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c := New(Config{
		URL:               wsURL,
		Token:             "test-token",
		Site:              SiteInfo{ID: "site-123", Alias: "my-home"},
		HeartbeatInterval: 30 * time.Second,
		MinReconnectDelay: 50 * time.Millisecond,
		MaxReconnectDelay: 200 * time.Millisecond,
		Logger:            slog.Default(),
	})

	go c.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for reconnect")
	}

	mu.Lock()
	defer mu.Unlock()
	if connectCount < 2 {
		t.Fatalf("connect_count = %d, want >= 2", connectCount)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/directory/cloudconnector/... -v -run "TestConnector"
```

Expected: FAIL — `New` and `Config` not defined.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/directory/cloudconnector/connector.go
package cloudconnector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// SiteInfo identifies this on-prem site.
type SiteInfo struct {
	ID           string
	Alias        string
	Version      string
	LANCIDRs     []string
	Capabilities Capabilities
}

// Config for the cloud connector.
type Config struct {
	// URL is the cloud broker WSS endpoint, e.g.
	// "wss://connect.raikada.com/ws/directory".
	URL string

	// Token is the bearer token for authentication.
	Token string

	// Site describes this on-prem installation.
	Site SiteInfo

	// HeartbeatInterval between heartbeat messages. Default: 30s.
	HeartbeatInterval time.Duration

	// MinReconnectDelay is the initial backoff delay. Default: 1s.
	MinReconnectDelay time.Duration

	// MaxReconnectDelay is the maximum backoff delay. Default: 60s.
	MaxReconnectDelay time.Duration

	// SiteInfoFunc is called before each heartbeat to get current
	// stats (camera count, disk usage, etc). Optional.
	SiteInfoFunc func() HeartbeatPayload

	// CommandHandler is called for each command received from cloud.
	// Optional — unhandled commands get a failure response.
	CommandHandler func(ctx context.Context, cmd CommandPayload) (json.RawMessage, error)

	Logger *slog.Logger
}

func (c *Config) withDefaults() {
	if c.HeartbeatInterval == 0 {
		c.HeartbeatInterval = 30 * time.Second
	}
	if c.MinReconnectDelay == 0 {
		c.MinReconnectDelay = 1 * time.Second
	}
	if c.MaxReconnectDelay == 0 {
		c.MaxReconnectDelay = 60 * time.Second
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

// Connector maintains the outbound WSS control channel to the cloud.
type Connector struct {
	cfg Config

	mu       sync.Mutex
	conn     *websocket.Conn
	relayURL string // populated from RegisteredPayload
}

// New creates a new cloud connector.
func New(cfg Config) *Connector {
	cfg.withDefaults()
	return &Connector{cfg: cfg}
}

// RelayURL returns the relay URL provided by the cloud after registration.
func (c *Connector) RelayURL() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.relayURL
}

// SendEvent sends an event to the cloud. Safe to call from any goroutine.
// Returns an error if not connected.
func (c *Connector) SendEvent(event EventPayload) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	env := Envelope{
		Type:  MsgTypeEvent,
		Event: &event,
	}
	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// Run connects to the cloud and maintains the connection with automatic
// reconnect. Blocks until ctx is cancelled.
func (c *Connector) Run(ctx context.Context) {
	attempt := 0
	for {
		if ctx.Err() != nil {
			return
		}

		err := c.connectAndServe(ctx)
		if ctx.Err() != nil {
			return
		}

		attempt++
		delay := c.backoff(attempt)
		c.cfg.Logger.Warn("cloudconnector: connection lost, reconnecting",
			"error", err, "attempt", attempt, "delay", delay)

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
	}
}

// connectAndServe dials the cloud, registers, and runs the heartbeat +
// read loop. Returns when the connection is lost.
func (c *Connector) connectAndServe(ctx context.Context) error {
	log := c.cfg.Logger

	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.cfg.Token)

	log.Info("cloudconnector: connecting", "url", c.cfg.URL)
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

	// Send register message.
	regMsg := Envelope{
		Type: MsgTypeRegister,
		Register: &RegisterPayload{
			SiteID:       c.cfg.Site.ID,
			SiteAlias:    c.cfg.Site.Alias,
			Version:      c.cfg.Site.Version,
			LANCIDRs:     c.cfg.Site.LANCIDRs,
			Capabilities: c.cfg.Site.Capabilities,
		},
	}
	regData, _ := json.Marshal(regMsg)
	if err := ws.WriteMessage(websocket.TextMessage, regData); err != nil {
		return fmt.Errorf("send register: %w", err)
	}

	// Wait for registered ack.
	ws.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, ackData, err := ws.ReadMessage()
	if err != nil {
		return fmt.Errorf("read registered ack: %w", err)
	}
	ws.SetReadDeadline(time.Time{}) // clear deadline

	var ack Envelope
	if err := json.Unmarshal(ackData, &ack); err != nil {
		return fmt.Errorf("unmarshal ack: %w", err)
	}
	if ack.Type != MsgTypeRegistered || ack.Registered == nil || !ack.Registered.OK {
		errMsg := "unknown"
		if ack.Registered != nil {
			errMsg = ack.Registered.Error
		}
		return fmt.Errorf("registration rejected: %s", errMsg)
	}

	c.mu.Lock()
	c.relayURL = ack.Registered.RelayURL
	c.mu.Unlock()

	log.Info("cloudconnector: registered",
		"site_alias", c.cfg.Site.Alias,
		"relay_url", ack.Registered.RelayURL)

	// Run heartbeat and read loops concurrently.
	readErr := make(chan error, 1)
	go func() {
		readErr <- c.readLoop(ctx, ws)
	}()

	ticker := time.NewTicker(c.cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			ws.WriteMessage(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseGoingAway, "shutdown"),
			)
			return ctx.Err()

		case err := <-readErr:
			return fmt.Errorf("read loop: %w", err)

		case <-ticker.C:
			if err := c.sendHeartbeat(ws); err != nil {
				return fmt.Errorf("heartbeat: %w", err)
			}
		}
	}
}

// readLoop reads incoming messages (commands from cloud).
func (c *Connector) readLoop(ctx context.Context, ws *websocket.Conn) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		_, data, err := ws.ReadMessage()
		if err != nil {
			return err
		}

		var env Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			c.cfg.Logger.Warn("cloudconnector: invalid message", "error", err)
			continue
		}

		if env.Type == MsgTypeCommand && env.Command != nil {
			go c.handleCommand(ctx, ws, *env.Command)
		}
	}
}

// handleCommand processes a command from cloud and sends a response.
func (c *Connector) handleCommand(ctx context.Context, ws *websocket.Conn, cmd CommandPayload) {
	resp := CommandResponsePayload{ID: cmd.ID}

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
		resp.Error = "no handler registered"
	}

	env := Envelope{
		Type:            MsgTypeCommandResponse,
		CommandResponse: &resp,
	}
	envData, _ := json.Marshal(env)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.WriteMessage(websocket.TextMessage, envData)
	}
}

// sendHeartbeat sends a heartbeat with current site stats.
func (c *Connector) sendHeartbeat(ws *websocket.Conn) error {
	hb := HeartbeatPayload{
		SiteID:    c.cfg.Site.ID,
		Timestamp: time.Now().UTC(),
	}

	if c.cfg.SiteInfoFunc != nil {
		hb = c.cfg.SiteInfoFunc()
		hb.SiteID = c.cfg.Site.ID
		hb.Timestamp = time.Now().UTC()
	}

	env := Envelope{
		Type:      MsgTypeHeartbeat,
		Heartbeat: &hb,
	}
	data, _ := json.Marshal(env)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// backoff returns the delay before the next reconnect attempt.
// Uses exponential backoff with jitter.
func (c *Connector) backoff(attempt int) time.Duration {
	min := c.cfg.MinReconnectDelay
	max := c.cfg.MaxReconnectDelay

	delay := time.Duration(float64(min) * math.Pow(2, float64(attempt-1)))
	if delay > max {
		delay = max
	}
	// Add up to 25% jitter.
	jitter := time.Duration(rand.Int64N(int64(delay / 4)))
	return delay + jitter
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/directory/cloudconnector/... -v -run "TestConnector" -count=1
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/directory/cloudconnector/connector.go internal/directory/cloudconnector/connector_test.go
git commit -m "feat(cloudconnector): implement WSS client with register, heartbeat, and reconnect"
```

---

### Task 4: Integrate Connector into Directory boot sequence

**Files:**
- Modify: `internal/directory/boot.go`

- [ ] **Step 1: Add Connector field to DirectoryServer**

In `internal/directory/boot.go`, add to the `DirectoryServer` struct (after `Broadcaster` on line 122):

```go
	CloudConn   *cloudconnector.Connector
	cloudCancel context.CancelFunc
```

Add the import:

```go
	"github.com/bluenviron/mediamtx/internal/directory/cloudconnector"
```

- [ ] **Step 2: Add shutdown logic**

In the `Shutdown` method, add before the Broadcaster shutdown (before line 130):

```go
	if ds.cloudCancel != nil {
		ds.cloudCancel()
	}
```

- [ ] **Step 3: Start connector after mDNS broadcaster (step 7)**

After the mDNS broadcaster block (after line 596), add a new step 8:

```go
	// ---------------------------------------------------------------
	// 8. Start cloud connector (optional — air-gapped if URL is empty)
	// ---------------------------------------------------------------
	if cfg.CloudConnectURL != "" {
		log.Info("directory: starting cloud connector",
			"url", cfg.CloudConnectURL,
			"alias", cfg.CloudSiteAlias)

		cloudCtx, cloudCancel := context.WithCancel(context.Background())
		srv.cloudCancel = cloudCancel

		cc := cloudconnector.New(cloudconnector.Config{
			URL:   cfg.CloudConnectURL,
			Token: cfg.CloudConnectToken,
			Site: cloudconnector.SiteInfo{
				ID:    cfg.CloudSiteAlias,
				Alias: cfg.CloudSiteAlias,
				Capabilities: cloudconnector.Capabilities{
					Streams:  true,
					Playback: true,
					AI:       true,
				},
			},
			Logger: log.With(slog.String("component", "cloudconnector")),
		})
		srv.CloudConn = cc

		go cc.Run(cloudCtx)
	} else {
		log.Info("directory: cloud connector disabled (air-gapped mode)")
	}
```

- [ ] **Step 4: Verify compilation**

```bash
go build ./internal/directory/...
```

Expected: clean compilation.

- [ ] **Step 5: Commit**

```bash
git add internal/directory/boot.go
git commit -m "feat(directory): integrate cloud connector into boot sequence"
```

---

## Phase 3: Cloud-Side Broker

### Task 5: Cloud DB migration for site registry

**Files:**
- Create: `internal/cloud/db/migrations/0035_cloud_connect.up.sql`
- Create: `internal/cloud/db/migrations/0035_cloud_connect.down.sql`

- [ ] **Step 1: Write the up migration**

```sql
-- internal/cloud/db/migrations/0035_cloud_connect.up.sql

-- Sites registered via cloud connector (on-prem Directories).
CREATE TABLE IF NOT EXISTS connected_sites (
    site_id        TEXT PRIMARY KEY,
    tenant_id      TEXT NOT NULL REFERENCES customer_tenants(id),
    site_alias     TEXT NOT NULL,
    display_name   TEXT NOT NULL DEFAULT '',
    version        TEXT NOT NULL DEFAULT '',
    public_ip      TEXT NOT NULL DEFAULT '',
    lan_cidrs      TEXT NOT NULL DEFAULT '[]',   -- JSON array of CIDR strings
    capabilities   TEXT NOT NULL DEFAULT '{}',   -- JSON object
    status         TEXT NOT NULL DEFAULT 'offline' CHECK (status IN ('online','offline')),
    relay_url      TEXT NOT NULL DEFAULT '',
    last_seen_at   DATETIME,
    camera_count   INTEGER NOT NULL DEFAULT 0,
    recorder_count INTEGER NOT NULL DEFAULT 0,
    disk_used_pct  REAL NOT NULL DEFAULT 0,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_connected_sites_alias
    ON connected_sites(tenant_id, site_alias);

-- Relay sessions — tracks active relay tunnels through the cloud.
CREATE TABLE IF NOT EXISTS relay_sessions (
    session_id    TEXT PRIMARY KEY,
    site_id       TEXT NOT NULL REFERENCES connected_sites(site_id),
    client_id     TEXT NOT NULL,
    stream_id     TEXT NOT NULL DEFAULT '',
    started_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_active   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    bytes_relayed INTEGER NOT NULL DEFAULT 0,
    status        TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','closed'))
);

CREATE INDEX IF NOT EXISTS idx_relay_sessions_site
    ON relay_sessions(site_id, status);
```

- [ ] **Step 2: Write the down migration**

```sql
-- internal/cloud/db/migrations/0035_cloud_connect.down.sql
DROP INDEX IF EXISTS idx_relay_sessions_site;
DROP TABLE IF EXISTS relay_sessions;
DROP INDEX IF EXISTS idx_connected_sites_alias;
DROP TABLE IF EXISTS connected_sites;
```

- [ ] **Step 3: Verify migration applies**

```bash
go test ./internal/cloud/db/... -v -run TestMigrations -count=1
```

Expected: PASS (if there's a migration test), or verify manually that the SQL is valid:

```bash
sqlite3 ":memory:" < internal/cloud/db/migrations/0035_cloud_connect.up.sql && echo "OK"
```

- [ ] **Step 4: Commit**

```bash
git add internal/cloud/db/migrations/0035_cloud_connect.up.sql internal/cloud/db/migrations/0035_cloud_connect.down.sql
git commit -m "feat(clouddb): add connected_sites and relay_sessions tables"
```

---

### Task 6: Site registry (session manager)

**Files:**
- Create: `internal/cloud/connect/registry.go`
- Create: `internal/cloud/connect/registry_test.go`

The registry tracks which on-prem Directories are currently connected and maps site aliases to active WebSocket sessions.

- [ ] **Step 1: Write the failing test**

```go
// internal/cloud/connect/registry_test.go
package connect

import (
	"testing"
	"time"
)

func TestRegistryAddAndLookup(t *testing.T) {
	r := NewRegistry()

	r.Add(Session{
		SiteID:    "site-123",
		TenantID:  "tenant-abc",
		SiteAlias: "my-home",
		PublicIP:  "73.42.100.5",
		LANCIDRs:  []string{"192.168.1.0/24"},
		Status:    StatusOnline,
		LastSeen:  time.Now(),
	})

	s, ok := r.LookupByAlias("tenant-abc", "my-home")
	if !ok {
		t.Fatal("expected to find site by alias")
	}
	if s.SiteID != "site-123" {
		t.Fatalf("site_id = %q, want %q", s.SiteID, "site-123")
	}
	if s.PublicIP != "73.42.100.5" {
		t.Fatalf("public_ip = %q, want %q", s.PublicIP, "73.42.100.5")
	}
}

func TestRegistryRemove(t *testing.T) {
	r := NewRegistry()

	r.Add(Session{
		SiteID:    "site-123",
		TenantID:  "tenant-abc",
		SiteAlias: "my-home",
		Status:    StatusOnline,
		LastSeen:  time.Now(),
	})

	r.Remove("site-123")

	_, ok := r.LookupByAlias("tenant-abc", "my-home")
	if ok {
		t.Fatal("expected site to be removed")
	}
}

func TestRegistryUpdateHeartbeat(t *testing.T) {
	r := NewRegistry()

	r.Add(Session{
		SiteID:    "site-123",
		TenantID:  "tenant-abc",
		SiteAlias: "my-home",
		Status:    StatusOnline,
		LastSeen:  time.Now().Add(-5 * time.Minute),
	})

	r.UpdateHeartbeat("site-123", HeartbeatUpdate{
		CameraCount:   4,
		RecorderCount: 1,
		DiskUsedPct:   42.5,
		PublicIP:       "73.42.100.6",
	})

	s, ok := r.LookupByAlias("tenant-abc", "my-home")
	if !ok {
		t.Fatal("expected to find site")
	}
	if s.CameraCount != 4 {
		t.Fatalf("camera_count = %d, want 4", s.CameraCount)
	}
	if s.PublicIP != "73.42.100.6" {
		t.Fatalf("public_ip = %q, want %q", s.PublicIP, "73.42.100.6")
	}
}

func TestRegistryListByTenant(t *testing.T) {
	r := NewRegistry()

	r.Add(Session{SiteID: "s1", TenantID: "t1", SiteAlias: "home", Status: StatusOnline, LastSeen: time.Now()})
	r.Add(Session{SiteID: "s2", TenantID: "t1", SiteAlias: "office", Status: StatusOnline, LastSeen: time.Now()})
	r.Add(Session{SiteID: "s3", TenantID: "t2", SiteAlias: "home", Status: StatusOnline, LastSeen: time.Now()})

	sites := r.ListByTenant("t1")
	if len(sites) != 2 {
		t.Fatalf("len = %d, want 2", len(sites))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/cloud/connect/... -v -run TestRegistry
```

Expected: FAIL — package does not exist.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/cloud/connect/registry.go
package connect

import (
	"sync"
	"time"
)

// Status of a connected site.
const (
	StatusOnline  = "online"
	StatusOffline = "offline"
)

// Session represents an active cloud-connected on-prem site.
type Session struct {
	SiteID        string
	TenantID      string
	SiteAlias     string
	PublicIP      string
	LANCIDRs      []string
	Capabilities  map[string]bool
	Status        string
	LastSeen      time.Time
	CameraCount   int
	RecorderCount int
	DiskUsedPct   float64
}

// HeartbeatUpdate carries the mutable fields from a heartbeat.
type HeartbeatUpdate struct {
	CameraCount   int
	RecorderCount int
	DiskUsedPct   float64
	PublicIP      string
}

// Registry is an in-memory index of active cloud-connected sites.
// Thread-safe.
type Registry struct {
	mu       sync.RWMutex
	bySiteID map[string]*Session
	// alias key: "tenant_id:site_alias"
	byAlias map[string]string // alias key → site_id
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		bySiteID: make(map[string]*Session),
		byAlias:  make(map[string]string),
	}
}

func aliasKey(tenantID, alias string) string {
	return tenantID + ":" + alias
}

// Add registers a site session. Overwrites if site_id already exists.
func (r *Registry) Add(s Session) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove old alias if site already existed with a different alias.
	if old, ok := r.bySiteID[s.SiteID]; ok {
		delete(r.byAlias, aliasKey(old.TenantID, old.SiteAlias))
	}

	r.bySiteID[s.SiteID] = &s
	r.byAlias[aliasKey(s.TenantID, s.SiteAlias)] = s.SiteID
}

// Remove deletes a site from the registry.
func (r *Registry) Remove(siteID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if s, ok := r.bySiteID[siteID]; ok {
		delete(r.byAlias, aliasKey(s.TenantID, s.SiteAlias))
		delete(r.bySiteID, siteID)
	}
}

// LookupByAlias finds a site by tenant + alias.
func (r *Registry) LookupByAlias(tenantID, alias string) (Session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	siteID, ok := r.byAlias[aliasKey(tenantID, alias)]
	if !ok {
		return Session{}, false
	}
	s, ok := r.bySiteID[siteID]
	if !ok {
		return Session{}, false
	}
	return *s, true
}

// UpdateHeartbeat updates mutable fields for a site.
func (r *Registry) UpdateHeartbeat(siteID string, u HeartbeatUpdate) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.bySiteID[siteID]
	if !ok {
		return
	}
	s.CameraCount = u.CameraCount
	s.RecorderCount = u.RecorderCount
	s.DiskUsedPct = u.DiskUsedPct
	s.LastSeen = time.Now()
	if u.PublicIP != "" {
		s.PublicIP = u.PublicIP
	}
}

// ListByTenant returns all sessions for a tenant.
func (r *Registry) ListByTenant(tenantID string) []Session {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []Session
	for _, s := range r.bySiteID {
		if s.TenantID == tenantID {
			out = append(out, *s)
		}
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/cloud/connect/... -v -run TestRegistry
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cloud/connect/registry.go internal/cloud/connect/registry_test.go
git commit -m "feat(connect): implement in-memory site registry"
```

---

### Task 7: Connection broker (WSS server + resolve endpoint)

**Files:**
- Create: `internal/cloud/connect/broker.go`
- Create: `internal/cloud/connect/broker_test.go`
- Create: `internal/cloud/connect/resolve.go`
- Create: `internal/cloud/connect/resolve_test.go`

The broker accepts incoming WSS connections from on-prem Directories and exposes an HTTP endpoint for clients to resolve a site alias into a connection plan.

- [ ] **Step 1: Write the failing test for broker**

```go
// internal/cloud/connect/broker_test.go
package connect

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/directory/cloudconnector"
	"github.com/gorilla/websocket"
)

func TestBrokerAcceptsDirectoryConnection(t *testing.T) {
	reg := NewRegistry()
	b := NewBroker(BrokerConfig{
		Registry:     reg,
		Authenticate: func(token string) (tenantID string, ok bool) {
			if token == "valid-token" {
				return "tenant-abc", true
			}
			return "", false
		},
		Logger: slog.Default(),
	})

	srv := httptest.NewServer(b)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	header := http.Header{}
	header.Set("Authorization", "Bearer valid-token")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()

	// Send register.
	reg_msg := cloudconnector.Envelope{
		Type: cloudconnector.MsgTypeRegister,
		Register: &cloudconnector.RegisterPayload{
			SiteID:    "site-123",
			SiteAlias: "my-home",
			LANCIDRs:  []string{"192.168.1.0/24"},
			Capabilities: cloudconnector.Capabilities{
				Streams: true,
			},
		},
	}
	data, _ := json.Marshal(reg_msg)
	if err := ws.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read ack.
	_, ackData, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var ack cloudconnector.Envelope
	if err := json.Unmarshal(ackData, &ack); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ack.Type != cloudconnector.MsgTypeRegistered {
		t.Fatalf("type = %q, want %q", ack.Type, cloudconnector.MsgTypeRegistered)
	}
	if !ack.Registered.OK {
		t.Fatalf("registered.ok = false, error = %q", ack.Registered.Error)
	}

	// Verify registry has the site.
	time.Sleep(50 * time.Millisecond) // let broker goroutine process
	s, ok := reg.LookupByAlias("tenant-abc", "my-home")
	if !ok {
		t.Fatal("expected site in registry")
	}
	if s.SiteID != "site-123" {
		t.Fatalf("site_id = %q, want %q", s.SiteID, "site-123")
	}
}

func TestBrokerRejectsInvalidToken(t *testing.T) {
	reg := NewRegistry()
	b := NewBroker(BrokerConfig{
		Registry:     reg,
		Authenticate: func(token string) (string, bool) { return "", false },
		Logger:       slog.Default(),
	})

	srv := httptest.NewServer(b)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	header := http.Header{}
	header.Set("Authorization", "Bearer bad-token")
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err == nil {
		t.Fatal("expected dial to fail")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/cloud/connect/... -v -run TestBroker
```

Expected: FAIL — `NewBroker`, `BrokerConfig` not defined.

- [ ] **Step 3: Write broker implementation**

```go
// internal/cloud/connect/broker.go
package connect

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/directory/cloudconnector"
	"github.com/gorilla/websocket"
)

var brokerUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

// BrokerConfig configures the cloud-side broker.
type BrokerConfig struct {
	Registry *Registry

	// Authenticate validates a bearer token and returns the tenant ID.
	Authenticate func(token string) (tenantID string, ok bool)

	// RelayURL is the base URL for the relay server, e.g.
	// "wss://relay.raikada.com". Sent to Directories on registration.
	RelayURL string

	Logger *slog.Logger
}

// Broker accepts WebSocket connections from on-prem Directories.
// Implements http.Handler.
type Broker struct {
	cfg BrokerConfig
}

// NewBroker creates a new broker.
func NewBroker(cfg BrokerConfig) *Broker {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Broker{cfg: cfg}
}

func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Authenticate via bearer token.
	auth := r.Header.Get("Authorization")
	token := strings.TrimPrefix(auth, "Bearer ")
	if token == "" || token == auth {
		http.Error(w, `{"error":"missing bearer token"}`, http.StatusUnauthorized)
		return
	}

	tenantID, ok := b.cfg.Authenticate(token)
	if !ok {
		http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
		return
	}

	ws, err := brokerUpgrader.Upgrade(w, r, nil)
	if err != nil {
		b.cfg.Logger.Error("broker: upgrade failed", "error", err)
		return
	}
	defer ws.Close()

	b.cfg.Logger.Info("broker: directory connected", "tenant", tenantID, "remote", r.RemoteAddr)

	b.serveDirectory(ws, tenantID)
}

func (b *Broker) serveDirectory(ws *websocket.Conn, tenantID string) {
	log := b.cfg.Logger

	// Read register message.
	ws.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, data, err := ws.ReadMessage()
	if err != nil {
		log.Warn("broker: read register failed", "error", err)
		return
	}
	ws.SetReadDeadline(time.Time{})

	var env cloudconnector.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		log.Warn("broker: invalid register message", "error", err)
		return
	}
	if env.Type != cloudconnector.MsgTypeRegister || env.Register == nil {
		log.Warn("broker: expected register message", "type", env.Type)
		return
	}

	reg := env.Register
	siteID := reg.SiteID

	// Register in the session registry.
	b.cfg.Registry.Add(Session{
		SiteID:    siteID,
		TenantID:  tenantID,
		SiteAlias: reg.SiteAlias,
		PublicIP:  reg.PublicIP,
		LANCIDRs:  reg.LANCIDRs,
		Capabilities: map[string]bool{
			"streams":  reg.Capabilities.Streams,
			"playback": reg.Capabilities.Playback,
			"ai":       reg.Capabilities.AI,
		},
		Status:   StatusOnline,
		LastSeen: time.Now(),
	})
	defer b.cfg.Registry.Remove(siteID)

	// Send registered ack.
	ack := cloudconnector.Envelope{
		Type: cloudconnector.MsgTypeRegistered,
		Registered: &cloudconnector.RegisteredPayload{
			OK:       true,
			RelayURL: b.cfg.RelayURL,
		},
	}
	ackData, _ := json.Marshal(ack)
	if err := ws.WriteMessage(websocket.TextMessage, ackData); err != nil {
		log.Warn("broker: write ack failed", "error", err)
		return
	}

	log.Info("broker: directory registered",
		"site_id", siteID,
		"alias", reg.SiteAlias,
		"tenant", tenantID)

	// Read loop — process heartbeats and events until disconnect.
	for {
		_, msgData, err := ws.ReadMessage()
		if err != nil {
			log.Info("broker: directory disconnected", "site_id", siteID, "error", err)
			return
		}

		var msg cloudconnector.Envelope
		if err := json.Unmarshal(msgData, &msg); err != nil {
			log.Warn("broker: invalid message", "error", err)
			continue
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
			}

		case cloudconnector.MsgTypeEvent:
			// TODO: fan out to notification dispatcher, analytics, etc.
			log.Debug("broker: event received", "site_id", siteID, "kind", msg.Event.Kind)

		case cloudconnector.MsgTypeCommandResponse:
			// TODO: deliver to waiting command callers.
			log.Debug("broker: command response", "site_id", siteID, "id", msg.CommandResponse.ID)
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/cloud/connect/... -v -run TestBroker
```

Expected: all 2 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cloud/connect/broker.go internal/cloud/connect/broker_test.go
git commit -m "feat(connect): implement cloud-side broker for Directory WSS connections"
```

- [ ] **Step 6: Write the failing test for resolve endpoint**

```go
// internal/cloud/connect/resolve_test.go
package connect

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResolveReturnsConnectionPlan(t *testing.T) {
	reg := NewRegistry()
	reg.Add(Session{
		SiteID:    "site-123",
		TenantID:  "tenant-abc",
		SiteAlias: "my-home",
		PublicIP:  "73.42.100.5",
		LANCIDRs:  []string{"192.168.1.0/24"},
		Status:    StatusOnline,
		LastSeen:  time.Now(),
	})

	h := ResolveHandler(ResolveConfig{
		Registry:       reg,
		RelayBaseURL:   "wss://relay.raikada.com",
		STUNServers:    []string{"stun:stun.l.google.com:19302"},
	})

	req := httptest.NewRequest(http.MethodGet, "/connect/resolve?tenant_id=tenant-abc&alias=my-home", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}

	var resp ConnectionPlan
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.SiteID != "site-123" {
		t.Fatalf("site_id = %q, want %q", resp.SiteID, "site-123")
	}
	if resp.Status != StatusOnline {
		t.Fatalf("status = %q, want %q", resp.Status, StatusOnline)
	}
	if len(resp.Endpoints) == 0 {
		t.Fatal("expected at least one endpoint")
	}

	// Should have LAN + relay at minimum.
	kinds := map[string]bool{}
	for _, ep := range resp.Endpoints {
		kinds[ep.Kind] = true
	}
	if !kinds["lan"] {
		t.Fatal("expected lan endpoint")
	}
	if !kinds["relay"] {
		t.Fatal("expected relay endpoint")
	}
}

func TestResolveOfflineSite(t *testing.T) {
	reg := NewRegistry()

	h := ResolveHandler(ResolveConfig{
		Registry: reg,
	})

	req := httptest.NewRequest(http.MethodGet, "/connect/resolve?tenant_id=tenant-abc&alias=no-such-site", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}
```

- [ ] **Step 7: Run test to verify it fails**

```bash
go test ./internal/cloud/connect/... -v -run TestResolve
```

Expected: FAIL — `ResolveHandler`, `ResolveConfig`, `ConnectionPlan` not defined.

- [ ] **Step 8: Write resolve implementation**

```go
// internal/cloud/connect/resolve.go
package connect

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ResolveConfig configures the resolve endpoint.
type ResolveConfig struct {
	Registry     *Registry
	RelayBaseURL string
	STUNServers  []string
}

// ConnectionPlan is returned to clients resolving a site alias.
type ConnectionPlan struct {
	SiteID      string              `json:"site_id"`
	SiteAlias   string              `json:"site_alias"`
	Status      string              `json:"status"`
	PublicIP    string              `json:"public_ip,omitempty"`
	LANCIDRs    []string            `json:"lan_cidrs,omitempty"`
	Endpoints   []PlanEndpoint      `json:"endpoints"`
	STUNServers []string            `json:"stun_servers,omitempty"`
}

// PlanEndpoint is one candidate connectivity method.
type PlanEndpoint struct {
	Kind               string `json:"kind"`     // "lan", "direct", "relay"
	URL                string `json:"url"`
	Priority           int    `json:"priority"` // lower = try first
	EstimatedLatencyMS int    `json:"estimated_latency_ms"`
}

// ResolveHandler returns an http.Handler that resolves a site alias
// into a connection plan.
func ResolveHandler(cfg ResolveConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		tenantID := r.URL.Query().Get("tenant_id")
		alias := r.URL.Query().Get("alias")

		if tenantID == "" || alias == "" {
			http.Error(w, `{"error":"tenant_id and alias are required"}`, http.StatusBadRequest)
			return
		}

		session, ok := cfg.Registry.LookupByAlias(tenantID, alias)
		if !ok {
			http.Error(w, `{"error":"site not found or offline"}`, http.StatusNotFound)
			return
		}

		plan := ConnectionPlan{
			SiteID:      session.SiteID,
			SiteAlias:   session.SiteAlias,
			Status:      session.Status,
			PublicIP:    session.PublicIP,
			LANCIDRs:    session.LANCIDRs,
			STUNServers: cfg.STUNServers,
		}

		// Endpoint 1: LAN direct (lowest latency, only works on same network).
		for _, cidr := range session.LANCIDRs {
			plan.Endpoints = append(plan.Endpoints, PlanEndpoint{
				Kind:               "lan",
				URL:                fmt.Sprintf("https://%s:9997", cidrToHost(cidr)),
				Priority:           1,
				EstimatedLatencyMS: 5,
			})
		}

		// Endpoint 2: Direct via public IP (works if port forwarded or
		// STUN hole-punch succeeds).
		if session.PublicIP != "" {
			plan.Endpoints = append(plan.Endpoints, PlanEndpoint{
				Kind:               "direct",
				URL:                fmt.Sprintf("https://%s:8889", session.PublicIP),
				Priority:           2,
				EstimatedLatencyMS: 30,
			})
		}

		// Endpoint 3: Cloud relay (guaranteed, higher latency).
		if cfg.RelayBaseURL != "" {
			plan.Endpoints = append(plan.Endpoints, PlanEndpoint{
				Kind:               "relay",
				URL:                fmt.Sprintf("%s/session/%s", cfg.RelayBaseURL, session.SiteID),
				Priority:           3,
				EstimatedLatencyMS: 80,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(plan)
	})
}

// cidrToHost extracts the network address from a CIDR string as a rough
// LAN host hint. In production the Directory would advertise its actual
// LAN IP; this is a fallback.
func cidrToHost(cidr string) string {
	for i, c := range cidr {
		if c == '/' {
			return cidr[:i]
		}
	}
	return cidr
}
```

- [ ] **Step 9: Run test to verify it passes**

```bash
go test ./internal/cloud/connect/... -v -run TestResolve
```

Expected: all 2 tests PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/cloud/connect/resolve.go internal/cloud/connect/resolve_test.go
git commit -m "feat(connect): implement resolve endpoint for site alias → connection plan"
```

---

## Phase 4: Relay Server

### Task 8: Relay session and bidirectional proxy

**Files:**
- Create: `internal/cloud/relay/session.go`
- Create: `internal/cloud/relay/session_test.go`
- Create: `internal/cloud/relay/relay.go`
- Create: `internal/cloud/relay/relay_test.go`

The relay bridges two WebSocket connections — one from the Flutter client and one from the on-prem Directory — and pipes data bidirectionally.

- [ ] **Step 1: Write the failing test for relay session**

```go
// internal/cloud/relay/session_test.go
package relay

import (
	"sync"
	"testing"
	"time"
)

func TestSessionManagerCreateAndGet(t *testing.T) {
	sm := NewSessionManager()

	s := sm.Create("site-123", "client-456")
	if s.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if s.SiteID != "site-123" {
		t.Fatalf("site_id = %q, want %q", s.SiteID, "site-123")
	}

	got, ok := sm.Get(s.ID)
	if !ok {
		t.Fatal("expected to find session")
	}
	if got.ID != s.ID {
		t.Fatalf("id = %q, want %q", got.ID, s.ID)
	}
}

func TestSessionManagerExpiry(t *testing.T) {
	sm := NewSessionManager()
	sm.sessionTTL = 100 * time.Millisecond

	s := sm.Create("site-123", "client-456")

	time.Sleep(150 * time.Millisecond)
	sm.Cleanup()

	_, ok := sm.Get(s.ID)
	if ok {
		t.Fatal("expected session to be expired")
	}
}

func TestSessionManagerConcurrency(t *testing.T) {
	sm := NewSessionManager()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := sm.Create("site", "client")
			sm.Get(s.ID)
			sm.Remove(s.ID)
		}()
	}
	wg.Wait()
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/cloud/relay/... -v -run TestSessionManager
```

Expected: FAIL — package does not exist.

- [ ] **Step 3: Write session implementation**

```go
// internal/cloud/relay/session.go
package relay

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// RelaySession tracks a single relay tunnel between client and Directory.
type RelaySession struct {
	ID        string
	SiteID    string
	ClientID  string
	CreatedAt time.Time
	LastActive time.Time
}

// SessionManager tracks active relay sessions.
type SessionManager struct {
	mu         sync.RWMutex
	sessions   map[string]*RelaySession
	sessionTTL time.Duration
}

// NewSessionManager creates a session manager with 5-minute default TTL.
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions:   make(map[string]*RelaySession),
		sessionTTL: 5 * time.Minute,
	}
}

// Create a new relay session. Returns the session with a generated ID.
func (sm *SessionManager) Create(siteID, clientID string) RelaySession {
	id := generateID()
	s := &RelaySession{
		ID:         id,
		SiteID:     siteID,
		ClientID:   clientID,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
	}

	sm.mu.Lock()
	sm.sessions[id] = s
	sm.mu.Unlock()

	return *s
}

// Get returns a session by ID.
func (sm *SessionManager) Get(id string) (RelaySession, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	s, ok := sm.sessions[id]
	if !ok {
		return RelaySession{}, false
	}
	return *s, true
}

// Touch updates the last-active timestamp.
func (sm *SessionManager) Touch(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if s, ok := sm.sessions[id]; ok {
		s.LastActive = time.Now()
	}
}

// Remove deletes a session.
func (sm *SessionManager) Remove(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, id)
}

// Cleanup removes expired sessions.
func (sm *SessionManager) Cleanup() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	cutoff := time.Now().Add(-sm.sessionTTL)
	for id, s := range sm.sessions {
		if s.LastActive.Before(cutoff) {
			delete(sm.sessions, id)
		}
	}
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/cloud/relay/... -v -run TestSessionManager
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Write the failing test for relay handler**

```go
// internal/cloud/relay/relay_test.go
package relay

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRelayBridgesTwoConnections(t *testing.T) {
	sm := NewSessionManager()
	h := NewHandler(HandlerConfig{
		Sessions: sm,
		Logger:   slog.Default(),
	})

	srv := httptest.NewServer(h)
	defer srv.Close()

	wsBase := "ws" + strings.TrimPrefix(srv.URL, "http")

	// Create a session.
	session := sm.Create("site-123", "client-456")

	// Connect the "Directory" side.
	dirWS, _, err := websocket.DefaultDialer.Dial(wsBase+"/relay/"+session.ID+"/directory", nil)
	if err != nil {
		t.Fatalf("dir dial: %v", err)
	}
	defer dirWS.Close()

	// Connect the "Client" side.
	clientWS, _, err := websocket.DefaultDialer.Dial(wsBase+"/relay/"+session.ID+"/client", nil)
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}
	defer clientWS.Close()

	// Give relay a moment to pair.
	time.Sleep(50 * time.Millisecond)

	// Client sends a message.
	testMsg := []byte("hello from client")
	if err := clientWS.WriteMessage(websocket.BinaryMessage, testMsg); err != nil {
		t.Fatalf("client write: %v", err)
	}

	// Directory should receive it.
	dirWS.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := dirWS.ReadMessage()
	if err != nil {
		t.Fatalf("dir read: %v", err)
	}
	if string(data) != "hello from client" {
		t.Fatalf("dir received = %q, want %q", string(data), "hello from client")
	}

	// Directory sends a response.
	respMsg := []byte("hello from directory")
	if err := dirWS.WriteMessage(websocket.BinaryMessage, respMsg); err != nil {
		t.Fatalf("dir write: %v", err)
	}

	// Client should receive it.
	clientWS.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err = clientWS.ReadMessage()
	if err != nil {
		t.Fatalf("client read: %v", err)
	}
	if string(data) != "hello from directory" {
		t.Fatalf("client received = %q, want %q", string(data), "hello from directory")
	}
}

func TestRelayRejectsInvalidSession(t *testing.T) {
	sm := NewSessionManager()
	h := NewHandler(HandlerConfig{
		Sessions: sm,
		Logger:   slog.Default(),
	})

	srv := httptest.NewServer(h)
	defer srv.Close()

	wsBase := "ws" + strings.TrimPrefix(srv.URL, "http")

	_, resp, err := websocket.DefaultDialer.Dial(wsBase+"/relay/nonexistent/client", nil)
	if err == nil {
		t.Fatal("expected dial to fail")
	}
	if resp != nil && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

```bash
go test ./internal/cloud/relay/... -v -run TestRelay
```

Expected: FAIL — `NewHandler`, `HandlerConfig` not defined.

- [ ] **Step 7: Write relay handler implementation**

```go
// internal/cloud/relay/relay.go
package relay

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

var relayUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

// HandlerConfig configures the relay handler.
type HandlerConfig struct {
	Sessions *SessionManager
	Logger   *slog.Logger
}

// Handler serves relay WebSocket connections.
// Routes: /relay/{session_id}/directory and /relay/{session_id}/client
type Handler struct {
	cfg HandlerConfig

	mu    sync.Mutex
	pairs map[string]*relayPair
}

type relayPair struct {
	mu        sync.Mutex
	directory *websocket.Conn
	client    *websocket.Conn
	ready     chan struct{} // closed when both sides are connected
}

// NewHandler creates a relay handler.
func NewHandler(cfg HandlerConfig) *Handler {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Handler{
		cfg:   cfg,
		pairs: make(map[string]*relayPair),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse path: /relay/{session_id}/{side}
	path := strings.TrimPrefix(r.URL.Path, "/relay/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}

	sessionID := parts[0]
	side := parts[1] // "directory" or "client"

	if side != "directory" && side != "client" {
		http.Error(w, `{"error":"side must be directory or client"}`, http.StatusBadRequest)
		return
	}

	// Validate session exists.
	if _, ok := h.cfg.Sessions.Get(sessionID); !ok {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}

	ws, err := relayUpgrader.Upgrade(w, r, nil)
	if err != nil {
		h.cfg.Logger.Error("relay: upgrade failed", "error", err)
		return
	}

	pair := h.getOrCreatePair(sessionID)

	pair.mu.Lock()
	if side == "directory" {
		pair.directory = ws
	} else {
		pair.client = ws
	}
	bothReady := pair.directory != nil && pair.client != nil
	pair.mu.Unlock()

	if bothReady {
		select {
		case <-pair.ready:
		default:
			close(pair.ready)
		}
	}

	// Wait for both sides to connect.
	<-pair.ready

	pair.mu.Lock()
	dir := pair.directory
	cli := pair.client
	pair.mu.Unlock()

	h.cfg.Logger.Info("relay: bridging", "session", sessionID, "side", side)

	// Bridge: pipe data from this side to the other.
	var src, dst *websocket.Conn
	if side == "client" {
		src, dst = cli, dir
	} else {
		src, dst = dir, cli
	}

	// Pipe messages until one side disconnects.
	for {
		msgType, data, err := src.ReadMessage()
		if err != nil {
			break
		}
		h.cfg.Sessions.Touch(sessionID)
		if err := dst.WriteMessage(msgType, data); err != nil {
			break
		}
	}

	// Cleanup.
	h.mu.Lock()
	delete(h.pairs, sessionID)
	h.mu.Unlock()

	// Close the other side. Ignore errors — may already be closed.
	_ = dir.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	_ = cli.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	_ = dir.Close()
	_ = cli.Close()

	_ = io.Discard // suppress unused import if needed
}

func (h *Handler) getOrCreatePair(sessionID string) *relayPair {
	h.mu.Lock()
	defer h.mu.Unlock()

	if p, ok := h.pairs[sessionID]; ok {
		return p
	}
	p := &relayPair{ready: make(chan struct{})}
	h.pairs[sessionID] = p
	return p
}
```

- [ ] **Step 8: Run test to verify it passes**

```bash
go test ./internal/cloud/relay/... -v -run TestRelay
```

Expected: all 2 tests PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/cloud/relay/session.go internal/cloud/relay/session_test.go internal/cloud/relay/relay.go internal/cloud/relay/relay_test.go
git commit -m "feat(relay): implement bidirectional WebSocket relay with session management"
```

---

## Phase 5: Event Forwarder (On-Prem → Cloud)

### Task 9: Forward local events over the WSS channel

**Files:**
- Create: `internal/directory/cloudconnector/eventforwarder.go`
- Create: `internal/directory/cloudconnector/eventforwarder_test.go`

The event forwarder subscribes to local event sources (alert triggers, camera status changes, AI detections) and sends them to the cloud via the connector's `SendEvent` method.

- [ ] **Step 1: Write the failing test**

```go
// internal/directory/cloudconnector/eventforwarder_test.go
package cloudconnector

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEventForwarderQueues(t *testing.T) {
	// Use a mock sender that records events.
	var sent []EventPayload
	sender := func(e EventPayload) error {
		sent = append(sent, e)
		return nil
	}

	fwd := NewEventForwarder(sender, 100)

	fwd.Forward(EventPayload{
		Kind:      "alert",
		CameraID:  "cam-01",
		Timestamp: time.Now().UTC(),
		Data:      json.RawMessage(`{"severity":"high"}`),
	})

	fwd.Forward(EventPayload{
		Kind:      "camera_status",
		CameraID:  "cam-02",
		Timestamp: time.Now().UTC(),
		Data:      json.RawMessage(`{"state":"offline"}`),
	})

	// Drain the queue.
	fwd.Flush()

	if len(sent) != 2 {
		t.Fatalf("sent = %d, want 2", len(sent))
	}
	if sent[0].Kind != "alert" {
		t.Fatalf("sent[0].kind = %q, want %q", sent[0].Kind, "alert")
	}
	if sent[1].Kind != "camera_status" {
		t.Fatalf("sent[1].kind = %q, want %q", sent[1].Kind, "camera_status")
	}
}

func TestEventForwarderDropsWhenFull(t *testing.T) {
	sender := func(e EventPayload) error { return nil }
	fwd := NewEventForwarder(sender, 2)

	// Fill the buffer.
	fwd.Forward(EventPayload{Kind: "a", Timestamp: time.Now()})
	fwd.Forward(EventPayload{Kind: "b", Timestamp: time.Now()})

	// This should be dropped (buffer full), not block.
	fwd.Forward(EventPayload{Kind: "c", Timestamp: time.Now()})

	if fwd.Dropped() < 1 {
		t.Fatalf("dropped = %d, want >= 1", fwd.Dropped())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/directory/cloudconnector/... -v -run TestEventForwarder
```

Expected: FAIL — `NewEventForwarder` not defined.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/directory/cloudconnector/eventforwarder.go
package cloudconnector

import (
	"sync/atomic"
)

// EventSender sends an event to the cloud. Matches Connector.SendEvent.
type EventSender func(EventPayload) error

// EventForwarder buffers local events and forwards them via the cloud
// connector. Non-blocking — drops events if the buffer is full.
type EventForwarder struct {
	sender  EventSender
	ch      chan EventPayload
	dropped atomic.Int64
}

// NewEventForwarder creates a forwarder with the given buffer size.
func NewEventForwarder(sender EventSender, bufferSize int) *EventForwarder {
	return &EventForwarder{
		sender: sender,
		ch:     make(chan EventPayload, bufferSize),
	}
}

// Forward queues an event for sending. Non-blocking — drops if buffer is full.
func (f *EventForwarder) Forward(e EventPayload) {
	select {
	case f.ch <- e:
	default:
		f.dropped.Add(1)
	}
}

// Flush sends all buffered events synchronously. Used in tests.
func (f *EventForwarder) Flush() {
	for {
		select {
		case e := <-f.ch:
			_ = f.sender(e)
		default:
			return
		}
	}
}

// Dropped returns the number of events dropped due to buffer overflow.
func (f *EventForwarder) Dropped() int64 {
	return f.dropped.Load()
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/directory/cloudconnector/... -v -run TestEventForwarder
```

Expected: all 2 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/directory/cloudconnector/eventforwarder.go internal/directory/cloudconnector/eventforwarder_test.go
git commit -m "feat(cloudconnector): implement event forwarder with backpressure"
```

---

## Phase 6: Integration Test

### Task 10: End-to-end test — Directory → Broker → Resolve

**Files:**
- Create: `internal/cloud/connect/integration_test.go`

This test boots a broker, connects a fake Directory via the connector, then resolves the site alias and verifies the connection plan.

- [ ] **Step 1: Write the integration test**

```go
// internal/cloud/connect/integration_test.go
package connect

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/directory/cloudconnector"
)

func TestEndToEndConnectAndResolve(t *testing.T) {
	// 1. Set up registry and broker.
	reg := NewRegistry()
	broker := NewBroker(BrokerConfig{
		Registry: reg,
		Authenticate: func(token string) (string, bool) {
			if token == "site-token" {
				return "tenant-1", true
			}
			return "", false
		},
		RelayURL: "wss://relay.test.com",
		Logger:   slog.Default(),
	})

	// 2. Set up resolve handler.
	mux := http.NewServeMux()
	mux.Handle("/ws/directory", broker)
	mux.Handle("/connect/resolve", ResolveHandler(ResolveConfig{
		Registry:     reg,
		RelayBaseURL: "wss://relay.test.com",
		STUNServers:  []string{"stun:stun.test.com:19302"},
	}))

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// 3. Connect an on-prem Directory.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/directory"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	connector := cloudconnector.New(cloudconnector.Config{
		URL:   wsURL,
		Token: "site-token",
		Site: cloudconnector.SiteInfo{
			ID:    "site-abc",
			Alias: "my-home",
			LANCIDRs: []string{"192.168.1.0/24"},
			Capabilities: cloudconnector.Capabilities{
				Streams:  true,
				Playback: true,
			},
		},
		HeartbeatInterval: 30 * time.Second,
		Logger:            slog.Default(),
	})

	go connector.Run(ctx)

	// 4. Wait for registration.
	time.Sleep(500 * time.Millisecond)

	// 5. Resolve the site alias.
	resolveURL := srv.URL + "/connect/resolve?tenant_id=tenant-1&alias=my-home"
	resp, err := http.Get(resolveURL)
	if err != nil {
		t.Fatalf("resolve request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var plan ConnectionPlan
	if err := json.NewDecoder(resp.Body).Decode(&plan); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if plan.SiteID != "site-abc" {
		t.Fatalf("site_id = %q, want %q", plan.SiteID, "site-abc")
	}
	if plan.Status != StatusOnline {
		t.Fatalf("status = %q, want %q", plan.Status, StatusOnline)
	}

	// Verify we got the expected endpoints.
	kinds := map[string]bool{}
	for _, ep := range plan.Endpoints {
		kinds[ep.Kind] = true
	}
	if !kinds["lan"] {
		t.Fatal("expected lan endpoint in plan")
	}
	if !kinds["relay"] {
		t.Fatal("expected relay endpoint in plan")
	}

	// Verify STUN servers are included.
	if len(plan.STUNServers) == 0 {
		t.Fatal("expected STUN servers in plan")
	}

	// 6. Verify disconnect removes from registry.
	cancel()
	time.Sleep(500 * time.Millisecond)

	_, ok := reg.LookupByAlias("tenant-1", "my-home")
	if ok {
		t.Fatal("expected site to be removed from registry after disconnect")
	}
}
```

- [ ] **Step 2: Run the integration test**

```bash
go test ./internal/cloud/connect/... -v -run TestEndToEnd -count=1
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/cloud/connect/integration_test.go
git commit -m "test(connect): add end-to-end integration test for connect-and-resolve flow"
```

---

## Summary

| Phase | Tasks | What It Delivers |
|-------|-------|------------------|
| **1: Wire Protocol** | Task 1 | Shared message types for WSS channel |
| **2: On-Prem Connector** | Tasks 2-4 | Directory dials cloud, registers, heartbeats, reconnects |
| **3: Cloud Broker** | Tasks 5-7 | Accepts Directory connections, resolves site aliases |
| **4: Relay** | Task 8 | Bidirectional WebSocket proxy for guaranteed connectivity |
| **5: Event Forwarder** | Task 9 | On-prem events flow to cloud services |
| **6: Integration** | Task 10 | End-to-end test proving the full flow |

After this plan is complete, the system supports:
- **Air-gapped**: `cloudConnectURL` is empty → nothing starts, fully local
- **Cloud-connected**: set the URL → Directory registers, cloud can relay, resolve, and receive events

The Flutter client changes (resolving site aliases, trying endpoints in order) are a follow-up plan once this backend is solid.
