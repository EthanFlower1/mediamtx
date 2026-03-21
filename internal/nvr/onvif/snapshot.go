package onvif

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	onvifmedia "github.com/use-go/onvif/media"
	sdkmedia "github.com/use-go/onvif/sdk/media"
	onviftypes "github.com/use-go/onvif/xsd/onvif"
)

// GetSnapshotURI queries the camera's Media service for its snapshot URI.
func GetSnapshotURI(xaddr, username, password, profileToken string) (string, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return "", err
	}
	if !client.HasService("media") {
		return "", fmt.Errorf("camera does not support media service")
	}
	ctx := context.Background()
	resp, err := sdkmedia.Call_GetSnapshotUri(ctx, client.Dev, onvifmedia.GetSnapshotUri{
		ProfileToken: onviftypes.ReferenceToken(profileToken),
	})
	if err != nil {
		return "", fmt.Errorf("get snapshot URI: %w", err)
	}
	uri := string(resp.MediaUri.Uri)
	if uri == "" {
		return "", fmt.Errorf("camera returned empty snapshot URI")
	}
	return uri, nil
}

// CaptureSnapshot fetches a JPEG snapshot from a camera's snapshot URI.
// If snapshotURI is provided (e.g. from ONVIF discovery), it is tried first.
// Otherwise the function falls back to guessing common snapshot URLs derived
// from the RTSP URL.
func CaptureSnapshot(rtspURL, username, password, outputDir, cameraID, snapshotURI string) (string, error) {
	// Try provided snapshot URI first (from ONVIF or cached).
	if snapshotURI != "" {
		path, err := downloadSnapshot(snapshotURI, username, password, outputDir, cameraID)
		if err == nil {
			return path, nil
		}
	}

	// Fall back to guessing common URLs.
	u, err := url.Parse(rtspURL)
	if err != nil {
		return "", fmt.Errorf("parse RTSP URL: %w", err)
	}
	host := u.Hostname()
	port := "80"

	// Common snapshot URLs for various camera brands.
	snapshotURLs := []string{
		fmt.Sprintf("http://%s:%s/cgi-bin/snapshot.cgi", host, port),                // Dahua/Amcrest
		fmt.Sprintf("http://%s:%s/cgi-bin/snapshot.cgi?channel=1", host, port),      // Dahua with channel
		fmt.Sprintf("http://%s:%s/snap.cgi", host, port),                            // Generic
		fmt.Sprintf("http://%s:%s/ISAPI/Streaming/channels/101/picture", host, port), // Hikvision
		fmt.Sprintf("http://%s:%s/onvif-http/snapshot", host, port),                  // ONVIF standard
		fmt.Sprintf("http://%s:%s/tmpfs/auto.jpg", host, port),                       // Some IP cameras
		fmt.Sprintf("http://%s:%s/image/jpeg.cgi", host, port),                       // Axis
	}

	for _, snapURL := range snapshotURLs {
		path, err := downloadSnapshot(snapURL, username, password, outputDir, cameraID)
		if err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no snapshot URL worked for %s", host)
}

// downloadSnapshot fetches a JPEG image from snapURL with optional basic-auth
// credentials, saves it to outputDir, and returns the file path.
func downloadSnapshot(snapURL, username, password, outputDir, cameraID string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequest("GET", snapURL, nil)
	if err != nil {
		return "", err
	}
	if username != "" {
		req.SetBasicAuth(username, password)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, snapURL)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && contentType != "image/jpeg" && contentType != "image/jpg" && contentType != "application/octet-stream" {
		return "", fmt.Errorf("unexpected content-type %q from %s", contentType, snapURL)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create thumbnail dir: %w", err)
	}

	idPrefix := cameraID
	if len(idPrefix) > 8 {
		idPrefix = idPrefix[:8]
	}
	filename := fmt.Sprintf("event_%s_%s.jpg", idPrefix, time.Now().Format("20060102-150405"))
	outputPath := filepath.Join(outputDir, filename)

	f, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("create thumbnail file: %w", err)
	}

	_, copyErr := io.Copy(f, io.LimitReader(resp.Body, 5<<20)) // 5MB max
	f.Close()

	if copyErr != nil {
		os.Remove(outputPath)
		return "", fmt.Errorf("write thumbnail: %w", copyErr)
	}

	return outputPath, nil
}
