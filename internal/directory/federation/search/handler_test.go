package search

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
	"github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1/kaivuev1connect"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupRouter(peers []Peer) *gin.Engine {
	r := gin.New()
	rg := r.Group("/api/v1")
	RegisterRoutes(rg, HandlerConfig{
		Peers: func() []Peer { return peers },
		SearchConfig: Config{
			PeerTimeout: 2 * time.Second,
			Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return r
}

func TestHandler_Success(t *testing.T) {
	srv := fakePeerServer([]*kaivuev1.RecordingHit{
		makeHit("cam-1", 100),
	}, 0, "")
	defer srv.Close()

	peers := []Peer{makePeerFromServer("p1", srv)}
	router := setupRouter(peers)

	body := `{"start_time":"1970-01-01T00:01:40Z","end_time":"1970-01-01T00:03:20Z"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/search/recordings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp searchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Partial {
		t.Error("expected partial=false")
	}
	if len(resp.Hits) != 1 {
		t.Errorf("expected 1 hit, got %d", len(resp.Hits))
	}
}

func TestHandler_PartialWithOfflinePeer(t *testing.T) {
	srvOnline := fakePeerServer([]*kaivuev1.RecordingHit{
		makeHit("cam-ok", 200),
	}, 0, "")
	defer srvOnline.Close()

	srvOffline := fakePeerServer(nil, 0, "")
	offlineURL := srvOffline.URL
	offlineClient := srvOffline.Client()
	srvOffline.Close()

	peers := []Peer{
		makePeerFromServer("online", srvOnline),
		{ID: "offline", Client: kaivuev1connect.NewFederationPeerServiceClient(offlineClient, offlineURL)},
	}
	router := setupRouter(peers)

	body := `{}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/search/recordings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp searchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Partial {
		t.Error("expected partial=true")
	}
	if len(resp.Hits) != 1 {
		t.Errorf("expected 1 hit, got %d", len(resp.Hits))
	}
	if _, ok := resp.PeerErrors["offline"]; !ok {
		t.Error("expected peer error for 'offline'")
	}
}

func TestHandler_BadRequest(t *testing.T) {
	router := setupRouter(nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/search/recordings", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
