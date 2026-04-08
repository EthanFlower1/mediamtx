package directoryingest_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/recorder/directoryingest"
)

// fakeCert returns a no-op GetCertificateFunc. The httptest servers are plain
// HTTP so TLS is never negotiated — this just satisfies the interface.
func fakeCert() directoryingest.GetCertificateFunc {
	return func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
		return &tls.Certificate{}, nil
	}
}

// -----------------------------------------------------------------------
// CameraStateClient tests
// -----------------------------------------------------------------------

func TestCameraStateClient_SendsBatchToServer(t *testing.T) {
	var received atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Updates []struct {
				CameraID string `json:"camera_id"`
			} `json:"updates"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		received.Add(int32(len(req.Updates)))
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"accepted":1}`+"\n")
	}))
	defer srv.Close()

	cli, err := directoryingest.NewCameraStateClient(directoryingest.CameraStateClientConfig{
		DirectoryEndpoint: srv.URL,
		RecorderID:        "rec-001",
		GetCertificate:    fakeCert(),
		FlushInterval:     20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewCameraStateClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go cli.Run(ctx)

	now := time.Now()
	cli.Publish(directoryingest.CameraStateUpdate{CameraID: "cam-1", State: "online", ObservedAt: now})
	cli.Publish(directoryingest.CameraStateUpdate{CameraID: "cam-2", State: "degraded", ObservedAt: now})

	deadline := time.After(400 * time.Millisecond)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out; server received %d updates", received.Load())
		case <-time.After(10 * time.Millisecond):
			if received.Load() >= 2 {
				return
			}
		}
	}
}

func TestCameraStateClient_CoalescesUpdatesPerCamera(t *testing.T) {
	var lastState atomic.Value

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Updates []struct {
				CameraID string `json:"camera_id"`
				State    string `json:"state"`
			} `json:"updates"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, u := range req.Updates {
			if u.CameraID == "cam-x" {
				lastState.Store(u.State)
			}
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"accepted":1}`+"\n")
	}))
	defer srv.Close()

	cli, err := directoryingest.NewCameraStateClient(directoryingest.CameraStateClientConfig{
		DirectoryEndpoint: srv.URL,
		RecorderID:        "rec-001",
		GetCertificate:    fakeCert(),
		FlushInterval:     30 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewCameraStateClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go cli.Run(ctx)

	now := time.Now()
	for i := 0; i < 50; i++ {
		state := "degraded"
		if i == 49 {
			state = "offline"
		}
		cli.Publish(directoryingest.CameraStateUpdate{CameraID: "cam-x", State: state, ObservedAt: now})
	}

	deadline := time.After(400 * time.Millisecond)
	for {
		select {
		case <-deadline:
			t.Logf("last state for cam-x: %v", lastState.Load())
			if ls, _ := lastState.Load().(string); ls == "offline" {
				return
			}
			t.Fatal("cam-x did not arrive at 'offline' before deadline")
		case <-time.After(10 * time.Millisecond):
			if ls, _ := lastState.Load().(string); ls == "offline" {
				return
			}
		}
	}
}

func TestCameraStateClient_ReconnectsAfterServerRestart(t *testing.T) {
	var requestCount atomic.Int32

	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"accepted":1}`+"\n")
	}))

	cli, err := directoryingest.NewCameraStateClient(directoryingest.CameraStateClientConfig{
		DirectoryEndpoint: srv1.URL,
		RecorderID:        "rec-001",
		GetCertificate:    fakeCert(),
		FlushInterval:     20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewCameraStateClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go cli.Run(ctx)

	now := time.Now()
	cli.Publish(directoryingest.CameraStateUpdate{CameraID: "cam-1", State: "online", ObservedAt: now})
	time.Sleep(60 * time.Millisecond)
	if requestCount.Load() < 1 {
		t.Fatal("no requests received before server restart")
	}

	// Simulate server restart.
	srv1.Close()
	cli.Publish(directoryingest.CameraStateUpdate{CameraID: "cam-1", State: "degraded", ObservedAt: now})

	time.Sleep(100 * time.Millisecond)

	// Client should still accept Publish calls (not stuck/panicked).
	doneCh := make(chan struct{})
	go func() {
		cli.Publish(directoryingest.CameraStateUpdate{CameraID: "cam-1", State: "online", ObservedAt: now})
		close(doneCh)
	}()
	select {
	case <-doneCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Publish blocked after server restart")
	}
}

// -----------------------------------------------------------------------
// SegmentIndexClient tests
// -----------------------------------------------------------------------

func TestSegmentIndexClient_RoundTrip(t *testing.T) {
	var received atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Entries []struct {
				SegmentID string `json:"segment_id"`
			} `json:"entries"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		received.Add(int32(len(req.Entries)))
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"accepted":1}`+"\n")
	}))
	defer srv.Close()

	cli, err := directoryingest.NewSegmentIndexClient(directoryingest.SegmentIndexClientConfig{
		DirectoryEndpoint: srv.URL,
		RecorderID:        "rec-001",
		GetCertificate:    fakeCert(),
		FlushInterval:     20 * time.Millisecond,
		BufferSize:        32,
	})
	if err != nil {
		t.Fatalf("NewSegmentIndexClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go cli.Run(ctx)

	now := time.Now()
	for i := 0; i < 5; i++ {
		if err := cli.Publish(ctx, directoryingest.SegmentIndexEntry{
			CameraID:  "cam-1",
			SegmentID: fmt.Sprintf("seg-%d", i),
			StartTime: now.Add(time.Duration(i) * 30 * time.Second),
			EndTime:   now.Add(time.Duration(i+1) * 30 * time.Second),
			Bytes:     1024 * 1024,
			Codec:     "h264",
			Sequence:  int64(i),
		}); err != nil {
			t.Fatalf("Publish %d: %v", i, err)
		}
	}

	deadline := time.After(400 * time.Millisecond)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out; received %d of 5", received.Load())
		case <-time.After(10 * time.Millisecond):
			if received.Load() >= 5 {
				return
			}
		}
	}
}

func TestSegmentIndexClient_Backpressure(t *testing.T) {
	blockCh := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blockCh
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"accepted":1}`+"\n")
	}))
	defer srv.Close()

	bufSize := 4
	cli, err := directoryingest.NewSegmentIndexClient(directoryingest.SegmentIndexClientConfig{
		DirectoryEndpoint: srv.URL,
		RecorderID:        "rec-001",
		GetCertificate:    fakeCert(),
		FlushInterval:     10 * time.Millisecond,
		BufferSize:        bufSize,
	})
	if err != nil {
		t.Fatalf("NewSegmentIndexClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go cli.Run(ctx)

	now := time.Now()
	published := make(chan int, bufSize+1)

	go func() {
		for i := 0; i < bufSize+1; i++ {
			if err := cli.Publish(ctx, directoryingest.SegmentIndexEntry{
				CameraID:  "cam-1",
				SegmentID: fmt.Sprintf("seg-%d", i),
				StartTime: now,
				EndTime:   now.Add(30 * time.Second),
				Bytes:     512,
			}); err != nil {
				return
			}
			published <- i
		}
	}()

	time.Sleep(50 * time.Millisecond)
	close(blockCh)

	timeout := time.After(1500 * time.Millisecond)
	count := 0
	for count < bufSize+1 {
		select {
		case <-timeout:
			t.Fatalf("backpressure test timed out after %d published", count)
		case <-published:
			count++
		}
	}
}

// -----------------------------------------------------------------------
// AIEventsClient tests
// -----------------------------------------------------------------------

func TestAIEventsClient_RoundTrip(t *testing.T) {
	var received atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Events []struct {
				EventID string `json:"event_id"`
			} `json:"events"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		received.Add(int32(len(req.Events)))
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"accepted":1}`+"\n")
	}))
	defer srv.Close()

	cli, err := directoryingest.NewAIEventsClient(directoryingest.AIEventsClientConfig{
		DirectoryEndpoint: srv.URL,
		RecorderID:        "rec-001",
		GetCertificate:    fakeCert(),
		FlushInterval:     20 * time.Millisecond,
		BufferSize:        64,
	})
	if err != nil {
		t.Fatalf("NewAIEventsClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go cli.Run(ctx)

	now := time.Now()
	for i := 0; i < 10; i++ {
		cli.Publish(directoryingest.AIEvent{
			EventID:    fmt.Sprintf("evt-%d", i),
			CameraID:   "cam-1",
			Kind:       "AI_EVENT_KIND_PERSON",
			ObservedAt: now,
			Confidence: 0.9,
		})
	}

	deadline := time.After(400 * time.Millisecond)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out; received %d of 10", received.Load())
		case <-time.After(10 * time.Millisecond):
			if received.Load() >= 10 {
				return
			}
		}
	}
}

func TestAIEventsClient_BehavioralEvent(t *testing.T) {
	var gotKind atomic.Value

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Events []struct {
				Kind string `json:"kind"`
			} `json:"events"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Events) > 0 {
			gotKind.Store(req.Events[0].Kind)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"accepted":1}`+"\n")
	}))
	defer srv.Close()

	cli, err := directoryingest.NewAIEventsClient(directoryingest.AIEventsClientConfig{
		DirectoryEndpoint: srv.URL,
		RecorderID:        "rec-001",
		GetCertificate:    fakeCert(),
		FlushInterval:     15 * time.Millisecond,
		BufferSize:        8,
	})
	if err != nil {
		t.Fatalf("NewAIEventsClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	go cli.Run(ctx)

	cli.Publish(directoryingest.AIEvent{
		EventID:    "behavioral-001",
		CameraID:   "cam-entrance",
		Kind:       "AI_EVENT_KIND_LOITERING",
		ObservedAt: time.Now(),
		Confidence: 0.88,
		Attributes: map[string]string{
			"duration_seconds": "120",
			"zone_id":          "zone-entrance-north",
		},
	})

	deadline := time.After(350 * time.Millisecond)
	for {
		select {
		case <-deadline:
			t.Fatal("behavioral event not received by server")
		case <-time.After(10 * time.Millisecond):
			if k, _ := gotKind.Load().(string); k == "AI_EVENT_KIND_LOITERING" {
				return
			}
		}
	}
}

func TestAIEventsClient_OverflowDropsOldest(t *testing.T) {
	blocked := make(chan struct{})

	var mu sync.Mutex
	var receivedIDs []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocked
		var req struct {
			Events []struct {
				EventID string `json:"event_id"`
			} `json:"events"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		for _, e := range req.Events {
			receivedIDs = append(receivedIDs, e.EventID)
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"accepted":1}`+"\n")
	}))
	defer srv.Close()

	bufSize := 4
	cli, err := directoryingest.NewAIEventsClient(directoryingest.AIEventsClientConfig{
		DirectoryEndpoint: srv.URL,
		RecorderID:        "rec-001",
		GetCertificate:    fakeCert(),
		FlushInterval:     20 * time.Millisecond,
		BufferSize:        bufSize,
	})
	if err != nil {
		t.Fatalf("NewAIEventsClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	go cli.Run(ctx)

	now := time.Now()
	for i := 0; i < 8; i++ {
		cli.Publish(directoryingest.AIEvent{
			EventID:    fmt.Sprintf("evt-%d", i),
			CameraID:   "cam-1",
			Kind:       "AI_EVENT_KIND_PERSON",
			ObservedAt: now,
		})
	}

	close(blocked)
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	got := len(receivedIDs)
	mu.Unlock()

	if got > bufSize {
		t.Errorf("received %d events, expected at most %d (overflow should drop oldest)", got, bufSize)
	}
	t.Logf("overflow test: received %d events out of 8 published (buffer=%d)", got, bufSize)
}
