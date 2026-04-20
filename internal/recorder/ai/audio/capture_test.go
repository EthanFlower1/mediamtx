package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os/exec"
	"testing"
	"time"
)

func TestAudioCapture_ReadFrameWithoutOpenReturnsError(t *testing.T) {
	// When Open() has not been called (no stdout pipe), ReadFrame returns ErrNotImplemented.
	ac := NewAudioCapture("rtsp://test/stream")

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := ac.ReadFrame(ctx)
	if err == nil {
		t.Fatal("expected error from capture without Open()")
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
	// Use a pipe that blocks forever so we can test context cancellation.
	pr, pw := io.Pipe()
	defer pw.Close()

	ac := &AudioCapture{
		streamURL:  "rtsp://test/stream",
		sampleRate: 16000,
		windowSize: 16000,
		stdout:     pr,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := ac.ReadFrame(ctx)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestAudioCapture_FFmpeg_OpenFailsWhenNotFound(t *testing.T) {
	// Override lookup to simulate ffmpeg not being installed.
	origLookPath := ffmpegLookPath
	defer func() { ffmpegLookPath = origLookPath }()
	ffmpegLookPath = func(file string) (string, error) {
		return "", fmt.Errorf("not found")
	}

	ac := NewAudioCapture("rtsp://test/stream")
	err := ac.Open()
	if err != ErrFFmpegNotFound {
		t.Fatalf("expected ErrFFmpegNotFound, got %v", err)
	}
}

func TestAudioCapture_FFmpeg_ReadFrameFromPipe(t *testing.T) {
	// Build a buffer with known float32 samples in little-endian format.
	const numSamples = 16000
	var buf bytes.Buffer
	for i := 0; i < numSamples; i++ {
		val := float32(i) / float32(numSamples) // values in [0, 1)
		if err := binary.Write(&buf, binary.LittleEndian, val); err != nil {
			t.Fatal(err)
		}
	}

	// Create capture and inject a fake stdout pipe directly (bypassing Open).
	ac := &AudioCapture{
		streamURL:  "rtsp://test/stream",
		sampleRate: 16000,
		windowSize: 16000,
		stdout:     io.NopCloser(&buf),
	}

	ctx := context.Background()
	frame, err := ac.ReadFrame(ctx)
	if err != nil {
		t.Fatalf("ReadFrame() error: %v", err)
	}
	if len(frame.Samples) != numSamples {
		t.Fatalf("expected %d samples, got %d", numSamples, len(frame.Samples))
	}
	if frame.SampleRate != 16000 {
		t.Errorf("expected sample rate 16000, got %d", frame.SampleRate)
	}

	// Verify a few sample values.
	for _, idx := range []int{0, 100, 8000, 15999} {
		expected := float32(idx) / float32(numSamples)
		if math.Abs(float64(frame.Samples[idx]-expected)) > 1e-6 {
			t.Errorf("sample[%d] = %f, expected %f", idx, frame.Samples[idx], expected)
		}
	}
}

func TestAudioCapture_FFmpeg_ReadFrameEOF(t *testing.T) {
	// Empty reader simulates ffmpeg exiting with no output.
	ac := &AudioCapture{
		streamURL:  "rtsp://test/stream",
		sampleRate: 16000,
		windowSize: 16000,
		stdout:     io.NopCloser(bytes.NewReader(nil)),
	}

	ctx := context.Background()
	_, err := ac.ReadFrame(ctx)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestAudioCapture_FFmpeg_OpenSetsUpCommand(t *testing.T) {
	// Override lookup to succeed.
	origLookPath := ffmpegLookPath
	defer func() { ffmpegLookPath = origLookPath }()
	ffmpegLookPath = func(file string) (string, error) {
		return "/usr/bin/ffmpeg", nil
	}

	// Override command builder to capture args without launching a real process.
	origCmdCtx := ffmpegCommandContext
	defer func() { ffmpegCommandContext = origCmdCtx }()

	var capturedArgs []string
	ffmpegCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		capturedArgs = append([]string{name}, args...)
		// Return a command that will succeed (use "true" or "echo").
		return exec.CommandContext(ctx, "cat") // cat with no args reads stdin, StdoutPipe works
	}

	ac := NewAudioCapture("rtsp://example.com/stream1")
	err := ac.Open()
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer ac.Close()

	// Verify the ffmpeg arguments.
	expectedArgs := []string{
		"ffmpeg",
		"-i", "rtsp://example.com/stream1",
		"-vn",
		"-acodec", "pcm_f32le",
		"-ar", "16000",
		"-ac", "1",
		"-f", "f32le",
		"pipe:1",
	}
	if len(capturedArgs) != len(expectedArgs) {
		t.Fatalf("expected %d args, got %d: %v", len(expectedArgs), len(capturedArgs), capturedArgs)
	}
	for i, want := range expectedArgs {
		if capturedArgs[i] != want {
			t.Errorf("arg[%d] = %q, want %q", i, capturedArgs[i], want)
		}
	}
}

func TestAudioCapture_FFmpeg_CloseKillsProcess(t *testing.T) {
	// Override lookup to succeed.
	origLookPath := ffmpegLookPath
	defer func() { ffmpegLookPath = origLookPath }()
	ffmpegLookPath = func(file string) (string, error) {
		return "/usr/bin/ffmpeg", nil
	}

	// Use "sleep 60" as a long-running process we can kill.
	origCmdCtx := ffmpegCommandContext
	defer func() { ffmpegCommandContext = origCmdCtx }()
	ffmpegCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sleep", "60")
	}

	ac := NewAudioCapture("rtsp://example.com/stream1")
	err := ac.Open()
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	// Close should kill the process without hanging.
	done := make(chan error, 1)
	go func() {
		done <- ac.Close()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close() error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close() timed out — ffmpeg process not killed")
	}

	// Verify the cmd was cleaned up.
	if ac.cmd != nil {
		t.Error("expected cmd to be nil after Close()")
	}
}
