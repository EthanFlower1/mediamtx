package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	"github.com/bluenviron/mediamtx/internal/cloud/audit/middleware"
)

func waitForRecord(t *testing.T, r audit.Recorder, tenant string, want int) []audit.Entry {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := r.Query(context.Background(), audit.QueryFilter{TenantID: tenant})
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(got) >= want {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %d entries (got %d)", want, len(got))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestMiddleware_AllowOn2xx(t *testing.T) {
	inner := audit.NewMemoryRecorder()
	mw := middleware.New(middleware.Config{
		Recorder: inner,
		Principal: func(req *http.Request) (middleware.Principal, bool) {
			return middleware.Principal{TenantID: "tenant-a", UserID: "alice", Agent: audit.AgentCloud}, true
		},
		Resolve: func(req *http.Request) (string, string, string) {
			return "cameras.add", "camera", "cam-1"
		},
	})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))

	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, httptest.NewRequest("POST", "/v1/cameras", nil))

	got := waitForRecord(t, inner, "tenant-a", 1)
	if got[0].Result != audit.ResultAllow {
		t.Errorf("want allow, got %s", got[0].Result)
	}
	_ = rw
}

func TestMiddleware_DenyOn403(t *testing.T) {
	inner := audit.NewMemoryRecorder()
	mw := middleware.New(middleware.Config{
		Recorder: inner,
		Principal: func(req *http.Request) (middleware.Principal, bool) {
			return middleware.Principal{TenantID: "tenant-a", UserID: "alice", Agent: audit.AgentCloud}, true
		},
		Resolve: func(req *http.Request) (string, string, string) {
			return "cameras.add", "camera", "cam-1"
		},
	})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(403) }))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/cameras", nil))

	got := waitForRecord(t, inner, "tenant-a", 1)
	if got[0].Result != audit.ResultDeny {
		t.Errorf("want deny, got %s", got[0].Result)
	}
}

func TestMiddleware_NoRecordOn500(t *testing.T) {
	inner := audit.NewMemoryRecorder()
	mw := middleware.New(middleware.Config{
		Recorder: inner,
		Principal: func(req *http.Request) (middleware.Principal, bool) {
			return middleware.Principal{TenantID: "tenant-a", UserID: "alice", Agent: audit.AgentCloud}, true
		},
		Resolve: func(req *http.Request) (string, string, string) {
			return "cameras.add", "camera", "cam-1"
		},
	})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/cameras", nil))

	// Give any stray goroutine a beat, then assert nothing was recorded.
	time.Sleep(20 * time.Millisecond)
	got, _ := inner.Query(context.Background(), audit.QueryFilter{TenantID: "tenant-a"})
	if len(got) != 0 {
		t.Errorf("middleware recorded on 500 (should not): %+v", got)
	}
}

func TestMiddleware_UnauthenticatedSkipsRecord(t *testing.T) {
	inner := audit.NewMemoryRecorder()
	mw := middleware.New(middleware.Config{
		Recorder: inner,
		Principal: func(req *http.Request) (middleware.Principal, bool) {
			return middleware.Principal{}, false
		},
		Resolve: func(req *http.Request) (string, string, string) {
			return "cameras.add", "camera", "cam-1"
		},
	})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/cameras", nil))

	time.Sleep(20 * time.Millisecond)
	got, _ := inner.Query(context.Background(), audit.QueryFilter{TenantID: "tenant-a"})
	if len(got) != 0 {
		t.Errorf("recorded unauthenticated request: %+v", got)
	}
}
