package authwebhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeVerifier is a test double for TokenVerifier.
type fakeVerifier struct {
	subject  string
	tenantID string
	err      error
}

func (f *fakeVerifier) Verify(_ context.Context, token string) (string, string, error) {
	if f.err != nil {
		return "", "", f.err
	}
	return f.subject, f.tenantID, nil
}

// fakeResolver is a test double for PathResolver.
type fakeResolver struct {
	cameras map[string][2]string // path → [cameraID, tenantID]
}

func (f *fakeResolver) ResolvePath(_ context.Context, path string) (string, string, error) {
	if pair, ok := f.cameras[path]; ok {
		return pair[0], pair[1], nil
	}
	return "", "", nil // not a managed camera
}

func startTestServer(t *testing.T, verifier TokenVerifier, resolver PathResolver) *Server {
	t.Helper()
	srv, err := New(ServerConfig{
		Verifier: verifier,
		Resolver: resolver,
		Log:      slog.Default(),
	})
	require.NoError(t, err)
	srv.Start()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	})
	return srv
}

func postAuth(t *testing.T, addr string, req AuthRequest) *http.Response {
	t.Helper()
	body, err := json.Marshal(req)
	require.NoError(t, err)

	resp, err := http.Post("http://"+addr+"/", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	return resp
}

func TestAuthorizedViewerSameTenant(t *testing.T) {
	verifier := &fakeVerifier{subject: "user-1", tenantID: "tenant-A"}
	resolver := &fakeResolver{cameras: map[string][2]string{
		"cam_abc": {"abc", "tenant-A"},
	}}

	srv := startTestServer(t, verifier, resolver)

	resp := postAuth(t, srv.Addr(), AuthRequest{
		Action: "read",
		Path:   "cam_abc",
		Token:  "valid-jwt",
		IP:     "192.168.1.10",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestTenantMismatchForbidden(t *testing.T) {
	verifier := &fakeVerifier{subject: "user-2", tenantID: "tenant-B"}
	resolver := &fakeResolver{cameras: map[string][2]string{
		"cam_xyz": {"xyz", "tenant-A"}, // camera belongs to tenant-A
	}}

	srv := startTestServer(t, verifier, resolver)

	resp := postAuth(t, srv.Addr(), AuthRequest{
		Action: "read",
		Path:   "cam_xyz",
		Token:  "valid-jwt-tenant-b",
		IP:     "192.168.1.11",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestInvalidTokenUnauthorized(t *testing.T) {
	verifier := &fakeVerifier{err: fmt.Errorf("token expired")}
	resolver := &fakeResolver{cameras: map[string][2]string{
		"cam_abc": {"abc", "tenant-A"},
	}}

	srv := startTestServer(t, verifier, resolver)

	resp := postAuth(t, srv.Addr(), AuthRequest{
		Action: "read",
		Path:   "cam_abc",
		Token:  "expired-jwt",
		IP:     "192.168.1.10",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestNoTokenUnauthorized(t *testing.T) {
	verifier := &fakeVerifier{subject: "user-1", tenantID: "tenant-A"}
	resolver := &fakeResolver{cameras: map[string][2]string{
		"cam_abc": {"abc", "tenant-A"},
	}}

	srv := startTestServer(t, verifier, resolver)

	resp := postAuth(t, srv.Addr(), AuthRequest{
		Action: "read",
		Path:   "cam_abc",
		IP:     "192.168.1.10",
		// no token
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestUnmanagedPathAllowed(t *testing.T) {
	verifier := &fakeVerifier{subject: "user-1", tenantID: "tenant-A"}
	resolver := &fakeResolver{cameras: map[string][2]string{}} // no cameras

	srv := startTestServer(t, verifier, resolver)

	resp := postAuth(t, srv.Addr(), AuthRequest{
		Action: "read",
		Path:   "system/healthz",
		Token:  "valid-jwt",
		IP:     "192.168.1.10",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestPublishActionAutoAllowed(t *testing.T) {
	verifier := &fakeVerifier{subject: "user-1", tenantID: "tenant-A"}
	resolver := &fakeResolver{cameras: map[string][2]string{
		"cam_abc": {"abc", "tenant-A"},
	}}

	srv := startTestServer(t, verifier, resolver)

	// "publish" is not in the default AllowedActions, so it auto-allows.
	resp := postAuth(t, srv.Addr(), AuthRequest{
		Action: "publish",
		Path:   "cam_abc",
		IP:     "192.168.1.10",
		// no token needed — publish from RTSP source is always local
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestTokenFromPassword(t *testing.T) {
	verifier := &fakeVerifier{subject: "user-1", tenantID: "tenant-A"}
	resolver := &fakeResolver{cameras: map[string][2]string{
		"cam_abc": {"abc", "tenant-A"},
	}}

	srv := startTestServer(t, verifier, resolver)

	// RTSP basic auth: user is empty, password carries the JWT.
	resp := postAuth(t, srv.Addr(), AuthRequest{
		Action:   "read",
		Path:     "cam_abc",
		Password: "jwt-in-password-field",
		IP:       "192.168.1.10",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestTokenFromQueryString(t *testing.T) {
	verifier := &fakeVerifier{subject: "user-1", tenantID: "tenant-A"}
	resolver := &fakeResolver{cameras: map[string][2]string{
		"cam_abc": {"abc", "tenant-A"},
	}}

	srv := startTestServer(t, verifier, resolver)

	resp := postAuth(t, srv.Addr(), AuthRequest{
		Action: "read",
		Path:   "cam_abc",
		Query:  "token=jwt-in-query&other=val",
		IP:     "192.168.1.10",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestPlaybackActionChecked(t *testing.T) {
	verifier := &fakeVerifier{err: fmt.Errorf("invalid")}
	resolver := &fakeResolver{cameras: map[string][2]string{
		"cam_abc": {"abc", "tenant-A"},
	}}

	srv := startTestServer(t, verifier, resolver)

	resp := postAuth(t, srv.Addr(), AuthRequest{
		Action: "playback",
		Path:   "cam_abc",
		Token:  "bad-token",
		IP:     "192.168.1.10",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestExtractQueryParam(t *testing.T) {
	require.Equal(t, "abc", extractQueryParam("token=abc&foo=bar", "token"))
	require.Equal(t, "bar", extractQueryParam("token=abc&foo=bar", "foo"))
	require.Equal(t, "", extractQueryParam("token=abc&foo=bar", "baz"))
	require.Equal(t, "", extractQueryParam("", "token"))
	require.Equal(t, "x", extractQueryParam("token=x", "token"))
}
