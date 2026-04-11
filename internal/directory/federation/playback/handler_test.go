package playback

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fakeUserExtractor always returns userID="test-user".
func fakeUserExtractor(_ *http.Request) (string, bool) {
	return "test-user", true
}

// noUserExtractor simulates an unauthenticated request.
func noUserExtractor(_ *http.Request) (string, bool) {
	return "", false
}

func newTestDelegator(peerResp *kaivuev1.MintStreamURLResponse, peerErr error) *Delegator {
	catalog := &fakeCatalog{
		entries: map[string]CatalogEntry{
			"cam-remote-1": {CameraID: "cam-remote-1", PeerID: "peer-b", RecorderID: "rec-b-1"},
		},
	}
	peerClient := &fakePeerClient{
		mintResp: peerResp,
		mintErr:  peerErr,
	}
	factory := &fakePeerFactory{
		clients: map[string]*fakePeerClient{"peer-b": peerClient},
	}
	return NewDelegator(catalog, factory, discardLog)
}

func TestHandler_SuccessfulDelegation(t *testing.T) {
	expiresAt := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	d := newTestDelegator(&kaivuev1.MintStreamURLResponse{
		Url: "https://rec-b-1.peer-b.example.com/webrtc/cam-remote-1?token=signed",
		Claims: &kaivuev1.StreamClaims{
			CameraId:   "cam-remote-1",
			RecorderId: "rec-b-1",
			Kind:       1,
			ExpiresAt:  timestamppb.New(expiresAt),
		},
		GrantedKind: 1,
	}, nil)

	h := Handler(d, fakeUserExtractor)

	body := `{"camera_id":"cam-remote-1","requested_kind":1,"preferred_protocol":"webrtc"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/streams/request", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	var resp streamURLResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp.URL, "cam-remote-1")
	assert.Equal(t, uint32(1), resp.GrantedKind)
	assert.Equal(t, "peer-b", resp.PeerID)
	assert.NotEmpty(t, resp.ExpiresAt)
}

func TestHandler_Unauthenticated(t *testing.T) {
	d := newTestDelegator(nil, nil)
	h := Handler(d, noUserExtractor)

	body := `{"camera_id":"cam-remote-1","requested_kind":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/streams/request", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandler_WrongMethod(t *testing.T) {
	d := newTestDelegator(nil, nil)
	h := Handler(d, fakeUserExtractor)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/federation/streams/request", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandler_InvalidJSON(t *testing.T) {
	d := newTestDelegator(nil, nil)
	h := Handler(d, fakeUserExtractor)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/streams/request", strings.NewReader("not json"))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_MissingCameraID(t *testing.T) {
	d := newTestDelegator(nil, nil)
	h := Handler(d, fakeUserExtractor)

	body := `{"requested_kind":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/streams/request", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp["message"], "camera_id")
}

func TestHandler_MissingRequestedKind(t *testing.T) {
	d := newTestDelegator(nil, nil)
	h := Handler(d, fakeUserExtractor)

	body := `{"camera_id":"cam-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/streams/request", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp["message"], "requested_kind")
}

func TestHandler_InvalidProtocol(t *testing.T) {
	d := newTestDelegator(nil, nil)
	h := Handler(d, fakeUserExtractor)

	body := `{"camera_id":"cam-1","requested_kind":1,"preferred_protocol":"ftp"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/streams/request", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp["message"], "preferred_protocol")
}

func TestHandler_CameraNotFound_Returns404(t *testing.T) {
	// Use an empty catalog so no cameras resolve.
	catalog := &fakeCatalog{entries: map[string]CatalogEntry{}}
	factory := &fakePeerFactory{clients: map[string]*fakePeerClient{}}
	d := NewDelegator(catalog, factory, discardLog)
	h := Handler(d, fakeUserExtractor)

	body := `{"camera_id":"unknown","requested_kind":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/streams/request", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "CAMERA_NOT_FOUND", resp["code"])
}

func TestHandler_PeerUnreachable_Returns502(t *testing.T) {
	catalog := &fakeCatalog{
		entries: map[string]CatalogEntry{
			"cam-1": {CameraID: "cam-1", PeerID: "peer-gone", RecorderID: "rec-1"},
		},
	}
	factory := &fakePeerFactory{clients: map[string]*fakePeerClient{}}
	d := NewDelegator(catalog, factory, discardLog)
	h := Handler(d, fakeUserExtractor)

	body := `{"camera_id":"cam-1","requested_kind":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/streams/request", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadGateway, rec.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "PEER_UNREACHABLE", resp["code"])
}

func TestHandler_PermissionDenied_Returns403(t *testing.T) {
	catalog := &fakeCatalog{
		entries: map[string]CatalogEntry{
			"cam-1": {CameraID: "cam-1", PeerID: "peer-b", RecorderID: "rec-1"},
		},
	}
	peerClient := &fakePeerClient{
		mintErr: connect.NewError(connect.CodePermissionDenied, errors.New("not authorized")),
	}
	factory := &fakePeerFactory{
		clients: map[string]*fakePeerClient{"peer-b": peerClient},
	}
	del := NewDelegator(catalog, factory, discardLog)
	h := Handler(del, fakeUserExtractor)

	body := `{"camera_id":"cam-1","requested_kind":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/streams/request", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "PERMISSION_DENIED", resp["code"])
}

func TestHandler_PlaybackRange(t *testing.T) {
	expiresAt := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	d := newTestDelegator(&kaivuev1.MintStreamURLResponse{
		Url: "https://rec-b-1.peer-b.example.com/mp4/cam-remote-1?token=signed",
		Claims: &kaivuev1.StreamClaims{
			CameraId:   "cam-remote-1",
			RecorderId: "rec-b-1",
			Kind:       2,
			ExpiresAt:  timestamppb.New(expiresAt),
		},
		GrantedKind: 2,
	}, nil)

	h := Handler(d, fakeUserExtractor)

	body := `{"camera_id":"cam-remote-1","requested_kind":2,"preferred_protocol":"mp4","playback_start":"2026-04-10T00:00:00Z","playback_end":"2026-04-10T01:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/streams/request", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestHandler_InvalidPlaybackStart(t *testing.T) {
	d := newTestDelegator(nil, nil)
	h := Handler(d, fakeUserExtractor)

	body := `{"camera_id":"cam-1","requested_kind":2,"playback_start":"not-a-date","playback_end":"2026-04-10T01:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/streams/request", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
