// internal/nvr/ai/frame_source.go
package ai

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// FrameSrc spawns an FFmpeg subprocess to decode RTSP frames and sends them
// to an output channel.
type FrameSrc struct {
	streamURL string
	width     int
	height    int
	out       chan Frame
}

// NewFrameSrc creates a new FrameSrc. If width/height are 0, they must be
// probed before calling Run.
func NewFrameSrc(streamURL string, width, height int, out chan Frame) *FrameSrc {
	return &FrameSrc{
		streamURL: streamURL,
		width:     width,
		height:    height,
		out:       out,
	}
}

// ProbeResolution uses ffprobe to determine stream resolution.
func ProbeResolution(streamURL string) (int, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "csv=p=0:s=x",
		"-rtsp_transport", "tcp",
		streamURL,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("ffprobe failed: %w", err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("ffprobe unexpected output: %q", string(out))
	}
	w, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("parse width: %w", err)
	}
	h, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("parse height: %w", err)
	}
	return w, h, nil
}

// Run starts the FFmpeg subprocess and reads frames until ctx is cancelled.
// It retries on failure with exponential backoff.
func (fs *FrameSrc) Run(ctx context.Context) {
	defer close(fs.out)

	backoff := 3 * time.Second
	maxBackoff := 30 * time.Second
	connected := false

	for {
		if ctx.Err() != nil {
			return
		}

		err := fs.readFrames(ctx)

		if ctx.Err() != nil {
			return
		}

		// Log state transition.
		if connected {
			log.Printf("[ai][%s] ffmpeg disconnected: %v", fs.streamURL, err)
			connected = false
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (fs *FrameSrc) readFrames(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-rtsp_transport", "tcp",
		"-i", fs.streamURL,
		"-f", "rawvideo",
		"-pix_fmt", "rgb24",
		"-an", "-sn",
		"-v", "quiet",
		"-",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	defer cmd.Wait() //nolint:errcheck

	log.Printf("[ai][%s] ffmpeg connected (%dx%d)", fs.streamURL, fs.width, fs.height)

	frameSize := fs.width * fs.height * 3
	buf := make([]byte, frameSize)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		_, err := io.ReadFull(stdout, buf)
		if err != nil {
			return fmt.Errorf("read frame: %w", err)
		}

		img := rgbToImage(buf, fs.width, fs.height)
		frame := Frame{
			Image:     img,
			Timestamp: time.Now(),
			Width:     fs.width,
			Height:    fs.height,
		}

		// Drop-oldest: if channel is full, drain old frame and send new.
		select {
		case fs.out <- frame:
		default:
			select {
			case <-fs.out:
			default:
			}
			fs.out <- frame
		}
	}
}

// rgbToImage converts raw RGB24 bytes to an image.NRGBA.
func rgbToImage(data []byte, width, height int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			off := (y*width + x) * 3
			img.SetNRGBA(x, y, color.NRGBA{
				R: data[off],
				G: data[off+1],
				B: data[off+2],
				A: 255,
			})
		}
	}
	return img
}
