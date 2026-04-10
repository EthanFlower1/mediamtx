package talkback

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test doubles ---

type fakeSender struct {
	codec     string
	openErr   error
	sendErr   error
	closeErr  error
	codecErr  error
	opened    bool
	closed    bool
	frames    [][]byte
}

func (f *fakeSender) Open(_ context.Context, _ string) error {
	f.opened = true
	return f.openErr
}

func (f *fakeSender) SendAudio(frame []byte) error {
	f.frames = append(f.frames, frame)
	return f.sendErr
}

func (f *fakeSender) Close() error {
	f.closed = true
	return f.closeErr
}

func (f *fakeSender) SupportedCodec(_ context.Context, _ string) (string, error) {
	return f.codec, f.codecErr
}

func testUserID(r *http.Request) (string, bool) {
	uid := r.Header.Get("X-User-ID")
	return uid, uid != ""
}

// --- Manager tests ---

func TestManager_StartAndStop(t *testing.T) {
	sender := &fakeSender{codec: "pcmu"}
	mgr := NewManager(sender, nil)

	sess, err := mgr.Start(context.Background(), StartRequest{CameraID: "cam-1", UserID: "u1"})
	require.NoError(t, err)
	assert.Equal(t, "cam-1", sess.CameraID)
	assert.Equal(t, "u1", sess.UserID)
	assert.True(t, sess.IsActive())
	assert.True(t, sender.opened)

	err = mgr.Stop("cam-1")
	require.NoError(t, err)

	// Give background goroutine time to close.
	time.Sleep(10 * time.Millisecond)
	assert.True(t, sender.closed)
}

func TestManager_DuplicateSessionRejected(t *testing.T) {
	sender := &fakeSender{codec: "pcmu"}
	mgr := NewManager(sender, nil)

	_, err := mgr.Start(context.Background(), StartRequest{CameraID: "cam-1", UserID: "u1"})
	require.NoError(t, err)

	_, err = mgr.Start(context.Background(), StartRequest{CameraID: "cam-1", UserID: "u2"})
	assert.ErrorIs(t, err, ErrSessionExists)
}

func TestManager_StopNonexistent(t *testing.T) {
	mgr := NewManager(&fakeSender{codec: "pcmu"}, nil)
	err := mgr.Stop("cam-999")
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestManager_BackchannelUnavailable(t *testing.T) {
	sender := &fakeSender{codecErr: fmt.Errorf("no backchannel")}
	mgr := NewManager(sender, nil)

	_, err := mgr.Start(context.Background(), StartRequest{CameraID: "cam-1", UserID: "u1"})
	assert.ErrorIs(t, err, ErrBackchannelUnavailable)
}

func TestManager_SendAudio(t *testing.T) {
	sender := &fakeSender{codec: "pcmu"}
	mgr := NewManager(sender, nil)

	_, err := mgr.Start(context.Background(), StartRequest{CameraID: "cam-1", UserID: "u1"})
	require.NoError(t, err)

	err = mgr.SendAudio("cam-1", []byte{0x80, 0x00})
	require.NoError(t, err)
	assert.Len(t, sender.frames, 1)
}

func TestManager_SendAudioNoSession(t *testing.T) {
	mgr := NewManager(&fakeSender{codec: "pcmu"}, nil)
	err := mgr.SendAudio("cam-1", []byte{0x80})
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestManager_ActiveSessions(t *testing.T) {
	sender := &fakeSender{codec: "pcmu"}
	mgr := NewManager(sender, nil)

	assert.Empty(t, mgr.ActiveSessions())

	_, _ = mgr.Start(context.Background(), StartRequest{CameraID: "cam-1", UserID: "u1"})
	_, _ = mgr.Start(context.Background(), StartRequest{CameraID: "cam-2", UserID: "u2"})

	sessions := mgr.ActiveSessions()
	assert.Len(t, sessions, 2)
}

func TestManager_EmptyCameraIDFails(t *testing.T) {
	mgr := NewManager(&fakeSender{codec: "pcmu"}, nil)
	_, err := mgr.Start(context.Background(), StartRequest{CameraID: "", UserID: "u1"})
	assert.Error(t, err)
}

// --- Handler tests ---

func TestStartHandler_Success(t *testing.T) {
	sender := &fakeSender{codec: "pcmu"}
	mgr := NewManager(sender, nil)
	h := StartHandler(mgr, testUserID)

	body := `{"camera_id":"cam-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/talkback/start", strings.NewReader(body))
	req.Header.Set("X-User-ID", "u1")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	var resp StartTalkbackResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "cam-1", resp.CameraID)
	assert.Equal(t, "pcmu", resp.Codec)
}

func TestStartHandler_Conflict(t *testing.T) {
	sender := &fakeSender{codec: "pcmu"}
	mgr := NewManager(sender, nil)
	_, _ = mgr.Start(context.Background(), StartRequest{CameraID: "cam-1", UserID: "u1"})

	h := StartHandler(mgr, testUserID)
	body := `{"camera_id":"cam-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/talkback/start", strings.NewReader(body))
	req.Header.Set("X-User-ID", "u2")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestStartHandler_NoAuth(t *testing.T) {
	h := StartHandler(NewManager(&fakeSender{codec: "pcmu"}, nil), testUserID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/talkback/start", strings.NewReader(`{"camera_id":"cam-1"}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestStopHandler_Success(t *testing.T) {
	sender := &fakeSender{codec: "pcmu"}
	mgr := NewManager(sender, nil)
	_, _ = mgr.Start(context.Background(), StartRequest{CameraID: "cam-1", UserID: "u1"})

	h := StopHandler(mgr)
	body := `{"camera_id":"cam-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/talkback/stop", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestStopHandler_NotFound(t *testing.T) {
	h := StopHandler(NewManager(&fakeSender{codec: "pcmu"}, nil))
	body := `{"camera_id":"cam-999"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/talkback/stop", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestListHandler_Empty(t *testing.T) {
	h := ListHandler(NewManager(&fakeSender{codec: "pcmu"}, nil))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/talkback/sessions", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string][]SessionInfo
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Empty(t, resp["sessions"])
}

func TestListHandler_WithSessions(t *testing.T) {
	sender := &fakeSender{codec: "pcmu"}
	mgr := NewManager(sender, nil)
	_, _ = mgr.Start(context.Background(), StartRequest{CameraID: "cam-1", UserID: "u1"})

	h := ListHandler(mgr)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/talkback/sessions", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string][]SessionInfo
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp["sessions"], 1)
}
