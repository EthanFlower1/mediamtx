package streams

import (
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

type fakeResolver struct {
	baseURL string
	path    string
	err     error
}

func (f *fakeResolver) Resolve(_ string) (string, string, error) {
	return f.baseURL, f.path, f.err
}

var testSecret = []byte("test-hmac-secret-for-stream-tokens-at-least-32b!")

// --- URLSigner tests ---

func TestURLSigner_SignAndVerify(t *testing.T) {
	signer := NewURLSigner(testSecret, 5*time.Minute)

	token, expiresAt := signer.Sign("cam-1/live", time.Now())
	assert.NotEmpty(t, token)
	assert.True(t, expiresAt.After(time.Now()))

	path, err := signer.Verify(token)
	require.NoError(t, err)
	assert.Equal(t, "cam-1/live", path)
}

func TestURLSigner_ExpiredToken(t *testing.T) {
	signer := NewURLSigner(testSecret, 1*time.Second)

	// Sign with a time in the past.
	token, _ := signer.Sign("cam-1/live", time.Now().Add(-2*time.Second))

	_, err := signer.Verify(token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestURLSigner_TamperedToken(t *testing.T) {
	signer := NewURLSigner(testSecret, 5*time.Minute)

	token, _ := signer.Sign("cam-1/live", time.Now())
	// Tamper with the signature portion.
	tampered := token[:len(token)-3] + "XXX"

	_, err := signer.Verify(tampered)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid signature")
}

func TestURLSigner_DifferentSecretFails(t *testing.T) {
	signer1 := NewURLSigner([]byte("secret-one"), 5*time.Minute)
	signer2 := NewURLSigner([]byte("secret-two"), 5*time.Minute)

	token, _ := signer1.Sign("cam-1/live", time.Now())

	_, err := signer2.Verify(token)
	assert.Error(t, err)
}

func TestURLSigner_MalformedToken(t *testing.T) {
	signer := NewURLSigner(testSecret, 5*time.Minute)

	_, err := signer.Verify("not-a-valid-token")
	assert.Error(t, err)
}

func TestURLSigner_UniqueTokensPerSign(t *testing.T) {
	signer := NewURLSigner(testSecret, 5*time.Minute)
	now := time.Now()

	t1, _ := signer.Sign("cam-1/live", now)
	t2, _ := signer.Sign("cam-1/live", now)

	// Random nonce makes tokens unique even for same path+time.
	assert.NotEqual(t, t1, t2)
}

// --- Handler tests ---

func TestHandler_RTSPRequest(t *testing.T) {
	resolver := &fakeResolver{baseURL: "recorder-1.local:8554", path: "cam-1/live"}
	signer := NewURLSigner(testSecret, 5*time.Minute)
	h := Handler(resolver, signer)

	body := `{"camera_id":"cam-1","protocol":"rtsp"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/streams/request", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	var resp StreamResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "rtsp", resp.Protocol)
	assert.Contains(t, resp.URL, "rtsp://recorder-1.local:8554/cam-1/live?token=")
	assert.NotEmpty(t, resp.Token)
	assert.NotEmpty(t, resp.ExpiresAt)
}

func TestHandler_WebRTCRequest(t *testing.T) {
	resolver := &fakeResolver{baseURL: "recorder-1.local", path: "cam-2/live"}
	signer := NewURLSigner(testSecret, 5*time.Minute)
	h := Handler(resolver, signer)

	body := `{"camera_id":"cam-2","protocol":"webrtc"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/streams/request", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	var resp StreamResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp.URL, "https://recorder-1.local/webrtc/cam-2/live?token=")
}

func TestHandler_HLSRequest(t *testing.T) {
	resolver := &fakeResolver{baseURL: "recorder-1.local", path: "cam-3/live"}
	signer := NewURLSigner(testSecret, 5*time.Minute)
	h := Handler(resolver, signer)

	body := `{"camera_id":"cam-3","protocol":"hls"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/streams/request", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	var resp StreamResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp.URL, "/hls/cam-3/live/index.m3u8?token=")
}

func TestHandler_MissingCameraID(t *testing.T) {
	h := Handler(&fakeResolver{}, NewURLSigner(testSecret, 5*time.Minute))

	body := `{"protocol":"rtsp"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/streams/request", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_InvalidProtocol(t *testing.T) {
	h := Handler(&fakeResolver{}, NewURLSigner(testSecret, 5*time.Minute))

	body := `{"camera_id":"cam-1","protocol":"ftp"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/streams/request", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp["message"], "invalid protocol")
}

func TestHandler_CameraNotFound(t *testing.T) {
	resolver := &fakeResolver{err: fmt.Errorf("not found")}
	h := Handler(resolver, NewURLSigner(testSecret, 5*time.Minute))

	body := `{"camera_id":"nonexistent","protocol":"rtsp"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/streams/request", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_WrongMethod(t *testing.T) {
	h := Handler(&fakeResolver{}, NewURLSigner(testSecret, 5*time.Minute))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/streams/request", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandler_InvalidJSON(t *testing.T) {
	h := Handler(&fakeResolver{}, NewURLSigner(testSecret, 5*time.Minute))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/streams/request", strings.NewReader("not json"))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
