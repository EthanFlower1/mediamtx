package audio

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os/exec"
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
	// uses the ffmpeg subprocess path. This allows tests to inject synthetic
	// audio without launching ffmpeg.
	readFunc func(ctx context.Context) (AudioFrame, error)

	// closeFunc is called on Close to release resources.
	closeFunc func() error

	// ffmpeg subprocess state (used when readFunc is nil).
	cmd    *exec.Cmd
	stdout io.ReadCloser
}

// ErrNotImplemented is returned when the audio capture backend is not available.
var ErrNotImplemented = fmt.Errorf("audio capture not implemented for this platform")

// ErrFFmpegNotFound is returned when the ffmpeg binary cannot be located.
var ErrFFmpegNotFound = fmt.Errorf("ffmpeg not found in PATH")

// ffmpegLookPath is the function used to find ffmpeg. Tests can override it.
var ffmpegLookPath = exec.LookPath

// ffmpegCommandContext builds the ffmpeg exec.Cmd. Tests can override it.
var ffmpegCommandContext = exec.CommandContext

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

// Open initializes the audio capture pipeline by launching ffmpeg as a
// subprocess that decodes the audio track to raw PCM float32 on stdout:
//
//	ffmpeg -i <rtsp_url> -vn -acodec pcm_f32le -ar 16000 -ac 1 -f f32le pipe:1
//
// If a readFunc was injected (e.g., for testing), Open returns immediately.
func (ac *AudioCapture) Open() error {
	// If we have an injected read function, we're ready.
	if ac.readFunc != nil {
		return nil
	}

	// Verify ffmpeg is available.
	if _, err := ffmpegLookPath("ffmpeg"); err != nil {
		return ErrFFmpegNotFound
	}

	// Launch ffmpeg. We use a long-lived background context; the caller
	// controls lifetime via Close().
	ac.cmd = ffmpegCommandContext(context.Background(),
		"ffmpeg",
		"-i", ac.streamURL,
		"-vn",
		"-acodec", "pcm_f32le",
		"-ar", fmt.Sprintf("%d", ac.sampleRate),
		"-ac", "1",
		"-f", "f32le",
		"pipe:1",
	)

	var err error
	ac.stdout, err = ac.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}

	if err := ac.cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start: %w", err)
	}

	return nil
}

// ReadFrame reads the next audio frame from the capture source.
// Each frame contains windowSize samples (1 second of audio at 16 kHz).
func (ac *AudioCapture) ReadFrame(ctx context.Context) (AudioFrame, error) {
	if ac.readFunc != nil {
		return ac.readFunc(ctx)
	}

	if ac.stdout == nil {
		return AudioFrame{}, ErrNotImplemented
	}

	// Each float32 sample is 4 bytes (little-endian).
	frameBytes := ac.windowSize * 4
	buf := make([]byte, frameBytes)

	// Read exactly one full frame from ffmpeg stdout.
	// Use a channel so we can respect context cancellation.
	type readResult struct {
		n   int
		err error
	}
	ch := make(chan readResult, 1)
	go func() {
		n, err := io.ReadFull(ac.stdout, buf)
		ch <- readResult{n, err}
	}()

	select {
	case <-ctx.Done():
		return AudioFrame{}, ctx.Err()
	case res := <-ch:
		if res.err != nil {
			if res.err == io.EOF || res.err == io.ErrUnexpectedEOF {
				return AudioFrame{}, io.EOF
			}
			return AudioFrame{}, fmt.Errorf("ffmpeg read: %w", res.err)
		}

		// Convert little-endian bytes to float32 samples.
		samples := make([]float32, ac.windowSize)
		for i := 0; i < ac.windowSize; i++ {
			bits := binary.LittleEndian.Uint32(buf[i*4 : i*4+4])
			samples[i] = math.Float32frombits(bits)
		}

		return AudioFrame{
			Samples:    samples,
			SampleRate: ac.sampleRate,
			Timestamp:  time.Now(),
		}, nil
	}
}

// Close releases capture resources by killing the ffmpeg process and
// closing the stdout pipe.
func (ac *AudioCapture) Close() error {
	if ac.closeFunc != nil {
		return ac.closeFunc()
	}

	if ac.cmd != nil && ac.cmd.Process != nil {
		// Kill the ffmpeg process.
		_ = ac.cmd.Process.Kill()

		// Close the stdout pipe so any blocked reads unblock.
		if ac.stdout != nil {
			_ = ac.stdout.Close()
		}

		// Wait for the process to exit to avoid zombies.
		_ = ac.cmd.Wait()

		ac.cmd = nil
		ac.stdout = nil
	}

	return nil
}
