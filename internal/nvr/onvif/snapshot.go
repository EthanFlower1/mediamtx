package onvif

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	resp, err := client.Dev.GetSnapshotURI(ctx, profileToken)
	if err != nil {
		return "", fmt.Errorf("get snapshot URI: %w", err)
	}
	if resp == nil || resp.URI == "" {
		return "", fmt.Errorf("camera returned empty snapshot URI")
	}
	return resp.URI, nil
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
		fmt.Sprintf("http://%s:%s/cgi-bin/snapshot.cgi", host, port),                 // Dahua/Amcrest
		fmt.Sprintf("http://%s:%s/cgi-bin/snapshot.cgi?channel=1", host, port),       // Dahua with channel
		fmt.Sprintf("http://%s:%s/snap.cgi", host, port),                             // Generic
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

// downloadSnapshot fetches a JPEG image from snapURL trying multiple auth
// methods (URL-embedded credentials, Basic, Digest, no-auth), saves it to
// outputDir, and returns the file path.
func downloadSnapshot(snapURL, username, password, outputDir, cameraID string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}

	// Try 1: URL with credentials embedded (works for many cameras)
	if username != "" {
		u, err := url.Parse(snapURL)
		if err == nil {
			u.User = url.UserPassword(username, password)
			if path, err := tryDownload(client, u.String(), "", "", outputDir, cameraID); err == nil {
				return path, nil
			}
		}
	}

	// Try 2: Basic auth
	if username != "" {
		if path, err := tryDownload(client, snapURL, username, password, outputDir, cameraID); err == nil {
			return path, nil
		}
	}

	// Try 3: Digest auth (challenge-response)
	if username != "" {
		if path, err := tryDownloadDigest(client, snapURL, username, password, outputDir, cameraID); err == nil {
			return path, nil
		}
	}

	// Try 4: No auth (some cameras allow anonymous snapshots)
	if path, err := tryDownload(client, snapURL, "", "", outputDir, cameraID); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("snapshot download failed for %s", snapURL)
}

// tryDownload attempts to download a snapshot using optional Basic auth.
func tryDownload(client *http.Client, snapURL, username, password, outputDir, cameraID string) (string, error) {
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

	return saveSnapshot(resp.Body, outputDir, cameraID)
}

// tryDownloadDigest attempts to download a snapshot using HTTP Digest auth.
// It performs the standard challenge-response handshake: first request to get
// the 401 + WWW-Authenticate header, then a second request with the computed
// Digest Authorization header.
func tryDownloadDigest(client *http.Client, snapURL, username, password, outputDir, cameraID string) (string, error) {
	// Step 1: Make initial request to get Digest challenge.
	resp, err := client.Get(snapURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	// Drain the body so the connection can be reused.
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusUnauthorized {
		return "", fmt.Errorf("expected 401 for digest challenge, got %d", resp.StatusCode)
	}

	authHeader := resp.Header.Get("WWW-Authenticate")
	if !strings.HasPrefix(authHeader, "Digest ") {
		return "", fmt.Errorf("not digest auth: %q", authHeader)
	}

	// Step 2: Parse the challenge and build the Digest Authorization header.
	u, err := url.Parse(snapURL)
	if err != nil {
		return "", err
	}
	digestValue := buildDigestAuth(username, password, "GET", u.RequestURI(), authHeader)

	req, err := http.NewRequest("GET", snapURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", digestValue)

	resp2, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		return "", fmt.Errorf("digest auth failed: HTTP %d", resp2.StatusCode)
	}

	return saveSnapshot(resp2.Body, outputDir, cameraID)
}

// saveSnapshot reads from r (expected JPEG data) and saves it to outputDir.
func saveSnapshot(r io.Reader, outputDir, cameraID string) (string, error) {
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

	_, copyErr := io.Copy(f, io.LimitReader(r, 5<<20)) // 5MB max
	f.Close()

	if copyErr != nil {
		os.Remove(outputPath)
		return "", fmt.Errorf("write thumbnail: %w", copyErr)
	}

	// Return a web-friendly path (e.g. "/thumbnails/event_*.jpg") so clients
	// can use it directly as a URL path without extra manipulation.
	return "/thumbnails/" + filename, nil
}

// buildDigestAuth constructs an HTTP Digest Authorization header value from the
// server's WWW-Authenticate challenge.
func buildDigestAuth(username, password, method, uri, challenge string) string {
	// Parse fields from the challenge header: Digest realm="...", nonce="...", ...
	fields := parseDigestChallenge(challenge)

	realm := fields["realm"]
	nonce := fields["nonce"]
	qop := fields["qop"]
	opaque := fields["opaque"]

	// Generate a client nonce.
	cnonce := fmt.Sprintf("%08x", rand.Uint32())
	nc := "00000001"

	// HA1 = MD5(username:realm:password)
	ha1 := md5Hex(fmt.Sprintf("%s:%s:%s", username, realm, password))

	// HA2 = MD5(method:uri)
	ha2 := md5Hex(fmt.Sprintf("%s:%s", method, uri))

	// Compute the response hash.
	var response string
	if qop == "auth" || qop == "auth-int" {
		// response = MD5(HA1:nonce:nc:cnonce:qop:HA2)
		response = md5Hex(fmt.Sprintf("%s:%s:%s:%s:%s:%s", ha1, nonce, nc, cnonce, qop, ha2))
	} else {
		// response = MD5(HA1:nonce:HA2) — legacy RFC 2069 style
		response = md5Hex(fmt.Sprintf("%s:%s:%s", ha1, nonce, ha2))
	}

	// Build the Authorization header value.
	parts := []string{
		fmt.Sprintf(`username="%s"`, username),
		fmt.Sprintf(`realm="%s"`, realm),
		fmt.Sprintf(`nonce="%s"`, nonce),
		fmt.Sprintf(`uri="%s"`, uri),
		fmt.Sprintf(`response="%s"`, response),
	}
	if qop != "" {
		parts = append(parts, fmt.Sprintf(`qop=%s`, qop))
		parts = append(parts, fmt.Sprintf(`nc=%s`, nc))
		parts = append(parts, fmt.Sprintf(`cnonce="%s"`, cnonce))
	}
	if opaque != "" {
		parts = append(parts, fmt.Sprintf(`opaque="%s"`, opaque))
	}

	return "Digest " + strings.Join(parts, ", ")
}

// parseDigestChallenge parses key=value pairs from a WWW-Authenticate: Digest header.
func parseDigestChallenge(header string) map[string]string {
	result := make(map[string]string)
	// Strip the "Digest " prefix.
	s := strings.TrimPrefix(header, "Digest ")

	// Split on commas, then parse key="value" pairs.
	for _, part := range splitDigestParts(s) {
		part = strings.TrimSpace(part)
		eqIdx := strings.IndexByte(part, '=')
		if eqIdx < 0 {
			continue
		}
		key := strings.TrimSpace(part[:eqIdx])
		val := strings.TrimSpace(part[eqIdx+1:])
		// Remove surrounding quotes if present.
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1 : len(val)-1]
		}
		result[key] = val
	}
	return result
}

// splitDigestParts splits a Digest header value on commas, respecting quoted strings.
func splitDigestParts(s string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			inQuote = !inQuote
			current.WriteByte(c)
		} else if c == ',' && !inQuote {
			parts = append(parts, current.String())
			current.Reset()
		} else {
			current.WriteByte(c)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// md5Hex returns the hex-encoded MD5 hash of s.
func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return fmt.Sprintf("%x", h)
}
