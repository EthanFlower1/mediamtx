package gateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
	"github.com/bluenviron/mediamtx/internal/shared/streamclaims"
)

func sampleClaims() *streamclaims.StreamClaims {
	return &streamclaims.StreamClaims{
		UserID:      "u-1",
		TenantRef:   auth.TenantRef{Type: auth.TenantTypeCustomer, ID: "t-1"},
		CameraID:    "cam-front-door",
		RecorderID:  "rec-1",
		DirectoryID: "dir-1",
		Kind:        streamclaims.StreamKindLive,
		Protocol:    streamclaims.ProtocolWebRTC,
		Nonce:       "nonce-aaa",
		ExpiresAt:   time.Now().Add(2 * time.Minute),
	}
}

func sampleEndpoints() []RecorderEndpoint {
	return []RecorderEndpoint{
		{
			RecorderID: "rec-1",
			Host:       "recorder-rec-1",
			MediaPort:  8554,
			Scheme:     "rtsp",
			PathName:   "cam-front-door",
		},
	}
}

func newTestService(t *testing.T, v *fakeVerifier, r RecorderResolver, n NonceChecker) *Service {
	t.Helper()
	svc, err := NewService(context.Background(), Config{
		Listen:          "127.0.0.1:0",
		MediaMTXBaseURL: "http://127.0.0.1:8888",
		Verifier:        v,
		Resolver:        r,
		Nonce:           n,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func TestConfigValidate(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		ok   bool
	}{
		{"empty", Config{}, false},
		{"missing-listen", Config{MediaMTXBaseURL: "http://x", Verifier: &fakeVerifier{}, Resolver: newFakeResolver(nil)}, false},
		{"missing-mediamtx", Config{Listen: ":0", Verifier: &fakeVerifier{}, Resolver: newFakeResolver(nil)}, false},
		{"missing-verifier", Config{Listen: ":0", MediaMTXBaseURL: "http://x", Resolver: newFakeResolver(nil)}, false},
		{"missing-resolver", Config{Listen: ":0", MediaMTXBaseURL: "http://x", Verifier: &fakeVerifier{}}, false},
		{"ok", Config{Listen: ":0", MediaMTXBaseURL: "http://x", Verifier: &fakeVerifier{}, Resolver: newFakeResolver(nil)}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.cfg.Validate()
			if c.ok && err != nil {
				t.Fatalf("expected ok, got %v", err)
			}
			if !c.ok && err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestNewServiceListsEndpoints(t *testing.T) {
	svc := newTestService(t, &fakeVerifier{Result: sampleClaims()}, newFakeResolver(sampleEndpoints()), nil)
	got := svc.Endpoints()
	if len(got) != 1 || got[0].RecorderID != "rec-1" {
		t.Fatalf("unexpected endpoints: %+v", got)
	}
}

func TestNewServicePropagatesListError(t *testing.T) {
	_, err := NewService(context.Background(), Config{
		Listen:          ":0",
		MediaMTXBaseURL: "http://127.0.0.1:8888",
		Verifier:        &fakeVerifier{},
		Resolver:        errResolver{err: errBoom},
	})
	if err == nil {
		t.Fatalf("expected error from initial refresh")
	}
}

func TestHandleStream_Success(t *testing.T) {
	v := &fakeVerifier{Result: sampleClaims()}
	svc := newTestService(t, v, newFakeResolver(sampleEndpoints()), newFakeNonce())

	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	req.Header.Set("Authorization", "Bearer xyz")
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307, got %d body=%s", rr.Code, rr.Body.String())
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "/rec-1/cam-front-door") {
		t.Fatalf("location missing recorder/camera path: %q", loc)
	}
	if !strings.Contains(loc, "proto=webrtc") {
		t.Fatalf("location missing proto query: %q", loc)
	}
	if v.Calls != 1 {
		t.Fatalf("verifier should have been called once, got %d", v.Calls)
	}
}

func TestHandleStream_TokenViaQuery(t *testing.T) {
	svc := newTestService(t, &fakeVerifier{Result: sampleClaims()}, newFakeResolver(sampleEndpoints()), nil)
	req := httptest.NewRequest(http.MethodGet, "/stream?token=xyz", nil)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 for query-token path, got %d", rr.Code)
	}
}

func TestHandleStream_MissingToken(t *testing.T) {
	svc := newTestService(t, &fakeVerifier{Result: sampleClaims()}, newFakeResolver(sampleEndpoints()), nil)
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestHandleStream_VerifyError(t *testing.T) {
	svc := newTestService(t, &fakeVerifier{Err: errors.New("bad sig")}, newFakeResolver(sampleEndpoints()), nil)
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	req.Header.Set("Authorization", "Bearer xyz")
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestHandleStream_NonceReplay(t *testing.T) {
	svc := newTestService(t, &fakeVerifier{Result: sampleClaims()}, newFakeResolver(sampleEndpoints()), newFakeNonce())

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/stream", nil)
		req.Header.Set("Authorization", "Bearer xyz")
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if i == 0 && rr.Code != http.StatusTemporaryRedirect {
			t.Fatalf("first request should succeed, got %d", rr.Code)
		}
		if i == 1 && rr.Code != http.StatusUnauthorized {
			t.Fatalf("replay should be 401, got %d", rr.Code)
		}
	}
}

func TestHandleStream_CameraNotFound(t *testing.T) {
	c := sampleClaims()
	c.CameraID = "missing"
	svc := newTestService(t, &fakeVerifier{Result: c}, newFakeResolver(sampleEndpoints()), nil)
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	req.Header.Set("Authorization", "Bearer xyz")
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandleStream_ResolverError(t *testing.T) {
	// First refresh succeeds (use a fake), then swap to errResolver via
	// a wrapper that fails Resolve only.
	svc := newTestService(t, &fakeVerifier{Result: sampleClaims()},
		&splitResolver{
			list:    newFakeResolver(sampleEndpoints()),
			resolve: errResolver{err: errBoom},
		}, nil)
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	req.Header.Set("Authorization", "Bearer xyz")
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rr.Code)
	}
}

func TestMintUpstreamURL_PlaybackQuery(t *testing.T) {
	svc := newTestService(t, &fakeVerifier{Result: sampleClaims()}, newFakeResolver(sampleEndpoints()), nil)
	c := sampleClaims()
	c.Kind = streamclaims.StreamKindPlayback
	rng := streamclaims.TimeRange{
		Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC),
	}
	c.PlaybackRange = &rng

	got, err := svc.MintUpstreamURL(c, sampleEndpoints()[0])
	if err != nil {
		t.Fatalf("MintUpstreamURL: %v", err)
	}
	if !strings.Contains(got, "start=2026-01-01T00") || !strings.Contains(got, "end=2026-01-01T01") {
		t.Fatalf("playback range missing from url: %q", got)
	}
}

func TestRenderMediaMTXPaths(t *testing.T) {
	svc := newTestService(t, &fakeVerifier{Result: sampleClaims()}, newFakeResolver(sampleEndpoints()), nil)
	paths := svc.RenderMediaMTXPaths()
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}
	for k, v := range paths {
		if !strings.HasPrefix(k, "rec-1/") {
			t.Fatalf("path key should be tenant-scoped: %q", k)
		}
		m := v.(map[string]any)
		src := m["source"].(string)
		if !strings.Contains(src, "recorder-rec-1") || !strings.Contains(src, ":8554") {
			t.Fatalf("source url malformed: %q", src)
		}
	}
}

func TestHealthz(t *testing.T) {
	svc := newTestService(t, &fakeVerifier{Result: sampleClaims()}, newFakeResolver(sampleEndpoints()), nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

// splitResolver lets ListRecorders succeed while Resolve fails — used
// to test the 502 path without preventing service construction.
type splitResolver struct {
	list    RecorderResolver
	resolve RecorderResolver
}

func (s *splitResolver) Resolve(ctx context.Context, c *streamclaims.StreamClaims) (*RecorderEndpoint, error) {
	return s.resolve.Resolve(ctx, c)
}
func (s *splitResolver) ListRecorders(ctx context.Context) ([]RecorderEndpoint, error) {
	return s.list.ListRecorders(ctx)
}
