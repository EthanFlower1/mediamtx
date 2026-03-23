package ai

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// AIPipeline processes video frames through YOLO detection and CLIP embedding.
// It manages detection lifecycle including motion event creation and detection
// storage.
type AIPipeline struct {
	cameraID string
	detector *Detector
	embedder *Embedder // may be nil if CLIP is not available
	db       *db.DB
	stopCh   chan struct{}

	// confThreshold is the minimum YOLO confidence to consider a detection.
	confThreshold float32
	// motionGap is the maximum time between detections before a motion event
	// is considered ended and a new one is started.
	motionGap time.Duration

	lastDetectionTime time.Time
	currentEventID    int64
}

// NewAIPipeline creates a new AI processing pipeline for the given camera.
// The embedder may be nil if CLIP models are not available; in that case,
// detections will be stored without embeddings.
func NewAIPipeline(cameraID string, detector *Detector, embedder *Embedder, database *db.DB) *AIPipeline {
	return &AIPipeline{
		cameraID:      cameraID,
		detector:      detector,
		embedder:      embedder,
		db:            database,
		stopCh:        make(chan struct{}),
		confThreshold: 0.5,
		motionGap:     30 * time.Second,
	}
}

// ProcessFrame runs YOLO detection on the image, optionally generates CLIP
// embeddings for each detected object, and stores the results in the database.
//
// For each detection above the confidence threshold:
//  1. Crop the bounding box region from the original image
//  2. Generate a CLIP embedding of the crop (if embedder available)
//  3. Find or create a motion event for this camera
//  4. Insert the detection record with embedding
func (p *AIPipeline) ProcessFrame(img image.Image, timestamp time.Time) error {
	select {
	case <-p.stopCh:
		return fmt.Errorf("pipeline stopped")
	default:
	}

	detections, err := p.detector.Detect(img, p.confThreshold)
	if err != nil {
		return fmt.Errorf("detection: %w", err)
	}

	if len(detections) == 0 {
		// No detections — check if we should close the current motion event.
		if p.currentEventID > 0 && !p.lastDetectionTime.IsZero() &&
			time.Since(p.lastDetectionTime) > p.motionGap {
			p.closeCurrentEvent(timestamp)
		}
		return nil
	}

	// Ensure we have an open motion event.
	if err := p.ensureMotionEvent(detections, timestamp); err != nil {
		return fmt.Errorf("motion event: %w", err)
	}

	p.lastDetectionTime = timestamp

	// Process each detection.
	bounds := img.Bounds()
	imgW := float64(bounds.Dx())
	imgH := float64(bounds.Dy())

	for _, det := range detections {
		// Crop the bounding box region from the original image.
		x := int(math.Round(float64(det.X) * imgW))
		y := int(math.Round(float64(det.Y) * imgH))
		w := int(math.Round(float64(det.W) * imgW))
		h := int(math.Round(float64(det.H) * imgH))

		// Clamp to image bounds.
		if x < bounds.Min.X {
			x = bounds.Min.X
		}
		if y < bounds.Min.Y {
			y = bounds.Min.Y
		}
		if x+w > bounds.Max.X {
			w = bounds.Max.X - x
		}
		if y+h > bounds.Max.Y {
			h = bounds.Max.Y - y
		}

		// Skip tiny crops.
		if w < 8 || h < 8 {
			continue
		}

		crop := cropImage(img, image.Rect(x, y, x+w, y+h))

		// Generate CLIP embedding if available.
		var embeddingBytes []byte
		if p.embedder != nil {
			embedding, err := p.embedder.EncodeImage(crop)
			if err == nil {
				embeddingBytes = float32SliceToBytes(embedding)
			}
			// Non-fatal: store detection without embedding on error.
		}

		// Insert detection.
		detection := &db.Detection{
			MotionEventID: p.currentEventID,
			FrameTime:     timestamp.UTC().Format("2006-01-02T15:04:05.000Z"),
			Class:         det.ClassName,
			Confidence:    float64(det.Confidence),
			BoxX:          float64(det.X),
			BoxY:          float64(det.Y),
			BoxW:          float64(det.W),
			BoxH:          float64(det.H),
			Embedding:     embeddingBytes,
		}

		if err := p.db.InsertDetection(detection); err != nil {
			return fmt.Errorf("insert detection: %w", err)
		}
	}

	return nil
}

// Run starts the AI pipeline's frame capture and inference loop.
// It captures JPEG snapshots from the camera at approximately the given FPS
// and runs detection on each frame. Blocks until Stop is called.
func (p *AIPipeline) Run(snapshotURL, username, password string, fps float64) {
	interval := time.Duration(float64(time.Second) / fps)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	client := &http.Client{Timeout: 5 * time.Second}

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			img, err := captureAndDecode(client, snapshotURL, username, password)
			if err != nil {
				// Silently skip failed captures to avoid log spam during transient
				// network issues. The ticker will retry on the next interval.
				continue
			}
			if err := p.ProcessFrame(img, time.Now()); err != nil {
				log.Printf("AI pipeline %s: process frame: %v", p.cameraID, err)
			}
		}
	}
}

// Stop signals the pipeline to stop processing.
func (p *AIPipeline) Stop() {
	select {
	case <-p.stopCh:
		// Already stopped.
	default:
		close(p.stopCh)
	}

	// Close the current motion event if open.
	if p.currentEventID > 0 {
		p.closeCurrentEvent(time.Now())
	}
}

// captureAndDecode fetches a JPEG snapshot from the camera's snapshot URL and
// decodes it to an image.Image. It tries multiple auth methods in order:
// URL-embedded credentials, Basic auth, Digest auth, and no auth.
func captureAndDecode(client *http.Client, snapshotURL, username, password string) (image.Image, error) {
	// Try 1: URL with credentials embedded.
	if username != "" {
		u, err := url.Parse(snapshotURL)
		if err == nil {
			u.User = url.UserPassword(username, password)
			if img, err := tryFetchAndDecode(client, u.String(), "", ""); err == nil {
				return img, nil
			}
		}
	}

	// Try 2: Basic auth.
	if username != "" {
		if img, err := tryFetchAndDecode(client, snapshotURL, username, password); err == nil {
			return img, nil
		}
	}

	// Try 3: Digest auth (challenge-response).
	if username != "" {
		if img, err := tryFetchAndDecodeDigest(client, snapshotURL, username, password); err == nil {
			return img, nil
		}
	}

	// Try 4: No auth.
	if img, err := tryFetchAndDecode(client, snapshotURL, "", ""); err == nil {
		return img, nil
	}

	return nil, fmt.Errorf("snapshot capture failed for %s", snapshotURL)
}

// tryFetchAndDecode fetches a URL with optional Basic auth and decodes the JPEG response.
func tryFetchAndDecode(client *http.Client, snapURL, username, password string) (image.Image, error) {
	req, err := http.NewRequest("GET", snapURL, nil)
	if err != nil {
		return nil, err
	}
	if username != "" {
		req.SetBasicAuth(username, password)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Limit to 10 MB to prevent memory exhaustion from misbehaving cameras.
	img, err := jpeg.Decode(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("decode JPEG: %w", err)
	}
	return img, nil
}

// tryFetchAndDecodeDigest performs HTTP Digest auth challenge-response and decodes the JPEG.
func tryFetchAndDecodeDigest(client *http.Client, snapURL, username, password string) (image.Image, error) {
	// Step 1: Initial request to get Digest challenge.
	resp, err := client.Get(snapURL)
	if err != nil {
		return nil, err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		return nil, fmt.Errorf("expected 401 for digest challenge, got %d", resp.StatusCode)
	}

	authHeader := resp.Header.Get("WWW-Authenticate")
	if !strings.HasPrefix(authHeader, "Digest ") {
		return nil, fmt.Errorf("not digest auth: %q", authHeader)
	}

	// Step 2: Build Digest Authorization header and retry.
	u, err := url.Parse(snapURL)
	if err != nil {
		return nil, err
	}
	digestValue := buildDigestAuthHeader(username, password, "GET", u.RequestURI(), authHeader)

	req, err := http.NewRequest("GET", snapURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", digestValue)

	resp2, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp2.Body)
		return nil, fmt.Errorf("digest auth failed: HTTP %d", resp2.StatusCode)
	}

	img, err := jpeg.Decode(io.LimitReader(resp2.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("decode JPEG: %w", err)
	}
	return img, nil
}

// buildDigestAuthHeader constructs an HTTP Digest Authorization header value from
// the server's WWW-Authenticate challenge. This mirrors the logic in
// onvif/snapshot.go but is kept local to avoid a circular dependency.
func buildDigestAuthHeader(username, password, method, uri, challenge string) string {
	fields := parseDigestFields(challenge)

	realm := fields["realm"]
	nonce := fields["nonce"]
	qop := fields["qop"]
	opaque := fields["opaque"]

	cnonce := fmt.Sprintf("%08x", rand.Uint32())
	nc := "00000001"

	ha1 := digestMD5Hex(fmt.Sprintf("%s:%s:%s", username, realm, password))
	ha2 := digestMD5Hex(fmt.Sprintf("%s:%s", method, uri))

	var response string
	if qop == "auth" || qop == "auth-int" {
		response = digestMD5Hex(fmt.Sprintf("%s:%s:%s:%s:%s:%s", ha1, nonce, nc, cnonce, qop, ha2))
	} else {
		response = digestMD5Hex(fmt.Sprintf("%s:%s:%s", ha1, nonce, ha2))
	}

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

// parseDigestFields parses key=value pairs from a WWW-Authenticate: Digest header.
func parseDigestFields(header string) map[string]string {
	result := make(map[string]string)
	s := strings.TrimPrefix(header, "Digest ")

	var current strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			inQuote = !inQuote
			current.WriteByte(c)
		} else if c == ',' && !inQuote {
			parseDigestField(current.String(), result)
			current.Reset()
		} else {
			current.WriteByte(c)
		}
	}
	if current.Len() > 0 {
		parseDigestField(current.String(), result)
	}
	return result
}

func parseDigestField(part string, result map[string]string) {
	part = strings.TrimSpace(part)
	eqIdx := strings.IndexByte(part, '=')
	if eqIdx < 0 {
		return
	}
	key := strings.TrimSpace(part[:eqIdx])
	val := strings.TrimSpace(part[eqIdx+1:])
	if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
		val = val[1 : len(val)-1]
	}
	result[key] = val
}

func digestMD5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return fmt.Sprintf("%x", h)
}

// ensureMotionEvent ensures there is an open motion event for the current
// detection batch. Creates a new one if needed.
func (p *AIPipeline) ensureMotionEvent(detections []YOLODetection, timestamp time.Time) error {
	// If we already have an event and it's not stale, keep using it.
	if p.currentEventID > 0 && !p.lastDetectionTime.IsZero() &&
		timestamp.Sub(p.lastDetectionTime) <= p.motionGap {
		return nil
	}

	// Close any previous event.
	if p.currentEventID > 0 {
		p.closeCurrentEvent(timestamp)
	}

	// Find the highest-confidence detection for the event metadata.
	best := detections[0]
	for _, d := range detections[1:] {
		if d.Confidence > best.Confidence {
			best = d
		}
	}

	event := &db.MotionEvent{
		CameraID:    p.cameraID,
		StartedAt:   timestamp.UTC().Format("2006-01-02T15:04:05.000Z"),
		EventType:   "ai_detection",
		ObjectClass: best.ClassName,
		Confidence:  float64(best.Confidence),
	}

	if err := p.db.InsertMotionEvent(event); err != nil {
		return err
	}

	p.currentEventID = event.ID
	return nil
}

// closeCurrentEvent ends the current motion event.
func (p *AIPipeline) closeCurrentEvent(timestamp time.Time) {
	endTime := timestamp.UTC().Format("2006-01-02T15:04:05.000Z")
	_ = p.db.EndMotionEvent(p.cameraID, endTime)
	p.currentEventID = 0
}

// cropImage extracts a sub-rectangle from an image.
func cropImage(img image.Image, rect image.Rectangle) image.Image {
	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	if si, ok := img.(subImager); ok {
		return si.SubImage(rect)
	}
	// Fallback: create a new RGBA image and draw the crop.
	cropped := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			cropped.Set(x-rect.Min.X, y-rect.Min.Y, img.At(x, y))
		}
	}
	return cropped
}

// float32SliceToBytes converts a float32 slice to a byte slice for DB storage.
func float32SliceToBytes(fs []float32) []byte {
	buf := make([]byte, len(fs)*4)
	for i, f := range fs {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// bytesToFloat32Slice converts a byte slice from DB storage back to float32 slice.
func bytesToFloat32Slice(b []byte) []float32 {
	if len(b) == 0 || len(b)%4 != 0 {
		return nil
	}
	fs := make([]float32, len(b)/4)
	for i := range fs {
		fs[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return fs
}
