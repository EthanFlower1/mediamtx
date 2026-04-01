// internal/nvr/ai/onvif_source.go
package ai

import (
	"context"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/onvif"
)

// ONVIFSrc subscribes to a camera's ONVIF metadata stream and converts
// parsed detections into the Detection type for merging with YOLO results.
type ONVIFSrc struct {
	metadataURL string
	username    string
	password    string
	// latestDets holds the most recent ONVIF detections, read by the
	// Detector stage when merging.
	latestDets []Detection
}

// NewONVIFSrc creates a new ONVIFSrc. Returns nil if metadataURL is empty.
func NewONVIFSrc(metadataURL, username, password string) *ONVIFSrc {
	if metadataURL == "" {
		return nil
	}
	return &ONVIFSrc{
		metadataURL: metadataURL,
		username:    username,
		password:    password,
	}
}

// LatestDetections returns the most recently parsed ONVIF detections.
func (os *ONVIFSrc) LatestDetections() []Detection {
	return os.latestDets
}

// Run connects to the metadata stream and parses frames until ctx is cancelled.
// Errors are logged but not retried — ONVIF metadata is supplementary.
func (os *ONVIFSrc) Run(ctx context.Context) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", os.metadataURL, nil)
	if err != nil {
		log.Printf("[ai][onvif] invalid metadata URL: %v", err)
		return
	}
	if os.username != "" {
		req.SetBasicAuth(os.username, os.password)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[ai][onvif] metadata connect failed: %v", err)
		return
	}
	defer resp.Body.Close()

	buf := make([]byte, 64*1024)
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := resp.Body.Read(buf)
		if n > 0 {
			frame, parseErr := onvif.ParseMetadataFrame(buf[:n])
			if parseErr == nil && frame != nil {
				os.latestDets = convertONVIFDetections(frame)
			}
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("[ai][onvif] metadata read error: %v", err)
			}
			return
		}
	}
}

func convertONVIFDetections(frame *onvif.MetadataFrame) []Detection {
	dets := make([]Detection, 0, len(frame.Objects))
	for _, obj := range frame.Objects {
		dets = append(dets, Detection{
			Class:      obj.Class,
			Confidence: float32(obj.Score),
			Box: BoundingBox{
				X: float32(obj.Box.Left),
				Y: float32(obj.Box.Top),
				W: float32(obj.Box.Right - obj.Box.Left),
				H: float32(obj.Box.Bottom - obj.Box.Top),
			},
			Source: SourceONVIF,
		})
	}
	return dets
}
