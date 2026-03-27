// internal/nvr/ai/publisher.go
package ai

import (
	"context"
	"image"
	"log"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// Publisher handles all output from tracked frames: WebSocket broadcast,
// database persistence, and CLIP embedding generation.
type Publisher struct {
	in         <-chan TrackedFrame
	cameraID   string
	cameraName string
	eventPub   EventPublisher
	database   *db.DB
	embedder   *Embedder // may be nil

	mu            sync.Mutex
	activeEventID int64
	lastStoredAt  map[int]time.Time // trackID → last DB insert time
}

// NewPublisher creates a new Publisher stage.
func NewPublisher(
	in <-chan TrackedFrame,
	cameraID, cameraName string,
	eventPub EventPublisher,
	database *db.DB,
	embedder *Embedder,
) *Publisher {
	return &Publisher{
		in:           in,
		cameraID:     cameraID,
		cameraName:   cameraName,
		eventPub:     eventPub,
		database:     database,
		embedder:     embedder,
		lastStoredAt: make(map[int]time.Time),
	}
}

// Run processes tracked frames until the input channel closes or ctx is cancelled.
func (pub *Publisher) Run(ctx context.Context) {
	var lastActivityAt time.Time
	motionGap := 8 * time.Second

	for {
		select {
		case <-ctx.Done():
			pub.closeEvent(time.Now())
			return

		case tf, ok := <-pub.in:
			if !ok {
				pub.closeEvent(time.Now())
				return
			}

			hasImportant := false
			for _, obj := range tf.Objects {
				if importantClasses[obj.Class] {
					hasImportant = true
					break
				}
			}

			// Broadcast detection_frame to WebSocket clients.
			pub.broadcastFrame(tf)

			// Handle object lifecycle events.
			for _, obj := range tf.Objects {
				switch obj.State {
				case ObjectEntered:
					if importantClasses[obj.Class] {
						pub.ensureEvent(obj, tf.Timestamp)
						pub.eventPub.PublishAIDetection(pub.cameraName, obj.Class, obj.Confidence)
					}
					pub.storeDetection(obj, tf.Timestamp, tf.Image)

				case ObjectActive:
					pub.maybeStoreDetection(obj, tf.Timestamp, tf.Image)

				case ObjectLeft:
					pub.storeDetection(obj, tf.Timestamp, nil)
					delete(pub.lastStoredAt, obj.TrackID)
				}
			}

			if hasImportant {
				lastActivityAt = tf.Timestamp
			}

			// Close event if no important activity for motionGap.
			if pub.activeEventID != 0 && !lastActivityAt.IsZero() &&
				tf.Timestamp.Sub(lastActivityAt) > motionGap {
				pub.closeEvent(tf.Timestamp)
			}
		}
	}
}

func (pub *Publisher) broadcastFrame(tf TrackedFrame) {
	if len(tf.Objects) == 0 {
		return
	}
	dets := make([]DetectionFrameData, 0, len(tf.Objects))
	for _, obj := range tf.Objects {
		if obj.State == ObjectLeft {
			continue // don't render left objects in overlay
		}
		dets = append(dets, DetectionFrameData{
			Class:      obj.Class,
			Confidence: obj.Confidence,
			TrackID:    obj.TrackID,
			X:          obj.Box.X,
			Y:          obj.Box.Y,
			W:          obj.Box.W,
			H:          obj.Box.H,
		})
	}
	if len(dets) > 0 {
		pub.eventPub.PublishDetectionFrame(pub.cameraName, dets)
	}
}

func (pub *Publisher) ensureEvent(obj TrackedObject, ts time.Time) {
	pub.mu.Lock()
	defer pub.mu.Unlock()

	if pub.activeEventID != 0 {
		return
	}

	event := &db.MotionEvent{
		CameraID:    pub.cameraID,
		StartedAt:   ts.UTC().Format("2006-01-02T15:04:05.000Z"),
		EventType:   "ai_detection",
		ObjectClass: obj.Class,
		Confidence:  float64(obj.Confidence),
	}
	if err := pub.database.InsertMotionEvent(event); err != nil {
		log.Printf("[ai][%s] insert motion event: %v", pub.cameraName, err)
		return
	}
	pub.activeEventID = event.ID
}

func (pub *Publisher) closeEvent(ts time.Time) {
	pub.mu.Lock()
	defer pub.mu.Unlock()

	if pub.activeEventID == 0 {
		return
	}
	endTime := ts.UTC().Format("2006-01-02T15:04:05.000Z")
	if err := pub.database.EndMotionEvent(pub.cameraID, endTime); err != nil {
		log.Printf("[ai][%s] end motion event: %v", pub.cameraName, err)
	}
	pub.activeEventID = 0
}

func (pub *Publisher) storeDetection(obj TrackedObject, ts time.Time, img image.Image) {
	pub.mu.Lock()
	eventID := pub.activeEventID
	pub.mu.Unlock()

	if eventID == 0 {
		return
	}

	det := &db.Detection{
		MotionEventID: eventID,
		FrameTime:     ts.UTC().Format("2006-01-02T15:04:05.000Z"),
		Class:         obj.Class,
		Confidence:    float64(obj.Confidence),
		BoxX:          float64(obj.Box.X),
		BoxY:          float64(obj.Box.Y),
		BoxW:          float64(obj.Box.W),
		BoxH:          float64(obj.Box.H),
	}

	// Generate CLIP embedding asynchronously for enter events.
	if img != nil && pub.embedder != nil && obj.State == ObjectEntered {
		go pub.generateEmbedding(det, img, obj.Box)
	}

	if err := pub.database.InsertDetection(det); err != nil {
		log.Printf("[ai][%s] insert detection: %v", pub.cameraName, err)
	}
	pub.lastStoredAt[obj.TrackID] = ts
}

func (pub *Publisher) maybeStoreDetection(obj TrackedObject, ts time.Time, img image.Image) {
	last, ok := pub.lastStoredAt[obj.TrackID]
	if ok && ts.Sub(last) < 2*time.Second {
		return
	}
	pub.storeDetection(obj, ts, img)
}

func (pub *Publisher) generateEmbedding(det *db.Detection, img image.Image, box BoundingBox) {
	crop := cropRegion(img, box)
	if crop == nil {
		return
	}
	embedding, err := pub.embedder.EncodeImage(crop)
	if err != nil {
		log.Printf("[ai] embedding error: %v", err)
		return
	}
	det.Embedding = float32SliceToBytes(embedding)
}

// cropRegion extracts a bounding box region from an image.
func cropRegion(img image.Image, box BoundingBox) image.Image {
	if img == nil {
		return nil
	}
	bounds := img.Bounds()
	x := int(float32(bounds.Dx()) * box.X)
	y := int(float32(bounds.Dy()) * box.Y)
	w := int(float32(bounds.Dx()) * box.W)
	h := int(float32(bounds.Dy()) * box.H)
	if w <= 0 || h <= 0 {
		return nil
	}
	rect := image.Rect(x, y, x+w, y+h).Intersect(bounds)
	if rect.Empty() {
		return nil
	}
	return cropImage(img, rect)
}
