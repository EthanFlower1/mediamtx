// Package thumbnail provides a background generator that extracts JPEG frames
// from recorded fMP4 segments at configurable intervals.
package thumbnail

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	db "github.com/bluenviron/mediamtx/internal/recorder/db"
)

const (
	// DefaultInterval is the default time between thumbnail extractions.
	DefaultInterval = 10 * time.Second

	// DefaultScanInterval is how often the generator checks for new recordings.
	DefaultScanInterval = 30 * time.Second

	// thumbnailWidth is the output thumbnail width; height is auto-scaled.
	thumbnailWidth = 320
)

// Generator periodically scans for new recordings and extracts JPEG
// thumbnails at a configurable interval within each recording.
type Generator struct {
	db             *db.DB
	recordingsPath string
	interval       time.Duration
	scanInterval   time.Duration

	mu        sync.Mutex
	processed map[int64]bool // recording IDs already processed

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Config holds configuration for the thumbnail generator.
type Config struct {
	DB             *db.DB
	RecordingsPath string
	// Interval between thumbnail frames within a recording. Zero uses DefaultInterval.
	Interval time.Duration
	// ScanInterval is how often to scan for new recordings. Zero uses DefaultScanInterval.
	ScanInterval time.Duration
}

// NewGenerator creates a new thumbnail generator but does not start it.
func NewGenerator(cfg Config) *Generator {
	interval := cfg.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	scanInterval := cfg.ScanInterval
	if scanInterval <= 0 {
		scanInterval = DefaultScanInterval
	}
	return &Generator{
		db:             cfg.DB,
		recordingsPath: cfg.RecordingsPath,
		interval:       interval,
		scanInterval:   scanInterval,
		processed:      make(map[int64]bool),
	}
}

// Start begins the background thumbnail generation loop.
func (g *Generator) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	g.cancel = cancel
	g.wg.Add(1)
	go g.run(ctx)
	log.Printf("[thumbnail] generator started (interval=%s, scan=%s)", g.interval, g.scanInterval)
}

// Stop gracefully shuts down the generator.
func (g *Generator) Stop() {
	if g.cancel != nil {
		g.cancel()
	}
	g.wg.Wait()
	log.Println("[thumbnail] generator stopped")
}

func (g *Generator) run(ctx context.Context) {
	defer g.wg.Done()

	// Run once immediately, then on interval.
	g.scan()

	ticker := time.NewTicker(g.scanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.scan()
		}
	}
}

func (g *Generator) scan() {
	// Look at recordings from the last 24 hours that we haven't processed yet.
	now := time.Now().UTC()
	start := now.Add(-24 * time.Hour)

	cameras, err := g.db.ListCameras()
	if err != nil {
		log.Printf("[thumbnail] failed to list cameras: %v", err)
		return
	}

	for _, cam := range cameras {
		recordings, err := g.db.QueryRecordings(cam.ID, start, now)
		if err != nil {
			log.Printf("[thumbnail] failed to query recordings for camera %s: %v", cam.ID, err)
			continue
		}

		for _, rec := range recordings {
			g.mu.Lock()
			already := g.processed[rec.ID]
			g.mu.Unlock()
			if already {
				continue
			}

			// Only process completed recordings (non-empty end_time).
			if rec.EndTime == "" {
				continue
			}

			if err := g.processRecording(rec); err != nil {
				log.Printf("[thumbnail] failed to process recording %d: %v", rec.ID, err)
				continue
			}

			g.mu.Lock()
			g.processed[rec.ID] = true
			g.mu.Unlock()
		}
	}
}

// ThumbnailDir returns the directory where thumbnails for a camera are stored.
func ThumbnailDir(recordingsPath, cameraID string) string {
	return filepath.Join(recordingsPath, cameraID, "thumbnails")
}

// ThumbnailFilename returns a deterministic filename for a thumbnail at the
// given timestamp.
func ThumbnailFilename(cameraID string, ts time.Time) string {
	return fmt.Sprintf("%s_%s.jpg", cameraID, ts.Format("20060102T150405"))
}

func (g *Generator) processRecording(rec *db.Recording) error {
	if rec.FilePath == "" {
		return nil
	}

	// Verify the recording file exists.
	if _, err := os.Stat(rec.FilePath); os.IsNotExist(err) {
		return nil // file gone, skip silently
	}

	// Parse recording time range.
	startTime, err := time.Parse("2006-01-02T15:04:05.000Z", rec.StartTime)
	if err != nil {
		return fmt.Errorf("parse start_time: %w", err)
	}
	endTime, err := time.Parse("2006-01-02T15:04:05.000Z", rec.EndTime)
	if err != nil {
		return fmt.Errorf("parse end_time: %w", err)
	}

	duration := endTime.Sub(startTime)
	if duration <= 0 {
		return nil
	}

	outDir := ThumbnailDir(g.recordingsPath, rec.CameraID)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("mkdir thumbnails: %w", err)
	}

	// Extract thumbnails at each interval offset.
	for offset := time.Duration(0); offset < duration; offset += g.interval {
		ts := startTime.Add(offset)
		filename := ThumbnailFilename(rec.CameraID, ts)
		outPath := filepath.Join(outDir, filename)

		// Skip if already generated.
		if _, err := os.Stat(outPath); err == nil {
			continue
		}

		if err := extractFrame(rec.FilePath, offset, outPath); err != nil {
			log.Printf("[thumbnail] extract frame at %s from recording %d: %v", offset, rec.ID, err)
			// Continue with remaining frames.
		}
	}

	return nil
}

// extractFrame uses ffmpeg to extract a single JPEG frame from a video file
// at the given offset.
func extractFrame(inputPath string, offset time.Duration, outputPath string) error {
	seekSecs := fmt.Sprintf("%.3f", offset.Seconds())

	//nolint:gosec // inputPath and outputPath are derived from internal state, not user input
	cmd := exec.Command("ffmpeg",
		"-ss", seekSecs,
		"-i", inputPath,
		"-frames:v", "1",
		"-vf", fmt.Sprintf("scale=%d:-1", thumbnailWidth),
		"-q:v", "5",
		"-y",
		outputPath,
	)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		// Clean up partial output.
		os.Remove(outputPath)
		return fmt.Errorf("ffmpeg: %w", err)
	}

	return nil
}

// ListThumbnails returns thumbnail file paths for a camera within the given
// time range, sorted by timestamp ascending.
func ListThumbnails(recordingsPath, cameraID string, start, end time.Time) ([]ThumbnailInfo, error) {
	dir := ThumbnailDir(recordingsPath, cameraID)

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	prefix := cameraID + "_"
	var results []ThumbnailInfo

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".jpg") {
			continue
		}

		// Parse timestamp from filename: {cameraID}_{20060102T150405}.jpg
		tsStr := strings.TrimPrefix(name, prefix)
		tsStr = strings.TrimSuffix(tsStr, ".jpg")
		ts, err := time.Parse("20060102T150405", tsStr)
		if err != nil {
			continue
		}

		if ts.Before(start) || !ts.Before(end) {
			continue
		}

		results = append(results, ThumbnailInfo{
			Timestamp: ts,
			FilePath:  filepath.Join(dir, name),
			Filename:  name,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.Before(results[j].Timestamp)
	})

	return results, nil
}

// ThumbnailInfo describes a single thumbnail image.
type ThumbnailInfo struct {
	Timestamp time.Time `json:"timestamp"`
	FilePath  string    `json:"-"`
	Filename  string    `json:"filename"`
}

// CleanupThumbnails removes all thumbnails for a camera that are older than
// the given cutoff time. It returns the number of files removed.
func CleanupThumbnails(recordingsPath, cameraID string, before time.Time) int {
	dir := ThumbnailDir(recordingsPath, cameraID)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}

	prefix := cameraID + "_"
	removed := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".jpg") {
			continue
		}

		tsStr := strings.TrimPrefix(name, prefix)
		tsStr = strings.TrimSuffix(tsStr, ".jpg")
		ts, err := time.Parse("20060102T150405", tsStr)
		if err != nil {
			continue
		}

		if ts.Before(before) {
			path := filepath.Join(dir, name)
			if os.Remove(path) == nil {
				removed++
			}
		}
	}

	return removed
}
