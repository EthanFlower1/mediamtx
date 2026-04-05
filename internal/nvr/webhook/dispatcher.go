// Package webhook implements detection webhook dispatch with retry logic.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// DetectionEvent is the payload sent to webhook endpoints.
type DetectionEvent struct {
	EventType  string          `json:"event_type"`
	CameraID   string          `json:"camera_id"`
	CameraName string          `json:"camera_name,omitempty"`
	Timestamp  string          `json:"timestamp"`
	Detection  *DetectionInfo  `json:"detection,omitempty"`
	Motion     *MotionInfo     `json:"motion,omitempty"`
}

// DetectionInfo contains detection details sent in webhook payloads.
type DetectionInfo struct {
	Class      string  `json:"class"`
	Confidence float64 `json:"confidence"`
	BoxX       float64 `json:"box_x"`
	BoxY       float64 `json:"box_y"`
	BoxW       float64 `json:"box_w"`
	BoxH       float64 `json:"box_h"`
	Attributes string  `json:"attributes,omitempty"`
}

// MotionInfo contains motion event details sent in webhook payloads.
type MotionInfo struct {
	EventID   int64  `json:"event_id"`
	StartedAt string `json:"started_at"`
	EndedAt   string `json:"ended_at,omitempty"`
}

// Dispatcher matches detection events against webhook configs and delivers them.
type Dispatcher struct {
	db     *db.DB
	client *http.Client

	ctx       context.Context
	ctxCancel context.CancelFunc
	wg        sync.WaitGroup

	// deliveryCh receives new deliveries that need sending.
	deliveryCh chan *deliveryJob
}

type deliveryJob struct {
	config   *db.WebhookConfig
	delivery *db.WebhookDelivery
	payload  []byte
}

// New creates a new webhook dispatcher. Call Start() to begin processing.
func New(database *db.DB) *Dispatcher {
	return &Dispatcher{
		db:         database,
		client:     &http.Client{},
		deliveryCh: make(chan *deliveryJob, 256),
	}
}

// Start begins the dispatcher goroutines.
func (d *Dispatcher) Start() {
	d.ctx, d.ctxCancel = context.WithCancel(context.Background())

	// Worker pool for sending webhooks.
	for i := 0; i < 4; i++ {
		d.wg.Add(1)
		go d.worker()
	}

	// Retry loop checks for pending retries periodically.
	d.wg.Add(1)
	go d.retryLoop()

	log.Println("[NVR] [webhook] dispatcher started")
}

// Stop shuts down the dispatcher gracefully.
func (d *Dispatcher) Stop() {
	if d.ctxCancel != nil {
		d.ctxCancel()
	}
	d.wg.Wait()
	log.Println("[NVR] [webhook] dispatcher stopped")
}

// OnDetection is called when a new detection occurs. It matches against
// enabled webhook configs and enqueues deliveries.
func (d *Dispatcher) OnDetection(cameraID, cameraName string, det *db.Detection) {
	d.dispatch(DetectionEvent{
		EventType:  "detection",
		CameraID:   cameraID,
		CameraName: cameraName,
		Timestamp:  det.FrameTime,
		Detection: &DetectionInfo{
			Class:      det.Class,
			Confidence: det.Confidence,
			BoxX:       det.BoxX,
			BoxY:       det.BoxY,
			BoxW:       det.BoxW,
			BoxH:       det.BoxH,
			Attributes: det.Attributes,
		},
	})
}

// OnMotionEvent is called when a motion event starts or ends.
func (d *Dispatcher) OnMotionEvent(cameraID, cameraName string, eventID int64, startedAt, endedAt string) {
	d.dispatch(DetectionEvent{
		EventType:  "motion",
		CameraID:   cameraID,
		CameraName: cameraName,
		Timestamp:  startedAt,
		Motion: &MotionInfo{
			EventID:   eventID,
			StartedAt: startedAt,
			EndedAt:   endedAt,
		},
	})
}

func (d *Dispatcher) dispatch(event DetectionEvent) {
	configs, err := d.db.ListEnabledWebhookConfigs()
	if err != nil {
		log.Printf("[NVR] [webhook] failed to list configs: %v", err)
		return
	}

	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("[NVR] [webhook] failed to marshal event: %v", err)
		return
	}

	for _, cfg := range configs {
		if !d.matches(cfg, &event) {
			continue
		}

		delivery := &db.WebhookDelivery{
			WebhookID: cfg.ID,
			EventType: event.EventType,
			Payload:   string(payload),
			Attempt:   1,
			Status:    "pending",
		}

		if err := d.db.InsertWebhookDelivery(delivery); err != nil {
			log.Printf("[NVR] [webhook] failed to insert delivery: %v", err)
			continue
		}

		select {
		case d.deliveryCh <- &deliveryJob{config: cfg, delivery: delivery, payload: payload}:
		default:
			log.Printf("[NVR] [webhook] delivery channel full, dropping webhook %s", cfg.ID)
		}
	}
}

// matches checks if a webhook config should fire for the given event.
func (d *Dispatcher) matches(cfg *db.WebhookConfig, event *DetectionEvent) bool {
	// Check camera filter.
	if cfg.CameraID != "" && cfg.CameraID != event.CameraID {
		return false
	}

	// Check event type filter.
	if cfg.EventTypes != "" {
		types := strings.Split(cfg.EventTypes, ",")
		found := false
		for _, t := range types {
			if strings.TrimSpace(t) == event.EventType {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// For detection events, check object class and confidence filters.
	if event.EventType == "detection" && event.Detection != nil {
		if cfg.ObjectClasses != "" {
			classes := strings.Split(cfg.ObjectClasses, ",")
			found := false
			for _, c := range classes {
				if strings.TrimSpace(c) == event.Detection.Class {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}

		if cfg.MinConfidence > 0 && event.Detection.Confidence < cfg.MinConfidence {
			return false
		}
	}

	return true
}

func (d *Dispatcher) worker() {
	defer d.wg.Done()

	for {
		select {
		case <-d.ctx.Done():
			return
		case job := <-d.deliveryCh:
			d.send(job)
		}
	}
}

func (d *Dispatcher) send(job *deliveryJob) {
	cfg := job.config
	del := job.delivery

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	ctx, cancel := context.WithTimeout(d.ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(job.payload))
	if err != nil {
		d.failDelivery(del, cfg, fmt.Sprintf("build request: %v", err))
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "MediaMTX-NVR-Webhook/1.0")
	req.Header.Set("X-Webhook-ID", cfg.ID)
	req.Header.Set("X-Delivery-ID", fmt.Sprintf("%d", del.ID))

	// Sign payload with HMAC-SHA256 if secret is configured.
	if cfg.Secret != "" {
		mac := hmac.New(sha256.New, []byte(cfg.Secret))
		mac.Write(job.payload)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Webhook-Signature", "sha256="+sig)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		d.failDelivery(del, cfg, fmt.Sprintf("http error: %v", err))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	del.ResponseStatus = resp.StatusCode
	del.ResponseBody = string(body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		del.Status = "success"
		del.CompletedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		if err := d.db.UpdateWebhookDelivery(del); err != nil {
			log.Printf("[NVR] [webhook] failed to update delivery %d: %v", del.ID, err)
		}
	} else {
		d.failDelivery(del, cfg, fmt.Sprintf("HTTP %d", resp.StatusCode))
	}
}

func (d *Dispatcher) failDelivery(del *db.WebhookDelivery, cfg *db.WebhookConfig, errMsg string) {
	del.Error = errMsg
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	if del.Attempt < maxRetries {
		// Exponential backoff: 2^attempt seconds (2s, 4s, 8s, 16s, ...)
		backoff := time.Duration(1<<uint(del.Attempt)) * time.Second
		del.Status = "retrying"
		del.NextRetryAt = time.Now().UTC().Add(backoff).Format("2006-01-02T15:04:05.000Z")
	} else {
		del.Status = "failed"
		del.CompletedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	}

	if err := d.db.UpdateWebhookDelivery(del); err != nil {
		log.Printf("[NVR] [webhook] failed to update delivery %d: %v", del.ID, err)
	}
}

func (d *Dispatcher) retryLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.processRetries()
		}
	}
}

func (d *Dispatcher) processRetries() {
	deliveries, err := d.db.ListPendingWebhookDeliveries()
	if err != nil {
		log.Printf("[NVR] [webhook] failed to list pending deliveries: %v", err)
		return
	}

	for _, del := range deliveries {
		cfg, err := d.db.GetWebhookConfig(del.WebhookID)
		if err != nil {
			log.Printf("[NVR] [webhook] config %s not found for delivery %d, marking failed", del.WebhookID, del.ID)
			del.Status = "failed"
			del.Error = "webhook config not found"
			del.CompletedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			_ = d.db.UpdateWebhookDelivery(del)
			continue
		}

		del.Attempt++

		select {
		case d.deliveryCh <- &deliveryJob{
			config:   cfg,
			delivery: del,
			payload:  []byte(del.Payload),
		}:
		default:
			log.Printf("[NVR] [webhook] delivery channel full during retry, skipping delivery %d", del.ID)
		}
	}
}
