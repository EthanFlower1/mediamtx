package integrity

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestScanner_RunsVerification(t *testing.T) {
	var callCount atomic.Int32

	s := &Scanner{
		Interval:  50 * time.Millisecond,
		BatchSize: 10,
		FetchFunc: func(cutoff time.Time, batchSize int) ([]ScanItem, error) {
			if callCount.Load() > 0 {
				return nil, nil
			}
			return []ScanItem{
				{
					RecordingID: 1,
					Info: RecordingInfo{
						FilePath: "/nonexistent/file.mp4",
						FileSize: 1000,
					},
				},
			}, nil
		},
		OnResult: func(recordingID int64, result VerificationResult) {
			callCount.Add(1)
			if result.Status != StatusCorrupted {
				t.Errorf("expected corrupted status, got %s", result.Status)
			}
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go s.Run(ctx)

	<-ctx.Done()
	time.Sleep(20 * time.Millisecond)

	if callCount.Load() == 0 {
		t.Error("expected at least one verification call")
	}
}

func TestScanner_RespectsContext(t *testing.T) {
	s := &Scanner{
		Interval:  1 * time.Hour,
		BatchSize: 10,
		FetchFunc: func(cutoff time.Time, batchSize int) ([]ScanItem, error) {
			return nil, nil
		},
		OnResult: func(recordingID int64, result VerificationResult) {},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Error("scanner did not stop after context cancellation")
	}
}
