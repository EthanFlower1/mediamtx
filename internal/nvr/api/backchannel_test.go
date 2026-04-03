package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBackchannelInfoNoCameraID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := &BackchannelHandler{}
	router.GET("/cameras/:id/audio/backchannel/info", handler.Info)

	req := httptest.NewRequest(http.MethodGet, "/cameras/nonexistent/audio/backchannel/info", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 404 or 500, got %d", w.Code)
	}
}

func TestBackchannelWSMessageTypes(t *testing.T) {
	started := wsSessionStarted{
		Type:       "session_started",
		Codec:      "G711",
		SampleRate: 8000,
		Bitrate:    64000,
	}
	data, err := json.Marshal(started)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)
	if parsed["type"] != "session_started" {
		t.Fatalf("got %v", parsed["type"])
	}
	if parsed["codec"] != "G711" {
		t.Fatalf("got %v", parsed["codec"])
	}

	stopped := wsMessage{Type: "session_stopped"}
	data, _ = json.Marshal(stopped)

	errMsg := wsError{Type: "error", Message: "camera busy"}
	data, _ = json.Marshal(errMsg)
	json.Unmarshal(data, &parsed)
	if parsed["message"] != "camera busy" {
		t.Fatalf("got %v", parsed["message"])
	}

	_ = data
	_ = stopped
}
