package audio

import (
	"context"
	"fmt"
	"time"
)

// AudioCapture extracts audio from an RTSP stream and delivers PCM frames.
//
// In production, this uses ffmpeg (via exec) to demux the audio track from
// the RTSP stream, decode to PCM float32, and deliver 1-second windows at
// 16 kHz mono. The implementation is structured for easy replacement with
// a native Go RTSP audio decoder when available.
type AudioCapture struct {
	streamURL  string
	sampleRate int
	windowSize int // samples per frame (1s = sampleRate)

	// readFunc is the pluggable read implementation. When nil, the capture
	// operates in stub mode (returns ErrNotImplemented). This allows tests
	// to inject synthetic audio without launching ffmpeg.
	readFunc func(ctx context.Context) (AudioFrame, error)

	// closeFunc is called on Close to release resources.
	closeFunc func() error
}

// ErrNotImplemented is returned when the audio capture backend is not available.
var ErrNotImplemented = fmt.Errorf("audio capture not implemented for this platform")

// NewAudioCapture creates a capture instance for the given RTSP stream URL.
func NewAudioCapture(streamURL string) *AudioCapture {
	return &AudioCapture{
		streamURL:  streamURL,
		sampleRate: 16000,
		windowSize: 16000, // 1 second at 16 kHz
	}
}

// NewAudioCaptureWithFunc creates a capture with an injected read function,
// primarily for testing.
func NewAudioCaptureWithFunc(
	readFunc func(ctx context.Context) (AudioFrame, error),
	closeFunc func() error,
) *AudioCapture {
	return &AudioCapture{
		sampleRate: 16000,
		windowSize: 16000,
		readFunc:   readFunc,
		closeFunc:  closeFunc,
	}
}

// Open initializes the audio capture pipeline (e.g., launches ffmpeg).
// In the production path this would exec:
//
//	ffmpeg -i <rtsp_url> -vn -acodec pcm_f32le -ar 16000 -ac 1 -f f32le pipe:1
//
// For now, Open succeeds and ReadFrame returns ErrNotImplemented unless
// a readFunc was injected.
func (ac *AudioCapture) Open() error {
	// If we have an injected read function, we're ready.
	if ac.readFunc != nil {
		return nil
	}
	// Production ffmpeg launch would go here.
	// For now, return nil so the pipeline can start; ReadFrame will
	// return ErrNotImplemented.
	return nil
}

// ReadFrame reads the next audio frame from the capture source.
// Each frame contains windowSize samples (1 second of audio).
func (ac *AudioCapture) ReadFrame(ctx context.Context) (AudioFrame, error) {
	if ac.readFunc != nil {
		return ac.readFunc(ctx)
	}
	// Stub: sleep briefly to avoid busy-loop, then error.
	select {
	case <-ctx.Done():
		return AudioFrame{}, ctx.Err()
	case <-time.After(100 * time.Millisecond):
		return AudioFrame{}, ErrNotImplemented
	}
}

// Close releases capture resources.
func (ac *AudioCapture) Close() error {
	if ac.closeFunc != nil {
		return ac.closeFunc()
	}
	return nil
}
