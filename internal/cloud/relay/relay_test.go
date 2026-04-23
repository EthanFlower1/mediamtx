package relay

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestRelayBridgesTwoConnections(t *testing.T) {
	sm := NewSessionManager()
	h := NewHandler(HandlerConfig{
		Sessions: sm,
		Logger:   slog.Default(),
	})

	srv := httptest.NewServer(h)
	defer srv.Close()

	sess := sm.Create("site-1", "client-1")

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	// Connect directory side
	dirConn, resp, err := websocket.DefaultDialer.Dial(wsURL+"/relay/"+sess.ID+"/directory", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	defer dirConn.Close()

	// Connect client side
	clientConn, resp, err := websocket.DefaultDialer.Dial(wsURL+"/relay/"+sess.ID+"/client", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	defer clientConn.Close()

	// Give the relay goroutines a moment to start piping
	time.Sleep(50 * time.Millisecond)

	// Client -> Directory
	err = clientConn.WriteMessage(websocket.TextMessage, []byte("hello from client"))
	require.NoError(t, err)

	dirConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	mt, msg, err := dirConn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, mt)
	require.Equal(t, "hello from client", string(msg))

	// Directory -> Client
	err = dirConn.WriteMessage(websocket.TextMessage, []byte("hello from directory"))
	require.NoError(t, err)

	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	mt, msg, err = clientConn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, mt)
	require.Equal(t, "hello from directory", string(msg))
}

func TestRelayRejectsInvalidSession(t *testing.T) {
	sm := NewSessionManager()
	h := NewHandler(HandlerConfig{
		Sessions: sm,
		Logger:   slog.Default(),
	})

	srv := httptest.NewServer(h)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	_, resp, err := websocket.DefaultDialer.Dial(wsURL+"/relay/nonexistent/client", nil)
	require.Error(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}
