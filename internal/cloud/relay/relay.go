package relay

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

// HandlerConfig holds dependencies for the relay Handler.
type HandlerConfig struct {
	Sessions *SessionManager
	Logger   *slog.Logger
}

// Handler serves WebSocket relay endpoints. It implements http.Handler.
// Route pattern: /relay/{session_id}/{side} where side is "directory" or "client".
type Handler struct {
	cfg   HandlerConfig
	mu    sync.Mutex
	pairs map[string]*relayPair
}

type relayPair struct {
	mu        sync.Mutex
	directory *websocket.Conn
	client    *websocket.Conn
	ready     chan struct{} // closed when both sides are connected
}

// NewHandler creates a relay Handler.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		cfg:   cfg,
		pairs: make(map[string]*relayPair),
	}
}

// ServeHTTP routes incoming requests to the relay logic.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse: /relay/{session_id}/{side}
	path := strings.TrimPrefix(r.URL.Path, "/relay/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	sessionID := parts[0]
	side := parts[1]

	if side != "directory" && side != "client" {
		http.Error(w, "side must be 'directory' or 'client'", http.StatusBadRequest)
		return
	}

	// Validate session exists
	if _, ok := h.cfg.Sessions.Get(sessionID); !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.cfg.Logger.Error("websocket upgrade failed", "err", err)
		return
	}

	// Get or create relay pair
	pair := h.getOrCreatePair(sessionID)

	// Assign the connection to the correct side
	pair.mu.Lock()
	switch side {
	case "directory":
		pair.directory = conn
	case "client":
		pair.client = conn
	}
	bothReady := pair.directory != nil && pair.client != nil
	pair.mu.Unlock()

	if bothReady {
		close(pair.ready)
	}

	// Wait for both sides to connect
	<-pair.ready

	// Determine src and dst
	pair.mu.Lock()
	var src, dst *websocket.Conn
	if side == "client" {
		src, dst = pair.client, pair.directory
	} else {
		src, dst = pair.directory, pair.client
	}
	pair.mu.Unlock()

	// Pipe messages from src to dst
	h.pipe(src, dst)

	// On disconnect, clean up
	h.removePair(sessionID)
	pair.mu.Lock()
	if pair.directory != nil {
		pair.directory.Close()
	}
	if pair.client != nil {
		pair.client.Close()
	}
	pair.mu.Unlock()
}

func (h *Handler) getOrCreatePair(sessionID string) *relayPair {
	h.mu.Lock()
	defer h.mu.Unlock()

	if p, ok := h.pairs[sessionID]; ok {
		return p
	}
	p := &relayPair{
		ready: make(chan struct{}),
	}
	h.pairs[sessionID] = p
	return p
}

func (h *Handler) removePair(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.pairs, sessionID)
}

// pipe reads messages from src and writes them to dst until an error occurs.
func (h *Handler) pipe(src, dst *websocket.Conn) {
	for {
		mt, msg, err := src.ReadMessage()
		if err != nil {
			return
		}
		if err := dst.WriteMessage(mt, msg); err != nil {
			return
		}
	}
}
