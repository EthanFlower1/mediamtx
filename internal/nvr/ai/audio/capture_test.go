package audio

import (
	"context"
	"testing"
	"time"
)

func TestAudioCapture_StubReturnsError(t *testing.T) {
	ac := NewAudioCapture("rtsp://test/stream")
	if err := ac.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer ac.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := ac.ReadFrame(ctx)
	if err == nil {
		t.Fatal("expected error from stub capture")
	}
	if err != ErrNotImplemented {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func TestAudioCapture_WithInjectedFunc(t *testing.T) {
	callCount := 0
	ac := NewAudioCaptureWithFunc(
		func(ctx context.Context) (AudioFrame, error) {
			callCount++
			return AudioFrame{
				Samples:    make([]float32, 16000),
				SampleRate: 16000,
				Timestamp:  time.Now(),
			}, nil
		},
		func() error { return nil },
	)

	if err := ac.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer ac.Close()

	ctx := context.Background()
	frame, err := ac.ReadFrame(ctx)
	if err != nil {
		t.Fatalf("ReadFrame() error: %v", err)
	}
	if len(frame.Samples) != 16000 {
		t.Errorf("expected 16000 samples, got %d", len(frame.Samples))
	}
	if frame.SampleRate != 16000 {
		t.Errorf("expected sample rate 16000, got %d", frame.SampleRate)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestAudioCapture_ContextCancellation(t *testing.T) {
	ac := NewAudioCapture("rtsp://test/stream")
	if err := ac.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer ac.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := ac.ReadFrame(ctx)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
