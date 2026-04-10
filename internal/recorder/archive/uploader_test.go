package archive

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test doubles ---

type fakeObjectStore struct {
	mu        sync.Mutex
	objects   map[string]int64 // key → bytes
	uploadErr error
	deleteErr error
}

func newFakeObjectStore() *fakeObjectStore {
	return &fakeObjectStore{objects: make(map[string]int64)}
}

func (f *fakeObjectStore) Upload(_ context.Context, key string, r io.Reader, contentLength int64) (string, error) {
	if f.uploadErr != nil {
		return "", f.uploadErr
	}
	f.mu.Lock()
	f.objects[key] = contentLength
	f.mu.Unlock()
	return fmt.Sprintf("etag-%s", key), nil
}

func (f *fakeObjectStore) Delete(_ context.Context, key string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.mu.Lock()
	delete(f.objects, key)
	f.mu.Unlock()
	return nil
}

func (f *fakeObjectStore) Exists(_ context.Context, key string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.objects[key]
	return ok, nil
}

func (f *fakeObjectStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.objects)
}

func testSegment(id string) SegmentInfo {
	return SegmentInfo{
		SegmentID: id,
		CameraID:  "cam-1",
		StartTime: time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 8, 12, 0, 30, 0, time.UTC),
		LocalPath: "/recordings/cam-1/" + id + ".mp4",
		Bytes:     1024,
		Codec:     "h264",
	}
}

// --- tests ---

func TestNewUploader_Success(t *testing.T) {
	u, err := NewUploader(newFakeObjectStore(), Config{RecorderID: "rec-1"})
	require.NoError(t, err)
	assert.NotNil(t, u)
}

func TestNewUploader_MissingStore(t *testing.T) {
	_, err := NewUploader(nil, Config{RecorderID: "rec-1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ObjectStore")
}

func TestNewUploader_MissingRecorderID(t *testing.T) {
	_, err := NewUploader(newFakeObjectStore(), Config{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "RecorderID")
}

func TestUpload_SingleSegment(t *testing.T) {
	store := newFakeObjectStore()
	var callbackResult UploadResult
	u, err := NewUploader(store, Config{
		RecorderID:   "rec-1",
		BucketPrefix: "tenant-abc/rec-1/",
		Workers:      1,
		OnUpload: func(r UploadResult) {
			callbackResult = r
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go u.Start(ctx)

	seg := testSegment("seg-001")
	assert.True(t, u.Enqueue(seg))

	// Wait for upload to complete.
	time.Sleep(50 * time.Millisecond)
	cancel()
	u.Stop()

	assert.Equal(t, 1, store.count())
	assert.Equal(t, "seg-001", callbackResult.SegmentID)
	assert.NoError(t, callbackResult.Error)
	assert.Contains(t, callbackResult.Key, "cam-1/2026/04/08/seg-001.mp4")

	stats := u.Stats()
	assert.Equal(t, int64(1), stats.Uploaded)
	assert.Equal(t, int64(0), stats.Failed)
	assert.Equal(t, int64(1024), stats.BytesTotal)
}

func TestUpload_MultipleSegments(t *testing.T) {
	store := newFakeObjectStore()
	var uploaded atomic.Int64

	u, err := NewUploader(store, Config{
		RecorderID:   "rec-1",
		BucketPrefix: "t/r/",
		Workers:      2,
		OnUpload:     func(r UploadResult) { uploaded.Add(1) },
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go u.Start(ctx)

	for i := 0; i < 5; i++ {
		u.Enqueue(testSegment(fmt.Sprintf("seg-%03d", i)))
	}

	time.Sleep(100 * time.Millisecond)
	cancel()
	u.Stop()

	assert.Equal(t, int64(5), uploaded.Load())
	assert.Equal(t, 5, store.count())
}

func TestUpload_FailedUpload(t *testing.T) {
	store := newFakeObjectStore()
	store.uploadErr = fmt.Errorf("network error")

	var callbackResult UploadResult
	u, err := NewUploader(store, Config{
		RecorderID: "rec-1",
		Workers:    1,
		OnUpload:   func(r UploadResult) { callbackResult = r },
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go u.Start(ctx)

	u.Enqueue(testSegment("seg-fail"))
	time.Sleep(50 * time.Millisecond)
	cancel()
	u.Stop()

	assert.Error(t, callbackResult.Error)
	assert.Contains(t, callbackResult.Error.Error(), "network error")

	stats := u.Stats()
	assert.Equal(t, int64(0), stats.Uploaded)
	assert.Equal(t, int64(1), stats.Failed)
}

func TestUpload_IdempotentSkip(t *testing.T) {
	store := newFakeObjectStore()

	// Pre-populate the store with the expected key.
	seg := testSegment("seg-exists")
	key := fmt.Sprintf("%s/2026/04/08/seg-exists.mp4", seg.CameraID)
	store.objects[key] = 1024

	var callbackCalled atomic.Bool
	u, err := NewUploader(store, Config{
		RecorderID:   "rec-1",
		BucketPrefix: "",
		Workers:      1,
		OnUpload:     func(r UploadResult) { callbackCalled.Store(true) },
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go u.Start(ctx)

	u.Enqueue(seg)
	time.Sleep(50 * time.Millisecond)
	cancel()
	u.Stop()

	// Should have been counted as success (already exists).
	stats := u.Stats()
	assert.Equal(t, int64(1), stats.Uploaded)
}

func TestEnqueue_QueueFull(t *testing.T) {
	store := newFakeObjectStore()
	u, err := NewUploader(store, Config{
		RecorderID: "rec-1",
		Workers:    1,
		QueueSize:  1,
	})
	require.NoError(t, err)

	// Fill the queue without starting workers.
	assert.True(t, u.Enqueue(testSegment("seg-1")))
	assert.False(t, u.Enqueue(testSegment("seg-2")))
}

func TestObjectKey_Format(t *testing.T) {
	u, _ := NewUploader(newFakeObjectStore(), Config{
		RecorderID:   "rec-1",
		BucketPrefix: "tenant-abc/rec-1/",
	})

	seg := SegmentInfo{
		SegmentID: "seg-xyz",
		CameraID:  "cam-5",
		StartTime: time.Date(2026, 1, 15, 8, 30, 0, 0, time.UTC),
	}
	key := u.objectKey(seg)
	assert.Equal(t, "tenant-abc/rec-1/cam-5/2026/01/15/seg-xyz.mp4", key)
}

func TestStats_Initial(t *testing.T) {
	u, _ := NewUploader(newFakeObjectStore(), Config{RecorderID: "rec-1"})
	stats := u.Stats()
	assert.Equal(t, int64(0), stats.Uploaded)
	assert.Equal(t, int64(0), stats.Failed)
	assert.Equal(t, int64(0), stats.BytesTotal)
	assert.Equal(t, 0, stats.QueueDepth)
}
