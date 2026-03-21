package onvif

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// CaptureSnapshot fetches a JPEG snapshot from a camera's snapshot URI.
// Most ONVIF cameras expose a snapshot URL that returns a JPEG image.
// Common patterns: /cgi-bin/snapshot.cgi, /ISAPI/Streaming/channels/1/picture, etc.
//
// This function tries common snapshot URLs for the camera and saves the first
// successful response as a JPEG file.
func CaptureSnapshot(rtspURL, username, password, outputDir, cameraID string) (string, error) {
	// Extract host from RTSP URL.
	u, err := url.Parse(rtspURL)
	if err != nil {
		return "", fmt.Errorf("parse RTSP URL: %w", err)
	}
	host := u.Hostname()
	port := "80"

	// Common snapshot URLs for various camera brands.
	snapshotURLs := []string{
		fmt.Sprintf("http://%s:%s/cgi-bin/snapshot.cgi", host, port),          // Dahua/Amcrest
		fmt.Sprintf("http://%s:%s/cgi-bin/snapshot.cgi?channel=1", host, port), // Dahua with channel
		fmt.Sprintf("http://%s:%s/snap.cgi", host, port),                       // Generic
		fmt.Sprintf("http://%s:%s/ISAPI/Streaming/channels/101/picture", host, port), // Hikvision
		fmt.Sprintf("http://%s:%s/onvif-http/snapshot", host, port),             // ONVIF standard
		fmt.Sprintf("http://%s:%s/tmpfs/auto.jpg", host, port),                  // Some IP cameras
		fmt.Sprintf("http://%s:%s/image/jpeg.cgi", host, port),                  // Axis
	}

	client := &http.Client{Timeout: 5 * time.Second}

	for _, snapURL := range snapshotURLs {
		req, err := http.NewRequest("GET", snapURL, nil)
		if err != nil {
			continue
		}
		if username != "" {
			req.SetBasicAuth(username, password)
		}

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode == http.StatusOK {
			contentType := resp.Header.Get("Content-Type")
			if contentType == "" || contentType == "image/jpeg" || contentType == "image/jpg" || contentType == "application/octet-stream" {
				// Save the snapshot.
				if err := os.MkdirAll(outputDir, 0o755); err != nil {
					resp.Body.Close()
					return "", fmt.Errorf("create thumbnail dir: %w", err)
				}

				filename := fmt.Sprintf("event_%s_%s.jpg", cameraID[:8], time.Now().Format("20060102-150405"))
				outputPath := filepath.Join(outputDir, filename)

				f, err := os.Create(outputPath)
				if err != nil {
					resp.Body.Close()
					return "", fmt.Errorf("create thumbnail file: %w", err)
				}

				_, copyErr := io.Copy(f, io.LimitReader(resp.Body, 5<<20)) // 5MB max
				f.Close()
				resp.Body.Close()

				if copyErr != nil {
					os.Remove(outputPath)
					return "", fmt.Errorf("write thumbnail: %w", copyErr)
				}

				return outputPath, nil
			}
		}
		resp.Body.Close()
	}

	return "", fmt.Errorf("no snapshot URL worked for %s", host)
}
