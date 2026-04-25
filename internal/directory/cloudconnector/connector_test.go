package cloudconnector

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// TestConnectorRegisters verifies the connector sends a register message and
// receives the cloud ack.
func TestConnectorRegisters(t *testing.T) {
	var gotRegister atomic.Value

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()

		// Read register message.
		var env Envelope
		if err := ws.ReadJSON(&env); err != nil {
			return
		}
		gotRegister.Store(env.Register)

		// Send ack.
		ack := Envelope{
			Type: MsgTypeRegistered,
			Registered: &RegisteredPayload{
				OK:       true,
				RelayURL: "https://relay.example.com",
			},
		}
		_ = ws.WriteJSON(ack)

		// Keep connection alive until test ends.
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c := New(Config{
		URL:               url,
		Token:             "test-token",
		Site:              SiteInfo{ID: "site-1", Alias: "My Site", Version: "1.0.0"},
		HeartbeatInterval: 10 * time.Second, // long so we don't get heartbeats
		Logger:            slog.Default(),
	})

	done := make(chan struct{})
	go func() {
		c.Run(ctx)
		close(done)
	}()

	// Wait for relay URL to be populated (indicates registration succeeded).
	require.Eventually(t, func() bool {
		return c.RelayURL() == "https://relay.example.com"
	}, 3*time.Second, 20*time.Millisecond)

	reg := gotRegister.Load().(*RegisterPayload)
	require.Equal(t, "site-1", reg.SiteID)
	require.Equal(t, "My Site", reg.SiteAlias)
	require.Equal(t, "1.0.0", reg.Version)

	cancel()
	<-done
}

// TestConnectorSendsHeartbeats verifies periodic heartbeats reach the server.
func TestConnectorSendsHeartbeats(t *testing.T) {
	var heartbeatCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()

		for {
			var env Envelope
			if err := ws.ReadJSON(&env); err != nil {
				return
			}
			switch env.Type {
			case MsgTypeRegister:
				_ = ws.WriteJSON(Envelope{
					Type:       MsgTypeRegistered,
					Registered: &RegisteredPayload{OK: true, RelayURL: "https://relay.example.com"},
				})
			case MsgTypeHeartbeat:
				heartbeatCount.Add(1)
			}
		}
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c := New(Config{
		URL:               url,
		Token:             "test-token",
		Site:              SiteInfo{ID: "site-1", Version: "1.0.0"},
		HeartbeatInterval: 100 * time.Millisecond,
		Logger:            slog.Default(),
	})

	done := make(chan struct{})
	go func() {
		c.Run(ctx)
		close(done)
	}()

	require.Eventually(t, func() bool {
		return heartbeatCount.Load() >= 2
	}, 3*time.Second, 50*time.Millisecond)

	cancel()
	<-done
}

// TestConnectorReconnectsOnClose verifies the connector reconnects after the
// server drops the connection.
func TestConnectorReconnectsOnClose(t *testing.T) {
	var connectCount atomic.Int32
	var mu sync.Mutex
	connections := make([]*websocket.Conn, 0)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		n := connectCount.Add(1)

		mu.Lock()
		connections = append(connections, ws)
		mu.Unlock()

		// Read register.
		var env Envelope
		if err := ws.ReadJSON(&env); err != nil {
			ws.Close()
			return
		}
		_ = ws.WriteJSON(Envelope{
			Type:       MsgTypeRegistered,
			Registered: &RegisteredPayload{OK: true, RelayURL: "https://relay.example.com"},
		})

		if n == 1 {
			// First connection: close immediately after registration to
			// trigger reconnect.
			time.Sleep(50 * time.Millisecond)
			ws.Close()
			return
		}

		// Subsequent connections: keep alive.
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				ws.Close()
				return
			}
		}
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c := New(Config{
		URL:               url,
		Token:             "test-token",
		Site:              SiteInfo{ID: "site-1", Version: "1.0.0"},
		HeartbeatInterval: 10 * time.Second,
		MinReconnectDelay: 50 * time.Millisecond,
		MaxReconnectDelay: 200 * time.Millisecond,
		Logger:            slog.Default(),
	})

	done := make(chan struct{})
	go func() {
		c.Run(ctx)
		close(done)
	}()

	require.Eventually(t, func() bool {
		return connectCount.Load() >= 2
	}, 5*time.Second, 50*time.Millisecond)

	cancel()
	<-done
}
